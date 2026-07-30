package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func valid(root string) Config {
	return Config{Domain: ".TINY.TEST.", SessionCookie: "__Host-tiny_app", ListenHTTP: "127.0.0.1:80", ListenHTTPS: "127.0.0.1:443", DataDirectory: filepath.Join(root, "data"), ACMECachedir: filepath.Join(root, "cache"), EmailFrom: "a@test", ACMEEmail: "a@test", ResendAPIKeyRef: "env:R", HMACKeyRef: "env:H", OTPExpiry: time.Minute, SessionExpiry: time.Hour, OTPMaxAttempts: 5}
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
	text := "domain: TINY.TEST.\nsession_cookie: __Host-tiny_app\nlisten_http: 127.0.0.1:80\nlisten_https: 127.0.0.1:443\ndata_directory: " + filepath.Join(root, "data") + "\nsecret_refs:\n  hmac_key: env:H\nemail:\n  from: a@test\n  resend_api_key: env:R\nacme:\n  email: a@test\n  cache_directory: " + filepath.Join(root, "cache") + "\notp:\n  expiry: 1m\n  max_attempts: 5\nsession:\n  expiry: 1h\n"
	os.WriteFile(p, []byte(text), 0600)
	c, e := LoadYAML(p)
	if e != nil || c.Domain != "tiny.test" || c.PlatformHost() != "admin.tiny.test" || c.AppSuffix() != "tiny.test" {
		t.Fatal(c, e)
	}
}

func TestConfigRejectsSplitAndUnsafeDomains(t *testing.T) {
	root := t.TempDir()
	for _, domain := range []string{"", "localhost", "127.0.0.1", "*.tiny.test", "admin@tiny.test", "-tiny.test", "tiny..test", "tïny.test"} {
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
	for _, path := range []string{"/", "/var/lib/tiny/../escape", "/var/lib/tiny\nReadWritePaths=/"} {
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
