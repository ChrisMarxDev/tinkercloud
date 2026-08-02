package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func valid(root string) Config {
	return Config{Domain: ".TINKER.TEST.", SessionCookie: "__Host-tinker_app", ListenHTTP: "127.0.0.1:80", ListenHTTPS: "127.0.0.1:443", DataDirectory: filepath.Join(root, "data"), ACMECachedir: filepath.Join(root, "cache"), EmailFrom: "a@test", ACMEEmail: "a@test", EmailAPIKeyRef: "env:R", HMACKeyRef: "env:H", OTPExpiry: time.Minute, SessionExpiry: time.Hour, OTPMaxAttempts: 5}
}
func TestConfigValidationAndSecrets(t *testing.T) {
	c := valid(t.TempDir())
	if e := c.Validate(); e != nil {
		t.Fatal(e)
	}
	if _, e := c.ResolveSecrets(func(k string) string {
		if k == "H" {
			return strings.Repeat("x", 32)
		}
		return "r"
	}); e != nil {
		t.Fatal(e)
	}
	if _, e := c.ResolveSecrets(func(string) string { return "x" }); e == nil {
		t.Fatal("short hmac")
	}
	for _, v := range c.Redacted() {
		if strings.Contains(v, "env:") {
			t.Fatal(v)
		}
	}
}
func TestLoadCanonical(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "c.yaml")
	text := "domain: TINKER.TEST.\nsession_cookie: __Host-tinker_app\nlisten_http: 127.0.0.1:80\nlisten_https: 127.0.0.1:443\ndata_directory: " + filepath.Join(root, "data") + "\nsecret_refs:\n  hmac_key: env:H\nemail:\n  from: a@test\n  resend_api_key: env:R\nacme:\n  email: a@test\n  cache_directory: " + filepath.Join(root, "cache") + "\notp:\n  expiry: 1m\n  max_attempts: 5\nsession:\n  expiry: 1h\n"
	os.WriteFile(p, []byte(text), 0600)
	c, e := LoadYAML(p)
	if e != nil || c.Domain != "tinker.test" || c.PlatformHost() != "admin.tinker.test" || c.AppSuffix() != "tinker.test" {
		t.Fatal(c, e)
	}
}

func TestEmailProviderConfigRoundTripAndLegacyResendCompatibility(t *testing.T) {
	postmark := valid(t.TempDir())
	postmark.EmailProvider = EmailProviderPostmark
	postmark.EmailAPIKeyRef = "env:POSTMARK_SERVER_TOKEN"
	body, err := postmark.RenderYAML()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "provider: postmark") || !strings.Contains(string(body), "api_key: env:POSTMARK_SERVER_TOKEN") || strings.Contains(string(body), "resend_api_key") {
		t.Fatalf("rendered config = %s", body)
	}
	path := filepath.Join(t.TempDir(), "postmark.yaml")
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadYAML(path)
	if err != nil || loaded.EffectiveEmailProvider() != EmailProviderPostmark || loaded.EmailAPIKeyRef != "env:POSTMARK_SERVER_TOKEN" {
		t.Fatalf("loaded = %#v, %v", loaded, err)
	}

	legacyPath := filepath.Join(t.TempDir(), "legacy.yaml")
	legacy := strings.ReplaceAll(string(body), "  provider: postmark\n", "")
	legacy = strings.ReplaceAll(legacy, "  api_key: env:POSTMARK_SERVER_TOKEN", "  resend_api_key: env:RESEND_API_KEY")
	if err := os.WriteFile(legacyPath, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err = LoadYAML(legacyPath)
	if err != nil || loaded.EffectiveEmailProvider() != EmailProviderResend || loaded.EmailAPIKeyRef != "env:RESEND_API_KEY" {
		t.Fatalf("legacy loaded = %#v, %v", loaded, err)
	}
}

