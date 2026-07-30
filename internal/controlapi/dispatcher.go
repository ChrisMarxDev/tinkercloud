// Package controlapi exposes the platform control namespace. It deliberately
// accepts only an actor derived by Authenticator, never actor/app ownership data
// from JSON or query input.
package controlapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/ChrisMarxDev/tinkercloud/internal/deployments"
	"github.com/ChrisMarxDev/tinkercloud/internal/identity"
	"github.com/ChrisMarxDev/tinkercloud/internal/otp"
	"github.com/ChrisMarxDev/tinkercloud/internal/ratelimit"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/ChrisMarxDev/tinkercloud/internal/collections"
	"github.com/ChrisMarxDev/tinkercloud/internal/compatibility"
	"github.com/ChrisMarxDev/tinkercloud/internal/kv"
)

const MaxBody = 1 << 20

var ErrPolicyRevision = errors.New("policy revision conflict")
var ErrDeployerRevision = errors.New("deployer allowlist revision conflict")

// ErrLLMRevision deliberately carries no profile, grant, provider, or
// persistence detail to a browser. It only tells the control UI to reload its
// server-derived safe read model before an operator retries a mutation.
var ErrLLMRevision = errors.New("llm capability revision conflict")

type Actor struct {
	ID, Email, Role string
	// IdentitySessionID is populated only for the server-derived dashboard
	// browser identity. It binds dashboard CSRF to that current identity
	// session and is deliberately never serialized or accepted from clients.
	IdentitySessionID string `json:"-"`
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
	DataKVList(context.Context, Actor, string, string, string, int) (any, error)
	DataKVGet(context.Context, Actor, string, string) (any, error)
	DataKVSet(context.Context, Actor, string, string, json.RawMessage, *uint64, string) (any, error)
	DataKVDelete(context.Context, Actor, string, string, *uint64, string) (any, error)
	DataCollections(context.Context, Actor, string, string, int) (any, error)
	DataDocumentsList(context.Context, Actor, string, string, string, int) (any, error)
	DataDocumentGet(context.Context, Actor, string, string, string) (any, error)
	DataDocumentCreate(context.Context, Actor, string, string, json.RawMessage, string) (any, error)
	DataDocumentUpdate(context.Context, Actor, string, string, string, json.RawMessage, *uint64, string) (any, error)
	DataDocumentDelete(context.Context, Actor, string, string, string, *uint64, string) (any, error)
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
	// Domain is server-derived deployment evidence. It lets a deployer verify
	// the returned app hostname without assuming the dashboard host from its
	// control client URL.
	Domain               string `json:"domain"`
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
	// Compatibility is static build policy. It must never contain host,
	// identity, app, persistence, or credential-derived values.
	Compatibility compatibility.Matrix
	// OTPIssuanceFailure receives only a fixed failure category. Production
	// composition records it in the server log; it must never receive request,
	// email, transaction, provider, or database error details.
	OTPIssuanceFailure func(OTPIssuanceFailureCategory)
}

type OTPIssuanceFailureCategory = otp.IssuanceFailureCategory

const (
	OTPIssuanceEntropy                   = otp.IssuanceEntropy
	OTPIssuancePersistenceBeginWriteLock = otp.IssuancePersistenceBeginWriteLock
	OTPIssuancePersistenceInvalidate     = otp.IssuancePersistenceInvalidate
	OTPIssuancePersistenceEligibility    = otp.IssuancePersistenceEligibility
	OTPIssuancePersistenceInsert         = otp.IssuancePersistenceInsert
	OTPIssuancePersistenceCommit         = otp.IssuancePersistenceCommit
)

// Login is the CLI-only OTP boundary. Browser identities use the independent
// platform identity broker and never mint a control-plane bearer.
type Login interface {
	RequestOTP(context.Context, string, string) (string, error)
	VerifyOTP(context.Context, string, string) (string, error)
}

