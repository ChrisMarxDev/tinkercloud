package vps

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/ChrisMarxDev/tinkercloud/internal/client"
	"github.com/ChrisMarxDev/tinkercloud/internal/releases"
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
	return map[string]string{EnvEnabled: "1", EnvTarget: "root@203.0.113.10", EnvAcknowledge: "root@203.0.113.10", EnvKnownHosts: kh, "TINKERCLOUD_VPS_DOMAIN": "example.test", "TINKERCLOUD_VPS_OPERATOR_EMAIL": "operator@example.test", "TINKERCLOUD_VPS_DEPLOYER_EMAIL": "deployer@example.test", "TINKERCLOUD_VPS_VIEWER_EMAIL": "viewer@example.test", "TINKERCLOUD_VPS_EMAIL_FROM": "tinker@example.test", "TINKERCLOUD_VPS_RESEND_API_KEY_FILE": key}
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

func TestLoadPublicExampleConfigRequiresItsOwnExactOptInAndStrictVPSConfig(t *testing.T) {
	v := configEnv(t)
	if _, err := LoadPublicExampleConfig(env(v)); err == nil {
		t.Fatal("public example check accepted without its exact opt-in")
	}
	v[EnvPublicExample] = "true"
	if _, err := LoadPublicExampleConfig(env(v)); err == nil {
		t.Fatal("public example check accepted non-exact opt-in")
	}
	v[EnvPublicExample] = "1"
	v[EnvEnabled] = ""
	if _, err := LoadPublicExampleConfig(env(v)); err == nil {
		t.Fatal("public example check bypassed strict VPS configuration")
	}
	v[EnvEnabled] = "1"
	if _, err := LoadPublicExampleConfig(env(v)); err != nil {
		t.Fatalf("public example check rejected strict configuration: %v", err)
	}
}

func TestReuseConfigRequiresRealLocalReleaseDirectory(t *testing.T) {
	v := configEnv(t)
	v["TINKERCLOUD_VPS_REUSE"] = "1"
	if _, err := LoadConfig(env(v)); err == nil {
		t.Fatal("reuse accepted without a release directory")
	}
	release := filepath.Join(t.TempDir(), "release")
	if err := os.Mkdir(release, 0755); err != nil {
		t.Fatal(err)
	}
	v["TINKERCLOUD_VPS_RELEASE_DIR"] = release
	if _, err := LoadConfig(env(v)); err != nil {
		t.Fatalf("reuse rejected real release directory: %v", err)
	}
	link := filepath.Join(t.TempDir(), "release-link")
	if err := os.Symlink(release, link); err != nil {
		t.Fatal(err)
	}
	v["TINKERCLOUD_VPS_RELEASE_DIR"] = link
	if _, err := LoadConfig(env(v)); err == nil {
		t.Fatal("reuse accepted symlinked release directory")
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
	if err := s.remote(context.Background(), "tinkercloud", "deployers", "authorize", "a'; touch /pwned; echo 'b"); err != nil {
		t.Fatal(err)
	}
	if len(f.got) != 1 {
		t.Fatal("remote command was not invoked")
	}
	got := f.got[0][len(f.got[0])-1]
	if strings.Contains(got, "; touch /pwned;") && !strings.Contains(got, "'\\''") {
		t.Fatalf("unquoted remote source: %s", got)
	}
	if !strings.HasPrefix(got, "'tinkercloud' 'deployers' 'authorize' ") {
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
	v["TINKERCLOUD_VPS_VIEWER_EMAIL"] = v["TINKERCLOUD_VPS_DEPLOYER_EMAIL"]
	if _, err := LoadConfig(env(v)); err == nil {
		t.Fatal("same deployer and viewer accepted")
	}
	v = configEnv(t)
	v["TINKERCLOUD_VPS_VIEWER_EMAIL"] = v["TINKERCLOUD_VPS_OPERATOR_EMAIL"]
	if _, err := LoadConfig(env(v)); err == nil {
		t.Fatal("same operator and viewer accepted")
	}
}

func TestDashboardOwnershipExpectationIsRoleAware(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		operator, deployer   string
		operatorReadsAllApps bool
	}{
		{name: "distinct operator and deployer", operator: "operator@example.test", deployer: "deployer@example.test", operatorReadsAllApps: false},
		{name: "same operator and deployer", operator: "owner@example.test", deployer: "owner@example.test", operatorReadsAllApps: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := Suite{Config: Config{OperatorEmail: tc.operator, DeployerEmail: tc.deployer}}
			if got := s.deployerDashboardIsOperator(); got != tc.operatorReadsAllApps {
				t.Fatalf("operator dashboard scope = %t, want %t", got, tc.operatorReadsAllApps)
			}
		})
	}
}

func TestConfigRequiresOneCanonicalRootDomain(t *testing.T) {
	v := configEnv(t)
	v["TINKERCLOUD_VPS_DOMAIN"] = "Admin.Example.Test"
	if _, err := LoadConfig(env(v)); err == nil {
		t.Fatal("non-canonical root domain accepted")
	}
	v["TINKERCLOUD_VPS_DOMAIN"] = "admin.example.test"
	if _, err := LoadConfig(env(v)); err != nil {
		t.Fatalf("canonical root domain rejected: %v", err)
	}
}

