package hostops

import (
	"context"
	"io"
	"reflect"
	"testing"
)

func TestFixedHostPlans(t *testing.T) {
	status, err := Build("status", "root@host.example", "")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-o", "BatchMode=yes", "-o", "ClearAllForwardings=yes", "-T", "--", "root@host.example", "tinkercloud", "status"}
	if !reflect.DeepEqual(status.Args, want) || len(status.Stdin) != 0 {
		t.Fatalf("status plan = %#v", status)
	}
	install, err := Build("install", "root@192.0.2.10", "https://releases.example/v0.1.0/")
	if err != nil || len(install.Stdin) == 0 || install.Args[len(install.Args)-1] != "'https://releases.example/v0.1.0/'" {
		t.Fatalf("install plan = %#v, %v", install, err)
	}
	uninstall, err := Build("uninstall", "root@host.example", "")
	if err != nil {
		t.Fatal(err)
	}
	wantUninstall := []string{"-o", "BatchMode=yes", "-o", "ClearAllForwardings=yes", "-T", "--", "root@host.example", "tinkercloud", "uninstall", "--confirm-uninstall"}
	if !reflect.DeepEqual(uninstall.Args, wantUninstall) || len(uninstall.Stdin) != 0 {
		t.Fatalf("uninstall plan = %#v", uninstall)
	}
}

func TestHostPlanDenials(t *testing.T) {
	targets := []string{"host.example", "admin@host.example", "root@-oProxyCommand=x", "root@host:22", "root@host;id", "-root@host"}
	for _, target := range targets {
		if _, err := Build("status", target, ""); err == nil {
			t.Fatalf("accepted target %q", target)
		}
	}
	origins := []string{"", "http://releases.example/v1", "https://user@releases.example/v1", "https://127.0.0.1/v1", "https://releases.example:8443/v1", "https://releases.example/v1?q=1"}
	for _, origin := range origins {
		if _, err := Build("install", "root@host.example", origin); err == nil {
			t.Fatalf("accepted origin %q", origin)
		}
	}
	if _, err := Build("exec", "root@host.example", ""); err == nil {
		t.Fatal("accepted arbitrary operation")
	}
	if _, err := Build("uninstall", "root@host.example", "https://releases.example/v1"); err == nil {
		t.Fatal("accepted uninstall release base")
	}
}

type recordingRunner struct {
	name  string
	args  []string
	stdin []byte
}

func (r *recordingRunner) Run(_ context.Context, name string, args []string, stdin []byte, _, _ io.Writer) error {
	r.name, r.args, r.stdin = name, append([]string(nil), args...), append([]byte(nil), stdin...)
	return nil
}

func TestExecuteUsesOnlySSH(t *testing.T) {
	plan, _ := Build("doctor", "root@host.example", "")
	runner := &recordingRunner{}
	if err := Execute(context.Background(), runner, plan, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	if runner.name != "ssh" || !reflect.DeepEqual(runner.args, plan.Args) {
		t.Fatalf("execution = %q %#v", runner.name, runner.args)
	}
}
