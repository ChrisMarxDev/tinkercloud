package deployments

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"github.com/tinyhost/tiny/internal/archive"
	"github.com/tinyhost/tiny/internal/operations"
	"github.com/tinyhost/tiny/internal/releases"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type denyWriteGate struct{}

func (denyWriteGate) AllowWrite(context.Context, operations.WriteKind) error {
	return operations.ErrWriteDisabled
}

type countedRepository struct {
	MemoryRepository
	attempts int
	err      error
}

func (r *countedRepository) DeploymentAttempts(context.Context, string, time.Time) (int, error) {
	return r.attempts, r.err
}

type gates struct{ p, c, probe bool }

func (g gates) Policy(context.Context, Record) bool      { return g.p }
func (g gates) Certificate(context.Context, Record) bool { return g.c }
func (g gates) Probe(context.Context, Record) bool       { return g.probe }

func tarGz(name string, data []byte) []byte {
	var b bytes.Buffer
	g := gzip.NewWriter(&b)
	t := tar.NewWriter(g)
	t.WriteHeader(&tar.Header{Name: name, Mode: 0644, Size: int64(len(data))})
	t.Write(data)
	if name == "index.html" {
		m := []byte("version: 1\nname: app\n")
		t.WriteHeader(&tar.Header{Name: "tiny.yaml", Mode: 0644, Size: int64(len(m))})
		t.Write(m)
	}
	t.Close()
	g.Close()
	return b.Bytes()
}
func TestUploadTarGzVerified(t *testing.T) {
	root := t.TempDir()
	defer filepath.Walk(root, func(p string, i os.FileInfo, e error) error {
		if e == nil && i.IsDir() {
			_ = os.Chmod(p, 0755)
		} else if e == nil {
			_ = os.Chmod(p, 0644)
		}
		return nil
	})
	repo := &MemoryRepository{}
	s := &Service{Repo: repo, Root: root, UploadLimit: 1024, Limits: archive.Limits{MaxEntries: 10, MaxDepth: 4, MaxExpanded: 1024, MaxFile: 1024}}
	a := Actor{ID: "owner", Active: true}
	if _, e := s.Create(context.Background(), a, "app", "app", "dep", "key"); e != nil {
		t.Fatal(e)
	}
	if e := s.Upload(context.Background(), a, "dep", "application/gzip", bytes.NewReader(tarGz("index.html", []byte("ok")))); e != nil {
		t.Fatal(e)
	}
	r, e := repo.Get(context.Background(), "dep")
	if e != nil || r.State != releases.Verified || r.ReleaseHash == "" || r.Manifest.Name != "app" {
		t.Fatalf("%+v %v", r, e)
	}
}
func TestUploadWrongOwnerDoesNotConsume(t *testing.T) {
	repo := &MemoryRepository{}
	s := &Service{Repo: repo, Root: t.TempDir(), UploadLimit: 100, Limits: archive.Limits{MaxEntries: 1, MaxDepth: 1, MaxExpanded: 100, MaxFile: 100}}
	a := Actor{ID: "a", Active: true}
	s.Create(context.Background(), a, "app", "app", "dep", "key")
	if e := s.Upload(context.Background(), Actor{ID: "b", Active: true}, "dep", "application/gzip", bytes.NewReader([]byte("bad"))); e != ErrDenied {
		t.Fatal(e)
	}
}

