package config

import (
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// Config contains only non-secret M1 gateway settings.
type Config struct {
	Domain                                    string
	SessionCookie                             string
	ListenHTTP, ListenHTTPS, DataDirectory    string
	EmailAPIKeyRef, HMACKeyRef, LLMRootKeyRef string
	EmailProvider                             EmailProvider
	EmailFrom, ACMEEmail, ACMECachedir        string
	OTPExpiry, SessionExpiry                  time.Duration
	OTPMaxAttempts                            int
	Limits                                    ResourceLimits
	Realtime                                  RealtimeLimits
	UpdateReleaseBase                         string
}

type EmailProvider string

const (
	EmailProviderResend   EmailProvider = "resend"
	EmailProviderPostmark EmailProvider = "postmark"
)

func ParseEmailProvider(raw string) (EmailProvider, error) {
	provider := EmailProvider(raw)
	switch provider {
	case EmailProviderResend, EmailProviderPostmark:
		return provider, nil
	default:
		return "", fmt.Errorf("unsupported email provider")
	}
}

func (c Config) EffectiveEmailProvider() EmailProvider {
	if c.EmailProvider == "" {
		return EmailProviderResend
	}
	return c.EmailProvider
}

func (c Config) PlatformHost() string {
	domain := normalizeDomain(c.Domain)
	if domain == "" {
		return ""
	}
	return "admin." + domain
}

func (c Config) AppSuffix() string { return normalizeDomain(c.Domain) }

type Secrets struct {
	EmailAPIKey, HMACKey string
	LLMRootKey           []byte
}

// ResourceLimits are the intentionally small V1 growth controls. Zero-valued
// limits are normalized to Defaults so upgrading an existing installation does
// not silently become unlimited.
type ResourceLimits struct {
	AppsPerDeployer           int           `yaml:"apps_per_deployer"`
	ArchiveUploadBytes        int64         `yaml:"archive_upload_bytes"`
	ExpandedReleaseBytes      int64         `yaml:"expanded_release_bytes"`
	FilesPerRelease           int           `yaml:"files_per_release"`
	SingleFileBytes           int64         `yaml:"single_file_bytes"`
	DeploymentAttemptsPerHour int           `yaml:"deployment_attempts_per_hour"`
	ReleaseRetention          int           `yaml:"release_retention"`
	BlobBytes                 int64         `yaml:"blob_bytes"`
	BlobsPerApp               int           `yaml:"blobs_per_app"`
	TotalBlobBytesPerApp      int64         `yaml:"total_blob_bytes_per_app"`
	BlobListLimit             int           `yaml:"blob_list_limit"`
	BlobUploadsPerMinute      int           `yaml:"blob_uploads_per_minute"`
	BlobConcurrentUploads     int           `yaml:"blob_concurrent_uploads"`
	BlobUploadDuration        time.Duration `yaml:"blob_upload_duration"`
	DiskWarningPercent        uint64        `yaml:"disk_warning_percent"`
	DiskStopPercent           uint64        `yaml:"disk_stop_percent"`
}

// RealtimeLimits bound each authenticated WebSocket's liveness and resource
// use. Zero values are normalized to V1 defaults so an older config cannot
// accidentally create an unbounded connection after an upgrade.
type RealtimeLimits struct {
	IdleTimeout   time.Duration `yaml:"idle_timeout"`
	PingInterval  time.Duration `yaml:"ping_interval"`
	PongTimeout   time.Duration `yaml:"pong_timeout"`
	WriteTimeout  time.Duration `yaml:"write_timeout"`
	OutboundQueue int           `yaml:"outbound_queue"`
}

func DefaultRealtimeLimits() RealtimeLimits {
	return RealtimeLimits{IdleTimeout: time.Minute, PingInterval: 20 * time.Second, PongTimeout: 10 * time.Second, WriteTimeout: 3 * time.Second, OutboundQueue: 16}
}

func (c Config) EffectiveRealtimeLimits() RealtimeLimits {
	d, v := DefaultRealtimeLimits(), c.Realtime
	if v.IdleTimeout != 0 {
		d.IdleTimeout = v.IdleTimeout
	}
	if v.PingInterval != 0 {
		d.PingInterval = v.PingInterval
	}
	if v.PongTimeout != 0 {
		d.PongTimeout = v.PongTimeout
	}
	if v.WriteTimeout != 0 {
		d.WriteTimeout = v.WriteTimeout
	}
	if v.OutboundQueue != 0 {
		d.OutboundQueue = v.OutboundQueue
	}
	return d
}

func (l RealtimeLimits) Validate() error {
	if l.IdleTimeout <= 0 || l.IdleTimeout > time.Hour ||
		l.PingInterval <= 0 || l.PingInterval > l.IdleTimeout ||
		l.PongTimeout <= 0 || l.PongTimeout > l.IdleTimeout ||
		l.PingInterval+l.PongTimeout > l.IdleTimeout ||
		l.WriteTimeout <= 0 || l.WriteTimeout > 30*time.Second ||
		l.OutboundQueue < 1 || l.OutboundQueue > 1000 {
		return fmt.Errorf("unsafe realtime limits")
	}
	return nil
}

func DefaultResourceLimits() ResourceLimits {
	return ResourceLimits{AppsPerDeployer: 20, ArchiveUploadBytes: 100 << 20, ExpandedReleaseBytes: 250 << 20, FilesPerRelease: 10000, SingleFileBytes: 50 << 20, DeploymentAttemptsPerHour: 20, ReleaseRetention: 10, BlobBytes: 25000000, BlobsPerApp: 1000, TotalBlobBytesPerApp: 250000000, BlobListLimit: 100, BlobUploadsPerMinute: 20, BlobConcurrentUploads: 2, BlobUploadDuration: 2 * time.Minute, DiskWarningPercent: 80, DiskStopPercent: 90}
}

func (c Config) EffectiveLimits() ResourceLimits {
	d, v := DefaultResourceLimits(), c.Limits
	if v.AppsPerDeployer != 0 {
		d.AppsPerDeployer = v.AppsPerDeployer
	}
	if v.ArchiveUploadBytes != 0 {
		d.ArchiveUploadBytes = v.ArchiveUploadBytes
	}
	if v.ExpandedReleaseBytes != 0 {
		d.ExpandedReleaseBytes = v.ExpandedReleaseBytes
	}
	if v.FilesPerRelease != 0 {
		d.FilesPerRelease = v.FilesPerRelease
	}
	if v.SingleFileBytes != 0 {
		d.SingleFileBytes = v.SingleFileBytes
	}
	if v.DeploymentAttemptsPerHour != 0 {
		d.DeploymentAttemptsPerHour = v.DeploymentAttemptsPerHour
	}
	if v.ReleaseRetention != 0 {
		d.ReleaseRetention = v.ReleaseRetention
	}
	if v.BlobBytes != 0 {
		d.BlobBytes = v.BlobBytes
	}
	if v.BlobsPerApp != 0 {
		d.BlobsPerApp = v.BlobsPerApp
	}
	if v.TotalBlobBytesPerApp != 0 {
		d.TotalBlobBytesPerApp = v.TotalBlobBytesPerApp
	}
	if v.BlobListLimit != 0 {
		d.BlobListLimit = v.BlobListLimit
	}
	if v.BlobUploadsPerMinute != 0 {
		d.BlobUploadsPerMinute = v.BlobUploadsPerMinute
	}
	if v.BlobConcurrentUploads != 0 {
		d.BlobConcurrentUploads = v.BlobConcurrentUploads
	}
	if v.BlobUploadDuration != 0 {
		d.BlobUploadDuration = v.BlobUploadDuration
	}
	if v.DiskWarningPercent != 0 {
		d.DiskWarningPercent = v.DiskWarningPercent
	}
	if v.DiskStopPercent != 0 {
		d.DiskStopPercent = v.DiskStopPercent
	}
	return d
}

func (l ResourceLimits) Validate() error {
	// Upper bounds are deliberately conservative for a single-VPS V1. They
	// prevent malformed config from weakening the resource boundary by orders
	// of magnitude while allowing sensible small-server tuning.
	if l.AppsPerDeployer < 1 || l.AppsPerDeployer > 1000 ||
		l.ArchiveUploadBytes < 1<<20 || l.ArchiveUploadBytes > 1<<30 ||
		l.ExpandedReleaseBytes < l.ArchiveUploadBytes || l.ExpandedReleaseBytes > 4<<30 ||
		l.FilesPerRelease < 1 || l.FilesPerRelease > 100000 ||
		l.SingleFileBytes < 1<<10 || l.SingleFileBytes > l.ExpandedReleaseBytes ||
		l.DeploymentAttemptsPerHour < 1 || l.DeploymentAttemptsPerHour > 1000 ||
		l.ReleaseRetention < 1 || l.ReleaseRetention > 1000 ||
		l.BlobBytes < 1<<10 || l.BlobBytes > 1<<30 ||
		l.BlobsPerApp < 1 || l.BlobsPerApp > 100000 ||
		l.TotalBlobBytesPerApp < l.BlobBytes || l.TotalBlobBytesPerApp > 4<<30 ||
		l.BlobListLimit < 1 || l.BlobListLimit > 1000 ||
		l.BlobUploadsPerMinute < 1 || l.BlobUploadsPerMinute > 10000 ||
		l.BlobConcurrentUploads < 1 || l.BlobConcurrentUploads > 100 ||
		l.BlobUploadDuration < time.Second || l.BlobUploadDuration > time.Hour ||
		l.DiskWarningPercent < 1 || l.DiskWarningPercent >= l.DiskStopPercent || l.DiskStopPercent > 100 {
		return fmt.Errorf("unsafe resource limits")
	}
	return nil
}

// RenderYAML is the only writer for the deliberately narrow server config.
// It emits references to the systemd-owned environment file, never secret
// values. Inputs have already passed Validate, which excludes YAML control
// characters from the host fields used here.
func (c Config) RenderYAML() ([]byte, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	l := c.EffectiveLimits()
	r := c.EffectiveRealtimeLimits()
	secretRefs := "secret_refs:\n  hmac_key: " + c.HMACKeyRef + "\n"
	if c.LLMRootKeyRef != "" {
		secretRefs += "  llm_root_key: " + c.LLMRootKeyRef + "\n"
	}
	return []byte(fmt.Sprintf("domain: %s\nsession_cookie: %s\nlisten_http: %s\nlisten_https: %s\ndata_directory: %s\n%semail:\n  provider: %s\n  from: %s\n  api_key: %s\nacme:\n  email: %s\n  cache_directory: %s\nupdates:\n  release_base: %s\notp:\n  expiry: %s\n  max_attempts: %d\nsession:\n  expiry: %s\nrealtime:\n  idle_timeout: %s\n  ping_interval: %s\n  pong_timeout: %s\n  write_timeout: %s\n  outbound_queue: %d\nlimits:\n  apps_per_deployer: %d\n  archive_upload_bytes: %d\n  expanded_release_bytes: %d\n  files_per_release: %d\n  single_file_bytes: %d\n  deployment_attempts_per_hour: %d\n  release_retention: %d\n  blob_bytes: %d\n  blobs_per_app: %d\n  total_blob_bytes_per_app: %d\n  blob_list_limit: %d\n  blob_uploads_per_minute: %d\n  blob_concurrent_uploads: %d\n  blob_upload_duration: %s\n  disk_warning_percent: %d\n  disk_stop_percent: %d\n", c.AppSuffix(), c.SessionCookie, c.ListenHTTP, c.ListenHTTPS, c.DataDirectory, secretRefs, c.EffectiveEmailProvider(), c.EmailFrom, c.EmailAPIKeyRef, c.ACMEEmail, c.ACMECachedir, c.UpdateReleaseBase, c.OTPExpiry, c.OTPMaxAttempts, c.SessionExpiry, r.IdleTimeout, r.PingInterval, r.PongTimeout, r.WriteTimeout, r.OutboundQueue, l.AppsPerDeployer, l.ArchiveUploadBytes, l.ExpandedReleaseBytes, l.FilesPerRelease, l.SingleFileBytes, l.DeploymentAttemptsPerHour, l.ReleaseRetention, l.BlobBytes, l.BlobsPerApp, l.TotalBlobBytesPerApp, l.BlobListLimit, l.BlobUploadsPerMinute, l.BlobConcurrentUploads, l.BlobUploadDuration, l.DiskWarningPercent, l.DiskStopPercent)), nil
}

func (c Config) Redacted() map[string]string {
	return map[string]string{"domain": c.AppSuffix(), "admin_host": c.PlatformHost(), "email_api_key": "[redacted]", "hmac_key": "[redacted]", "llm_root_key": "[redacted]"}
}
func (c Config) ResolveSecrets(get func(string) string) (Secrets, error) {
	if c.EmailAPIKeyRef == "" || c.HMACKeyRef == "" {
		return Secrets{}, fmt.Errorf("missing secret reference")
	}
	s := Secrets{}
	if c.LLMRootKeyRef != "" {
		raw := get(strings.TrimPrefix(c.LLMRootKeyRef, "env:"))
		decoded, err := hex.DecodeString(raw)
		if err != nil || len(decoded) != 32 {
			return Secrets{}, fmt.Errorf("invalid llm root secret")
		}
		s.LLMRootKey = decoded
	}
	for _, x := range []struct {
		ref string
		dst *string
	}{{c.EmailAPIKeyRef, &s.EmailAPIKey}, {c.HMACKeyRef, &s.HMACKey}} {
		if x.ref != "" {
			*x.dst = get(strings.TrimPrefix(x.ref, "env:"))
			if *x.dst == "" {
				return Secrets{}, fmt.Errorf("missing secret environment reference")
			}
		}
	}
	if len(s.HMACKey) < 32 {
		return Secrets{}, fmt.Errorf("invalid hmac secret")
	}
	return s, nil
}

// LoadYAML accepts a deliberately narrow server config. Secrets are references,
// never raw values, so diagnostics and browser code cannot expose them.
func LoadYAML(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer f.Close()
	var raw struct {
		Domain        string            `yaml:"domain"`
		SessionCookie string            `yaml:"session_cookie"`
		SecretRefs    map[string]string `yaml:"secret_refs"`
		ListenHTTP    string            `yaml:"listen_http"`
		ListenHTTPS   string            `yaml:"listen_https"`
		DataDirectory string            `yaml:"data_directory"`
		Email         struct {
			Provider     EmailProvider `yaml:"provider"`
			From         string        `yaml:"from"`
			APIKey       string        `yaml:"api_key"`
			ResendAPIKey string        `yaml:"resend_api_key"`
		} `yaml:"email"`
		ACME struct {
			Email          string `yaml:"email"`
			CacheDirectory string `yaml:"cache_directory"`
		} `yaml:"acme"`
		Updates struct {
			ReleaseBase string `yaml:"release_base"`
		} `yaml:"updates"`
		OTP struct {
			Expiry      time.Duration `yaml:"expiry"`
			MaxAttempts int           `yaml:"max_attempts"`
		} `yaml:"otp"`
		Session struct {
			Expiry time.Duration `yaml:"expiry"`
		} `yaml:"session"`
		Realtime RealtimeLimits `yaml:"realtime"`
		Limits   ResourceLimits `yaml:"limits"`
	}
	d := yaml.NewDecoder(f)
	d.KnownFields(true)
	if err := d.Decode(&raw); err != nil {
		return Config{}, fmt.Errorf("invalid config: %w", err)
	}
	if err := d.Decode(&struct{}{}); err != io.EOF {
		return Config{}, fmt.Errorf("invalid config: multiple documents")
	}
	provider := raw.Email.Provider
	if provider == "" {
		provider = EmailProviderResend
	}
	apiKeyRef := raw.Email.APIKey
	if raw.Email.ResendAPIKey != "" {
		if apiKeyRef != "" || provider != EmailProviderResend {
			return Config{}, fmt.Errorf("invalid config: ambiguous email credential")
		}
		apiKeyRef = raw.Email.ResendAPIKey
	}
	c := Config{Domain: raw.Domain, SessionCookie: raw.SessionCookie, ListenHTTP: raw.ListenHTTP, ListenHTTPS: raw.ListenHTTPS, DataDirectory: raw.DataDirectory, EmailProvider: provider, EmailAPIKeyRef: apiKeyRef, HMACKeyRef: raw.SecretRefs["hmac_key"], LLMRootKeyRef: raw.SecretRefs["llm_root_key"], EmailFrom: raw.Email.From, ACMEEmail: raw.ACME.Email, ACMECachedir: raw.ACME.CacheDirectory, UpdateReleaseBase: raw.Updates.ReleaseBase, OTPExpiry: raw.OTP.Expiry, OTPMaxAttempts: raw.OTP.MaxAttempts, SessionExpiry: raw.Session.Expiry, Realtime: raw.Realtime, Limits: raw.Limits}
	c.Domain = normalizeDomain(c.Domain)
	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	for name, ref := range raw.SecretRefs {
		if strings.TrimSpace(name) == "" || !strings.HasPrefix(ref, "env:") {
			return Config{}, fmt.Errorf("secret reference %q must use env prefix", name)
		}
	}
	if c.EmailAPIKeyRef != "" && !strings.HasPrefix(c.EmailAPIKeyRef, "env:") {
		return Config{}, fmt.Errorf("email api_key must be env reference")
	}
	return c, nil
}

func (c Config) Validate() error {
	c.Domain = normalizeDomain(c.Domain)
	if !validDomain(c.Domain) || c.SessionCookie == "" {
		return fmt.Errorf("domain and session cookie are required")
	}
	if c.SessionCookie != "__Host-tinker_app" {
		return fmt.Errorf("app session cookie must use the __Host- contract")
	}
	if c.UpdateReleaseBase != "" {
		u, err := url.Parse(c.UpdateReleaseBase)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Port() != "" || !strings.HasSuffix(u.Path, "/") {
			return fmt.Errorf("invalid update release base")
		}
	}
	if strings.ContainsAny(c.EmailFrom+c.ACMEEmail+c.ListenHTTP+c.ListenHTTPS, "\r\n") {
		return fmt.Errorf("config values cannot contain control newlines")
	}
	if _, err := ParseEmailProvider(string(c.EffectiveEmailProvider())); err != nil {
		return err
	}
	if c.EmailFrom == "" || c.ACMEEmail == "" || c.HMACKeyRef == "" || c.EmailAPIKeyRef == "" {
		return fmt.Errorf("email from and hmac reference are required")
	}
	if !safePrivateRoot(c.DataDirectory) {
		return fmt.Errorf("data directory must be absolute")
	}
	if !safePrivateRoot(c.ACMECachedir) || filepath.Clean(c.ACMECachedir) == filepath.Clean(c.DataDirectory) {
		return fmt.Errorf("acme cache directory must be distinct absolute")
	}
	if c.ListenHTTP == c.ListenHTTPS {
		return fmt.Errorf("listener addresses must differ")
	}
	if _, port, e := net.SplitHostPort(c.ListenHTTP); e != nil || c.ListenHTTP == "" || port != "80" {
		return fmt.Errorf("invalid http listen address")
	}
	if _, port, e := net.SplitHostPort(c.ListenHTTPS); e != nil || c.ListenHTTPS == "" || port != "443" {
		return fmt.Errorf("invalid https listen address")
	}
	if c.OTPExpiry <= 0 || c.OTPExpiry > 24*time.Hour || c.SessionExpiry <= 0 || c.SessionExpiry > 7*24*time.Hour || c.OTPMaxAttempts < 1 || c.OTPMaxAttempts > 10 {
		return fmt.Errorf("unsafe auth durations")
	}
	if err := c.EffectiveLimits().Validate(); err != nil {
		return err
	}
	if err := c.EffectiveRealtimeLimits().Validate(); err != nil {
		return err
	}
	return nil
}

func normalizeDomain(raw string) string {
	return strings.TrimPrefix(strings.TrimSuffix(strings.ToLower(strings.TrimSpace(raw)), "."), ".")
}

func validDomain(domain string) bool {
	if domain == "" || len(domain) > 253 || net.ParseIP(domain) != nil || !strings.Contains(domain, ".") || strings.ContainsAny(domain, "/@:* \t\r\n") {
		return false
	}
	for _, label := range strings.Split(domain, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, char := range label {
			if !(char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '-') {
				return false
			}
		}
	}
	return true
}

func safePrivateRoot(path string) bool {
	return filepath.IsAbs(path) &&
		path != string(filepath.Separator) &&
		filepath.Clean(path) == path &&
		!strings.ContainsAny(path, "\r\n\x00")
}
