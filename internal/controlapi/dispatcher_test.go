package controlapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tinyhost/tiny/internal/compatibility"
	"github.com/tinyhost/tiny/internal/deployments"
	"github.com/tinyhost/tiny/internal/kv"
	"github.com/tinyhost/tiny/internal/otp"
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
func (s *svcFake) RevokeCurrentBearer(context.Context, Actor) error       { return s.err }
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
func (s *svcFake) DataKVList(context.Context, Actor, string, string, string, int) (any, error) {
	return nil, s.err
}
func (s *svcFake) DataKVGet(context.Context, Actor, string, string) (any, error) { return nil, s.err }
func (s *svcFake) DataKVSet(context.Context, Actor, string, string, json.RawMessage, *uint64, string) (any, error) {
	return nil, s.err
}
func (s *svcFake) DataKVDelete(context.Context, Actor, string, string, *uint64, string) (any, error) {
	return nil, s.err
}
func (s *svcFake) DataCollections(context.Context, Actor, string, string, int) (any, error) {
	return nil, s.err
}
func (s *svcFake) DataDocumentsList(context.Context, Actor, string, string, string, int) (any, error) {
	return nil, s.err
}
func (s *svcFake) DataDocumentGet(context.Context, Actor, string, string, string) (any, error) {
	return nil, s.err
}
func (s *svcFake) DataDocumentCreate(context.Context, Actor, string, string, json.RawMessage, string) (any, error) {
	return nil, s.err
}
func (s *svcFake) DataDocumentUpdate(context.Context, Actor, string, string, string, json.RawMessage, *uint64, string) (any, error) {
	return nil, s.err
}
func (s *svcFake) DataDocumentDelete(context.Context, Actor, string, string, string, *uint64, string) (any, error) {
	return nil, s.err
}
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
	if w := call(d, "POST", "/api/v1/apps/a/rollback", `{"deployment":"old"}`); w.Code != 404 {
		t.Fatalf("deferred rollback route = %d", w.Code)
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

func TestDispatcherActivationFailureIsSafeAndCarriesGatewayRequestID(t *testing.T) {
	a := &authFake{a: Actor{ID: "u", Active: true}}
	s := &svcFake{err: deployments.CertificateNotReady}
	d := Dispatcher{Auth: a, Service: s}
	r := httptest.NewRequest(http.MethodPost, "/api/v1/apps/a/deployments/dep/activate", nil)
	r.Header.Set("Idempotency-Key", "activation-key")
	w := httptest.NewRecorder()
	w.Header().Set("X-Request-ID", "req_test")
	d.ServeHTTP(w, r)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), `"code":"activation_certificate_not_ready"`) || !strings.Contains(w.Body.String(), `"request_id":"req_test"`) {
		t.Fatalf("activation error = %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "certificate") && !strings.Contains(w.Body.String(), "activation_certificate_not_ready") {
		t.Fatalf("unsafe activation detail: %s", w.Body.String())
	}
}

func TestDispatcherLogoutUsesAuthenticatedActorOnly(t *testing.T) {
	a := &authFake{a: Actor{ID: "u", CredentialID: "tok", Active: true}}
	s := &svcFake{}
	w := call(Dispatcher{Auth: a, Service: s}, http.MethodPost, "/api/v1/auth/logout", "")
	if w.Code != http.StatusNoContent || !a.seen {
		t.Fatalf("logout = %d", w.Code)
	}
	a.err = errors.New("denied")
	w = call(Dispatcher{Auth: a, Service: s}, http.MethodPost, "/api/v1/auth/logout", "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous logout = %d", w.Code)
	}
	a.err = nil
	s.err = errors.New("persistence failed")
	w = call(Dispatcher{Auth: a, Service: s}, http.MethodPost, "/api/v1/auth/logout", "")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("failed logout = %d", w.Code)
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

