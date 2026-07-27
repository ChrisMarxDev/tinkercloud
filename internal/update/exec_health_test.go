package update

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func healthScript(t *testing.T, body string) string {
	p := filepath.Join(t.TempDir(), "x")
	if e := os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0755); e != nil {
		t.Fatal(e)
	}
	return p
}
func TestExecHealth(t *testing.T) {
	p := healthScript(t, "test \"$1\" = doctor && test \"$2\" = --config && test \"$3\" = cfg && test \"$4\" = --update-health-check && test \"$X\" = y\n")
	if e := (ExecHealth{Binary: p, Config: "cfg", Env: []string{"X=y"}}).Check(context.Background()); e != nil {
		t.Fatal(e)
	}
	for _, body := range []string{"exit 1", "sleep 2", "head -c 70000 /dev/zero"} {
		p = healthScript(t, body)
		if e := (ExecHealth{Binary: p, Timeout: time.Millisecond}).Check(context.Background()); e != ErrHealth {
			t.Fatal(e)
		}
	}
}
func TestExecHealthRejectsSymlink(t *testing.T) {
	p := healthScript(t, "exit 0")
	l := filepath.Join(t.TempDir(), "l")
	os.Symlink(p, l)
	if e := (ExecHealth{Binary: l}).Check(context.Background()); e != ErrHealth {
		t.Fatal(e)
	}
}
