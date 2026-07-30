package main

import (
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/tinyhost/tiny/internal/config"
	"github.com/tinyhost/tiny/internal/operations"
)

// buildVersion is set by the release build. It intentionally contains no host
// or release URL so status output remains safe to paste into support tickets.
var buildVersion = "dev"

const diagnosticTimeout = 5 * time.Second
const maxDoctorCredentialBytes = 64 << 10

// doctorCredentials is intentionally local to diagnostic composition. It keeps
// a provider value out of the process environment and gives no output path a
// printable secret reference.
type doctorCredentials struct{ resendAPIKey string }

var credentialOwnerUID = func(info os.FileInfo) (uint32, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	return st.Uid, ok
}

// diagnosticDeps isolates host and network effects. Doctor must be usable on a
// broken host, so every dependency has a bounded, fail-closed default and tests
// can prove failure and redaction paths without contacting a provider.
type diagnosticDeps struct {
	DiskFreePercent func(string) (uint64, error)
	SQLiteCheck     func(context.Context, string) error
	ClockOK         func(context.Context) error
	ServiceOK       func(context.Context) error
	PortOK          func(context.Context, string) error
	DNSLookup       func(context.Context, string) ([]string, error)
	TLSCheck        func(context.Context, string, string) error
	ResendCheck     func(context.Context, string) error
}

// diskFreePercent is retained as a focused test seam for the local watermark.
var diskFreePercent = func(path string) (uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, err
	}
	if st.Blocks == 0 {
		return 0, syscall.EINVAL
	}
	return uint64(st.Bavail) * 100 / uint64(st.Blocks), nil
}

func productionDiagnosticDeps() diagnosticDeps {
	return diagnosticDeps{
		DiskFreePercent: diskFreePercent,
		SQLiteCheck:     sqliteQuickCheck,
		ClockOK: func(ctx context.Context) error {
			out, err := exec.CommandContext(ctx, "timedatectl", "show", "--property=NTPSynchronized", "--value").Output()
			if err != nil || strings.TrimSpace(string(out)) != "yes" {
				return errors.New("clock unsynchronized")
			}
			return nil
		},
		ServiceOK: func(ctx context.Context) error {
			return exec.CommandContext(ctx, "systemctl", "is-active", "--quiet", "tinyhost.service").Run()
		},
		PortOK: localPortOK,
		DNSLookup: func(ctx context.Context, host string) ([]string, error) {
			return net.DefaultResolver.LookupHost(ctx, host)
		},
		TLSCheck: tlsCertificateOK,
		ResendCheck: func(ctx context.Context, key string) error {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.resend.com/domains", nil)
			if err != nil {
				return err
			}
			req.Header.Set("Authorization", "Bearer "+key)
			client := &http.Client{Timeout: diagnosticTimeout}
			resp, err := client.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			// Deliberately discard provider bodies: they may contain account data.
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				return fmt.Errorf("resend status %d", resp.StatusCode)
			}
			return nil
		},
	}
}

func sqliteQuickCheck(ctx context.Context, dataDir string) error {
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dataDir, "tinyhost.db")+"?mode=ro")
	if err != nil {
		return err
	}
	defer db.Close()
	if err = db.PingContext(ctx); err != nil {
		return err
	}
	var result string
	if err = db.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&result); err != nil || result != "ok" {
		return errors.New("sqlite integrity unavailable")
	}
	return nil
}

func localPortOK(ctx context.Context, addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		return errors.New("invalid listener")
	}
	if host == "" || host == "0.0.0.0" || host == "::" || host == "[::]" {
		host = "127.0.0.1"
	}
	d := net.Dialer{Timeout: diagnosticTimeout}
	c, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		return err
	}
	return c.Close()
}

func tlsCertificateOK(ctx context.Context, host, port string) error {
	d := tls.Dialer{NetDialer: &net.Dialer{Timeout: diagnosticTimeout}, Config: &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}}
	c, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		return err
	}
	defer c.Close()
	tlsConn, ok := c.(*tls.Conn)
	if !ok {
		return errors.New("unexpected tls connection")
	}
	state := tlsConn.ConnectionState()
	if len(state.VerifiedChains) == 0 || len(state.PeerCertificates) == 0 {
		return errors.New("unverified certificate")
	}
	cert := state.PeerCertificates[0]
	if err := cert.VerifyHostname(host); err != nil || time.Until(cert.NotAfter) < 24*time.Hour || !cert.NotAfter.After(cert.NotBefore) {
		return errors.New("certificate invalid")
	}
	return nil
}

