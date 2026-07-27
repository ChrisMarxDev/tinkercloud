// Package vps implements the opt-in, black-box VPS acceptance run.
//
// It deliberately has no server-side test hook: OTP codes come from the
// operator's local mail-reader command and all app identity is derived by the
// deployed gateway. The package is safe to compile and test on developer
// machines; a live run is impossible without the explicit environment gate.
package vps

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/tinyhost/tiny/internal/client"
	"github.com/tinyhost/tiny/internal/operations"
)

const (
	EnvEnabled     = "TINYHOST_VPS_E2E"
	EnvTarget      = "TINYHOST_VPS_SSH_TARGET"
	EnvAcknowledge = "TINYHOST_VPS_ACKNOWLEDGE"
	EnvKnownHosts  = "TINYHOST_VPS_KNOWN_HOSTS_FILE"

	initReadinessAttempts = 8
	initReadinessDelay    = 15 * time.Second
)

var numericCode = regexp.MustCompile(`^[0-9]{4,12}$`)
var rootTarget = regexp.MustCompile(`^root@(?:[a-zA-Z0-9](?:[a-zA-Z0-9.-]*[a-zA-Z0-9])?|\[[0-9A-Fa-f:]+\])$`)
var dnsName = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$`)
var emailName = regexp.MustCompile(`^[a-z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@[a-z0-9](?:[a-z0-9-]*[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]*[a-z0-9])?)+$`)

// Config intentionally separates SSH arguments. In particular, SSH_TARGET is
// not a shell fragment and no mode disables host-key verification.
type Config struct {
	Target, Port, IdentityFile, KnownHosts          string
	PlatformHost, AppSuffix                         string
	OperatorEmail, DeployerEmail, ViewerEmail       string
	EmailFrom, ACMEEmail, ResendKeyFile, OTPCommand string
	ReleaseDir                                      string
	Reuse                                           bool
}

// LoadConfig reads a deliberately small, strict environment contract. The
// caller must provide a pre-existing known-hosts file; accepting a new host key
// would defeat the purpose of an SSH acceptance check.
func LoadConfig(getenv func(string) string) (Config, error) {
	if getenv(EnvEnabled) != "1" {
		return Config{}, errors.New("VPS E2E is disabled; set TINYHOST_VPS_E2E=1")
	}
	c := Config{
		Target: strings.TrimSpace(getenv(EnvTarget)), Port: strings.TrimSpace(getenv("TINYHOST_VPS_SSH_PORT")), IdentityFile: strings.TrimSpace(getenv("TINYHOST_VPS_SSH_IDENTITY_FILE")), KnownHosts: strings.TrimSpace(getenv(EnvKnownHosts)),
		PlatformHost: strings.TrimSpace(getenv("TINYHOST_VPS_PLATFORM_HOST")), AppSuffix: strings.TrimSpace(getenv("TINYHOST_VPS_APP_SUFFIX")),
		OperatorEmail: strings.TrimSpace(getenv("TINYHOST_VPS_OPERATOR_EMAIL")), DeployerEmail: strings.TrimSpace(getenv("TINYHOST_VPS_DEPLOYER_EMAIL")), ViewerEmail: strings.TrimSpace(getenv("TINYHOST_VPS_VIEWER_EMAIL")),
		EmailFrom: strings.TrimSpace(getenv("TINYHOST_VPS_EMAIL_FROM")), ACMEEmail: strings.TrimSpace(getenv("TINYHOST_VPS_ACME_EMAIL")), ResendKeyFile: strings.TrimSpace(getenv("TINYHOST_VPS_RESEND_API_KEY_FILE")), OTPCommand: strings.TrimSpace(getenv("TINYHOST_VPS_OTP_COMMAND")), ReleaseDir: strings.TrimSpace(getenv("TINYHOST_VPS_RELEASE_DIR")), Reuse: getenv("TINYHOST_VPS_REUSE") == "1",
	}
	for name, value := range map[string]string{EnvTarget: c.Target, EnvKnownHosts: c.KnownHosts, "TINYHOST_VPS_PLATFORM_HOST": c.PlatformHost, "TINYHOST_VPS_APP_SUFFIX": c.AppSuffix, "TINYHOST_VPS_OPERATOR_EMAIL": c.OperatorEmail, "TINYHOST_VPS_DEPLOYER_EMAIL": c.DeployerEmail, "TINYHOST_VPS_VIEWER_EMAIL": c.ViewerEmail, "TINYHOST_VPS_EMAIL_FROM": c.EmailFrom, "TINYHOST_VPS_ACME_EMAIL": c.ACMEEmail, "TINYHOST_VPS_RESEND_API_KEY_FILE": c.ResendKeyFile} {
		if value == "" || strings.ContainsAny(value, "\r\n\x00") {
			return Config{}, fmt.Errorf("%s is required and must be a single line", name)
		}
	}
	if getenv(EnvAcknowledge) != c.Target {
		return Config{}, fmt.Errorf("%s must exactly equal %s", EnvAcknowledge, EnvTarget)
	}
	if !rootTarget.MatchString(c.Target) {
		return Config{}, errors.New("SSH target must be a root@host destination")
	}
	if c.Port != "" {
		port, err := strconv.Atoi(c.Port)
		if err != nil || port < 1 || port > 65535 {
			return Config{}, errors.New("invalid SSH port")
		}
	}
	for _, p := range []string{c.KnownHosts, c.ResendKeyFile} {
		if !filepath.IsAbs(p) {
			return Config{}, errors.New("local file paths must be absolute")
		}
		if s, e := os.Stat(p); e != nil || !s.Mode().IsRegular() {
			return Config{}, fmt.Errorf("required local file unavailable: %s", p)
		}
	}
	if c.IdentityFile != "" {
		if !filepath.IsAbs(c.IdentityFile) {
			return Config{}, errors.New("identity file must be absolute")
		}
		if s, e := os.Stat(c.IdentityFile); e != nil || !s.Mode().IsRegular() {
			return Config{}, errors.New("identity file unavailable")
		}
	}
	if c.ReleaseDir != "" {
		if !filepath.IsAbs(c.ReleaseDir) {
			return Config{}, errors.New("release directory must be absolute")
		}
		if st, e := os.Stat(c.ReleaseDir); e != nil || !st.IsDir() {
			return Config{}, errors.New("release directory unavailable")
		}
	}
	if !dnsName.MatchString(strings.ToLower(c.PlatformHost)) || !dnsName.MatchString(strings.ToLower(c.AppSuffix)) || strings.EqualFold(c.PlatformHost, c.AppSuffix) || strings.HasSuffix(strings.ToLower(c.PlatformHost), "."+strings.ToLower(c.AppSuffix)) {
		return Config{}, errors.New("platform and app suffix must be distinct DNS names")
	}
	for _, e := range []string{c.OperatorEmail, c.DeployerEmail, c.ViewerEmail, c.EmailFrom, c.ACMEEmail} {
		if !emailName.MatchString(strings.ToLower(e)) {
			return Config{}, errors.New("invalid email address")
		}
	}
	if strings.EqualFold(c.DeployerEmail, c.ViewerEmail) {
		return Config{}, errors.New("deployer and viewer identities must differ")
	}
	if c.OTPCommand != "" {
		if !filepath.IsAbs(c.OTPCommand) {
			return Config{}, errors.New("OTP command must be absolute")
		}
		st, e := os.Stat(c.OTPCommand)
		if e != nil || !st.Mode().IsRegular() || st.Mode()&0111 == 0 {
			return Config{}, errors.New("OTP command must be executable")
		}
	}
	return c, nil
}

// Runner is injectable so ordinary unit tests never open SSH or SCP.
type Runner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}
type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

type Suite struct {
	Config Config
	Runner Runner
	HTTP   *http.Client
	Temp   string
	// RetryWait is test-only injection for the bounded init readiness wait. A
	// nil value uses a cancellation-aware timer in production.
	RetryWait func(context.Context, time.Duration) error
}

func (s *Suite) runner() Runner {
	if s.Runner != nil {
		return s.Runner
	}
	return execRunner{}
}
func (s *Suite) sshArgs() []string {
	a := []string{"-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes", "-o", "UserKnownHostsFile=" + s.Config.KnownHosts}
	if s.Config.Port != "" {
		a = append(a, "-p", s.Config.Port)
	}
	if s.Config.IdentityFile != "" {
		a = append(a, "-i", s.Config.IdentityFile)
	}
	return a
}
func (s *Suite) remoteRun(ctx context.Context, argv ...string) ([]byte, error) {
	a := append(s.sshArgs(), s.Config.Target, quoteRemote(argv...))
	out, e := s.runner().Run(ctx, "ssh", a...)
	if e != nil {
		return out, fmt.Errorf("remote %q: %w", argv, e)
	}
	return out, nil
}
func remoteError(err error, out []byte) error {
	if len(bytes.TrimSpace(out)) == 0 {
		return err
	}
	return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
}
func (s *Suite) remote(ctx context.Context, argv ...string) error {
	out, err := s.remoteRun(ctx, argv...)
	if err != nil {
		return remoteError(err, out)
	}
	return nil
}
func (s *Suite) remoteOutput(ctx context.Context, argv ...string) ([]byte, error) {
	out, err := s.remoteRun(ctx, argv...)
	if err != nil {
		return nil, remoteError(err, out)
	}
	return out, nil
}

func (s *Suite) initState(ctx context.Context) (operations.InitState, error) {
	out, err := s.remoteOutput(ctx, "cat", "/etc/tinyhost/init-state.json")
	if err != nil {
		return operations.InitState{}, fmt.Errorf("read init state: %w", err)
	}
	state, err := operations.ParseInitState(out)
	if err != nil {
		return operations.InitState{}, fmt.Errorf("parse init state: %w", err)
	}
	return state, nil
}

func (s *Suite) waitInitRetry(ctx context.Context) error {
	if s.RetryWait != nil {
		return s.RetryWait(ctx, initReadinessDelay)
	}
	timer := time.NewTimer(initReadinessDelay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// initWithReadinessRetry retries only the persisted final public-health step.
// The error token is the CLI's documented stable readiness result; checking
// the signed binary's root-owned state independently prevents a preflight or
// provisioning failure from being treated as an ACME/DNS delay.
func (s *Suite) initWithReadinessRetry(ctx context.Context, argv ...string) error {
	for attempt := 1; attempt <= initReadinessAttempts; attempt++ {
		out, err := s.remoteRun(ctx, argv...)
		if err == nil {
			return nil
		}
		if strings.TrimSpace(string(out)) != "tinyhost: public_health_failed" {
			return remoteError(err, out)
		}
		state, stateErr := s.initState(ctx)
		if stateErr != nil {
			return fmt.Errorf("init readiness could not be confirmed: %w", stateErr)
		}
		if state.Next() != operations.InitVerified {
			return remoteError(err, out)
		}
		if attempt == initReadinessAttempts {
			return fmt.Errorf("init public readiness did not succeed after %d attempts: %w", attempt, remoteError(err, out))
		}
		if err := s.waitInitRetry(ctx); err != nil {
			return err
		}
	}
	return errors.New("init readiness retry exhausted")
}

// OpenSSH sends a remote command through the server's login shell. Quote every
// argument ourselves so environment values remain literal data, never syntax.
func quoteRemote(argv ...string) string {
	q := make([]string, len(argv))
	for i, v := range argv {
		q[i] = "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
	}
	return strings.Join(q, " ")
}
func (s *Suite) copy(ctx context.Context, local, remote string) error {
	a := append([]string{}, s.sshArgs()...)
	for i := range a {
		if a[i] == "-p" {
			a[i] = "-P"
		}
	}
	a = append(a, local, s.Config.Target+":"+remote)
	out, e := s.runner().Run(ctx, "scp", a...)
	if e != nil {
		return fmt.Errorf("copy %s: %w: %s", filepath.Base(local), e, strings.TrimSpace(string(out)))
	}
	return nil
}

func (s *Suite) cleanGuard(ctx context.Context) error {
	if s.Config.Reuse {
		out, err := s.remoteOutput(ctx, "cat", "/var/lib/tinyhost-vps-e2e/marker")
		if err != nil || string(out) != s.reuseMarker() {
			return errors.New("reuse refused: matching TinyHost VPS E2E marker is required")
		}
		return nil
	}
	for _, p := range []string{"/etc/tinyhost", "/var/lib/tinyhost", "/var/lib/tinyhost-acme", "/usr/local/bin/tinyhost", "/etc/systemd/system/tinyhost.service"} {
		if err := s.remote(ctx, "test", "!", "-e", p); err != nil {
			return fmt.Errorf("host is not clean (%s exists); set TINYHOST_VPS_REUSE=1 only for a disposable existing TinyHost host: %w", p, err)
		}
	}
	return nil
}
func (s *Suite) reuseMarker() string {
	return "platform=" + s.Config.PlatformHost + "\napp_suffix=" + s.Config.AppSuffix + "\ntarget=" + s.Config.Target + "\n"
}

func tempSecret(dir string) (string, error) {
	b := make([]byte, 48)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	p := filepath.Join(dir, "tinyhost-hmac.key")
	return p, os.WriteFile(p, []byte(hex.EncodeToString(b)+"\n"), 0600)
}
func randomID() (string, error) {
	b := make([]byte, 8)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return hex.EncodeToString(b), nil
}

// Run executes a full remote acceptance path. It does not clean a host after a
// failure: logs, failed state, and the deployed release are evidence for the
// operator. Use a disposable VPS and destroy it after the run.
func (s *Suite) Run(ctx context.Context) error {
	if runtime.GOOS == "windows" {
		return errors.New("VPS E2E build host must provide Go cross compilation")
	}
	if err := s.cleanGuard(ctx); err != nil {
		return err
	}
	dir := s.Temp
	if dir == "" {
		var e error
		dir, e = os.MkdirTemp("", "tinyhost-vps-e2e-")
		if e != nil {
			return e
		}
		defer os.RemoveAll(dir)
	}
	markerPath := filepath.Join(dir, "reuse-marker")
	if err := os.WriteFile(markerPath, []byte(s.reuseMarker()), 0600); err != nil {
		return err
	}
	prepared, err := prepareRelease(ctx, dir, s.Config.ReleaseDir)
	if err != nil {
		return err
	}
	release := prepared.dir
	hmac, e := tempSecret(dir)
	if e != nil {
		return e
	}
	id, e := randomID()
	if e != nil {
		return e
	}
	remoteDir := "/root/tinyhost-vps-e2e-" + id
	// This directory is generated from crypto/rand and is the only remote path
	// the suite removes. Clean it on both success and failure so copied provider
	// and HMAC secrets never become debugging residue.
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_ = s.remote(cleanup, "rm", "-rf", remoteDir)
	}()
	if err = s.remote(ctx, "mkdir", "-m", "0700", remoteDir); err != nil {
		return err
	}
	if err = s.remote(ctx, "mkdir", "-p", remoteDir+"/packaging/systemd"); err != nil {
		return err
	}
	for _, v := range []struct{ local, remote string }{{filepath.Join(release, "tinyhost-linux-amd64"), remoteDir + "/tinyhost-linux-amd64"}, {filepath.Join(release, "tinyhost-linux-amd64.metadata.json"), remoteDir + "/tinyhost-linux-amd64.metadata.json"}, {filepath.Join(release, "tinyhost-linux-amd64.signature"), remoteDir + "/tinyhost-linux-amd64.signature"}, {prepared.publicKey, remoteDir + "/packaging/release-public-key.pem"}, {repoPath("packaging", "install.sh"), remoteDir + "/packaging/install.sh"}, {repoPath("packaging", "systemd", "tinyhost.service"), remoteDir + "/packaging/systemd/tinyhost.service"}, {s.Config.ResendKeyFile, remoteDir + "/resend.key"}, {hmac, remoteDir + "/hmac.key"}, {markerPath, remoteDir + "/marker"}} {
		if err = s.copy(ctx, v.local, v.remote); err != nil {
			return err
		}
	}
	for _, p := range []string{remoteDir + "/resend.key", remoteDir + "/hmac.key"} {
		if err = s.remote(ctx, "chmod", "0600", p); err != nil {
			return err
		}
	}
	if !s.Config.Reuse {
		if err = s.remote(ctx, "/bin/sh", remoteDir+"/packaging/install.sh", remoteDir+"/tinyhost-linux-amd64", remoteDir+"/tinyhost-linux-amd64.metadata.json", remoteDir+"/tinyhost-linux-amd64.signature"); err != nil {
			return err
		}
		if err = s.initWithReadinessRetry(ctx, "/usr/local/bin/tinyhost", "init", "--non-interactive", "--platform-host", s.Config.PlatformHost, "--app-suffix", s.Config.AppSuffix, "--operator-email", s.Config.OperatorEmail, "--email-from", s.Config.EmailFrom, "--acme-email", s.Config.ACMEEmail, "--resend-api-key-file", remoteDir+"/resend.key", "--hmac-key-file", remoteDir+"/hmac.key"); err != nil {
			return err
		}
		if err = s.remote(ctx, "mkdir", "-m", "0700", "/var/lib/tinyhost-vps-e2e"); err != nil {
			return err
		}
		if err = s.remote(ctx, "install", "-m", "0600", remoteDir+"/marker", "/var/lib/tinyhost-vps-e2e/marker"); err != nil {
			return err
		}
	}
	if err = s.remote(ctx, "/usr/local/bin/tinyhost", "doctor", "--config", "/etc/tinyhost/config.yaml"); err != nil {
		return err
	}
	if err = s.socketInventory(ctx); err != nil {
		return err
	}
	if err = s.remote(ctx, "/usr/local/bin/tinyhost", "deployers", "authorize", s.Config.DeployerEmail); err != nil {
		return err
	}
	return s.exercise(ctx)
}

func commandEnv(ctx context.Context, values []string, name string, args ...string) error {
	c := exec.CommandContext(ctx, name, args...)
	c.Env = append(os.Environ(), values...)
	if b, e := c.CombinedOutput(); e != nil {
		return fmt.Errorf("%s failed: %w: %s", name, e, strings.TrimSpace(string(b)))
	}
	return nil
}

func repoPath(parts ...string) string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return filepath.Join(parts...)
	}
	return filepath.Join(append([]string{filepath.Dir(file), "..", ".."}, parts...)...)
}

type preparedRelease struct {
	dir, publicKey string
}

// prepareRelease verifies the complete immutable release before returning its
// separately staged installer key. The E2E key is auxiliary transport input,
// never release evidence; adding it before verification must deny the release.
// Without an operator-provided release directory it generates a disposable
// Ed25519 authority locally; the private key is never uploaded or logged.
func prepareRelease(ctx context.Context, dir, supplied string) (preparedRelease, error) {
	if supplied != "" {
		if e := commandEnv(ctx, nil, repoPath("scripts", "release-verify.sh"), supplied); e != nil {
			return preparedRelease{}, e
		}
		return preparedRelease{dir: supplied, publicKey: repoPath("packaging", "release-public-key.pem")}, nil
	}
	key := filepath.Join(dir, "vps-e2e-ed25519.pem")
	pub := filepath.Join(dir, "vps-e2e-ed25519.pub.pem")
	out := filepath.Join(dir, "release")
	if e := commandEnv(ctx, nil, "openssl", "genpkey", "-algorithm", "ED25519", "-out", key); e != nil {
		return preparedRelease{}, e
	}
	if e := os.Chmod(key, 0600); e != nil {
		return preparedRelease{}, e
	}
	if e := commandEnv(ctx, nil, "openssl", "pkey", "-in", key, "-pubout", "-out", pub); e != nil {
		return preparedRelease{}, e
	}
	version, e := sdkReleaseVersion()
	if e != nil {
		return preparedRelease{}, e
	}
	if e := commandEnv(ctx, []string{"TINYHOST_RELEASE_SIGNING_KEY=" + key, "TINYHOST_RELEASE_PUBLIC_KEY=" + pub}, repoPath("scripts", "release-build.sh"), version, out); e != nil {
		return preparedRelease{}, e
	}
	if e := commandEnv(ctx, []string{"TINYHOST_RELEASE_PUBLIC_KEY=" + pub}, repoPath("scripts", "release-verify.sh"), out); e != nil {
		return preparedRelease{}, e
	}
	return preparedRelease{dir: out, publicKey: pub}, nil
}

// sdkReleaseVersion reads the release version from the same package metadata
// release-build.sh validates. A disposable VPS E2E authority changes only the
// signing key, never the release or SDK artifact contract.
func sdkReleaseVersion() (string, error) {
	raw, err := os.ReadFile(repoPath("sdk", "typescript", "package.json"))
	if err != nil {
		return "", fmt.Errorf("read SDK package version: %w", err)
	}
	var pkg struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(raw, &pkg); err != nil {
		return "", fmt.Errorf("parse SDK package version: %w", err)
	}
	if pkg.Version == "" {
		return "", errors.New("SDK package version is empty")
	}
	return pkg.Version, nil
}

type otpPrompt struct {
	config        Config
	purpose, host string
	ctx           context.Context
}

func (p otpPrompt) Ask(q string) (string, error) {
	if strings.HasPrefix(q, "Email:") {
		return p.config.DeployerEmail, nil
	}
	return readOTP(p.ctx, p.config, p.purpose, p.config.DeployerEmail, p.host)
}
func readOTP(ctx context.Context, c Config, purpose, email, host string) (string, error) {
	if c.OTPCommand != "" {
		out, e := exec.CommandContext(ctx, c.OTPCommand, purpose, email, host).Output()
		if e != nil {
			return "", fmt.Errorf("OTP command failed: %w", e)
		}
		code := strings.TrimSpace(string(out))
		if !numericCode.MatchString(code) {
			return "", errors.New("OTP command returned invalid code")
		}
		return code, nil
	}
	if f, e := os.Stdin.Stat(); e == nil && f.Mode()&os.ModeCharDevice != 0 {
		fmt.Fprintf(os.Stderr, "OTP for %s (%s): ", email, purpose)
		if e := setTerminalEcho(ctx, false); e != nil {
			return "", errors.New("could not disable terminal echo for OTP")
		}
		defer func() { _ = setTerminalEcho(context.Background(), true); fmt.Fprintln(os.Stderr) }()
		var code string
		if _, e := fmt.Fscan(os.Stdin, &code); e == nil && numericCode.MatchString(code) {
			return code, nil
		}
	}
	return "", errors.New("TINYHOST_VPS_OTP_COMMAND is required without an interactive terminal")
}

// setTerminalEcho binds stty to the actual terminal. os/exec otherwise gives
// the child a nil stdin, which makes stty operate on /dev/null and fail.
var setTerminalEcho = func(ctx context.Context, enabled bool) error {
	mode := "-echo"
	if enabled {
		mode = "echo"
	}
	c := exec.CommandContext(ctx, "stty", mode)
	c.Stdin = os.Stdin
	c.Stdout = os.Stderr
	c.Stderr = os.Stderr
	return c.Run()
}

func (s *Suite) exercise(ctx context.Context) error {
	base := "https://" + s.Config.PlatformHost
	login, err := client.Login(ctx, base, otpPrompt{ctx: ctx, config: s.Config, purpose: "deployer", host: s.Config.PlatformHost})
	if err != nil {
		return fmt.Errorf("deployer OTP login: %w", err)
	}
	c := client.New(base, login.Token)
	suffix, err := randomID()
	if err != nil {
		return err
	}
	slug := "vps-" + suffix
	key, _ := client.IdempotencyKey()
	if err = c.Do(ctx, http.MethodPost, "/api/v1/apps", key, map[string]string{"slug": slug}, nil); err != nil {
		return fmt.Errorf("create app: %w", err)
	}
	policy := map[string]any{"mode": "private", "expected_revision": 1, "confirm_broadening": true, "allow": map[string]any{"emails": []string{s.Config.ViewerEmail}, "domains": []string{}}}
	key, _ = client.IdempotencyKey()
	if err = c.Do(ctx, http.MethodPut, "/api/v1/apps/"+slug+"/access", key, policy, nil); err != nil {
		return fmt.Errorf("set access policy: %w", err)
	}
	appHost := slug + "." + s.Config.AppSuffix
	if err = s.warmCertificate(ctx, appHost); err != nil {
		return err
	}
	archive, size, marker, err := smokeArchive(slug, s.Config.ViewerEmail)
	if err != nil {
		return err
	}
	key, _ = client.IdempotencyKey()
	deployed, err := c.Deploy(ctx, slug, bytes.NewReader(archive), size, key)
	if err != nil {
		return fmt.Errorf("deploy smoke app: %w", err)
	}
	if err = deployed.Verified(); err != nil {
		return err
	}
	if err = s.anonymousDenied(ctx, appHost, marker); err != nil {
		return err
	}
	return s.viewerFlow(ctx, appHost, marker)
}

// warmCertificate intentionally uses the gateway's pre-auth app route. This
// causes the normal autocert flow for a valid, inactive-release app hostname;
// it never exposes app bytes or bypasses deployment activation.
func (s *Suite) warmCertificate(ctx context.Context, host string) error {
	deadline := time.Now().Add(3 * time.Minute)
	for {
		r, e := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+host+"/_tiny/auth/login", nil)
		if e == nil {
			x, e := s.httpClient().Do(r)
			if e == nil {
				x.Body.Close()
				if x.TLS != nil && len(x.TLS.VerifiedChains) > 0 && x.StatusCode < 500 {
					return nil
				}
			}
		}
		if time.Now().After(deadline) {
			return errors.New("app certificate did not become ready")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

func smokeArchive(slug, viewerEmail string) ([]byte, int64, string, error) {
	d, e := os.MkdirTemp("", "tinyhost-vps-app-")
	if e != nil {
		return nil, 0, "", e
	}
	defer os.RemoveAll(d)
	if e = os.Mkdir(filepath.Join(d, "dist"), 0755); e != nil {
		return nil, 0, "", e
	}
	id, e := randomID()
	if e != nil {
		return nil, 0, "", e
	}
	marker := "TINYHOST_VPS_MARKER_" + id
	if e = os.WriteFile(filepath.Join(d, "dist", "index.html"), []byte("<!doctype html><title>tiny</title>"+marker), 0644); e != nil {
		return nil, 0, "", e
	}
	if e = os.WriteFile(filepath.Join(d, "dist", "private.js"), []byte("window.privateMarker='"+marker+"'"), 0644); e != nil {
		return nil, 0, "", e
	}
	manifest := []byte("version: 1\nname: " + slug + "\nbuild:\n  output: dist\naccess:\n  mode: private\n  allow:\n    emails:\n      - " + viewerEmail + "\n    domains: []\n")
	if e = os.WriteFile(filepath.Join(d, "tiny.yaml"), manifest, 0644); e != nil {
		return nil, 0, "", e
	}
	var b bytes.Buffer
	e = client.ArchiveProject(d, manifest, &b)
	return b.Bytes(), int64(b.Len()), marker, e
}
func (s *Suite) httpClient() *http.Client {
	if s.HTTP != nil {
		return s.HTTP
	}
	return &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}
func (s *Suite) anonymousDenied(ctx context.Context, host, marker string) error {
	for _, path := range []string{"/", "/private.js", "/_tiny/api/v1/me", "/_tiny/ws/v1"} {
		r, e := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+host+path, nil)
		if e != nil {
			return e
		}
		if path == "/_tiny/ws/v1" {
			r.Header.Set("Connection", "Upgrade")
			r.Header.Set("Upgrade", "websocket")
			r.Header.Set("Sec-WebSocket-Version", "13")
			r.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
		}
		x, e := s.httpClient().Do(r)
		if e != nil {
			return e
		}
		b, _ := io.ReadAll(io.LimitReader(x.Body, 1<<20))
		x.Body.Close()
		if x.StatusCode != http.StatusUnauthorized || x.StatusCode == http.StatusSwitchingProtocols || bytes.Contains(b, []byte(marker)) {
			return fmt.Errorf("anonymous %s leaked or upgraded: status=%d", path, x.StatusCode)
		}
	}
	return nil
}
func (s *Suite) viewerFlow(ctx context.Context, host, marker string) error {
	jar, e := cookiejar.New(nil)
	if e != nil {
		return e
	}
	h := s.httpClient()
	hc := *h
	hc.Jar = jar
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	form := url.Values{"email": {s.Config.ViewerEmail}, "return": {"/"}}
	r, e := http.NewRequestWithContext(ctx, http.MethodPost, "https://"+host+"/_tiny/auth/otp", strings.NewReader(form.Encode()))
	if e != nil {
		return e
	}
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	x, e := hc.Do(r)
	if e != nil {
		return e
	}
	b, e := io.ReadAll(io.LimitReader(x.Body, 1<<20))
	x.Body.Close()
	if e != nil {
		return e
	}
	tx := hiddenValue(string(b), "transaction")
	if tx == "" {
		return errors.New("viewer OTP transaction missing")
	}
	code, e := readOTP(ctx, s.Config, "viewer", s.Config.ViewerEmail, host)
	if e != nil {
		return e
	}
	form = url.Values{"email": {s.Config.ViewerEmail}, "transaction": {tx}, "code": {code}, "return": {"/"}}
	r, e = http.NewRequestWithContext(ctx, http.MethodPost, "https://"+host+"/_tiny/auth/verify", strings.NewReader(form.Encode()))
	if e != nil {
		return e
	}
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	x, e = hc.Do(r)
	if e != nil {
		return e
	}
	x.Body.Close()
	if x.StatusCode != http.StatusSeeOther {
		return fmt.Errorf("viewer OTP verify: status=%d", x.StatusCode)
	}
	r, e = http.NewRequestWithContext(ctx, http.MethodGet, "https://"+host+"/", nil)
	if e != nil {
		return e
	}
	x, e = hc.Do(r)
	if e != nil {
		return e
	}
	b, _ = io.ReadAll(io.LimitReader(x.Body, 1<<20))
	x.Body.Close()
	if x.StatusCode != 200 || !bytes.Contains(b, []byte(marker)) {
		return fmt.Errorf("viewer app access failed: status=%d", x.StatusCode)
	}
	r, e = http.NewRequestWithContext(ctx, http.MethodGet, "https://"+host+"/_tiny/api/v1/me", nil)
	if e != nil {
		return e
	}
	x, e = hc.Do(r)
	if e != nil {
		return e
	}
	defer x.Body.Close()
	var me struct {
		Identity struct {
			Email string `json:"email"`
		} `json:"identity"`
	}
	if x.StatusCode != 200 || json.NewDecoder(io.LimitReader(x.Body, 1<<20)).Decode(&me) != nil || !strings.EqualFold(me.Identity.Email, s.Config.ViewerEmail) {
		return errors.New("current viewer identity was not server-derived")
	}
	return nil
}

func (s *Suite) socketInventory(ctx context.Context) error {
	out, e := s.remoteOutput(ctx, "ss", "-ltnp")
	if e != nil {
		return e
	}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		local := fields[3]
		if local == "" || loopbackSocket(local) || !strings.Contains(line, `"tinyhost"`) {
			continue
		}
		port := socketPort(local)
		if port != "80" && port != "443" {
			return fmt.Errorf("unexpected tinyhost public listener: %s", line)
		}
	}
	for _, port := range []string{"80", "443"} {
		found := false
		for _, line := range lines {
			fields := strings.Fields(line)
			if len(fields) < 4 {
				continue
			}
			// ss -ltnp has a fixed Local Address:Port column after State,
			// Recv-Q and Send-Q. Do not scan arbitrary process text for ports.
			local := fields[3]
			if local != "" && !loopbackSocket(local) {
				if !strings.HasSuffix(local, ":"+port) {
					continue
				}
				if !strings.Contains(line, `"tinyhost"`) {
					return fmt.Errorf("public port %s is not owned by tinyhost: %s", port, line)
				}
				found = true
			}
		}
		if !found {
			return fmt.Errorf("tinyhost does not own public port %s", port)
		}
	}
	return nil
}
func socketPort(local string) string {
	i := strings.LastIndexByte(local, ':')
	if i < 0 || i == len(local)-1 {
		return ""
	}
	return local[i+1:]
}
func loopbackSocket(local string) bool {
	return strings.HasPrefix(local, "127.") || strings.HasPrefix(local, "[::1]") || strings.HasPrefix(local, "::1:")
}

var hiddenInput = regexp.MustCompile(`name="transaction" value="([^"]+)"`)

func hiddenValue(html, name string) string {
	if name != "transaction" {
		return ""
	}
	m := hiddenInput.FindStringSubmatch(html)
	if len(m) == 2 {
		return m[1]
	}
	return ""
}
