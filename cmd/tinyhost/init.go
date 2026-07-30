package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/tinyhost/tiny/internal/config"
	"github.com/tinyhost/tiny/internal/operations"
	"github.com/tinyhost/tiny/internal/persistence"
)

const (
	defaultConfigPath     = "/etc/tinyhost/config.yaml"
	defaultCredentialPath = "/etc/tinyhost/credentials/tinyhost.env"
	maxPublicGatewayProof = 32 << 10
)

// initRuntime isolates host mutation and network checks. Production uses the
// defaults below; tests must opt into fakes rather than an undocumented bypass.
type initRuntime struct {
	GOOS, GOARCH func() string
	ReadFile     func(string) ([]byte, error)
	Listen       func(network, address string) (net.Listener, error)
	ClockOK      func(context.Context) error
	Run          func(context.Context, string, ...string) error
	Install      func(configPath, credentialPath string) error
	LocalHealth  func(context.Context, string) error
	PublicHealth func(context.Context, string) error
}

var productionInitRuntime = initRuntime{
	GOOS:     func() string { return runtime.GOOS },
	GOARCH:   func() string { return runtime.GOARCH },
	ReadFile: os.ReadFile,
	Listen:   net.Listen,
	ClockOK: func(ctx context.Context) error {
		out, err := exec.CommandContext(ctx, "timedatectl", "show", "--property=NTPSynchronized", "--value").Output()
		if err != nil || strings.TrimSpace(string(out)) != "yes" {
			return errors.New("clock is not NTP synchronized")
		}
		return nil
	},
	Run: func(ctx context.Context, name string, args ...string) error {
		return exec.CommandContext(ctx, name, args...).Run()
	},
	Install: func(configPath, credentialPath string) error {
		return runInstallService([]string{"--config", configPath, "--credentials", credentialPath})
	},
	LocalHealth: func(ctx context.Context, _ string) error {
		return exec.CommandContext(ctx, "systemctl", "is-active", "--quiet", "tinyhost.service").Run()
	},
	PublicHealth: productionPublicGatewayHealth,
}

// productionPublicGatewayHealth derives its sole public probe target from the
// validated platform host. Its transport deliberately uses normal certificate
// verification: init must not turn a reachable but impersonated TLS endpoint
// into readiness evidence.
func productionPublicGatewayHealth(ctx context.Context, platformHost string) error {
	tr := &http.Transport{TLSClientConfig: &tls.Config{ServerName: platformHost, MinVersion: tls.VersionTLS12}}
	defer tr.CloseIdleConnections()
	return publicGatewayHealth(ctx, platformHost, &http.Client{
		Transport: tr,
		Timeout:   10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	})
}

// publicGatewayHealth proves that the public HTTPS endpoint is this
// TinyHost gateway, rather than merely some reachable web server. It is kept
// separate from its production transport so tests can supply a locally trusted
// TLS client without adding an insecure production configuration path.
func publicGatewayHealth(ctx context.Context, platformHost string, client *http.Client) error {
	if platformHost == "" || client == nil {
		return errors.New("gateway public health response invalid")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	endpoint := "https://" + platformHost + "/api/v1/version"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return errors.New("gateway public health response invalid")
	}
	resp, err := client.Do(req)
	if err != nil || resp == nil || resp.Body == nil || resp.Request == nil || resp.Request.URL == nil {
		return errors.New("gateway public health response invalid")
	}
	defer resp.Body.Close()
	// A health probe must not follow a portal, proxy, or different endpoint.
	// Comparing the final URL also keeps a custom test transport honest.
	if resp.Request.URL.String() != endpoint || resp.Request.URL.Scheme != "https" || !strings.EqualFold(resp.Request.URL.Hostname(), platformHost) {
		return errors.New("gateway public health response invalid")
	}
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Cache-Control") != "no-store" || resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		return errors.New("gateway public health response invalid")
	}
	contentType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" {
		return errors.New("gateway public health response invalid")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxPublicGatewayProof+1))
	if err != nil || len(body) > maxPublicGatewayProof || !validGatewayVersionProof(body) {
		return errors.New("gateway public health response invalid")
	}
	return nil
}

// validGatewayVersionProof accepts exactly one protocol field. Parsing the
// object token-by-token rejects duplicate keys and trailing JSON, both of
// which would make an otherwise simple version proof ambiguous.
func validGatewayVersionProof(body []byte) bool {
	dec := json.NewDecoder(bytes.NewReader(body))
	token, err := dec.Token()
	if err != nil || token != json.Delim('{') || !dec.More() {
		return false
	}
	key, err := dec.Token()
	if err != nil || key != "api_version" {
		return false
	}
	var version json.RawMessage
	if err := dec.Decode(&version); err != nil || string(version) != "1" || dec.More() {
		return false
	}
	token, err = dec.Token()
	if err != nil || token != json.Delim('}') {
		return false
	}
	var trailing any
	return dec.Decode(&trailing) == io.EOF
}

