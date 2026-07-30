package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tinyhost/tiny/internal/config"
)

func rootDoctorTest(t *testing.T) {
	t.Helper()
	oldUID, oldOwner := effectiveUID, credentialOwnerUID
	effectiveUID = func() int { return 0 }
	credentialOwnerUID = func(os.FileInfo) (uint32, bool) { return 0, true }
	t.Cleanup(func() {
		effectiveUID = oldUID
		credentialOwnerUID = oldOwner
	})
}

func doctorCredentialConfig() config.Config {
	return config.Config{ResendAPIKeyRef: "env:RESEND_API_KEY", HMACKeyRef: "env:TINYHOST_HMAC_KEY"}
}

func writeDoctorCredential(t *testing.T, body string, mode os.FileMode) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "tinyhost.env")
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
	return path
}

const validDoctorCredential = "RESEND_API_KEY=re_test_correct_key\nTINYHOST_HMAC_KEY=0123456789abcdef0123456789abcdef\n"

func TestDoctorReadsValidatedCredentialWithoutEnvironmentMutation(t *testing.T) {
	rootDoctorTest(t)
	cfg, state := diagnosticFixture(t)
	cfg.ResendAPIKeyRef = "env:RESEND_API_KEY"
	cfg.HMACKeyRef = "env:TINYHOST_HMAC_KEY"
	path := writeDoctorCredential(t, validDoctorCredential, 0600)
	t.Setenv("RESEND_API_KEY", "ambient-wrong-key")
	credentials, err := readDoctorCredentials(cfg, path)
	if err != nil {
		t.Fatal(err)
	}
	if credentials.resendAPIKey != "re_test_correct_key" {
		t.Fatal("doctor did not receive the credential-file key")
	}
	if got := os.Getenv("RESEND_API_KEY"); got != "ambient-wrong-key" {
		t.Fatalf("doctor changed ambient environment: %q", got)
	}
	d := goodDeps()
	d.ResendCheck = func(_ context.Context, key string) error {
		if key != "re_test_correct_key" {
			t.Fatalf("provider key = %q", key)
		}
		return nil
	}
	for _, check := range diagnoseWith(context.Background(), cfg, true, d, credentials, state) {
		if check.Name == "resend" && !check.Healthy {
			t.Fatal("correct credential failed Resend diagnostic")
		}
	}
}

func TestDoctorCredentialDenialsDoNotReachProvider(t *testing.T) {
	rootDoctorTest(t)
	cfg, state := diagnosticFixture(t)
	cfg.ResendAPIKeyRef = "env:RESEND_API_KEY"
	cfg.HMACKeyRef = "env:TINYHOST_HMAC_KEY"
	valid := validDoctorCredential
	for name, tc := range map[string]struct {
		body   string
		mode   os.FileMode
		owner  uint32
		link   bool
		absent bool
	}{
		"absent":     {mode: 0600, absent: true},
		"permissive": {body: valid, mode: 0640},
		"non_root":   {body: valid, mode: 0600, owner: 1000},
		"malformed":  {body: "RESEND_API_KEY\nTINYHOST_HMAC_KEY=0123456789abcdef0123456789abcdef\n", mode: 0600},
		"duplicate":  {body: "RESEND_API_KEY=one\nRESEND_API_KEY=two\nTINYHOST_HMAC_KEY=0123456789abcdef0123456789abcdef\n", mode: 0600},
		"unexpected": {body: "RESEND_API_KEY=one\nTINYHOST_HMAC_KEY=0123456789abcdef0123456789abcdef\nEXTRA=value\n", mode: 0600},
		"symlink":    {body: valid, mode: 0600, link: true},
	} {
		t.Run(name, func(t *testing.T) {
			oldOwner := credentialOwnerUID
			credentialOwnerUID = func(os.FileInfo) (uint32, bool) { return tc.owner, true }
			t.Cleanup(func() { credentialOwnerUID = oldOwner })
			path := filepath.Join(t.TempDir(), "absent.env")
			if !tc.absent {
				path = writeDoctorCredential(t, tc.body, tc.mode)
			}
			if tc.link {
				link := filepath.Join(filepath.Dir(path), "link.env")
				if err := os.Symlink(path, link); err != nil {
					t.Fatal(err)
				}
				path = link
			}
			credentials, err := readDoctorCredentials(cfg, path)
			if err == nil || credentials.resendAPIKey != "" {
				t.Fatalf("accepted invalid credential: %#v, %v", credentials, err)
			}
			if strings.Contains(err.Error(), "RESEND_API_KEY") || strings.Contains(err.Error(), "tinyhost.env") || strings.Contains(err.Error(), "one") {
				t.Fatalf("credential error leaked detail: %v", err)
			}
			d := goodDeps()
			d.ResendCheck = func(context.Context, string) error {
				t.Fatal("invalid credential reached provider")
				return nil
			}
			for _, check := range diagnoseWith(context.Background(), cfg, true, d, credentials, state) {
				if check.Name == "resend" && check.Healthy {
					t.Fatal("invalid credential reported healthy")
				}
			}
		})
	}
}

