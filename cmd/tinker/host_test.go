package main

import (
	"context"
	"io"
	"strings"
	"testing"
)

type cliHostRunner struct {
	called bool
	args   []string
	stdin  []byte
}

func (r *cliHostRunner) Run(_ context.Context, name string, args []string, stdin []byte, _, _ io.Writer) error {
	if name != "ssh" {
		panic("unexpected executable")
	}
	r.called, r.args, r.stdin = true, append([]string(nil), args...), append([]byte(nil), stdin...)
	return nil
}

func TestHostInstallRoutesReviewedBootstrap(t *testing.T) {
	old := hostRunner
	runner := &cliHostRunner{}
	hostRunner = runner
	t.Cleanup(func() { hostRunner = old })
	var out, stderr strings.Builder
	code := runWith([]string{"host", "install", "root@host.example", "--release-base", "https://releases.example/v0.1.0/"}, &out, &stderr, runnerDeps{})
	if code != 0 || !runner.called || len(runner.stdin) == 0 {
		t.Fatalf("code=%d called=%v stdin=%d out=%q err=%q", code, runner.called, len(runner.stdin), out.String(), stderr.String())
	}
}

func TestHostCommandDeniesJSONAndArbitraryTarget(t *testing.T) {
	var out, stderr strings.Builder
	if code := runWith([]string{"--json", "host", "status", "root@host.example"}, &out, &stderr, runnerDeps{}); code != 2 {
		t.Fatalf("JSON host code = %d", code)
	}
	out.Reset()
	if code := runWith([]string{"host", "status", "root@host;id"}, &out, &stderr, runnerDeps{}); code != 2 {
		t.Fatalf("unsafe target code = %d", code)
	}
}
