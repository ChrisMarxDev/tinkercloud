package persistence

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/tinyhost/tiny/internal/blob"
	"github.com/tinyhost/tiny/internal/controlapi"
)

type fakeBlobBytes struct {
	putErr, openErr error
	installed       map[string]bool
}

type signalingAppDataPurger struct {
	inner   AppDataPurger
	entered chan struct{}
}

func (p signalingAppDataPurger) Purge(ctx context.Context, appID string) error {
	close(p.entered)
	return p.inner.Purge(ctx, appID)
}

func k(app, id string) string { return app + "/" + id }
func (f *fakeBlobBytes) Put(app, id string, r io.Reader, _ int64) (int64, string, error) {
	_, _ = io.ReadAll(r)
	if f.putErr != nil {
		return 0, "", f.putErr
	}
	if f.installed == nil {
		f.installed = map[string]bool{}
	}
	f.installed[k(app, id)] = true
	return 1, "hash", nil
}
func (f *fakeBlobBytes) Open(app, id string) (io.ReadCloser, int64, string, error) {
	if f.openErr != nil {
		return nil, 0, "", f.openErr
	}
	if !f.installed[k(app, id)] {
		return nil, 0, "", errors.New("missing")
	}
	return io.NopCloser(strings.NewReader("x")), 1, "hash", nil
}
func (f *fakeBlobBytes) Delete(app, id string) error { delete(f.installed, k(app, id)); return nil }
func (f *fakeBlobBytes) RemoveAppNamespace(app string) error {
	for key := range f.installed {
		if strings.HasPrefix(key, app+"/") {
			delete(f.installed, key)
		}
	}
	return nil
}
func (f *fakeBlobBytes) Keys(after blob.Key, limit int) ([]blob.Key, bool, error) {
	out := []blob.Key{}
	for x := range f.installed {
		p := strings.Split(x, "/")
		key := blob.Key{App: p[0], ID: p[1]}
		if key.App > after.App || (key.App == after.App && key.ID > after.ID) {
			out = append(out, key)
		}
	}
	return out, len(out) >= limit, nil
}
func TestBlobRepositoryFailureStatesAreNeverReadableAndReconcileOrphans(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	b := &fakeBlobBytes{putErr: errors.New("sync failed")}
	r := &BlobRepository{Store: s, Bytes: b}
	m := blob.Metadata{ID: "blb_0123456789abcdef0123456789abcdef", Name: "x.txt", ContentType: "text/plain"}
	if _, e := r.Upload(context.Background(), "a", "i", m, strings.NewReader("x"), blob.DefaultLimits()); e == nil {
		t.Fatal("write failure accepted")
	}
	var n int
	if e := s.DB.QueryRow("SELECT COUNT(*) FROM app_blobs").Scan(&n); e != nil || n != 0 {
		t.Fatalf("rows=%d err=%v", n, e)
	}
	b.putErr = nil
	b.installed = map[string]bool{k("a", m.ID): true}
	if e := r.Reconcile(context.Background()); e != nil {
		t.Fatal(e)
	}
	if b.installed[k("a", m.ID)] {
		t.Fatal("orphan remained")
	}
}
func TestBlobRepositoryCrossAppAndMissingBytesFailClosed(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	b := &fakeBlobBytes{}
	r := &BlobRepository{Store: s, Bytes: b}
	m := blob.Metadata{ID: "blb_0123456789abcdef0123456789abcdef", Name: "x.txt", ContentType: "text/plain"}
	if _, e := r.Upload(context.Background(), "a", "i", m, strings.NewReader("x"), blob.DefaultLimits()); e != nil {
		t.Fatal(e)
	}
	if _, _, e := r.Open(context.Background(), "b", m.ID); e == nil {
		t.Fatal("cross app opened")
	}
	delete(b.installed, k("a", m.ID))
	if _, _, e := r.Open(context.Background(), "a", m.ID); !errors.Is(e, blob.ErrUnavailable) {
		t.Fatalf("missing bytes=%v", e)
	}
}

func TestBlobRepositoryReconcileRemovesMissingReadyCatalogWithoutQuotaGhost(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	b := &fakeBlobBytes{}
	r := &BlobRepository{Store: s, Bytes: b}
	m := blob.Metadata{ID: "blb_0123456789abcdef0123456789abcdef", Name: "x.txt", ContentType: "text/plain"}
	if _, e := r.Upload(context.Background(), "a", "i", m, strings.NewReader("x"), blob.DefaultLimits()); e != nil {
		t.Fatal(e)
	}
	delete(b.installed, k("a", m.ID))
	if e := r.Reconcile(context.Background()); e != nil {
		t.Fatal(e)
	}
	var n int
	if e := s.DB.QueryRow("SELECT COUNT(*) FROM app_blobs WHERE app_id='a' AND id=?", m.ID).Scan(&n); e != nil || n != 0 {
		t.Fatalf("corrupt ready catalog remained n=%d err=%v", n, e)
	}
}
func TestBlobRepositoryReadyCommitFailureLeavesNoReadyMetadataOrBytes(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	b := &fakeBlobBytes{}
	r := &BlobRepository{Store: s, Bytes: b, BeforeReadyCommit: func() error { return errors.New("commit") }}
	m := blob.Metadata{ID: "blb_0123456789abcdef0123456789abcdef", Name: "x.txt", ContentType: "text/plain"}
	if _, e := r.Upload(context.Background(), "a", "i", m, strings.NewReader("x"), blob.DefaultLimits()); e == nil {
		t.Fatal("commit failure accepted")
	}
	var n int
	_ = s.DB.QueryRow("SELECT COUNT(*) FROM app_blobs WHERE state='ready'").Scan(&n)
	if n != 0 || b.installed[k("a", m.ID)] {
		t.Fatalf("ready=%d bytes=%t", n, b.installed[k("a", m.ID)])
	}
}

