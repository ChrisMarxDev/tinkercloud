package controlapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/tinyhost/tiny/internal/ratelimit"
)

type uiAuth struct {
	actor Actor
	err   error
}

func (a uiAuth) AuthenticatePlatform(context.Context, *http.Request) (Actor, error) {
	return a.actor, a.err
}

type uiLogin struct {
	tx, token string
	err       error
	email     string
	channel   LoginChannel
	requests  int
}

func (l *uiLogin) RequestOTP(_ context.Context, email string, channel LoginChannel, _ string) (string, error) {
	l.email = email
	l.channel = channel
	l.requests++
	return l.tx, l.err
}

func TestPlatformUIRateLimitKeepsGenericLoginResponse(t *testing.T) {
	now := time.Unix(100, 0)
	limits := ratelimit.New([]byte("test-key"), ratelimit.Config{Request: ratelimit.Policy{Window: time.Minute, PerIP: 1, PerEmail: 10, PerApp: 10, Global: 10}, Verify: ratelimit.Policy{Window: time.Minute, PerIP: 10, PerEmail: 10, PerApp: 10, Global: 10}, MaxKeys: 100})
	limits.SetClock(func() time.Time { return now })
	l := &uiLogin{tx: "otp_tx"}
	p := Platform{Login: l, RateLimits: limits}
	first := uiRequest(t, p, http.MethodPost, "/login", "email=a%40example.test")
	second := uiRequest(t, p, http.MethodPost, "/login", "email=b%40example.test")
	if first.Code != http.StatusOK || second.Code != http.StatusOK || l.requests != 1 || !strings.Contains(first.Body.String(), "If that address is authorized") || !strings.Contains(second.Body.String(), "If that address is authorized") || strings.Contains(strings.ToLower(second.Body.String()), "rate") {
		t.Fatalf("unexpected rate response: first=%d second=%d requests=%d body=%q", first.Code, second.Code, l.requests, second.Body.String())
	}
}
func (l *uiLogin) VerifyOTP(_ context.Context, _, _ string, channel LoginChannel) (string, error) {
	l.channel = channel
	return l.token, l.err
}

type uiViews struct {
	value DashboardView
	err   error
	got   Actor
}

type uiActions struct {
	calls  []string
	err    error
	access AccessPolicyInput
}

func (a *uiActions) SetDeployerStatus(_ context.Context, actor Actor, email, status, _ string) error {
	a.calls = append(a.calls, "deployer:"+actor.Role+":"+email+":"+status)
	return a.err
}
func (a *uiActions) ReplaceAccess(_ context.Context, _ Actor, slug string, in AccessPolicyInput, _ string) error {
	a.calls = append(a.calls, "access:"+slug)
	a.access = in
	return a.err
}
func (a *uiActions) CreateToken(_ context.Context, _ Actor, slug string, _ TokenInput, _ string) (TokenResult, error) {
	a.calls = append(a.calls, "token:"+slug)
	return TokenResult{ID: "tok_safe", Token: "tiny_only_once", ExpiresAt: "later"}, a.err
}
func (a *uiActions) RevokeToken(_ context.Context, _ Actor, slug, token, _ string) error {
	a.calls = append(a.calls, "revoke:"+slug+":"+token)
	return a.err
}
func (a *uiActions) Rollback(_ context.Context, _ Actor, slug, deployment, _ string) error {
	a.calls = append(a.calls, "rollback:"+slug+":"+deployment)
	return a.err
}
func (a *uiActions) SetAppStatus(_ context.Context, _ Actor, slug, status, _ string) error {
	a.calls = append(a.calls, "app:"+slug+":"+status)
	return a.err
}
func (a *uiActions) DeleteApp(_ context.Context, _ Actor, slug, _ string) error {
	a.calls = append(a.calls, "delete:"+slug)
	return a.err
}

func (v *uiViews) Dashboard(_ context.Context, a Actor) (DashboardView, error) {
	v.got = a
	return v.value, v.err
}