func TestDoctorCredentialReadRequiresRoot(t *testing.T) {
	oldUID := effectiveUID
	effectiveUID = func() int { return 1000 }
	t.Cleanup(func() { effectiveUID = oldUID })
	path := writeDoctorCredential(t, validDoctorCredential, 0600)
	if _, err := readDoctorCredentials(doctorCredentialConfig(), path); err == nil {
		t.Fatal("non-root credential read succeeded")
	}
}

func TestDoctorCommandRequiresRootBeforeCredentialRead(t *testing.T) {
	oldUID, oldLoader := effectiveUID, loadDoctorCredentials
	effectiveUID = func() int { return 1000 }
	loadDoctorCredentials = func(config.Config, string) (doctorCredentials, error) {
		t.Fatal("non-root doctor read credentials")
		return doctorCredentials{}, nil
	}
	t.Cleanup(func() {
		effectiveUID = oldUID
		loadDoctorCredentials = oldLoader
	})
	out, err := os.CreateTemp(t.TempDir(), "doctor-output")
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	if err := run([]string{"doctor", "--config", filepath.Join(t.TempDir(), "missing.yaml")}, out, out); err == nil || err.Error() != "tinyhost: root_required" {
		t.Fatalf("doctor error = %v", err)
	}
}

func TestStatusNeverLoadsDoctorCredentials(t *testing.T) {
	root := t.TempDir()
	cfg := config.Config{
		Domain: "apps.example.test", SessionCookie: "__Host-tiny_app",
		ListenHTTP: ":80", ListenHTTPS: ":443", DataDirectory: filepath.Join(root, "data"), ACMECachedir: filepath.Join(root, "acme"),
		ResendAPIKeyRef: "env:RESEND_API_KEY", HMACKeyRef: "env:TINYHOST_HMAC_KEY", EmailFrom: "sender@example.test", ACMEEmail: "operator@example.test",
		OTPExpiry: 10 * time.Minute, OTPMaxAttempts: 5, SessionExpiry: 24 * time.Hour,
	}
	if err := os.Mkdir(cfg.DataDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(cfg.ACMECachedir, 0700); err != nil {
		t.Fatal(err)
	}
	b, err := cfg.RenderYAML()
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "config.yaml")
	if err := os.WriteFile(configPath, b, 0600); err != nil {
		t.Fatal(err)
	}
	oldLoader := loadDoctorCredentials
	loadDoctorCredentials = func(config.Config, string) (doctorCredentials, error) {
		t.Fatal("status loaded provider credentials")
		return doctorCredentials{}, errors.New("unreachable")
	}
	t.Cleanup(func() { loadDoctorCredentials = oldLoader })
	out, err := os.CreateTemp(t.TempDir(), "status-output")
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	_ = run([]string{"status", "--config", configPath, "--credentials", filepath.Join(root, "must-not-read")}, out, out)
}