func TestGlobalIdentityHandoffCallbackIsExact(t *testing.T) {
	const handoff = "handoff_opaque"
	if !isAppHandoffCallback("https://alpha.example.test/_tinker/auth/callback?handoff="+handoff, handoff) {
		t.Fatal("valid opaque callback rejected")
	}
	for _, raw := range []string{
		"/_tinker/auth/callback?handoff=" + handoff,
		"https://alpha.example.test/_tinker/auth/callback?handoff=" + handoff + "&return=https://evil.example",
		"https://alpha.example.test/_tinker/auth/callback?handoff=other",
		"https://alpha.example.test/_tinker/auth/login?handoff=" + handoff,
	} {
		if isAppHandoffCallback(raw, handoff) {
			t.Fatalf("accepted unsafe generic callback %q", raw)
		}
	}
	if !isExactHandoffCallback("https://alpha.example.test/_tinker/auth/callback?handoff="+handoff, "alpha.example.test", handoff) {
		t.Fatal("exact callback rejected")
	}
	for _, raw := range []string{
		"https://beta.example.test/_tinker/auth/callback?handoff=" + handoff,
		"https://alpha.example.test/_tinker/auth/callback?handoff=" + handoff + "&x=1",
		"https://alpha.example.test/_tinker/auth/callback?handoff=other",
	} {
		if isExactHandoffCallback(raw, "alpha.example.test", handoff) {
			t.Fatalf("accepted wrong-app or malformed callback %q", raw)
		}
	}
}

func TestCookieScopeAssertionsRequirePlatformAndPerAppCookies(t *testing.T) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	platform, _ := url.Parse("https://admin.example.test/")
	first, _ := url.Parse("https://first.example.test/")
	second, _ := url.Parse("https://second.example.test/")
	jar.SetCookies(platform, []*http.Cookie{
		{Name: "__Host-tinker_identity", Value: "identity", Path: "/", Secure: true},
		{Name: "__Host-tinker_browser", Value: "browser-profile", Path: "/", Secure: true},
	})
	jar.SetCookies(first, []*http.Cookie{
		{Name: "__Host-tinker_app", Value: "first", Path: "/", Secure: true},
		{Name: "__Host-tinker_identity_state", Value: "state", Path: "/", Secure: true},
	})
	jar.SetCookies(second, []*http.Cookie{{Name: "__Host-tinker_app", Value: "second", Path: "/", Secure: true}})
	h := &http.Client{Jar: jar}
	s := Suite{Config: Config{Domain: "example.test"}}
	if err := s.assertCookieScopes(h, "first.example.test", ""); err != nil {
		t.Fatal(err)
	}
	if err := assertDistinctAppCookies(h, "first.example.test", "second.example.test"); err != nil {
		t.Fatal(err)
	}
	if state, ok := identityStateCookie(h, "first.example.test"); !ok || state != "state" {
		t.Fatal("app-host handoff state was not retained for replay evidence")
	}
	if !hasCookie(jar.Cookies(platform), "__Host-tinker_browser") || hasCookie(jar.Cookies(first), "__Host-tinker_browser") || hasCookie(jar.Cookies(second), "__Host-tinker_browser") {
		t.Fatal("platform browser binding was not retained as an exact-host cookie")
	}
	jar.SetCookies(second, []*http.Cookie{{Name: "__Host-tinker_app", Value: "first", Path: "/", Secure: true}})
	if err := assertDistinctAppCookies(h, "first.example.test", "second.example.test"); err == nil {
		t.Fatal("accepted shared app token across app hosts")
	}
}

func TestConfigRejectsOutOfRangeSSHPort(t *testing.T) {
	v := configEnv(t)
	v["TINKERCLOUD_VPS_SSH_PORT"] = "65536"
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
	if err := commandEnv(context.Background(), []string{"TINKERCLOUD_RELEASE_PUBLIC_KEY=" + prepared.publicKey}, repoPath("scripts", "release-verify.sh"), prepared.dir); err == nil {
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
	case strings.Contains(remote, "'/usr/local/bin/tinkercloud' 'init'"):
		r.initCalls++
		if r.terminalOut != nil {
			return r.terminalOut, errors.New("exit status 1")
		}
		if r.initCalls <= r.initFailures {
			return []byte("tinkercloud: public_health_failed\n"), errors.New("exit status 1")
		}
		return nil, nil
	case strings.Contains(remote, "'cat' '/etc/tinkercloud/init-state.json'"):
		return r.state, nil
	default:
		return nil, errors.New("unexpected remote command")
	}
}

func verificationPendingState(t *testing.T) []byte {
	t.Helper()
	return []byte(`{"completed":{"preflight":true,"paths":true,"database":true,"operator":true,"dns":true,"service":true}}`)
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
	argv := []string{"/usr/local/bin/tinkercloud", "init", "--non-interactive", "--domain", "example.test"}
	if err := s.initWithReadinessRetry(context.Background(), argv...); err != nil {
		t.Fatal(err)
	}
	if r.initCalls != 3 || waits != 2 {
		t.Fatalf("init calls=%d waits=%d", r.initCalls, waits)
	}
	var initRemote string
	for _, call := range r.calls {
		remote := call[len(call)-1]
		if strings.Contains(remote, "'/usr/local/bin/tinkercloud' 'init'") {
			if initRemote == "" {
				initRemote = remote
			} else if remote != initRemote {
				t.Fatalf("init argv changed across retry: first=%q next=%q", initRemote, remote)
			}
			continue
		}
		if !strings.Contains(remote, "'cat' '/etc/tinkercloud/init-state.json'") {
			t.Fatalf("retry performed unrelated mutation: %q", remote)
		}
	}
}

