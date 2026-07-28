package controlapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tinyhost/tiny/internal/ratelimit"
)

type authFake struct {
	a    Actor
	err  error
	seen bool
}

func (a *authFake) AuthenticateControl(context.Context, *http.Request) (Actor, error) {
	a.seen = true
	return a.a, a.err
}

type svcFake struct {
	actor       Actor
	err         error
	upload      func(Upload)
	accessCalls int
}

func (s *svcFake) Whoami(_ context.Context, a Actor) any {
	s.actor = a
	return map[string]string{"email": a.Email}
}
func (s *svcFake) Apps(context.Context, Actor) any                        { return []any{} }
func (s *svcFake) CreateApp(context.Context, Actor, string, string) error { return s.err }
func (s *svcFake) Access(context.Context, Actor, string) (any, error)     { return nil, s.err }
func (s *svcFake) ReplaceAccess(context.Context, Actor, string, AccessPolicyInput, string) error {
	s.accessCalls++
	return s.err
}
func (s *svcFake) Releases(context.Context, Actor, string) (any, error)   { return nil, s.err }
func (s *svcFake) DeleteApp(context.Context, Actor, string, string) error { return s.err }
func (s *svcFake) Tokens(context.Context, Actor, string) (any, error)     { return nil, s.err }
func (s *svcFake) CreateToken(context.Context, Actor, string, TokenInput, string) (TokenResult, error) {
	return TokenResult{}, s.err
}
func (s *svcFake) RevokeToken(context.Context, Actor, string, string, string) error { return s.err }

func (s *svcFake) CreateDeployment(_ context.Context, _ Actor, _ string, _ string, u Upload) (any, error) {
	if s.upload != nil {
		s.upload(u)
	}
	return nil, s.err
}
func (s *svcFake) Activate(context.Context, Actor, string, string, string) (ActivationResult, error) {
	return ActivationResult{}, s.err
}
func (s *svcFake) Rollback(context.Context, Actor, string, string, string) error { return s.err }
func call(d Dispatcher, m, p, b string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(m, p, strings.NewReader(b))
	w := httptest.NewRecorder()
	d.ServeHTTP(w, r)
	return w
}
func TestDispatcherDenyPaths(t *testing.T) {
	a := &authFake{a: Actor{ID: "u", Email: "u@test", Active: true}}
	s := &svcFake{err: errors.New("denied")}
	d := Dispatcher{Auth: a, Service: s}
	if w := call(d, "GET", "/nope", ""); w.Code != 404 || a.seen {
		t.Fatal(w.Code)
	}
	a.err = errors.New("deny")
	if w := call(d, "GET", "/api/v1/apps", ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	a.err = nil
	a.a.Active = false
	if w := call(d, "GET", "/api/v1/apps", ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	a.a.Active = true
	if w := call(d, "POST", "/api/v1/apps", `{"slug":"x"}`); w.Code != 409 {
		t.Fatal(w.Code)
	}
	if w := call(d, "POST", "/api/v1/apps", `{"slug":`); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w := call(d, "PUT", "/api/v1/apps/a/access", `{}`); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w := call(d, "PUT", "/api/v1/apps/a/access", `{"mode":"private","allow":{"emails":["a@example.com"],"domains":[],"unknown":true}}`); w.Code != 400 || s.accessCalls != 0 {
		t.Fatalf("unknown access shape = %d calls=%d", w.Code, s.accessCalls)
	}
	s.err = ErrPolicyRevision
	r := httptest.NewRequest("PUT", "/api/v1/apps/a/access", strings.NewReader(`{"mode":"private","expected_revision":1,"confirm_broadening":true,"allow":{"emails":["a@example.com"],"domains":[]}}`))
	r.Header.Set("Idempotency-Key", "policy-conflict")
	w := httptest.NewRecorder()
	d.ServeHTTP(w, r)
	if w.Code != http.StatusConflict || s.accessCalls != 1 {
		t.Fatalf("policy conflict = %d calls=%d", w.Code, s.accessCalls)
	}
	if w := call(d, "GET", "/api/v1/unknown", ""); w.Code != 404 {
		t.Fatal(w.Code)
	}
}
func TestDispatcherSuccessPropagatesActor(t *testing.T) {
	a := &authFake{a: Actor{ID: "u", Email: "u@test", Active: true}}
	s := &svcFake{}
	w := call(Dispatcher{Auth: a, Service: s}, "GET", "/api/v1/whoami", "")
	if w.Code != 200 || s.actor.ID != "u" || strings.Contains(w.Body.String(), "error") {
		t.Fatal(w.Code, w.Body.String())
	}
}
func TestDispatcherBodyAndUploadBoundaries(t *testing.T) {
	a := &authFake{a: Actor{ID: "u", Active: true}}
	s := &svcFake{}
	d := Dispatcher{Auth: a, Service: s}
	if w := call(d, "POST", "/api/v1/apps", `{"slug":"`+strings.Repeat("x", MaxBody+10)+`"}`); w.Code != 400 {
		t.Fatal(w.Code)
	}
	r := httptest.NewRequest("POST", "/api/v1/apps/a/deployments", strings.NewReader("x"))
	r.Header.Set("Idempotency-Key", "k")
	r.Header.Set("Content-Type", "application/gzip")
	r.ContentLength = MaxBody + 1
	w := httptest.NewRecorder()
	d.ServeHTTP(w, r)
	if w.Code != 413 {
		t.Fatal(w.Code)
	}
	r = httptest.NewRequest("POST", "/api/v1/apps/a/deployments", strings.NewReader("x"))
	r.Header.Set("Idempotency-Key", "k")
	r.Header.Set("Content-Type", "text/plain")
	w = httptest.NewRecorder()
	d.ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
	s.err = errors.New("SUPER_SECRET")
	if w := call(d, "POST", "/api/v1/apps", `{"slug":"x"}`); strings.Contains(w.Body.String(), "SUPER_SECRET") {
		t.Fatal(w.Body.String())
	}
}

func TestDispatcherArchiveUploadUsesConfiguredStreamingLimit(t *testing.T) {
	const limit = int64(2 << 20)
	a := &authFake{a: Actor{ID: "u", Active: true}}
	s := &svcFake{upload: func(u Upload) {
		if _, err := io.Copy(io.Discard, u.Reader); err != nil {
			t.Fatal(err)
		}
	}}
	d := Dispatcher{Auth: a, Service: s, ArchiveUploadBytes: limit}
	within := strings.Repeat("x", 1<<20+1)
	r := httptest.NewRequest(http.MethodPost, "/api/v1/apps/a/deployments", strings.NewReader(within))
	r.Header.Set("Idempotency-Key", "k")
	r.Header.Set("Content-Type", "application/gzip")
	w := httptest.NewRecorder()
	d.ServeHTTP(w, r)
	if w.Code != http.StatusAccepted {
		t.Fatalf("configured limit rejected >1MiB upload: %d", w.Code)
	}

	over := strings.Repeat("x", int(limit)+1)
	r = httptest.NewRequest(http.MethodPost, "/api/v1/apps/a/deployments", strings.NewReader(over))
	r.Header.Set("Idempotency-Key", "k2")
	r.Header.Set("Content-Type", "application/gzip")
	w = httptest.NewRecorder()
	d.ServeHTTP(w, r)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("limit+1 status = %d", w.Code)
	}

	// Chunked bodies have no trustworthy declared size. The wrapper permits one
	// evidence byte, then returns the same 413 after the service discards the
	// private staging candidate.
	r = httptest.NewRequest(http.MethodPost, "/api/v1/apps/a/deployments", strings.NewReader(over))
	r.ContentLength = -1
	r.Header.Set("Idempotency-Key", "k3")
	r.Header.Set("Content-Type", "application/gzip")
	w = httptest.NewRecorder()
	d.ServeHTTP(w, r)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("chunked limit+1 status = %d", w.Code)
	}
}

