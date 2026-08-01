// Package vps implements the opt-in, black-box VPS acceptance run.
//
// It deliberately has no server-side test hook: OTP codes come from the
// operator's local mail-reader command and all app identity is derived by the
// deployed gateway. The package is safe to compile and test on developer
// machines; a live run is impossible without the explicit environment gate.
package vps

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
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
	"syscall"
	"time"

	"github.com/ChrisMarxDev/tinkercloud/internal/client"
	"github.com/ChrisMarxDev/tinkercloud/internal/operations"
)

const (
	EnvEnabled = "TINKERCLOUD_VPS_E2E"
	// EnvPublicExample enables the separate, verification-only public example
	// check. It deliberately requires the normal VPS E2E configuration too,
	// but it never opens SSH or invokes any mutating suite method.
	EnvPublicExample = "TINKERCLOUD_PUBLIC_EXAMPLE_E2E"
	EnvTarget        = "TINKERCLOUD_VPS_SSH_TARGET"
	EnvAcknowledge   = "TINKERCLOUD_VPS_ACKNOWLEDGE"
	EnvKnownHosts    = "TINKERCLOUD_VPS_KNOWN_HOSTS_FILE"

	initReadinessAttempts = 8
	initReadinessDelay    = 15 * time.Second

	// A systemd restart returns once the process has been asked to start, not
	// once the HTTPS listener can serve an authenticated request. Keep the
	// post-restart durability proof bounded, but do not insert a blind sleep
	// after the restart.
	restartBlobReadinessAttempts = 10
	restartBlobReadinessBudget   = 45 * time.Second
	restartBlobInitialDelay      = 500 * time.Millisecond
	restartBlobMaxDelay          = 5 * time.Second

	// These are the complete, bounded set of fixture app names used by the
	// external acceptance suite. Their hostnames are intentionally stable: a
	// valid exact-host certificate is cached after its first issuance, so
	// repeated clean or reuse runs do not create new ACME names.
	vpsUpdateProbeSlug  = "vps-e2e-update-probe"
	vpsPrimaryAppSlug   = "vps-e2e-primary"
	vpsIsolationAppSlug = "vps-e2e-isolation"
	vpsDeniedAppSlug    = "vps-e2e-denied"
	vpsPublicAppSlug    = "vps-e2e-public"
	vpsPublicOtherSlug  = "vps-e2e-public-other"
	publicExampleSlug   = "public-static-product-story"
)

var vpsFixtureSlugs = [...]string{
	vpsUpdateProbeSlug,
	vpsPrimaryAppSlug,
	vpsIsolationAppSlug,
	vpsDeniedAppSlug,
	vpsPublicAppSlug,
	vpsPublicOtherSlug,
}

