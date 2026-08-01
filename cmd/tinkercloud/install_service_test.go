package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ChrisMarxDev/tinkercloud/internal/config"
	"github.com/ChrisMarxDev/tinkercloud/internal/operations"
)

func installServiceFixture(t *testing.T) (root string, cfg config.Config, configPath, credentialPath, unitPath string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfg = config.Config{
		Domain:          "apps.tinker.example.test",
		SessionCookie:   "__Host-tinker_app",
		ListenHTTP:      ":80",
		ListenHTTPS:     ":443",
		DataDirectory:   filepath.Join(root, "custom-data"),
		ACMECachedir:    filepath.Join(root, "custom-acme"),
		EmailFrom:       "operator@example.test",
		ACMEEmail:       "operator@example.test",
		ResendAPIKeyRef: "env:RESEND_API_KEY",
		HMACKeyRef:      "env:TINKERCLOUD_HMAC_KEY",
		OTPExpiry:       10 * time.Minute,
		OTPMaxAttempts:  5,
		SessionExpiry:   24 * time.Hour,
	}
	for _, path := range []string{cfg.DataDirectory, cfg.ACMECachedir} {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	configPath = filepath.Join(root, "config.yaml")
	credentialPath = filepath.Join(root, "tinkercloud.env")
	unitPath = filepath.Join(root, "systemd", "tinkercloud.service")
	if err := os.Mkdir(filepath.Dir(unitPath), 0755); err != nil {
		t.Fatal(err)
	}
	body, err := cfg.RenderYAML()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, body, 0640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(credentialPath, []byte(credentialTestFixture("x")), 0600); err != nil {
		t.Fatal(err)
	}
	return root, cfg, configPath, credentialPath, unitPath
}

func TestInstallServiceUsesConfiguredWritablePaths(t *testing.T) {
	_, cfg, configPath, credentialPath, unitPath := installServiceFixture(t)
	oldUID, oldSystemctl := effectiveUID, systemctlRunner
	effectiveUID = func() int { return 0 }
	var systemctlCalls int
	systemctlRunner = func(...string) error { systemctlCalls++; return nil }
	t.Cleanup(func() { effectiveUID, systemctlRunner = oldUID, oldSystemctl })

	if err := runInstallService([]string{"--unit-path", unitPath, "--config", configPath, "--credentials", credentialPath}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatal(err)
	}
	unit := string(body)
	if !strings.Contains(unit, "ReadWritePaths="+cfg.DataDirectory+"\n") ||
		!strings.Contains(unit, "ReadWritePaths="+cfg.ACMECachedir+"\n") ||
		operations.ValidateServiceUnit(unit, cfg.DataDirectory, cfg.ACMECachedir) != nil ||
		systemctlCalls != 2 {
		t.Fatal(unit, systemctlCalls)
	}
}

func TestInstallServiceRejectsSymlinkInputsBeforeSystemctl(t *testing.T) {
	root, cfg, configPath, credentialPath, unitPath := installServiceFixture(t)
	oldUID, oldSystemctl := effectiveUID, systemctlRunner
	effectiveUID = func() int { return 0 }
	systemctlCalls := 0
	systemctlRunner = func(...string) error { systemctlCalls++; return nil }
	t.Cleanup(func() { effectiveUID, systemctlRunner = oldUID, oldSystemctl })

	link := filepath.Join(root, "config-link.yaml")
	if err := os.Symlink(configPath, link); err != nil {
		t.Fatal(err)
	}
	if err := runInstallService([]string{"--unit-path", unitPath, "--config", link, "--credentials", credentialPath}); err == nil {
		t.Fatal("symlink config accepted")
	}
	if systemctlCalls != 0 {
		t.Fatal("systemctl called for rejected input")
	}

	dataLink := filepath.Join(root, "data-link")
	if err := os.Symlink(cfg.DataDirectory, dataLink); err != nil {
		t.Fatal(err)
	}
	cfg.DataDirectory = dataLink
	body, err := cfg.RenderYAML()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, body, 0640); err != nil {
		t.Fatal(err)
	}
	if err := runInstallService([]string{"--unit-path", unitPath, "--config", configPath, "--credentials", credentialPath}); err == nil {
		t.Fatal("symlink data directory accepted")
	}
	if systemctlCalls != 0 {
		t.Fatal("systemctl called for rejected data directory")
	}
}
