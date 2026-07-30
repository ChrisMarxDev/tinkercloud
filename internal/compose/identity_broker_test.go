package compose

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tinyhost/tiny/internal/apps"
	"github.com/tinyhost/tiny/internal/browseridentity"
	"github.com/tinyhost/tiny/internal/gateway"
	"github.com/tinyhost/tiny/internal/identity"
	"github.com/tinyhost/tiny/internal/otp"
	"github.com/tinyhost/tiny/internal/persistence"
	"github.com/tinyhost/tiny/internal/ratelimit"
	"github.com/tinyhost/tiny/internal/sessions"
)

// TestIdentityBrokerReusesOneGlobalOTPAcrossApps exercises the browser-facing
// composition boundary. The fake records provider issuance, while the test
// drives the same app/platform redirects and host-only cookies as the gateway.
func TestIdentityBrokerReusesOneGlobalOTPAcrossApps(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	store := newBrokerStore(now)
	var closed []persistence.AppSessionRef
	b := &IdentityBroker{
		Store: store, Domain: "apps.tiny.test",
		Now: func() time.Time { return now }, AppSessionTTL: time.Hour,
		RevokeChildren: func(refs []persistence.AppSessionRef) { closed = append(closed, refs...) },
	}
	login := Login{IdentityBroker: b}
	platform := b.PlatformHandler(http.NotFoundHandler())
	alpha := apps.App{ID: "app-alpha", Slug: "alpha"}
	beta := apps.App{ID: "app-beta", Slug: "beta"}
	denied := apps.App{ID: "app-denied", Slug: "denied"}
	const preservedReturn = "/projects/42?tag=a&tag=b&next=%2Ffoo%3Fx%3D1"

	start := func(app apps.App, ret string) (*http.Cookie, string) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/_tiny/auth/login?return="+url.QueryEscape(ret), nil)
		r.Host = app.Slug + ".apps.tiny.test"
		login.DispatchPreAuth(app, gateway.AppLogin, w, r)
		if w.Code != http.StatusSeeOther || !strings.HasPrefix(w.Header().Get("Location"), "https://tiny.test/_tiny/identity?handoff=") {
			t.Fatalf("%s start status=%d location=%q", app.Slug, w.Code, w.Header().Get("Location"))
		}
		cookies := w.Result().Cookies()
		if len(cookies) != 1 || cookies[0].Name != identityStateCookieName || !cookies[0].Secure || !cookies[0].HttpOnly {
			t.Fatalf("%s state cookie=%+v", app.Slug, cookies)
		}
		u, _ := url.Parse(w.Header().Get("Location"))
		return cookies[0], u.Query().Get("handoff")
	}
	callback := func(app apps.App, state *http.Cookie, handoff string) *http.Cookie {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/_tiny/auth/callback?handoff="+url.QueryEscape(handoff), nil)
		r.Host = app.Slug + ".apps.tiny.test"
		r.AddCookie(state)
		login.DispatchPreAuth(app, gateway.AppIdentityCallback, w, r)
		if w.Code != http.StatusSeeOther || w.Header().Get("Location") != preservedReturn {
			t.Fatalf("%s callback status=%d location=%q", app.Slug, w.Code, w.Header().Get("Location"))
		}
		for _, c := range w.Result().Cookies() {
			if c.Name == sessions.AppCookieName {
				return c
			}
		}
		t.Fatalf("%s callback did not issue app cookie", app.Slug)
		return nil
	}

	// The handoff retains the full same-host path and query exactly, including
	// repeated and percent-encoded values. A fragment is browser-local and is
	// intentionally not server-guaranteed.
	alphaState, alphaHandoff := start(alpha, preservedReturn+"#browser-only")
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/_tiny/identity?handoff="+url.QueryEscape(alphaHandoff), nil)
	r.Host = "tiny.test"
	platform.ServeHTTP(w, r)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `name="email"`) || strings.Contains(w.Body.String(), "app-alpha") {
		t.Fatalf("identity email page status=%d body=%q", w.Code, w.Body.String())
	}
	binding := browserBindingCookieFrom(t, w.Result().Cookies())
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost, "/_tiny/identity/otp", strings.NewReader("handoff="+url.QueryEscape(alphaHandoff)+"&email=viewer%40example.com"))
	r.Host = "tiny.test"
	r.Header.Set("Origin", "https://tiny.test")
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(binding)
	platform.ServeHTTP(w, r)
	if w.Code != http.StatusOK || store.requests != 1 || !strings.Contains(w.Body.String(), `name="transaction"`) {
		t.Fatalf("OTP form status=%d requests=%d body=%q", w.Code, store.requests, w.Body.String())
	}
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost, "/_tiny/identity/verify", strings.NewReader("handoff="+url.QueryEscape(alphaHandoff)+"&email=viewer%40example.com&transaction=otp_1&code=123456"))
	r.Host = "tiny.test"
	r.Header.Set("Origin", "https://tiny.test")
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(binding)
	platform.ServeHTTP(w, r)
	if w.Code != http.StatusSeeOther || !strings.Contains(w.Header().Get("Location"), "alpha.apps.tiny.test/_tiny/auth/callback") {
		t.Fatalf("verify status=%d location=%q", w.Code, w.Header().Get("Location"))
	}
	var global *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == GlobalIdentityCookieName {
			global = c
		}
	}
	if global == nil || global.Value != "global" || !global.HttpOnly || !global.Secure {
		t.Fatalf("global cookie=%+v", global)
	}
	if app := callback(alpha, alphaState, alphaHandoff); app == nil || app.Value == "" {
		t.Fatal("missing alpha app cookie")
	}

	betaState, betaHandoff := start(beta, preservedReturn)
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/_tiny/identity?handoff="+url.QueryEscape(betaHandoff), nil)
	r.Host = "tiny.test"
	r.AddCookie(global)
	platform.ServeHTTP(w, r)
	if w.Code != http.StatusSeeOther || !strings.Contains(w.Header().Get("Location"), "beta.apps.tiny.test/_tiny/auth/callback") || store.requests != 1 {
		t.Fatalf("second app status=%d location=%q provider requests=%d", w.Code, w.Header().Get("Location"), store.requests)
	}
	if app := callback(beta, betaState, betaHandoff); app == nil || app.Value == "" {
		t.Fatal("missing beta app cookie")
	}

	_, deniedHandoff := start(denied, "/report?tab=1")
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/_tiny/identity?handoff="+url.QueryEscape(deniedHandoff), nil)
	r.Host = "tiny.test"
	r.AddCookie(global)
	platform.ServeHTTP(w, r)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "viewer@example.com") || !strings.Contains(w.Body.String(), "Use another email") || strings.Contains(w.Body.String(), "allowlist") {
		t.Fatalf("denied page status=%d body=%q", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost, "/_tiny/identity/use-another", strings.NewReader("handoff="+url.QueryEscape(deniedHandoff)))
	r.Host = "tiny.test"
	r.Header.Set("Origin", "https://tiny.test")
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(binding)
	platform.ServeHTTP(w, r)
	if w.Code != http.StatusOK || !store.handoffs[deniedHandoff].ForceLogin || len(closed) != 0 || !strings.Contains(w.Body.String(), `name="email"`) {
		t.Fatalf("switch page status=%d force=%v closed=%v body=%q", w.Code, store.handoffs[deniedHandoff].ForceLogin, closed, w.Body.String())
	}
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost, "/_tiny/identity/otp", strings.NewReader("handoff="+url.QueryEscape(deniedHandoff)+"&email=other%40example.com"))
	r.Host = "tiny.test"
	r.Header.Set("Origin", "https://tiny.test")
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(binding)
	platform.ServeHTTP(w, r)
	if w.Code != http.StatusOK || store.requests != 2 {
		t.Fatalf("switch OTP request status=%d requests=%d", w.Code, store.requests)
	}
	// The old global identity is replaced only after a successful OTP. The
	// persistence result's child refs are then forwarded to live revocation.
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost, "/_tiny/identity/verify", strings.NewReader("handoff="+url.QueryEscape(deniedHandoff)+"&email=other%40example.com&transaction=otp_2&code=123456"))
	r.Host = "tiny.test"
	r.Header.Set("Origin", "https://tiny.test")
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(global)
	r.AddCookie(binding)
	platform.ServeHTTP(w, r)
	if w.Code != http.StatusSeeOther || len(closed) != 1 || closed[0].AppID != "app-alpha" {
		t.Fatalf("switch verify status=%d closed=%+v", w.Code, closed)
	}
}

