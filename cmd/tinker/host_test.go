package main

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/ChrisMarxDev/tinkercloud/internal/client"
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

func TestHostInstallDerivesPinnedReleaseOnlyForReleasedBuild(t *testing.T) {
	oldRunner, oldVersion := hostRunner, client.BuildVersion
	runner := &cliHostRunner{}
	hostRunner, client.BuildVersion = runner, "0.1.3"
	t.Cleanup(func() { hostRunner, client.BuildVersion = oldRunner, oldVersion })
	var out, stderr strings.Builder
	if code := runWith([]string{"host", "install", "root@host.example"}, &out, &stderr, runnerDeps{}); code != 0 || !runner.called || !strings.Contains(strings.Join(runner.args, " "), "https://github.com/ChrisMarxDev/tinkercloud/releases/download/v0.1.3/") {
		t.Fatalf("released install code=%d called=%v args=%q", code, runner.called, runner.args)
	}
	runner.called = false
	client.BuildVersion = "dev"
	out.Reset()
	stderr.Reset()
	if code := runWith([]string{"host", "install", "root@host.example"}, &out, &stderr, runnerDeps{}); code != 2 || runner.called || !strings.Contains(stderr.String(), "Development builds") {
		t.Fatalf("development install code=%d called=%v stderr=%q", code, runner.called, stderr.String())
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

func TestHostUninstallRequiresExactLocalConfirmationOrExplicitYes(t *testing.T) {
	old := hostRunner
	runner := &cliHostRunner{}
	hostRunner = runner
	t.Cleanup(func() { hostRunner = old })
	var out, stderr strings.Builder
	if code := runWith([]string{"host", "uninstall", "root@host.example"}, &out, &stderr, runnerDeps{input: strings.NewReader("no\n")}); code != 1 || runner.called {
		t.Fatalf("declined uninstall code=%d called=%v", code, runner.called)
	}
	out.Reset()
	stderr.Reset()
	if code := runWith([]string{"host", "uninstall", "root@host.example"}, &out, &stderr, runnerDeps{input: strings.NewReader("uninstall root@host.example\n")}); code != 0 || !runner.called {
		t.Fatalf("confirmed uninstall code=%d called=%v out=%q err=%q", code, runner.called, out.String(), stderr.String())
	}
	if got := strings.Join(runner.args[len(runner.args)-3:], " "); got != "tinkercloud uninstall --confirm-uninstall" {
		t.Fatalf("remote command=%q", got)
	}
	runner.called = false
	if code := runWith([]string{"host", "uninstall", "root@host.example", "--yes"}, &out, &stderr, runnerDeps{}); code != 0 || !runner.called {
		t.Fatalf("noninteractive uninstall code=%d called=%v", code, runner.called)
	}
}
