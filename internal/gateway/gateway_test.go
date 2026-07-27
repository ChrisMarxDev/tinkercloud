package gateway_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tinyhost/tiny/internal/appauth"
	"github.com/tinyhost/tiny/internal/apps"
	"github.com/tinyhost/tiny/internal/config"
	"github.com/tinyhost/tiny/internal/gateway"
	"github.com/tinyhost/tiny/internal/identity"
	"github.com/tinyhost/tiny/internal/policies"
	"github.com/tinyhost/tiny/internal/releases"
	"github.com/tinyhost/tiny/internal/sessions"
	"github.com/tinyhost/tiny/internal/staticruntime"
)

func TestProtectedStaticPathDeniesBeforeReleaseRead(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("SECRET-RELEASE"), 0600); err != nil {
		t.Fatal(err)
	}
	viewer := identity.Identity{ID: "idn_a", Email: "alice@example.com"}
	store := sessions.NewMemoryStore()
	token, _, err := store.Create("app_a", viewer, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := releases.Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	appsRepo := apps.NewMemoryRepository(apps.App{ID: "app_a", Slug: "alpha", OwnerIdentityID: "idn_owner", ReleaseRoot: root, ReleaseEvidence: evidence, Status: apps.Active, SPAFallback: true})
	policiesStore := &policies.MemoryStore{Policies: map[string]policies.Policy{"app_a": {AppID: "app_a", OwnerIdentityID: "idn_owner", Revision: 1, Emails: map[string]struct{}{viewer.Email: {}}, Valid: true}}}
	h := gateway.Gateway{Config: config.Config{PlatformHost: "tiny.test", AppSuffix: "apps.tiny.test", SessionCookie: "__Host-tiny_app"}, Apps: appsRepo, Authorizer: appauth.Authorizer{Sessions: store, Policies: policiesStore}}
	inspections := 0
	restore := staticruntime.SetInspectorForTest(func(path string) (releases.FileManifest, error) {
		inspections++
		return releases.Inspect(path)
	})
	defer restore()
	s := httptest.NewServer(h)
	defer s.Close()
	client := s.Client()
	request := func(host, cookie string) *http.Response {
		r, _ := http.NewRequest(http.MethodGet, s.URL+"/", nil)
		r.Host = host
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: "__Host-tiny_app", Value: cookie})
		}
		resp, e := client.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		return resp
	}
	for _, tc := range []struct {
		name, host, cookie string
		want               int
	}{{"anonymous", "alpha.apps.tiny.test", "", 401}, {"wrong host", "alpha.apps.tiny.test.evil.test", token, 404}, {"wrong app session", "beta.apps.tiny.test", token, 404}} {
		resp := request(tc.host, tc.cookie)
		body := readBody(t, resp)
		resp.Body.Close()
		if resp.StatusCode != tc.want {
			t.Fatalf("%s: got %d", tc.name, resp.StatusCode)
		}
		if body == "SECRET-RELEASE" {
			t.Fatalf("%s leaked release", tc.name)
		}
	}
	// Route classification and authorization are both release-file-free. This
	// includes login/OTP routes, reserved routes, anonymous static, and a
	// cross-app session. Only a sealed context may trigger inspection.
	for _, p := range []string{"/_tiny/auth/login", "/_tiny/auth/otp", "/_tiny/auth/verify", "/_tiny/auth/logout", "/_tiny/not-a-route"} {
		method := http.MethodGet
		if p != "/_tiny/auth/login" && p != "/_tiny/not-a-route" {
			method = http.MethodPost
		}
		r, _ := http.NewRequest(method, s.URL+p, nil)
		r.Host = "alpha.apps.tiny.test"
		response, e := client.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		response.Body.Close()
	}
	if inspections != 0 {
		t.Fatalf("pre-authorization route inspected release %d times", inspections)
	}
	resp := request("alpha.apps.tiny.test", token)
	if got := readBody(t, resp); got != "SECRET-RELEASE" {
		t.Fatalf("allowed request got %q", got)
	}
	resp.Body.Close()
	if inspections != 1 {
		t.Fatalf("authorized static did not inspect exactly once: %d", inspections)
	}
	rangeRequest, _ := http.NewRequest(http.MethodGet, s.URL+"/", nil)
	rangeRequest.Host = "alpha.apps.tiny.test"
	rangeRequest.AddCookie(&http.Cookie{Name: "__Host-tiny_app", Value: token})
	rangeRequest.Header.Set("Range", "bytes=0-5")
	rangeResponse, err := client.Do(rangeRequest)
	if err != nil {
		t.Fatal(err)
	}
	if rangeResponse.StatusCode != http.StatusPartialContent || readBody(t, rangeResponse) != "SECRET" {
		t.Fatal("range serving failed")
	}
	rangeResponse.Body.Close()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("OUTSIDE"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape.txt")); err != nil {
		t.Fatal(err)
	}
	symlinkRequest, _ := http.NewRequest(http.MethodGet, s.URL+"/escape.txt", nil)
	symlinkRequest.Host = "alpha.apps.tiny.test"
	symlinkRequest.AddCookie(&http.Cookie{Name: "__Host-tiny_app", Value: token})
	symlinkResponse, err := client.Do(symlinkRequest)
	if err != nil {
		t.Fatal(err)
	}
	if symlinkResponse.StatusCode != http.StatusNotFound || readBody(t, symlinkResponse) == "OUTSIDE" {
		t.Fatal("symlink component escaped release")
	}
	symlinkResponse.Body.Close()
	store.Revoke(token)
	resp = request("alpha.apps.tiny.test", token)
	if got := readBody(t, resp); resp.StatusCode != 401 || got == "SECRET-RELEASE" {
		t.Fatalf("revoked session: %d %q", resp.StatusCode, got)
	}
	resp.Body.Close()
	if inspections != 3 { // initial static, symlink, range; no revoked read
		t.Fatalf("denied revoked session inspected release: %d", inspections)
	}
	token, _, err = store.Create("app_a", viewer, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	policiesStore.Err = policies.ErrUnavailable
	resp = request("alpha.apps.tiny.test", token)
	if got := readBody(t, resp); resp.StatusCode != 401 || got == "SECRET-RELEASE" {
		t.Fatalf("policy failure: %d %q", resp.StatusCode, got)
	}
	resp.Body.Close()
	if inspections != 3 {
		t.Fatalf("policy failure inspected release: %d", inspections)
	}
}
func readBody(t *testing.T, r *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
func TestHostClassificationFailsClosed(t *testing.T) {
	cfg := config.Config{PlatformHost: "tiny.test", AppSuffix: "apps.tiny.test"}
	for _, raw := range []string{"alpha.apps.tiny.test", "ALPHA.APPS.TINY.TEST.", "alpha.apps.tiny.test:443"} {
		h, ok := gateway.ClassifyHost(raw, cfg)
		if !ok || h.Kind != gateway.App || h.Slug != "alpha" {
			t.Fatalf("expected app for %q", raw)
		}
	}
	for _, raw := range []string{"a.b.apps.tiny.test", "-a.apps.tiny.test", "alpha.apps.tiny.test:bad", ""} {
		if _, ok := gateway.ClassifyHost(raw, cfg); ok {
			t.Fatalf("accepted malformed %q", raw)
		}
	}
	if h, ok := gateway.ClassifyHost("alpha.apps.tiny.test.evil", cfg); !ok || h.Kind != gateway.Unknown {
		t.Fatal("suffix trick must be an unknown host")
	}
}
func TestRouteRegistryCoversClasses(t *testing.T) {
	r := gateway.Registry()
	for _, class := range []gateway.Endpoint{gateway.Reserved, gateway.AppLogin, gateway.AppOTPRequest, gateway.AppOTPVerify, gateway.AppLogout, gateway.CurrentUser, gateway.AppInfo, gateway.Capabilities, gateway.KV, gateway.Blobs, gateway.Live, gateway.ProtectedStatic} {
		if r[class] == "" {
			t.Fatalf("route class %d missing", class)
		}
	}
}

func TestReturnPathRejectsCrossHostAndControlInput(t *testing.T) {
	if got, ok := gateway.ValidReturnPath("/reports?tab=1"); !ok || got != "/reports?tab=1" {
		t.Fatal("relative return path rejected")
	}
	for _, raw := range []string{"https://evil.test", "//evil.test", "javascript:alert(1)", "/ok\nLocation: evil", `/\\evil`, `/%5cevil`} {
		if _, ok := gateway.ValidReturnPath(raw); ok {
			t.Fatalf("accepted unsafe return path %q", raw)
		}
	}
}

func TestAnonymousDocumentNavigationRedirectsOnlyProtectedStatic(t *testing.T) {
	h := gateway.Gateway{
		Config: config.Config{PlatformHost: "tiny.test", AppSuffix: "apps.tiny.test", SessionCookie: sessions.AppCookieName},
		Apps:   apps.NewMemoryRepository(apps.App{ID: "app_a", Slug: "alpha", Status: apps.Active}),
	}
	request := func(method, path, dest, accept string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(method, path, nil)
		r.Host = "alpha.apps.tiny.test"
		r.Header.Set("Sec-Fetch-Dest", dest)
		r.Header.Set("Accept", accept)
		h.ServeHTTP(w, r)
		return w
	}
	if w := request(http.MethodGet, "/dashboard?tab=1", "document", "text/html,application/xhtml+xml"); w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/_tiny/auth/login?return=%2Fdashboard%3Ftab%3D1" || strings.Contains(w.Body.String(), "SECRET-RELEASE") {
		t.Fatalf("document navigation = status %d location %q body %q", w.Code, w.Header().Get("Location"), w.Body.String())
	}
	for name, tc := range map[string]struct{ method, path, dest, accept string }{
		"api":       {http.MethodGet, "/_tiny/api/v1/me", "document", "text/html"},
		"asset":     {http.MethodGet, "/app.js", "script", "*/*"},
		"range":     {http.MethodGet, "/", "empty", "text/html"},
		"websocket": {http.MethodGet, "/_tiny/ws/v1", "websocket", "*/*"},
		"ambiguous": {http.MethodGet, "/", "", "text/html"},
	} {
		t.Run(name, func(t *testing.T) {
			w := request(tc.method, tc.path, tc.dest, tc.accept)
			if w.Code != http.StatusUnauthorized || w.Header().Get("Location") != "" || !strings.Contains(w.Body.String(), `"not_authorized"`) {
				t.Fatalf("%s denial = status %d location %q body %q", name, w.Code, w.Header().Get("Location"), w.Body.String())
			}
		})
	}
	// Even if a malicious client injects a valid session token from another app,
	// the alpha host must not treat it as identity or expose static bytes.
	store := sessions.NewMemoryStore()
	wrongAppToken, _, err := store.Create("app_b", identity.Identity{ID: "viewer", Email: "viewer@example.com"}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	h.Authorizer = appauth.Authorizer{Sessions: store, Policies: &policies.MemoryStore{Policies: map[string]policies.Policy{"app_a": {AppID: "app_a", Valid: true}}}}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Host = "alpha.apps.tiny.test"
	r.Header.Set("Accept", "text/html")
	r.Header.Set("Sec-Fetch-Dest", "document")
	r.AddCookie(&http.Cookie{Name: sessions.AppCookieName, Value: wrongAppToken})
	h.ServeHTTP(w, r)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/_tiny/auth/login?return=%2F" || strings.Contains(w.Body.String(), "SECRET-RELEASE") {
		t.Fatalf("wrong-app document session = status %d location %q body %q", w.Code, w.Header().Get("Location"), w.Body.String())
	}
}

func TestSameOriginFailsClosed(t *testing.T) {
	for name, origin := range map[string]string{
		"same":     "https://alpha.apps.tiny.test",
		"cross":    "https://evil.test",
		"http":     "http://alpha.apps.tiny.test",
		"absent":   "",
		"path":     "https://alpha.apps.tiny.test/evil",
		"userinfo": "https://viewer@alpha.apps.tiny.test",
	} {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "https://alpha.apps.tiny.test/_tiny/auth/logout", nil)
			r.Host = "alpha.apps.tiny.test"
			r.Header.Set("Origin", origin)
			got := gateway.SameOrigin(r)
			if got != (name == "same") {
				t.Fatalf("origin %q accepted=%v", origin, got)
			}
		})
	}
}