func TestInitReadinessDoesNotRetryTerminalOrUnconfirmedFailures(t *testing.T) {
	t.Run("terminal init result", func(t *testing.T) {
		r := &initRetryRunner{terminalOut: []byte("tinkercloud: config_invalid\n")}
		waited := false
		s := Suite{Config: Config{Target: "root@host", KnownHosts: "/kh"}, Runner: r, RetryWait: func(context.Context, time.Duration) error { waited = true; return nil }}
		if err := s.initWithReadinessRetry(context.Background(), "/usr/local/bin/tinkercloud", "init"); err == nil {
			t.Fatal("terminal config failure retried")
		}
		if r.initCalls != 1 || waited || len(r.calls) != 1 {
			t.Fatalf("calls=%d waited=%t all=%d", r.initCalls, waited, len(r.calls))
		}
	})
	t.Run("state is not final verification", func(t *testing.T) {
		r := &initRetryRunner{initFailures: 1, state: []byte(`{"completed":{"preflight":true}}`)}
		s := Suite{Config: Config{Target: "root@host", KnownHosts: "/kh"}, Runner: r, RetryWait: func(context.Context, time.Duration) error { t.Fatal("unexpected retry"); return nil }}
		if err := s.initWithReadinessRetry(context.Background(), "/usr/local/bin/tinkercloud", "init"); err == nil {
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
	if err := s.initWithReadinessRetry(ctx, "/usr/local/bin/tinkercloud", "init"); !errors.Is(err, context.Canceled) {
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

func TestCleanGuardPreservesACMECacheAcrossFreshApplicationState(t *testing.T) {
	f := &calls{}
	s := Suite{Config: Config{Target: "root@host", KnownHosts: "/kh"}, Runner: f}
	if err := s.cleanGuard(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, call := range f.got {
		if strings.Contains(strings.Join(call, " "), "/var/lib/tinkercloud-acme") {
			t.Fatal("clean application-state guard rejected reusable ACME cache")
		}
	}
}

func TestVPSE2EFixtureSlugsAreStableAndBounded(t *testing.T) {
	want := []string{
		"vps-e2e-update-probe",
		"vps-e2e-primary",
		"vps-e2e-isolation",
		"vps-e2e-denied",
		"vps-e2e-public",
		"vps-e2e-public-other",
	}
	if got := vpsFixtureSlugs[:]; !slices.Equal(got, want) {
		t.Fatalf("fixture slugs = %v, want %v", got, want)
	}
}

func TestResetFixtureAppsDeletesOnlyListedOwnedFixtures(t *testing.T) {
	var deleted, keys []string
	h := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps":
			// Deliberately out of fixture order and mixed with a non-fixture app:
			// cleanup must remain bounded to the named, ownership-scoped entries.
			return jsonResponse(r, http.StatusOK, `[{"slug":"unrelated-app","status":"active"},{"slug":"vps-e2e-denied","status":"active"},{"slug":"vps-e2e-primary","status":"active"}]`), nil
		case r.Method == http.MethodDelete:
			deleted = append(deleted, r.URL.Path)
			key := r.Header.Get("Idempotency-Key")
			if !strings.HasPrefix(key, "idem_") {
				t.Fatalf("delete missing fresh idempotency key: %q", key)
			}
			keys = append(keys, key)
			return jsonResponse(r, http.StatusNoContent, ``), nil
		default:
			t.Fatalf("unexpected cleanup request %s %s", r.Method, r.URL.Path)
			return nil, nil
		}
	})}
	c := client.Client{Base: "https://admin.example.test", Token: "fixture-token", HTTP: h}
	if err := resetFixtureApps(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if want := []string{"/api/v1/apps/vps-e2e-primary", "/api/v1/apps/vps-e2e-denied"}; !slices.Equal(deleted, want) {
		t.Fatalf("deleted = %v, want %v", deleted, want)
	}
	if len(keys) != 2 || keys[0] == keys[1] {
		t.Fatalf("deletion keys must be present and distinct: %v", keys)
	}
}

func TestResetFixtureAppsFailsClosedOnListFailure(t *testing.T) {
	h := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/apps" {
			t.Fatalf("cleanup mutated after list failure: %s %s", r.Method, r.URL.Path)
		}
		return jsonResponse(r, http.StatusInternalServerError, `failure`), nil
	})}
	c := client.Client{Base: "https://admin.example.test", Token: "fixture-token", HTTP: h}
	if err := resetFixtureApps(context.Background(), c); err == nil {
		t.Fatal("accepted failed ownership-scoped fixture list")
	}
}

func TestReuseUpdateEnablesLLMRootThenRestartsAndDoctors(t *testing.T) {
	f := &calls{}
	s := Suite{Config: Config{Target: "root@host", KnownHosts: "/kh"}, Runner: f}
	if err := s.applyReuseUpdate(context.Background(), "/stage", "update-probe"); err != nil {
		t.Fatal(err)
	}
	if len(f.got) != 6 {
		t.Fatalf("remote command count=%d, want 6", len(f.got))
	}
	joined := make([]string, len(f.got))
	for i, call := range f.got {
		joined[i] = strings.Join(call, " ")
	}
	for i, want := range []string{
		"verify-artifact",
		"'update'",
		"'doctor'",
		"'llm' 'enable'",
		"'systemctl' 'restart' 'tinkercloud.service'",
		"'doctor'",
	} {
		if !strings.Contains(joined[i], want) {
			t.Fatalf("command %d = %q, want %q", i, joined[i], want)
		}
	}
	if got := f.got[1][len(f.got[1])-1]; !strings.Contains(got, "'--release-manifest'") {
		t.Fatalf("new updater unexpectedly omitted manifest evidence: %q", got)
	}
	if got := f.got[2][len(f.got[2])-1]; !strings.Contains(got, "'doctor'") {
		t.Fatalf("new updater did not doctor before LLM enable: %q", got)
	}
	// remote wraps the root-local argv in the fixed SSH transport. Assert the
	// final remote command rather than falsely requiring the transport wrapper
	// to disappear from the runner trace.
	if got, want := f.got[3][len(f.got[3])-1], "'/usr/local/bin/tinkercloud' 'llm' 'enable' '--config' '/etc/tinkercloud/config.yaml'"; got != want {
		t.Fatalf("LLM enable remote argv = %q, want exact root-local argv %q", got, want)
	}
}

type updateCompatibilityRunner struct {
	got                   [][]string
	rejectNew, failLegacy bool
	genericReject         bool
}

func (r *updateCompatibilityRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.got = append(r.got, append([]string{name}, args...))
	remote := args[len(args)-1]
	if strings.Contains(remote, "'update'") && strings.Contains(remote, "'--release-manifest'") {
		if r.genericReject {
			return []byte("tinkercloud: verification_failed\n"), errors.New("exit status 2")
		}
		if r.rejectNew {
			return []byte("flag provided but not defined: -release-manifest\n"), errors.New("exit status 2")
		}
	}
	if strings.Contains(remote, "'update'") && !strings.Contains(remote, "'--release-manifest'") && r.failLegacy {
		return []byte("tinkercloud: verification_failed\n"), errors.New("exit status 2")
	}
	return nil, nil
}