func TestIdentityBrokerPlatformLoginIssuesTheSameIdentityUsedForAppHandoffs(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	store := newBrokerStore(now)
	b := IdentityBroker{Store: store, PlatformHost: "admin.apps.tiny.test", AppSuffix: "apps.tiny.test", HMACKey: []byte("test"), Now: func() time.Time { return now }}
	h := b.PlatformHandler(http.NotFoundHandler())

	page := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "https://admin.apps.tiny.test/login", nil)
	r.Host = "admin.apps.tiny.test"
	h.ServeHTTP(page, r)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Sign in to TinyHost") {
		t.Fatalf("platform login page status=%d body=%q", page.Code, page.Body.String())
	}
	binding := browserBindingCookieFrom(t, page.Result().Cookies())
	request := httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost, "https://admin.apps.tiny.test/login", strings.NewReader("email=viewer%40example.test"))
	r.Host = "admin.apps.tiny.test"
	r.Header.Set("Origin", "https://admin.apps.tiny.test")
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(binding)
	h.ServeHTTP(request, r)
	if request.Code != http.StatusOK || store.platformRequests != 1 || !strings.Contains(request.Body.String(), `name="transaction" value="pid_1"`) {
		t.Fatalf("platform OTP request status=%d body=%q", request.Code, request.Body.String())
	}
	verify := httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost, "https://admin.apps.tiny.test/login/verify", strings.NewReader("email=viewer%40example.test&transaction=pid_1&code=123456"))
	r.Host = "admin.apps.tiny.test"
	r.Header.Set("Origin", "https://admin.apps.tiny.test")
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(binding)
	h.ServeHTTP(verify, r)
	if verify.Code != http.StatusSeeOther || verify.Header().Get("Location") != "/dashboard" {
		t.Fatalf("platform verify status=%d location=%q", verify.Code, verify.Header().Get("Location"))
	}
	global := cookieByName(verify.Result().Cookies(), GlobalIdentityCookieName)
	if global == nil || !global.Secure || !global.HttpOnly || global.Value != "global" {
		t.Fatalf("global identity cookie=%+v", global)
	}

	app := apps.App{ID: "app-alpha", Slug: "alpha"}
	login := Login{IdentityBroker: &b}
	start := httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "https://alpha.apps.tiny.test/_tiny/auth/login?return=/projects/42%3Ftag%3Da%26tag%3Db%26next%3D%252Ffoo%253Fx%253D1", nil)
	r.Host = "alpha.apps.tiny.test"
	login.DispatchPreAuth(app, gateway.AppLogin, start, r)
	u, _ := url.Parse(start.Header().Get("Location"))
	handoff := u.Query().Get("handoff")
	appIdentity := httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "https://admin.apps.tiny.test/_tiny/identity?handoff="+url.QueryEscape(handoff), nil)
	r.Host = "admin.apps.tiny.test"
	r.AddCookie(global)
	h.ServeHTTP(appIdentity, r)
	if appIdentity.Code != http.StatusSeeOther || !strings.Contains(appIdentity.Header().Get("Location"), "alpha.apps.tiny.test/_tiny/auth/callback") || store.requests != 0 {
		t.Fatalf("global app handoff status=%d location=%q appOTPs=%d", appIdentity.Code, appIdentity.Header().Get("Location"), store.requests)
	}
}

