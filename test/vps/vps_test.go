package vps

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/tinyhost/tiny/internal/releases"
)

func env(values map[string]string) func(string) string {
	return func(k string) string { return values[k] }
}
func configEnv(t *testing.T) map[string]string {
	t.Helper()
	d := t.TempDir()
	kh := filepath.Join(d, "known_hosts")
	key := filepath.Join(d, "resend")
	if err := os.WriteFile(kh, []byte("host key\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(key, []byte("re_test\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return map[string]string{EnvEnabled: "1", EnvTarget: "root@203.0.113.10", EnvAcknowledge: "root@203.0.113.10", EnvKnownHosts: kh, "TINYHOST_VPS_PLATFORM_HOST": "tiny.example.test", "TINYHOST_VPS_APP_SUFFIX": "apps.example.test", "TINYHOST_VPS_OPERATOR_EMAIL": "operator@example.test", "TINYHOST_VPS_DEPLOYER_EMAIL": "deployer@example.test", "TINYHOST_VPS_VIEWER_EMAIL": "viewer@example.test", "TINYHOST_VPS_EMAIL_FROM": "tiny@example.test", "TINYHOST_VPS_ACME_EMAIL": "admin@example.test", "TINYHOST_VPS_RESEND_API_KEY_FILE": key}
}
func TestLoadConfigRequiresExplicitGateAndAcknowledgement(t *testing.T) {
	v := configEnv(t)
	v[EnvEnabled] = ""
	if _, e := LoadConfig(env(v)); e == nil {
		t.Fatal("accepted disabled live run")
	}
	v[EnvEnabled] = "1"
	v[EnvAcknowledge] = "wrong"
	if _, e := LoadConfig(env(v)); e == nil {
		t.Fatal("accepted wrong acknowledgement")
	}
	v[EnvAcknowledge] = v[EnvTarget]
	c, e := LoadConfig(env(v))
	if e != nil {
		t.Fatal(e)
	}
	if c.Target != "root@203.0.113.10" {
		t.Fatal(c.Target)
	}
}
func TestSSHAndSCPUseStrictHostKeyArguments(t *testing.T) {
	c := Config{Target: "root@host", KnownHosts: "/safe/known_hosts", Port: "2222", IdentityFile: "/safe/key"}
	s := Suite{Config: c}
	a := strings.Join(s.sshArgs(), " ")
	for _, want := range []string{"BatchMode=yes", "StrictHostKeyChecking=yes", "UserKnownHostsFile=/safe/known_hosts", "-p 2222", "-i /safe/key"} {
		if !strings.Contains(a, want) {
			t.Fatalf("missing %s: %s", want, a)
		}
	}
}

func TestRemoteArgumentsAreQuotedBeforeOpenSSHRemoteShell(t *testing.T) {
	f := &calls{}
	s := Suite{Config: Config{Target: "root@host", KnownHosts: "/kh"}, Runner: f}
	if err := s.remote(context.Background(), "tinyhost", "deployers", "authorize", "a'; touch /pwned; echo 'b"); err != nil {
		t.Fatal(err)
	}
	if len(f.got) != 1 {
		t.Fatal("remote command was not invoked")
	}
	got := f.got[0][len(f.got[0])-1]
	if strings.Contains(got, "; touch /pwned;") && !strings.Contains(got, "'\\''") {
		t.Fatalf("unquoted remote source: %s", got)
	}
	if !strings.HasPrefix(got, "'tinyhost' 'deployers' 'authorize' ") {
		t.Fatalf("unexpected remote argv: %s", got)
	}
}

func TestConfigRejectsNonRootAndSameIdentity(t *testing.T) {
	v := configEnv(t)
	v[EnvTarget], v[EnvAcknowledge] = "admin@203.0.113.10", "admin@203.0.113.10"
	if _, err := LoadConfig(env(v)); err == nil {
		t.Fatal("non-root target accepted")
	}
	v[EnvTarget], v[EnvAcknowledge] = "root@203.0.113.10", "root@203.0.113.10"
	v["TINYHOST_VPS_VIEWER_EMAIL"] = v["TINYHOST_VPS_DEPLOYER_EMAIL"]
	if _, err := LoadConfig(env(v)); err == nil {
		t.Fatal("same deployer and viewer accepted")
	}
}

func TestConfigRejectsOutOfRangeSSHPort(t *testing.T) {
	v := configEnv(t)
	v["TINYHOST_VPS_SSH_PORT"] = "65536"
	if _, err := LoadConfig(env(v)); err == nil {
		t.Fatal("out-of-range SSH port accepted")
	}
}

func TestPrepareReleaseVerifiesCompleteEvidenceBeforeStagingE2EKey(t *testing.T) {
	prepared, err := prepareRelease(context.Background(), t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	if prepared.publicKey == "" || filepath.Dir(prepared.publicKey) == prepared.dir {
		t.Fatalf("E2E verification key was staged as release evidence: %+v", prepared)
	}
	if _, err := os.Stat(filepath.Join(prepared.dir, "release-public-key.pem")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unsigned auxiliary key present in verified release: %v", err)
	}
	// The verifier must continue to deny an otherwise valid release once an
	// unsigned file is added. The previous ordering added this exact kind of
	// file before verification, which made preparation fail closed.
	if err := os.WriteFile(filepath.Join(prepared.dir, "release-public-key.pem"), []byte("unsigned auxiliary key\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := commandEnv(context.Background(), []string{"TINYHOST_RELEASE_PUBLIC_KEY=" + prepared.publicKey}, repoPath("scripts", "release-verify.sh"), prepared.dir); err == nil {
		t.Fatal("release verifier accepted incomplete release evidence")
	}
}

type calls struct {
	got  [][]string
	fail bool
	out  []byte
}

type initRetryRunner struct {
	initFailures int
	state        []byte
	initCalls    int
	calls        [][]string
	terminalOut  []byte
}

func (r *initRetryRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, append([]string{name}, args...))
	remote := args[len(args)-1]
	switch {
	case strings.Contains(remote, "'/usr/local/bin/tinyhost' 'init'"):
		r.initCalls++
		if r.terminalOut != nil {
			return r.terminalOut, errors.New("exit status 1")
		}
		if r.initCalls <= r.initFailures {
			return []byte("tinyhost: public_health_failed\n"), errors.New("exit status 1")
		}
		return nil, nil
	case strings.Contains(remote, "'cat' '/etc/tinyhost/init-state.json'"):
		return r.state, nil
	default:
		return nil, errors.New("unexpected remote command")
	}
}

func verificationPendingState(t *testing.T) []byte {
	t.Helper()
	return []byte(`{"completed":{"preflight":true,"paths":true,"database":true,"operator":true,"service":true}}`)
}

func TestInitReadinessRetriesOnlyPersistedPublicHealth(t *testing.T) {
	r := &initRetryRunner{initFailures: 2, state: verificationPendingState(t)}
	waits := 0
	s := Suite{Config: Config{Target: "root@host", KnownHosts: "/kh"}, Runner: r, RetryWait: func(ctx context.Context, d time.Duration) error {
		if d != initReadinessDelay || ctx.Err() != nil {
			t.Fatalf("unexpected retry wait: duration=%s err=%v", d, ctx.Err())
		}
		waits++
		return nil
	}}
	argv := []string{"/usr/local/bin/tinyhost", "init", "--non-interactive", "--platform-host", "tiny.example.test"}
	if err := s.initWithReadinessRetry(context.Background(), argv...); err != nil {
		t.Fatal(err)
	}
	if r.initCalls != 3 || waits != 2 {
		t.Fatalf("init calls=%d waits=%d", r.initCalls, waits)
	}
	var initRemote string
	for _, call := range r.calls {
		remote := call[len(call)-1]
		if strings.Contains(remote, "'/usr/local/bin/tinyhost' 'init'") {
			if initRemote == "" {
				initRemote = remote
			} else if remote != initRemote {
				t.Fatalf("init argv changed across retry: first=%q next=%q", initRemote, remote)
			}
			continue
		}
		if !strings.Contains(remote, "'cat' '/etc/tinyhost/init-state.json'") {
			t.Fatalf("retry performed unrelated mutation: %q", remote)
		}
	}
}

func TestInitReadinessDoesNotRetryTerminalOrUnconfirmedFailures(t *testing.T) {
	t.Run("terminal init result", func(t *testing.T) {
		r := &initRetryRunner{terminalOut: []byte("tinyhost: config_invalid\n")}
		waited := false
		s := Suite{Config: Config{Target: "root@host", KnownHosts: "/kh"}, Runner: r, RetryWait: func(context.Context, time.Duration) error { waited = true; return nil }}
		if err := s.initWithReadinessRetry(context.Background(), "/usr/local/bin/tinyhost", "init"); err == nil {
			t.Fatal("terminal config failure retried")
		}
		if r.initCalls != 1 || waited || len(r.calls) != 1 {
			t.Fatalf("calls=%d waited=%t all=%d", r.initCalls, waited, len(r.calls))
		}
	})
	t.Run("state is not final verification", func(t *testing.T) {
		r := &initRetryRunner{initFailures: 1, state: []byte(`{"completed":{"preflight":true}}`)}
		s := Suite{Config: Config{Target: "root@host", KnownHosts: "/kh"}, Runner: r, RetryWait: func(context.Context, time.Duration) error { t.Fatal("unexpected retry"); return nil }}
		if err := s.initWithReadinessRetry(context.Background(), "/usr/local/bin/tinyhost", "init"); err == nil {
			t.Fatal("unconfirmed public health failure retried")
		}
		if r.initCalls != 1 || len(r.calls) != 2 {
			t.Fatalf("calls=%d all=%d", r.initCalls, len(r.calls))
		}
	})
}

func TestInitReadinessRetryHonorsContextCancellation(t *testing.T) {
	r := &initRetryRunner{initFailures: 1, state: verificationPendingState(t)}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// The init attempt itself still receives the context (as a real SSH command
	// would); the injected runner makes its cancellation-independent outcome
	// deterministic so this test can assert the retry wait stops immediately.
	s := Suite{Config: Config{Target: "root@host", KnownHosts: "/kh"}, Runner: r, RetryWait: func(got context.Context, _ time.Duration) error {
		if got != ctx {
			t.Fatal("retry wait lost caller context")
		}
		return got.Err()
	}}
	if err := s.initWithReadinessRetry(ctx, "/usr/local/bin/tinyhost", "init"); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context cancellation", err)
	}
	if r.initCalls != 1 {
		t.Fatalf("canceled retry reran init %d times", r.initCalls)
	}
}

func (f *calls) Run(_ context.Context, n string, a ...string) ([]byte, error) {
	f.got = append(f.got, append([]string{n}, a...))
	if f.fail {
		return []byte("exists"), errors.New("failed")
	}
	return f.out, nil
}
func TestCleanGuardRejectsExistingHostAndReuseIsExplicit(t *testing.T) {
	f := &calls{fail: true}
	s := Suite{Config: Config{Target: "root@host", KnownHosts: "/kh"}, Runner: f}
	if e := s.cleanGuard(context.Background()); e == nil || !strings.Contains(e.Error(), "not clean") {
		t.Fatalf("%v", e)
	}
	s.Config.Reuse = true
	f.fail = false
	f.out = []byte(s.reuseMarker())
	if e := s.cleanGuard(context.Background()); e != nil {
		t.Fatal(e)
	}
	if len(f.got) != 2 || !strings.Contains(strings.Join(f.got[1], " "), "cat") {
		t.Fatal("reuse should verify only its marker before mutation")
	}
}
func TestHiddenTransactionAndMarkerDenial(t *testing.T) {
	if got := hiddenValue(`<input name="transaction" value="otp_x">`, "transaction"); got != "otp_x" {
		t.Fatal(got)
	}
	if got := hiddenValue(`<input name="transaction" value="otp_x">`, "email"); got != "" {
		t.Fatal(got)
	}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func denialSuite(status func(string) int, body func(string) string) Suite {
	return Suite{HTTP: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: status(r.URL.Path), Body: io.NopCloser(strings.NewReader(body(r.URL.Path))), Header: make(http.Header), Request: r}, nil
	})}}
}