func TestReuseUpdateUsesLegacyOnlyForExactOldManifestFlagRejection(t *testing.T) {
	r := &updateCompatibilityRunner{rejectNew: true}
	s := Suite{Config: Config{Target: "root@host", KnownHosts: "/kh"}, Runner: r}
	if err := s.applyReuseUpdate(context.Background(), "/stage", "update-probe"); err != nil {
		t.Fatal(err)
	}
	if len(r.got) != 7 {
		t.Fatalf("remote command count=%d, want 7", len(r.got))
	}
	newUpdate, legacyUpdate := r.got[1][len(r.got[1])-1], r.got[2][len(r.got[2])-1]
	if !strings.Contains(newUpdate, "'--release-manifest'") || strings.Contains(legacyUpdate, "'--release-manifest'") {
		t.Fatalf("compatibility update argv new=%q legacy=%q", newUpdate, legacyUpdate)
	}
	if !strings.Contains(r.got[3][len(r.got[3])-1], "'doctor'") || !strings.Contains(r.got[4][len(r.got[4])-1], "'llm' 'enable'") {
		t.Fatalf("legacy update did not doctor before LLM enable: %#v", r.got)
	}
}

func TestReuseUpdateRejectsNonLegacyAndLegacyUpdateFailures(t *testing.T) {
	for name, runner := range map[string]*updateCompatibilityRunner{
		"new updater verification failure":    {genericReject: true},
		"legacy updater verification failure": {rejectNew: true, failLegacy: true},
	} {
		t.Run(name, func(t *testing.T) {
			s := Suite{Config: Config{Target: "root@host", KnownHosts: "/kh"}, Runner: runner}
			if err := s.applyReuseUpdate(context.Background(), "/stage", "update-probe"); err == nil {
				t.Fatal("unsafe update failure was accepted")
			}
			for _, call := range runner.got {
				if strings.Contains(call[len(call)-1], "'llm' 'enable'") || strings.Contains(call[len(call)-1], "'systemctl' 'restart'") {
					t.Fatalf("failure continued into LLM lifecycle: %#v", runner.got)
				}
			}
		})
	}
}

func TestReuseUpdateStagesOnlySignedManifestEvidence(t *testing.T) {
	files := reuseUpdateFiles("/release", "/stage")
	if len(files) != 3 {
		t.Fatalf("reuse manifest files=%d, want 3", len(files))
	}
	for _, want := range []string{
		"/release/release-manifest.json:/stage/release-manifest.json",
		"/release/release-manifest.json.metadata.json:/stage/release-manifest.json.metadata.json",
		"/release/release-manifest.json.signature:/stage/release-manifest.json.signature",
	} {
		found := false
		for _, file := range files {
			if file.local+":"+file.remote == want {
				found = true
			}
			if strings.Contains(file.local, "resend") || strings.Contains(file.local, "hmac") || strings.Contains(file.local, "key") {
				t.Fatalf("reuse staged secret-like input: %#v", file)
			}
		}
		if !found {
			t.Fatalf("missing reuse evidence %q", want)
		}
	}
}
func TestHiddenTransactionAndCSRFExtractionIsFixedToKnownServerFields(t *testing.T) {
	if got := hiddenValue(`<input name="transaction" value="otp_x">`, "transaction"); got != "otp_x" {
		t.Fatal(got)
	}
	if got := hiddenValue(`<input name="csrf" value="gis_1.csrf-value">`, "csrf"); got != "gis_1.csrf-value" {
		t.Fatal(got)
	}
	if got := hiddenValue(`<input name="csrf" value="first"><input name="transaction" value="second">`, "transaction"); got != "second" {
		t.Fatal(got)
	}
	if got := hiddenValue(`<input name="transaction" value="otp_x">`, "email"); got != "" {
		t.Fatal(got)
	}
}

func TestDashboardIdentityOTPRequiresDashboardCompletionRedirect(t *testing.T) {
	command := filepath.Join(t.TempDir(), "otp-reader")
	if err := os.WriteFile(command, []byte("#!/bin/sh\nprintf '123456\\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}

	newClient := func(location string) *http.Client {
		jar, err := cookiejar.New(nil)
		if err != nil {
			t.Fatal(err)
		}
		return &http.Client{
			Jar: jar,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
			Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
				headers := make(http.Header)
				body := ""
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/login":
					body = `<form action="/login"><input name="email"></form>`
					headers.Add("Set-Cookie", (&http.Cookie{Name: "__Host-tinker_browser", Value: "binding", Path: "/", Secure: true, HttpOnly: true}).String())
					return &http.Response{StatusCode: http.StatusOK, Header: headers, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
				case r.Method == http.MethodPost && r.URL.Path == "/login":
					body = `<input name="transaction" value="pid_1">`
					return &http.Response{StatusCode: http.StatusOK, Header: headers, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
				case r.Method == http.MethodPost && r.URL.Path == "/login/verify":
					headers.Set("Location", location)
					headers.Add("Set-Cookie", (&http.Cookie{Name: "__Host-tinker_identity", Value: "identity", Path: "/", Secure: true, HttpOnly: true}).String())
					return &http.Response{StatusCode: http.StatusSeeOther, Header: headers, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
				default:
					t.Fatalf("unexpected request %s %s", r.Method, r.URL)
					return nil, nil
				}
			}),
		}
	}

	s := Suite{Config: Config{Domain: "example.test", ViewerEmail: "viewer@example.test", OTPCommand: command}}
	if err := s.completeDashboardIdentityOTP(context.Background(), newClient("/dashboard")); err != nil {
		t.Fatalf("dashboard completion redirect rejected: %v", err)
	}
	if err := s.completeDashboardIdentityOTP(context.Background(), newClient("/")); err == nil {
		t.Fatal("unexpected dashboard completion redirect accepted")
	}
}

func TestIdentityBrokerOutcomeDistinguishesPolicyDenialFromOTP(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{name: "authenticated policy denial", status: http.StatusOK, body: `<h1>This account cannot open this app.</h1>`, want: "was denied by the app policy"},
		{name: "actual OTP form", status: http.StatusOK, body: `<form action="/_tinker/identity/otp"><input name="email"></form>`, want: "requires OTP"},
		{name: "unrecognized broker page", status: http.StatusOK, body: `<h1>Try again</h1>`, want: "returned an unrecognized broker page"},
		{name: "non OK status", status: http.StatusServiceUnavailable, body: "", want: "returned an unexpected broker response"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := identityBrokerOutcome(tc.status, []byte(tc.body)); got != tc.want {
				t.Fatalf("identityBrokerOutcome(%d, %q)=%q want %q", tc.status, tc.body, got, tc.want)
			}
		})
	}
}