func uiRequest(t *testing.T, p Platform, method, path, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, "https://tiny.test"+path, strings.NewReader(body))
	r.Host = "tiny.test"
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range cookies {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	p.ServeHTTP(w, r)
	return w
}

func TestPlatformUIAnonymousAndCrossRoleDenials(t *testing.T) {
	p := Platform{Auth: uiAuth{err: errors.New("denied")}}
	if w := uiRequest(t, p, http.MethodGet, "/", ""); w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/login" {
		t.Fatalf("anonymous dashboard: %d %q", w.Code, w.Header().Get("Location"))
	}
	if w := uiRequest(t, p, http.MethodPost, "/logout", "csrf=x"); w.Code != http.StatusForbidden {
		t.Fatalf("anonymous logout: %d", w.Code)
	}
	views := &uiViews{value: DashboardView{Deployers: []DashboardDeployer{{Email: "deployer@example.test", Status: "active"}}}}
	p = Platform{Auth: uiAuth{actor: Actor{ID: "d", Email: "deployer@example.test", Role: "deployer", Active: true}}, Views: views}
	w := uiRequest(t, p, http.MethodGet, "/", "")
	if w.Code != 200 || strings.Contains(w.Body.String(), "deployer@example.test</td>") || views.got.ID != "d" {
		t.Fatalf("deployer page disclosed operator data: %d %s", w.Code, w.Body.String())
	}
}

func TestPlatformUILoginAndControlCookie(t *testing.T) {
	l := &uiLogin{tx: "otp_tx", token: "tiny_control_secret"}
	p := Platform{Login: l, Now: func() time.Time { return time.Unix(0, 0) }}
	w := uiRequest(t, p, http.MethodPost, "/login", "email=person%40example.test")
	if w.Code != 200 || l.email != "person@example.test" || l.channel != BrowserLoginChannel || !strings.Contains(w.Body.String(), "otp_tx") {
		t.Fatalf("request: %d %q", w.Code, w.Body.String())
	}
	w = uiRequest(t, p, http.MethodPost, "/login/verify", "transaction=otp_tx&code=123456")
	if w.Code != http.StatusSeeOther || l.channel != BrowserLoginChannel || w.Header().Get("Location") != "/" {
		t.Fatalf("verify: %d", w.Code)
	}
	var control *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == ControlCookieName {
			control = c
		}
	}
	if control == nil || !control.Secure || !control.HttpOnly || control.SameSite != http.SameSiteLaxMode || control.Path != "/" {
		t.Fatalf("bad control cookie: %#v", control)
	}
	if strings.Contains(w.Body.String(), l.token) {
		t.Fatal("token rendered")
	}
}

func TestPlatformUIUsesEmbeddedNativeDesignSystem(t *testing.T) {
	p := Platform{}
	w := uiRequest(t, p, http.MethodGet, "/login", "")
	body := w.Body.String()
	for _, want := range []string{
		`class="tiny-auth-card"`,
		`class="tiny-brand__mark"`,
		`--tiny-canvas:`,
		`--tiny-primary:`,
		`TinyUI`,
		`autocomplete="email"`,
		`Authorized deployers only`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("login missing design-system contract %q: %s", want, body)
		}
	}
	for _, forbidden := range []string{`src="http`, `href="http`, `fetch(`, `document.cookie`} {
		if strings.Contains(strings.ToLower(body), forbidden) {
			t.Fatalf("login contains remote or executable dependency %q", forbidden)
		}
	}

	views := &uiViews{value: DashboardView{
		Apps: []DashboardApp{{
			Slug:   "alpha",
			Status: "active",
			Access: DashboardAccess{Mode: "private", Revision: 2},
		}},
		Health: []DashboardHealth{{
			Name:   "email",
			State:  "degraded",
			Detail: "Existing sessions continue; new sign-ins are unavailable.",
		}},
	}}
	p = Platform{
		Auth:  uiAuth{actor: Actor{ID: "u", Email: "owner@example.test", Role: "deployer", Active: true}},
		Views: views,
	}
	w = uiRequest(t, p, http.MethodGet, "/dashboard", "")
	body = w.Body.String()
	for _, want := range []string{
		`data-state="active"`,
		`data-state="degraded"`,
		`data-tiny-app-filter hidden`,
		`aria-controls="app-list"`,
		`id="app-list"`,
		`data-tiny-app-list`,
		`data-tiny-app-card`,
		`data-tiny-app-slug="alpha"`,
		`data-tiny-app-status="active"`,
		`data-tiny-app-filter-count role="status" aria-live="polite"`,
		`No matching apps.`,
		`<details class="tiny-details">`,
		`Replace current policy`,
		`Type <code>delete:alpha</code>`,
		`Your VPS remains the recovery authority`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("dashboard missing design-system contract %q: %s", want, body)
		}
	}
	for _, forbidden := range []string{`fetch(`, `innerHTML`, `localStorage`, `sessionStorage`, `document.cookie`} {
		if strings.Contains(strings.ToLower(body), strings.ToLower(forbidden)) {
			t.Fatalf("dashboard filter contains unsafe browser state or request primitive %q", forbidden)
		}
	}
}

