package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/tinyhost/tiny/internal/certificates"
	"github.com/tinyhost/tiny/internal/config"
)

func TestPlainHTTPDoesNotReachTinyHostHandler(t *testing.T) {
	t.Parallel()
	cfg := httpBoundaryConfig(t)
	called := false
	tinyhost := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true })
	// This mirrors the production order: autocert wraps a plaintext-only
	// fallback, never the TinyHost HTTPS gateway.
	cm := certificates.NewAutocert(t.TempDir(), "operator@example.test", cfg.PlatformHost(), nil, func(string) bool { return true })
	h, _ := publicHandlers(cfg, cm, func(host string) bool { return host == cfg.PlatformHost() }, tinyhost)

	r := httptest.NewRequest(http.MethodGet, "http://admin.apps.example.test/_tiny/auth/login", nil)
	r.Host = cfg.PlatformHost()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if called {
		t.Fatal("plaintext request reached TinyHost handler")
	}
	if w.Code != http.StatusMovedPermanently {
		t.Fatalf("status = %d, want redirect", w.Code)
	}
	if got, want := w.Header().Get("Location"), "https://admin.apps.example.test/_tiny/auth/login"; got != want {
		t.Fatalf("Location = %q, want %q", got, want)
	}
}

func TestPlainHTTPKnownAppRedirectsToCanonicalHTTPS(t *testing.T) {
	t.Parallel()
	cfg := httpBoundaryConfig(t)
	h := httpRedirectHandler(cfg, func(host string) bool { return host == "demo.apps.example.test" })
	r := httptest.NewRequest(http.MethodGet, "http://demo.apps.example.test/a?b=c", nil)
	r.Host = "DEMO.APPS.EXAMPLE.TEST.:80"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusMovedPermanently {
		t.Fatalf("status = %d, want redirect", w.Code)
	}
	if got, want := w.Header().Get("Location"), "https://demo.apps.example.test/a?b=c"; got != want {
		t.Fatalf("Location = %q, want %q", got, want)
	}
}

func TestPlainHTTPMalformedOrUnknownHostFailsClosed(t *testing.T) {
	t.Parallel()
	cfg := httpBoundaryConfig(t)
	h := httpRedirectHandler(cfg, func(host string) bool { return host == cfg.PlatformHost() })
	for _, host := range []string{
		"unknown.example.test",
		"platform.example.test@attacker.test",
		"platform.example.test/path",
		"platform.example.test:443",
		"127.0.0.1",
		"-platform.example.test",
	} {
		t.Run(host, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "http://platform.example.test/", nil)
			r.Host = host
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want fail-closed 404", w.Code)
			}
			if got := w.Header().Get("Location"); got != "" {
				t.Fatalf("unexpected Location %q", got)
			}
		})
	}
}

func TestACMEHTTPPathStaysWithAutocert(t *testing.T) {
	t.Parallel()
	cfg := httpBoundaryConfig(t)
	fallbackCalled := false
	plainFallback := httpRedirectHandler(cfg, func(string) bool {
		fallbackCalled = true
		return false
	})
	cm := certificates.NewAutocert(t.TempDir(), "operator@example.test", cfg.PlatformHost(), nil, func(string) bool { return true })
	// Test the autocert wrapper directly to prove it reserves HTTP-01 before
	// the redirect fallback. publicHandlers uses this exact wrapper.
	h := cm.HTTPHandler(plainFallback)
	r := httptest.NewRequest(http.MethodGet, "http://platform.example.test/.well-known/acme-challenge/test-token", nil)
	r.Host = cfg.PlatformHost()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if fallbackCalled {
		t.Fatal("ACME path was not delegated to autocert")
	}
	if w.Code == http.StatusMovedPermanently {
		t.Fatal("ACME path was redirected instead of delegated to autocert")
	}
	if got := w.Header().Get("Strict-Transport-Security"); got != "" {
		t.Fatalf("ACME HSTS = %q, want empty", got)
	}
}

func TestHTTPSResponsesHaveHSTSOnly(t *testing.T) {
	t.Parallel()
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	w := httptest.NewRecorder()
	httpsSecurityHeaders(next).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "https://platform.example.test/", nil))
	if got := w.Header().Get("Strict-Transport-Security"); got != hstsValue {
		t.Fatalf("HSTS = %q, want %q", got, hstsValue)
	}

	cfg := httpBoundaryConfig(t)
	w = httptest.NewRecorder()
	httpRedirectHandler(cfg, func(host string) bool { return host == cfg.PlatformHost() }).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "http://platform.example.test/", nil))
	if got := w.Header().Get("Strict-Transport-Security"); got != "" {
		t.Fatalf("plaintext HSTS = %q, want empty", got)
	}
}

func httpBoundaryConfig(t *testing.T) config.Config {
	t.Helper()
	root := t.TempDir()
	return config.Config{Domain: "apps.example.test", DataDirectory: filepath.Join(root, "data")}
}
