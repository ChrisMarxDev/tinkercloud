package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDevConfigUsesManifestOutputAndProjectLocalState(t *testing.T) {
	p := t.TempDir()
	if err := os.Mkdir(filepath.Join(p, "dist"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, "tinker.yaml"), []byte("version: 1\nname: local-checklist\nbuild:\n  output: dist\n"), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := devConfig([]string{p, "--listen", "127.0.0.1:8788"})
	if err != nil {
		t.Fatal(err)
	}
	if c.AppDir != filepath.Join(p, "dist") || c.StateDir != filepath.Join(p, ".tinker", "local") || c.Slug != "local-checklist" || c.Listen != "127.0.0.1:8788" {
		t.Fatalf("config %#v", c)
	}
}
func TestDevConfigFallsBackToSuppliedDirectoryAndRejectsUnsafeInput(t *testing.T) {
	p := t.TempDir()
	c, err := devConfig([]string{"--listen", "127.0.0.1:8788", p})
	if err != nil {
		t.Fatal(err)
	}
	if c.AppDir != p || c.Listen != "127.0.0.1:8788" {
		t.Fatalf("config %#v", c)
	}
	if _, err = devConfig([]string{"one", "two"}); err == nil {
		t.Fatal("multiple directories accepted")
	}
	if _, err = devConfig([]string{"--listen", "0.0.0.0:8787", p}); err != nil {
		t.Fatal("loopback is enforced by emulator construction, not ambiguous cli parsing")
	}
}
func TestRunWithCapturesDevUsage(t *testing.T) {
	var out, err strings.Builder
	code := runWith([]string{"--json", "dev"}, &out, &err, runnerDeps{})
	if code != 2 || !strings.Contains(out.String(), "tinker dev is a local server") {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), err.String())
	}
}
