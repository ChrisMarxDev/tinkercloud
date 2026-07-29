// Package controlapi exposes the platform control namespace. It deliberately
// accepts only an actor derived by Authenticator, never actor/app ownership data
// from JSON or query input.
package controlapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/tinyhost/tiny/internal/identity"
	"github.com/tinyhost/tiny/internal/ratelimit"
	"io"
	"net/http"
	"sort"
	"strings"
)

const MaxBody = 1 << 20

var ErrPolicyRevision = errors.New("policy revision conflict")
var ErrDeployerRevision = errors.New("deployer allowlist revision conflict")

type Actor struct {
	ID, Email, Role string
	// CredentialID is server-derived from the bearer row and is used only by
	// narrow self-revocation. It is never accepted from the request body.
	CredentialID string
	Active       bool
}
type Authenticator interface {
	AuthenticateControl(context.Context, *http.Request) (Actor, error)
}
type Service interface {
	Whoami(context.Context, Actor) any
	RevokeCurrentBearer(context.Context, Actor) error
	Apps(context.Context, Actor) any
	CreateApp(context.Context, Actor, string, string) error
	Access(context.Context, Actor, string) (any, error)
	ReplaceAccess(context.Context, Actor, string, AccessPolicyInput, string) error
	Releases(context.Context, Actor, string) (any, error)
	DeleteApp(context.Context, Actor, string, string) error
	Tokens(context.Context, Actor, string) (any, error)
	CreateToken(context.Context, Actor, string, TokenInput, string) (TokenResult, error)
	RevokeToken(context.Context, Actor, string, string, string) error
	CreateDeployment(context.Context, Actor, string, string, Upload) (any, error)
	Activate(context.Context, Actor, string, string, string) (ActivationResult, error)
}
type TokenInput struct {
	Scopes           []string `json:"scopes"`
	ExpiresInSeconds int64    `json:"expires_in_seconds"`
}
type TokenResult struct {
	ID        string   `json:"id"`
	Token     string   `json:"token"`
	Scopes    []string `json:"scopes"`
	ExpiresAt string   `json:"expires_at"`
}
type AccessPolicyInput struct {
	Mode              string `json:"mode"`
	ExpectedRevision  uint64 `json:"expected_revision"`
	ConfirmBroadening bool   `json:"confirm_broadening"`
	Allow             struct {
		Emails  []string `json:"emails"`
		Domains []string `json:"domains"`
	} `json:"allow"`
}
type Upload struct {
	Reader        io.Reader
	ContentLength int64
	ContentType   string
}
type ActivationResult struct {
	DeploymentID string `json:"deployment_id"`
	URL          string `json:"url"`
	// AppSuffix is server-derived deployment evidence. It lets a deployer verify
	// the returned app hostname without assuming the control-plane host is also
	// the wildcard app suffix.
	AppSuffix            string `json:"app_suffix"`
	PolicyReady          bool   `json:"policy_ready"`
	TLSReady             bool   `json:"tls_ready"`
	AnonymousDenied      bool   `json:"anonymous_denied"`
	AuthenticatedHealthy bool   `json:"authenticated_healthy"`
}
type Dispatcher struct {
	Auth       Authenticator
	Service    Service
	Login      Login
	RateLimits *ratelimit.Limiter
	// ArchiveUploadBytes is deliberately independent from MaxBody, which caps
	// JSON control payloads. The composition root supplies the validated
	// configured archive limit; an unset dispatcher fails conservatively at the
	// JSON limit for tests and incomplete wiring.
	ArchiveUploadBytes int64
}
type LoginChannel string

const (
	BrowserLoginChannel LoginChannel = "browser"
	CLILoginChannel     LoginChannel = "cli"
)

type Login interface {
	RequestOTP(context.Context, string, LoginChannel, string) (string, error)
	VerifyOTP(context.Context, string, string, LoginChannel) (string, error)
}

// PlatformAuthenticator is intentionally distinct from the CLI bearer-token
// authenticator. It is used only by the platform-host HTML surface, whose
// credential is a host-only control cookie.
type PlatformAuthenticator interface {
	AuthenticatePlatform(context.Context, *http.Request) (Actor, error)
}

