package requestlog

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMiddlewareRedactsSensitiveRequestParts(t *testing.T) {
	var out bytes.Buffer
	h := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-ID", "req_abc123")
		w.WriteHeader(http.StatusUnauthorized)
	}), slog.New(slog.NewJSONHandler(&out, nil)))
	r := httptest.NewRequest(http.MethodPost, "https://secret.example/_tinker/auth/otp?email=person@example.com&token=secret", strings.NewReader("cookie=bad"))
	r.Header.Set("Cookie", "session=secret")
	h.ServeHTTP(httptest.NewRecorder(), r)
	got := out.String()
	for _, forbidden := range []string{"secret.example", "person@example.com", "token=", "session=", "cookie=bad", "otp?"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("logged sensitive input %q: %s", forbidden, got)
		}
	}
	for _, expected := range []string{"req_abc123", "app_auth", "denied", `"status":401`} {
		if !strings.Contains(got, expected) {
			t.Fatalf("missing %q: %s", expected, got)
		}
	}
}

func TestRouteClassNeverContainsPath(t *testing.T) {
	if got := RouteClass(http.MethodGet, "/files/a-personal-name.txt"); got != "app_static" {
		t.Fatalf("%q", got)
	}
	if got := safeRequestID("Bearer top-secret"); got != "req_unavailable" {
		t.Fatalf("%q", got)
	}
}

func TestServiceRejectsCallerSuppliedPath(t *testing.T) {
	var out bytes.Buffer
	Service(slog.New(slog.NewJSONHandler(&out, nil)), "/var/lib/tinkercloud/releases/private", "succeeded", 1)
	if strings.Contains(out.String(), "/var/lib/") || !strings.Contains(out.String(), "service.unknown") {
		t.Fatalf("unsafe service log: %s", out.String())
	}
}

func TestServiceLogsOnlyFixedCLIIssuerFailureCategory(t *testing.T) {
	var out bytes.Buffer
	Service(slog.New(slog.NewJSONHandler(&out, nil)), "cli_otp_issuance_persistence_insert", "failed", 1)
	got := out.String()
	for _, forbidden := range []string{"@", "login_", "sqlite", "resend", "transaction"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("issuer diagnostic leaked %q: %s", forbidden, got)
		}
	}
	if !strings.Contains(got, "service.cli_otp_issuance_persistence_insert") || !strings.Contains(got, `"outcome":"failed"`) {
		t.Fatalf("missing fixed issuer category: %s", got)
	}
}