func TestIdentityBrokerDashboardLogoutRequiresOriginAndCSRFThenRevokesGlobalIdentity(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	store := newBrokerStore(now)
	closed := 0
	b := IdentityBroker{Store: store, Now: func() time.Time { return now }, RevokeChildren: func([]persistence.AppSessionRef) { closed++ }}
	h := b.PlatformHandler(http.NotFoundHandler())
	csrf := &http.Cookie{Name: browseridentity.CSRFCookieName, Value: "gis_test.csrf-value"}
	identityCookie := &http.Cookie{Name: GlobalIdentityCookieName, Value: "global"}
	for _, tc := range []struct {
		name, origin string
		want         int
	}{
		{"missing-origin", "", http.StatusForbidden},
		{"cross-origin", "https://evil.test", http.StatusForbidden},
		{"same-origin", "https://admin.apps.tiny.test", http.StatusSeeOther},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "https://admin.apps.tiny.test/logout", strings.NewReader("csrf=gis_test.csrf-value"))
			r.Host = "admin.apps.tiny.test"
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			r.AddCookie(csrf)
			r.AddCookie(identityCookie)
			h.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("status=%d", w.Code)
			}
			if tc.want == http.StatusSeeOther {
				if w.Header().Get("Location") != "/login" || closed != 1 || !hasExpiredCookie(w.Result().Cookies(), GlobalIdentityCookieName) || !hasExpiredCookie(w.Result().Cookies(), browseridentity.CSRFCookieName) {
					t.Fatalf("logout location=%q closed=%d cookies=%+v", w.Header().Get("Location"), closed, w.Result().Cookies())
				}
			}
		})
	}
}

func TestIdentityBrokerBrowserBindingIsHostOnlyOpaqueAndNonAuthorizing(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	store := newBrokerStore(now)
	b := &IdentityBroker{Store: store, Domain: "apps.tiny.test", Now: func() time.Time { return now }}
	h, _, err := store.CreateIdentityHandoff(context.Background(), "app-alpha", "/", false, now)
	if err != nil {
		t.Fatal(err)
	}
	platform := b.PlatformHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) }))
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/_tiny/identity?handoff="+url.QueryEscape(h.ID), nil)
	r.Host = "tiny.test"
	platform.ServeHTTP(w, r)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `name="email"`) {
		t.Fatalf("initial identity page status=%d body=%q", w.Code, w.Body.String())
	}
	binding := browserBindingCookieFrom(t, w.Result().Cookies())
	if binding.Domain != "" || binding.Path != "/" || !binding.Secure || !binding.HttpOnly || binding.SameSite != http.SameSiteLaxMode || binding.Value == "" || strings.Contains(w.Body.String(), binding.Value) || binding.Expires.Sub(now) != browserBindingLifetime {
		t.Fatalf("unsafe binding cookie=%+v body=%q", binding, w.Body.String())
	}

	// A binding alone does not authenticate the viewer, even on the platform
	// host. The identity page remains the generic email form rather than
	// authorizing this handoff.
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/_tiny/identity?handoff="+url.QueryEscape(h.ID), nil)
	r.Host = "tiny.test"
	r.AddCookie(binding)
	platform.ServeHTTP(w, r)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `name="email"`) || w.Header().Get("Location") != "" || store.handoffs[h.ID].AuthorizedAt != nil {
		t.Fatalf("binding authenticated platform identity: status=%d location=%q handoff=%+v", w.Code, w.Header().Get("Location"), store.handoffs[h.ID])
	}

	// It is also neither a control-plane nor an app-host credential. Platform
	// routes outside the fixed broker namespace still delegate unchanged.
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	r.Host = "tiny.test"
	r.AddCookie(binding)
	platform.ServeHTTP(w, r)
	if w.Code != http.StatusTeapot {
		t.Fatalf("binding intercepted control route: status=%d", w.Code)
	}
	login := Login{IdentityBroker: b}
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/_tiny/auth/callback?handoff="+url.QueryEscape(h.ID), nil)
	r.Host = "alpha.apps.tiny.test"
	r.AddCookie(binding)
	login.DispatchPreAuth(apps.App{ID: "app-alpha", Slug: "alpha"}, gateway.AppIdentityCallback, w, r)
	if w.Code/100 == 3 || strings.Contains(w.Header().Get("Set-Cookie"), sessions.AppCookieName) {
		t.Fatalf("binding became app credential: status=%d headers=%v", w.Code, w.Header())
	}
}

func TestIdentityBrokerOTPRejectsMissingOrMismatchedBrowserBinding(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	store := newBrokerStore(now)
	b := &IdentityBroker{Store: store, Domain: "apps.tiny.test", Now: func() time.Time { return now }}
	h, _, err := store.CreateIdentityHandoff(context.Background(), "app-alpha", "/", false, now)
	if err != nil {
		t.Fatal(err)
	}
	platform := b.PlatformHandler(http.NotFoundHandler())
	post := func(path, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		r.Host = "tiny.test"
		r.Header.Set("Origin", "https://tiny.test")
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		for _, c := range cookies {
			r.AddCookie(c)
		}
		platform.ServeHTTP(w, r)
		return w
	}
	body := "handoff=" + url.QueryEscape(h.ID) + "&email=viewer%40example.com"
	if w := post("/_tiny/identity/otp", body); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Sign-in needs another try") || store.requests != 0 {
		t.Fatalf("missing binding request status=%d requests=%d body=%q", w.Code, store.requests, w.Body.String())
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/_tiny/identity?handoff="+url.QueryEscape(h.ID), nil)
	r.Host = "tiny.test"
	platform.ServeHTTP(w, r)
	binding := browserBindingCookieFrom(t, w.Result().Cookies())
	if w = post("/_tiny/identity/otp", body, binding); w.Code != http.StatusOK || store.requests != 1 {
		t.Fatalf("bound request status=%d requests=%d", w.Code, store.requests)
	}
	wrong := &http.Cookie{Name: BrowserBindingCookieName, Value: "bb_wrong"}
	verify := body + "&transaction=otp_1&code=123456"
	if w = post("/_tiny/identity/verify", verify, wrong); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Sign-in needs another try") || hasCookie(w.Result().Cookies(), GlobalIdentityCookieName) || store.handoffs[h.ID].AuthorizedAt != nil {
		t.Fatalf("mismatched binding verify status=%d cookies=%v handoff=%+v body=%q", w.Code, w.Result().Cookies(), store.handoffs[h.ID], w.Body.String())
	}
}

