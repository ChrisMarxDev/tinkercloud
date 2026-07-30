package deployments

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func valid(t *testing.T) string {
	r := t.TempDir()
	os.WriteFile(filepath.Join(r, "tinker.yaml"), []byte("version: 1\nname: demo\nspa:\n  fallback: index.html\n"), 0600)
	os.WriteFile(filepath.Join(r, "index.html"), []byte("ok"), 0600)
	return r
}
func TestValidateRelease(t *testing.T) {
	r := valid(t)
	if _, e := ValidateRelease(r, "demo"); e != nil {
		t.Fatal(e)
	}
}
func TestValidateReleaseDenials(t *testing.T) {
	r := valid(t)
	os.Remove(filepath.Join(r, "index.html"))
	if _, e := ValidateRelease(r, "demo"); e == nil {
		t.Fatal("missing index")
	}
	r = valid(t)
	if _, e := ValidateRelease(r, "other"); e == nil {
		t.Fatal("mismatch")
	}
	r = t.TempDir()
	os.WriteFile(filepath.Join(r, "tinker.yaml"), []byte(strings.Repeat("x", 65537)), 0600)
	if _, e := ValidateRelease(r, "demo"); e == nil {
		t.Fatal("oversize")
	}
}
func TestValidateReleaseMissingAndFallback(t *testing.T) {
	r := t.TempDir()
	if _, e := ValidateRelease(r, "demo"); e == nil {
		t.Fatal("missing manifest")
	}
	r = valid(t)
	os.WriteFile(filepath.Join(r, "tinker.yaml"), []byte("version: 1\nname: demo\nspa:\n  fallback: missing.html\n"), 0600)
	if _, e := ValidateRelease(r, "demo"); e == nil {
		t.Fatal("missing fallback")
	}
}
func TestValidateReleaseLinksDeny(t *testing.T) {
	r := valid(t)
	outside := filepath.Join(t.TempDir(), "outside")
	os.WriteFile(outside, []byte("x"), 0600)
	os.Remove(filepath.Join(r, "index.html"))
	if e := os.Symlink(outside, filepath.Join(r, "index.html")); e == nil {
		if _, e := ValidateRelease(r, "demo"); e == nil {
			t.Fatal("index link")
		}
	}
}