func TestPlatformUIDashboardEscapesDescriptionsAndUsesOnlyStableGatewayLink(t *testing.T) {
	p := Platform{Auth: uiAuth{actor: Actor{ID: "u", Role: "deployer", Active: true}}, Views: &uiViews{value: DashboardView{Apps: []DashboardApp{{Slug: "alpha", Status: "active", Description: `<script>alert("x")</script>`, StableURL: "https://alpha.apps.example.test/", Access: DashboardAccess{Mode: "private", Revision: 1}, Releases: []DashboardRelease{{ID: "d", State: "active", Description: "Immutable summary"}}}}}}}
	w := uiRequest(t, p, http.MethodGet, "/dashboard", "")
	body := w.Body.String()
	for _, want := range []string{`data-tiny-app-description="&lt;script&gt;alert(&#34;x&#34;)&lt;/script&gt;"`, `href="https://alpha.apps.example.test/"`, `target="_blank"`, `rel="noopener noreferrer"`, `aria-label="Open alpha in a new tab"`, `Immutable summary`} {
		if !strings.Contains(body, want) {
			t.Fatalf("dashboard missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, `<script>alert`) || strings.Contains(body, "release_hash") {
		t.Fatalf("unsafe dashboard description/link: %s", body)
	}
}

func TestPlatformUIErrorStatesAreStyledAndKeepSafeStatusCodes(t *testing.T) {
	p := Platform{}
	for _, tc := range []struct {
		name, method, path string
		want               int
	}{
		{"not found", http.MethodGet, "/missing", http.StatusNotFound},
		{"method", http.MethodDelete, "/login", http.StatusMethodNotAllowed},
		{"logout denied", http.MethodPost, "/logout", http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := uiRequest(t, p, tc.method, tc.path, "")
			body := w.Body.String()
			if w.Code != tc.want || !strings.Contains(body, `class="tiny-auth-card"`) || !strings.Contains(body, "Safe next step") || !strings.Contains(body, `role="alert"`) {
				t.Fatalf("status=%d body=%s", w.Code, body)
			}
		})
	}
}

func TestPlatformUIStalePolicyConflictIsStyledAndDoesNotExposeState(t *testing.T) {
	actions := &uiActions{err: ErrPolicyRevision}
	p := Platform{Auth: uiAuth{actor: Actor{ID: "u", Role: "deployer", Active: true}}, Actions: actions}
	w := uiRequest(t, p, http.MethodGet, "/", "")
	var csrf *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == controlCSRFCookie {
			csrf = c
		}
	}
	form := url.Values{"csrf": {csrf.Value}, "expected_revision": {"1"}}
	r := httptest.NewRequest(http.MethodPost, "https://tiny.test/apps/alpha/access", strings.NewReader(form.Encode()))
	r.Host = "tiny.test"
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", "https://tiny.test")
	r.AddCookie(&http.Cookie{Name: ControlCookieName, Value: "opaque"})
	r.AddCookie(csrf)
	w = httptest.NewRecorder()
	p.ServeHTTP(w, r)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "Policy changed") || !strings.Contains(w.Body.String(), "Refresh dashboard") || strings.Contains(w.Body.String(), "alpha") {
		t.Fatalf("stale policy response: %d %s", w.Code, w.Body.String())
	}
}