func TestEmailProviderConfigDenials(t *testing.T) {
	base := valid(t.TempDir())
	body, err := base.RenderYAML()
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(string) string{
		"unknown":      func(raw string) string { return strings.Replace(raw, "provider: resend", "provider: unknown", 1) },
		"case variant": func(raw string) string { return strings.Replace(raw, "provider: resend", "provider: Resend", 1) },
		"raw key":      func(raw string) string { return strings.Replace(raw, "api_key: env:R", "api_key: secret", 1) },
		"both key fields": func(raw string) string {
			return strings.Replace(raw, "  api_key: env:R", "  api_key: env:R\n  resend_api_key: env:R2", 1)
		},
		"postmark legacy key": func(raw string) string {
			raw = strings.Replace(raw, "provider: resend", "provider: postmark", 1)
			return strings.Replace(raw, "  api_key: env:R", "  resend_api_key: env:R", 1)
		},
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(mutate(string(body))), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadYAML(path); err == nil {
				t.Fatal("invalid email provider config accepted")
			}
		})
	}
}

func TestEmailProviderEnvironmentCascade(t *testing.T) {
	cfg := valid(t.TempDir())
	values := map[string]string{
		"RESEND_API_KEY":            "resend-key",
		"POSTMARK_SERVER_TOKEN":     "postmark-key",
		"SENDGRID_API_KEY":          "sendgrid-key",
		"TINKERCLOUD_SMTP_HOST":     "smtp.example.test",
		"TINKERCLOUD_SMTP_PORT":     "587",
		"TINKERCLOUD_SMTP_USERNAME": "operator@example.test",
		"TINKERCLOUD_SMTP_PASSWORD": "smtp-password",
		"TINKERCLOUD_SMTP_TLS":      "starttls",
	}
	get := func(name string) string { return values[name] }
	credentials, err := cfg.ResolveEmailCredentials(get)
	if err != nil || credentials.Provider != EmailProviderResend || credentials.APIKey != "resend-key" {
		t.Fatalf("Resend priority = %#v, %v", credentials, err)
	}
	delete(values, "RESEND_API_KEY")
	credentials, err = cfg.ResolveEmailCredentials(get)
	if err != nil || credentials.Provider != EmailProviderPostmark || credentials.APIKey != "postmark-key" {
		t.Fatalf("Postmark priority = %#v, %v", credentials, err)
	}
	delete(values, "POSTMARK_SERVER_TOKEN")
	credentials, err = cfg.ResolveEmailCredentials(get)
	if err != nil || credentials.Provider != EmailProviderSendGrid || credentials.APIKey != "sendgrid-key" {
		t.Fatalf("SendGrid priority = %#v, %v", credentials, err)
	}
	delete(values, "SENDGRID_API_KEY")
	credentials, err = cfg.ResolveEmailCredentials(get)
	if err != nil || credentials.Provider != EmailProviderSMTP || credentials.SMTP.Host != "smtp.example.test" || credentials.SMTP.Port != 587 {
		t.Fatalf("SMTP fallback = %#v, %v", credentials, err)
	}
}

func TestEmailProviderEnvironmentDenials(t *testing.T) {
	cfg := valid(t.TempDir())
	cfg.EmailAPIKeyRef = ""
	for name, values := range map[string]map[string]string{
		"absent":       {},
		"partial smtp": {"TINKERCLOUD_SMTP_HOST": "smtp.example.test"},
		"plaintext smtp": {
			"TINKERCLOUD_SMTP_HOST": "smtp.example.test", "TINKERCLOUD_SMTP_PORT": "25",
			"TINKERCLOUD_SMTP_USERNAME": "operator@example.test", "TINKERCLOUD_SMTP_PASSWORD": "secret",
			"TINKERCLOUD_SMTP_TLS": "none",
		},
		"invalid smtp port": {
			"TINKERCLOUD_SMTP_HOST": "smtp.example.test", "TINKERCLOUD_SMTP_PORT": "70000",
			"TINKERCLOUD_SMTP_USERNAME": "operator@example.test", "TINKERCLOUD_SMTP_PASSWORD": "secret",
			"TINKERCLOUD_SMTP_TLS": "tls",
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := cfg.ResolveEmailCredentials(func(key string) string { return values[key] }); err == nil {
				t.Fatal("invalid provider environment accepted")
			}
		})
	}
}

func TestConfigRejectsSplitAndUnsafeDomains(t *testing.T) {
	root := t.TempDir()
	for _, domain := range []string{"", "localhost", "127.0.0.1", "*.tinker.test", "admin@tinker.test", "-tinker.test", "tinker..test", "tïny.test"} {
		c := valid(root)
		c.Domain = domain
		if c.Validate() == nil {
			t.Fatalf("unsafe domain accepted: %q", domain)
		}
	}
}

