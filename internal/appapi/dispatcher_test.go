package appapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tinyhost/tiny/internal/appauth"
	"github.com/tinyhost/tiny/internal/apps"
	"github.com/tinyhost/tiny/internal/identity"
	"github.com/tinyhost/tiny/internal/kv"
	"github.com/tinyhost/tiny/internal/policies"
	"github.com/tinyhost/tiny/internal/sessions"
)

func authFor(t *testing.T, appID string) appauth.AuthorizationContext {
	return authForKV(t, appID, true)
}

func authForKV(t *testing.T, appID string, enabled bool) appauth.AuthorizationContext {
	t.Helper()
	v := identity.Identity{ID: "viewer", Email: "v@example.com"}
	s := sessions.NewMemoryStore()
	token, _, _ := s.Create(appID, v, time.Now().Add(time.Hour))
	a, e := appauth.Authorizer{Sessions: s, Policies: &policies.MemoryStore{Policies: map[string]policies.Policy{appID: {AppID: appID, OwnerIdentityID: "viewer", Valid: true}}}}.Authorize(context.Background(), apps.App{ID: appID, KVEnabled: enabled, RealtimeEnabled: true}, token, "req_safe")
	if e != nil {
		t.Fatal(e)
	}
	return a
}

type listSpy struct{ calls int }

func (s *listSpy) Get(context.Context, appauth.AuthorizationContext, string) (*kv.Entry, error) {
	s.calls++
	return nil, nil
}
func (s *listSpy) Set(context.Context, appauth.AuthorizationContext, string, json.RawMessage, *uint64) (kv.Entry, error) {
	s.calls++
	return kv.Entry{}, nil
}
func (s *listSpy) Delete(context.Context, appauth.AuthorizationContext, string, *uint64) (bool, error) {
	s.calls++
	return false, nil
}
func (s *listSpy) List(context.Context, appauth.AuthorizationContext, string, string, int) (kv.ListResult, error) {
	s.calls++
	return kv.ListResult{}, nil
}
func request(d Dispatcher, a appauth.AuthorizationContext, method, path, body string, contentType bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Host = "example.com"
	if contentType {
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", "http://"+r.Host)
	}
	w := httptest.NewRecorder()
	d.Dispatch(a, w, r)
	return w
}
func TestTwoAppIsolationVersionAndMalformedRequest(t *testing.T) {
	d := Dispatcher{KV: kv.New(kv.DefaultLimits(), nil), AppSlug: func(a appauth.AuthorizationContext) string { return "app-" + a.AppID() }}
	a, b := authFor(t, "a"), authFor(t, "b")
	w := request(d, a, http.MethodPut, "/_tiny/api/v1/kv/state%2Fx", `{"value":{"ok":true}}`, true)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request(d, b, http.MethodGet, "/_tiny/api/v1/kv/state%2Fx", "", false)
	if w.Code != 404 {
		t.Fatal("cross-app isolation", w.Code)
	}
	w = request(d, a, http.MethodPut, "/_tiny/api/v1/kv/state%2Fx", `{"value":1,"expected_version":9}`, true)
	if w.Code != 409 || !strings.Contains(w.Body.String(), "req_safe") {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request(d, a, http.MethodPut, "/_tiny/api/v1/kv/x", `{"unknown":1}`, true)
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
	w = request(d, a, http.MethodPut, "/_tiny/api/v1/kv/x", `{"value":1}`, false)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
}
func TestCurrentIdentityAndAppAreServerDerived(t *testing.T) {
	d := Dispatcher{KV: kv.New(kv.DefaultLimits(), nil), AppSlug: func(appauth.AuthorizationContext) string { return "demo" }}
	w := request(d, authFor(t, "a"), http.MethodGet, "/_tiny/api/v1/me", "", false)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "demo") || strings.Contains(w.Body.String(), "app_id") {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request(d, nil, http.MethodGet, "/_tiny/api/v1/me", "", false)
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
}

func TestDisabledKVListDeniedBeforeDispatcher(t *testing.T) {
	spy := &listSpy{}
	d := Dispatcher{KV: spy}
	w := request(d, authForKV(t, "a", false), http.MethodGet, "/_tiny/api/v1/kv?prefix=state", "", false)
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "capability_unavailable") {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if spy.calls != 0 {
		t.Fatalf("disabled list reached dispatcher capability %d times", spy.calls)
	}
}

// An empty page is a successful collection response, not a nullable value.
// SDK clients must be able to iterate entries without special-casing it.
func TestKVListEmptyPageSerializesEntriesAsArray(t *testing.T) {
	d := Dispatcher{KV: kv.New(kv.DefaultLimits(), nil)}
	w := request(d, authFor(t, "a"), http.MethodGet, "/_tiny/api/v1/kv?prefix=missing/", "", false)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var response struct {
		Entries    []kv.Entry `json:"entries"`
		NextCursor string     `json:"next_cursor"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Entries == nil || len(response.Entries) != 0 {
		t.Fatalf("entries must be an empty array, got %#v; body=%s", response.Entries, w.Body.String())
	}
	if response.NextCursor != "" {
		t.Fatalf("empty page cursor=%q", response.NextCursor)
	}
	if strings.Contains(w.Body.String(), `"entries":null`) {
		t.Fatalf("empty page serialized null entries: %s", w.Body.String())
	}
}

func TestSDKVersionNegotiation(t *testing.T) {
	d := Dispatcher{AppSlug: func(appauth.AuthorizationContext) string { return "alpha" }}
	a := authFor(t, "a")
	for _, version := range []string{"", "0.1.0", "0.999.12"} {
		r := httptest.NewRequest(http.MethodGet, "/_tiny/api/v1/me", nil)
		r.Header.Set("X-Tiny-SDK-Version", version)
		w := httptest.NewRecorder()
		d.Dispatch(a, w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("version %q: got %d", version, w.Code)
		}
	}
	for _, version := range []string{"1.0.0", "not-semver", "0.1", "0.1.0-beta"} {
		r := httptest.NewRequest(http.MethodGet, "/_tiny/api/v1/me", nil)
		r.Header.Set("X-Tiny-SDK-Version", version)
		w := httptest.NewRecorder()
		d.Dispatch(a, w, r)
		if w.Code != http.StatusUpgradeRequired || !strings.Contains(w.Body.String(), "sdk_version_incompatible") {
			t.Fatalf("version %q: got %d %s", version, w.Code, w.Body.String())
		}
	}
}