func TestExistingIdentityHandoffChecksTheExplicitOwnerIdentity(t *testing.T) {
	const (
		platform = "admin.example.test"
		host     = "owned.example.test"
		marker   = "owner-app-marker"
		owner    = "owner@example.test"
		viewer   = "viewer@example.test"
	)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	platformURL, err := url.Parse("https://" + platform + "/")
	if err != nil {
		t.Fatal(err)
	}
	jar.SetCookies(platformURL, []*http.Cookie{
		{Name: "__Host-tinker_identity", Value: "identity", Path: "/", Secure: true, HttpOnly: true},
		{Name: "__Host-tinker_browser", Value: "binding", Path: "/", Secure: true, HttpOnly: true},
	})

	h := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	h.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		headers := make(http.Header)
		switch {
		case r.Host == host && r.Method == http.MethodGet && r.URL.Path == "/":
			if hasCookie(r.Cookies(), "__Host-tinker_app") {
				return &http.Response{StatusCode: http.StatusOK, Header: headers, Body: io.NopCloser(strings.NewReader(marker)), Request: r}, nil
			}
			headers.Set("Location", "/_tinker/auth/login?return=%2F")
			return &http.Response{StatusCode: http.StatusSeeOther, Header: headers, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
		case r.Host == host && r.Method == http.MethodGet && r.URL.Path == "/_tinker/auth/login":
			headers.Set("Location", "https://"+platform+"/_tinker/identity?handoff=handoff_owner")
			return &http.Response{StatusCode: http.StatusSeeOther, Header: headers, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
		case r.Host == platform && r.Method == http.MethodGet && r.URL.Path == "/_tinker/identity":
			headers.Set("Location", "https://"+host+"/_tinker/auth/callback?handoff=handoff_owner")
			return &http.Response{StatusCode: http.StatusSeeOther, Header: headers, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
		case r.Host == host && r.Method == http.MethodGet && r.URL.Path == "/_tinker/auth/callback":
			headers.Set("Location", "/")
			headers.Add("Set-Cookie", (&http.Cookie{Name: "__Host-tinker_app", Value: "owner-app", Path: "/", Secure: true, HttpOnly: true}).String())
			return &http.Response{StatusCode: http.StatusSeeOther, Header: headers, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
		case r.Host == host && r.Method == http.MethodGet && r.URL.Path == "/_tinker/api/v1/me":
			return jsonResponse(r, http.StatusOK, `{"identity":{"email":"`+owner+`"}}`), nil
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL)
			return nil, nil
		}
	})

	s := Suite{Config: Config{Domain: "example.test", ViewerEmail: viewer, DeployerEmail: owner}}
	if err := s.viewerFlowWithExistingIdentityForEmail(context.Background(), h, host, marker, owner); err != nil {
		t.Fatalf("owner global identity handoff rejected: %v", err)
	}
	appURL, err := url.Parse("https://" + host + "/")
	if err != nil {
		t.Fatal(err)
	}
	jar.SetCookies(appURL, []*http.Cookie{{Name: "__Host-tinker_app", Value: "owner-app", Path: "/", Secure: true, HttpOnly: true, MaxAge: -1}})
	if err := s.viewerFlowWithExistingIdentity(context.Background(), h, host, marker); err == nil || !strings.Contains(err.Error(), "current viewer identity was not server-derived") {
		t.Fatalf("viewer convenience wrapper did not reject owner /me identity: %v", err)
	}
}

func TestDashboardCardInsightsAreExactAndScopedToOwnedCard(t *testing.T) {
	dailyRows := strings.Repeat(`<tr><td>Aug 01</td><td>0</td><td>0</td></tr>`, 30)
	page := `
<article data-tinker-app-slug="vps-e2e-public"><section>
<article class="tinker-stat"><p class="tinker-stat__label">Approximate visitors</p><p class="tinker-stat__value">1</p><p class="tinker-card__meta">Last 7 days · 2 page views</p></article>
<article class="tinker-stat"><p class="tinker-stat__label">Approximate visitors</p><p class="tinker-stat__value">9</p><p class="tinker-card__meta">Last 30 days · 99 page views</p></article>
<dl class="tinker-definition"><dt>Last activity</dt><dd>2026-08-01 12:04 UTC</dd></dl><table><caption>Last 30 UTC days</caption><tbody>` + dailyRows + `</tbody></table></section></article>
<article data-tinker-app-slug="vps-e2e-public-other"><section><article class="tinker-stat"><p class="tinker-stat__label">Approximate visitors</p><p class="tinker-stat__value">7</p><p class="tinker-card__meta">Last 7 days · 42 page views</p></article></section></article>`
	insights, ok := dashboardCardInsights(dashboardAppCard(page, "vps-e2e-public"))
	if !ok || !insights.available || insights.pageViews != 2 || insights.visitors != 1 || insights.lastActivity != "2026-08-01 12:04 UTC" || insights.dailyRowCount != 30 {
		t.Fatalf("owned card insights = %#v, parsed=%t; want exact 2 views / 1 visitor", insights, ok)
	}
	if _, ok := dashboardCardInsights(dashboardAppCard(page, "missing")); ok {
		t.Fatal("missing dashboard card was accepted as insights evidence")
	}
}

func TestDashboardCardInsightsDistinguishUnavailableAndMalformed(t *testing.T) {
	unavailable, ok := dashboardCardInsights(`<article data-tinker-app-slug="owned"><h4>Local insights unavailable</h4></article>`)
	if !ok || unavailable.available {
		t.Fatalf("unavailable card = %#v, parsed=%t", unavailable, ok)
	}
	if _, ok := dashboardCardInsights(`<article data-tinker-app-slug="owned"><p class="tinker-stat__value">1</p></article>`); ok {
		t.Fatal("malformed dashboard card was accepted as insights evidence")
	}
}

func TestDashboardInsightsEvidenceDistinguishesOverviewUnavailableFromMissingCard(t *testing.T) {
	if matched, diagnostic := dashboardInsightsEvidence("<h1>Your app overview is unavailable.</h1>", http.StatusOK, nil, "owned", 2, 1); matched || diagnostic != "dashboard overview unavailable" {
		t.Fatalf("overview unavailable = matched=%t diagnostic=%q", matched, diagnostic)
	}
	if matched, diagnostic := dashboardInsightsEvidence("<main>ready</main>", http.StatusOK, nil, "owned", 2, 1); matched || diagnostic != "owned dashboard app card missing" {
		t.Fatalf("missing card = matched=%t diagnostic=%q", matched, diagnostic)
	}
}

func TestDashboardInsightsEvidenceRequiresOwnerVisibleActivityAndBoundedDailySeries(t *testing.T) {
	base := `<article data-tinker-app-slug="owned"><article class="tinker-stat"><p class="tinker-stat__label">Approximate visitors</p><p class="tinker-stat__value">1</p><p class="tinker-card__meta">Last 7 days · 2 page views</p></article><dl><dt>Last activity</dt><dd>2026-08-01 12:04 UTC</dd></dl><table><caption>Last 30 UTC days</caption><tbody>` + strings.Repeat(`<tr><td>Aug 01</td></tr>`, 30) + `</tbody></table></article>`
	if matched, diagnostic := dashboardInsightsEvidence(base, http.StatusOK, nil, "owned", 2, 1); !matched || diagnostic != "" {
		t.Fatalf("complete owner insights evidence = matched=%t diagnostic=%q", matched, diagnostic)
	}
	withoutActivity := strings.Replace(base, `<dt>Last activity</dt><dd>2026-08-01 12:04 UTC</dd>`, "", 1)
	if matched, diagnostic := dashboardInsightsEvidence(withoutActivity, http.StatusOK, nil, "owned", 2, 1); matched || diagnostic != "owned dashboard last activity missing" {
		t.Fatalf("missing activity = matched=%t diagnostic=%q", matched, diagnostic)
	}
	shortSeries := strings.Replace(base, strings.Repeat(`<tr><td>Aug 01</td></tr>`, 30), strings.Repeat(`<tr><td>Aug 01</td></tr>`, 29), 1)
	if matched, diagnostic := dashboardInsightsEvidence(shortSeries, http.StatusOK, nil, "owned", 2, 1); matched || diagnostic != "owned dashboard daily series rows=29 want=30" {
		t.Fatalf("short series = matched=%t diagnostic=%q", matched, diagnostic)
	}
}

func TestReplacePrivateEmailPolicyUsesCurrentRevisionAndExactSupportedShape(t *testing.T) {
	var put map[string]any
	h := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps/primary/access":
			return jsonResponse(r, http.StatusOK, `{"mode":"private","revision":7,"allow":{"emails":["viewer@example.test"],"domains":[]}}`), nil
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/apps/primary/access":
			if r.Header.Get("Idempotency-Key") == "" {
				t.Fatal("policy replacement omitted idempotency key")
			}
			body, err := io.ReadAll(r.Body)
			if err != nil || json.Unmarshal(body, &put) != nil {
				t.Fatalf("policy replacement body=%q err=%v", body, err)
			}
			return jsonResponse(r, http.StatusNoContent, ``), nil
		default:
			t.Fatalf("unexpected policy request %s %s", r.Method, r.URL.Path)
			return nil, nil
		}
	})}
	owner := client.Client{Base: "https://admin.example.test", Token: "owner-token", HTTP: h}
	if err := replacePrivateEmailPolicy(context.Background(), owner, "primary", nil, false); err != nil {
		t.Fatal(err)
	}
	if put["mode"] != "private" || put["expected_revision"] != float64(7) || put["confirm_broadening"] != false {
		t.Fatalf("policy envelope=%#v", put)
	}
	allow, ok := put["allow"].(map[string]any)
	if !ok || len(allow["emails"].([]any)) != 0 || len(allow["domains"].([]any)) != 0 {
		t.Fatalf("policy allowlist=%#v", allow)
	}
}