var numericCode = regexp.MustCompile(`^[0-9]{4,12}$`)
var rootTarget = regexp.MustCompile(`^root@(?:[a-zA-Z0-9](?:[a-zA-Z0-9.-]*[a-zA-Z0-9])?|\[[0-9A-Fa-f:]+\])$`)
var dnsName = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$`)
var emailName = regexp.MustCompile(`^[a-z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@[a-z0-9](?:[a-z0-9-]*[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]*[a-z0-9])?)+$`)
var legacyUpdateManifestFlag = regexp.MustCompile(`(?m)^(?:flag provided but not defined: -release-manifest|unknown flag: --release-manifest)\s*$`)
var dashboardLast7Insights = regexp.MustCompile(`<p class="tinker-stat__label">Approximate visitors</p><p class="tinker-stat__value">([0-9]+)</p><p class="tinker-card__meta">Last 7 days · ([0-9]+) page views</p>`)
var dashboardLastActivity = regexp.MustCompile(`<dt>Last activity</dt><dd>([^<]+)</dd>`)
var dashboard30DaySeries = regexp.MustCompile(`(?s)<caption>Last 30 UTC days</caption>.*?<tbody>(.*?)</tbody>`)

// Config intentionally separates SSH arguments. In particular, SSH_TARGET is
// not a shell fragment and no mode disables host-key verification.
type Config struct {
	Target, Port, IdentityFile, KnownHosts    string
	Domain                                    string
	OperatorEmail, DeployerEmail, ViewerEmail string
	EmailFrom, ResendKeyFile, OTPCommand      string
	ReleaseDir                                string
	Reuse                                     bool
}

// LoadConfig reads a deliberately small, strict environment contract. The
// caller must provide a pre-existing known-hosts file; accepting a new host key
// would defeat the purpose of an SSH acceptance check.
func LoadConfig(getenv func(string) string) (Config, error) {
	if getenv(EnvEnabled) != "1" {
		return Config{}, errors.New("VPS E2E is disabled; set TINKERCLOUD_VPS_E2E=1")
	}
	c := Config{
		Target: strings.TrimSpace(getenv(EnvTarget)), Port: strings.TrimSpace(getenv("TINKERCLOUD_VPS_SSH_PORT")), IdentityFile: strings.TrimSpace(getenv("TINKERCLOUD_VPS_SSH_IDENTITY_FILE")), KnownHosts: strings.TrimSpace(getenv(EnvKnownHosts)),
		Domain:        strings.TrimSpace(getenv("TINKERCLOUD_VPS_DOMAIN")),
		OperatorEmail: strings.TrimSpace(getenv("TINKERCLOUD_VPS_OPERATOR_EMAIL")), DeployerEmail: strings.TrimSpace(getenv("TINKERCLOUD_VPS_DEPLOYER_EMAIL")), ViewerEmail: strings.TrimSpace(getenv("TINKERCLOUD_VPS_VIEWER_EMAIL")),
		EmailFrom: strings.TrimSpace(getenv("TINKERCLOUD_VPS_EMAIL_FROM")), ResendKeyFile: strings.TrimSpace(getenv("TINKERCLOUD_VPS_RESEND_API_KEY_FILE")), OTPCommand: strings.TrimSpace(getenv("TINKERCLOUD_VPS_OTP_COMMAND")), ReleaseDir: strings.TrimSpace(getenv("TINKERCLOUD_VPS_RELEASE_DIR")), Reuse: getenv("TINKERCLOUD_VPS_REUSE") == "1",
	}
	for name, value := range map[string]string{EnvTarget: c.Target, EnvKnownHosts: c.KnownHosts, "TINKERCLOUD_VPS_DOMAIN": c.Domain, "TINKERCLOUD_VPS_OPERATOR_EMAIL": c.OperatorEmail, "TINKERCLOUD_VPS_DEPLOYER_EMAIL": c.DeployerEmail, "TINKERCLOUD_VPS_VIEWER_EMAIL": c.ViewerEmail, "TINKERCLOUD_VPS_EMAIL_FROM": c.EmailFrom, "TINKERCLOUD_VPS_RESEND_API_KEY_FILE": c.ResendKeyFile} {
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
		if st, e := os.Lstat(c.ReleaseDir); e != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
			return Config{}, errors.New("release directory unavailable")
		}
	}
	if c.Reuse && c.ReleaseDir == "" {
		return Config{}, errors.New("reuse requires an explicit release directory")
	}
	if c.Domain != strings.ToLower(c.Domain) || !dnsName.MatchString(c.Domain) {
		return Config{}, errors.New("domain must be a canonical DNS name")
	}
	for _, e := range []string{c.OperatorEmail, c.DeployerEmail, c.ViewerEmail, c.EmailFrom} {
		if !emailName.MatchString(strings.ToLower(e)) {
			return Config{}, errors.New("invalid email address")
		}
	}
	if strings.EqualFold(c.DeployerEmail, c.ViewerEmail) || strings.EqualFold(c.OperatorEmail, c.ViewerEmail) {
		return Config{}, errors.New("viewer identity must differ from operator and deployer")
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

// LoadPublicExampleConfig makes the public product-story verification opt-in
// in addition to the normal strict VPS/OTP configuration. Keeping both gates
// prevents an ordinary package test from contacting a host by accident.
func LoadPublicExampleConfig(getenv func(string) string) (Config, error) {
	if getenv(EnvPublicExample) != "1" {
		return Config{}, fmt.Errorf("public example E2E is disabled; set %s=1", EnvPublicExample)
	}
	return LoadConfig(getenv)
}

func (c Config) PlatformHost() string { return "admin." + c.Domain }
func (c Config) AppSuffix() string    { return c.Domain }

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
	// RestartReadinessWait is test-only injection for the bounded post-restart
	// blob readiness retry. A nil value uses a cancellation-aware timer.
	RestartReadinessWait func(context.Context, time.Duration) error
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
	out, err := s.remoteOutput(ctx, "cat", "/etc/tinkercloud/init-state.json")
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

func (s *Suite) waitRestartBlobReadiness(ctx context.Context, delay time.Duration) error {
	if s.RestartReadinessWait != nil {
		return s.RestartReadinessWait(ctx, delay)
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func restartBlobDelay(attempt int) time.Duration {
	delay := restartBlobInitialDelay
	for retry := 1; retry < attempt && delay < restartBlobMaxDelay; retry++ {
		delay *= 2
		if delay > restartBlobMaxDelay {
			return restartBlobMaxDelay
		}
	}
	return delay
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
		if strings.TrimSpace(string(out)) != "tinkercloud: public_health_failed" {
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
		out, err := s.remoteOutput(ctx, "cat", "/var/lib/tinkercloud-vps-e2e/marker")
		if err != nil || string(out) != s.reuseMarker() {
			return errors.New("reuse refused: matching Tinkercloud VPS E2E marker is required")
		}
		return nil
	}
	// The ACME cache is deliberately not application state. A previous
	// disposable run may leave its account and still-valid certificates at the
	// fixed cache path so another clean install for the same domain does not
	// needlessly consume public CA issuance capacity. `tinkercloud init` still
	// applies the normal no-symlink/path checks before trusting that directory.
	for _, p := range []string{"/etc/tinkercloud", "/var/lib/tinkercloud", "/usr/local/bin/tinkercloud", "/etc/systemd/system/tinkercloud.service"} {
		if err := s.remote(ctx, "test", "!", "-e", p); err != nil {
			return fmt.Errorf("host is not clean (%s exists); set TINKERCLOUD_VPS_REUSE=1 only for a disposable existing Tinkercloud host: %w", p, err)
		}
	}
	return nil
}
func (s *Suite) reuseMarker() string {
	return "domain=" + s.Config.Domain + "\ntarget=" + s.Config.Target + "\n"
}

func tempSecret(dir string) (string, error) {
	b := make([]byte, 48)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	p := filepath.Join(dir, "tinkercloud-hmac.key")
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
	if s.Config.Reuse && s.Config.ReleaseDir == "" {
		return errors.New("reuse requires TINKERCLOUD_VPS_RELEASE_DIR so the current build can be verified and health-gated")
	}
	dir := s.Temp
	if dir == "" {
		var e error
		dir, e = os.MkdirTemp("", "tinkercloud-vps-e2e-")
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
	// Verify supplied reuse evidence locally before opening the SSH trust
	// boundary. A remote marker alone never authorizes an unverified update.
	if err := s.cleanGuard(ctx); err != nil {
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
	remoteDir := "/root/tinkercloud-vps-e2e-" + id
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
	files := []stagedFile{
		{filepath.Join(release, "tinkercloud-linux-amd64"), remoteDir + "/tinkercloud-linux-amd64"},
		{filepath.Join(release, "tinkercloud-linux-amd64.metadata.json"), remoteDir + "/tinkercloud-linux-amd64.metadata.json"},
		{filepath.Join(release, "tinkercloud-linux-amd64.signature"), remoteDir + "/tinkercloud-linux-amd64.signature"},
	}
	if !s.Config.Reuse {
		files = append(files,
			stagedFile{prepared.publicKey, remoteDir + "/packaging/release-public-key.pem"},
			stagedFile{repoPath("packaging", "install.sh"), remoteDir + "/packaging/install.sh"},
			stagedFile{repoPath("packaging", "systemd", "tinkercloud.service"), remoteDir + "/packaging/systemd/tinkercloud.service"},
			stagedFile{s.Config.ResendKeyFile, remoteDir + "/resend.key"},
			stagedFile{hmac, remoteDir + "/hmac.key"},
			stagedFile{markerPath, remoteDir + "/marker"},
		)
	} else {
		files = append(files, reuseUpdateFiles(release, remoteDir)...)
	}
	for _, v := range files {
		if err = s.copy(ctx, v.local, v.remote); err != nil {
			return err
		}
	}
	if !s.Config.Reuse {
		for _, p := range []string{remoteDir + "/resend.key", remoteDir + "/hmac.key"} {
			if err = s.remote(ctx, "chmod", "0600", p); err != nil {
				return err
			}
		}
	}
	if !s.Config.Reuse {
		if err = s.remote(ctx, "/bin/sh", remoteDir+"/packaging/install.sh", remoteDir+"/tinkercloud-linux-amd64", remoteDir+"/tinkercloud-linux-amd64.metadata.json", remoteDir+"/tinkercloud-linux-amd64.signature"); err != nil {
			return err
		}
		if err = s.initWithReadinessRetry(ctx, "/usr/local/bin/tinkercloud", "init", "--non-interactive", "--domain", s.Config.Domain, "--operator-email", s.Config.OperatorEmail, "--email-from", s.Config.EmailFrom, "--resend-api-key-file", remoteDir+"/resend.key", "--hmac-key-file", remoteDir+"/hmac.key"); err != nil {
			return err
		}
		if err = s.remote(ctx, "mkdir", "-m", "0700", "/var/lib/tinkercloud-vps-e2e"); err != nil {
			return err
		}
		if err = s.remote(ctx, "install", "-m", "0600", remoteDir+"/marker", "/var/lib/tinkercloud-vps-e2e/marker"); err != nil {
			return err
		}
	}
	if err = s.remote(ctx, "/usr/local/bin/tinkercloud", "doctor", "--config", "/etc/tinkercloud/config.yaml"); err != nil {
		return err
	}
	if err = s.socketInventory(ctx); err != nil {
		return err
	}
	if err = s.remote(ctx, "/usr/local/bin/tinkercloud", "deployers", "authorize", s.Config.DeployerEmail); err != nil {
		return err
	}
	if err = s.remote(ctx, "/usr/local/bin/tinkercloud", "deployers", "authorize", s.Config.ViewerEmail); err != nil {
		return err
	}
	return s.exercise(ctx, remoteDir)
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

type stagedFile struct{ local, remote string }

// reuseUpdateFiles names only signed, public release evidence. Reuse never
// recopies provider credentials, HMAC material, or a test signing authority.
func reuseUpdateFiles(release, remoteDir string) []stagedFile {
	return []stagedFile{
		{filepath.Join(release, "release-manifest.json"), remoteDir + "/release-manifest.json"},
		{filepath.Join(release, "release-manifest.json.metadata.json"), remoteDir + "/release-manifest.json.metadata.json"},
		{filepath.Join(release, "release-manifest.json.signature"), remoteDir + "/release-manifest.json.signature"},
	}
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
	if e := commandEnv(ctx, []string{"TINKERCLOUD_RELEASE_SIGNING_KEY=" + key, "TINKERCLOUD_RELEASE_PUBLIC_KEY=" + pub}, repoPath("scripts", "release-build.sh"), version, out); e != nil {
		return preparedRelease{}, e
	}
	if e := commandEnv(ctx, []string{"TINKERCLOUD_RELEASE_PUBLIC_KEY=" + pub}, repoPath("scripts", "release-verify.sh"), out); e != nil {
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
	email         string
	ctx           context.Context
}

func (p otpPrompt) Ask(q string) (string, error) {
	email := p.email
	if email == "" {
		email = p.config.DeployerEmail
	}
	if strings.HasPrefix(q, "Email:") {
		return email, nil
	}
	return readOTP(p.ctx, p.config, p.purpose, email, p.host)
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
	return "", errors.New("TINKERCLOUD_VPS_OTP_COMMAND is required without an interactive terminal")
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

func (s *Suite) exercise(ctx context.Context, remoteDir string) error {
	base := "https://" + s.Config.PlatformHost()
	login, err := client.Login(ctx, base, otpPrompt{ctx: ctx, config: s.Config, purpose: "deployer", host: s.Config.PlatformHost()})
	if err != nil {
		return fmt.Errorf("deployer OTP login: %w", err)
	}
	c := client.New(base, login.Token)
	// A previous interrupted acceptance run can leave only this suite's known
	// fixture apps behind. ListApps is ownership-scoped, so delete exclusively
	// names returned for this authenticated deployer; never probe or remove any
	// other app. Each deletion receives a new key because deletion is durable
	// and a prior invocation may have completed after the client lost its reply.
	if err := resetFixtureApps(ctx, c); err != nil {
		return err
	}
	secondLogin, err := client.Login(ctx, base, otpPrompt{ctx: ctx, config: s.Config, purpose: "deployer", host: s.Config.PlatformHost(), email: s.Config.ViewerEmail})
	if err != nil {
		return fmt.Errorf("second-owner deployer OTP login: %w", err)
	}
	secondOwner := client.New(base, secondLogin.Token)
	if err := resetFixtureApps(ctx, secondOwner); err != nil {
		return err
	}
	// A reused VPS may deliberately be on a pre-blob release. Deploy a legacy
	// manifest solely to create the updater's active anonymous-denial probe,
	// then upgrade through the signed health-gated path before any blob feature
	// is requested or parsed by that old server.
	if s.Config.Reuse {
		if _, _, err := s.deploySmokeApp(ctx, c, vpsUpdateProbeSlug, false); err != nil {
			return fmt.Errorf("deploy legacy update probe: %w", err)
		}
		if err := s.applyReuseUpdate(ctx, remoteDir, vpsUpdateProbeSlug); err != nil {
			return err
		}
	}
	publicState, err := s.beginPublicMatrix(ctx, c, secondOwner)
	if err != nil {
		return err
	}
	slug := vpsPrimaryAppSlug
	appHost, marker, err := s.deploySmokeAppWithRedeploy(ctx, c, slug, true)
	if err != nil {
		return err
	}
	if err = s.anonymousDenied(ctx, appHost, marker); err != nil {
		return err
	}
	if err = s.anonymousLLMChatDenied(ctx, appHost); err != nil {
		return err
	}
	// The dashboard signs the browser in once. Each app later receives only a
	// server-derived, host-only child session through its handoff; the second
	// allowed app must not need another OTP. A later, explicit account-switch
	// security case intentionally verifies a separate viewer-purpose OTP.
	viewer, blobID, firstCallback, firstState, err := s.firstViewerFlow(ctx, appHost, marker)
	if err != nil {
		return err
	}
	deniedHost, deniedMarker, err := s.deploySmokeAppForViewer(ctx, c, vpsDeniedAppSlug, s.Config.DeployerEmail, false)
	if err != nil {
		return fmt.Errorf("deploy catalog-denied fixture: %w", err)
	}
	if err := s.verifyPublicCatalog(ctx, viewer, publicState, marker, deniedMarker); err != nil {
		return err
	}
	if err := s.revokePrimaryViewerAndRestore(ctx, c, viewer, appHost, marker); err != nil {
		return err
	}
	// Collections deliberately share the same app-private database as KV. This
	// is gateway evidence, not a database inspection: every request uses the
	// viewer's host-only cookie and never supplies an app identity.
	collectionID, err := s.exerciseCollections(ctx, viewer, appHost)
	if err != nil {
		return err
	}
	if err := s.anonymousCollectionDenied(ctx, appHost, collectionID); err != nil {
		return err
	}
	if err := s.anonymousBlobDenied(ctx, appHost, blobID); err != nil {
		return err
	}
	if err := s.remote(ctx, "systemctl", "restart", "tinkercloud.service"); err != nil {
		return fmt.Errorf("restart service for app-data durability check: %w", err)
	}
	if err := s.verifyBlobPersistsAfterRestart(ctx, viewer, appHost, blobID); err != nil {
		return err
	}
	if err := s.verifyCollectionPersistsAfterRestart(ctx, viewer, appHost, collectionID); err != nil {
		return err
	}
	if err := s.verifyPublicAfterRestart(ctx, publicState); err != nil {
		return err
	}
	secondHost, err := s.crossAppBlobDenied(ctx, c, slug, appHost, blobID, viewer)
	if err != nil {
		return err
	}
	if err := s.crossAppCollectionDenied(ctx, viewer, secondHost, collectionID); err != nil {
		return err
	}
	if err := s.replayedAndWrongAppHandoffsDeny(ctx, viewer, appHost, secondHost, firstCallback, firstState, marker); err != nil {
		return err
	}
	// Delete while the original viewer session remains available; the following
	// global account switch intentionally revokes every child app session.
	if err := s.deleteBlobAndVerify(ctx, viewer, appHost, blobID); err != nil {
		return err
	}
	if err := s.deleteCollectionAndVerify(ctx, viewer, appHost, collectionID); err != nil {
		return err
	}
	if err := s.appLocalLogoutAndBrokerReopen(ctx, viewer, appHost, secondHost, marker); err != nil {
		return err
	}
	if err := s.globalIdentityDeniedApp(ctx, viewer, deniedHost, deniedMarker, appHost, secondHost); err != nil {
		return err
	}
	if err := s.finishPublicMatrix(ctx, c, secondOwner, viewer, publicState); err != nil {
		return err
	}
	return s.dashboardGlobalLogout(ctx, viewer, appHost, secondHost, deniedHost)
}

type publicMatrixState struct {
	host, otherHost        string
	marker, asset          string
	otherMarker            string
	activeDeployment       string
	secondActiveDeployment string
	analyticsClient        *http.Client
}

func (s *Suite) setPublicGate(ctx context.Context, enabled bool) error {
	action := "disable"
	if enabled {
		action = "enable"
	}
	if err := s.remote(ctx, "/usr/local/bin/tinkercloud", "public", action, "--config", "/etc/tinkercloud/config.yaml"); err != nil {
		return fmt.Errorf("set public gate %s: %w", action, err)
	}
	return nil
}

func (s *Suite) beginPublicMatrix(ctx context.Context, owner, secondOwner client.Client) (publicMatrixState, error) {
	state := publicMatrixState{host: vpsPublicAppSlug + "." + s.Config.AppSuffix(), otherHost: vpsPublicOtherSlug + "." + s.Config.AppSuffix()}
	if s.Config.Reuse {
		if err := s.setPublicGate(ctx, false); err != nil {
			return state, err
		}
	}
	for _, fixture := range []struct {
		api  client.Client
		slug string
	}{{owner, vpsPublicAppSlug}, {secondOwner, vpsPublicOtherSlug}} {
		if err := fixture.api.EnsureApp(ctx, fixture.slug); err != nil {
			return state, fmt.Errorf("create public fixture %q: %w", fixture.slug, err)
		}
		if err := s.warmCertificate(ctx, fixture.slug+"."+s.Config.AppSuffix()); err != nil {
			return state, err
		}
	}

	privateArchive, privateSize, privateMarker, _, err := publicArchive(vpsPublicAppSlug, "private", false, false, s.Config.DeployerEmail)
	if err != nil {
		return state, fmt.Errorf("build public-matrix private baseline fixture: %w", err)
	}
	privateKey, err := client.IdempotencyKey()
	if err != nil {
		return state, err
	}
	baseline, err := owner.Deploy(ctx, vpsPublicAppSlug, bytes.NewReader(privateArchive), privateSize, privateKey)
	if err != nil || baseline.Posture != client.PosturePrivate {
		return state, fmt.Errorf("deploy public-matrix private baseline: %w", err)
	}

	publicArchiveBytes, publicSize, marker, asset, err := publicArchive(vpsPublicAppSlug, "public", true, false, s.Config.DeployerEmail)
	if err != nil {
		return state, fmt.Errorf("build public-matrix indexed public fixture: %w", err)
	}
	confirmed := owner
	confirmed.PublicAcknowledged = true
	key, err := client.IdempotencyKey()
	if err != nil {
		return state, err
	}
	if _, err = confirmed.Deploy(ctx, vpsPublicAppSlug, bytes.NewReader(publicArchiveBytes), publicSize, key); err == nil {
		return state, errors.New("gate-off public candidate activated")
	}
	if err := assertActiveDeployment(ctx, owner, vpsPublicAppSlug, baseline.DeploymentID); err != nil {
		return state, fmt.Errorf("gate-off candidate replaced private baseline: %w", err)
	}
	if err := s.anonymousDenied(ctx, state.host, privateMarker); err != nil {
		return state, fmt.Errorf("gate-off public fixture exposed bytes: %w", err)
	}
	if err := s.setPublicGate(ctx, true); err != nil {
		return state, err
	}

	unacknowledged := owner
	key, _ = client.IdempotencyKey()
	if _, err = unacknowledged.Deploy(ctx, vpsPublicAppSlug, bytes.NewReader(publicArchiveBytes), publicSize, key); err == nil {
		return state, errors.New("unacknowledged public candidate activated")
	}
	if err := assertActiveDeployment(ctx, owner, vpsPublicAppSlug, baseline.DeploymentID); err != nil {
		return state, fmt.Errorf("unacknowledged candidate replaced private baseline: %w", err)
	}
	capabilityArchive, capabilitySize, _, _, err := publicArchive(vpsPublicAppSlug, "public", false, true, s.Config.DeployerEmail)
	if err != nil {
		return state, fmt.Errorf("build public-matrix capability-bearing public fixture: %w", err)
	}
	key, _ = client.IdempotencyKey()
	if _, err = confirmed.Deploy(ctx, vpsPublicAppSlug, bytes.NewReader(capabilityArchive), capabilitySize, key); err == nil {
		return state, errors.New("capability-bearing public candidate activated")
	}
	if err := assertActiveDeployment(ctx, owner, vpsPublicAppSlug, baseline.DeploymentID); err != nil {
		return state, fmt.Errorf("capability-bearing candidate replaced private baseline: %w", err)
	}
	// The manifest is structurally valid, but the archive deliberately omits
	// index.html. It must fail before activation and leave the private baseline
	// active. Validation necessarily catches this before the candidate probe;
	// the production-composition suite separately injects its probe failure.
	missingIndexArchive, missingIndexSize, err := publicArchiveWithoutIndex(vpsPublicAppSlug)
	if err != nil {
		return state, fmt.Errorf("build missing-index public candidate: %w", err)
	}
	key, _ = client.IdempotencyKey()
	if _, err = confirmed.Deploy(ctx, vpsPublicAppSlug, bytes.NewReader(missingIndexArchive), missingIndexSize, key); err == nil {
		return state, errors.New("missing-index public candidate activated")
	}
	if err := assertActiveDeployment(ctx, owner, vpsPublicAppSlug, baseline.DeploymentID); err != nil {
		return state, fmt.Errorf("missing-index candidate replaced private baseline: %w", err)
	}
	if err := malformedPublicAcknowledgementDenied(ctx, owner, vpsPublicAppSlug, publicArchiveBytes); err != nil {
		return state, err
	}
	if err := assertActiveDeployment(ctx, owner, vpsPublicAppSlug, baseline.DeploymentID); err != nil {
		return state, fmt.Errorf("malformed acknowledgement replaced private baseline: %w", err)
	}

	key, _ = client.IdempotencyKey()
	active, err := confirmed.Deploy(ctx, vpsPublicAppSlug, bytes.NewReader(publicArchiveBytes), publicSize, key)
	if err != nil || active.Posture != client.PosturePublicStatic || active.PublicStatic == nil || !active.PublicStatic.Indexing {
		return state, fmt.Errorf("deploy confirmed public fixture: %w", err)
	}
	state.marker, state.asset, state.activeDeployment = marker, asset, active.DeploymentID

	otherArchive, otherSize, otherMarker, _, err := publicArchive(vpsPublicOtherSlug, "public", false, false, s.Config.ViewerEmail)
	if err != nil {
		return state, fmt.Errorf("build second-owner public fixture: %w", err)
	}
	secondConfirmed := secondOwner
	secondConfirmed.PublicAcknowledged = true
	key, _ = client.IdempotencyKey()
	otherActive, err := secondConfirmed.Deploy(ctx, vpsPublicOtherSlug, bytes.NewReader(otherArchive), otherSize, key)
	if err != nil || otherActive.Posture != client.PosturePublicStatic || otherActive.PublicStatic == nil || otherActive.PublicStatic.Indexing {
		return state, fmt.Errorf("deploy second-owner public fixture: %w", err)
	}
	state.otherMarker, state.secondActiveDeployment = otherMarker, otherActive.DeploymentID
	if err := assertOwnerIsolation(ctx, owner, secondOwner); err != nil {
		return state, err
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return state, err
	}
	h := s.httpClient()
	hc := *h
	hc.Jar = jar
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	state.analyticsClient = &hc
	if err := verifyExactPublic(ctx, &hc, state.host, state.marker, state.asset, true); err != nil {
		return state, err
	}
	if err := verifyPublicReservedDenials(ctx, &hc, state.host, state.marker, state.asset); err != nil {
		return state, err
	}
	// Exactly one more document view in the same app-host jar: together with the
	// root document above this is the 2 page-view / 1 approximate-visitor proof.
	if err := requestExactPublicDocument(ctx, &hc, state.host, state.marker, true); err != nil {
		return state, err
	}
	return state, nil
}

func assertActiveDeployment(ctx context.Context, c client.Client, slug, want string) error {
	releases, err := c.ListReleases(ctx, slug)
	if err != nil {
		return err
	}
	for _, release := range releases {
		if release.State == "active" {
			if release.ID == want {
				return nil
			}
			return fmt.Errorf("active deployment=%q want=%q", release.ID, want)
		}
	}
	return errors.New("active deployment missing")
}

func assertOwnerIsolation(ctx context.Context, first, second client.Client) error {
	firstApps, err := first.ListApps(ctx)
	if err != nil {
		return err
	}
	secondApps, err := second.ListApps(ctx)
	if err != nil {
		return err
	}
	if !containsOwnedApp(firstApps, vpsPublicAppSlug) || containsOwnedApp(firstApps, vpsPublicOtherSlug) || !containsOwnedApp(secondApps, vpsPublicOtherSlug) || containsOwnedApp(secondApps, vpsPublicAppSlug) {
		return errors.New("two-owner fixture lists crossed ownership")
	}
	if _, err := first.Access(ctx, vpsPublicOtherSlug); err == nil {
		return errors.New("first owner read second owner's public policy")
	}
	if _, err := second.Access(ctx, vpsPublicAppSlug); err == nil {
		return errors.New("second owner read first owner's public policy")
	}
	return nil
}

func containsOwnedApp(apps []client.AppSummary, slug string) bool {
	for _, app := range apps {
		if app.Slug == slug && app.Status == "active" {
			return true
		}
	}
	return false
}

func publicArchive(slug, mode string, indexing, capability bool, viewerEmail string) ([]byte, int64, string, string, error) {
	if mode != "private" && mode != "public" {
		return nil, 0, "", "", errors.New("invalid public fixture mode")
	}
	d, err := os.MkdirTemp("", "tinkercloud-vps-public-")
	if err != nil {
		return nil, 0, "", "", err
	}
	defer os.RemoveAll(d)
	id, err := randomID()
	if err != nil {
		return nil, 0, "", "", err
	}
	marker := "TINKERCLOUD_VPS_PUBLIC_DOCUMENT_" + id
	asset := "TINKERCLOUD_VPS_PUBLIC_ASSET_" + id
	if err := os.WriteFile(filepath.Join(d, "index.html"), []byte("<!doctype html><title>public matrix</title>"+marker), 0644); err != nil {
		return nil, 0, "", "", err
	}
	if err := os.WriteFile(filepath.Join(d, "asset.txt"), []byte(asset), 0644); err != nil {
		return nil, 0, "", "", err
	}
	if err := os.WriteFile(filepath.Join(d, "private.js"), []byte("window.fixture='"+marker+"'"), 0644); err != nil {
		return nil, 0, "", "", err
	}
	manifest := "version: 2\nname: " + slug + "\ndescription: VPS public reach fixture\ntags:\n  - acceptance\n  - public\nbuild:\n  output: .\naccess:\n  mode: " + mode + "\n"
	if mode == "public" {
		manifest += "  indexing: " + strconv.FormatBool(indexing) + "\n"
	} else {
		manifest += "  allow:\n    emails:\n      - " + viewerEmail + "\n    domains: []\n"
	}
	if capability {
		manifest += "features:\n  kv: true\n"
	}
	manifestBytes := []byte(manifest)
	if err := os.WriteFile(filepath.Join(d, "tinker.yaml"), manifestBytes, 0644); err != nil {
		return nil, 0, "", "", err
	}
	var archive bytes.Buffer
	if capability {
		// A public release with a browser capability is intentionally malformed.
		// ArchiveProject correctly rejects it locally, but the VPS suite must send
		// this hostile archive to prove that the server rejects it too.
		if err := archivePublicFixture(&archive, manifestBytes, map[string][]byte{
			"asset.txt":  []byte(asset),
			"index.html": []byte("<!doctype html><title>public matrix</title>" + marker),
			"private.js": []byte("window.fixture='" + marker + "'"),
		}); err != nil {
			return nil, 0, "", "", fmt.Errorf("archive public fixture mode=%s indexing=%t capability=%t: %w", mode, indexing, capability, err)
		}
	} else if err := client.ArchiveProject(d, manifestBytes, &archive); err != nil {
		return nil, 0, "", "", fmt.Errorf("archive public fixture mode=%s indexing=%t capability=%t: %w", mode, indexing, capability, err)
	}
	return archive.Bytes(), int64(archive.Len()), marker, asset, nil
}

// publicArchiveWithoutIndex builds a manifest-valid public archive that fails
// release validation. It is deliberately distinct from a candidate-probe
// failure: the production service validates index.html before a candidate can
// reach verification, so a clean black-box run cannot honestly manufacture a
// probe failure without a test-only production seam.
func publicArchiveWithoutIndex(slug string) ([]byte, int64, error) {
	d, err := os.MkdirTemp("", "tinkercloud-vps-public-no-index-")
	if err != nil {
		return nil, 0, err
	}
	defer os.RemoveAll(d)
	manifest := []byte("version: 2\nname: " + slug + "\nbuild:\n  output: .\naccess:\n  mode: public\n  indexing: false\n")
	if err := os.WriteFile(filepath.Join(d, "asset.txt"), []byte("missing-index asset"), 0644); err != nil {
		return nil, 0, err
	}
	if err := os.WriteFile(filepath.Join(d, "tinker.yaml"), manifest, 0644); err != nil {
		return nil, 0, err
	}
	var archive bytes.Buffer
	if err := client.ArchiveProject(d, manifest, &archive); err != nil {
		return nil, 0, err
	}
	return archive.Bytes(), int64(archive.Len()), nil
}

// archivePublicFixture writes a deliberately unvalidated archive for one
// negative server-side deployment case. Its layout matches ArchiveProject:
// manifest first, then deterministic asset paths, with no duplicate manifest.
func archivePublicFixture(w io.Writer, manifest []byte, files map[string][]byte) error {
	gz := gzip.NewWriter(w)
	gz.Header.ModTime = time.Unix(0, 0)
	tw := tar.NewWriter(gz)
	entries := []struct {
		name string
		body []byte
	}{{"tinker.yaml", manifest}, {"asset.txt", files["asset.txt"]}, {"index.html", files["index.html"]}, {"private.js", files["private.js"]}}
	for _, entry := range entries {
		h := &tar.Header{Name: entry.name, Mode: 0644, Size: int64(len(entry.body)), ModTime: time.Unix(0, 0), Format: tar.FormatPAX}
		if err := tw.WriteHeader(h); err != nil {
			return err
		}
		if _, err := tw.Write(entry.body); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

func malformedPublicAcknowledgementDenied(ctx context.Context, c client.Client, slug string, archive []byte) error {
	key, err := client.IdempotencyKey()
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.Base, "/")+"/api/v1/apps/"+url.PathEscape(slug)+"/deployments", bytes.NewReader(archive))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/gzip")
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Idempotency-Key", key)
	req.Header["X-Tinker-Public-Acknowledged"] = []string{"true", "true"}
	h := c.HTTP
	if h == nil {
		h = http.DefaultClient
	}
	resp, err := h.Do(req)
	if err != nil {
		return err
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 32<<10))
	resp.Body.Close()
	if readErr != nil || resp.StatusCode != http.StatusBadRequest || bytes.Contains(body, archive) {
		return fmt.Errorf("malformed public acknowledgement was not a bounded denial: status=%d", resp.StatusCode)
	}
	return nil
}

func requestExactPublicDocument(ctx context.Context, h *http.Client, host, marker string, indexing bool) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, appURL(host, "/"), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/html")
	req.Header.Set("Sec-Fetch-Dest", "document")
	resp, err := h.Do(req)
	if err != nil {
		return err
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	want := "noindex, nofollow"
	if indexing {
		want = ""
	}
	exact := "<!doctype html><title>public matrix</title>" + marker
	if readErr != nil || resp.StatusCode != http.StatusOK || string(body) != exact || resp.Header.Get("X-Robots-Tag") != want || resp.Header.Get("Cache-Control") != "no-store" {
		return fmt.Errorf("public document mismatch: status=%d robots=%q", resp.StatusCode, resp.Header.Get("X-Robots-Tag"))
	}
	return nil
}

func verifyExactPublic(ctx context.Context, h *http.Client, host, marker, asset string, indexing bool) error {
	if err := requestExactPublicDocument(ctx, h, host, marker, indexing); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, appURL(host, "/asset.txt"), nil)
	if err != nil {
		return err
	}
	resp, err := h.Do(req)
	if err != nil {
		return err
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	if readErr != nil || resp.StatusCode != http.StatusOK || string(body) != asset || resp.Header.Get("Cache-Control") != "no-store" {
		return fmt.Errorf("public asset mismatch: status=%d", resp.StatusCode)
	}
	return nil
}

// RunPublicExampleAcceptance verifies the separately deployed first-party
// public example through HTTPS only. It intentionally does not call Run,
// create a Suite runner, deploy, clean, install, SSH, or mutate host state.
// The normal dashboard OTP flow is used solely to read the owner-visible
// catalog and aggregate view after two anonymous document requests.
func (s *Suite) RunPublicExampleAcceptance(ctx context.Context) error {
	index, stylesheet, err := publicExampleAssets()
	if err != nil {
		return err
	}
	host := publicExampleSlug + "." + s.Config.AppSuffix()
	jar, err := cookiejar.New(nil)
	if err != nil {
		return err
	}
	base := s.httpClient()
	anonymous := *base
	anonymous.Jar = jar
	anonymous.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	for i := 0; i < 2; i++ {
		if err := requestExactPublicExampleDocument(ctx, &anonymous, host, index); err != nil {
			return fmt.Errorf("public example document view %d: %w", i+1, err)
		}
	}
	if err := requestExactPublicExampleAsset(ctx, &anonymous, host, stylesheet); err != nil {
		return err
	}
	if err := verifyPublicReservedDenials(ctx, &anonymous, host, string(index), string(stylesheet)); err != nil {
		return err
	}

	owner, err := s.dashboardBrowserForEmail(ctx, s.Config.DeployerEmail)
	if err != nil {
		return fmt.Errorf("public example owner dashboard login: %w", err)
	}
	catalog, status, err := fetchPlatformPage(ctx, owner, "https://"+s.Config.PlatformHost()+"/apps")
	if err != nil || status != http.StatusOK || !strings.Contains(catalog, `data-tinker-catalog-slug="`+publicExampleSlug+`"`) {
		return fmt.Errorf("public example owner catalog missing card: status=%d err=%w", status, err)
	}
	if err := s.waitDashboardInsights(ctx, owner, publicExampleSlug, 2, 1); err != nil {
		return fmt.Errorf("public example owner insights: %w", err)
	}
	return nil
}

func publicExampleAssets() ([]byte, []byte, error) {
	index, err := os.ReadFile(repoPath("examples", "public-static-product-story", "index.html"))
	if err != nil || len(index) == 0 {
		return nil, nil, errors.New("public example index is unavailable")
	}
	stylesheet, err := os.ReadFile(repoPath("examples", "public-static-product-story", "styles.css"))
	if err != nil || len(stylesheet) == 0 {
		return nil, nil, errors.New("public example stylesheet is unavailable")
	}
	return index, stylesheet, nil
}

func requestExactPublicExampleDocument(ctx context.Context, h *http.Client, host string, expected []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, appURL(host, "/"), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/html")
	req.Header.Set("Sec-Fetch-Dest", "document")
	resp, err := h.Do(req)
	if err != nil {
		return err
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, int64(len(expected))+1))
	resp.Body.Close()
	if readErr != nil || resp.StatusCode != http.StatusOK || !bytes.Equal(body, expected) || resp.Header.Get("Cache-Control") != "no-store" || resp.Header.Get("X-Content-Type-Options") != "nosniff" || !strings.HasPrefix(strings.ToLower(resp.Header.Get("Content-Type")), "text/html") || resp.Header.Get("X-Robots-Tag") != "" {
		return fmt.Errorf("public example document mismatch: status=%d", resp.StatusCode)
	}
	return nil
}

func requestExactPublicExampleAsset(ctx context.Context, h *http.Client, host string, expected []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, appURL(host, "/styles.css"), nil)
	if err != nil {
		return err
	}
	resp, err := h.Do(req)
	if err != nil {
		return err
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, int64(len(expected))+1))
	resp.Body.Close()
	if readErr != nil || resp.StatusCode != http.StatusOK || !bytes.Equal(body, expected) || resp.Header.Get("Cache-Control") != "no-store" || resp.Header.Get("X-Content-Type-Options") != "nosniff" || !strings.HasPrefix(strings.ToLower(resp.Header.Get("Content-Type")), "text/css") || resp.Header.Get("X-Robots-Tag") != "" {
		return fmt.Errorf("public example stylesheet mismatch: status=%d", resp.StatusCode)
	}
	return nil
}

func verifyPublicReservedDenials(ctx context.Context, h *http.Client, host string, forbidden ...string) error {
	routes := []struct{ method, path string }{
		{http.MethodGet, "/_tinker/auth/login"},
		{http.MethodGet, "/_tinker/auth/callback"},
		{http.MethodPost, "/_tinker/auth/logout"},
		{http.MethodPost, "/_tinker/auth/otp"},
		{http.MethodGet, "/_tinker/api/v1/me"},
		{http.MethodGet, "/_tinker/api/v1/app"},
		{http.MethodGet, "/_tinker/api/v1/capabilities"},
		{http.MethodGet, "/_tinker/api/v1/kv/key"},
		{http.MethodPost, "/_tinker/api/v1/db/items"},
		{http.MethodGet, "/_tinker/api/v1/blobs"},
		{http.MethodGet, "/_tinker/ws/v1"},
		{http.MethodPost, "/_tinker/api/v1/llm/chat"},
	}
	for _, route := range routes {
		req, err := http.NewRequestWithContext(ctx, route.method, appURL(host, route.path), nil)
		if err != nil {
			return err
		}
		if route.path == "/_tinker/ws/v1" {
			req.Header.Set("Connection", "Upgrade")
			req.Header.Set("Upgrade", "websocket")
			req.Header.Set("Sec-WebSocket-Version", "13")
			req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
		}
		resp, err := h.Do(req)
		if err != nil {
			return err
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 32<<10))
		resp.Body.Close()
		if readErr != nil || resp.StatusCode != http.StatusNotFound || resp.Header.Get("Cache-Control") != "no-store" || resp.Header.Get("X-Content-Type-Options") != "nosniff" || resp.Header.Get("Location") != "" || resp.Header.Get("Set-Cookie") != "" || !bytes.Contains(body, []byte(`"not_found"`)) {
			return fmt.Errorf("public reserved route %s %s was not exact denial: status=%d", route.method, route.path, resp.StatusCode)
		}
		for _, secret := range forbidden {
			if secret != "" && bytes.Contains(body, []byte(secret)) {
				return fmt.Errorf("public reserved route %s %s leaked app bytes", route.method, route.path)
			}
		}
	}
	return nil
}

// verifyPublicCatalog proves the viewer's server-derived catalog has every
// currently accessible fixture card while disclosing neither a policy-denied
// card nor application bytes. It does not infer access from a card: app entry
// points later re-check the current policy through the normal handoff.
func (s *Suite) verifyPublicCatalog(ctx context.Context, viewer *http.Client, state publicMatrixState, primaryMarker, deniedMarker string) error {
	body, status, err := fetchPlatformPage(ctx, viewer, "https://"+s.Config.PlatformHost()+"/apps")
	if err != nil || status != http.StatusOK {
		return fmt.Errorf("verified viewer catalog unavailable: status=%d err=%w", status, err)
	}
	for _, slug := range []string{vpsPrimaryAppSlug, vpsPublicAppSlug, vpsPublicOtherSlug} {
		if !strings.Contains(body, `data-tinker-catalog-slug="`+slug+`"`) {
			return fmt.Errorf("verified catalog omitted allowed app %q", slug)
		}
	}
	if strings.Contains(body, `data-tinker-catalog-slug="`+vpsDeniedAppSlug+`"`) || strings.Contains(body, primaryMarker) || strings.Contains(body, deniedMarker) || strings.Contains(body, state.marker) || strings.Contains(body, state.asset) {
		return errors.New("verified catalog disclosed denied metadata or app bytes")
	}
	return nil
}

// revokePrimaryViewerAndRestore uses the supported revisioned access-policy
// API to prove that a verified global identity immediately loses both catalog
// discovery and its app session. Restoration is necessary for the following
// SDK and session checks. The existing host-only session may resume only after
// the policy is restored; the revocation proof above must deny it first.
func (s *Suite) revokePrimaryViewerAndRestore(ctx context.Context, owner client.Client, viewer *http.Client, host, marker string) error {
	if err := replacePrivateEmailPolicy(ctx, owner, vpsPrimaryAppSlug, nil, false); err != nil {
		return fmt.Errorf("revoke primary viewer policy: %w", err)
	}
	body, status, err := fetchPlatformPage(ctx, viewer, "https://"+s.Config.PlatformHost()+"/apps")
	if err != nil || status != http.StatusOK {
		return fmt.Errorf("revoked viewer catalog unavailable: status=%d err=%w", status, err)
	}
	if strings.Contains(body, `data-tinker-catalog-slug="`+vpsPrimaryAppSlug+`"`) {
		return errors.New("revoked viewer retained private app catalog card")
	}
	if err := s.assertAppDenied(ctx, viewer, host, marker); err != nil {
		return fmt.Errorf("revoked viewer retained primary app access: %w", err)
	}
	if err := replacePrivateEmailPolicy(ctx, owner, vpsPrimaryAppSlug, []string{s.Config.ViewerEmail}, true); err != nil {
		return fmt.Errorf("restore primary viewer policy: %w", err)
	}
	if err := s.assertExactAppDocument(ctx, viewer, host, []byte("<!doctype html><title>tinker</title>"+marker)); err != nil {
		return fmt.Errorf("restored primary viewer did not regain its scoped app access: %w", err)
	}
	body, status, err = fetchPlatformPage(ctx, viewer, "https://"+s.Config.PlatformHost()+"/apps")
	if err != nil || status != http.StatusOK || !strings.Contains(body, `data-tinker-catalog-slug="`+vpsPrimaryAppSlug+`"`) {
		return fmt.Errorf("restored viewer catalog did not contain primary app: status=%d err=%w", status, err)
	}
	return nil
}

// replacePrivateEmailPolicy only drives the supported owner-scoped control API.
// It reads the current server revision immediately before the mutation and
// never accepts a caller-supplied app or identity selector.
func replacePrivateEmailPolicy(ctx context.Context, owner client.Client, slug string, emails []string, confirmBroadening bool) error {
	current, err := owner.Access(ctx, slug)
	if err != nil || current.Mode != "private" || current.Revision == 0 {
		return errors.New("current private access policy unavailable")
	}
	if emails == nil {
		emails = []string{}
	}
	key, err := client.IdempotencyKey()
	if err != nil {
		return err
	}
	policy := map[string]any{
		"mode":               "private",
		"expected_revision":  current.Revision,
		"confirm_broadening": confirmBroadening,
		"allow": map[string]any{
			"emails":  emails,
			"domains": []string{},
		},
	}
	if err := owner.Do(ctx, http.MethodPut, "/api/v1/apps/"+url.PathEscape(slug)+"/access", key, policy, nil); err != nil {
		return err
	}
	return nil
}

func (s *Suite) verifyPublicAfterRestart(ctx context.Context, state publicMatrixState) error {
	if state.analyticsClient == nil {
		return errors.New("public analytics client missing")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, appURL(state.host, "/asset.txt"), nil)
	if err != nil {
		return err
	}
	resp, err := state.analyticsClient.Do(req)
	if err != nil {
		return fmt.Errorf("public restart persistence request: %w", err)
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	if readErr != nil || resp.StatusCode != http.StatusOK || string(body) != state.asset {
		return fmt.Errorf("public bytes did not persist across restart: status=%d", resp.StatusCode)
	}
	return verifyPublicReservedDenials(ctx, state.analyticsClient, state.host, state.marker, state.asset)
}

func (s *Suite) finishPublicMatrix(ctx context.Context, owner, secondOwner client.Client, ownerBrowser *http.Client, state publicMatrixState) error {
	if err := s.waitDashboardInsights(ctx, ownerBrowser, vpsPublicAppSlug, 2, 1); err != nil {
		return err
	}
	body, status, err := fetchPlatformPage(ctx, ownerBrowser, "https://"+s.Config.PlatformHost()+"/dashboard")
	if err != nil || status != http.StatusOK {
		return fmt.Errorf("owner dashboard unavailable: status=%d err=%w", status, err)
	}
	if s.deployerDashboardIsOperator() {
		// The configured deployer can also be the initial operator. That role is
		// deliberately allowed to read all app summaries, so this dashboard is
		// not ownership-isolation evidence. It must still include both fixture
		// owners while the scoped API proof and second deployer dashboard below
		// enforce isolation.
		if !strings.Contains(body, `data-tinker-app-slug="`+vpsPublicAppSlug+`"`) || !strings.Contains(body, `data-tinker-app-slug="`+vpsPublicOtherSlug+`"`) {
			return errors.New("operator dashboard omitted a public fixture app")
		}
	} else if strings.Contains(body, `data-tinker-app-slug="`+vpsPublicOtherSlug+`"`) {
		return errors.New("first owner dashboard disclosed second owner's app")
	}

	secondBrowser, err := s.dashboardBrowserForEmail(ctx, s.Config.ViewerEmail)
	if err != nil {
		return fmt.Errorf("second-owner dashboard login: %w", err)
	}
	secondBody, secondStatus, err := fetchPlatformPage(ctx, secondBrowser, "https://"+s.Config.PlatformHost()+"/dashboard")
	if err != nil || secondStatus != http.StatusOK {
		return fmt.Errorf("second-owner dashboard unavailable: status=%d err=%w", secondStatus, err)
	}
	if !strings.Contains(secondBody, `data-tinker-app-slug="`+vpsPublicOtherSlug+`"`) || strings.Contains(secondBody, `data-tinker-app-slug="`+vpsPublicAppSlug+`"`) {
		return errors.New("second-owner dashboard crossed app ownership")
	}
	if err := assertOwnerIsolation(ctx, owner, secondOwner); err != nil {
		return err
	}

	if err := s.setPublicGate(ctx, false); err != nil {
		return err
	}
	if err := assertAnonymousStaticDenied(ctx, s.httpClient(), state.host, state.marker, state.asset); err != nil {
		return fmt.Errorf("gate-disable next-request denial: %w", err)
	}
	if err := s.viewerFlowWithExistingIdentityForEmail(ctx, ownerBrowser, state.host, state.marker, s.Config.DeployerEmail); err != nil {
		return fmt.Errorf("private owner access after gate disable: %w", err)
	}
	if err := s.setPublicGate(ctx, true); err != nil {
		return err
	}
	if err := verifyExactPublic(ctx, state.analyticsClient, state.host, state.marker, state.asset, true); err != nil {
		return fmt.Errorf("public re-enable: %w", err)
	}

	privateArchive, privateSize, privateMarker, _, err := publicArchive(vpsPublicAppSlug, "private", false, false, s.Config.DeployerEmail)
	if err != nil {
		return fmt.Errorf("build public-to-private fixture: %w", err)
	}
	key, err := client.IdempotencyKey()
	if err != nil {
		return err
	}
	privateResult, err := owner.Deploy(ctx, vpsPublicAppSlug, bytes.NewReader(privateArchive), privateSize, key)
	if err != nil || privateResult.Posture != client.PosturePrivate {
		return fmt.Errorf("public-to-private deployment: %w", err)
	}
	if err := assertAnonymousStaticDenied(ctx, s.httpClient(), state.host, privateMarker, state.asset); err != nil {
		return fmt.Errorf("public-to-private next-request denial: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, appURL(state.host, "/"), nil)
	if err != nil {
		return err
	}
	resp, err := ownerBrowser.Do(req)
	if err != nil {
		return err
	}
	privateBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	if readErr != nil || resp.StatusCode != http.StatusOK || !bytes.Contains(privateBody, []byte(privateMarker)) {
		return fmt.Errorf("owner session failed after public-to-private transition: status=%d", resp.StatusCode)
	}
	if err := s.setPublicGate(ctx, false); err != nil {
		return err
	}
	return assertAnonymousStaticDenied(ctx, s.httpClient(), state.otherHost, state.otherMarker)
}

// deployerDashboardIsOperator identifies the intentional role overlap in the
// VPS acceptance identity setup. Dashboard authority comes from the durable
// account role, not from the deployer token used for API fixture setup.
func (s *Suite) deployerDashboardIsOperator() bool {
	return strings.EqualFold(s.Config.OperatorEmail, s.Config.DeployerEmail)
}

func assertAnonymousStaticDenied(ctx context.Context, h *http.Client, host string, forbidden ...string) error {
	for _, path := range []string{"/", "/asset.txt"} {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, appURL(host, path), nil)
		if err != nil {
			return err
		}
		resp, err := h.Do(req)
		if err != nil {
			return err
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if readErr != nil || resp.StatusCode != http.StatusUnauthorized {
			return fmt.Errorf("anonymous static denial %s status=%d", path, resp.StatusCode)
		}
		for _, value := range forbidden {
			if value != "" && bytes.Contains(body, []byte(value)) {
				return fmt.Errorf("anonymous static denial %s leaked app bytes", path)
			}
		}
	}
	return nil
}

func fetchPlatformPage(ctx context.Context, h *http.Client, raw string) (string, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return "", 0, err
	}
	resp, err := h.Do(req)
	if err != nil {
		return "", 0, err
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	resp.Body.Close()
	return string(body), resp.StatusCode, readErr
}

func dashboardAppCard(page, slug string) string {
	start := strings.Index(page, `data-tinker-app-slug="`+slug+`"`)
	if start < 0 {
		return ""
	}
	next := strings.Index(page[start+1:], `data-tinker-app-slug="`)
	if next < 0 {
		return page[start:]
	}
	return page[start : start+1+next]
}