// PlatformAuthenticator is intentionally distinct from the CLI bearer-token
// authenticator. It is used only by the admin-host HTML surface, whose
// credential is the host-only global browser identity cookie.
type PlatformAuthenticator interface {
	AuthenticatePlatform(context.Context, http.ResponseWriter, *http.Request) (Actor, error)
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
	LLMConnections         []LLMConnection
	// LLMKeyManagementReady is derived exclusively by the control service. It
	// means the server has both its root-owned envelope boundary and a validator
	// available for write-only provider credential mutations. The dashboard must
	// never infer this from browser input or expose the root itself.
	LLMKeyManagementReady bool
	LLMProfiles           []LLMProfile
	LLMGrants             []LLMGrant
}
type LLMConnection struct{ ID, DisplayName, Provider, Status string }
type LLMProfile struct {
	ID, ConnectionID, Model, Status string
	Revision                        uint64
	MaxMessages, MaxMessageBytes    int
	MaxInputBytes, MaxOutputTokens  int
	TimeoutMS                       int64
	ViewerRequests, AppRequests     int
	RateWindowMS                    int64
	ConcurrencyLimit                int
	MonthlyTokenLimit               int
}
type LLMGrant struct {
	AppSlug, ProfileID, Status           string
	Revision                             uint64
	UsedTokens, ReservedTokens, InFlight int
}
type DashboardApp struct {
	Slug, Status, Description, StableURL string
	Access                               DashboardAccess
	Releases                             []DashboardRelease
	Tokens                               []DashboardToken
	// LLMGrant is a credential-free operator-only read model. It is attached
	// to the app rather than selected from a browser-provided app identifier.
	LLMGrant *LLMGrant
}

// LLMProfileInput contains only operator-selected safe profile limits. IDs for
// profiles are generated server-side; an operator may choose only an existing
// server-rendered connection ID.
type LLMProfileInput struct {
	ConnectionID, Model                         string
	MaxMessages, MaxMessageBytes, MaxInputBytes int
	MaxOutputTokens                             int
	TimeoutMS                                   int64
	ViewerRequests, AppRequests                 int
	RateWindowMS                                int64
	ConcurrencyLimit, MonthlyTokenLimit         int
	ExpectedRevision                            uint64
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
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

func (d Dispatcher) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/v1/version" && r.Method == "GET" {
		write(w, 200, map[string]int{"api_version": 1}, "")
		return
	}
	if r.URL.Path == "/api/v1/compatibility" && r.Method == "GET" {
		matrix := d.Compatibility
		if matrix.ServerVersion == "" {
			matrix = compatibility.Runtime("")
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		write(w, http.StatusOK, matrix, "")
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
			var err error
			tx, err = d.Login.RequestOTP(r.Context(), v.Email, ratelimit.RequestFingerprint(r))
			if err != nil {
				if d.OTPIssuanceFailure != nil {
					d.OTPIssuanceFailure(otpFailureCategory(err))
				}
				// The requester must not learn anything about deployer eligibility,
				// storage state, or delivery configuration. Unlike an opaque
				// transaction for an ineligible address, an issuer failure cannot
				// be verified later, so fail explicitly rather than minting a false
				// login_* receipt.
				write(w, http.StatusServiceUnavailable, nil, "temporarily_unavailable")
				return
			}
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
		token, e := d.Login.VerifyOTP(r.Context(), v.Transaction, v.Code)
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
	if !compatibleControlClient(r.Header.Get("X-Tinker-CLI-Version"), r.Header.Get("X-Tinker-Control-API-Version")) {
		write(w, http.StatusUpgradeRequired, nil, "cli_version_incompatible")
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
		case r.Method == "GET" && tail == "data/kv":
			prefix, cursor, limit, ok := dataPage(r)
			if !ok {
				write(w, 400, nil, "validation_failed")
				return
			}
			value, err := d.Service.DataKVList(r.Context(), a, app, prefix, cursor, limit)
			d.dataResult(w, value, err)
		case strings.HasPrefix(tail, "data/kv/"):
			kvKey, ok := dataKVKey(r, app)
			if !ok {
				write(w, 400, nil, "validation_failed")
				return
			}
			switch r.Method {
			case http.MethodGet:
				value, err := d.Service.DataKVGet(r.Context(), a, app, kvKey)
				d.dataResult(w, value, err)
			case http.MethodPut:
				var input dataValueInput
				if key(r) == "" || !body(r, &input) || input.Value == nil {
					write(w, 400, nil, "validation_failed")
					return
				}
				value, err := d.Service.DataKVSet(r.Context(), a, app, kvKey, input.Value, input.ExpectedVersion, key(r))
				d.dataResult(w, value, err)
			case http.MethodDelete:
				var input dataExpectedVersionInput
				if key(r) == "" || !optionalBody(r, &input) || input.ExpectedVersion == nil {
					write(w, 400, nil, "validation_failed")
					return
				}
				value, err := d.Service.DataKVDelete(r.Context(), a, app, kvKey, input.ExpectedVersion, key(r))
				d.dataResult(w, value, err)
			default:
				write(w, 404, nil, "not_found")
			}
		case r.Method == "GET" && tail == "data/collections":
			_, cursor, limit, ok := dataPage(r)
			if !ok {
				write(w, 400, nil, "validation_failed")
				return
			}
			value, err := d.Service.DataCollections(r.Context(), a, app, cursor, limit)
			d.dataResult(w, value, err)
		case strings.HasPrefix(tail, "data/collections/"):
			collection, id, root, ok := dataDocumentRoute(r, app)
			if !ok {
				write(w, 400, nil, "validation_failed")
				return
			}
			if root {
				switch r.Method {
				case http.MethodGet:
					_, cursor, limit, pageOK := dataPage(r)
					if !pageOK {
						write(w, 400, nil, "validation_failed")
						return
					}
					value, err := d.Service.DataDocumentsList(r.Context(), a, app, collection, cursor, limit)
					d.dataResult(w, value, err)
				case http.MethodPost:
					var input dataDocumentInput
					if key(r) == "" || !body(r, &input) || input.Data == nil {
						write(w, 400, nil, "validation_failed")
						return
					}
					value, err := d.Service.DataDocumentCreate(r.Context(), a, app, collection, input.Data, key(r))
					d.dataResult(w, value, err)
				default:
					write(w, 404, nil, "not_found")
				}
				return
			}
			switch r.Method {
			case http.MethodGet:
				value, err := d.Service.DataDocumentGet(r.Context(), a, app, collection, id)
				d.dataResult(w, value, err)
			case http.MethodPut:
				var input dataDocumentInput
				if key(r) == "" || !body(r, &input) || input.Data == nil || input.ExpectedVersion == nil {
					write(w, 400, nil, "validation_failed")
					return
				}
				value, err := d.Service.DataDocumentUpdate(r.Context(), a, app, collection, id, input.Data, input.ExpectedVersion, key(r))
				d.dataResult(w, value, err)
			case http.MethodDelete:
				var input dataExpectedVersionInput
				if key(r) == "" || !optionalBody(r, &input) || input.ExpectedVersion == nil {
					write(w, 400, nil, "validation_failed")
					return
				}
				value, err := d.Service.DataDocumentDelete(r.Context(), a, app, collection, id, input.ExpectedVersion, key(r))
				d.dataResult(w, value, err)
			default:
				write(w, 404, nil, "not_found")
			}
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
				var activation deployments.ActivationFailure
				if errors.As(e, &activation) {
					write(w, 409, nil, string(activation))
				} else {
					write(w, 409, nil, "activation_commit_failed")
				}
				return
			}
			write(w, 200, out, "")
		default:
			write(w, 404, nil, "not_found")
		}
	}
}

