package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAtomicPrivate(t *testing.T) {
	p := filepath.Join(t.TempDir(), "state")
	if e := writeAtomicPrivate(p, []byte("one")); e != nil {
		t.Fatal(e)
	}
	if e := writeAtomicPrivate(p, []byte("two")); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(p)
	st, _ := os.Stat(p)
	if string(b) != "two" || st.Mode().Perm() != 0600 {
		t.Fatal(string(b), st.Mode())
	}
	link := filepath.Join(t.TempDir(), "link")
	os.Symlink(p, link)
	if e := writeAtomicPrivate(link, []byte("x")); e == nil {
		t.Fatal("symlink accepted")
	}
}