func osReleaseValue(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	if raw[0] == '"' || raw[0] == '\'' {
		quote := raw[0]
		if len(raw) < 2 || raw[len(raw)-1] != quote {
			return "", false
		}
		value := raw[1 : len(raw)-1]
		if value == "" || strings.ContainsRune(value, rune(quote)) || strings.ContainsAny(value, "\\\r\n") {
			return "", false
		}
		return value, true
	}
	if strings.ContainsAny(raw, " \t\"'\\`$;\r\n") {
		return "", false
	}
	return raw, true
}

func supportedUbuntuHost(data []byte) bool {
	values := map[string]string{}
	seen := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		key, raw, ok := strings.Cut(line, "=")
		if !ok || key != "ID" && key != "VERSION_ID" {
			continue
		}
		if seen[key] {
			return false
		}
		value, valid := osReleaseValue(raw)
		if !valid {
			return false
		}
		seen[key] = true
		values[key] = value
	}
	if values["ID"] != "ubuntu" {
		return false
	}
	switch values["VERSION_ID"] {
	case "24.04", "26.04":
		return true
	default:
		return false
	}
}

func runPreflight(ctx context.Context, rt initRuntime) error {
	if rt.GOOS() != "linux" || rt.GOARCH() != "amd64" {
		return errors.New("tinyhost: unsupported_host")
	}
	osRelease, err := rt.ReadFile("/etc/os-release")
	if err != nil || !supportedUbuntuHost(osRelease) {
		return errors.New("tinyhost: unsupported_host")
	}
	if err = rt.ClockOK(ctx); err != nil {
		return errors.New("tinyhost: clock_unsynchronized")
	}
	for _, addr := range []string{":80", ":443"} {
		listener, listenErr := rt.Listen("tcp", addr)
		if listenErr != nil {
			return errors.New("tinyhost: required_port_unavailable")
		}
		_ = listener.Close()
	}
	return nil
}