func TestBlobRepositoryAppDeletionCannotRaceUpload(t *testing.T) {
	limits := blob.DefaultLimits()
	newMetadata := func(id string) blob.Metadata {
		return blob.Metadata{ID: id, Name: "x.txt", ContentType: "text/plain"}
	}
	t.Run("deleted app cannot acquire staging", func(t *testing.T) {
		s := seeded(t)
		defer s.Close()
		b := &fakeBlobBytes{}
		r := &BlobRepository{Store: s, Bytes: b}
		if _, err := s.DB.Exec("UPDATE applications SET status='deleted' WHERE id='a'"); err != nil {
			t.Fatal(err)
		}
		m := newMetadata("blb_0123456789abcdef0123456789abcdef")
		if _, err := r.Upload(context.Background(), "a", "i", m, strings.NewReader("x"), limits); !errors.Is(err, blob.ErrUnavailable) {
			t.Fatalf("deleted app staging err=%v", err)
		}
		var n int
		if err := s.DB.QueryRow("SELECT COUNT(*) FROM app_blobs WHERE app_id='a'").Scan(&n); err != nil || n != 0 || b.installed != nil {
			t.Fatalf("deleted app acquired blob row=%d bytes=%v err=%v", n, b.installed, err)
		}
	})

	t.Run("delete before ready prevents finalization", func(t *testing.T) {
		s := seeded(t)
		defer s.Close()
		b := &fakeBlobBytes{}
		entered, release := make(chan struct{}), make(chan struct{})
		r := &BlobRepository{Store: s, Bytes: b, BeforeReadyCommit: func() error {
			close(entered)
			<-release
			return nil
		}}
		m := newMetadata("blb_1123456789abcdef0123456789abcdef")
		out := make(chan error, 1)
		go func() {
			_, err := r.Upload(context.Background(), "a", "i", m, strings.NewReader("x"), limits)
			out <- err
		}()
		<-entered
		purgeEntered := make(chan struct{})
		deleteResult := make(chan error, 1)
		go func() {
			deleteResult <- (ControlService{Store: s, AppDataCleanup: signalingAppDataPurger{
				inner:   AppDataPurger{DataRoot: s.DataRoot, Store: s, BlobCleanup: r},
				entered: purgeEntered,
			}}).DeleteApp(context.Background(), controlapi.Actor{ID: "u", Active: true}, "alpha", "delete-before-ready")
		}()
		<-purgeEntered
		close(release)
		if err := <-out; !errors.Is(err, blob.ErrUnavailable) {
			t.Fatalf("upload finalized after deletion: %v", err)
		}
		if err := <-deleteResult; err != nil {
			t.Fatal(err)
		}
		if err := r.Reconcile(context.Background()); err != nil {
			t.Fatal(err)
		}
		assertNoReadyOrBytes(t, s, b, m)
	})

	t.Run("ready before deletion is removed by cleanup", func(t *testing.T) {
		s := seeded(t)
		defer s.Close()
		b := &fakeBlobBytes{}
		r := &BlobRepository{Store: s, Bytes: b}
		m := newMetadata("blb_2123456789abcdef0123456789abcdef")
		if _, err := r.Upload(context.Background(), "a", "i", m, strings.NewReader("x"), limits); err != nil {
			t.Fatal(err)
		}
		if err := (ControlService{Store: s, AppDataCleanup: AppDataPurger{DataRoot: s.DataRoot, Store: s, BlobCleanup: r}}).DeleteApp(context.Background(), controlapi.Actor{ID: "u", Active: true}, "alpha", "ready-before-delete"); err != nil {
			t.Fatal(err)
		}
		if err := r.Reconcile(context.Background()); err != nil {
			t.Fatal(err)
		}
		assertNoReadyOrBytes(t, s, b, m)
	})
}

func assertNoReadyOrBytes(t *testing.T, s *SQLiteStore, b *fakeBlobBytes, m blob.Metadata) {
	t.Helper()
	var n int
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM app_blobs WHERE app_id='a' AND id=? AND state='ready'", m.ID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("ready catalog remained=%d err=%v", n, err)
	}
	if b.installed != nil && b.installed[k("a", m.ID)] {
		t.Fatal("deleted-app bytes remained")
	}
}
