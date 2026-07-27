package releases

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeRelease(t *testing.T, root, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "assets"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "assets", "app.js"), []byte("console.log(1)"), 0644); err != nil {
		t.Fatal(err)
	}
}

func resetFSHooks() {
	fsMkdirAll = os.MkdirAll
	fsRename = os.Rename
	fsChmod = os.Chmod
	fsSyncDir = syncDir
	fsSyncTree = syncTree
}

func unsealForCleanup(root string) {
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err == nil {
			if info.IsDir() {
				return os.Chmod(path, 0755)
			}
			return os.Chmod(path, 0644)
		}
		return nil
	})
}

func TestFinalizeSealsAndReusesOnlyExactContent(t *testing.T) {
	defer resetFSHooks()
	data := t.TempDir()
	defer unsealForCleanup(data)
	first := filepath.Join(data, "candidate-one")
	writeRelease(t, first, "one")
	dest, evidence, err := Finalize(data, "app", first)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(first); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("candidate still present: %v", err)
	}
	info, err := os.Stat(filepath.Join(dest, "index.html"))
	if err != nil || info.Mode().Perm() != 0444 {
		t.Fatalf("release not sealed: %v %v", info, err)
	}
	second := filepath.Join(data, "candidate-two")
	writeRelease(t, second, "one")
	got, same, err := Finalize(data, "app", second)
	if err != nil || got != dest || !same.Equal(evidence) {
		t.Fatalf("identical release was not safely reused: %q %#v %v", got, same, err)
	}
	if err := os.Chmod(filepath.Join(dest, "index.html"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "index.html"), []byte("tampered"), 0644); err != nil {
		t.Fatal(err)
	}
	third := filepath.Join(data, "candidate-three")
	writeRelease(t, third, "one")
	if _, _, err := Finalize(data, "app", third); !errors.Is(err, ErrReleaseFS) {
		t.Fatalf("corrupt content-addressed destination accepted: %v", err)
	}
}

func TestFinalizePropagatesSealAndRenameFailures(t *testing.T) {
	defer resetFSHooks()
	data := t.TempDir()
	defer unsealForCleanup(data)
	candidate := filepath.Join(data, "candidate")
	writeRelease(t, candidate, "one")
	fsChmod = func(string, os.FileMode) error { return errors.New("chmod injected") }
	if _, _, err := Finalize(data, "app", candidate); err == nil {
		t.Fatal("chmod failure accepted")
	}
	if _, err := os.Stat(candidate); err != nil {
		t.Fatalf("failed candidate unexpectedly moved: %v", err)
	}
	resetFSHooks()
	fsRename = func(string, string) error { return errors.New("rename injected") }
	if _, _, err := Finalize(data, "app", candidate); err == nil {
		t.Fatal("rename failure accepted")
	}
	matches, _ := filepath.Glob(filepath.Join(data, "releases", "app", "*"))
	if len(matches) != 0 {
		t.Fatalf("failed finalization published a release: %v", matches)
	}
	resetFSHooks()
	fsSyncTree = func(string) error { return errors.New("fsync injected") }
	if _, _, err := Finalize(data, "app", candidate); err == nil {
		t.Fatal("sync failure accepted")
	}
	matches, _ = filepath.Glob(filepath.Join(data, "releases", "app", "*"))
	if len(matches) != 0 {
		t.Fatalf("sync failure published a release: %v", matches)
	}
}

func TestInspectRejectsSymlinkRoot(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := Inspect(filepath.Join(root, "link")); !errors.Is(err, ErrReleaseFS) {
		t.Fatalf("symlink root accepted: %v", err)
	}
}
