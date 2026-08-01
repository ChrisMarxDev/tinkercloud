package update

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFileInstallerSnapshotApplyRestore(t *testing.T) {
	d := t.TempDir()
	target := filepath.Join(d, "bin")
	old := []byte("old")
	os.WriteFile(target, old, 0755)
	f := FileInstaller{Target: target, RollbackDir: filepath.Join(d, "rollback")}
	if e := f.Snapshot(context.Background()); e != nil {
		t.Fatal(e)
	}
	if e := f.Apply(context.Background(), Artifact{Bytes: []byte("new")}); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(target)
	if string(b) != "new" {
		t.Fatal(string(b))
	}
	if e := f.Restore(context.Background()); e != nil {
		t.Fatal(e)
	}
	b, _ = os.ReadFile(target)
	if string(b) != "old" {
		t.Fatal(string(b))
	}
	if e := f.Commit(context.Background()); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(d, "rollback", "previous")); !os.IsNotExist(e) {
		t.Fatal("snapshot remains", e)
	}
}
func TestFileInstallerRejectsSymlink(t *testing.T) {
	d := t.TempDir()
	target := filepath.Join(d, "bin")
	os.WriteFile(target, []byte("x"), 0755)
	link := filepath.Join(d, "link")
	os.Symlink(target, link)
	if e := (FileInstaller{Target: link, RollbackDir: filepath.Join(d, "r")}).Snapshot(context.Background()); e == nil {
		t.Fatal("symlink accepted")
	}
}
