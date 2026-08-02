package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ChrisMarxDev/tinkercloud/internal/config"
)

func setupTestRuntime(lines ...string) setupRuntime {
	i := 0
	return setupRuntime{
		UID:   func() int { return 0 },
		IsTTY: func(*os.File) bool { return true },
		ReadLine: func(_ *os.File, _ *os.File, _ string) (string, error) {
			if i >= len(lines) {
				return "", errors.New("unexpected prompt")
			}
			v := lines[i]
			i++
			return v, nil
		},
		Preflight: func(initRuntime) error { return nil },
		RunInit:   func([]string, *os.File, initRuntime) error { return nil },
	}
}

func TestSetupRunsNewHostPreflightBeforePrompting(t *testing.T) {
	rt := setupTestRuntime("must-not-be-read")
	rt.Preflight = func(initRuntime) error { return errors.New("unsupported") }
	rt.ReadLine = func(*os.File, *os.File, string) (string, error) {
		t.Fatal("prompted before preflight")
		return "", nil
	}
	err := runSetup([]string{"--config", filepath.Join(t.TempDir(), "config.yaml"), "--credentials", filepath.Join(t.TempDir(), "credentials")}, os.Stdout, os.Stderr, rt)
	if err == nil || err.Error() != "unsupported" {
		t.Fatalf("preflight failure: %v", err)
	}
}

func TestSetupDeniesNonRootAndNonTTY(t *testing.T) {
	rt := setupTestRuntime()
	rt.UID = func() int { return 1000 }
	if err := runSetup(nil, os.Stdout, os.Stderr, rt); err == nil || err.Error() != "tinkercloud: root_required" {
		t.Fatalf("non-root: %v", err)
	}
	rt = setupTestRuntime()
	rt.IsTTY = func(*os.File) bool { return false }
	if err := runSetup(nil, os.Stdout, os.Stderr, rt); err == nil || err.Error() != "tinkercloud: setup_tty_required" {
		t.Fatalf("non-tty: %v", err)
	}
}

func TestSetupCollectsOnlyMissingValuesAndGeneratesPrivateHMACSource(t *testing.T) {
	root := t.TempDir()
	resend := filepath.Join(root, "resend")
	if err := os.WriteFile(resend, []byte("resend-secret"), 0600); err != nil {
		t.Fatal(err)
	}
	rt := setupTestRuntime("example.test", "operator@example.test", "sender@example.test", resend)
	var got []string
	rt.RunInit = func(args []string, _ *os.File, _ initRuntime) error {
		got = append([]string(nil), args...)
		var hmac string
		for i := range args {
			if args[i] == "--hmac-key-file" {
				hmac = args[i+1]
			}
		}
		st, err := os.Stat(hmac)
		if err != nil || st.Mode().Perm() != 0600 {
			t.Fatalf("private HMAC source: %v, %v", st, err)
		}
		b, err := os.ReadFile(hmac)
		if err != nil || len(strings.TrimSpace(string(b))) < 64 {
			t.Fatalf("generated HMAC: %q, %v", b, err)
		}
		return nil
	}
	err := runSetup([]string{"--config", filepath.Join(root, "etc", "config.yaml"), "--credentials", filepath.Join(root, "credentials", "env")}, os.Stdout, os.Stderr, rt)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(got, " ")
	for _, forbidden := range []string{"resend-secret", "--acme-email"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("secret/ACME flag leaked into init argv: %s", joined)
		}
	}
	if !strings.Contains(joined, "--operator-email operator@example.test") {
		t.Fatalf("operator not passed: %s", joined)
	}
}