// diagnose is the local-only status contract. It neither resolves DNS nor
// loads provider credentials, making it safe for an offline recovery shell.
func diagnose(c config.Config, initStatePath ...string) []operations.Check {
	return diagnoseWith(context.Background(), c, false, productionDiagnosticDeps(), doctorCredentials{}, initStatePath...)
}

func doctor(c config.Config, credentials doctorCredentials, initStatePath ...string) []operations.Check {
	return diagnoseWith(context.Background(), c, true, productionDiagnosticDeps(), credentials, initStatePath...)
}

// candidateDoctor is only for the updater's replacement-process health gate.
// It keeps every doctor check, but regards the canonical, private rollback
// snapshot as expected until the updater commits it away.
func candidateDoctor(c config.Config, credentials doctorCredentials, initStatePath ...string) []operations.Check {
	return diagnoseWithOptions(context.Background(), c, true, productionDiagnosticDeps(), credentials, true, initStatePath...)
}

func diagnoseWith(parent context.Context, c config.Config, network bool, d diagnosticDeps, credentials doctorCredentials, initStatePath ...string) []operations.Check {
	return diagnoseWithOptions(parent, c, network, d, credentials, false, initStatePath...)
}

func diagnoseWithOptions(parent context.Context, c config.Config, network bool, d diagnosticDeps, credentials doctorCredentials, expectedRollbackSnapshot bool, initStatePath ...string) []operations.Check {
	checks := []operations.Check{{Name: "config", Healthy: true, Detail: "loaded"}, {Name: "version", Healthy: strings.TrimSpace(buildVersion) != "", Detail: strings.TrimSpace(buildVersion)}}
	for _, v := range []struct{ name, path string }{{"data_directory", c.DataDirectory}, {"acme_cache", c.ACMECachedir}} {
		st, err := os.Lstat(v.path)
		ok := err == nil && st.IsDir() && st.Mode()&os.ModeSymlink == 0 && st.Mode().Perm()&0077 == 0
		checks = append(checks, operations.Check{Name: v.name, Healthy: ok, Detail: map[bool]string{true: "private", false: "unavailable or insecure"}[ok]})
	}
	state := filepath.Join(c.DataDirectory, "init-state.json")
	if len(initStatePath) == 1 && initStatePath[0] != "" {
		state = initStatePath[0]
	}
	if b, err := os.ReadFile(state); err == nil {
		parsed, parseErr := operations.ParseInitState(b)
		ok := parseErr == nil && parsed.Next() == ""
		checks = append(checks, operations.Check{Name: "init_state", Healthy: ok, Detail: map[bool]string{true: "complete", false: "incomplete or invalid"}[ok]})
	} else {
		checks = append(checks, operations.Check{Name: "init_state", Healthy: false, Detail: "absent"})
	}
	checks = append(checks, checkWith(parent, "sqlite", "integrity verified", "unavailable or corrupt", func(ctx context.Context) error { return d.SQLiteCheck(ctx, c.DataDirectory) }))
	free, diskErr := d.DiskFreePercent(c.DataDirectory)
	minFree := uint64(100 - c.EffectiveLimits().DiskWarningPercent)
	checks = append(checks, operations.Check{Name: "disk", Healthy: diskErr == nil && free >= minFree, Detail: map[bool]string{true: "capacity healthy", false: "capacity warning"}[diskErr == nil && free >= minFree]})
	checks = append(checks, checkUpdateRollback(c.DataDirectory, expectedRollbackSnapshot))
	checks = append(checks, checkWith(parent, "clock", "NTP synchronized", "not synchronized", d.ClockOK))
	checks = append(checks, checkWith(parent, "service", "active", "inactive", d.ServiceOK))
	checks = append(checks, checkWith(parent, "port_http", "listening", "not listening", func(ctx context.Context) error { return d.PortOK(ctx, c.ListenHTTP) }))
	checks = append(checks, checkWith(parent, "port_https", "listening", "not listening", func(ctx context.Context) error { return d.PortOK(ctx, c.ListenHTTPS) }))
	if network {
		checks = append(checks, checkWith(parent, "dns_platform", "resolves", "unavailable", func(ctx context.Context) error { return resolved(d.DNSLookup(ctx, c.PlatformHost)) }))
		// A fixed, otherwise unused valid label tests the configured wildcard
		// record without requiring an active application.
		checks = append(checks, checkWith(parent, "dns_wildcard", "resolves", "unavailable", func(ctx context.Context) error { return resolved(d.DNSLookup(ctx, "tinyhost-doctor."+c.AppSuffix)) }))
		_, port, err := net.SplitHostPort(c.ListenHTTPS)
		checks = append(checks, checkWith(parent, "tls_platform", "valid", "unavailable or invalid", func(ctx context.Context) error {
			if err != nil {
				return err
			}
			return d.TLSCheck(ctx, c.PlatformHost, port)
		}))
		checks = append(checks, checkWith(parent, "resend", "credential accepted", "unavailable", func(ctx context.Context) error {
			if credentials.resendAPIKey == "" {
				return errors.New("credential unavailable")
			}
			return d.ResendCheck(ctx, credentials.resendAPIKey)
		}))
	}
	sort.Slice(checks, func(i, j int) bool { return checks[i].Name < checks[j].Name })
	return checks
}