func TestVerifyPublicCatalogRequiresAllowedPrivateAndPublicCardsAndRejectsDeniedMetadata(t *testing.T) {
	const (
		primaryMarker = "primary-private-marker"
		deniedMarker  = "denied-private-marker"
		publicMarker  = "public-marker"
		publicAsset   = "public-asset"
	)
	valid := `<article data-tinker-catalog-slug="vps-e2e-primary"></article><article data-tinker-catalog-slug="vps-e2e-public"></article><article data-tinker-catalog-slug="vps-e2e-public-other"></article>`
	page := valid
	h := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(page)), Request: r}, nil
	})}
	s := Suite{Config: Config{Domain: "example.test"}}
	state := publicMatrixState{marker: publicMarker, asset: publicAsset}
	if err := s.verifyPublicCatalog(context.Background(), h, state, primaryMarker, deniedMarker); err != nil {
		t.Fatalf("complete authorized catalog rejected: %v", err)
	}
	page = valid + `<article data-tinker-catalog-slug="vps-e2e-denied">` + deniedMarker + `</article>`
	if err := s.verifyPublicCatalog(context.Background(), h, state, primaryMarker, deniedMarker); err == nil || !strings.Contains(err.Error(), "disclosed denied metadata") {
		t.Fatalf("denied catalog metadata accepted: %v", err)
	}
}