// DashboardReader is an optional, safe read-model seam for the HTML UI. It
// contains no token secret/hash, provider credential, or release path.
type DashboardReader interface {
	Dashboard(context.Context, Actor) (DashboardView, error)
}

type DashboardView struct {
	Apps                   []DashboardApp
	ActiveDeployerEmails   []string
	ActiveDeployerRevision string
	Audit                  []DashboardAudit
	Health                 []DashboardHealth
}
type DashboardApp struct {
	Slug, Status, Description, StableURL string
	Access                               DashboardAccess
	Releases                             []DashboardRelease
	Tokens                               []DashboardToken
}
type DashboardAccess struct {
	Mode     string
	Revision uint64
	Emails   []string
	Domains  []string
}
type DashboardToken struct {
	ID, ExpiresAt string
	LastUsedAt    *string
	Scopes        []string
	Revoked       bool
}
type DashboardRelease struct{ ID, ReleaseHash, State, Description, CreatedAt, VerifiedAt, ActivatedAt string }
type DashboardAudit struct{ OccurredAt, Action, Outcome, Target string }
type DashboardHealth struct{ Name, State, Detail string }
type envelope struct {
	Error *apiError `json:"error,omitempty"`
}
type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (d Dispatcher) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/v1/version" && r.Method == "GET" {
		write(w, 200, map[string]int{"api_version": 1}, "")
		return
	}
	if r.URL.Path == "/api/v1/auth/otp" && r.Method == "POST" {
		var v struct {
			Email string `json:"email"`
		}
		if !body(r, &v) || v.Email == "" {
			write(w, 400, nil, "validation_failed")
			return
		}
		if d.RateLimits != nil && !d.RateLimits.AllowRequest(ratelimit.OTPRequest, r, v.Email, "control") {
			write(w, http.StatusTooManyRequests, nil, "rate_limited")
			return
		}
		tx := ""
		if d.Login != nil {
			tx, _ = d.Login.RequestOTP(r.Context(), v.Email, CLILoginChannel, ratelimit.RequestFingerprint(r))
			if tx != "" && d.RateLimits != nil {
				d.RateLimits.BindTransactionRequest(tx, v.Email, r)
			}
		}
		if tx == "" {
			b := make([]byte, 16)
			_, _ = rand.Read(b)
			tx = "login_" + hex.EncodeToString(b)
		}
		write(w, 202, map[string]string{"transaction": tx}, "")
		return
	}
	if r.URL.Path == "/api/v1/auth/verify" && r.Method == "POST" {
		var v struct {
			Transaction string `json:"transaction"`
			Code        string `json:"code"`
		}
		if !body(r, &v) || v.Transaction == "" || v.Code == "" || d.Login == nil {
			write(w, 401, nil, "not_authorized")
			return
		}
		if d.RateLimits != nil && !d.RateLimits.AllowTransactionRequest(ratelimit.OTPVerify, r, v.Transaction, "control") {
			write(w, http.StatusTooManyRequests, nil, "rate_limited")
			return
		}
		token, e := d.Login.VerifyOTP(r.Context(), v.Transaction, v.Code, CLILoginChannel)
		if e != nil || token == "" {
			write(w, 401, nil, "not_authorized")
			return
		}
		write(w, 200, map[string]any{"token": token, "api_version": 1}, "")
		return
	}
	if !strings.HasPrefix(r.URL.Path, "/api/v1/") {
		write(w, 404, nil, "not_found")
		return
	}
	if d.Auth == nil || d.Service == nil {
		write(w, 503, nil, "unavailable")
		return
	}
	a, e := d.Auth.AuthenticateControl(r.Context(), r)
	if e != nil || !a.Active {
		write(w, 401, nil, "not_authorized")
		return
	}
	p := strings.TrimPrefix(r.URL.Path, "/api/v1/")
	switch {
	case r.Method == "GET" && p == "whoami":
		write(w, 200, d.Service.Whoami(r.Context(), a), "")
	case r.Method == "POST" && p == "auth/logout":
		if err := d.Service.RevokeCurrentBearer(r.Context(), a); err != nil {
			write(w, http.StatusServiceUnavailable, nil, "unavailable")
			return
		}
		write(w, http.StatusNoContent, nil, "")
	case r.Method == "GET" && p == "apps":
		write(w, 200, d.Service.Apps(r.Context(), a), "")
	case r.Method == "POST" && p == "apps":
		var v struct {
			Slug string `json:"slug"`
		}
		if !body(r, &v) || v.Slug == "" {
			write(w, 400, nil, "validation_failed")
			return
		}
		if err := d.Service.CreateApp(r.Context(), a, v.Slug, key(r)); err != nil {
			write(w, 409, nil, "conflict")
			return
		}
		write(w, 201, map[string]string{"slug": v.Slug}, "")
	default:
		parts := strings.Split(p, "/")
		if len(parts) < 2 || parts[0] != "apps" {
			write(w, 404, nil, "not_found")
			return
		}
		app := parts[1]
		tail := strings.Join(parts[2:], "/")
		switch {
		case r.Method == "GET" && tail == "releases":
			value, err := d.Service.Releases(r.Context(), a, app)
			if err != nil {
				write(w, 403, nil, "not_authorized")
				return
			}
			write(w, 200, value, "")
		case r.Method == "DELETE" && tail == "":
			var v struct {
				Confirmation string `json:"confirmation"`
			}
			if key(r) == "" || !body(r, &v) || v.Confirmation != "delete:"+app {
				write(w, 400, nil, "validation_failed")
				return
			}
			if err := d.Service.DeleteApp(r.Context(), a, app, key(r)); err != nil {
				write(w, 403, nil, "not_authorized")
				return
			}
			write(w, 204, nil, "")
		case r.Method == "GET" && tail == "access":
			value, err := d.Service.Access(r.Context(), a, app)
			if err != nil {
				write(w, 403, nil, "not_authorized")
				return
			}
			write(w, 200, value, "")
		case r.Method == "PUT" && tail == "access":
			var v AccessPolicyInput
			if !body(r, &v) || !validAccess(&v) || key(r) == "" {
				write(w, 400, nil, "validation_failed")
				return
			}
			if e := d.Service.ReplaceAccess(r.Context(), a, app, v, key(r)); e != nil {
				if errors.Is(e, ErrPolicyRevision) {
					write(w, http.StatusConflict, nil, "conflict")
					return
				}
				write(w, 403, nil, "not_authorized")
				return
			}
			write(w, 200, map[string]bool{"ok": true}, "")
		case r.Method == "GET" && tail == "tokens":
			value, err := d.Service.Tokens(r.Context(), a, app)
			if err != nil {
				write(w, 403, nil, "not_authorized")
				return
			}
			write(w, 200, value, "")
		case r.Method == "POST" && tail == "tokens":
			var v TokenInput
			if key(r) == "" || !body(r, &v) || !validTokenInput(v) {
				write(w, 400, nil, "validation_failed")
				return
			}
			out, e := d.Service.CreateToken(r.Context(), a, app, v, key(r))
			if e != nil {
				write(w, 403, nil, "not_authorized")
				return
			}
			write(w, 201, out, "")
		case r.Method == "DELETE" && strings.HasPrefix(tail, "tokens/") && validTokenID(strings.TrimPrefix(tail, "tokens/")):
			if key(r) == "" {
				write(w, 400, nil, "validation_failed")
				return
			}
			if e := d.Service.RevokeToken(r.Context(), a, app, strings.TrimPrefix(tail, "tokens/"), key(r)); e != nil {
				write(w, 403, nil, "not_authorized")
				return
			}
			write(w, 204, nil, "")
		case r.Method == "POST" && tail == "deployments":
			if key(r) == "" || r.ContentLength == 0 || (r.Header.Get("Content-Type") != "application/gzip" && r.Header.Get("Content-Type") != "application/zip") {
				write(w, 400, nil, "validation_failed")
				return
			}
			limit := d.archiveUploadLimit()
			if r.ContentLength > limit {
				write(w, 413, nil, "validation_failed")
				return
			}
			// Content-Length is attacker-controlled and may be absent. Limit the
			// streaming body to one byte beyond the configured boundary so the
			// domain can discard staging while the transport returns 413.
			stream := &uploadReader{r: r.Body, remaining: limit + 1, limit: limit}
			out, e := d.Service.CreateDeployment(r.Context(), a, app, key(r), Upload{Reader: stream, ContentLength: r.ContentLength, ContentType: r.Header.Get("Content-Type")})
			if stream.tooLarge {
				write(w, 413, nil, "validation_failed")
				return
			}
			if e != nil {
				write(w, 409, nil, "conflict")
				return
			}
			write(w, 202, out, "")
		case r.Method == "POST" && strings.HasPrefix(tail, "deployments/") && strings.HasSuffix(tail, "/activate"):
			id := strings.TrimSuffix(strings.TrimPrefix(tail, "deployments/"), "/activate")
			if id == "" || key(r) == "" {
				write(w, 400, nil, "validation_failed")
				return
			}
			out, e := d.Service.Activate(r.Context(), a, app, id, key(r))
			if e != nil {
				write(w, 409, nil, "conflict")
				return
			}
			write(w, 200, out, "")
		default:
			write(w, 404, nil, "not_found")
		}
	}
}
func (d Dispatcher) archiveUploadLimit() int64 {
	if d.ArchiveUploadBytes > 0 {
		return d.ArchiveUploadBytes
	}
	return MaxBody
}