func TestCreateDeniedByDiskGateDoesNotCreateDeployment(t *testing.T) {
	repo := &MemoryRepository{}
	s := &Service{Repo: repo, WriteGate: denyWriteGate{}}
	if _, err := s.Create(context.Background(), Actor{ID: "a", Active: true}, "app", "app", "dep", "key"); !errors.Is(err, operations.ErrWriteDisabled) {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := repo.Get(context.Background(), "dep"); err == nil {
		t.Fatal("deployment was persisted despite write stop")
	}
}

func TestCreateRateLimitDoesNotCreateDeployment(t *testing.T) {
	repo := &countedRepository{attempts: 20}
	s := &Service{Repo: repo, AttemptLimit: 20}
	if _, err := s.Create(context.Background(), Actor{ID: "a", Active: true}, "app", "app", "dep", "key"); !errors.Is(err, ErrRateLimited) {
		t.Fatal(err)
	}
	if _, err := repo.Get(context.Background(), "dep"); err == nil {
		t.Fatal("rate limited deployment was persisted")
	}
}
func TestUploadUnknownTypeFailsClosed(t *testing.T) {
	repo := &MemoryRepository{}
	s := &Service{Repo: repo, Root: t.TempDir(), UploadLimit: 10, Limits: archive.Limits{MaxEntries: 1, MaxDepth: 1, MaxExpanded: 10, MaxFile: 10}}
	a := Actor{ID: "a", Active: true}
	s.Create(context.Background(), a, "app", "app", "dep", "k")
	if e := s.Upload(context.Background(), a, "dep", "text/plain", bytes.NewReader([]byte("x"))); e == nil {
		t.Fatal("accepted type")
	}
}
func TestUploadTraversalFails(t *testing.T) {
	root := t.TempDir()
	repo := &MemoryRepository{}
	s := &Service{Repo: repo, Root: root, UploadLimit: 1000, Limits: archive.Limits{MaxEntries: 5, MaxDepth: 4, MaxExpanded: 1000, MaxFile: 1000}}
	a := Actor{ID: "a", Active: true}
	s.Create(context.Background(), a, "app", "app", "dep", "k")
	if e := s.Upload(context.Background(), a, "dep", "application/gzip", bytes.NewReader(tarGz("../x", []byte("x")))); e == nil {
		t.Fatal("traversal accepted")
	}
	r, _ := repo.Get(context.Background(), "dep")
	if r.State != releases.Rejected {
		t.Fatal(r.State)
	}
}
func TestUploadZipRetained(t *testing.T) {
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	f, _ := w.Create("index.html")
	f.Write([]byte("ok"))
	m, _ := w.Create("tiny.yaml")
	m.Write([]byte("version: 1\nname: app\n"))
	w.Close()
	root := t.TempDir()
	defer filepath.Walk(root, func(p string, i os.FileInfo, e error) error {
		if e == nil && i.IsDir() {
			os.Chmod(p, 0755)
		} else if e == nil {
			os.Chmod(p, 0644)
		}
		return nil
	})
	repo := &MemoryRepository{}
	s := &Service{Repo: repo, Root: root, UploadLimit: 1000, Limits: archive.Limits{MaxEntries: 5, MaxDepth: 4, MaxExpanded: 1000, MaxFile: 1000}}
	a := Actor{ID: "a", Active: true}
	s.Create(context.Background(), a, "app", "app", "dep", "k")
	if e := s.Upload(context.Background(), a, "dep", "application/zip", bytes.NewReader(b.Bytes())); e != nil {
		t.Fatal(e)
	}
}
func TestUploadLimitFailureCleansStaging(t *testing.T) {
	root := t.TempDir()
	repo := &MemoryRepository{}
	s := &Service{Repo: repo, Root: root, UploadLimit: 2, Limits: archive.Limits{MaxEntries: 2, MaxDepth: 2, MaxExpanded: 10, MaxFile: 10}}
	a := Actor{ID: "a", Active: true}
	s.Create(context.Background(), a, "app", "app", "dep", "k")
	if e := s.Upload(context.Background(), a, "dep", "application/gzip", bytes.NewReader([]byte("too-large"))); e == nil {
		t.Fatal("limit accepted")
	}
	r, _ := repo.Get(context.Background(), "dep")
	if r.State != releases.Failed {
		t.Fatal(r.State)
	}
	matches, _ := filepath.Glob(filepath.Join(root, "staging-*"))
	if len(matches) != 0 {
		t.Fatal(matches)
	}
}
func TestZipSymlinkFailureCleansStaging(t *testing.T) {
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	h := &zip.FileHeader{Name: "link"}
	h.SetMode(os.ModeSymlink | 0777)
	f, _ := w.CreateHeader(h)
	f.Write([]byte("target"))
	w.Close()
	root := t.TempDir()
	repo := &MemoryRepository{}
	s := &Service{Repo: repo, Root: root, UploadLimit: 1000, Limits: archive.Limits{MaxEntries: 2, MaxDepth: 2, MaxExpanded: 100, MaxFile: 100}}
	a := Actor{ID: "a", Active: true}
	s.Create(context.Background(), a, "app", "app", "dep", "k")
	if e := s.Upload(context.Background(), a, "dep", "application/zip", bytes.NewReader(b.Bytes())); e == nil {
		t.Fatal("symlink accepted")
	}
	r, _ := repo.Get(context.Background(), "dep")
	if r.State != releases.Rejected {
		t.Fatal(r.State)
	}
	matches, _ := filepath.Glob(filepath.Join(root, "staging-*"))
	if len(matches) != 0 {
		t.Fatal(matches)
	}
}
func TestActivateGatesAndOwner(t *testing.T) {
	repo := &MemoryRepository{Records: map[string]Record{"old": {Deployment: releases.Deployment{ID: "old", AppID: "app", State: releases.Active}, OwnerID: "a"}, "new": {Deployment: releases.Deployment{ID: "new", AppID: "app", State: releases.Verified}, OwnerID: "a"}}, Current: map[string]string{"app": "old"}}
	s := &Service{Repo: repo, Gates: gates{true, true, true}}
	if e := s.Activate(context.Background(), Actor{ID: "b", Active: true}, "new", "k"); e != ErrDenied {
		t.Fatal(e)
	}
	if e := s.Activate(context.Background(), Actor{ID: "a", Active: true}, "new", "k"); e != nil {
		t.Fatal(e)
	}
	r, _ := repo.Get(context.Background(), "new")
	if r.State != releases.Active {
		t.Fatal(r.State)
	}
}
func TestActivateGateDenialPreservesPointer(t *testing.T) {
	for _, g := range []gates{{false, true, true}, {true, false, true}, {true, true, false}} {
		repo := &MemoryRepository{Records: map[string]Record{"old": {Deployment: releases.Deployment{ID: "old", AppID: "app", State: releases.Active}, OwnerID: "a"}, "new": {Deployment: releases.Deployment{ID: "new", AppID: "app", State: releases.Verified}, OwnerID: "a"}}, Current: map[string]string{"app": "old"}}
		s := &Service{Repo: repo, Gates: g}
		if s.Activate(context.Background(), Actor{ID: "a", Active: true}, "new", "k") == nil {
			t.Fatal("gate accepted")
		}
		if repo.Current["app"] != "old" {
			t.Fatal("pointer changed")
		}
	}
}

func TestActivateCommitFailurePreservesPreviousRelease(t *testing.T) {
	repo := &MemoryRepository{
		Records: map[string]Record{
			"old": {Deployment: releases.Deployment{ID: "old", AppID: "app", State: releases.Active}, OwnerID: "a"},
			"new": {Deployment: releases.Deployment{ID: "new", AppID: "app", State: releases.Verified}, OwnerID: "a"},
		},
		Current:    map[string]string{"app": "old"},
		FailCommit: errors.New("sqlite injected"),
	}
	s := &Service{Repo: repo, Gates: gates{true, true, true}}
	if err := s.Activate(context.Background(), Actor{ID: "a", Active: true}, "new", "request"); err == nil {
		t.Fatal("commit failure accepted")
	}
	if repo.Current["app"] != "old" {
		t.Fatalf("previous release pointer changed: %q", repo.Current["app"])
	}
	old, _ := repo.Get(context.Background(), "old")
	next, _ := repo.Get(context.Background(), "new")
	if old.State != releases.Active || next.State != releases.Verified {
		t.Fatalf("activation was partially committed: old=%s next=%s", old.State, next.State)
	}
}