type dashboardInsights struct {
	available     bool
	pageViews     int
	visitors      int
	lastActivity  string
	dailyRowCount int
}

// dashboardCardInsights reads only the stable, owner-scoped Last 7 days
// summary from one dashboard app card. It deliberately does not inspect the
// page outside that card: another app's aggregate must never satisfy this
// fixture's proof.
func dashboardCardInsights(card string) (dashboardInsights, bool) {
	if strings.Contains(card, "Local insights unavailable") {
		return dashboardInsights{}, true
	}
	match := dashboardLast7Insights.FindStringSubmatch(card)
	if len(match) != 3 {
		return dashboardInsights{}, false
	}
	visitors, visitorErr := strconv.Atoi(match[1])
	pageViews, viewsErr := strconv.Atoi(match[2])
	if visitorErr != nil || viewsErr != nil {
		return dashboardInsights{}, false
	}
	insights := dashboardInsights{available: true, pageViews: pageViews, visitors: visitors}
	if activity := dashboardLastActivity.FindStringSubmatch(card); len(activity) == 2 {
		insights.lastActivity = strings.TrimSpace(activity[1])
	}
	if series := dashboard30DaySeries.FindStringSubmatch(card); len(series) == 2 {
		insights.dailyRowCount = strings.Count(series[1], "<tr>")
	}
	return insights, true
}