func TestBrokerStoreBindingGroupsConcurrentDifferentEmails(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	store := newBrokerStore(now)
	first, _, err := store.CreateIdentityHandoff(context.Background(), "app-alpha", "/", false, now)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := store.CreateIdentityHandoff(context.Background(), "app-beta", "/", false, now)
	if err != nil {
		t.Fatal(err)
	}
	const binding = "bb_same_browser"
	firstMessage, err := store.RequestIdentityOTP(context.Background(), first.ID, binding, "alice@example.com", "fp", []byte("key"), now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	secondMessage, err := store.RequestIdentityOTP(context.Background(), second.ID, binding, "bob@example.com", "fp", []byte("key"), now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	go func() {
		_, e := store.VerifyIdentityOTP(context.Background(), first.ID, binding, "alice@example.com", firstMessage.ID, firstMessage.Code, "", []byte("key"), now, 5)
		results <- e
	}()
	go func() {
		_, e := store.VerifyIdentityOTP(context.Background(), second.ID, binding, "bob@example.com", secondMessage.ID, secondMessage.Code, "", []byte("key"), now, 5)
		results <- e
	}()
	var accepted, denied int
	for range 2 {
		if <-results == nil {
			accepted++
		} else {
			denied++
		}
	}
	if accepted != 1 || denied != 1 {
		t.Fatalf("same browser binding accepted %d completions and denied %d; want one each", accepted, denied)
	}
}

func TestConfiguredIdentityBrokerFailsClosedInsteadOfLegacyAppLogin(t *testing.T) {
	l := Login{IdentityBroker: &IdentityBroker{Domain: "apps.tiny.test"}}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/_tiny/auth/login?return=%2F", nil)
	r.Host = "alpha.apps.tiny.test"
	l.DispatchPreAuth(apps.App{ID: "app-alpha", Slug: "alpha"}, gateway.AppLogin, w, r)
	if w.Code != http.StatusServiceUnavailable || strings.Contains(w.Body.String(), `name="email"`) {
		t.Fatalf("configured broker fallback status=%d body=%q", w.Code, w.Body.String())
	}
}

func TestPlatformLoginWithViewerIdentityAndNoDashboardRoleDoesNotLoop(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	store := dashboardDeniedBrokerStore{brokerStore: *newBrokerStore(now)}
	b := IdentityBroker{Store: &store, Now: func() time.Time { return now }}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "https://admin.apps.tiny.test/login", nil)
	r.Host = "admin.apps.tiny.test"
	r.AddCookie(&http.Cookie{Name: GlobalIdentityCookieName, Value: "global"})
	b.PlatformHandler(http.NotFoundHandler()).ServeHTTP(w, r)
	if w.Code != http.StatusOK || w.Header().Get("Location") != "" || !strings.Contains(w.Body.String(), "This account cannot use the dashboard.") || !strings.Contains(w.Body.String(), "viewer@example.com") {
		t.Fatalf("viewer dashboard login looped or disclosed unsafe state: status=%d location=%q body=%q", w.Code, w.Header().Get("Location"), w.Body.String())
	}
	var csrf *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == browseridentity.CSRFCookieName {
			csrf = c
		}
	}
	if csrf == nil || !csrf.HttpOnly || !strings.HasPrefix(csrf.Value, "gis_test.") {
		t.Fatalf("no-role page did not issue identity-bound CSRF cookie: %#v", csrf)
	}
}

func TestIdentityBrokerRotationSetsReplacementPlatformCookie(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	store := newBrokerStore(now)
	b := &IdentityBroker{Store: store, Domain: "apps.tiny.test", Now: func() time.Time { return now }}
	h, _, err := store.CreateIdentityHandoff(context.Background(), "app-alpha", "/", false, now)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/_tiny/identity?handoff="+url.QueryEscape(h.ID), nil)
	r.Host = "tiny.test"
	r.AddCookie(&http.Cookie{Name: GlobalIdentityCookieName, Value: "rotate"})
	b.PlatformHandler(http.NotFoundHandler()).ServeHTTP(w, r)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("rotation status=%d", w.Code)
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == GlobalIdentityCookieName && c.Value == "global" && c.Secure && c.HttpOnly {
			return
		}
	}
	t.Fatalf("rotation replacement cookie missing: %#v", w.Result().Cookies())
}

func TestIdentityBrokerClosesChildrenAfterRotatedTokenReplayRevocation(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	store := newBrokerStore(now)
	h, _, err := store.CreateIdentityHandoff(context.Background(), "app-alpha", "/", false, now)
	if err != nil {
		t.Fatal(err)
	}
	var closed []persistence.AppSessionRef
	b := &IdentityBroker{Store: store, Domain: "apps.tiny.test", Now: func() time.Time { return now }, RevokeChildren: func(refs []persistence.AppSessionRef) { closed = append(closed, refs...) }}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/_tiny/identity?handoff="+url.QueryEscape(h.ID), nil)
	r.Host = "tiny.test"
	r.AddCookie(&http.Cookie{Name: GlobalIdentityCookieName, Value: "replay"})
	b.PlatformHandler(http.NotFoundHandler()).ServeHTTP(w, r)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `name="email"`) || len(closed) != 1 || closed[0].AppID != "app-alpha" || w.Header().Get("Location") != "" || !expiredIdentityCookie(w.Result().Cookies()) {
		t.Fatalf("replay status=%d headers=%v closed=%+v body=%q", w.Code, w.Header(), closed, w.Body.String())
	}
}

