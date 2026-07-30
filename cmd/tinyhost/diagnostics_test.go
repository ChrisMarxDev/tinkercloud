package main

import (
	"context"
	"errors"
	"github.com/tinyhost/tiny/internal/config"
	"github.com/tinyhost/tiny/internal/operations"
	"github.com/tinyhost/tiny/internal/persistence"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func diagnosticFixture(t *testing.T) (config.Config, string) {
	t.Helper()
	root := t.TempDir()
	data, cache := filepath.Join(root, "data"), filepath.Join(root, "cache")
	if err := os.Mkdir(data, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(cache, 0700); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(root, "init-state.json")
	if err := os.WriteFile(state, []byte(`{"completed":{"preflight":true,"paths":true,"database":true,"operator":true,"service":true,"verified":true}}`), 0600); err != nil {
		t.Fatal(err)
	}
	db, err := persistence.OpenSQLite(context.Background(), filepath.Join(data, "tinyhost.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return config.Config{Domain: "apps.tiny.example.test", ListenHTTP: ":80", ListenHTTPS: ":443", DataDirectory: data, ACMECachedir: cache, ResendAPIKeyRef: "env:TINY_TEST_RESEND"}, state
}

func goodDeps() diagnosticDeps {
	return diagnosticDeps{
		DiskFreePercent: func(string) (uint64, error) { return 80, nil },
		SQLiteCheck:     sqliteQuickCheck,
		ClockOK:         func(context.Context) error { return nil }, ServiceOK: func(context.Context) error { return nil }, PortOK: func(context.Context, string) error { return nil },
		DNSLookup: func(context.Context, string) ([]string, error) { return []string{"127.0.0.1"}, nil }, TLSCheck: func(context.Context, string, string) error { return nil }, ResendCheck: func(context.Context, string) error { return nil },
	}
}

func named(checks []operations.Check) []string {
	out := make([]string, len(checks))
	for i := range checks {
		out[i] = checks[i].Name
	}
	return out
}

func TestStatusIsLocalOnlyAndSorted(t *testing.T) {
	cfg, state := diagnosticFixture(t)
	d := goodDeps()
	d.DNSLookup = func(context.Context, string) ([]string, error) { t.Fatal("status resolved DNS"); return nil, nil }
	d.TLSCheck = func(context.Context, string, string) error { t.Fatal("status checked TLS"); return nil }
	d.ResendCheck = func(context.Context, string) error { t.Fatal("status checked Resend"); return nil }
	got := diagnoseWith(context.Background(), cfg, false, d, doctorCredentials{}, state)
	want := []string{"acme_cache", "clock", "config", "data_directory", "disk", "init_state", "port_http", "port_https", "service", "sqlite", "update_rollback", "version"}
	if strings.Join(named(got), ",") != strings.Join(want, ",") {
		t.Fatalf("checks: %#v", named(got))
	}
	for _, check := range got {
		if !check.Healthy {
			t.Fatal(check)
		}
	}
}

func TestDoctorChecksNetworkWithoutLeakingSecret(t *testing.T) {
	cfg, state := diagnosticFixture(t)
	d := goodDeps()
	var dns, tls, resend int
	d.DNSLookup = func(_ context.Context, host string) ([]string, error) {
		dns++
		if host != cfg.PlatformHost() && host != "tinyhost-doctor."+cfg.AppSuffix() {
			t.Fatal(host)
		}
		return []string{"203.0.113.1"}, nil
	}
	d.TLSCheck = func(_ context.Context, host, port string) error {
		tls++
		if host != cfg.PlatformHost() || port != "443" {
			t.Fatal(host, port)
		}
		return nil
	}
	d.ResendCheck = func(_ context.Context, key string) error {
		resend++
		if key != "super-secret-value" {
			t.Fatal("missing credential")
		}
		return nil
	}
	got := diagnoseWith(context.Background(), cfg, true, d, doctorCredentials{resendAPIKey: "super-secret-value"}, state)
	if dns != 2 || tls != 1 || resend != 1 {
		t.Fatalf("dns=%d tls=%d resend=%d", dns, tls, resend)
	}
	for _, c := range got {
		if strings.Contains(c.Detail, "secret") || strings.Contains(c.Detail, "203.0.113") || c.Secret {
			t.Fatalf("leaked diagnostic: %#v", c)
		}
	}
}

func TestDoctorFailuresAreTypedAndRedacted(t *testing.T) {
	cfg, state := diagnosticFixture(t)
	d := goodDeps()
	d.DNSLookup = func(context.Context, string) ([]string, error) { return nil, errors.New("dns 198.51.100.7 secret") }
	d.TLSCheck = func(context.Context, string, string) error { return errors.New("certificate detail") }
	d.ResendCheck = func(context.Context, string) error { return errors.New("Bearer forbidden-secret body") }
	got := diagnoseWith(context.Background(), cfg, true, d, doctorCredentials{resendAPIKey: "forbidden-secret"}, state)
	for _, name := range []string{"dns_platform", "dns_wildcard", "tls_platform", "resend"} {
		for _, c := range got {
			if c.Name == name && c.Healthy {
				t.Fatalf("%s healthy", name)
			}
		}
	}
	for _, c := range got {
		if strings.Contains(c.Detail, "secret") || strings.Contains(c.Detail, "198.51") || strings.Contains(c.Detail, "certificate detail") {
			t.Fatalf("leaked detail: %#v", c)
		}
	}
}

func TestDiagnoseDeniesMissingOrInsecureDirectory(t *testing.T) {
	root := t.TempDir()
	cache := filepath.Join(root, "cache")
	if err := os.Mkdir(cache, 0755); err != nil {
		t.Fatal(err)
	}
	d := goodDeps()
	d.SQLiteCheck = func(context.Context, string) error { return errors.New("missing") }
	got := diagnoseWith(context.Background(), config.Config{DataDirectory: filepath.Join(root, "missing"), ACMECachedir: cache}, false, d, doctorCredentials{})
	for _, n := range []string{"acme_cache", "data_directory", "init_state", "sqlite"} {
		for _, c := range got {
			if c.Name == n && c.Healthy {
				t.Fatal(c)
			}
		}
	}
}

func TestDiagnoseDiskWatermarkUsesConfiguredWarning(t *testing.T) {
	cfg, state := diagnosticFixture(t)
	cfg.Limits.DiskWarningPercent = 90
	d := goodDeps()
	d.DiskFreePercent = func(string) (uint64, error) { return 9, nil }
	got := diagnoseWith(context.Background(), cfg, false, d, doctorCredentials{}, state)
	for _, c := range got {
		if c.Name == "disk" && c.Healthy {
			t.Fatal("disk watermark passed")
		}
	}
}

func TestUpdateRollbackStateFailsClosed(t *testing.T) {
	cfg, state := diagnosticFixture(t)
	if err := os.Mkdir(filepath.Join(cfg.DataDirectory, "update-rollback"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg.DataDirectory, "update-rollback", "previous"), []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	got := diagnoseWith(context.Background(), cfg, false, goodDeps(), doctorCredentials{}, state)
	for _, c := range got {
		if c.Name == "update_rollback" && c.Healthy {
			t.Fatal("rollback pending reported healthy")
		}
	}
}

func TestCandidateDoctorOnlyAcceptsExpectedSecureRollbackSnapshot(t *testing.T) {
	cfg, state := diagnosticFixture(t)
	rollback := filepath.Join(cfg.DataDirectory, "update-rollback")
	checks := diagnoseWithOptions(context.Background(), cfg, true, goodDeps(), doctorCredentials{resendAPIKey: "key"}, true, state)
	for _, c := range checks {
		if c.Name == "update_rollback" && c.Healthy {
			t.Fatal("missing candidate rollback snapshot was accepted", c)
		}
	}
	if err := os.Mkdir(rollback, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rollback, "previous"), []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	checks = diagnoseWithOptions(context.Background(), cfg, true, goodDeps(), doctorCredentials{resendAPIKey: "key"}, true, state)
	for _, c := range checks {
		if c.Name == "update_rollback" && !c.Healthy {
			t.Fatal("candidate rollback snapshot was not accepted", c)
		}
	}
	if err := os.Chmod(filepath.Join(rollback, "previous"), 0644); err != nil {
		t.Fatal(err)
	}
	checks = diagnoseWithOptions(context.Background(), cfg, true, goodDeps(), doctorCredentials{resendAPIKey: "key"}, true, state)
	for _, c := range checks {
		if c.Name == "update_rollback" && c.Healthy {
			t.Fatal("unsafe rollback snapshot was accepted", c)
		}
	}
}

func TestCandidateDoctorDoesNotMaskOtherFailures(t *testing.T) {
	cfg, state := diagnosticFixture(t)
	rollback := filepath.Join(cfg.DataDirectory, "update-rollback")
	if err := os.Mkdir(rollback, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rollback, "previous"), []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	d := goodDeps()
	d.ServiceOK = func(context.Context) error { return errors.New("inactive") }
	checks := diagnoseWithOptions(context.Background(), cfg, true, d, doctorCredentials{resendAPIKey: "key"}, true, state)
	for _, c := range checks {
		if c.Name == "service" && c.Healthy {
			t.Fatal("candidate doctor masked service failure", c)
		}
	}
}