// uploadReader bounds unknown-length request bodies without granting the
// deployment service an unbounded network stream. It exposes at most limit+1
// bytes: the extra byte is only evidence for a deterministic 413 response.
type uploadReader struct {
	r         io.Reader
	remaining int64
	limit     int64
	seen      int64
	tooLarge  bool
}

func (u *uploadReader) Read(p []byte) (int, error) {
	if u.remaining <= 0 {
		return 0, io.EOF
	}
	if int64(len(p)) > u.remaining {
		p = p[:int(u.remaining)]
	}
	n, err := u.r.Read(p)
	u.remaining -= int64(n)
	u.seen += int64(n)
	if u.seen > u.limit {
		u.tooLarge = true
	}
	return n, err
}
func validTokenInput(v TokenInput) bool {
	if len(v.Scopes) == 0 || v.ExpiresInSeconds < 60 || v.ExpiresInSeconds > 31536000 {
		return false
	}
	allowed := map[string]bool{"app:read": true, "app:create": true, "app:delete": true, "deploy:create": true, "deploy:activate": true, "access:read": true, "access:write": true, "token:create": true, "token:revoke": true}
	seen := map[string]bool{}
	for _, s := range v.Scopes {
		if !allowed[s] || seen[s] {
			return false
		}
		seen[s] = true
	}
	return true
}
func validTokenID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}
func validAccess(v *AccessPolicyInput) bool {
	if v.Mode != "private" || v.ExpectedRevision == 0 {
		return false
	}
	emails := map[string]bool{}
	for i, x := range v.Allow.Emails {
		n, e := identity.Normalize(x)
		if e != nil || emails[n] {
			return false
		}
		emails[n] = true
		v.Allow.Emails[i] = n
	}
	domains := map[string]bool{}
	for i, x := range v.Allow.Domains {
		d := strings.ToLower(strings.TrimSpace(x))
		if !validDomain(d) || domains[d] {
			return false
		}
		domains[d] = true
		v.Allow.Domains[i] = d
	}
	sort.Strings(v.Allow.Emails)
	sort.Strings(v.Allow.Domains)
	return true
}
func validDomain(d string) bool {
	if d == "" || strings.ContainsAny(d, "@/ \t\r\n") {
		return false
	}
	for _, p := range strings.Split(d, ".") {
		if p == "" || strings.HasPrefix(p, "-") || strings.HasSuffix(p, "-") {
			return false
		}
		for _, c := range p {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return true
}
func key(r *http.Request) string { return r.Header.Get("Idempotency-Key") }
func body(r *http.Request, v any) bool {
	de := json.NewDecoder(io.LimitReader(r.Body, MaxBody+1))
	de.DisallowUnknownFields()
	return de.Decode(v) == nil && de.Decode(&struct{}{}) == io.EOF
}
func write(w http.ResponseWriter, status int, v any, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if code != "" {
		v = envelope{Error: &apiError{code, "Request could not be completed."}}
	}
	_ = json.NewEncoder(w).Encode(v)
}