func TestIdentityBrokerInvalidIdentityCookieClearsBeforeGenericReauth(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	store := newBrokerStore(now)
	h, _, err := store.CreateIdentityHandoff(context.Background(), "app-alpha", "/", false, now)
	if err != nil {
		t.Fatal(err)
	}
	b := &IdentityBroker{Store: store, Domain: "apps.tiny.test", Now: func() time.Time { return now }}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/_tiny/identity?handoff="+url.QueryEscape(h.ID), nil)
	r.Host = "tiny.test"
	r.AddCookie(&http.Cookie{Name: GlobalIdentityCookieName, Value: "invalid"})
	r.AddCookie(&http.Cookie{Name: BrowserBindingCookieName, Value: "bb_existing_profile"})
	b.PlatformHandler(http.NotFoundHandler()).ServeHTTP(w, r)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `name="email"`) || !expiredIdentityCookie(w.Result().Cookies()) || hasCookie(w.Result().Cookies(), BrowserBindingCookieName) || w.Header().Get("Location") != "" {
		t.Fatalf("invalid identity status=%d headers=%v body=%q", w.Code, w.Header(), w.Body.String())
	}
}

func TestIdentityBrokerIdentityPersistenceFailurePreservesCookieAndDeniesOTP(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	base := newBrokerStore(now)
	h, _, err := base.CreateIdentityHandoff(context.Background(), "app-alpha", "/", false, now)
	if err != nil {
		t.Fatal(err)
	}
	store := failingValidateBrokerStore{brokerStore: base}
	b := &IdentityBroker{Store: &store, Domain: "apps.tiny.test", Now: func() time.Time { return now }}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/_tiny/identity?handoff="+url.QueryEscape(h.ID), nil)
	r.Host = "tiny.test"
	r.AddCookie(&http.Cookie{Name: GlobalIdentityCookieName, Value: "global"})
	b.PlatformHandler(http.NotFoundHandler()).ServeHTTP(w, r)
	if w.Code != http.StatusServiceUnavailable || w.Header().Get("Set-Cookie") != "" || !strings.Contains(w.Body.String(), "Your access has not changed") || strings.Contains(w.Body.String(), `name="email"`) || base.requests != 0 {
		t.Fatalf("persistence failure status=%d headers=%v requests=%d body=%q", w.Code, w.Header(), base.requests, w.Body.String())
	}
}

func TestIdentityBrokerOTPPostRequiresExactPlatformOrigin(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	store := newBrokerStore(now)
	b := &IdentityBroker{Store: store, Domain: "apps.tiny.test", Now: func() time.Time { return now }}
	h, _, err := store.CreateIdentityHandoff(context.Background(), "app-alpha", "/", false, now)
	if err != nil {
		t.Fatal(err)
	}
	platform := b.PlatformHandler(http.NotFoundHandler())
	for _, test := range []struct {
		name, path, body string
	}{
		{name: "request-missing-origin", path: "/_tiny/identity/otp", body: "handoff=" + url.QueryEscape(h.ID) + "&email=viewer%40example.com"},
		{name: "verify-cross-origin", path: "/_tiny/identity/verify", body: "handoff=" + url.QueryEscape(h.ID) + "&email=viewer%40example.com&transaction=otp_1&code=123456"},
	} {
		t.Run(test.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(test.body))
			r.Host = "tiny.test"
			if test.name == "verify-cross-origin" {
				r.Header.Set("Origin", "https://evil.test")
			}
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			platform.ServeHTTP(w, r)
			if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Sign-in needs another try") || w.Header().Get("Set-Cookie") != "" {
				t.Fatalf("status=%d headers=%v body=%q", w.Code, w.Header(), w.Body.String())
			}
		})
	}
	if store.requests != 0 || store.handoffs[h.ID].AuthorizedAt != nil {
		t.Fatalf("cross-origin broker POST mutated store: requests=%d handoff=%+v", store.requests, store.handoffs[h.ID])
	}
}

func TestIdentityBrokerHandoffCreationIsSeparatelyRateLimited(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	limits := ratelimit.New([]byte("test-key"), ratelimit.Config{
		Request: ratelimit.Policy{Window: time.Minute, PerIP: 1, PerFingerprint: 1, PerEmail: 10, PerApp: 10, Global: 10},
		Verify:  ratelimit.Policy{Window: time.Minute, PerIP: 10, PerFingerprint: 10, PerEmail: 10, PerApp: 10, Global: 10},
		MaxKeys: 100,
	})
	limits.SetClock(func() time.Time { return now })
	store := newBrokerStore(now)
	b := &IdentityBroker{Store: store, Domain: "apps.tiny.test", RateLimits: limits, Now: func() time.Time { return now }}
	login := Login{IdentityBroker: b}
	for attempt := 0; attempt < 2; attempt++ {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/_tiny/auth/login?return=%2F", nil)
		r.Host = "alpha.apps.tiny.test"
		r.RemoteAddr = "203.0.113.11:1234"
		login.DispatchPreAuth(apps.App{ID: "app-alpha", Slug: "alpha"}, gateway.AppLogin, w, r)
		if attempt == 0 && w.Code != http.StatusSeeOther {
			t.Fatalf("first handoff status=%d", w.Code)
		}
		if attempt == 1 && w.Code != http.StatusServiceUnavailable {
			t.Fatalf("limited handoff status=%d body=%q", w.Code, w.Body.String())
		}
	}
	if got := len(store.handoffs); got != 1 {
		t.Fatalf("handoff limiter created %d durable handoffs, want 1", got)
	}
}