// runInit is intentionally non-interactive. Supplying secret values on argv is
// rejected by design: only root-readable files are accepted and copied to the
// systemd EnvironmentFile atomically.
func runInit(args []string, out *os.File, rt initRuntime) error {
	if effectiveUID() != 0 {
		return errors.New("tinyhost: root_required")
	}
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	cfgPath := fs.String("config", defaultConfigPath, "root-owned TinyHost config path")
	credentialPath := fs.String("credentials", defaultCredentialPath, "root-owned systemd environment file")
	platformHost := fs.String("platform-host", "", "platform DNS host")
	appSuffix := fs.String("app-suffix", "", "wildcard app DNS suffix")
	email := fs.String("operator-email", "", "initial operator email")
	emailFrom := fs.String("email-from", "", "verified Resend sender")
	acmeEmail := fs.String("acme-email", "", "ACME contact email")
	dataDir := fs.String("data-directory", "/var/lib/tinyhost", "private data directory")
	acmeDir := fs.String("acme-cache-directory", "/var/lib/tinyhost-acme", "private ACME cache directory")
	updateReleaseBase := fs.String("update-release-base", "", "HTTPS release directory used by manual updates")
	resendFile := fs.String("resend-api-key-file", "", "root-readable file containing the Resend key")
	hmacFile := fs.String("hmac-key-file", "", "root-readable file containing >=32 byte session HMAC key")
	nonInteractive := fs.Bool("non-interactive", false, "never prompt; require explicit values")
	if fs.Parse(args) != nil || len(fs.Args()) != 0 || !*nonInteractive || *email == "" {
		return errors.New("tinyhost: invalid_arguments")
	}
	if !filepath.IsAbs(*cfgPath) || !filepath.IsAbs(*credentialPath) {
		return errors.New("tinyhost: unsafe_path")
	}

	var cfg config.Config
	createdConfig := false
	if info, err := os.Lstat(*cfgPath); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0037 != 0 {
			return errors.New("tinyhost: unsafe_config_path")
		}
		var loadErr error
		cfg, loadErr = config.LoadYAML(*cfgPath)
		if loadErr != nil {
			return errors.New("tinyhost: config_invalid")
		}
	} else if os.IsNotExist(err) {
		if *platformHost == "" || *appSuffix == "" || *emailFrom == "" || *acmeEmail == "" {
			return errors.New("tinyhost: config_values_required")
		}
		cfg = config.Config{PlatformHost: *platformHost, AppSuffix: *appSuffix, SessionCookie: "__Host-tiny_app", ListenHTTP: ":80", ListenHTTPS: ":443", DataDirectory: *dataDir, ACMECachedir: *acmeDir, UpdateReleaseBase: *updateReleaseBase, EmailFrom: *emailFrom, ACMEEmail: *acmeEmail, ResendAPIKeyRef: "env:RESEND_API_KEY", HMACKeyRef: "env:TINYHOST_HMAC_KEY", LLMRootKeyRef: "env:TINYHOST_LLM_ROOT_KEY", OTPExpiry: 10 * time.Minute, OTPMaxAttempts: 5, SessionExpiry: 24 * time.Hour}
		if err := cfg.Validate(); err != nil {
			return errors.New("tinyhost: config_invalid")
		}
		createdConfig = true
	} else {
		return err
	}
	statePath := filepath.Join(filepath.Dir(*cfgPath), "init-state.json")
	if err := ensureStateDirectory(filepath.Dir(statePath)); err != nil {
		return err
	}
	stateBytes, err := os.ReadFile(statePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	state, err := operations.ParseInitState(stateBytes)
	if err != nil {
		return errors.New("tinyhost: init_state_invalid")
	}
	persist := func() error {
		b, e := state.MarshalJSONState()
		if e != nil {
			return e
		}
		return writeAtomicPrivate(statePath, b)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if state.Next() == operations.InitPreflight {
		if err := runPreflight(ctx, rt); err != nil {
			return err
		}
		if err := state.Complete(operations.InitPreflight); err != nil {
			return err
		}
		if err := persist(); err != nil {
			return err
		}
	}
	if state.Next() == operations.InitPaths {
		if err := provisionPaths(ctx, rt, cfg, *credentialPath, *resendFile, *hmacFile, createdConfig, *cfgPath); err != nil {
			return err
		}
		if err := state.Complete(operations.InitPaths); err != nil {
			return err
		}
		if err := persist(); err != nil {
			return err
		}
	}
	if state.Next() == operations.InitDatabase {
		db, openErr := persistence.OpenSQLite(ctx, filepath.Join(cfg.DataDirectory, "tinyhost.db"))
		if openErr != nil {
			return openErr
		}
		_ = db.Close()
		if err := state.Complete(operations.InitDatabase); err != nil {
			return err
		}
		if err := persist(); err != nil {
			return err
		}
	}
	if state.Next() == operations.InitOperator {
		db, openErr := persistence.OpenSQLite(ctx, filepath.Join(cfg.DataDirectory, "tinyhost.db"))
		if openErr != nil {
			return openErr
		}
		err = db.EnsureInitialOperator(ctx, *email, "init")
		_ = db.Close()
		if err != nil {
			return err
		}
		if err := state.Complete(operations.InitOperator); err != nil {
			return err
		}
		if err := persist(); err != nil {
			return err
		}
	}
	if state.Next() == operations.InitService {
		if err := rt.Install(*cfgPath, *credentialPath); err != nil {
			return errors.New("tinyhost: service_failed")
		}
		if err := state.Complete(operations.InitService); err != nil {
			return err
		}
		if err := persist(); err != nil {
			return err
		}
	}
	if state.Next() == operations.InitVerified {
		if err := rt.LocalHealth(ctx, *cfgPath); err != nil {
			return errors.New("tinyhost: local_health_failed")
		}
		if err := rt.PublicHealth(ctx, cfg.PlatformHost); err != nil {
			return errors.New("tinyhost: public_health_failed")
		}
		if err := state.Complete(operations.InitVerified); err != nil {
			return err
		}
		if err := persist(); err != nil {
			return err
		}
	}
	fmt.Fprintf(out, "init complete: https://%s/\n", cfg.PlatformHost)
	return nil
}

func provisionPaths(ctx context.Context, rt initRuntime, cfg config.Config, credentialPath, resendFile, hmacFile string, writeConfig bool, configPath string) error {
	if err := ensureServiceUser(ctx, rt); err != nil {
		return err
	}
	// The service group needs to traverse the config directory to read the
	// non-secret config. The persisted init state remains root-only (0600), and
	// credentials live under a separate root-only subdirectory.
	if err := os.Chmod(filepath.Dir(configPath), 0750); err != nil {
		return err
	}
	if err := rt.Run(ctx, "chown", "root:tinyhost", filepath.Dir(configPath)); err != nil {
		return errors.New("tinyhost: ownership_failed")
	}
	for _, p := range []string{cfg.DataDirectory, cfg.ACMECachedir, filepath.Dir(credentialPath)} {
		if err := ensurePrivateDirectory(p); err != nil {
			return err
		}
	}
	if err := rt.Run(ctx, "chown", "-R", "tinyhost:tinyhost", cfg.DataDirectory, cfg.ACMECachedir); err != nil {
		return errors.New("tinyhost: ownership_failed")
	}
	if writeConfig {
		b, err := cfg.RenderYAML()
		if err != nil {
			return err
		}
		if err = writeAtomicPrivate(configPath, b); err != nil {
			return err
		}
	}
	// The config deliberately contains no secret material. The service must be
	// able to read it after systemd drops privileges, while the credential file
	// remains root-only and is consumed by systemd before exec.
	if err := os.Chmod(configPath, 0640); err != nil {
		return err
	}
	if err := rt.Run(ctx, "chown", "root:tinyhost", configPath); err != nil {
		return errors.New("tinyhost: ownership_failed")
	}
	if info, err := os.Lstat(credentialPath); os.IsNotExist(err) {
		if resendFile == "" || hmacFile == "" {
			return errors.New("tinyhost: credential_files_required")
		}
		resend, err := readSecretFile(resendFile)
		if err != nil {
			return err
		}
		hmac, err := readSecretFile(hmacFile)
		if err != nil {
			return err
		}
		if len(hmac) < 32 {
			return errors.New("tinyhost: hmac_invalid")
		}
		resendName, hmacName, llmName, err := credentialNames(cfg)
		if err != nil {
			return err
		}
		credential := resendName + "=" + resend + "\n" + hmacName + "=" + hmac + "\n"
		if llmName != "" {
			root := make([]byte, 32)
			if _, err = rand.Read(root); err != nil {
				return errors.New("tinyhost: llm_root_generation_failed")
			}
			credential += llmName + "=" + hex.EncodeToString(root) + "\n"
		}
		if err = writeAtomicPrivate(credentialPath, []byte(credential)); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return errors.New("tinyhost: unsafe_credential_path")
	}
	if err := os.Chmod(credentialPath, 0600); err != nil {
		return err
	}
	if err := rt.Run(ctx, "chown", "root:root", credentialPath); err != nil {
		return errors.New("tinyhost: ownership_failed")
	}
	return nil
}

func credentialNames(cfg config.Config) (string, string, string, error) {
	name := func(ref string) (string, bool) {
		v, ok := strings.CutPrefix(ref, "env:")
		if !ok || v == "" {
			return "", false
		}
		for i, r := range v {
			if !(r == '_' || r >= 'A' && r <= 'Z' || i > 0 && r >= '0' && r <= '9') {
				return "", false
			}
		}
		return v, true
	}
	resend, ok := name(cfg.ResendAPIKeyRef)
	if !ok {
		return "", "", "", errors.New("tinyhost: unsafe_secret_reference")
	}
	hmac, ok := name(cfg.HMACKeyRef)
	if !ok || resend == hmac {
		return "", "", "", errors.New("tinyhost: unsafe_secret_reference")
	}
	if cfg.LLMRootKeyRef == "" {
		return resend, hmac, "", nil
	}
	llm, ok := name(cfg.LLMRootKeyRef)
	if !ok || llm == resend || llm == hmac {
		return "", "", "", errors.New("tinyhost: unsafe_secret_reference")
	}
	return resend, hmac, llm, nil
}

func ensurePrivateDirectory(path string) error {
	if info, err := os.Lstat(path); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
			return errors.New("tinyhost: unsafe_path")
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	return os.Chmod(path, 0700)
}

func ensureStateDirectory(path string) error {
	if info, err := os.Lstat(path); err == nil {
		// The service group may traverse this directory to read the non-secret
		// config, but may neither write it nor read the root-only init state.
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0027 != 0 {
			return errors.New("tinyhost: unsafe_path")
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	return os.Chmod(path, 0700)
}

func readSecretFile(path string) (string, error) {
	st, err := os.Lstat(path)
	if err != nil || st.Mode()&os.ModeSymlink != 0 || st.Mode().Perm()&0077 != 0 {
		return "", errors.New("tinyhost: unsafe_secret_file")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	v := strings.TrimSpace(string(b))
	if v == "" || strings.ContainsAny(v, " \t\r\n\x00#'\\\"") {
		return "", errors.New("tinyhost: unsafe_secret_file")
	}
	return v, nil
}

func ensureServiceUser(ctx context.Context, rt initRuntime) error {
	if err := rt.Run(ctx, "id", "-u", "tinyhost"); err == nil {
		return nil
	}
	if err := rt.Run(ctx, "useradd", "--system", "--home-dir", "/var/lib/tinyhost", "--shell", "/usr/sbin/nologin", "tinyhost"); err != nil {
		return errors.New("tinyhost: service_user_failed")
	}
	return nil
}