func TestExactPublicDocumentUsesDocumentHeadersAndRetainsOneJarMarker(t *testing.T) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	requests := 0
	h := &http.Client{Jar: jar, Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		requests++
		if r.Header.Get("Accept") != "text/html" || r.Header.Get("Sec-Fetch-Dest") != "document" {
			t.Fatalf("document tracking headers = Accept=%q Sec-Fetch-Dest=%q", r.Header.Get("Accept"), r.Header.Get("Sec-Fetch-Dest"))
		}
		if requests == 1 && r.Header.Get("Cookie") != "" {
			t.Fatalf("first public document unexpectedly carried a marker: %q", r.Header.Get("Cookie"))
		}
		if requests == 2 && !strings.Contains(r.Header.Get("Cookie"), "__Host-tinker-insights=opaque") {
			t.Fatalf("second public document did not retain the app-host marker: %q", r.Header.Get("Cookie"))
		}
		header := make(http.Header)
		header.Set("Cache-Control", "no-store")
		header.Add("Set-Cookie", "__Host-tinker-insights=opaque; Path=/; Secure; HttpOnly")
		return &http.Response{StatusCode: http.StatusOK, Header: header, Body: io.NopCloser(strings.NewReader("<!doctype html><title>public matrix</title>marker")), Request: r}, nil
	})}
	for range 2 {
		if err := requestExactPublicDocument(context.Background(), h, "public.example.test", "marker", true); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPublicExampleAssetsAreTrackedAndCapabilityFree(t *testing.T) {
	index, stylesheet, err := publicExampleAssets()
	if err != nil || len(index) == 0 || len(stylesheet) == 0 {
		t.Fatalf("tracked public example assets index=%d stylesheet=%d err=%v", len(index), len(stylesheet), err)
	}
	manifest, err := os.ReadFile(repoPath("examples", "public-static-product-story", "tinker.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := releases.ParseManifest(manifest)
	if err != nil || parsed.Name != publicExampleSlug || parsed.AccessMode != "public" || !parsed.Indexing || parsed.KV || parsed.Realtime || parsed.Blobs || parsed.LLMChat {
		t.Fatalf("public example manifest=%#v err=%v", parsed, err)
	}
}

func TestPublicExampleHTTPSHelpersRequireExactIndexedStaticResponses(t *testing.T) {
	const host = "public-static-product-story.example.test"
	index := []byte("<!doctype html><title>Northstar</title>")
	stylesheet := []byte("body{color:#202820}")
	seenDocuments := 0
	h := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		headers := make(http.Header)
		headers.Set("Cache-Control", "no-store")
		headers.Set("X-Content-Type-Options", "nosniff")
		switch r.URL.Path {
		case "/":
			seenDocuments++
			if r.Header.Get("Accept") != "text/html" || r.Header.Get("Sec-Fetch-Dest") != "document" {
				t.Fatalf("document headers accept=%q destination=%q", r.Header.Get("Accept"), r.Header.Get("Sec-Fetch-Dest"))
			}
			headers.Set("Content-Type", "text/html; charset=utf-8")
			return &http.Response{StatusCode: http.StatusOK, Header: headers, Body: io.NopCloser(bytes.NewReader(index)), Request: r}, nil
		case "/styles.css":
			headers.Set("Content-Type", "text/css; charset=utf-8")
			return &http.Response{StatusCode: http.StatusOK, Header: headers, Body: io.NopCloser(bytes.NewReader(stylesheet)), Request: r}, nil
		default:
			t.Fatalf("unexpected public example path %q", r.URL.Path)
			return nil, nil
		}
	})}
	for range 2 {
		if err := requestExactPublicExampleDocument(context.Background(), h, host, index); err != nil {
			t.Fatal(err)
		}
	}
	if seenDocuments != 2 {
		t.Fatalf("document requests=%d want=2", seenDocuments)
	}
	if err := requestExactPublicExampleAsset(context.Background(), h, host, stylesheet); err != nil {
		t.Fatal(err)
	}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func jsonResponse(r *http.Request, status int, body string) *http.Response {
	h := make(http.Header)
	h.Set("Content-Type", "application/json")
	return &http.Response{StatusCode: status, Header: h, Body: io.NopCloser(strings.NewReader(body)), Request: r}
}

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
			if path == "/_tinker/ws/v1" {
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
	f := &calls{out: []byte("LISTEN 0 4096 *:8080 *:* users:((\"tinkercloud\",pid=9,fd=1))\n")}
	s := Suite{Config: Config{Target: "root@host", KnownHosts: "/kh"}, Runner: f}
	if err := s.socketInventory(context.Background()); err == nil {
		t.Fatal("8080 was confused with port 80")
	}
	f.out = []byte("LISTEN 0 4096 *:80 *:* users:((\"tinkercloud\",pid=9,fd=1))\nLISTEN 0 4096 [::]:443 [::]:* users:((\"tinkercloud\",pid=9,fd=1))\n")
	if err := s.socketInventory(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.out = append(f.out, []byte("LISTEN 0 4096 *:8080 *:* users:((\"tinkercloud\",pid=9,fd=2))\n")...)
	if err := s.socketInventory(context.Background()); err == nil {
		t.Fatal("additional tinkercloud public listener was accepted")
	}
	f.out = []byte("LISTEN 0 4096 *:80 *:* users:((\"tinkercloud\",pid=9,fd=1))\nLISTEN 0 4096 [::]:443 [::]:* users:((\"tinkercloud\",pid=9,fd=1))\nLISTEN 0 4096 *:8080 *:* users:((\"operator-service\",pid=10,fd=1))\n")
	if err := s.socketInventory(context.Background()); err != nil {
		t.Fatal("operator-owned listener was attributed to tinkercloud:", err)
	}
	f.out = []byte("LISTEN 0 4096 *:80 *:* users:((\"not-tinkercloud\",pid=9,fd=1))\nLISTEN 0 4096 [::]:443 [::]:* users:((\"tinkercloud\",pid=9,fd=1))\n")
	if err := s.socketInventory(context.Background()); err == nil {
		t.Fatal("process-name substring was accepted")
	}
}

func TestPublicArchiveFixturesMatchManifestAndArchiveContract(t *testing.T) {
	viewer := "viewer@example.test"
	cases := []struct {
		name, mode    string
		indexing      bool
		capability    bool
		validManifest bool
	}{
		{name: "private", mode: "private", validManifest: true},
		{name: "public indexed", mode: "public", indexing: true, validManifest: true},
		{name: "public no index", mode: "public", validManifest: true},
		{name: "public capability denial", mode: "public", capability: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			archive, size, marker, asset, err := publicArchive("vps-public-"+strings.ReplaceAll(tc.name, " ", "-"), tc.mode, tc.indexing, tc.capability, viewer)
			if err != nil || len(archive) == 0 || size != int64(len(archive)) || marker == "" || asset == "" {
				t.Fatalf("public archive: bytes=%d size=%d marker=%q asset=%q err=%v", len(archive), size, marker, asset, err)
			}
			manifest, names := archiveManifestAndNames(t, archive)
			if want := []string{"tinker.yaml", "asset.txt", "index.html", "private.js"}; !slices.Equal(names, want) {
				t.Fatalf("archive entries = %v, want %v", names, want)
			}
			parsed, parseErr := releases.ParseManifest(manifest)
			if !tc.validManifest {
				if !errors.Is(parseErr, releases.ErrManifest) {
					t.Fatalf("capability-bearing public fixture parser error = %v, want invalid manifest", parseErr)
				}
				if !strings.Contains(string(manifest), "features:\n  kv: true\n") {
					t.Fatalf("capability fixture manifest omitted requested capability: %q", manifest)
				}
				var locallyValidated bytes.Buffer
				if err := client.ArchiveProject(t.TempDir(), manifest, &locallyValidated); !errors.Is(err, releases.ErrManifest) {
					t.Fatalf("ArchiveProject error = %v, want invalid manifest", err)
				}
				return
			}
			if parseErr != nil {
				t.Fatal(parseErr)
			}
			wantEmails := 0
			if tc.mode == "private" {
				wantEmails = 1
			}
			if parsed.Name != "vps-public-"+strings.ReplaceAll(tc.name, " ", "-") || parsed.AccessMode != tc.mode || parsed.Indexing != tc.indexing || parsed.KV || len(parsed.Emails) != wantEmails {
				t.Fatalf("public fixture manifest = %#v", parsed)
			}
			if tc.mode == "private" && parsed.Emails[0] != viewer {
				t.Fatalf("private fixture emails = %v, want %q", parsed.Emails, viewer)
			}
		})
	}
}

func TestPublicArchiveWithoutIndexKeepsManifestValidButOmitsDocument(t *testing.T) {
	archive, size, err := publicArchiveWithoutIndex("vps-public-no-index")
	if err != nil || len(archive) == 0 || size != int64(len(archive)) {
		t.Fatalf("missing-index archive bytes=%d size=%d err=%v", len(archive), size, err)
	}
	manifest, names := archiveManifestAndNames(t, archive)
	parsed, err := releases.ParseManifest(manifest)
	if err != nil || parsed.Name != "vps-public-no-index" || parsed.AccessMode != "public" || parsed.Indexing {
		t.Fatalf("missing-index manifest=%#v err=%v", parsed, err)
	}
	if want := []string{"tinker.yaml", "asset.txt"}; !slices.Equal(names, want) {
		t.Fatalf("missing-index archive entries=%v want=%v", names, want)
	}
}

func TestSmokeArchiveIsDeployableAndUsesUniqueMarker(t *testing.T) {
	viewer := "viewer@example.test"
	a, sizeA, markerA, err := smokeArchive("vps-smoke-a", viewer)
	if err != nil || len(a) == 0 || sizeA != int64(len(a)) || markerA == "" {
		t.Fatalf("archive A: bytes=%d size=%d marker=%q err=%v", len(a), sizeA, markerA, err)
	}
	m := smokeManifest(t, a)
	if m.Name != "vps-smoke-a" || m.SPAFallback != "index.html" || !m.KV || !m.Realtime || !m.Blobs || len(m.Emails) != 1 || m.Emails[0] != viewer || len(m.Domains) != 0 {
		t.Fatalf("smoke policy = %#v", m)
	}
	_, _, markerB, err := smokeArchive("vps-smoke-b", viewer)
	if err != nil || markerA == markerB {
		t.Fatalf("archive B marker=%q err=%v", markerB, err)
	}
}

func smokeManifest(t *testing.T, archive []byte) releases.Manifest {
	t.Helper()
	b, _ := archiveManifestAndNames(t, archive)
	m, err := releases.ParseManifest(b)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func archiveManifestAndNames(t *testing.T, archive []byte) ([]byte, []string) {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var manifest []byte
	var names []string
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, h.Name)
		if h.Name == "tinker.yaml" {
			manifest, err = io.ReadAll(tr)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if len(names) == 0 || names[0] != "tinker.yaml" || manifest == nil {
		t.Fatalf("archive missing first manifest: entries=%v", names)
	}
	return manifest, names
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

// TestPublicExampleAcceptance is intentionally separate from the destructive
// clean-host suite. It only verifies the already deployed first-party public
// example over HTTPS, then uses the normal dashboard OTP flow for its owner
// read-model evidence. It never installs, deploys, cleans, SSHes, or mutates
// VPS state.
func TestPublicExampleAcceptance(t *testing.T) {
	c, e := LoadPublicExampleConfig(os.Getenv)
	if e != nil {
		t.Skip(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	if e = (&Suite{Config: c}).RunPublicExampleAcceptance(ctx); e != nil {
		t.Fatal(e)
	}
}