func (s *Suite) waitDashboardInsights(ctx context.Context, h *http.Client, slug string, pageViews, visitors int) error {
	deadline := time.Now().Add(12 * time.Second)
	last := "dashboard request was not attempted"
	for {
		page, status, err := fetchPlatformPage(ctx, h, "https://"+s.Config.PlatformHost()+"/dashboard")
		matched, diagnostic := dashboardInsightsEvidence(page, status, err, slug, pageViews, visitors)
		if matched {
			return nil
		}
		last = diagnostic
		if time.Now().After(deadline) {
			return fmt.Errorf("owner insights did not reach exact page_views=%d approximate_visitors=%d: %s", pageViews, visitors, last)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// dashboardInsightsEvidence returns only bounded acceptance diagnostics. It
// distinguishes the server-rendered unavailable page from a successful
// dashboard that simply lacks the expected authorized app card.
func dashboardInsightsEvidence(page string, status int, err error, slug string, pageViews, visitors int) (bool, string) {
	switch {
	case err != nil:
		return false, "dashboard HTTP request failed"
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return false, fmt.Sprintf("dashboard authentication failed: status=%d", status)
	case status != http.StatusOK:
		return false, fmt.Sprintf("dashboard HTTP status=%d", status)
	case strings.Contains(page, "Your app overview is unavailable."):
		return false, "dashboard overview unavailable"
	}
	card := dashboardAppCard(page, slug)
	if card == "" {
		return false, "owned dashboard app card missing"
	}
	insights, ok := dashboardCardInsights(card)
	switch {
	case !ok:
		return false, "owned dashboard insights card malformed"
	case !insights.available:
		return false, "owned dashboard insights unavailable"
	case insights.lastActivity == "":
		return false, "owned dashboard last activity missing"
	case insights.lastActivity == "No counted document views yet.":
		return false, "owned dashboard last activity did not record a document view"
	case insights.dailyRowCount != 30:
		return false, fmt.Sprintf("owned dashboard daily series rows=%d want=30", insights.dailyRowCount)
	case insights.pageViews == pageViews && insights.visitors == visitors:
		return true, ""
	default:
		return false, fmt.Sprintf("owned dashboard observed page_views=%d approximate_visitors=%d", insights.pageViews, insights.visitors)
	}
}

func (s *Suite) dashboardBrowserForEmail(ctx context.Context, email string) (*http.Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	h := s.httpClient()
	hc := *h
	hc.Jar = jar
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if err := s.completeDashboardIdentityOTPForEmail(ctx, &hc, email); err != nil {
		return nil, err
	}
	return &hc, nil
}

// resetFixtureApps removes stale state only for fixture slugs that the
// authenticated deployer currently owns. It intentionally does not infer
// anything from absent names, inspect foreign apps, or attempt broad cleanup.
func resetFixtureApps(ctx context.Context, c client.Client) error {
	owned, err := c.ListApps(ctx)
	if err != nil {
		return fmt.Errorf("list owned fixture apps: %w", err)
	}
	present := make(map[string]struct{}, len(owned))
	for _, app := range owned {
		present[app.Slug] = struct{}{}
	}
	for _, slug := range vpsFixtureSlugs {
		if _, ok := present[slug]; !ok {
			continue
		}
		key, err := client.IdempotencyKey()
		if err != nil {
			return fmt.Errorf("create fixture deletion idempotency key: %w", err)
		}
		if err := c.DeleteApp(ctx, slug, key); err != nil {
			return fmt.Errorf("delete owned fixture app %q: %w", slug, err)
		}
	}
	return nil
}

// warmCertificate intentionally uses the gateway's pre-auth app route. This
// causes the normal autocert flow for a valid, inactive-release app hostname;
// it never exposes app bytes or bypasses deployment activation.
func (s *Suite) warmCertificate(ctx context.Context, host string) error {
	deadline := time.Now().Add(3 * time.Minute)
	for {
		r, e := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+host+"/_tinker/auth/login", nil)
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

func (s *Suite) applyReuseUpdate(ctx context.Context, remoteDir, probeSlug string) error {
	// The updater verifies the supplied artifacts against the installed binary's
	// compiled public key, stages a bounded rollback snapshot, restarts the
	// unprivileged service, and performs its own platform plus anonymous app
	// health checks. This is the only reuse upgrade path.
	if err := s.remote(ctx, "/usr/local/bin/tinkercloud", "verify-artifact",
		"--binary", remoteDir+"/tinkercloud-linux-amd64",
		"--metadata", remoteDir+"/tinkercloud-linux-amd64.metadata.json",
		"--signature", remoteDir+"/tinkercloud-linux-amd64.signature"); err != nil {
		return fmt.Errorf("reuse release is not trusted by installed server: %w", err)
	}
	newArgs := []string{"/usr/local/bin/tinkercloud", "update", "--config", "/etc/tinkercloud/config.yaml",
		"--binary", remoteDir + "/tinkercloud-linux-amd64",
		"--metadata", remoteDir + "/tinkercloud-linux-amd64.metadata.json",
		"--signature", remoteDir + "/tinkercloud-linux-amd64.signature",
		"--release-manifest", remoteDir + "/release-manifest.json",
		"--release-manifest-metadata", remoteDir + "/release-manifest.json.metadata.json",
		"--release-manifest-signature", remoteDir + "/release-manifest.json.signature",
		"--app-slug", probeSlug}
	if out, err := s.remoteRun(ctx, newArgs...); err != nil {
		// Only the precise old CLI flag rejection may use the legacy argv. Any
		// verification, health, transport, or other update failure remains final.
		if !legacyUpdateManifestFlag.Match(bytes.TrimSpace(out)) {
			return remoteError(err, out)
		}
		legacyArgs := []string{"/usr/local/bin/tinkercloud", "update", "--config", "/etc/tinkercloud/config.yaml",
			"--binary", remoteDir + "/tinkercloud-linux-amd64",
			"--metadata", remoteDir + "/tinkercloud-linux-amd64.metadata.json",
			"--signature", remoteDir + "/tinkercloud-linux-amd64.signature",
			"--app-slug", probeSlug}
		if legacyOut, legacyErr := s.remoteRun(ctx, legacyArgs...); legacyErr != nil {
			return remoteError(legacyErr, legacyOut)
		}
	}
	// Update itself restarts and health-gates its candidate. Confirm the
	// installed current binary's doctor/version check before creating the LLM
	// root; this prevents a compatibility fallback from masking a bad replace.
	if err := s.remote(ctx, "/usr/local/bin/tinkercloud", "doctor", "--config", "/etc/tinkercloud/config.yaml"); err != nil {
		return fmt.Errorf("doctor after reuse update: %w", err)
	}
	// Reuse exercises the explicit migration path for hosts initialized before
	// the LLM capability root existed. This remains a root-local operation:
	// the suite never reads the root or credentials, and the command never
	// prints them. A restart and normal doctor prove the updated service can use
	// the newly provisioned root without making a provider call.
	if err := s.remote(ctx, "/usr/local/bin/tinkercloud", "llm", "enable", "--config", "/etc/tinkercloud/config.yaml"); err != nil {
		return fmt.Errorf("enable LLM root after trusted reuse update: %w", err)
	}
	if err := s.remote(ctx, "systemctl", "restart", "tinkercloud.service"); err != nil {
		return fmt.Errorf("restart after LLM root enable: %w", err)
	}
	if err := s.remote(ctx, "/usr/local/bin/tinkercloud", "doctor", "--config", "/etc/tinkercloud/config.yaml"); err != nil {
		return fmt.Errorf("doctor after LLM root enable: %w", err)
	}
	return nil
}

func (s *Suite) crossAppBlobDenied(ctx context.Context, c client.Client, firstSlug, firstHost, blobID string, viewer *http.Client) (string, error) {
	slug := vpsIsolationAppSlug
	host, marker, err := s.deploySmokeApp(ctx, c, slug, true)
	if err != nil {
		return "", fmt.Errorf("deploy isolation app: %w", err)
	}
	// This is deliberately not another OTP flow. The browser already has an
	// admin-host global identity, so this app must receive only its own
	// derived, host-only app session after the one-time handoff.
	const preservedReturn = "/workspace/42?tab=open&tag=first&tag=second&encoded=a%2Bb"
	if err := s.viewerFlowWithExistingIdentityAt(ctx, viewer, host, marker, preservedReturn); err != nil {
		return "", fmt.Errorf("second allowed app did not reuse global identity: %w", err)
	}
	if err := assertDistinctAppCookies(viewer, firstHost, host); err != nil {
		return "", err
	}
	r, err := blobRequest(ctx, http.MethodGet, host, "/_tinker/api/v1/blobs/"+blobID, nil, "")
	if err != nil {
		return "", err
	}
	x, err := viewer.Do(r)
	if err != nil {
		return "", err
	}
	b, err := io.ReadAll(io.LimitReader(x.Body, int64(len(vpsBlobBytes)+1024)))
	x.Body.Close()
	if err != nil {
		return "", err
	}
	if x.StatusCode != http.StatusNotFound || bytes.Contains(b, vpsBlobBytes) {
		return "", fmt.Errorf("cross-app guessed blob ID leaked data from %s: status=%d", firstSlug, x.StatusCode)
	}
	return host, nil
}

// globalIdentityDeniedApp proves that a valid global identity remains subject
// to each app's current policy. The denied fixture already exists for catalog
// filtering evidence; this step intentionally never invokes readOTP before
// the account switch, so a rejection remains a friendly broker document rather
// than a second OTP or an app-byte leak.
func (s *Suite) globalIdentityDeniedApp(ctx context.Context, viewer *http.Client, host, marker, oldFirstHost, oldSecondHost string) error {
	handoff, err := s.beginAppHandoff(ctx, viewer, host)
	if err != nil {
		return err
	}
	platform := "https://" + s.Config.PlatformHost()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, platform+"/_tinker/identity?handoff="+url.QueryEscape(handoff), nil)
	if err != nil {
		return err
	}
	response, err := viewer.Do(request)
	if err != nil {
		return err
	}
	body, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	response.Body.Close()
	if readErr != nil {
		return readErr
	}
	if response.StatusCode != http.StatusOK || !bytes.Contains(body, []byte("This account cannot open this app.")) || bytes.Contains(body, []byte(marker)) || bytes.Contains(body, []byte("transaction")) {
		return fmt.Errorf("globally authenticated denied app leaked or did not render generic denial: status=%d", response.StatusCode)
	}
	// A switch is a same-origin POST. Its successful OTP verification revokes
	// every child session before the broker issues the replacement identity.
	form := url.Values{"handoff": {handoff}}
	request, err = http.NewRequestWithContext(ctx, http.MethodPost, platform+"/_tinker/identity/use-another", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Origin", platform)
	response, err = viewer.Do(request)
	if err != nil {
		return err
	}
	body, readErr = io.ReadAll(io.LimitReader(response.Body, 1<<20))
	response.Body.Close()
	if readErr != nil || response.StatusCode != http.StatusOK || !bytes.Contains(body, []byte("Sign in to continue.")) {
		return errors.New("account switch form unavailable")
	}
	form = url.Values{"email": {s.Config.DeployerEmail}, "handoff": {handoff}}
	request, err = http.NewRequestWithContext(ctx, http.MethodPost, platform+"/_tinker/identity/otp", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Origin", platform)
	response, err = viewer.Do(request)
	if err != nil {
		return err
	}
	body, readErr = io.ReadAll(io.LimitReader(response.Body, 1<<20))
	response.Body.Close()
	tx := hiddenValue(string(body), "transaction")
	if readErr != nil || response.StatusCode != http.StatusOK || tx == "" {
		return errors.New("switch OTP transaction missing")
	}
	code, err := readOTP(ctx, s.Config, "viewer", s.Config.DeployerEmail, s.Config.PlatformHost())
	if err != nil {
		return err
	}
	form = url.Values{"email": {s.Config.DeployerEmail}, "handoff": {handoff}, "transaction": {tx}, "code": {code}}
	request, err = http.NewRequestWithContext(ctx, http.MethodPost, platform+"/_tinker/identity/verify", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Origin", platform)
	response, err = viewer.Do(request)
	if err != nil {
		return err
	}
	callback := response.Header.Get("Location")
	response.Body.Close()
	if response.StatusCode != http.StatusSeeOther || !isExactHandoffCallback(callback, host, handoff) {
		return errors.New("account switch did not return exact callback")
	}
	for _, oldHost := range []string{oldFirstHost, oldSecondHost} {
		if err := s.assertAppDenied(ctx, viewer, oldHost, ""); err != nil {
			return fmt.Errorf("old child session remained usable after switch: %w", err)
		}
	}
	if err := s.finishAppHandoffForEmail(ctx, viewer, host, marker, callback, s.Config.DeployerEmail); err != nil {
		return err
	}
	if err := s.assertCookieScopes(viewer, host, ""); err != nil {
		return err
	}
	return nil
}

// dashboardGlobalLogout proves the one visible Tinkercloud sign-out action is
// global: it destroys the admin identity and every independently scoped app
// child session, rather than merely clearing the dashboard cookie.
func (s *Suite) dashboardGlobalLogout(ctx context.Context, h *http.Client, hosts ...string) error {
	platform := "https://" + s.Config.PlatformHost()
	page, err := http.NewRequestWithContext(ctx, http.MethodGet, platform+"/", nil)
	if err != nil {
		return err
	}
	response, err := h.Do(page)
	if err != nil {
		return err
	}
	body, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	response.Body.Close()
	if readErr != nil || response.StatusCode != http.StatusOK {
		return errors.New("dashboard was unavailable before global logout")
	}
	csrf := hiddenValue(string(body), "csrf")
	if csrf == "" {
		return errors.New("dashboard global logout CSRF token missing")
	}
	form := url.Values{"csrf": {csrf}}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, platform+"/logout", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Origin", platform)
	response, err = h.Do(request)
	if err != nil {
		return err
	}
	response.Body.Close()
	if response.StatusCode != http.StatusSeeOther || response.Header.Get("Location") != "/login" {
		return fmt.Errorf("dashboard global logout did not redirect to sign-in: status=%d", response.StatusCode)
	}
	admin, _ := url.Parse(platform + "/")
	if hasCookie(h.Jar.Cookies(admin), "__Host-tinker_identity") {
		return errors.New("dashboard global logout retained the global identity cookie")
	}
	for _, host := range hosts {
		if err := s.assertAppDenied(ctx, h, host, ""); err != nil {
			return fmt.Errorf("global logout left an app session usable: %w", err)
		}
	}
	return nil
}

func (s *Suite) replayedAndWrongAppHandoffsDeny(ctx context.Context, h *http.Client, firstHost, secondHost, consumedCallback, consumedState, marker string) error {
	if consumedState == "" {
		return errors.New("consumed handoff state was not retained for replay evidence")
	}
	firstToken, _ := appCookie(h, firstHost)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, consumedCallback, nil)
	if err != nil {
		return err
	}
	request.AddCookie(&http.Cookie{Name: "__Host-tinker_identity_state", Value: consumedState})
	response, err := h.Do(request)
	if err != nil {
		return err
	}
	body, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	response.Body.Close()
	if response.StatusCode == http.StatusSeeOther || bytes.Contains(body, []byte(marker)) {
		return errors.New("consumed handoff replay issued access")
	}
	if after, _ := appCookie(h, firstHost); after != firstToken {
		return errors.New("consumed handoff replay replaced app session")
	}
	wrong, err := url.Parse(consumedCallback)
	if err != nil {
		return err
	}
	wrong.Host = secondHost
	secondToken, _ := appCookie(h, secondHost)
	request, err = http.NewRequestWithContext(ctx, http.MethodGet, wrong.String(), nil)
	if err != nil {
		return err
	}
	// Supplying the original state on the sibling host proves exact app binding,
	// rather than merely exercising the missing-state branch.
	request.AddCookie(&http.Cookie{Name: "__Host-tinker_identity_state", Value: consumedState})
	response, err = h.Do(request)
	if err != nil {
		return err
	}
	body, _ = io.ReadAll(io.LimitReader(response.Body, 1<<20))
	response.Body.Close()
	if response.StatusCode == http.StatusSeeOther || bytes.Contains(body, []byte(marker)) {
		return errors.New("wrong-app callback issued access")
	}
	if after, _ := appCookie(h, secondHost); after != secondToken {
		return errors.New("wrong-app callback replaced app session")
	}
	return nil
}

func (s *Suite) appLocalLogoutAndBrokerReopen(ctx context.Context, h *http.Client, firstHost, secondHost, marker string) error {
	form := url.Values{"return": {"/"}}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, appURL(firstHost, "/_tinker/auth/logout"), strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Origin", "https://"+firstHost)
	response, err := h.Do(request)
	if err != nil {
		return err
	}
	response.Body.Close()
	if response.StatusCode != http.StatusSeeOther {
		return errors.New("app-local logout did not redirect")
	}
	if err := s.assertAppDenied(ctx, h, firstHost, marker); err != nil {
		return err
	}
	if err := s.assertServerDerivedViewer(ctx, h, secondHost); err != nil {
		return fmt.Errorf("app-local logout affected second app: %w", err)
	}
	if err := s.viewerFlowWithExistingIdentity(ctx, h, firstHost, marker); err != nil {
		return fmt.Errorf("app-local logout did not retain global identity: %w", err)
	}
	return nil
}

func (s *Suite) assertAppDenied(ctx context.Context, h *http.Client, host, marker string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, appURL(host, "/_tinker/api/v1/me"), nil)
	if err != nil {
		return err
	}
	response, err := h.Do(request)
	if err != nil {
		return err
	}
	body, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized || (marker != "" && bytes.Contains(body, []byte(marker))) {
		return fmt.Errorf("expected app denial, got status=%d", response.StatusCode)
	}
	return nil
}

// assertExactAppDocument proves an already-issued, host-only app session can
// access only the active release after the current policy permits it again.
// Callers must separately prove that the same session was denied while the
// policy was revoked; this helper never creates or consumes a handoff.
func (s *Suite) assertExactAppDocument(ctx context.Context, h *http.Client, host string, expected []byte) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, appURL(host, "/"), nil)
	if err != nil {
		return err
	}
	response, err := h.Do(request)
	if err != nil {
		return err
	}
	body, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	response.Body.Close()
	if readErr != nil {
		return readErr
	}
	if response.StatusCode != http.StatusOK || !bytes.Equal(body, expected) {
		return fmt.Errorf("expected exact restored app document, got status=%d", response.StatusCode)
	}
	return nil
}

func appCookie(h *http.Client, host string) (string, bool) {
	if h == nil || h.Jar == nil {
		return "", false
	}
	u, _ := url.Parse("https://" + host + "/")
	return cookieValue(h.Jar.Cookies(u), "__Host-tinker_app")
}

// identityStateCookie is used only to retain a handoff's original app-host
// state across its first successful consumption. The caller never logs,
// serializes, or reports the value.
func identityStateCookie(h *http.Client, host string) (string, bool) {
	if h == nil || h.Jar == nil {
		return "", false
	}
	u, _ := url.Parse("https://" + host + "/")
	return cookieValue(h.Jar.Cookies(u), "__Host-tinker_identity_state")
}

func (s *Suite) deploySmokeApp(ctx context.Context, c client.Client, slug string, blobs bool) (string, string, error) {
	return s.deploySmokeAppForViewer(ctx, c, slug, s.Config.ViewerEmail, blobs)
}

// deploySmokeAppWithRedeploy is the one black-box regression that sends the
// exact same immutable archive to an already active app. It proves same-hash
// deployment supersedes the old release without broadening policy.
func (s *Suite) deploySmokeAppWithRedeploy(ctx context.Context, c client.Client, slug string, blobs bool) (string, string, error) {
	return s.deploySmokeAppForViewerWithRedeploy(ctx, c, slug, s.Config.ViewerEmail, blobs, true)
}

func (s *Suite) deploySmokeAppForViewer(ctx context.Context, c client.Client, slug, viewerEmail string, blobs bool) (string, string, error) {
	return s.deploySmokeAppForViewerWithRedeploy(ctx, c, slug, viewerEmail, blobs, false)
}

func (s *Suite) deploySmokeAppForViewerWithRedeploy(ctx context.Context, c client.Client, slug, viewerEmail string, blobs, redeploy bool) (string, string, error) {
	key, _ := client.IdempotencyKey()
	if err := c.Do(ctx, http.MethodPost, "/api/v1/apps", key, map[string]string{"slug": slug}, nil); err != nil {
		return "", "", fmt.Errorf("create app: %w", err)
	}
	policy := map[string]any{"mode": "private", "expected_revision": 1, "confirm_broadening": true, "allow": map[string]any{"emails": []string{viewerEmail}, "domains": []string{}}}
	key, _ = client.IdempotencyKey()
	if err := c.Do(ctx, http.MethodPut, "/api/v1/apps/"+slug+"/access", key, policy, nil); err != nil {
		return "", "", fmt.Errorf("set access policy: %w", err)
	}
	host := slug + "." + s.Config.AppSuffix()
	if err := s.warmCertificate(ctx, host); err != nil {
		return "", "", err
	}
	archive, size, marker, err := smokeArchiveWithBlobs(slug, viewerEmail, blobs)
	if err != nil {
		return "", "", err
	}
	key, _ = client.IdempotencyKey()
	deployed, err := c.Deploy(ctx, slug, bytes.NewReader(archive), size, key)
	if err != nil {
		return "", "", fmt.Errorf("deploy app: %w", err)
	}
	if err = deployed.Verified(); err != nil {
		return "", "", err
	}
	if redeploy {
		key, _ = client.IdempotencyKey()
		second, err := c.Deploy(ctx, slug, bytes.NewReader(archive), size, key)
		if err != nil {
			return "", "", fmt.Errorf("redeploy same archive: %w", err)
		}
		if err = second.Verified(); err != nil || second.DeploymentID == deployed.DeploymentID {
			return "", "", fmt.Errorf("same-archive redeploy receipt: %w", err)
		}
		releases, err := c.ListReleases(ctx, slug)
		if err != nil {
			return "", "", fmt.Errorf("list redeployed releases: %w", err)
		}
		var active, superseded string
		for _, release := range releases {
			if release.State == "active" {
				active = release.ID
			}
			if release.ID == deployed.DeploymentID && release.State == "superseded" {
				superseded = release.ID
			}
		}
		if active != second.DeploymentID || superseded != deployed.DeploymentID {
			return "", "", errors.New("same-archive redeploy did not supersede the prior release")
		}
	}
	return host, marker, nil
}

func smokeArchive(slug, viewerEmail string) ([]byte, int64, string, error) {
	return smokeArchiveWithBlobs(slug, viewerEmail, true)
}

func smokeArchiveWithBlobs(slug, viewerEmail string, blobs bool) ([]byte, int64, string, error) {
	d, e := os.MkdirTemp("", "tinkercloud-vps-app-")
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
	marker := "TINKERCLOUD_VPS_MARKER_" + id
	if e = os.WriteFile(filepath.Join(d, "dist", "index.html"), []byte("<!doctype html><title>tinker</title>"+marker), 0644); e != nil {
		return nil, 0, "", e
	}
	if e = os.WriteFile(filepath.Join(d, "dist", "private.js"), []byte("window.privateMarker='"+marker+"'"), 0644); e != nil {
		return nil, 0, "", e
	}
	// This tinker browser fixture is intentionally SDK-equivalent: it discovers
	// capability state, then uses same-origin multipart upload/list/download/
	// delete requests with no app selector or credential. The black-box suite
	// below performs the exact operations after real viewer OTP authentication.
	fixture := `const api = "/_tinker/api/v1";
const request = (path, init = {}) => fetch(api + path, { credentials: "same-origin", ...init });
export async function blobSmoke(file) {
  const capabilities = await request("/capabilities").then(r => r.json());
  if (!capabilities.capabilities.some(c => c.name === "blobs")) throw new Error("blobs unavailable");
  const form = new FormData(); form.append("file", file, file.name);
  const uploaded = await request("/blobs", { method: "POST", body: form }).then(r => r.json());
  const listed = await request("/blobs").then(r => r.json());
  const bytes = await request("/blobs/" + encodeURIComponent(uploaded.id)).then(r => r.blob());
  await request("/blobs/" + encodeURIComponent(uploaded.id), { method: "DELETE" });
  return { capabilities, uploaded, listed, bytes };
}`
	if blobs {
		if e = os.WriteFile(filepath.Join(d, "dist", "blob-smoke.js"), []byte(fixture), 0644); e != nil {
			return nil, 0, "", e
		}
	}
	// The black-box fixture deliberately opts into the data primitives that it
	// probes. The browser never selects a database, app ID, or storage path.
	features := "features:\n  kv: true\n  realtime: true\n"
	if blobs {
		features += "  blobs: true\n"
	}
	// The acceptance flow deliberately returns to a nested document route after
	// the second app handoff. Declare the opt-in SPA fallback so this route
	// proves return-path preservation instead of testing a missing static file.
	manifest := []byte("version: 1\nname: " + slug + "\nbuild:\n  output: dist\nspa:\n  fallback: index.html\n" + features + "access:\n  mode: private\n  allow:\n    emails:\n      - " + viewerEmail + "\n    domains: []\n")
	if e = os.WriteFile(filepath.Join(d, "tinker.yaml"), manifest, 0644); e != nil {
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
	for _, path := range []string{"/", "/private.js", "/_tinker/api/v1/me", "/_tinker/ws/v1"} {
		r, e := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+host+path, nil)
		if e != nil {
			return e
		}
		if path == "/_tinker/ws/v1" {
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

// firstViewerFlow is the only point where the initial access traversal reads a
// viewer OTP. It creates a global identity at admin.<domain>, then obtains an
// app-scoped session for the first allowed app; opening the second app reads no
// OTP.
func (s *Suite) firstViewerFlow(ctx context.Context, host, marker string) (*http.Client, string, string, string, error) {
	jar, e := cookiejar.New(nil)
	if e != nil {
		return nil, "", "", "", e
	}
	h := s.httpClient()
	hc := *h
	hc.Jar = jar
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if e = s.completeDashboardIdentityOTP(ctx, &hc); e != nil {
		return nil, "", "", "", e
	}
	handoff, e := s.beginAppHandoff(ctx, &hc, host)
	if e != nil {
		return nil, "", "", "", e
	}
	platformRequest, e := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+s.Config.PlatformHost()+"/_tinker/identity?handoff="+url.QueryEscape(handoff), nil)
	if e != nil {
		return nil, "", "", "", e
	}
	response, e := hc.Do(platformRequest)
	if e != nil {
		return nil, "", "", "", e
	}
	callback := response.Header.Get("Location")
	response.Body.Close()
	if response.StatusCode != http.StatusSeeOther || !isExactHandoffCallback(callback, host, handoff) {
		return nil, "", "", "", fmt.Errorf("dashboard identity did not continue to app callback: status=%d", response.StatusCode)
	}
	state, ok := identityStateCookie(&hc, host)
	if !ok {
		return nil, "", "", "", errors.New("app handoff state cookie missing before consumption")
	}
	if e = s.finishAppHandoff(ctx, &hc, host, marker, callback); e != nil {
		return nil, "", "", "", e
	}
	if e = s.assertCookieScopes(&hc, host, ""); e != nil {
		return nil, "", "", "", e
	}
	if err := expectBlobCapability(ctx, &hc, host); err != nil {
		return nil, "", "", "", err
	}
	if err := rejectExtraBlobPartWithoutMutation(ctx, &hc, host); err != nil {
		return nil, "", "", "", err
	}
	blobID, err := uploadBlob(ctx, &hc, host, []byte("tinkercloud-vps-blob-exact-bytes\x00\xff"))
	if err != nil {
		return nil, "", "", "", err
	}
	return &hc, blobID, callback, state, nil
}

// viewerFlowWithExistingIdentity must never read or request another OTP. A
// valid platform identity is authorized only for this newly resolved app and
// exchanged for that host's own app cookie.
func (s *Suite) viewerFlowWithExistingIdentity(ctx context.Context, h *http.Client, host, marker string) error {
	return s.viewerFlowWithExistingIdentityForEmail(ctx, h, host, marker, s.Config.ViewerEmail)
}

// viewerFlowWithExistingIdentityForEmail is the explicit variant for a
// browser whose already-verified global identity is not the fixture viewer.
// The email is only the expected server-derived /me result; it is never sent
// to the gateway as part of the app handoff.
func (s *Suite) viewerFlowWithExistingIdentityForEmail(ctx context.Context, h *http.Client, host, marker, email string) error {
	return s.viewerFlowWithExistingIdentityForEmailAt(ctx, h, host, marker, email, "/")
}

// viewerFlowWithExistingIdentityAt proves an app path and its ordered raw
// query survive the app-to-admin handoff. The viewer identity remains entirely
// server-derived; this is only the safe relative return destination.
func (s *Suite) viewerFlowWithExistingIdentityAt(ctx context.Context, h *http.Client, host, marker, returnPath string) error {
	return s.viewerFlowWithExistingIdentityForEmailAt(ctx, h, host, marker, s.Config.ViewerEmail, returnPath)
}

// viewerFlowWithExistingIdentityForEmailAt proves the same handoff for an
// explicitly expected, already-verified identity. It keeps identity
// attribution server-derived while preventing the acceptance helper from
// assuming every browser belongs to Config.ViewerEmail.
func (s *Suite) viewerFlowWithExistingIdentityForEmailAt(ctx context.Context, h *http.Client, host, marker, email, returnPath string) error {
	handoff, err := s.beginAppHandoffAt(ctx, h, host, returnPath)
	if err != nil {
		return err
	}
	platformRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+s.Config.PlatformHost()+"/_tinker/identity?handoff="+url.QueryEscape(handoff), nil)
	if err != nil {
		return err
	}
	response, err := h.Do(platformRequest)
	if err != nil {
		return err
	}
	body, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	response.Body.Close()
	if readErr != nil {
		return readErr
	}
	if response.StatusCode != http.StatusSeeOther {
		return fmt.Errorf("existing identity handoff %s: status=%d", identityBrokerOutcome(response.StatusCode, body), response.StatusCode)
	}
	if !isExactHandoffCallback(response.Header.Get("Location"), host, handoff) {
		return errors.New("existing identity redirected to an unsafe callback")
	}
	if err := s.finishAppHandoffForEmailAt(ctx, h, host, marker, response.Header.Get("Location"), email, returnPath); err != nil {
		return err
	}
	return s.assertCookieScopes(h, host, "")
}

// identityBrokerOutcome intentionally classifies only stable, non-sensitive
// broker contracts. A 200 page is not automatically an OTP request: an
// authenticated but policy-denied identity gets a distinct generic page and
// must never be misreported as an email-delivery failure.
func identityBrokerOutcome(status int, body []byte) string {
	if status != http.StatusOK {
		return "returned an unexpected broker response"
	}
	switch {
	case bytes.Contains(body, []byte("This account cannot open this app.")):
		return "was denied by the app policy"
	case bytes.Contains(body, []byte(`action="/_tinker/identity/otp"`)) && bytes.Contains(body, []byte(`name="email"`)):
		return "requires OTP"
	default:
		return "returned an unrecognized broker page"
	}
}

// beginAppHandoff exercises the document-only gateway path. It returns the
// opaque server-created handoff id; the app ID and callback origin never come
// from this browser-facing protocol.
func (s *Suite) beginAppHandoff(ctx context.Context, h *http.Client, host string) (string, error) {
	return s.beginAppHandoffAt(ctx, h, host, "/")
}

func (s *Suite) beginAppHandoffAt(ctx context.Context, h *http.Client, host, returnPath string) (string, error) {
	if !strings.HasPrefix(returnPath, "/") || strings.HasPrefix(returnPath, "//") {
		return "", errors.New("unsafe app return path")
	}
	document, err := http.NewRequestWithContext(ctx, http.MethodGet, appURL(host, returnPath), nil)
	if err != nil {
		return "", err
	}
	document.Header.Set("Accept", "text/html")
	document.Header.Set("Sec-Fetch-Dest", "document")
	response, err := h.Do(document)
	if err != nil {
		return "", err
	}
	response.Body.Close()
	if response.StatusCode != http.StatusSeeOther || !strings.HasPrefix(response.Header.Get("Location"), "/_tinker/auth/login") {
		return "", fmt.Errorf("anonymous document did not enter app login: status=%d location=%q", response.StatusCode, response.Header.Get("Location"))
	}
	login, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+host+response.Header.Get("Location"), nil)
	if err != nil {
		return "", err
	}
	response, err = h.Do(login)
	if err != nil {
		return "", err
	}
	response.Body.Close()
	if response.StatusCode != http.StatusSeeOther {
		return "", fmt.Errorf("app login did not create broker handoff: status=%d", response.StatusCode)
	}
	location := response.Header.Get("Location")
	parsed, err := url.Parse(location)
	if err != nil || parsed.Scheme != "https" || !strings.EqualFold(parsed.Host, s.Config.PlatformHost()) || parsed.Path != "/_tinker/identity" {
		return "", fmt.Errorf("app login returned unsafe platform handoff location %q", location)
	}
	handoff := parsed.Query().Get("handoff")
	if handoff == "" || len(parsed.Query()) != 1 {
		return "", errors.New("app login did not return exactly one opaque handoff")
	}
	return handoff, nil
}

func (s *Suite) completeDashboardIdentityOTP(ctx context.Context, h *http.Client) error {
	return s.completeDashboardIdentityOTPForEmail(ctx, h, s.Config.ViewerEmail)
}

func (s *Suite) completeDashboardIdentityOTPForEmail(ctx context.Context, h *http.Client, email string) error {
	platform := "https://" + s.Config.PlatformHost()
	page, err := http.NewRequestWithContext(ctx, http.MethodGet, platform+"/login", nil)
	if err != nil {
		return err
	}
	response, err := h.Do(page)
	if err != nil {
		return err
	}
	body, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	response.Body.Close()
	if readErr != nil {
		return readErr
	}
	// Assert the stable form contract rather than product copy. The dashboard
	// and app broker deliberately use different headings, but both remain
	// generic and must not expose role or policy eligibility.
	if response.StatusCode != http.StatusOK ||
		!bytes.Contains(body, []byte(`action="/login"`)) ||
		!bytes.Contains(body, []byte(`name="email"`)) {
		return fmt.Errorf("dashboard login page missing generic sign-in form: status=%d", response.StatusCode)
	}
	form := url.Values{"email": {email}}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, platform+"/login", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// Platform identity mutations enforce an exact platform Origin. This is
	// intentionally different from an app-origin form: the broker owns the
	// global cookie and must never accept an app host as its mutation origin.
	request.Header.Set("Origin", platform)
	response, err = h.Do(request)
	if err != nil {
		return err
	}
	body, readErr = io.ReadAll(io.LimitReader(response.Body, 1<<20))
	response.Body.Close()
	if readErr != nil {
		return readErr
	}
	tx := hiddenValue(string(body), "transaction")
	if response.StatusCode != http.StatusOK || tx == "" {
		return errors.New("dashboard identity OTP transaction missing")
	}
	code, err := readOTP(ctx, s.Config, "viewer", email, s.Config.PlatformHost())
	if err != nil {
		return err
	}
	form = url.Values{"email": {email}, "transaction": {tx}, "code": {code}}
	request, err = http.NewRequestWithContext(ctx, http.MethodPost, platform+"/login/verify", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Origin", platform)
	response, err = h.Do(request)
	if err != nil {
		return err
	}
	response.Body.Close()
	if response.StatusCode != http.StatusSeeOther || response.Header.Get("Location") != "/dashboard" {
		return fmt.Errorf("dashboard identity OTP did not complete: status=%d location=%q", response.StatusCode, response.Header.Get("Location"))
	}
	if h.Jar == nil {
		return errors.New("dashboard browser did not retain cookies")
	}
	admin, _ := url.Parse(platform + "/")
	if !hasCookie(h.Jar.Cookies(admin), "__Host-tinker_identity") || !hasCookie(h.Jar.Cookies(admin), "__Host-tinker_browser") {
		return errors.New("dashboard login did not issue scoped global identity")
	}
	return nil
}

func (s *Suite) finishAppHandoff(ctx context.Context, h *http.Client, host, marker, callbackLocation string) error {
	return s.finishAppHandoffAt(ctx, h, host, marker, callbackLocation, "/")
}

func (s *Suite) finishAppHandoffAt(ctx context.Context, h *http.Client, host, marker, callbackLocation, returnPath string) error {
	return s.finishAppHandoffForEmailAt(ctx, h, host, marker, callbackLocation, s.Config.ViewerEmail, returnPath)
}

func (s *Suite) finishAppHandoffForEmail(ctx context.Context, h *http.Client, host, marker, callbackLocation, email string) error {
	return s.finishAppHandoffForEmailAt(ctx, h, host, marker, callbackLocation, email, "/")
}

func (s *Suite) finishAppHandoffForEmailAt(ctx context.Context, h *http.Client, host, marker, callbackLocation, email, returnPath string) error {
	if !isExactHandoffCallback(callbackLocation, host, "") {
		return fmt.Errorf("broker returned unsafe app callback %q", callbackLocation)
	}
	callback, err := http.NewRequestWithContext(ctx, http.MethodGet, callbackLocation, nil)
	if err != nil {
		return err
	}
	response, err := h.Do(callback)
	if err != nil {
		return err
	}
	response.Body.Close()
	if response.StatusCode != http.StatusSeeOther || response.Header.Get("Location") != returnPath {
		return fmt.Errorf("app callback did not issue app session: status=%d location=%q", response.StatusCode, response.Header.Get("Location"))
	}
	page, err := http.NewRequestWithContext(ctx, http.MethodGet, appURL(host, returnPath), nil)
	if err != nil {
		return err
	}
	response, err = h.Do(page)
	if err != nil {
		return err
	}
	body, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	response.Body.Close()
	if readErr != nil {
		return readErr
	}
	if response.StatusCode != http.StatusOK || !bytes.Contains(body, []byte(marker)) {
		return fmt.Errorf("viewer app access failed: status=%d", response.StatusCode)
	}
	return s.assertServerDerivedEmail(ctx, h, host, email)
}

func (s *Suite) assertServerDerivedViewer(ctx context.Context, h *http.Client, host string) error {
	return s.assertServerDerivedEmail(ctx, h, host, s.Config.ViewerEmail)
}

func (s *Suite) assertServerDerivedEmail(ctx context.Context, h *http.Client, host, email string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, appURL(host, "/_tinker/api/v1/me"), nil)
	if err != nil {
		return err
	}
	response, err := h.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	var me struct {
		Identity struct {
			Email string `json:"email"`
		} `json:"identity"`
	}
	if response.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&me) != nil || !strings.EqualFold(me.Identity.Email, email) {
		return errors.New("current viewer identity was not server-derived")
	}
	return nil
}

func isAppHandoffCallback(raw, handoff string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Path == "/_tinker/auth/callback" && u.Query().Get("handoff") == handoff && len(u.Query()) == 1
}

func isExactHandoffCallback(raw, host, handoff string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || !strings.EqualFold(u.Host, host) || u.Path != "/_tinker/auth/callback" || u.User != nil {
		return false
	}
	got := u.Query().Get("handoff")
	return got != "" && (handoff == "" || got == handoff) && len(u.Query()) == 1
}

func (s *Suite) assertCookieScopes(h *http.Client, appHost, _ string) error {
	if h == nil || h.Jar == nil {
		return errors.New("viewer browser does not retain cookies")
	}
	platform, _ := url.Parse("https://" + s.Config.PlatformHost() + "/")
	app, _ := url.Parse("https://" + appHost + "/")
	if !hasCookie(h.Jar.Cookies(platform), "__Host-tinker_identity") || !hasCookie(h.Jar.Cookies(platform), "__Host-tinker_browser") || hasCookie(h.Jar.Cookies(platform), "__Host-tinker_app") {
		return errors.New("platform global identity or browser-binding cookie scope is unsafe")
	}
	if !hasCookie(h.Jar.Cookies(app), "__Host-tinker_app") || hasCookie(h.Jar.Cookies(app), "__Host-tinker_identity") || hasCookie(h.Jar.Cookies(app), "__Host-tinker_browser") {
		return errors.New("app viewer cookie scope is unsafe")
	}
	return nil
}

func assertDistinctAppCookies(h *http.Client, firstHost, secondHost string) error {
	if h == nil || h.Jar == nil {
		return errors.New("viewer browser does not retain cookies")
	}
	first, _ := url.Parse("https://" + firstHost + "/")
	second, _ := url.Parse("https://" + secondHost + "/")
	firstValue, firstOK := cookieValue(h.Jar.Cookies(first), "__Host-tinker_app")
	secondValue, secondOK := cookieValue(h.Jar.Cookies(second), "__Host-tinker_app")
	if !firstOK || !secondOK || firstValue == secondValue {
		return errors.New("app sessions were not independently host scoped")
	}
	return nil
}

func hasCookie(cookies []*http.Cookie, name string) bool {
	_, ok := cookieValue(cookies, name)
	return ok
}

func cookieValue(cookies []*http.Cookie, name string) (string, bool) {
	for _, cookie := range cookies {
		if cookie.Name == name && cookie.Value != "" {
			return cookie.Value, true
		}
	}
	return "", false
}

func appURL(host, path string) string { return "https://" + host + path }

func expectBlobCapability(ctx context.Context, h *http.Client, host string) error {
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, appURL(host, "/_tinker/api/v1/capabilities"), nil)
	if err != nil {
		return err
	}
	x, err := h.Do(r)
	if err != nil {
		return err
	}
	defer x.Body.Close()
	var out struct {
		Capabilities []struct {
			Name string `json:"name"`
		} `json:"capabilities"`
	}
	if x.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(x.Body, 1<<20)).Decode(&out) != nil {
		return errors.New("blob capability discovery failed")
	}
	for _, c := range out.Capabilities {
		if c.Name == "blobs" {
			return nil
		}
	}
	return errors.New("blob capability is absent from discovery")
}

