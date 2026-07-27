package compose

import (
	"bytes"
	"context"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tinyhost/tiny/internal/apps"
	"github.com/tinyhost/tiny/internal/gateway"
	"github.com/tinyhost/tiny/internal/identity"
	"github.com/tinyhost/tiny/internal/otp"
	"github.com/tinyhost/tiny/internal/policies"
	"github.com/tinyhost/tiny/internal/ratelimit"
	"github.com/tinyhost/tiny/internal/sessions"
)

func TestAppAuthTemplatesKeepTrustedScriptInsideBody(t *testing.T) {
	for name, tpl := range map[string]any{
		"login": appLoginTemplate,
		"code":  appCodeTemplate,
		"retry": appRetryTemplate,
	} {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			var err error
			switch page := tpl.(type) {
			case *template.Template:
				if name == "code" {
					err = page.Execute(&out, map[string]string{"Email": "a@example.com", "Transaction": "otp", "Return": "/"})
				} else {
					err = page.Execute(&out, "/")
				}
			}
			if err != nil || !strings.Contains(out.String(), "<script>") || strings.Contains(out.String(), "</body>\n<script") {
				t.Fatalf("template %s invalid script placement: %v %q", name, err, out.String())
			}
		})
	}
}

type fakeAtomic struct{}

func (fakeAtomic) Request(context.Context, string, string, bool, string) (string, error) {
	return "otp_test", nil
}
func (fakeAtomic) VerifyAndCreateSession(context.Context, string, string, string, string, time.Time) (string, error) {
	return "session", nil
}

type denyAtomic struct {
	eligible bool
	requests int
}

func (f *denyAtomic) Request(_ context.Context, _ string, _ string, eligible bool, _ string) (string, error) {
	f.eligible = eligible
	f.requests++
	return "otp_fake", nil
}
func (*denyAtomic) VerifyAndCreateSession(context.Context, string, string, string, string, time.Time) (string, error) {
	return "", otp.ErrInvalid
}