func TestAnonymousDeniedRequiresEverySurfaceToDenyWithoutMarker(t *testing.T) {
	good := denialSuite(func(string) int { return http.StatusUnauthorized }, func(string) string { return "safe" })
	if err := good.anonymousDenied(context.Background(), "app.example.test", "MARKER"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		suite Suite
	}{
		{"success", denialSuite(func(string) int { return http.StatusOK }, func(string) string { return "safe" })},
		{"gateway_failure", denialSuite(func(string) int { return http.StatusInternalServerError }, func(string) string { return "safe" })},
		{"marker", denialSuite(func(string) int { return http.StatusUnauthorized }, func(string) string { return "MARKER" })},
		{"websocket", denialSuite(func(path string) int {
			if path == "/_tiny/ws/v1" {
				return http.StatusSwitchingProtocols
			}
			return http.StatusUnauthorized
		}, func(string) string { return "safe" })},
	} {
		if err := tc.suite.anonymousDenied(context.Background(), "app.example.test", "MARKER"); err == nil {
			t.Fatalf("%s response was accepted", tc.name)
		}
	}
}

func TestRestartBlobReadinessRetriesOnlyTransientStartupFailures(t *testing.T) {
	attempts, waits := 0, []time.Duration{}
	h := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		attempts++
		switch attempts {
		case 1:
			return nil, &url.Error{Op: "Get", URL: r.URL.String(), Err: syscall.ECONNREFUSED}
		case 2:
			return &http.Response{StatusCode: http.StatusServiceUnavailable, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("starting")), Request: r}, nil
		case 3:
			headers := make(http.Header)
			headers.Set("Cache-Control", "private, no-store")
			headers.Set("X-Content-Type-Options", "nosniff")
			headers.Set("Content-Disposition", "attachment; filename=exact-bytes.bin")
			return &http.Response{StatusCode: http.StatusOK, Header: headers, Body: io.NopCloser(bytes.NewReader(vpsBlobBytes)), Request: r}, nil
		default:
			t.Fatalf("unexpected readiness attempt %d", attempts)
			return nil, nil
		}
	})}
	s := Suite{RestartReadinessWait: func(ctx context.Context, delay time.Duration) error {
		if ctx.Err() != nil {
			t.Fatal("readiness wait received cancelled context")
		}
		waits = append(waits, delay)
		return nil
	}}
	if err := s.verifyBlobPersistsAfterRestart(context.Background(), h, "app.example.test", "blob_1"); err != nil {
		t.Fatal(err)
	}
	if attempts != 3 {
		t.Fatalf("attempts=%d, want 3", attempts)
	}
	if want := []time.Duration{restartBlobInitialDelay, restartBlobInitialDelay * 2}; !slices.Equal(waits, want) {
		t.Fatalf("waits=%v, want %v", waits, want)
	}
}