func TestCompatibilityDocumentAndClientDenial(t *testing.T) {
	d := Dispatcher{Compatibility: compatibility.Current("0.1.0")}
	w := call(d, http.MethodGet, "/api/v1/compatibility", "")
	if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "no-store" || !strings.Contains(w.Body.String(), `"server_version":"0.1.0"`) {
		t.Fatalf("compatibility response = %d %s", w.Code, w.Body.String())
	}
	a := &authFake{a: Actor{ID: "u", Active: true}}
	s := &svcFake{}
	d = Dispatcher{Auth: a, Service: s}
	r := httptest.NewRequest(http.MethodGet, "/api/v1/whoami", nil)
	r.Header.Set("X-Tiny-CLI-Version", "1.0.0")
	r.Header.Set("X-Tiny-Control-API-Version", "1")
	w = httptest.NewRecorder()
	d.ServeHTTP(w, r)
	if w.Code != http.StatusUpgradeRequired || !strings.Contains(w.Body.String(), "cli_version_incompatible") {
		t.Fatalf("incompatible CLI = %d %s", w.Code, w.Body.String())
	}
	if a.seen {
		t.Fatal("compatibility denial reached authentication")
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
	err      error
}

func (l *loginFake) RequestOTP(_ context.Context, _ string, _ string) (string, error) {
	l.requests++
	if l.err != nil {
		return "", l.err
	}
	return "login_test", nil
}
func (l *loginFake) VerifyOTP(_ context.Context, _, _ string) (string, error) {
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
	if w.Code != http.StatusAccepted || login.requests != 1 {
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

func TestDispatcherOTPDoesNotMintFallbackTransactionWhenIssuerFails(t *testing.T) {
	for _, tc := range []struct {
		name     string
		err      error
		category OTPIssuanceFailureCategory
	}{
		{"entropy", otp.Failure(otp.IssuanceEntropy), OTPIssuanceEntropy},
		{"begin", otp.Failure(otp.IssuancePersistenceBeginWriteLock), OTPIssuancePersistenceBeginWriteLock},
		{"invalidate", otp.Failure(otp.IssuancePersistenceInvalidate), OTPIssuancePersistenceInvalidate},
		{"eligibility", otp.Failure(otp.IssuancePersistenceEligibility), OTPIssuancePersistenceEligibility},
		{"insert", otp.Failure(otp.IssuancePersistenceInsert), OTPIssuancePersistenceInsert},
		{"commit", otp.Failure(otp.IssuancePersistenceCommit), OTPIssuancePersistenceCommit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			login := &loginFake{err: tc.err}
			var reported OTPIssuanceFailureCategory
			d := Dispatcher{Login: login, OTPIssuanceFailure: func(category OTPIssuanceFailureCategory) { reported = category }}
			r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/otp", strings.NewReader(`{"email":"deployer@example.com"}`))
			w := httptest.NewRecorder()
			d.ServeHTTP(w, r)
			if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), `"temporarily_unavailable"`) {
				t.Fatalf("issuer failure response = %d %s", w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "persistence") || strings.Contains(w.Body.String(), "login_") {
				t.Fatalf("issuer failure leaked internal state or fake transaction: %s", w.Body.String())
			}
			if reported != tc.category {
				t.Fatalf("issuer failure category=%q want=%q", reported, tc.category)
			}
		})
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

func TestDataResultKeepsQuotaDistinctFromAuthorizationDenial(t *testing.T) {
	w := httptest.NewRecorder()
	(Dispatcher{}).dataResult(w, nil, kv.ErrQuotaExceeded)
	if w.Code != http.StatusTooManyRequests || !strings.Contains(w.Body.String(), `"quota_exceeded"`) {
		t.Fatalf("quota response = %d %s", w.Code, w.Body.String())
	}
}

func TestUniqueJSONRejectsDuplicateMembersAndExcessiveNesting(t *testing.T) {
	if uniqueJSON([]byte(`{"outer":{"same":1,"same":2}}`)) {
		t.Fatal("nested duplicate member accepted")
	}
	deep := strings.Repeat("[", 66) + "0" + strings.Repeat("]", 66)
	if uniqueJSON([]byte(deep)) {
		t.Fatal("excessively nested JSON accepted")
	}
	if !uniqueJSON([]byte(`{"outer":[{"one":1},{"two":2}]}`)) {
		t.Fatal("valid bounded JSON rejected")
	}
}