func otpFailureCategory(err error) OTPIssuanceFailureCategory {
	return otp.FailureCategory(err)
}

func compatibleControlClient(clientVersion, apiVersion string) bool {
	if clientVersion == "" && apiVersion == "" {
		return true
	}
	if clientVersion == "" || apiVersion != compatibility.ControlAPIVersion {
		return false
	}
	return compatibility.Current("0.1.0").ControlAPI.Client.Contains(clientVersion)
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
	allowed := map[string]bool{"app:read": true, "app:create": true, "app:delete": true, "deploy:create": true, "deploy:activate": true, "access:read": true, "access:write": true, "token:create": true, "token:revoke": true, "data:read": true, "data:write": true}
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
	raw, err := io.ReadAll(io.LimitReader(r.Body, MaxBody+1))
	if err != nil || len(raw) > MaxBody || !uniqueJSON(raw) {
		return false
	}
	de := json.NewDecoder(bytes.NewReader(raw))
	de.DisallowUnknownFields()
	return de.Decode(v) == nil && de.Decode(&struct{}{}) == io.EOF
}

// uniqueJSON rejects duplicate object members at every nesting depth. Go's
// default JSON decoder silently selects a later duplicate, which is unsafe for
// optimistic-version and data-mutation requests.
func uniqueJSON(raw []byte) bool {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if !consumeJSONValue(d, 0) {
		return false
	}
	var extra any
	return d.Decode(&extra) == io.EOF
}
func consumeJSONValue(d *json.Decoder, depth int) bool {
	if depth > 64 {
		return false
	}
	token, err := d.Token()
	if err != nil {
		return false
	}
	switch token {
	case json.Delim('{'):
		seen := map[string]struct{}{}
		for d.More() {
			k, err := d.Token()
			key, ok := k.(string)
			if err != nil || !ok {
				return false
			}
			if _, duplicate := seen[key]; duplicate {
				return false
			}
			seen[key] = struct{}{}
			if !consumeJSONValue(d, depth+1) {
				return false
			}
		}
		end, err := d.Token()
		return err == nil && end == json.Delim('}')
	case json.Delim('['):
		for d.More() {
			if !consumeJSONValue(d, depth+1) {
				return false
			}
		}
		end, err := d.Token()
		return err == nil && end == json.Delim(']')
	default:
		return true
	}
}
func optionalBody(r *http.Request, v any) bool {
	if r.ContentLength == 0 {
		return true
	}
	return body(r, v)
}

