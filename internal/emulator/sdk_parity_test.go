package emulator

import (
	"context"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// TestBuiltSDKParity runs the compiled browser SDK against the real loopback
// emulator. It intentionally covers only the local supported subset: KV,
// document collections, and collection freshness/reconnect recovery.
func TestBuiltSDKParity(t *testing.T) {
	if _, err := exec.LookPath("npm"); err != nil {
		t.Skip("npm is required for built SDK parity")
	}
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is required for built SDK parity")
	}
	s := app(t, t.TempDir())
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	root := repositoryRoot(t)
	build := exec.Command("npm", "run", "build")
	build.Dir = filepath.Join(root, "sdk", "typescript")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build SDK: %v: %s", err, out)
	}
	run := exec.CommandContext(context.Background(), "node", "test/emulator-parity.mjs")
	run.Dir = filepath.Join(root, "sdk", "typescript")
	run.Env = append(run.Environ(), "TINY_EMULATOR_PARITY_ORIGIN="+ts.URL)
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("built SDK/emulator parity: %v: %s", err, out)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("discover emulator test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
