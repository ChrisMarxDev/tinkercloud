package blob

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestLocalStoreRecoveryOnlyCleansValidatedStagingNames(t *testing.T) {
	s := LocalStore{Root: t.TempDir()}
	app := "a"
	if _, _, e := s.Put(app, "blb_0123456789abcdef0123456789abcdef", strings.NewReader("x"), 10); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(s.Root+"/blobs/a/.staging-0123456789abcdef0123456789abcdef", []byte("x"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, _, e := s.Keys(Key{}, 100); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(s.Root + "/blobs/a/.staging-0123456789abcdef0123456789abcdef"); !os.IsNotExist(e) {
		t.Fatalf("validated staging stayed: %v", e)
	}
	if e := os.WriteFile(s.Root+"/blobs/a/.staging-not-ours", []byte("x"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, _, e := s.Keys(Key{}, 100); e == nil {
		t.Fatal("unexpected entry accepted")
	}
	if _, e := os.Stat(s.Root + "/blobs/a/.staging-not-ours"); e != nil {
		t.Fatalf("unexpected entry was deleted: %v", e)
	}
}

func TestLocalStoreMissingNamespaceIsEmptyAndDeleteIsIdempotent(t *testing.T) {
	s := LocalStore{Root: t.TempDir()}
	if keys, more, err := s.Keys(Key{}, 10); err != nil || more || len(keys) != 0 {
		t.Fatalf("empty namespace keys=%v more=%t err=%v", keys, more, err)
	}
	if err := s.Delete("missing", "blb_0123456789abcdef0123456789abcdef"); err != nil {
		t.Fatalf("missing delete: %v", err)
	}
}

func TestLocalStoreRemoveAppNamespaceRemovesOnlyEmptyValidatedTarget(t *testing.T) {
	s := LocalStore{Root: t.TempDir()}
	id := "blb_0123456789abcdef0123456789abcdef"
	for _, app := range []string{"app-a", "app-b"} {
		if _, _, err := s.Put(app, id, strings.NewReader("x"), 10); err != nil {
			t.Fatal(err)
		}
		if err := s.Delete(app, id); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.RemoveAppNamespace("app-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.Root + "/blobs/app-a"); !os.IsNotExist(err) {
		t.Fatalf("target namespace remained: %v", err)
	}
	if _, err := os.Stat(s.Root + "/blobs/app-b"); err != nil {
		t.Fatalf("other namespace removed: %v", err)
	}
	if err := s.RemoveAppNamespace("../app-b"); err == nil {
		t.Fatal("unsafe namespace accepted")
	}
	if err := s.RemoveAppNamespace("app-b"); err != nil {
		t.Fatal(err)
	}
}

func TestLocalStoreSyncAndFinalizeFailureLeaveNoReadableObject(t *testing.T) {
	cases := []struct {
		name string
		set  func()
	}{
		{"sync", func() { localSync = func(*os.File) error { return errors.New("sync") } }},
		{"rename", func() { localRename = func(*os.Root, string, string) error { return errors.New("rename") } }},
		{"directory sync", func() { localSyncDir = func(*os.Root) error { return errors.New("directory sync") } }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			localSync = func(f *os.File) error { return f.Sync() }
			localRename = func(r *os.Root, old, new string) error { return r.Rename(old, new) }
			localSyncDir = syncRootDir
			defer func() {
				localSync = func(f *os.File) error { return f.Sync() }
				localRename = func(r *os.Root, old, new string) error { return r.Rename(old, new) }
				localSyncDir = syncRootDir
			}()
			tc.set()
			s := LocalStore{Root: t.TempDir()}
			id := "blb_0123456789abcdef0123456789abcdef"
			if _, _, e := s.Put("a", id, strings.NewReader("x"), 10); e == nil {
				t.Fatal("failure accepted")
			}
			if _, _, _, e := s.Open("a", id); e == nil {
				t.Fatal("partial readable")
			}
		})
	}
}

func syncRootDir(r *os.Root) error {
	d, err := r.Open(".")
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

func TestLocalStoreRejectsPathLikeAppIDsBeforeRootAccess(t *testing.T) {
	s := LocalStore{Root: t.TempDir() + "/does-not-exist"}
	id := "blb_0123456789abcdef0123456789abcdef"
	for _, app := range []string{".", "..", "a/b", "a\\b", "a\x00b"} {
		t.Run(strings.NewReplacer("/", "slash", "\\", "backslash", "\x00", "nul").Replace(app), func(t *testing.T) {
			if _, _, err := s.Put(app, id, strings.NewReader("x"), 10); err == nil {
				t.Fatalf("unsafe app %q accepted", app)
			}
		})
	}
}

func TestLocalStoreKeysRejectsCorruptPathLikeNamespace(t *testing.T) {
	s := LocalStore{Root: t.TempDir()}
	if err := os.MkdirAll(s.Root+"/blobs/a\\b", 0700); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Keys(Key{}, 10); err == nil {
		t.Fatal("corrupt namespace accepted")
	}
}
func TestLocalStoreRejectsParentNamespaceSymlink(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, root+"/blobs"); err != nil {
		t.Fatal(err)
	}
	s := LocalStore{Root: root}
	id := "blb_0123456789abcdef0123456789abcdef"
	if _, _, err := s.Put("a", id, strings.NewReader("x"), 10); err == nil {
		t.Fatal("symlinked parent accepted")
	}
	if _, err := os.Stat(outside + "/a/" + id); !os.IsNotExist(err) {
		t.Fatalf("escaped root: %v", err)
	}
}
func TestLocalStoreRejectsEmptyUpload(t *testing.T) {
	s := LocalStore{Root: t.TempDir()}
	id := "blb_0123456789abcdef0123456789abcdef"
	if _, _, e := s.Put("a", id, strings.NewReader(""), 10); e == nil {
		t.Fatal("empty upload accepted")
	}
	if _, _, _, e := s.Open("a", id); e == nil {
		t.Fatal("empty upload became readable")
	}
}
