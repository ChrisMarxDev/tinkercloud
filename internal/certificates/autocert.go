package certificates

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"strings"
	"sync"

	"golang.org/x/crypto/acme/autocert"
)

var ErrHostDenied = errors.New("certificate host denied")

// HostResolver is backed by the server's current app repository; it must return
// true only for active exact app hostnames.
type HostResolver interface{ ActiveAppHost(string) bool }
type Manager struct {
	PlatformHost string
	Apps         HostResolver
	ACME         *autocert.Manager
	mu           sync.Mutex
	inflight     map[string]struct{}
}

func NewAutocert(dataDir, email, platform string, apps HostResolver, prompt func(string) bool) *Manager {
	m := &Manager{PlatformHost: strings.ToLower(platform), Apps: apps, inflight: map[string]struct{}{}}
	m.ACME = &autocert.Manager{Prompt: prompt, Email: email, Cache: autocert.DirCache(dataDir), HostPolicy: m.hostPolicy}
	return m
}
func (m *Manager) hostPolicy(_ context.Context, host string) error { return m.allowed(host) }
func (m *Manager) allowed(host string) error {
	h := strings.TrimSuffix(strings.ToLower(host), ".")
	if h == m.PlatformHost {
		return nil
	}
	if m.Apps != nil && m.Apps.ActiveAppHost(h) {
		return nil
	}
	return ErrHostDenied
}
func (m *Manager) HTTPHandler(next http.Handler) http.Handler { return m.ACME.HTTPHandler(next) }
func (m *Manager) TLSConfig() *tls.Config {
	return &tls.Config{GetCertificate: m.GetCertificate, MinVersion: tls.VersionTLS12}
}
func (m *Manager) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	if err := m.allowed(hello.ServerName); err != nil {
		return nil, err
	}
	m.mu.Lock()
	if _, ok := m.inflight[hello.ServerName]; ok {
		m.mu.Unlock()
		return nil, ErrHostDenied
	}
	m.inflight[hello.ServerName] = struct{}{}
	m.mu.Unlock()
	defer func() { m.mu.Lock(); delete(m.inflight, hello.ServerName); m.mu.Unlock() }()
	return m.ACME.GetCertificate(hello)
}