func TestRestartBlobReadinessRejectsNonTransientOrInvalidEvidenceWithoutRetry(t *testing.T) {
	for _, tc := range []struct {
		name     string
		response func(*http.Request) (*http.Response, error)
	}{
		{
			name: "authorization denial",
			response: func(r *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusUnauthorized, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("denied")), Request: r}, nil
			},
		},
		{
			name: "redirect",
			response: func(r *http.Request) (*http.Response, error) {
				headers := make(http.Header)
				headers.Set("Location", "https://other.example.test/")
				return &http.Response{StatusCode: http.StatusFound, Header: headers, Body: io.NopCloser(strings.NewReader("redirect")), Request: r}, nil
			},
		},
		{
			name: "wrong bytes with success status",
			response: func(r *http.Request) (*http.Response, error) {
				headers := make(http.Header)
				headers.Set("Cache-Control", "private, no-store")
				headers.Set("X-Content-Type-Options", "nosniff")
				headers.Set("Content-Disposition", "attachment; filename=exact-bytes.bin")
				return &http.Response{StatusCode: http.StatusOK, Header: headers, Body: io.NopCloser(strings.NewReader("wrong")), Request: r}, nil
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			attempts, waits := 0, 0
			h := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
				attempts++
				return tc.response(r)
			})}
			s := Suite{RestartReadinessWait: func(context.Context, time.Duration) error { waits++; return nil }}
			if err := s.verifyBlobPersistsAfterRestart(context.Background(), h, "app.example.test", "blob_1"); err == nil {
				t.Fatal("invalid restart evidence accepted")
			}
			if attempts != 1 || waits != 0 {
				t.Fatalf("attempts=%d waits=%d, want one request and no retry", attempts, waits)
			}
		})
	}
}

func TestRestartBlobReadinessWaitHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	attempts := 0
	h := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		attempts++
		return nil, &url.Error{Op: "Get", URL: r.URL.String(), Err: syscall.ECONNREFUSED}
	})}
	s := Suite{RestartReadinessWait: func(got context.Context, _ time.Duration) error {
		cancel()
		return got.Err()
	}}
	if err := s.verifyBlobPersistsAfterRestart(ctx, h, "app.example.test", "blob_1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context cancellation", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts=%d, want 1", attempts)
	}
}

func TestSocketInventoryMatchesExactPublicPorts(t *testing.T) {
	f := &calls{out: []byte("LISTEN 0 4096 *:8080 *:* users:((\"tinyhost\",pid=9,fd=1))\n")}
	s := Suite{Config: Config{Target: "root@host", KnownHosts: "/kh"}, Runner: f}
	if err := s.socketInventory(context.Background()); err == nil {
		t.Fatal("8080 was confused with port 80")
	}
	f.out = []byte("LISTEN 0 4096 *:80 *:* users:((\"tinyhost\",pid=9,fd=1))\nLISTEN 0 4096 [::]:443 [::]:* users:((\"tinyhost\",pid=9,fd=1))\n")
	if err := s.socketInventory(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.out = append(f.out, []byte("LISTEN 0 4096 *:8080 *:* users:((\"tinyhost\",pid=9,fd=2))\n")...)
	if err := s.socketInventory(context.Background()); err == nil {
		t.Fatal("additional tinyhost public listener was accepted")
	}
	f.out = []byte("LISTEN 0 4096 *:80 *:* users:((\"tinyhost\",pid=9,fd=1))\nLISTEN 0 4096 [::]:443 [::]:* users:((\"tinyhost\",pid=9,fd=1))\nLISTEN 0 4096 *:8080 *:* users:((\"operator-service\",pid=10,fd=1))\n")
	if err := s.socketInventory(context.Background()); err != nil {
		t.Fatal("operator-owned listener was attributed to tinyhost:", err)
	}
	f.out = []byte("LISTEN 0 4096 *:80 *:* users:((\"not-tinyhost\",pid=9,fd=1))\nLISTEN 0 4096 [::]:443 [::]:* users:((\"tinyhost\",pid=9,fd=1))\n")
	if err := s.socketInventory(context.Background()); err == nil {
		t.Fatal("process-name substring was accepted")
	}
}

func TestSmokeArchiveIsDeployableAndUsesUniqueMarker(t *testing.T) {
	viewer := "viewer@example.test"
	a, sizeA, markerA, err := smokeArchive("vps-smoke-a", viewer)
	if err != nil || len(a) == 0 || sizeA != int64(len(a)) || markerA == "" {
		t.Fatalf("archive A: bytes=%d size=%d marker=%q err=%v", len(a), sizeA, markerA, err)
	}
	m := smokeManifest(t, a)
	if m.Name != "vps-smoke-a" || !m.Blobs || len(m.Emails) != 1 || m.Emails[0] != viewer || len(m.Domains) != 0 {
		t.Fatalf("smoke policy = %#v", m)
	}
	_, _, markerB, err := smokeArchive("vps-smoke-b", viewer)
	if err != nil || markerA == markerB {
		t.Fatalf("archive B marker=%q err=%v", markerB, err)
	}
}

func smokeManifest(t *testing.T, archive []byte) releases.Manifest {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	h, err := tr.Next()
	if err != nil || h.Name != "tiny.yaml" {
		t.Fatalf("first archive entry = %#v, %v", h, err)
	}
	b, err := io.ReadAll(tr)
	if err != nil {
		t.Fatal(err)
	}
	m, err := releases.ParseManifest(b)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestVPSAcceptance(t *testing.T) {
	c, e := LoadConfig(os.Getenv)
	if e != nil {
		t.Skip(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	if e = (&Suite{Config: c}).Run(ctx); e != nil {
		t.Fatal(e)
	}
}
