package jobs

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestCleanupPreservesActiveAndRecovery(t *testing.T) {
	d := Cleanup([]Release{{"active", 1, true}, {"old", 2, false}, {"new", 3, false}}, 1)
	if len(d) != 1 || d[0].ID != "old" {
		t.Fatalf("%v", d)
	}
}

func TestCleanupRetainsTenInactivePlusActive(t *testing.T) {
	rs := []Release{{ID: "active", Active: true}}
	for i := int64(0); i < 12; i++ {
		rs = append(rs, Release{ID: string(rune('a' + i)), Created: i})
	}
	if got := Cleanup(rs, 10); len(got) != 2 || got[0].Created != 1 || got[1].Created != 0 {
		t.Fatalf("unexpected candidates: %#v", got)
	}
}

func TestRemoveReleaseIsContainedAndRetrySafe(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "releases", "app", "hash")
	if err := os.MkdirAll(p, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, "index.html"), []byte("ok"), 0444); err != nil {
		t.Fatal(err)
	}
	if err := RemoveRelease(root, "app", "hash"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("path remained: %v", err)
	}
	if err := RemoveRelease(root, "app", "hash"); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if _, err := ReleasePath(root, "../app", "hash"); !errors.Is(err, ErrUnsafeReleasePath) {
		t.Fatal(err)
	}
}

func TestExecutorDoesNotReportInjectedDeleteFailureAsSuccess(t *testing.T) {
	errInjected := errors.New("read-only filesystem")
	e := Executor{Remove: func(_, _, _ string) error { return errInjected }}
	if err := e.Execute(t.Context(), t.TempDir(), "app", "hash"); !errors.Is(err, errInjected) {
		t.Fatal(err)
	}
}

func TestRemoveReleaseRestoresModesWhenUnsafeChildAbortsDeletion(t *testing.T) {
	root, err := os.MkdirTemp("", "tinkercloud-cleanup-")
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(root, "releases", "app", "hash")
	defer func() {
		_ = os.Chmod(filepath.Join(p, "nested"), 0755)
		_ = os.Chmod(p, 0755)
		_ = os.Remove(filepath.Join(p, "nested", "escape"))
		_ = os.RemoveAll(root)
	}()
	if err := os.MkdirAll(filepath.Join(p, "nested"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(root, filepath.Join(p, "nested", "escape")); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(p, "nested"), 0555); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, 0555); err != nil {
		t.Fatal(err)
	}
	if err := RemoveRelease(root, "app", "hash"); !errors.Is(err, ErrUnsafeReleasePath) {
		t.Fatalf("%v", err)
	}
	info, err := os.Stat(p)
	if err != nil || info.Mode().Perm() != 0555 {
		t.Fatalf("mode was not restored: %v %v", info, err)
	}
}

func TestRemoveReleaseCanReclaimSealedTree(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "releases", "app", "hash")
	if err := os.MkdirAll(filepath.Join(p, "nested"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, "nested", "asset.js"), []byte("ok"), 0444); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(p, "nested"), 0555); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, 0555); err != nil {
		t.Fatal(err)
	}
	if err := RemoveRelease(root, "app", "hash"); err != nil {
		t.Fatalf("sealed cleanup failed: %v", err)
	}
	if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("sealed release remained: %v", err)
	}
}

func TestRemoveAppReleasesRemovesOnlyOneValidatedAppNamespace(t *testing.T) {
	root := t.TempDir()
	for _, app := range []string{"app-a", "app-b"} {
		p := filepath.Join(root, "releases", app, "hash")
		if err := os.MkdirAll(p, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p, "index.html"), []byte(app), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := RemoveAppReleases(root, "app-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "releases", "app-a")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("removed namespace err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "releases", "app-b", "hash", "index.html")); err != nil {
		t.Fatalf("other namespace removed: %v", err)
	}
	if err := RemoveAppReleases(root, "../app-b"); !errors.Is(err, ErrUnsafeReleasePath) {
		t.Fatalf("unsafe app id err=%v", err)
	}
}