type loginFake struct {
	requests int
	channel  LoginChannel
}

func (l *loginFake) RequestOTP(_ context.Context, _ string, channel LoginChannel, _ string) (string, error) {
	l.requests++
	l.channel = channel
	return "login_test", nil
}
func (l *loginFake) VerifyOTP(_ context.Context, _, _ string, channel LoginChannel) (string, error) {
	l.channel = channel
	return "token", nil
}

func TestDispatcherOTPRateLimitedBeforeLoginProvider(t *testing.T) {
	now := time.Unix(100, 0)
	limits := ratelimit.New([]byte("test-key"), ratelimit.Config{Request: ratelimit.Policy{Window: time.Minute, PerIP: 1, PerEmail: 10, PerApp: 10, Global: 10}, Verify: ratelimit.Policy{Window: time.Minute, PerIP: 1, PerEmail: 10, PerApp: 10, Global: 10}, MaxKeys: 100})
	limits.SetClock(func() time.Time { return now })
	login := &loginFake{}
	d := Dispatcher{Login: login, RateLimits: limits}
	first := httptest.NewRequest(http.MethodPost, "/api/v1/auth/otp", strings.NewReader(`{"email":"a@example.com"}`))
	first.RemoteAddr = "203.0.113.1:8"
	w := httptest.NewRecorder()
	d.ServeHTTP(w, first)
	if w.Code != http.StatusAccepted || login.requests != 1 || login.channel != CLILoginChannel {
		t.Fatal(w.Code, login.requests)
	}
	second := httptest.NewRequest(http.MethodPost, "/api/v1/auth/otp", strings.NewReader(`{"email":"b@example.com"}`))
	second.RemoteAddr = "203.0.113.1:8"
	w = httptest.NewRecorder()
	d.ServeHTTP(w, second)
	if w.Code != http.StatusTooManyRequests || !strings.Contains(w.Body.String(), `"rate_limited"`) || login.requests != 1 {
		t.Fatal(w.Code, w.Body.String(), login.requests)
	}
}

func TestDispatcherAppLifecycleDenials(t *testing.T) {
	a := &authFake{a: Actor{ID: "u", Active: true}}
	s := &svcFake{err: errors.New("denied")}
	d := Dispatcher{Auth: a, Service: s}
	if w := call(d, "GET", "/api/v1/apps/a/releases", ""); w.Code != 403 {
		t.Fatalf("release denial = %d", w.Code)
	}
	if w := call(d, "DELETE", "/api/v1/apps/a", `{"confirmation":"delete:a"}`); w.Code != 400 {
		t.Fatalf("missing idempotency key = %d", w.Code)
	}
	r := httptest.NewRequest("DELETE", "/api/v1/apps/a", strings.NewReader(`{"confirmation":"delete:other"}`))
	r.Header.Set("Idempotency-Key", "delete")
	w := httptest.NewRecorder()
	d.ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatalf("mismatched confirmation = %d", w.Code)
	}
	r = httptest.NewRequest("DELETE", "/api/v1/apps/a", strings.NewReader(`{"confirmation":"delete:a"}`))
	r.Header.Set("Idempotency-Key", "delete")
	w = httptest.NewRecorder()
	d.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("service denial = %d", w.Code)
	}
}