// Collection acceptance uses only the public, same-origin app API. In
// particular, the caller cannot select an app, SQLite file, or namespace: the
// gateway derives all three from the verified app hostname and viewer session.
func expectCollectionCapability(ctx context.Context, h *http.Client, host string) error {
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, appURL(host, "/_tinker/api/v1/capabilities"), nil)
	if err != nil {
		return err
	}
	assertNoAppSelector(r, host)
	x, err := h.Do(r)
	if err != nil {
		return err
	}
	defer x.Body.Close()
	var out struct {
		Capabilities []struct {
			Name string `json:"name"`
		} `json:"capabilities"`
	}
	if x.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(x.Body, 1<<20)).Decode(&out) != nil {
		return errors.New("collection capability discovery failed")
	}
	gotDB := false
	for _, c := range out.Capabilities {
		if c.Name == "db" {
			gotDB = true
		}
		if c.Name == "llm.chat" {
			return errors.New("ungranted llm.chat appeared in capability discovery")
		}
	}
	if !gotDB {
		return errors.New("collection capability is absent from discovery")
	}
	// The fixture never requests a chat grant. A valid viewer must receive the
	// protected capability-unavailable response, not the anonymous gateway
	// denial, and no provider detail may appear.
	if err := assertLLMChatDenied(ctx, h, host, http.StatusForbidden); err != nil {
		return err
	}
	return nil
}