func TestPlatformUICSRFAndSecretRedaction(t *testing.T) {
	views := &uiViews{value: DashboardView{Apps: []DashboardApp{{Slug: "alpha", Tokens: []DashboardToken{{ID: "tok_safe", Scopes: []string{"app:read"}}}}}, Health: []DashboardHealth{{Name: "email", State: "degraded", Detail: "provider unavailable"}}}}
	p := Platform{Auth: uiAuth{actor: Actor{ID: "u", Email: "owner@example.test", Role: "operator", Active: true}}, Views: views}
	w := uiRequest(t, p, http.MethodGet, "/", "")
	var csrf *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == controlCSRFCookie {
			csrf = c
		}
	}
	if csrf == nil || !csrf.Secure || csrf.HttpOnly || strings.Contains(w.Body.String(), "secret_hash") || strings.Contains(w.Body.String(), "tiny_") {
		t.Fatalf("unsafe dashboard: cookies=%#v body=%s", w.Result().Cookies(), w.Body.String())
	}
	control := &http.Cookie{Name: ControlCookieName, Value: "opaque"}
	if w = uiRequest(t, p, http.MethodPost, "/logout", "csrf="+url.QueryEscape(csrf.Value), control, csrf); w.Code != http.StatusForbidden {
		t.Fatalf("originless csrf accepted: %d", w.Code)
	}
	r := httptest.NewRequest(http.MethodPost, "https://tiny.test/logout", strings.NewReader("csrf="+url.QueryEscape(csrf.Value)))
	r.Host = "tiny.test"
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", "https://evil.test")
	r.AddCookie(control)
	r.AddCookie(csrf)
	w = httptest.NewRecorder()
	p.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("cross-origin csrf accepted: %d", w.Code)
	}
	r.Header.Set("Origin", "https://tiny.test")
	w = httptest.NewRecorder()
	p.ServeHTTP(w, r)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("same-origin logout denied: %d", w.Code)
	}
}

func TestPlatformUIMutationsRequireActorOriginCSRFAndConfirmation(t *testing.T) {
	actions := &uiActions{}
	p := Platform{Auth: uiAuth{actor: Actor{ID: "u", Email: "owner@example.test", Role: "deployer", Active: true}}, Actions: actions}
	// Bootstrap the double-submit token through the authenticated page.
	w := uiRequest(t, p, http.MethodGet, "/", "")
	var csrf *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == controlCSRFCookie {
			csrf = c
		}
	}
	if csrf == nil {
		t.Fatal("missing csrf")
	}
	control := &http.Cookie{Name: ControlCookieName, Value: "opaque"}
	request := func(path, form, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "https://tiny.test"+path, strings.NewReader(form+"&csrf="+url.QueryEscape(csrf.Value)))
		r.Host = "tiny.test"
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Origin", origin)
		r.AddCookie(control)
		r.AddCookie(csrf)
		x := httptest.NewRecorder()
		p.ServeHTTP(x, r)
		return x
	}
	if w = request("/apps/alpha/delete", "confirmation=delete%3Awrong", "https://tiny.test"); w.Code != http.StatusBadRequest || len(actions.calls) != 0 {
		t.Fatalf("wrong delete confirmation: %d %#v", w.Code, actions.calls)
	}
	if w = request("/apps/alpha/delete", "confirmation=delete%3Aalpha", "https://evil.test"); w.Code != http.StatusForbidden || len(actions.calls) != 0 {
		t.Fatalf("cross origin mutation: %d %#v", w.Code, actions.calls)
	}
	if w = request("/apps/alpha/delete", "confirmation=delete%3Aalpha", "https://tiny.test"); w.Code != http.StatusSeeOther || len(actions.calls) != 1 || actions.calls[0] != "delete:alpha" {
		t.Fatalf("valid deletion form: %d %#v", w.Code, actions.calls)
	}
	if w = request("/deployers/a%40example.test/suspend", "confirmation=suspend%3Aa%40example.test", "https://tiny.test"); w.Code != http.StatusForbidden || len(actions.calls) != 1 {
		t.Fatalf("deployer escalated: %d %#v", w.Code, actions.calls)
	}
}

