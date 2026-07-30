package compose

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tinyhost/tiny/internal/apps"
	"github.com/tinyhost/tiny/internal/gateway"
	"github.com/tinyhost/tiny/internal/identity"
	"github.com/tinyhost/tiny/internal/sessions"
)

func TestAppLoginWithoutBrokerNeverFallsBackToDirectOTP(t *testing.T) {
	l := Login{}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/_tiny/auth/login?return=/safe", nil)
	l.DispatchPreAuth(apps.App{ID: "a"}, gateway.AppLogin, w, r)
	if w.Code != http.StatusServiceUnavailable || strings.Contains(w.Body.String(), "one-time code") {
		t.Fatalf("app login unexpectedly offered a direct OTP: status=%d body=%q", w.Code, w.Body.String())
	}
}

func TestAppLogoutIsLocalAndRequiresSameOrigin(t *testing.T) {
	store := sessions.NewMemoryStore()
	issue := func(app string) string {
		token, _, err := store.Create(app, identity.Identity{ID: "viewer", Email: "viewer@example.com"}, time.Now().Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		return token
	}
	l := Login{Sessions: MemorySessions{Store: store}}
	app := apps.App{ID: "app_a"}
	request := func(origin, ret, token string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/_tiny/auth/logout", strings.NewReader("return="+ret))
		r.Host = "alpha.apps.tiny.test"
		r.Header.Set("Origin", origin)
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=utf-8")
		r.AddCookie(&http.Cookie{Name: sessions.AppCookieName, Value: token})
		l.DispatchPreAuth(app, gateway.AppLogout, w, r)
		return w
	}
	token := issue("app_a")
	if w := request("https://evil.test", "%2Fsafe", token); w.Code != http.StatusUnauthorized {
		t.Fatalf("cross-origin logout status=%d", w.Code)
	}
	if _, err := store.Validate(context.Background(), "app_a", token, time.Now()); err != nil {
		t.Fatal("cross-origin logout mutated app session")
	}
	if w := request("https://alpha.apps.tiny.test", "https%3A%2F%2Fevil.test", token); w.Code != http.StatusBadRequest {
		t.Fatalf("open return logout status=%d", w.Code)
	}
	if _, err := store.Validate(context.Background(), "app_a", token, time.Now()); err != nil {
		t.Fatal("open-return logout mutated app session")
	}
	if w := request("https://alpha.apps.tiny.test", "%2Fsafe%3Ftab%3D1", token); w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/_tiny/auth/login?return=%2Fsafe%3Ftab%3D1" {
		t.Fatalf("logout status=%d location=%q", w.Code, w.Header().Get("Location"))
	}
	if _, err := store.Validate(context.Background(), "app_a", token, time.Now()); err == nil {
		t.Fatal("same-origin logout retained app session")
	}
	other := issue("app_b")
	if w := request("https://alpha.apps.tiny.test", "%2F", other); w.Code != http.StatusSeeOther {
		t.Fatalf("wrong-app cookie logout status=%d", w.Code)
	}
	if _, err := store.Validate(context.Background(), "app_b", other, time.Now()); err != nil {
		t.Fatal("app-local logout revoked another app session")
	}
}