func TestIdentityBrokerGlobalLogoutFailsClosedOnPersistenceFailure(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	base := newBrokerStore(now)
	store := failingRevokeBrokerStore{brokerStore: base, err: errors.New("database unavailable")}
	var closed int
	b := &IdentityBroker{Store: &store, Now: func() time.Time { return now }, RevokeChildren: func([]persistence.AppSessionRef) { closed++ }}
	binding := &http.Cookie{Name: BrowserBindingCookieName, Value: "bb_stable_profile"}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/_tiny/identity/logout", nil)
	r.Host = "tiny.test"
	r.Header.Set("Origin", "https://tiny.test")
	r.AddCookie(&http.Cookie{Name: GlobalIdentityCookieName, Value: "global"})
	r.AddCookie(binding)
	b.PlatformHandler(http.NotFoundHandler()).ServeHTTP(w, r)
	if w.Code != http.StatusServiceUnavailable || w.Header().Get("Location") != "" || w.Header().Get("Set-Cookie") != "" || closed != 0 || !strings.Contains(w.Body.String(), "Your access has not changed") {
		t.Fatalf("failure logout status=%d headers=%v closed=%d body=%q", w.Code, w.Header(), closed, w.Body.String())
	}

	store.err = nil
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost, "/_tiny/identity/logout", nil)
	r.Host = "tiny.test"
	r.Header.Set("Origin", "https://tiny.test")
	r.AddCookie(&http.Cookie{Name: GlobalIdentityCookieName, Value: "global"})
	r.AddCookie(binding)
	b.PlatformHandler(http.NotFoundHandler()).ServeHTTP(w, r)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/" || closed != 1 || hasCookie(w.Result().Cookies(), BrowserBindingCookieName) {
		t.Fatalf("committed logout status=%d headers=%v closed=%d", w.Code, w.Header(), closed)
	}

	store.err = persistence.ErrIdentity
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost, "/_tiny/identity/logout", nil)
	r.Host = "tiny.test"
	r.Header.Set("Origin", "https://tiny.test")
	r.AddCookie(&http.Cookie{Name: GlobalIdentityCookieName, Value: "stale"})
	r.AddCookie(binding)
	b.PlatformHandler(http.NotFoundHandler()).ServeHTTP(w, r)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/" || closed != 2 || hasCookie(w.Result().Cookies(), BrowserBindingCookieName) {
		t.Fatalf("stale logout status=%d headers=%v closed=%d", w.Code, w.Header(), closed)
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == GlobalIdentityCookieName && c.MaxAge < 0 {
			return
		}
	}
	t.Fatalf("stale credential was not cleared: %#v", w.Result().Cookies())
}

func TestIdentityBrokerGlobalLogoutFallsBackToBrowserBinding(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	binding := &http.Cookie{Name: BrowserBindingCookieName, Value: "bb_profile"}
	logout := func(store IdentityStore, cookies ...*http.Cookie) (*httptest.ResponseRecorder, int) {
		closed := 0
		b := &IdentityBroker{Store: store, Now: func() time.Time { return now }, RevokeChildren: func([]persistence.AppSessionRef) { closed++ }}
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/_tiny/identity/logout", nil)
		r.Host = "tiny.test"
		r.Header.Set("Origin", "https://tiny.test")
		for _, c := range cookies {
			r.AddCookie(c)
		}
		b.PlatformHandler(http.NotFoundHandler()).ServeHTTP(w, r)
		return w, closed
	}

	t.Run("missing identity uses binding", func(t *testing.T) {
		store := &failingRevokeBrokerStore{brokerStore: newBrokerStore(now)}
		w, closed := logout(store, binding)
		if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/" || closed != 1 || hasCookie(w.Result().Cookies(), BrowserBindingCookieName) {
			t.Fatalf("binding fallback status=%d headers=%v closed=%d", w.Code, w.Header(), closed)
		}
	})
	t.Run("stale identity uses binding", func(t *testing.T) {
		store := &failingRevokeBrokerStore{brokerStore: newBrokerStore(now), err: persistence.ErrIdentity}
		w, closed := logout(store, &http.Cookie{Name: GlobalIdentityCookieName, Value: "stale"}, binding)
		if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/" || closed != 1 || hasCookie(w.Result().Cookies(), BrowserBindingCookieName) {
			t.Fatalf("stale fallback status=%d headers=%v closed=%d", w.Code, w.Header(), closed)
		}
	})
	t.Run("binding persistence error preserves cookies", func(t *testing.T) {
		store := &failingRevokeBrokerStore{brokerStore: newBrokerStore(now), err: persistence.ErrIdentity, bindingErr: errors.New("database unavailable")}
		w, closed := logout(store, &http.Cookie{Name: GlobalIdentityCookieName, Value: "stale"}, binding)
		if w.Code != http.StatusServiceUnavailable || w.Header().Get("Set-Cookie") != "" || closed != 0 || !strings.Contains(w.Body.String(), "Your access has not changed") {
			t.Fatalf("binding failure status=%d headers=%v closed=%d body=%q", w.Code, w.Header(), closed, w.Body.String())
		}
	})
	for _, test := range []struct {
		name     string
		cookies  []*http.Cookie
		identity error
	}{
		{name: "unknown binding with missing identity", cookies: []*http.Cookie{binding}},
		{name: "unknown binding with stale identity", cookies: []*http.Cookie{{Name: GlobalIdentityCookieName, Value: "stale"}, binding}, identity: persistence.ErrIdentity},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &failingRevokeBrokerStore{brokerStore: newBrokerStore(now), err: test.identity, bindingErr: persistence.ErrIdentity}
			w, closed := logout(store, test.cookies...)
			if w.Code != http.StatusOK || w.Header().Get("Location") != "" || w.Header().Get("Set-Cookie") != "" || closed != 0 || !strings.Contains(w.Body.String(), "Sign-in needs another try") {
				t.Fatalf("unknown binding logout status=%d headers=%v closed=%d body=%q", w.Code, w.Header(), closed, w.Body.String())
			}
		})
	}
	t.Run("no platform credential is not a successful logout", func(t *testing.T) {
		w, closed := logout(newBrokerStore(now))
		if w.Code != http.StatusOK || w.Header().Get("Location") != "" || w.Header().Get("Set-Cookie") != "" || closed != 0 || !strings.Contains(w.Body.String(), "Sign-in needs another try") {
			t.Fatalf("credential-free logout status=%d headers=%v closed=%d body=%q", w.Code, w.Header(), closed, w.Body.String())
		}
	})
}