func (s *Suite) anonymousLLMChatDenied(ctx context.Context, host string) error {
	return assertLLMChatDenied(ctx, s.httpClient(), host, http.StatusUnauthorized)
}

func assertLLMChatDenied(ctx context.Context, h *http.Client, host string, wantStatus int) error {
	r, err := collectionRequest(ctx, http.MethodPost, host, "/_tinker/api/v1/llm/chat", strings.NewReader(`{"messages":[{"role":"user","content":"no grant"}]}`))
	if err != nil {
		return err
	}
	x, err := h.Do(r)
	if err != nil {
		return err
	}
	body, readErr := io.ReadAll(io.LimitReader(x.Body, 1<<20))
	x.Body.Close()
	if readErr != nil {
		return readErr
	}
	if x.StatusCode != wantStatus || bytes.Contains(bytes.ToLower(body), []byte("provider")) {
		return fmt.Errorf("LLM invocation was not safely denied: status=%d", x.StatusCode)
	}
	return nil
}

type vpsCollectionDocument struct {
	ID      string          `json:"id"`
	Data    json.RawMessage `json:"data"`
	Version uint64          `json:"version"`
}

func collectionRequest(ctx context.Context, method, host, path string, body io.Reader) (*http.Request, error) {
	r, err := http.NewRequestWithContext(ctx, method, appURL(host, path), body)
	if err != nil {
		return nil, err
	}
	if method != http.MethodGet {
		r.Header.Set("Origin", "https://"+host)
		r.Header.Set("Content-Type", "application/json")
	}
	assertNoAppSelector(r, host)
	return r, nil
}

