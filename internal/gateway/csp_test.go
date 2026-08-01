package gateway_test

import (
	"github.com/ChrisMarxDev/tinkercloud/internal/config"
	"github.com/ChrisMarxDev/tinkercloud/internal/gateway"
	webui "github.com/ChrisMarxDev/tinkercloud/web"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDenyCSPAndNoCORS(t *testing.T) {
	g := gateway.Gateway{Config: config.Config{Domain: "apps.tinker.test"}}
	r := httptest.NewRequest(http.MethodOptions, "http://evil.test/", nil)
	r.Host = "evil.test"
	r.Header.Set("Origin", "https://evil.test")
	w := httptest.NewRecorder()
	g.ServeHTTP(w, r)
	csp := w.Header().Get("Content-Security-Policy")
	for _, v := range []string{"default-src 'self'", "object-src 'none'", "base-uri 'none'", "frame-ancestors 'none'", "connect-src 'self'", "style-src 'self' " + webui.TinkerStyleCSPSource(), "script-src 'self' " + webui.TinkerScriptCSPSource()} {
		if !strings.Contains(csp, v) {
			t.Fatal(csp)
		}
	}
	if strings.Contains(csp, "wss:") || strings.Contains(csp, "unsafe-inline") || strings.Contains(csp, "*") {
		t.Fatalf("CSP allows arbitrary WebSocket egress: %s", csp)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "" || strings.Contains(w.Body.String(), "SECRET-RELEASE") {
		t.Fatal(w.Header(), w.Body.String())
	}
}