func TestPlatformUIOperatorCanAuthorizeFreshDeployer(t *testing.T) {
	actions := &uiActions{}
	p := Platform{Auth: uiAuth{actor: Actor{ID: "operator", Email: "root@example.test", Role: "operator", Active: true}}, Actions: actions}
	w := uiRequest(t, p, http.MethodGet, "/dashboard", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `action="/deployers/authorize"`) || !strings.Contains(w.Body.String(), `name="email"`) || !strings.Contains(w.Body.String(), `autocomplete="email"`) {
		t.Fatalf("authorization form missing: %d %s", w.Code, w.Body.String())
	}
	var csrf *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == controlCSRFCookie {
			csrf = c
		}
	}
	if csrf == nil {
		t.Fatal("missing csrf")
	}
	post := func(email string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "https://tiny.test/deployers/authorize", strings.NewReader("email="+url.QueryEscape(email)+"&csrf="+url.QueryEscape(csrf.Value)))
		r.Host = "tiny.test"
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Origin", "https://tiny.test")
		r.AddCookie(&http.Cookie{Name: ControlCookieName, Value: "opaque"})
		r.AddCookie(csrf)
		out := httptest.NewRecorder()
		p.ServeHTTP(out, r)
		return out
	}
	if w = post("New@Example.test"); w.Code != http.StatusSeeOther || len(actions.calls) != 1 || actions.calls[0] != "deployer:operator:New@example.test:active" {
		t.Fatalf("fresh authorization: %d %#v", w.Code, actions.calls)
	}
	if w = post("not-an-email"); w.Code != http.StatusBadRequest || len(actions.calls) != 1 {
		t.Fatalf("malformed email authorized: %d %#v", w.Code, actions.calls)
	}
}

func TestPlatformUITokenIsDisplayedOnlyByCreateResponse(t *testing.T) {
	actions := &uiActions{}
	p := Platform{Auth: uiAuth{actor: Actor{ID: "u", Role: "deployer", Active: true}}, Actions: actions, Views: &uiViews{value: DashboardView{Apps: []DashboardApp{{Slug: "alpha"}}}}}
	w := uiRequest(t, p, http.MethodGet, "/", "")
	var csrf *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == controlCSRFCookie {
			csrf = c
		}
	}
	r := httptest.NewRequest(http.MethodPost, "https://tiny.test/apps/alpha/tokens", strings.NewReader("scopes=app%3Aread&expires_in_seconds=3600&csrf="+url.QueryEscape(csrf.Value)))
	r.Host = "tiny.test"
	r.Header.Set("Origin", "https://tiny.test")
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(&http.Cookie{Name: ControlCookieName, Value: "opaque"})
	r.AddCookie(csrf)
	w = httptest.NewRecorder()
	p.ServeHTTP(w, r)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "tiny_only_once") {
		t.Fatalf("create token response: %d %s", w.Code, w.Body.String())
	}
	w = uiRequest(t, p, http.MethodGet, "/", "")
	if strings.Contains(w.Body.String(), "tiny_only_once") {
		t.Fatal("raw token persisted in dashboard")
	}
}