// readDoctorCredentials accepts only the exact root-owned EnvironmentFile
// shape written by init. The value is returned only to the doctor composition
// root; callers must never put it into os.Environ or diagnostic output.
func readDoctorCredentials(c config.Config, path string) (doctorCredentials, error) {
	if effectiveUID() != 0 || !filepath.IsAbs(path) || filepath.Clean(path) != path || !pathHasNoSymlink(path, false) {
		return doctorCredentials{}, errors.New("credential unavailable")
	}
	f, err := os.Open(path)
	if err != nil {
		return doctorCredentials{}, errors.New("credential unavailable")
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || st.Mode().Perm() != 0600 {
		return doctorCredentials{}, errors.New("credential unavailable")
	}
	uid, ok := credentialOwnerUID(st)
	if !ok || uid != 0 {
		return doctorCredentials{}, errors.New("credential unavailable")
	}
	b, err := io.ReadAll(io.LimitReader(f, maxDoctorCredentialBytes+1))
	if err != nil || len(b) == 0 || len(b) > maxDoctorCredentialBytes {
		return doctorCredentials{}, errors.New("credential unavailable")
	}
	resendName, hmacName, llmName, err := credentialNames(c)
	if err != nil {
		return doctorCredentials{}, errors.New("credential unavailable")
	}
	values := map[string]string{}
	lines := strings.Split(string(b), "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	wantLines := 2
	if llmName != "" {
		wantLines = 3
	}
	if len(lines) != wantLines {
		return doctorCredentials{}, errors.New("credential unavailable")
	}
	for _, line := range lines {
		name, value, ok := strings.Cut(line, "=")
		if !ok || name == "" || !safeCredentialValue(value) || (name != resendName && name != hmacName && name != llmName) {
			return doctorCredentials{}, errors.New("credential unavailable")
		}
		if _, exists := values[name]; exists {
			return doctorCredentials{}, errors.New("credential unavailable")
		}
		values[name] = value
	}
	if values[resendName] == "" || len(values[hmacName]) < 32 || (llmName != "" && len(values[llmName]) != 64) {
		return doctorCredentials{}, errors.New("credential unavailable")
	}
	return doctorCredentials{resendAPIKey: values[resendName]}, nil
}

func safeCredentialValue(value string) bool {
	return value != "" && !strings.ContainsAny(value, " \t\r\n\x00#'\\\"")
}

func resolved(addrs []string, err error) error {
	if err != nil || len(addrs) == 0 {
		return errors.New("dns unavailable")
	}
	return nil
}

func checkWith(parent context.Context, name, pass, fail string, fn func(context.Context) error) operations.Check {
	ctx, cancel := context.WithTimeout(parent, diagnosticTimeout)
	defer cancel()
	err := error(nil)
	if fn == nil {
		err = errors.New("missing diagnostic")
	} else {
		err = fn(ctx)
	}
	return operations.Check{Name: name, Healthy: err == nil, Detail: map[bool]string{true: pass, false: fail}[err == nil]}
}

func checkUpdateRollback(dataDir string, expected bool) operations.Check {
	path := filepath.Join(dataDir, "update-rollback")
	st, err := os.Lstat(path)
	if os.IsNotExist(err) {
		if expected {
			return operations.Check{Name: "update_rollback", Healthy: false, Detail: "expected snapshot absent"}
		}
		return operations.Check{Name: "update_rollback", Healthy: true, Detail: "clear"}
	}
	if err != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 || st.Mode().Perm()&0077 != 0 {
		return operations.Check{Name: "update_rollback", Healthy: false, Detail: "unsafe or incomplete"}
	}
	previous, err := os.Lstat(filepath.Join(path, "previous"))
	if err != nil || previous.Mode()&os.ModeSymlink != 0 || !previous.Mode().IsRegular() || previous.Mode().Perm()&0077 != 0 {
		return operations.Check{Name: "update_rollback", Healthy: false, Detail: "unsafe or incomplete"}
	}
	if expected {
		return operations.Check{Name: "update_rollback", Healthy: true, Detail: "candidate rollback pending"}
	}
	return operations.Check{Name: "update_rollback", Healthy: false, Detail: "rollback pending"}
}