func TestLoginFormFlow(t *testing.T) {
	p := &policies.MemoryStore{Policies: map[string]policies.Policy{"a": {AppID: "a", Valid: true, Emails: map[string]struct{}{"a@example.com": {}}}}}
	l := Login{Atomic: fakeAtomic{}, Policies: p, SessionTTL: time.Hour}
	a := apps.App{ID: "a"}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/_tiny/auth/login?return=/safe", nil)
	l.DispatchPreAuth(a, gateway.AppLogin, w, r)
	if w.Code != http.StatusOK ||
		!strings.Contains(w.Body.String(), `for="email"`) ||
		!strings.Contains(w.Body.String(), `class="tiny-auth-card"`) ||
		!strings.Contains(w.Body.String(), `--tiny-primary:`) ||
		!strings.Contains(w.Body.String(), `(function ()`) ||
		strings.Contains(w.Body.String(), `<script src=`) {
		t.Fatal(w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost, "/_tiny/auth/otp", strings.NewReader("email=a%40example.com&return=%2Fsafe"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	l.DispatchPreAuth(a, gateway.AppOTPRequest, w, r)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "otp_test") {
		t.Fatal(w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost, "/_tiny/auth/verify", strings.NewReader("email=a%40example.com&transaction=otp_test&code=123456&return=%2Fsafe"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	l.DispatchPreAuth(a, gateway.AppOTPVerify, w, r)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/safe" {
		t.Fatal(w.Code, w.Header())
	}
	c := w.Result().Cookies()[0]
	if !c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode {
		t.Fatal(c)
	}
}

func TestLoginFormWrongCodeNoCookie(t *testing.T) {
	p := &policies.MemoryStore{Policies: map[string]policies.Policy{"a": {AppID: "a", Valid: true, Emails: map[string]struct{}{"a@example.com": {}}}}}
	l := Login{Atomic: &denyAtomic{}, Policies: p}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/_tiny/auth/verify", strings.NewReader("email=a%40example.com&transaction=otp_fake&code=bad"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	l.DispatchPreAuth(apps.App{ID: "a"}, gateway.AppOTPVerify, w, r)
	if w.Code != http.StatusUnauthorized || w.Header().Get("Set-Cookie") != "" || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/html") || !strings.Contains(w.Body.String(), `role="alert"`) || !strings.Contains(w.Body.String(), "Start again") {
		t.Fatal(w.Code, w.Header())
	}
}

func TestLoginFormFailuresAreStyledButJSONStaysStructured(t *testing.T) {
	l := Login{Atomic: &denyAtomic{}, Policies: &policies.MemoryStore{}}
	form := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/_tiny/auth/verify", strings.NewReader("email=a%40example.com&transaction=otp&code=bad&return=https%3A%2F%2Fevil.test"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	l.DispatchPreAuth(apps.App{ID: "a"}, gateway.AppOTPVerify, form, r)
	if form.Code != http.StatusUnauthorized || !strings.HasPrefix(form.Header().Get("Content-Type"), "text/html") || !strings.Contains(form.Body.String(), `role="alert"`) || strings.Contains(form.Body.String(), "evil.test") {
		t.Fatalf("form failure status=%d headers=%v body=%q", form.Code, form.Header(), form.Body.String())
	}
	jsonOut := httptest.NewRecorder()
	jr := httptest.NewRequest(http.MethodPost, "/_tiny/auth/verify", strings.NewReader(`{"email":"a@example.com"}`))
	jr.Header.Set("Content-Type", "application/json")
	l.DispatchPreAuth(apps.App{ID: "a"}, gateway.AppOTPVerify, jsonOut, jr)
	if jsonOut.Code != http.StatusUnauthorized || !strings.HasPrefix(jsonOut.Header().Get("Content-Type"), "application/json") || !strings.Contains(jsonOut.Body.String(), `"not_authorized"`) || strings.Contains(jsonOut.Body.String(), "<html") {
		t.Fatalf("json failure status=%d headers=%v body=%q", jsonOut.Code, jsonOut.Header(), jsonOut.Body.String())
	}
}

func TestLoginFormIneligibleHasGenericCodeForm(t *testing.T) {
	fake := &denyAtomic{}
	l := Login{Atomic: fake}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/_tiny/auth/otp", strings.NewReader("email=nobody%40example.com&return=%2F"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	l.DispatchPreAuth(apps.App{ID: "a"}, gateway.AppOTPRequest, w, r)
	body := w.Body.String()
	if w.Code != http.StatusOK || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/html") || !strings.Contains(body, `name="transaction"`) || !strings.Contains(body, "otp_fake") {
		t.Fatal(w.Code, w.Header(), body)
	}
	if fake.eligible || strings.Contains(strings.ToLower(body), "ineligible") || strings.Contains(strings.ToLower(body), "provider") {
		t.Fatal(body)
	}
	if !strings.Contains(body, "This message is the same for every address.") ||
		!strings.Contains(body, `autocomplete="one-time-code"`) {
		t.Fatalf("generic styled verification state missing: %s", body)
	}
}

func TestLoginRequestRateLimitDoesNotIssueAndKeepsGenericResponse(t *testing.T) {
	now := time.Unix(100, 0)
	limits := ratelimit.New([]byte("test-key"), ratelimit.Config{Request: ratelimit.Policy{Window: time.Minute, PerIP: 1, PerEmail: 10, PerApp: 10, Global: 10}, Verify: ratelimit.Policy{Window: time.Minute, PerIP: 10, PerEmail: 10, PerApp: 10, Global: 10}, MaxKeys: 100})
	limits.SetClock(func() time.Time { return now })
	fake := &denyAtomic{}
	l := Login{Atomic: fake, RateLimits: limits}
	request := func(email string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/_tiny/auth/otp", strings.NewReader("email="+email))
		r.RemoteAddr = "203.0.113.1:9"
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=utf-8")
		l.DispatchPreAuth(apps.App{ID: "a"}, gateway.AppOTPRequest, w, r)
		return w
	}
	if w := request("a%40example.com"); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "otp_fake") {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := request("b%40example.com"); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "name=\"transaction\"") || strings.Contains(w.Body.String(), "rate") || fake.requests != 1 {
		t.Fatal(w.Code, w.Body.String(), fake.requests)
	}
}

func TestAppLogoutRequiresSameOriginAndHasSafeAccountSwitchReturn(t *testing.T) {
	store := sessions.NewMemoryStore()
	issue := func() string {
		token, _, err := store.Create("app_a", identity.Identity{ID: "viewer", Email: "viewer@example.com"}, time.Now().Add(time.Hour))
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
	token := issue()
	if w := request("https://evil.test", "%2Fsafe", token); w.Code != http.StatusUnauthorized {
		t.Fatalf("cross-origin logout status=%d", w.Code)
	}
	if _, err := store.Validate(context.Background(), "app_a", token, time.Now()); err != nil {
		t.Fatal("cross-origin logout mutated session")
	}
	if w := request("https://alpha.apps.tiny.test", "https%3A%2F%2Fevil.test", token); w.Code != http.StatusUnauthorized {
		t.Fatalf("open return logout status=%d", w.Code)
	}
	if _, err := store.Validate(context.Background(), "app_a", token, time.Now()); err != nil {
		t.Fatal("open-return logout mutated session")
	}
	if w := request("https://alpha.apps.tiny.test", "%2Fsafe%3Ftab%3D1", token); w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/_tiny/auth/login?return=%2Fsafe%3Ftab%3D1" {
		t.Fatalf("account switch status=%d location=%q", w.Code, w.Header().Get("Location"))
	}
	if _, err := store.Validate(context.Background(), "app_a", token, time.Now()); err == nil {
		t.Fatal("same-origin logout retained app session")
	}
	other, _, err := store.Create("app_b", identity.Identity{ID: "viewer", Email: "viewer@example.com"}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if w := request("https://alpha.apps.tiny.test", "%2F", other); w.Code != http.StatusSeeOther {
		t.Fatalf("wrong-app cookie logout status=%d", w.Code)
	}
	if _, err := store.Validate(context.Background(), "app_b", other, time.Now()); err != nil {
		t.Fatal("app-a logout revoked a different app session")
	}
	if got := request("https://alpha.apps.tiny.test", "%2F", issue()).Result().Cookies()[0]; got.MaxAge >= 0 || !got.Secure || !got.HttpOnly {
		t.Fatalf("logout cookie=%+v", got)
	}
}