type dataValueInput struct {
	Value           json.RawMessage `json:"value"`
	ExpectedVersion *uint64         `json:"expected_version,omitempty"`
}
type dataDocumentInput struct {
	Data            json.RawMessage `json:"data"`
	ExpectedVersion *uint64         `json:"expected_version,omitempty"`
}
type dataExpectedVersionInput struct {
	ExpectedVersion *uint64 `json:"expected_version,omitempty"`
}

// dataPage recognizes exactly the bounded paging grammar shared by deployer
// data reads. It deliberately does not support filters or arbitrary queries.
func dataPage(r *http.Request) (prefix, cursor string, limit int, ok bool) {
	q := r.URL.Query()
	for name, values := range q {
		if (name != "prefix" && name != "cursor" && name != "limit") || len(values) != 1 {
			return "", "", 0, false
		}
	}
	prefix, cursor = q.Get("prefix"), q.Get("cursor")
	limit = 100
	if raw := q.Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return "", "", 0, false
		}
		limit = parsed
	}
	return prefix, cursor, limit, true
}

func dataKVKey(r *http.Request, slug string) (string, bool) {
	prefix := "/api/v1/apps/" + slug + "/data/kv/"
	raw := strings.TrimPrefix(r.URL.EscapedPath(), prefix)
	if raw == r.URL.EscapedPath() || raw == "" || strings.Contains(raw, "//") {
		return "", false
	}
	key, err := url.PathUnescape(raw)
	return key, err == nil && key != ""
}

// dataDocumentRoute returns a bounded collection/document route. The client
// never gets an app ID or database selector; the slug was authorized earlier.
func dataDocumentRoute(r *http.Request, slug string) (collection, id string, root, ok bool) {
	prefix := "/api/v1/apps/" + slug + "/data/collections/"
	raw := strings.TrimPrefix(r.URL.EscapedPath(), prefix)
	if raw == r.URL.EscapedPath() || raw == "" {
		return "", "", false, false
	}
	parts := strings.Split(raw, "/")
	if len(parts) < 2 || parts[1] != "documents" || len(parts) > 3 {
		return "", "", false, false
	}
	var err error
	collection, err = url.PathUnescape(parts[0])
	if err != nil || !collections.ValidCollection(collection) {
		return "", "", false, false
	}
	if len(parts) == 2 {
		return collection, "", true, true
	}
	id, err = url.PathUnescape(parts[2])
	if err != nil || !collections.ValidDocumentID(id) {
		return "", "", false, false
	}
	return collection, id, false, true
}

func (d Dispatcher) dataResult(w http.ResponseWriter, value any, err error) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if err == nil {
		write(w, http.StatusOK, value, "")
		return
	}
	if errors.Is(err, sql.ErrNoRows) {
		write(w, http.StatusNotFound, nil, "not_found")
		return
	}
	if errors.Is(err, kv.ErrVersionConflict) || errors.Is(err, collections.ErrVersionConflict) {
		write(w, http.StatusConflict, nil, "conflict")
		return
	}
	if errors.Is(err, kv.ErrInvalidKey) || errors.Is(err, kv.ErrInvalidValue) || errors.Is(err, kv.ErrInvalidListLimit) || errors.Is(err, collections.ErrInvalidCollection) || errors.Is(err, collections.ErrInvalidDocument) || errors.Is(err, collections.ErrInvalidDocumentID) || errors.Is(err, collections.ErrInvalidListLimit) {
		write(w, http.StatusBadRequest, nil, "validation_failed")
		return
	}
	if errors.Is(err, kv.ErrQuotaExceeded) || errors.Is(err, collections.ErrQuotaExceeded) || errors.Is(err, collections.ErrSnapshotTooLarge) {
		write(w, http.StatusTooManyRequests, nil, "quota_exceeded")
		return
	}
	write(w, http.StatusForbidden, nil, "not_authorized")
}
func write(w http.ResponseWriter, status int, v any, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if code != "" {
		v = envelope{Error: &apiError{Code: code, Message: "Request could not be completed.", RequestID: w.Header().Get("X-Request-ID")}}
	}
	_ = json.NewEncoder(w).Encode(v)
}