func TestUniqueAppSessionRefs(t *testing.T) {
	refs := uniqueAppSessionRefs([]persistence.AppSessionRef{
		{AppID: "a", SessionID: "one"},
		{AppID: "a", SessionID: "one"},
		{AppID: "b", SessionID: "two"},
		{AppID: "", SessionID: "invalid"},
	})
	if len(refs) != 2 || refs[0] != (persistence.AppSessionRef{AppID: "a", SessionID: "one"}) || refs[1] != (persistence.AppSessionRef{AppID: "b", SessionID: "two"}) {
		t.Fatalf("deduplicated refs=%+v", refs)
	}
}

type failingRevokeBrokerStore struct {
	*brokerStore
	err        error
	bindingErr error
}

func (s *failingRevokeBrokerStore) RevokeIdentitySession(context.Context, string, time.Time) ([]persistence.AppSessionRef, error) {
	return []persistence.AppSessionRef{{AppID: "app-alpha", SessionID: "s-alpha"}}, s.err
}

func (s *failingRevokeBrokerStore) RevokeIdentityBrowserBinding(context.Context, string, time.Time) ([]persistence.AppSessionRef, error) {
	return []persistence.AppSessionRef{{AppID: "app-alpha", SessionID: "s-alpha"}}, s.bindingErr
}

type failingValidateBrokerStore struct{ *brokerStore }

func (*failingValidateBrokerStore) ValidateIdentitySession(context.Context, string, time.Time) (persistence.IdentityValidationResult, error) {
	return persistence.IdentityValidationResult{}, errors.New("database unavailable")
}

func expiredIdentityCookie(cookies []*http.Cookie) bool {
	for _, c := range cookies {
		if c.Name == GlobalIdentityCookieName && c.MaxAge < 0 {
			return true
		}
	}
	return false
}

func browserBindingCookieFrom(t *testing.T, cookies []*http.Cookie) *http.Cookie {
	t.Helper()
	for _, c := range cookies {
		if c.Name == BrowserBindingCookieName {
			return c
		}
	}
	t.Fatalf("browser binding cookie missing: %#v", cookies)
	return nil
}

func hasCookie(cookies []*http.Cookie, name string) bool {
	for _, c := range cookies {
		if c.Name == name {
			return true
		}
	}
	return false
}