func TestRenderYAMLContainsReferencesNotSecrets(t *testing.T) {
	c := valid(t.TempDir())
	b, err := c.RenderYAML()
	if err != nil || !strings.Contains(string(b), "hmac_key: env:H") || strings.Contains(string(b), "012345") {
		t.Fatal(string(b), err)
	}
}

func TestConfigRejectsUnsafePrivateRoots(t *testing.T) {
	for _, path := range []string{"/", "/var/lib/tinker/../escape", "/var/lib/tinker\nReadWritePaths=/"} {
		c := valid(t.TempDir())
		c.DataDirectory = path
		if err := c.Validate(); err == nil {
			t.Fatal("unsafe data directory accepted", path)
		}
	}
}

func TestConfigRejectsNonGatewayPorts(t *testing.T) {
	for _, mutate := range []func(*Config){
		func(c *Config) { c.ListenHTTP = "0.0.0.0:8080" },
		func(c *Config) { c.ListenHTTPS = "0.0.0.0:8443" },
		func(c *Config) {
			c.ListenHTTP = "127.0.0.1:443"
			c.ListenHTTPS = "127.0.0.1:80"
		},
	} {
		c := valid(t.TempDir())
		mutate(&c)
		if err := c.Validate(); err == nil {
			t.Fatal("non-gateway listener ports accepted")
		}
	}
}

func TestConfigUpdateReleaseBaseMustBeHTTPSDirectory(t *testing.T) {
	c := valid(t.TempDir())
	for _, raw := range []string{"http://releases.example/r/", "https://releases.example/r", "https://user@releases.example/r/", "https://releases.example/r/?x=1"} {
		c.UpdateReleaseBase = raw
		if c.Validate() == nil {
			t.Fatal("unsafe update release base accepted", raw)
		}
	}
	c.UpdateReleaseBase = "https://releases.example/r/"
	if c.Validate() != nil {
		t.Fatal("safe release base rejected")
	}
}

func TestResourceLimitsDefaultRoundTripAndBounds(t *testing.T) {
	c := valid(t.TempDir())
	want := DefaultResourceLimits()
	if got := c.EffectiveLimits(); got != want {
		t.Fatalf("defaults = %#v, want %#v", got, want)
	}
	b, err := c.RenderYAML()
	if err != nil || !strings.Contains(string(b), "archive_upload_bytes: 104857600") {
		t.Fatalf("rendered limits missing: %s (%v)", b, err)
	}
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, b, 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadYAML(p)
	if err != nil || loaded.EffectiveLimits() != want {
		t.Fatalf("round trip = %#v, %v", loaded.EffectiveLimits(), err)
	}
	for _, bad := range []ResourceLimits{
		{AppsPerDeployer: -1},
		{ArchiveUploadBytes: 1},
		{ExpandedReleaseBytes: 1 << 20},
		{DiskWarningPercent: 95, DiskStopPercent: 90},
	} {
		c.Limits = bad
		if c.Validate() == nil {
			t.Fatalf("accepted unsafe limits: %#v", bad)
		}
	}
}

func TestRealtimeLimitsDefaultRoundTripAndBounds(t *testing.T) {
	c := valid(t.TempDir())
	want := DefaultRealtimeLimits()
	if got := c.EffectiveRealtimeLimits(); got != want {
		t.Fatalf("defaults = %#v, want %#v", got, want)
	}
	b, err := c.RenderYAML()
	if err != nil || !strings.Contains(string(b), "ping_interval: 20s") {
		t.Fatalf("rendered realtime limits missing: %s (%v)", b, err)
	}
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, b, 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadYAML(p)
	if err != nil || loaded.EffectiveRealtimeLimits() != want {
		t.Fatalf("round trip = %#v, %v", loaded.EffectiveRealtimeLimits(), err)
	}
	for _, bad := range []RealtimeLimits{
		{IdleTimeout: -time.Second},
		{IdleTimeout: time.Second, PingInterval: time.Second, PongTimeout: time.Second},
		{WriteTimeout: time.Minute},
		{OutboundQueue: -1},
	} {
		c.Realtime = bad
		if c.Validate() == nil {
			t.Fatalf("accepted unsafe realtime limits: %#v", bad)
		}
	}
}