func assertNoAppSelector(r *http.Request, host string) {
	if !strings.EqualFold(r.URL.Host, host) || r.URL.Query().Get("app") != "" || r.URL.Query().Get("app_id") != "" || r.URL.Query().Get("database") != "" {
		panic("VPS collection fixture must not select app or database")
	}
}

func (s *Suite) exerciseCollections(ctx context.Context, h *http.Client, host string) (string, error) {
	if err := expectCollectionCapability(ctx, h, host); err != nil {
		return "", err
	}
	r, err := collectionRequest(ctx, http.MethodPost, host, "/_tinker/api/v1/db/tasks", strings.NewReader(`{"data":{"title":"persist across restart","done":false}}`))
	if err != nil {
		return "", err
	}
	x, err := h.Do(r)
	if err != nil {
		return "", err
	}
	var created vpsCollectionDocument
	decodeErr := json.NewDecoder(io.LimitReader(x.Body, 1<<20)).Decode(&created)
	x.Body.Close()
	if x.StatusCode != http.StatusCreated || decodeErr != nil || created.ID == "" || created.Version != 1 || !bytes.Contains(created.Data, []byte("persist across restart")) {
		return "", errors.New("collection create did not return an authoritative document")
	}
	r, err = collectionRequest(ctx, http.MethodGet, host, "/_tinker/api/v1/db/tasks/"+url.PathEscape(created.ID), nil)
	if err != nil {
		return "", err
	}
	x, err = h.Do(r)
	if err != nil {
		return "", err
	}
	var fetched vpsCollectionDocument
	decodeErr = json.NewDecoder(io.LimitReader(x.Body, 1<<20)).Decode(&fetched)
	x.Body.Close()
	if x.StatusCode != http.StatusOK || decodeErr != nil || fetched.ID != created.ID || fetched.Version != 1 {
		return "", errors.New("collection get did not return the created document")
	}
	r, err = collectionRequest(ctx, http.MethodPut, host, "/_tinker/api/v1/db/tasks/"+url.PathEscape(created.ID), strings.NewReader(`{"data":{"title":"persist across restart","done":true},"expected_version":1}`))
	if err != nil {
		return "", err
	}
	x, err = h.Do(r)
	if err != nil {
		return "", err
	}
	var updated vpsCollectionDocument
	decodeErr = json.NewDecoder(io.LimitReader(x.Body, 1<<20)).Decode(&updated)
	x.Body.Close()
	if x.StatusCode != http.StatusOK || decodeErr != nil || updated.ID != created.ID || updated.Version != 2 || !bytes.Contains(updated.Data, []byte(`"done":true`)) {
		return "", errors.New("collection optimistic update did not succeed")
	}
	r, err = collectionRequest(ctx, http.MethodGet, host, "/_tinker/api/v1/db/tasks?snapshot=1", nil)
	if err != nil {
		return "", err
	}
	x, err = h.Do(r)
	if err != nil {
		return "", err
	}
	var snapshot struct {
		Documents []vpsCollectionDocument `json:"documents"`
		Revision  uint64                  `json:"revision"`
	}
	decodeErr = json.NewDecoder(io.LimitReader(x.Body, 1<<20)).Decode(&snapshot)
	x.Body.Close()
	if x.StatusCode != http.StatusOK || decodeErr != nil || snapshot.Revision < 2 || len(snapshot.Documents) != 1 || snapshot.Documents[0].ID != created.ID || snapshot.Documents[0].Version != 2 {
		return "", errors.New("collection snapshot did not prove list/current-state semantics")
	}
	return created.ID, nil
}