func TestSetupDeniesUnsafeOrMalformedSecretSource(t *testing.T) {
	root := t.TempDir()
	unsafe := filepath.Join(root, "unsafe")
	if err := os.WriteFile(unsafe, []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}
	rt := setupTestRuntime("example.test", "operator@example.test", "sender@example.test", unsafe)
	err := runSetup([]string{"--config", filepath.Join(root, "config.yaml"), "--credentials", filepath.Join(root, "credentials")}, os.Stdout, os.Stderr, rt)
	if err == nil || err.Error() != "tinkercloud: unsafe_secret_file" {
		t.Fatalf("unsafe source: %v", err)
	}
	rt = setupTestRuntime("example.test", "not-an-email", "sender@example.test", unsafe)
	err = runSetup([]string{"--config", filepath.Join(root, "other.yaml"), "--credentials", filepath.Join(root, "other-credentials")}, os.Stdout, os.Stderr, rt)
	if err == nil || err.Error() != "tinkercloud: config_invalid" {
		t.Fatalf("malformed input: %v", err)
	}
}

func TestSetupDeniesDirectoryConfigAndCredentialPaths(t *testing.T) {
	root := t.TempDir()
	rt := setupTestRuntime()
	if err := runSetup([]string{"--config", root, "--credentials", filepath.Join(root, "credentials")}, os.Stdout, os.Stderr, rt); err == nil || err.Error() != "tinkercloud: unsafe_config_path" {
		t.Fatalf("directory config: %v", err)
	}
	cfgPath := filepath.Join(root, "config.yaml")
	cfg, err := (config.Config{Domain: "example.test", EmailFrom: "sender@example.test", ACMEEmail: "operator@example.test", SessionCookie: "__Host-tinker_app", ListenHTTP: ":80", ListenHTTPS: ":443", DataDirectory: "/var/lib/tinkercloud", ACMECachedir: "/var/lib/tinkercloud-acme", EmailAPIKeyRef: "env:RESEND_API_KEY", HMACKeyRef: "env:TINKERCLOUD_HMAC_KEY", LLMRootKeyRef: "env:TINKERCLOUD_LLM_ROOT_KEY", OTPExpiry: 10 * time.Minute, OTPMaxAttempts: 5, SessionExpiry: 24 * time.Hour}).RenderYAML()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, cfg, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "credentials"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := runSetup([]string{"--config", cfgPath, "--credentials", filepath.Join(root, "credentials")}, os.Stdout, os.Stderr, rt); err == nil || err.Error() != "tinkercloud: unsafe_credential_path" {
		t.Fatalf("directory credentials: %v", err)
	}
}

func TestSetupResumeReusesConfigAndCredentialsWithoutPrompts(t *testing.T) {
	root := t.TempDir()
	cfgPath := filepath.Join(root, "config.yaml")
	cfg, err := (config.Config{Domain: "example.test", EmailFrom: "sender@example.test", ACMEEmail: "operator@example.test", SessionCookie: "__Host-tinker_app", ListenHTTP: ":80", ListenHTTPS: ":443", DataDirectory: "/var/lib/tinkercloud", ACMECachedir: "/var/lib/tinkercloud-acme", EmailAPIKeyRef: "env:RESEND_API_KEY", HMACKeyRef: "env:TINKERCLOUD_HMAC_KEY", LLMRootKeyRef: "env:TINKERCLOUD_LLM_ROOT_KEY", OTPExpiry: 10 * time.Minute, OTPMaxAttempts: 5, SessionExpiry: 24 * time.Hour}).RenderYAML()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, cfg, 0600); err != nil {
		t.Fatal(err)
	}
	credentials := filepath.Join(root, "credentials")
	if err := os.WriteFile(credentials, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	rt := setupTestRuntime()
	called := false
	rt.RunInit = func(args []string, _ *os.File, _ initRuntime) error {
		called = true
		if strings.Contains(strings.Join(args, " "), "--domain") || strings.Contains(strings.Join(args, " "), "--resend-api-key-file") {
			t.Fatal("resume recollected values")
		}
		return errors.New("interrupted")
	}
	err = runSetup([]string{"--config", cfgPath, "--credentials", credentials}, os.Stdout, os.Stderr, rt)
	if !called || err == nil || err.Error() != "interrupted" {
		t.Fatalf("resume: called=%v err=%v", called, err)
	}
}
