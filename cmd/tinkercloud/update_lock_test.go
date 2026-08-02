package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUpdateLockIsExclusiveAndReusableAfterRelease(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	unlock, err := acquireUpdateLock(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := acquireUpdateLock(root); err == nil {
		t.Fatal("concurrent update lock acquired")
	}
	unlock()
	unlockAgain, err := acquireUpdateLock(root)
	if err != nil {
		t.Fatalf("released update lock was stranded: %v", err)
	}
	unlockAgain()
}

func TestUpdateLockRejectsSymlink(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	updateDir := filepath.Join(root, "update")
	if err := os.Mkdir(updateDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "elsewhere"), filepath.Join(updateDir, ".lock")); err != nil {
		t.Fatal(err)
	}
	if _, err := acquireUpdateLock(root); err == nil {
		t.Fatal("symlink update lock accepted")
	}
}