func (s *Suite) anonymousCollectionDenied(ctx context.Context, host, id string) error {
	r, err := collectionRequest(ctx, http.MethodGet, host, "/_tinker/api/v1/db/tasks/"+url.PathEscape(id), nil)
	if err != nil {
		return err
	}
	x, err := s.httpClient().Do(r)
	if err != nil {
		return err
	}
	body, readErr := io.ReadAll(io.LimitReader(x.Body, 1<<20))
	x.Body.Close()
	if readErr != nil {
		return readErr
	}
	if x.StatusCode != http.StatusUnauthorized || bytes.Contains(body, []byte("persist across restart")) {
		return fmt.Errorf("anonymous collection request leaked state: status=%d", x.StatusCode)
	}
	return nil
}

func (s *Suite) verifyCollectionPersistsAfterRestart(ctx context.Context, h *http.Client, host, id string) error {
	r, err := collectionRequest(ctx, http.MethodGet, host, "/_tinker/api/v1/db/tasks/"+url.PathEscape(id), nil)
	if err != nil {
		return err
	}
	x, err := h.Do(r)
	if err != nil {
		return err
	}
	var got vpsCollectionDocument
	decodeErr := json.NewDecoder(io.LimitReader(x.Body, 1<<20)).Decode(&got)
	x.Body.Close()
	if x.StatusCode != http.StatusOK || decodeErr != nil || got.ID != id || got.Version != 2 || !bytes.Contains(got.Data, []byte(`"done":true`)) {
		return errors.New("collection state did not persist through service restart")
	}
	return nil
}

func (s *Suite) crossAppCollectionDenied(ctx context.Context, h *http.Client, host, id string) error {
	r, err := collectionRequest(ctx, http.MethodGet, host, "/_tinker/api/v1/db/tasks/"+url.PathEscape(id), nil)
	if err != nil {
		return err
	}
	x, err := h.Do(r)
	if err != nil {
		return err
	}
	body, readErr := io.ReadAll(io.LimitReader(x.Body, 1<<20))
	x.Body.Close()
	if readErr != nil {
		return readErr
	}
	if x.StatusCode != http.StatusNotFound || bytes.Contains(body, []byte("persist across restart")) {
		return fmt.Errorf("cross-app collection read leaked state: status=%d", x.StatusCode)
	}
	return nil
}

func (s *Suite) deleteCollectionAndVerify(ctx context.Context, h *http.Client, host, id string) error {
	r, err := collectionRequest(ctx, http.MethodDelete, host, "/_tinker/api/v1/db/tasks/"+url.PathEscape(id), strings.NewReader(`{"expected_version":2}`))
	if err != nil {
		return err
	}
	x, err := h.Do(r)
	if err != nil {
		return err
	}
	var deleted struct {
		Deleted bool `json:"deleted"`
	}
	decodeErr := json.NewDecoder(io.LimitReader(x.Body, 1<<20)).Decode(&deleted)
	x.Body.Close()
	if x.StatusCode != http.StatusOK || decodeErr != nil || !deleted.Deleted {
		return errors.New("collection delete did not succeed")
	}
	r, err = collectionRequest(ctx, http.MethodGet, host, "/_tinker/api/v1/db/tasks/"+url.PathEscape(id), nil)
	if err != nil {
		return err
	}
	x, err = h.Do(r)
	if err != nil {
		return err
	}
	io.Copy(io.Discard, io.LimitReader(x.Body, 1<<20))
	x.Body.Close()
	if x.StatusCode != http.StatusNotFound {
		return errors.New("deleted collection document remained readable")
	}
	return nil
}

func blobList(ctx context.Context, h *http.Client, host string) ([]struct {
	ID string `json:"id"`
}, error) {
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, appURL(host, "/_tinker/api/v1/blobs"), nil)
	if err != nil {
		return nil, err
	}
	x, err := h.Do(r)
	if err != nil {
		return nil, err
	}
	defer x.Body.Close()
	var out struct {
		Blobs []struct {
			ID string `json:"id"`
		} `json:"blobs"`
	}
	if x.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(x.Body, 1<<20)).Decode(&out) != nil || out.Blobs == nil {
		return nil, errors.New("blob list failed")
	}
	return out.Blobs, nil
}

func blobRequest(ctx context.Context, method, host, path string, body io.Reader, contentType string) (*http.Request, error) {
	r, err := http.NewRequestWithContext(ctx, method, appURL(host, path), body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	if method != http.MethodGet {
		r.Header.Set("Origin", "https://"+host)
	}
	return r, nil
}

func rejectExtraBlobPartWithoutMutation(ctx context.Context, h *http.Client, host string) error {
	before, err := blobList(ctx, h, host)
	if err != nil {
		return err
	}
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	p, err := w.CreateFormFile("file", "one.txt")
	if err != nil {
		return err
	}
	if _, err = p.Write([]byte("one")); err != nil {
		return err
	}
	if err = w.WriteField("unexpected", "two"); err != nil {
		return err
	}
	if err = w.Close(); err != nil {
		return err
	}
	r, err := blobRequest(ctx, http.MethodPost, host, "/_tinker/api/v1/blobs", &body, w.FormDataContentType())
	if err != nil {
		return err
	}
	x, err := h.Do(r)
	if err != nil {
		return err
	}
	io.Copy(io.Discard, io.LimitReader(x.Body, 1<<20))
	x.Body.Close()
	if x.StatusCode != http.StatusBadRequest {
		return fmt.Errorf("extra multipart part accepted: status=%d", x.StatusCode)
	}
	after, err := blobList(ctx, h, host)
	if err != nil {
		return err
	}
	if len(before) != len(after) {
		return errors.New("extra multipart part mutated blob catalog")
	}
	return nil
}

func uploadBlob(ctx context.Context, h *http.Client, host string, want []byte) (string, error) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	p, err := w.CreateFormFile("file", "exact-bytes.bin")
	if err != nil {
		return "", err
	}
	if _, err = p.Write(want); err != nil {
		return "", err
	}
	if err = w.Close(); err != nil {
		return "", err
	}
	r, err := blobRequest(ctx, http.MethodPost, host, "/_tinker/api/v1/blobs", &body, w.FormDataContentType())
	if err != nil {
		return "", err
	}
	x, err := h.Do(r)
	if err != nil {
		return "", err
	}
	defer x.Body.Close()
	var out struct {
		ID   string `json:"id"`
		Size int64  `json:"size"`
		Name string `json:"name"`
	}
	if x.StatusCode != http.StatusCreated || json.NewDecoder(io.LimitReader(x.Body, 1<<20)).Decode(&out) != nil || out.ID == "" || out.Size != int64(len(want)) || out.Name != "exact-bytes.bin" {
		return "", errors.New("SDK-equivalent blob upload failed")
	}
	return out.ID, nil
}

var vpsBlobBytes = []byte("tinkercloud-vps-blob-exact-bytes\x00\xff")

func (s *Suite) anonymousBlobDenied(ctx context.Context, host, id string) error {
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, appURL(host, "/_tinker/api/v1/blobs/"+id), nil)
	if err != nil {
		return err
	}
	x, err := s.httpClient().Do(r)
	if err != nil {
		return err
	}
	b, err := io.ReadAll(io.LimitReader(x.Body, int64(len(vpsBlobBytes)+1024)))
	x.Body.Close()
	if err != nil {
		return err
	}
	if x.StatusCode != http.StatusUnauthorized || bytes.Contains(b, vpsBlobBytes) || bytes.Equal(b, vpsBlobBytes) {
		return fmt.Errorf("anonymous blob download leaked bytes: status=%d", x.StatusCode)
	}
	return nil
}

func (s *Suite) verifyBlobPersistsAfterRestart(ctx context.Context, h *http.Client, host, id string) error {
	// The caller's viewer cookie is preserved, but redirects are always surfaced
	// to this verifier. A restarted gateway that sends a viewer to another host
	// is never evidence of durable access to the original protected blob.
	hc := *h
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	retryCtx, cancel := context.WithTimeout(ctx, restartBlobReadinessBudget)
	defer cancel()

	var lastErr error
	for attempt := 1; attempt <= restartBlobReadinessAttempts; attempt++ {
		if err := retryCtx.Err(); err != nil {
			return fmt.Errorf("blob restart readiness exhausted: %w", err)
		}
		r, err := blobRequest(retryCtx, http.MethodGet, host, "/_tinker/api/v1/blobs/"+id, nil, "")
		if err != nil {
			return err
		}
		x, err := hc.Do(r)
		if err == nil {
			if x.StatusCode == http.StatusBadGateway || x.StatusCode == http.StatusServiceUnavailable || x.StatusCode == http.StatusGatewayTimeout {
				io.Copy(io.Discard, io.LimitReader(x.Body, 1<<20))
				x.Body.Close()
				lastErr = fmt.Errorf("blob gateway is still starting: status=%d", x.StatusCode)
			} else {
				return verifyRestartedBlobResponse(x)
			}
		} else if transientRestartTransportError(err) {
			lastErr = fmt.Errorf("blob gateway is not ready: %w", err)
		} else {
			return fmt.Errorf("blob restart readiness transport failure: %w", err)
		}

		if attempt == restartBlobReadinessAttempts {
			break
		}
		if err := s.waitRestartBlobReadiness(retryCtx, restartBlobDelay(attempt)); err != nil {
			return fmt.Errorf("blob restart readiness wait: %w", err)
		}
	}
	return fmt.Errorf("blob restart readiness did not succeed after %d attempts: %w", restartBlobReadinessAttempts, lastErr)
}

func verifyRestartedBlobResponse(x *http.Response) error {
	defer x.Body.Close()
	b, err := io.ReadAll(io.LimitReader(x.Body, int64(len(vpsBlobBytes)+1)))
	if err != nil {
		return err
	}
	if x.StatusCode != http.StatusOK || !bytes.Equal(b, vpsBlobBytes) || x.Header.Get("Cache-Control") != "private, no-store" || x.Header.Get("X-Content-Type-Options") != "nosniff" || !strings.HasPrefix(x.Header.Get("Content-Disposition"), "attachment;") {
		return fmt.Errorf("blob download/durability evidence failed: status=%d", x.StatusCode)
	}
	return nil
}

func transientRestartTransportError(err error) bool {
	if errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	var networkErr net.Error
	return errors.As(err, &networkErr) && (networkErr.Timeout() || networkErr.Temporary())
}

func (s *Suite) deleteBlobAndVerify(ctx context.Context, h *http.Client, host, id string) error {
	r, err := blobRequest(ctx, http.MethodDelete, host, "/_tinker/api/v1/blobs/"+id, nil, "")
	if err != nil {
		return err
	}
	x, err := h.Do(r)
	if err != nil {
		return err
	}
	io.Copy(io.Discard, io.LimitReader(x.Body, 1<<20))
	x.Body.Close()
	if x.StatusCode != http.StatusOK {
		return fmt.Errorf("blob delete failed: status=%d", x.StatusCode)
	}
	r, err = blobRequest(ctx, http.MethodGet, host, "/_tinker/api/v1/blobs/"+id, nil, "")
	if err != nil {
		return err
	}
	x, err = h.Do(r)
	if err != nil {
		return err
	}
	b, _ := io.ReadAll(io.LimitReader(x.Body, int64(len(vpsBlobBytes)+1)))
	x.Body.Close()
	if x.StatusCode != http.StatusNotFound || bytes.Contains(b, vpsBlobBytes) {
		return errors.New("deleted blob remained readable")
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
		if local == "" || loopbackSocket(local) || !strings.Contains(line, `"tinkercloud"`) {
			continue
		}
		port := socketPort(local)
		if port != "80" && port != "443" {
			return fmt.Errorf("unexpected tinkercloud public listener: %s", line)
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
				if !strings.Contains(line, `"tinkercloud"`) {
					return fmt.Errorf("public port %s is not owned by tinkercloud: %s", port, line)
				}
				found = true
			}
		}
		if !found {
			return fmt.Errorf("tinkercloud does not own public port %s", port)
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

var hiddenInput = regexp.MustCompile(`name="(transaction|csrf)" value="([^"]+)"`)

func hiddenValue(html, name string) string {
	if name != "transaction" && name != "csrf" {
		return ""
	}
	for _, m := range hiddenInput.FindAllStringSubmatch(html, -1) {
		if len(m) == 3 && m[1] == name {
			return m[2]
		}
	}
	return ""
}