func cookieByName(cookies []*http.Cookie, name string) *http.Cookie {
	for _, c := range cookies {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func hasExpiredCookie(cookies []*http.Cookie, name string) bool {
	for _, c := range cookies {
		if c.Name == name && c.MaxAge < 0 {
			return true
		}
	}
	return false
}

type brokerStore struct {
	mu                 sync.Mutex
	now                time.Time
	handoffs           map[string]persistence.IdentityHandoff
	states             map[string]string
	bindings           map[string]string
	completedBindings  map[string]string
	requests           int
	platformRequests   int
	platformChallenges map[string]brokerPlatformChallenge
}

type dashboardDeniedBrokerStore struct{ brokerStore }

func (*dashboardDeniedBrokerStore) AuthenticateDashboardIdentity(context.Context, string, time.Time) (persistence.DashboardIdentityResult, error) {
	return persistence.DashboardIdentityResult{}, persistence.ErrIdentity
}

type brokerPlatformChallenge struct{ binding, email string }

func newBrokerStore(now time.Time) *brokerStore {
	return &brokerStore{now: now, handoffs: map[string]persistence.IdentityHandoff{}, states: map[string]string{}, bindings: map[string]string{}, completedBindings: map[string]string{}, platformChallenges: map[string]brokerPlatformChallenge{}}
}
func (s *brokerStore) CreateIdentityHandoff(_ context.Context, app, ret string, force bool, now time.Time) (persistence.IdentityHandoff, string, error) {
	id := "ih_" + string(rune('a'+len(s.handoffs)))
	slug := strings.TrimPrefix(app, "app-")
	h := persistence.IdentityHandoff{ID: id, AppID: app, AppSlug: slug, ReturnPath: ret, ForceLogin: force, ExpiresAt: now.Add(time.Minute)}
	s.handoffs[id] = h
	state := "state_" + id
	s.states[id] = state
	return h, state, nil
}
func (s *brokerStore) GetIdentityHandoff(_ context.Context, id string, _ time.Time) (persistence.IdentityHandoff, error) {
	h, ok := s.handoffs[id]
	if !ok {
		return persistence.IdentityHandoff{}, errors.New("denied")
	}
	return h, nil
}
func (s *brokerStore) ForceIdentityHandoff(_ context.Context, id string, _ time.Time) error {
	h, e := s.GetIdentityHandoff(context.Background(), id, time.Time{})
	if e != nil {
		return e
	}
	h.ForceLogin = true
	s.handoffs[id] = h
	return nil
}
func (s *brokerStore) ValidateIdentitySession(_ context.Context, raw string, _ time.Time) (persistence.IdentityValidationResult, error) {
	if raw == "replay" {
		return persistence.IdentityValidationResult{Revoked: []persistence.AppSessionRef{{AppID: "app-alpha", SessionID: "s-alpha"}}, FamilyRevoked: true}, persistence.ErrIdentity
	}
	if raw != "global" && raw != "rotate" {
		return persistence.IdentityValidationResult{}, persistence.ErrIdentity
	}
	replacement := ""
	if raw == "rotate" {
		replacement = "global"
	}
	return persistence.IdentityValidationResult{Session: persistence.IdentitySession{ID: "gis_test", Identity: identity.Identity{ID: "viewer@example.com", Email: "viewer@example.com"}, ExpiresAt: s.now.Add(time.Hour)}, ReplacementToken: replacement}, nil
}
func (s *brokerStore) AuthorizeIdentityHandoff(_ context.Context, id, raw string, _ time.Time) (persistence.IdentityHandoff, persistence.IdentitySession, error) {
	h, e := s.GetIdentityHandoff(context.Background(), id, time.Time{})
	if e != nil || raw != "global" {
		return persistence.IdentityHandoff{}, persistence.IdentitySession{}, errors.New("denied")
	}
	validation, _ := s.ValidateIdentitySession(context.Background(), raw, time.Time{})
	session := validation.Session
	if h.AppID == "app-denied" {
		return persistence.IdentityHandoff{}, session, errors.New("denied")
	}
	t := s.now
	h.AuthorizedAt = &t
	s.handoffs[id] = h
	return h, session, nil
}
func (s *brokerStore) RequestIdentityOTP(_ context.Context, id, binding, email, _ string, _ []byte, _ time.Time, _ time.Duration) (*persistence.ChallengeMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if binding == "" {
		return nil, errors.New("denied")
	}
	if _, e := s.GetIdentityHandoff(context.Background(), id, time.Time{}); e != nil {
		return nil, e
	}
	if existing, ok := s.bindings[id]; ok && existing != binding {
		return nil, errors.New("denied")
	}
	s.bindings[id] = binding
	s.requests++
	return &persistence.ChallengeMessage{ID: "otp_" + string(rune('0'+s.requests)), Code: "123456", Email: email, AppID: s.handoffs[id].AppID}, nil
}
func (s *brokerStore) VerifyIdentityOTP(_ context.Context, id, binding, email, tx, code, old string, _ []byte, _ time.Time, _ int) (persistence.IdentityOTPResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, e := s.GetIdentityHandoff(context.Background(), id, time.Time{})
	if e != nil || code != "123456" || tx == "" || binding == "" || s.bindings[id] != binding {
		return persistence.IdentityOTPResult{}, errors.New("denied")
	}
	if previous := s.completedBindings[binding]; previous != "" && previous != email && !h.ForceLogin {
		return persistence.IdentityOTPResult{}, errors.New("denied")
	}
	s.completedBindings[binding] = email
	authorized := s.now
	h.AuthorizedAt = &authorized
	s.handoffs[id] = h
	session := persistence.IdentitySession{Identity: identity.Identity{ID: email, Email: email}, ExpiresAt: s.now.Add(time.Hour)}
	if h.ForceLogin && old == "global" {
		return persistence.IdentityOTPResult{Token: "global", Session: session, Revoked: []persistence.AppSessionRef{{AppID: "app-alpha", SessionID: "s-alpha"}}}, nil
	}
	return persistence.IdentityOTPResult{Token: "global", Session: session}, nil
}
func (s *brokerStore) ConsumeIdentityHandoff(_ context.Context, app, id, state string, now, expiry time.Time) (persistence.IdentityHandoff, string, sessions.Session, error) {
	h, e := s.GetIdentityHandoff(context.Background(), id, now)
	if e != nil || h.AppID != app || s.states[id] != state || h.AuthorizedAt == nil {
		return persistence.IdentityHandoff{}, "", sessions.Session{}, errors.New("denied")
	}
	return h, "app_" + id, sessions.Session{ID: "s_" + id, AppID: app, ExpiresAt: expiry}, nil
}
func (s *brokerStore) RevokeIdentitySession(_ context.Context, raw string, _ time.Time) ([]persistence.AppSessionRef, error) {
	if raw != "global" {
		return nil, errors.New("denied")
	}
	return []persistence.AppSessionRef{{AppID: "app-alpha", SessionID: "s-alpha"}}, nil
}
func (s *brokerStore) RevokeIdentityBrowserBinding(_ context.Context, raw string, _ time.Time) ([]persistence.AppSessionRef, error) {
	if raw == "" {
		return nil, persistence.ErrIdentity
	}
	return []persistence.AppSessionRef{{AppID: "app-alpha", SessionID: "s-alpha"}}, nil
}

func (s *brokerStore) RequestPlatformIdentityOTP(_ context.Context, binding, email, _ string, _ []byte, _ time.Time, _ time.Duration) (*persistence.PlatformIdentityChallenge, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if binding == "" || email == "" {
		return nil, errors.New("denied")
	}
	s.platformRequests++
	id := "pid_" + string(rune('0'+s.platformRequests))
	s.platformChallenges[id] = brokerPlatformChallenge{binding: binding, email: email}
	return &persistence.PlatformIdentityChallenge{ID: id, Code: "123456", Email: email}, nil
}

func (s *brokerStore) VerifyPlatformIdentityOTP(_ context.Context, binding, email, tx, code, _ string, _ bool, _ []byte, _ time.Time, _ int) (persistence.IdentityOTPResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.platformChallenges[tx]
	if !ok || c.binding != binding || c.email != email || code != "123456" {
		return persistence.IdentityOTPResult{}, errors.New("denied")
	}
	return persistence.IdentityOTPResult{Token: "global", Session: persistence.IdentitySession{Identity: identity.Identity{ID: email, Email: email}, ExpiresAt: s.now.Add(time.Hour)}}, nil
}

var _ identityOutbox = noopBrokerOutbox{}

type noopBrokerOutbox struct{}

func (noopBrokerOutbox) EnqueueOTP(context.Context, otp.Message) error { return nil }
