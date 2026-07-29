// Package hostops provides the fixed, workstation-side SSH grammar used by
// `tiny host`. It does not own credentials, a listener, or a remote shell.
package hostops

import (
	"context"
	_ "embed"
	"errors"
	"io"
	"net"
	"net/url"
	"os/exec"
	"strings"
)

//go:embed bootstrap.sh
var bootstrap []byte

var ErrInvalid = errors.New("invalid host operation")

type Plan struct {
	Args  []string
	Stdin []byte
}

func Build(operation, target, releaseBase string) (Plan, error) {
	if !validTarget(target) {
		return Plan{}, ErrInvalid
	}
	baseArgs := []string{"-o", "BatchMode=yes", "-o", "ClearAllForwardings=yes", "-T", "--", target}
	switch operation {
	case "status", "doctor":
		if releaseBase != "" {
			return Plan{}, ErrInvalid
		}
		return Plan{Args: append(baseArgs, "tinyhost", operation)}, nil
	case "install":
		if !validReleaseBase(releaseBase) {
			return Plan{}, ErrInvalid
		}
		return Plan{Args: append(baseArgs, "sh", "-s", "--", shellQuote(releaseBase)), Stdin: append([]byte(nil), bootstrap...)}, nil
	case "update":
		if !validReleaseBase(releaseBase) {
			return Plan{}, ErrInvalid
		}
		return Plan{Args: append(baseArgs, "tinyhost", "update", "--release-base", shellQuote(releaseBase))}, nil
	default:
		return Plan{}, ErrInvalid
	}
}

func validTarget(target string) bool {
	if !strings.HasPrefix(target, "root@") || len(target) > 260 || strings.ContainsAny(target, " \t\r\n\x00/\\:;|&$`'\"<>") {
		return false
	}
	host := strings.TrimPrefix(target, "root@")
	if host == "" || strings.HasPrefix(host, "-") {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.To4() != nil
	}
	if len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
				return false
			}
		}
	}
	return true
}

func validReleaseBase(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	if u.Port() != "" && u.Port() != "443" {
		return false
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil && (!ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast()) {
		return false
	}
	return !strings.ContainsAny(raw, "\r\n\x00")
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

type Runner interface {
	Run(context.Context, string, []string, []byte, io.Writer, io.Writer) error
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args []string, stdin []byte, stdout, stderr io.Writer) error {
	command := exec.CommandContext(ctx, name, args...)
	command.Stdin = strings.NewReader(string(stdin))
	command.Stdout = stdout
	command.Stderr = stderr
	return command.Run()
}

func Execute(ctx context.Context, runner Runner, plan Plan, stdout, stderr io.Writer) error {
	if runner == nil || len(plan.Args) == 0 {
		return ErrInvalid
	}
	return runner.Run(ctx, "ssh", plan.Args, plan.Stdin, stdout, stderr)
}