func TestPlatformUIDashboardShowsAndRoundTripsCurrentAccessPolicy(t *testing.T) {
	access := DashboardAccess{Mode: "private", Revision: 7, Emails: []string{"alice@example.test"}, Domains: []string{"example.test"}}
	actions := &uiActions{}
	p := Platform{Auth: uiAuth{actor: Actor{ID: "u", Role: "deployer", Active: true}}, Views: &uiViews{value: DashboardView{Apps: []DashboardApp{{Slug: "alpha", Status: "active", Access: access}}}}, Actions: actions}
	w := uiRequest(t, p, http.MethodGet, "/", "")
	if w.Code != http.StatusOK {
		t.Fatal(w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{"Current policy: private (revision 7)", "alice@example.test", "example.test", "You are always an implicit viewer.", "A future deployment can replace this policy", "name=\"emails\"", "name=\"domains\""} {
		if !strings.Contains(body, want) {
			t.Fatalf("dashboard missing %q: %s", want, body)
		}
	}
	var csrf *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == controlCSRFCookie {
			csrf = c
		}
	}
	if csrf == nil {
		t.Fatal("missing csrf")
	}
	form := url.Values{"emails": {"Alice@Example.test\nbob@example.test"}, "domains": {"Example.test"}, "expected_revision": {"7"}, "confirm_broadening": {"confirm"}, "csrf": {csrf.Value}}
	r := httptest.NewRequest(http.MethodPost, "https://tiny.test/apps/alpha/access", strings.NewReader(form.Encode()))
	r.Host = "tiny.test"
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", "https://tiny.test")
	r.AddCookie(&http.Cookie{Name: ControlCookieName, Value: "opaque"})
	r.AddCookie(csrf)
	w = httptest.NewRecorder()
	p.ServeHTTP(w, r)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/dashboard?notice=policy_replaced" || len(actions.calls) != 1 || actions.calls[0] != "access:alpha" {
		t.Fatalf("replace access = %d %#v", w.Code, actions.calls)
	}
	if actions.access.Mode != "private" || actions.access.ExpectedRevision != 7 || !actions.access.ConfirmBroadening || strings.Join(actions.access.Allow.Emails, ",") != "Alice@example.test,bob@example.test" || strings.Join(actions.access.Allow.Domains, ",") != "example.test" {
		t.Fatalf("access input = %#v", actions.access)
	}
}

func TestPlatformUIDashboardLabelsEmptyAccessAsOwnerOnly(t *testing.T) {
	p := Platform{Auth: uiAuth{actor: Actor{ID: "u", Role: "deployer", Active: true}}, Views: &uiViews{value: DashboardView{Apps: []DashboardApp{{Slug: "alpha", Access: DashboardAccess{Mode: "private", Revision: 1}}}}}}
	w := uiRequest(t, p, http.MethodGet, "/", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "This app is owner-only: no additional email or domain viewers are allowed.") {
		t.Fatalf("owner-only dashboard = %d %s", w.Code, w.Body.String())
	}
}

func TestPlatformUIDashboardUnavailablePolicyIsNotEditableEmptyState(t *testing.T) {
	p := Platform{Auth: uiAuth{actor: Actor{ID: "u", Role: "deployer", Active: true}}, Views: &uiViews{err: errors.New("policy unavailable")}}
	w := uiRequest(t, p, http.MethodGet, "/", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "dashboard read model") || strings.Contains(w.Body.String(), "Replace current policy") {
		t.Fatalf("unavailable dashboard = %d %s", w.Code, w.Body.String())
	}
}

func TestPlatformUIAccessRejectsMissingOrMalformedExpectedRevision(t *testing.T) {
	actions := &uiActions{}
	p := Platform{Auth: uiAuth{actor: Actor{ID: "u", Role: "deployer", Active: true}}, Actions: actions}
	w := uiRequest(t, p, http.MethodGet, "/", "")
	var csrf *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == controlCSRFCookie {
			csrf = c
		}
	}
	for _, revision := range []string{"", "1st", "0"} {
		form := url.Values{"csrf": {csrf.Value}, "expected_revision": {revision}}
		r := httptest.NewRequest(http.MethodPost, "https://tiny.test/apps/alpha/access", strings.NewReader(form.Encode()))
		r.Host = "tiny.test"
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Origin", "https://tiny.test")
		r.AddCookie(&http.Cookie{Name: ControlCookieName, Value: "opaque"})
		r.AddCookie(csrf)
		w = httptest.NewRecorder()
		p.ServeHTTP(w, r)
		if w.Code != http.StatusBadRequest || len(actions.calls) != 0 {
			t.Fatalf("revision %q = %d %#v", revision, w.Code, actions.calls)
		}
	}
}
