package compose

// This file owns the HTTP composition for the global *viewer* identity. It is
// deliberately separate from control-plane authentication: the cookie proves
// only a browser email identity, while every app handoff still re-evaluates the
// app's current access policy before a host-only app session is issued.

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tinyhost/tiny/internal/browseridentity"
	"github.com/tinyhost/tiny/internal/config"
	"github.com/tinyhost/tiny/internal/gateway"
	"github.com/tinyhost/tiny/internal/otp"
	"github.com/tinyhost/tiny/internal/persistence"
	"github.com/tinyhost/tiny/internal/ratelimit"
	"github.com/tinyhost/tiny/internal/sessions"
	webui "github.com/tinyhost/tiny/web"
)

const (
	// GlobalIdentityCookieName is host-only at the platform origin. It is never
	// sent to deployed applications.
	GlobalIdentityCookieName = browseridentity.IdentityCookieName
	// BrowserBindingCookieName is intentionally not an identity credential. It
	// is a host-only, opaque browser-profile binding used only to serialize OTP
	// completions started by the same browser. It is never sent to app hosts,
	// templates, URLs, forms, JavaScript, or logs.
	BrowserBindingCookieName = browseridentity.BindingCookieName
	identityStateCookieName  = "__Host-tiny_identity_state"
	browserBindingLifetime   = 30 * 24 * time.Hour
	browserBindingMaxBytes   = 256
)

// IdentityStore is the persistence boundary needed by browser composition.
// It deliberately passes no browser-controlled app or identity input into the
// authorize/consume operations: those values are loaded from the opaque,
// server-created handoff.
type IdentityStore interface {
	CreateIdentityHandoff(context.Context, string, string, bool, time.Time) (persistence.IdentityHandoff, string, error)
	GetIdentityHandoff(context.Context, string, time.Time) (persistence.IdentityHandoff, error)
	ForceIdentityHandoff(context.Context, string, time.Time) error
	ValidateIdentitySession(context.Context, string, time.Time) (persistence.IdentityValidationResult, error)
	AuthorizeIdentityHandoff(context.Context, string, string, time.Time) (persistence.IdentityHandoff, persistence.IdentitySession, error)
	RequestIdentityOTP(context.Context, string, string, string, string, []byte, time.Time, time.Duration) (*persistence.ChallengeMessage, error)
	VerifyIdentityOTP(context.Context, string, string, string, string, string, string, []byte, time.Time, int) (persistence.IdentityOTPResult, error)
	ConsumeIdentityHandoff(context.Context, string, string, string, time.Time, time.Time) (persistence.IdentityHandoff, string, sessions.Session, error)
	RevokeIdentitySession(context.Context, string, time.Time) ([]persistence.AppSessionRef, error)
	RevokeIdentityBrowserBinding(context.Context, string, time.Time) ([]persistence.AppSessionRef, error)
}

// platformIdentityStore is deliberately separate from app handoffs. It lets
// any email establish one platform identity; dashboard access stays a current
// role lookup in ControlAuthenticator and app access stays a policy lookup.
type platformIdentityStore interface {
	RequestPlatformIdentityOTP(context.Context, string, string, string, []byte, time.Time, time.Duration) (*persistence.PlatformIdentityChallenge, error)
	VerifyPlatformIdentityOTP(context.Context, string, string, string, string, string, bool, []byte, time.Time, int) (persistence.IdentityOTPResult, error)
}

type dashboardIdentityStore interface {
	AuthenticateDashboardIdentity(context.Context, string, time.Time) (persistence.DashboardIdentityResult, error)
}

type identityOutbox interface {
	EnqueueOTP(context.Context, otp.Message) error
}

// IdentityBroker joins two safe browser transports: an admin-host-only
// global identity cookie and a one-time app-bound handoff. It never makes the
// platform cookie an app credential.
type IdentityBroker struct {
	Store         IdentityStore
	Outbox        identityOutbox
	HMACKey       []byte
	OTPExpiry     time.Duration
	OTPMaxAttempt int
	AppSessionTTL time.Duration
	PlatformHost  string
	AppSuffix     string
	// Domain remains an internal test convenience for pre-domain fixtures. New
	// production composition always supplies the exact PlatformHost/AppSuffix.
	Domain     string
	RateLimits *ratelimit.Limiter
	// RevokeChildren runs only after the persistence transaction commits. It is
	// used to close already-upgraded live connections whose child app sessions
	// were revoked by global logout or an account switch.
	RevokeChildren func([]persistence.AppSessionRef)
	Now            func() time.Time
}

func (b IdentityBroker) now() time.Time {
	if b.Now != nil {
		return b.Now()
	}
	return time.Now()
}

func (b IdentityBroker) otpTTL() time.Duration {
	if b.OTPExpiry > 0 {
		return b.OTPExpiry
	}
	return 10 * time.Minute
}

func (b IdentityBroker) appTTL() time.Duration {
	if b.AppSessionTTL > 0 {
		return b.AppSessionTTL
	}
	return 24 * time.Hour
}

func (b IdentityBroker) platformHost() string {
	if b.PlatformHost != "" {
		return b.PlatformHost
	}
	return "tiny.test"
}

func (b IdentityBroker) appSuffix() string {
	if b.AppSuffix != "" {
		return b.AppSuffix
	}
	return b.Domain
}

// AppLogin begins an app-origin handoff. raw state is host-only on this app;
// the platform receives only the opaque handoff id.
func (b IdentityBroker) AppLogin(appID, returnPath string, w http.ResponseWriter, r *http.Request) bool {
	if b.Store == nil || b.platformHost() == "" || b.appSuffix() == "" {
		return false
	}
	// A handoff is durable state. Keep anonymous document navigations bounded
	// independently from email OTP issuance before asking persistence to create
	// one. appID is gateway-derived from the validated host.
	if b.RateLimits != nil && !b.RateLimits.AllowHandoffRequest(r, appID) {
		return false
	}
	ret, ok := gateway.ValidReturnPath(returnPath)
	if !ok {
		ret = "/"
	}
	h, state, err := b.Store.CreateIdentityHandoff(r.Context(), appID, ret, false, b.now())
	if err != nil || h.ID == "" || state == "" {
		return false
	}
	expires := h.ExpiresAt
	if expires.IsZero() {
		expires = b.now().Add(b.otpTTL())
	}
	http.SetCookie(w, &http.Cookie{Name: identityStateCookieName, Value: state, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, Expires: expires})
	http.Redirect(w, r, "https://"+b.platformHost()+"/_tiny/identity?handoff="+url.QueryEscape(h.ID), http.StatusSeeOther)
	return true
}

// AppCallback consumes the state bound to the exact app that the gateway has
// already resolved. The callback cannot choose an app or return path.
func (b IdentityBroker) AppCallback(appID string, w http.ResponseWriter, r *http.Request) bool {
	if b.Store == nil {
		return false
	}
	handoffID := strings.TrimSpace(r.URL.Query().Get("handoff"))
	state, err := r.Cookie(identityStateCookieName)
	if handoffID == "" || err != nil || state.Value == "" {
		return false
	}
	now := b.now()
	h, appRaw, appSession, err := b.Store.ConsumeIdentityHandoff(r.Context(), appID, handoffID, state.Value, now, now.Add(b.appTTL()))
	if err != nil || appRaw == "" {
		return false
	}
	http.SetCookie(w, sessions.AppCookie(appRaw, appSession.ExpiresAt))
	http.SetCookie(w, expiredCookie(identityStateCookieName))
	ret, ok := gateway.ValidReturnPath(h.ReturnPath)
	if !ok {
		ret = "/"
	}
	http.Redirect(w, r, ret, http.StatusSeeOther)
	return true
}

// PlatformHandler takes ownership of only the private broker namespace. All
// dashboard/control routes keep their existing handler and credentials.
func (b IdentityBroker) PlatformHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" && r.Method == http.MethodGet {
			b.platformLoginPage(w, r, false)
			return
		}
		if r.URL.Path == "/login/verify" && r.Method == http.MethodPost {
			b.platformVerifyOTP(w, r)
			return
		}
		if r.URL.Path == "/login" && r.Method == http.MethodPost {
			b.platformRequestOTP(w, r)
			return
		}
		if r.URL.Path == "/logout" && r.Method == http.MethodPost {
			b.platformLogout(w, r)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/_tiny/identity") {
			next.ServeHTTP(w, r)
			return
		}
		switch r.URL.Path {
		case "/_tiny/identity":
			if r.Method == http.MethodGet {
				b.identityPage(w, r)
				return
			}
		case "/_tiny/identity/otp":
			if r.Method == http.MethodPost {
				b.requestOTP(w, r)
				return
			}
		case "/_tiny/identity/verify":
			if r.Method == http.MethodPost {
				b.verifyOTP(w, r)
				return
			}
		case "/_tiny/identity/use-another":
			if r.Method == http.MethodPost {
				b.useAnother(w, r)
				return
			}
		case "/_tiny/identity/logout":
			if r.Method == http.MethodPost {
				b.logout(w, r)
				return
			}
		}
		platformNotFound(w)
	})
}

func (b IdentityBroker) platformLoginPage(w http.ResponseWriter, r *http.Request, replace bool) {
	if b.Store == nil {
		b.retryStatus(w, http.StatusServiceUnavailable)
		return
	}
	if c, err := r.Cookie(GlobalIdentityCookieName); err == nil && c.Value != "" && !replace {
		v, err := b.Store.ValidateIdentitySession(r.Context(), c.Value, b.now())
		if err == nil {
			if dashboard, ok := b.Store.(dashboardIdentityStore); ok {
				result, dashboardErr := dashboard.AuthenticateDashboardIdentity(r.Context(), c.Value, b.now())
				if dashboardErr == nil {
					b.rotateIdentity(w, result.Identity, result.ReplacementToken)
					http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
					return
				}
				if errors.Is(dashboardErr, persistence.ErrIdentity) {
					csrf := b.platformCSRF(w, r, v.Session.ID)
					b.render(w, platformNoRoleTemplate, platformNoRolePage{Email: v.Session.Identity.Email, CSRF: csrf})
					return
				}
				b.retryStatus(w, http.StatusServiceUnavailable)
				return
			}
			b.rotateIdentity(w, v.Session, v.ReplacementToken)
			http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
			return
		}
		if errors.Is(err, persistence.ErrIdentity) {
			if len(v.Revoked) > 0 && b.RevokeChildren != nil {
				b.RevokeChildren(uniqueAppSessionRefs(v.Revoked))
			}
			http.SetCookie(w, browseridentity.ExpiredCookie(GlobalIdentityCookieName))
		} else {
			// A store failure is not evidence that the current identity is stale;
			// do not start a replacement-login flow that could mask it.
			b.retryStatus(w, http.StatusServiceUnavailable)
			return
		}
	}
	if !b.ensureBrowserBinding(w, r) {
		b.retryStatus(w, http.StatusServiceUnavailable)
		return
	}
	b.render(w, platformEmailTemplate, platformEmailPage{Replace: replace})
}

func (b IdentityBroker) platformRequestOTP(w http.ResponseWriter, r *http.Request) {
	if !gateway.SameOrigin(r) || !hasMediaType(r, "application/x-www-form-urlencoded") {
		b.retry(w)
		return
	}
	s, ok := b.Store.(platformIdentityStore)
	if !ok {
		b.retryStatus(w, http.StatusServiceUnavailable)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if r.ParseForm() != nil {
		b.retry(w)
		return
	}
	binding, ok := browserBinding(r)
	if !ok {
		b.retry(w)
		return
	}
	email := r.Form.Get("email")
	tx := opaqueTransaction()
	if b.RateLimits == nil || b.RateLimits.AllowRequest(ratelimit.OTPRequest, r, email, "platform_identity") {
		if m, err := s.RequestPlatformIdentityOTP(r.Context(), binding, email, ratelimit.RequestFingerprint(r), b.HMACKey, b.now(), b.otpTTL()); err == nil && m != nil {
			tx = m.ID
			if b.Outbox != nil {
				_ = b.Outbox.EnqueueOTP(r.Context(), otp.Message{Email: m.Email, Code: m.Code, ChallengeID: m.ID})
			}
			if b.RateLimits != nil {
				b.RateLimits.BindTransactionRequest(tx, email, r)
			}
		}
	}
	b.render(w, platformCodeTemplate, platformCodePage{Email: email, Transaction: tx, Replace: r.Form.Get("replace") == "1"})
}

func (b IdentityBroker) platformVerifyOTP(w http.ResponseWriter, r *http.Request) {
	if !gateway.SameOrigin(r) || !hasMediaType(r, "application/x-www-form-urlencoded") {
		b.retry(w)
		return
	}
	s, ok := b.Store.(platformIdentityStore)
	if !ok {
		b.retryStatus(w, http.StatusServiceUnavailable)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if r.ParseForm() != nil {
		b.retry(w)
		return
	}
	binding, ok := browserBinding(r)
	if !ok || (b.RateLimits != nil && !b.RateLimits.AllowTransactionRequest(ratelimit.OTPVerify, r, r.Form.Get("transaction"), "platform_identity")) {
		b.retry(w)
		return
	}
	old := ""
	if c, err := r.Cookie(GlobalIdentityCookieName); err == nil {
		old = c.Value
	}
	result, err := s.VerifyPlatformIdentityOTP(r.Context(), binding, r.Form.Get("email"), r.Form.Get("transaction"), r.Form.Get("code"), old, r.Form.Get("replace") == "1", b.HMACKey, b.now(), b.OTPMaxAttempt)
	if err != nil || result.Token == "" {
		b.retry(w)
		return
	}
	if len(result.Revoked) > 0 && b.RevokeChildren != nil {
		b.RevokeChildren(uniqueAppSessionRefs(result.Revoked))
	}
	http.SetCookie(w, identityCookie(result.Token, result.Session.ExpiresAt))
	http.SetCookie(w, browseridentity.ExpiredCookie(browseridentity.CSRFCookieName))
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (b IdentityBroker) platformLogout(w http.ResponseWriter, r *http.Request) {
	if !gateway.SameOrigin(r) || !b.validPlatformCSRF(r) {
		b.retryStatus(w, http.StatusForbidden)
		return
	}
	b.logoutTo(w, r, "/login")
}

func (b IdentityBroker) identityPage(w http.ResponseWriter, r *http.Request) {
	handoffID := strings.TrimSpace(r.URL.Query().Get("handoff"))
	if b.Store == nil || handoffID == "" {
		b.retry(w)
		return
	}
	if c, err := r.Cookie(GlobalIdentityCookieName); err == nil && c.Value != "" {
		validation, validationErr := b.Store.ValidateIdentitySession(r.Context(), c.Value, b.now())
		if validationErr != nil {
			// A stale rotated token can be replay evidence. Persistence revokes
			// that family atomically and returns the already-committed child refs
			// so upgraded app connections are closed before the generic denial.
			if len(validation.Revoked) > 0 && b.RevokeChildren != nil {
				b.RevokeChildren(validation.Revoked)
			}
			if errors.Is(validationErr, persistence.ErrIdentity) {
				// A known-invalid or replayed credential cannot become valid again.
				// Remove it before presenting the regular generic re-auth form so a
				// successful OTP does not inherit stale account-switch state.
				http.SetCookie(w, expiredCookie(GlobalIdentityCookieName))
				if !b.ensureBrowserBinding(w, r) {
					b.retryStatus(w, http.StatusServiceUnavailable)
					return
				}
				b.render(w, identityEmailTemplate, identityEmailPage{Handoff: handoffID, Replace: false})
				return
			}
			// Dependency ambiguity must not look like a sign-out or invite a new
			// credential issuance attempt. Keep the existing identity cookie and
			// surface only the generic retry state.
			b.retryStatus(w, http.StatusServiceUnavailable)
			return
		}
		session, rotated := validation.Session, validation.ReplacementToken
		raw := c.Value
		if rotated != "" {
			raw = rotated
		}
		h, _, err := b.Store.AuthorizeIdentityHandoff(r.Context(), handoffID, raw, b.now())
		if err == nil {
			b.rotateIdentity(w, session, rotated)
			b.redirectCallback(w, r, h)
			return
		}
		// The cookie proves an identity even if this particular app does not
		// allow it. Show that identity without exposing policy detail.
		b.rotateIdentity(w, session, rotated)
		b.render(w, identityDeniedTemplate, identityDeniedPage{Email: session.Identity.Email, Handoff: handoffID})
		return
	}
	if !b.ensureBrowserBinding(w, r) {
		b.retryStatus(w, http.StatusServiceUnavailable)
		return
	}
	b.render(w, identityEmailTemplate, identityEmailPage{Handoff: handoffID, Replace: false})
}

func (b IdentityBroker) requestOTP(w http.ResponseWriter, r *http.Request) {
	if !gateway.SameOrigin(r) || !hasMediaType(r, "application/x-www-form-urlencoded") || b.Store == nil {
		b.retry(w)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if r.ParseForm() != nil {
		b.retry(w)
		return
	}
	handoffID, email := strings.TrimSpace(r.Form.Get("handoff")), r.Form.Get("email")
	binding, hasBinding := browserBinding(r)
	if handoffID == "" || !hasBinding {
		b.retry(w)
		return
	}
	allowed := b.RateLimits == nil || b.RateLimits.AllowRequest(ratelimit.OTPRequest, r, email, "identity")
	tx := opaqueTransaction()
	if allowed {
		if m, err := b.Store.RequestIdentityOTP(r.Context(), handoffID, binding, email, ratelimit.RequestFingerprint(r), b.HMACKey, b.now(), b.otpTTL()); err == nil && m != nil {
			tx = m.ID
			if b.Outbox != nil {
				_ = b.Outbox.EnqueueOTP(r.Context(), otp.Message{AppID: m.AppID, Email: m.Email, Code: m.Code, ChallengeID: m.ID})
			}
			if b.RateLimits != nil {
				b.RateLimits.BindTransactionRequest(tx, email, r)
			}
		}
	}
	b.render(w, identityCodeTemplate, identityCodePage{Email: email, Handoff: handoffID, Transaction: tx})
}

func (b IdentityBroker) verifyOTP(w http.ResponseWriter, r *http.Request) {
	if !gateway.SameOrigin(r) || !hasMediaType(r, "application/x-www-form-urlencoded") || b.Store == nil {
		b.retry(w)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if r.ParseForm() != nil {
		b.retry(w)
		return
	}
	handoffID, email, tx, code := strings.TrimSpace(r.Form.Get("handoff")), r.Form.Get("email"), r.Form.Get("transaction"), r.Form.Get("code")
	binding, hasBinding := browserBinding(r)
	if handoffID == "" || !hasBinding || (b.RateLimits != nil && !b.RateLimits.AllowTransactionRequest(ratelimit.OTPVerify, r, tx, "identity")) {
		b.retry(w)
		return
	}
	oldRaw := ""
	if c, err := r.Cookie(GlobalIdentityCookieName); err == nil {
		oldRaw = c.Value
	}
	result, err := b.Store.VerifyIdentityOTP(r.Context(), handoffID, binding, email, tx, code, oldRaw, b.HMACKey, b.now(), b.OTPMaxAttempt)
	if err != nil || result.Token == "" {
		b.retry(w)
		return
	}
	if b.RevokeChildren != nil && len(result.Revoked) > 0 {
		b.RevokeChildren(result.Revoked)
	}
	http.SetCookie(w, identityCookie(result.Token, result.Session.ExpiresAt))
	http.SetCookie(w, browseridentity.ExpiredCookie(browseridentity.CSRFCookieName))
	// Verification authorizes the stored handoff atomically with global-session
	// issuance. Re-authorizing here would race the one-time transition and is
	// intentionally rejected by persistence.
	h, err := b.Store.GetIdentityHandoff(r.Context(), handoffID, b.now())
	if err != nil || h.AuthorizedAt == nil {
		b.retry(w)
		return
	}
	b.redirectCallback(w, r, h)
}

func (b IdentityBroker) useAnother(w http.ResponseWriter, r *http.Request) {
	if !gateway.SameOrigin(r) || !hasMediaType(r, "application/x-www-form-urlencoded") || b.Store == nil {
		b.retry(w)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2048)
	if r.ParseForm() != nil {
		b.retry(w)
		return
	}
	h, err := b.Store.GetIdentityHandoff(r.Context(), strings.TrimSpace(r.Form.Get("handoff")), b.now())
	if err != nil {
		b.retry(w)
		return
	}
	if err := b.Store.ForceIdentityHandoff(r.Context(), h.ID, b.now()); err != nil {
		b.retry(w)
		return
	}
	if !b.ensureBrowserBinding(w, r) {
		b.retryStatus(w, http.StatusServiceUnavailable)
		return
	}
	// The original handoff remains bound to the state cookie already set at the
	// app origin. Replacing its identity requires a successful OTP first.
	b.render(w, identityEmailTemplate, identityEmailPage{Handoff: h.ID, Replace: true})
}

func (b IdentityBroker) logout(w http.ResponseWriter, r *http.Request) {
	b.logoutTo(w, r, "/")
}

// logoutTo durably revokes the global family before clearing any cookies. It
// is shared by the private app identity route and the dashboard's visible
// global sign-out action.
func (b IdentityBroker) logoutTo(w http.ResponseWriter, r *http.Request, redirect string) {
	if !gateway.SameOrigin(r) || b.Store == nil {
		b.retry(w)
		return
	}
	identityRaw := ""
	if c, err := r.Cookie(GlobalIdentityCookieName); err == nil {
		identityRaw = c.Value
	}
	bindingRaw, hasBinding := browserBinding(r)
	if identityRaw == "" && !hasBinding {
		// A global sign-out without either server-recognized platform credential
		// cannot prove or revoke a browser family. Do not advertise success.
		b.retry(w)
		return
	}

	// The primary identity credential is authoritative when it is valid. Only a
	// known-invalid/missing identity falls back to the non-authorizing binding;
	// a real persistence error must not trigger a second independent write that
	// could make the user-visible outcome ambiguous.
	var refs []persistence.AppSessionRef
	needBindingFallback := identityRaw == ""
	if identityRaw != "" {
		got, err := b.Store.RevokeIdentitySession(r.Context(), identityRaw, b.now())
		if err == nil {
			refs = append(refs, got...)
		} else if errors.Is(err, persistence.ErrIdentity) {
			needBindingFallback = true
		} else {
			// A real dependency error is not evidence that the presented identity
			// is invalid. Preserve browser cookies and do not report logout
			// success.
			b.retryStatus(w, http.StatusServiceUnavailable)
			return
		}
	}
	if needBindingFallback && hasBinding {
		got, err := b.Store.RevokeIdentityBrowserBinding(r.Context(), bindingRaw, b.now())
		if err == nil {
			refs = append(refs, got...)
		} else if errors.Is(err, persistence.ErrIdentity) {
			// The non-authorizing binding did not identify a revocable family.
			// This is not a completed global logout: preserve every cookie and
			// return the same generic retry state as a credential-free request.
			b.retry(w)
			return
		} else {
			b.retryStatus(w, http.StatusServiceUnavailable)
			return
		}
	}
	if len(refs) > 0 && b.RevokeChildren != nil {
		b.RevokeChildren(uniqueAppSessionRefs(refs))
	}
	http.SetCookie(w, expiredCookie(GlobalIdentityCookieName))
	http.SetCookie(w, browseridentity.ExpiredCookie(browseridentity.CSRFCookieName))
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

func uniqueAppSessionRefs(refs []persistence.AppSessionRef) []persistence.AppSessionRef {
	seen := make(map[persistence.AppSessionRef]struct{}, len(refs))
	out := make([]persistence.AppSessionRef, 0, len(refs))
	for _, ref := range refs {
		if ref.AppID == "" || ref.SessionID == "" {
			continue
		}
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		out = append(out, ref)
	}
	return out
}

func (b IdentityBroker) redirectCallback(w http.ResponseWriter, r *http.Request, h persistence.IdentityHandoff) {
	if h.ID == "" || h.AppSlug == "" || !validAppHost(h.AppSlug, b.appSuffix()) {
		b.retry(w)
		return
	}
	http.Redirect(w, r, "https://"+appHost(h.AppSlug, b.appSuffix())+"/_tiny/auth/callback?handoff="+url.QueryEscape(h.ID), http.StatusSeeOther)
}

func (b IdentityBroker) rotateIdentity(w http.ResponseWriter, session persistence.IdentitySession, raw string) {
	if raw != "" {
		http.SetCookie(w, identityCookie(raw, session.ExpiresAt))
	}
}

func identityCookie(raw string, expiry time.Time) *http.Cookie {
	return browseridentity.IdentityCookie(raw, expiry)
}

// ensureBrowserBinding only prepares the normal browser form flow. Unlike the
// global identity cookie, it does not grant identity or authorization and is
// deliberately retained across global logout and known-invalid identity
// cleanup so the browser retains a stable concurrent-OTP grouping key.
func (b IdentityBroker) ensureBrowserBinding(w http.ResponseWriter, r *http.Request) bool {
	if _, ok := browserBinding(r); ok {
		return true
	}
	raw, err := newBrowserBinding()
	if err != nil {
		return false
	}
	http.SetCookie(w, browserBindingCookie(raw, b.now().Add(browserBindingLifetime)))
	return true
}

func browserBinding(r *http.Request) (string, bool) {
	c, err := r.Cookie(BrowserBindingCookieName)
	if err != nil || c.Value == "" || len(c.Value) > browserBindingMaxBytes {
		return "", false
	}
	return c.Value, true
}

func newBrowserBinding() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "bb_" + base64.RawURLEncoding.EncodeToString(b), nil
}

func browserBindingCookie(raw string, expiry time.Time) *http.Cookie {
	return &http.Cookie{Name: BrowserBindingCookieName, Value: raw, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, Expires: expiry}
}

func expiredCookie(name string) *http.Cookie {
	return browseridentity.ExpiredCookie(name)
}

func (b IdentityBroker) validPlatformCSRF(r *http.Request) bool {
	c, err := r.Cookie(browseridentity.CSRFCookieName)
	identityCookie, identityErr := r.Cookie(GlobalIdentityCookieName)
	if err != nil || identityErr != nil || c.Value == "" || r.ParseForm() != nil || b.Store == nil {
		return false
	}
	validation, validationErr := b.Store.ValidateIdentitySession(r.Context(), identityCookie.Value, b.now())
	if validationErr != nil || validation.Session.ID == "" || !strings.HasPrefix(c.Value, validation.Session.ID+".") {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(c.Value), []byte(r.Form.Get("csrf"))) == 1
}

func (b IdentityBroker) platformCSRF(w http.ResponseWriter, r *http.Request, identitySessionID string) string {
	if identitySessionID == "" {
		return ""
	}
	prefix := identitySessionID + "."
	if c, err := r.Cookie(browseridentity.CSRFCookieName); err == nil && strings.HasPrefix(c.Value, prefix) && len(c.Value) >= len(prefix)+32 {
		return c.Value
	}
	random := make([]byte, 24)
	if _, err := rand.Read(random); err != nil {
		return ""
	}
	v := prefix + base64.RawURLEncoding.EncodeToString(random)
	http.SetCookie(w, &http.Cookie{Name: browseridentity.CSRFCookieName, Value: v, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	return v
}

func appHost(slug, suffix string) string {
	return strings.ToLower(slug) + "." + strings.ToLower(suffix)
}
func validAppHost(slug, suffix string) bool {
	h, ok := gateway.ClassifyHost(appHost(slug, suffix), config.Config{Domain: suffix})
	return ok && h.Kind == gateway.App && h.Slug == strings.ToLower(slug)
}

func (b IdentityBroker) render(w http.ResponseWriter, tpl *template.Template, page any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = tpl.Execute(w, page)
}

func (b IdentityBroker) retry(w http.ResponseWriter) {
	b.render(w, identityRetryTemplate, nil)
}

func (b IdentityBroker) retryStatus(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = identityRetryTemplate.Execute(w, nil)
}

func platformNotFound(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNotFound)
	_ = identityRetryTemplate.Execute(w, nil)
}

type identityEmailPage struct {
	Handoff string
	Replace bool
}
type identityCodePage struct{ Email, Handoff, Transaction string }
type identityDeniedPage struct{ Email, Handoff string }
type platformEmailPage struct{ Replace bool }
type platformCodePage struct {
	Email, Transaction string
	Replace            bool
}
type platformNoRolePage struct{ Email, CSRF string }

var identityEmailTemplate = template.Must(template.New("identity-email").Funcs(webui.FuncMap()).Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Sign in · TinyHost</title><style>{{tinyCSS}}</style></head><body><a class="tiny-skip" href="#main">Skip to content</a><main class="tiny-auth" id="main"><section class="tiny-auth-card" aria-labelledby="identity-title"><div class="tiny-brand"><span class="tiny-brand__mark" aria-hidden="true">{{tinyMark}}</span><span>protected by tinyhost</span></div><p class="tiny-eyebrow">Private app</p><h1 id="identity-title">Sign in to continue.</h1><p class="tiny-auth-card__intro">Use an email address the app owner has allowed.</p><form class="tiny-stack" method="post" action="/_tiny/identity/otp"><label class="tiny-field" for="email"><span class="tiny-label">Email address</span><input class="tiny-input" id="email" name="email" type="email" required autocomplete="email"></label><input type="hidden" name="handoff" value="{{.Handoff}}"><button class="tiny-button tiny-button--primary tiny-button--full" type="submit">Send one-time code</button></form><p class="tiny-auth-card__footer">This message is the same for every address.</p></section></main><script>{{tinyJS}}</script></body></html>`))
var identityCodeTemplate = template.Must(template.New("identity-code").Funcs(webui.FuncMap()).Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Check your email · TinyHost</title><style>{{tinyCSS}}</style></head><body><a class="tiny-skip" href="#main">Skip to content</a><main class="tiny-auth" id="main"><section class="tiny-auth-card" aria-labelledby="code-title"><div class="tiny-brand"><span class="tiny-brand__mark" aria-hidden="true">{{tinyMark}}</span><span>protected by tinyhost</span></div><p class="tiny-eyebrow">One small step</p><h1 id="code-title">Check your email.</h1><p class="tiny-auth-card__intro">If that address is authorized, a one-time code has been sent.</p><form class="tiny-stack" method="post" action="/_tiny/identity/verify"><label class="tiny-field" for="code"><span class="tiny-label">One-time code</span><input class="tiny-input tiny-input--code" id="code" name="code" inputmode="numeric" autocomplete="one-time-code" required></label><input type="hidden" name="email" value="{{.Email}}"><input type="hidden" name="handoff" value="{{.Handoff}}"><input type="hidden" name="transaction" value="{{.Transaction}}"><button class="tiny-button tiny-button--primary tiny-button--full" type="submit">Verify and continue</button></form></section></main><script>{{tinyJS}}</script></body></html>`))
var identityDeniedTemplate = template.Must(template.New("identity-denied").Funcs(webui.FuncMap()).Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Access unavailable · TinyHost</title><style>{{tinyCSS}}</style></head><body><a class="tiny-skip" href="#main">Skip to content</a><main class="tiny-auth" id="main"><section class="tiny-auth-card" aria-labelledby="denied-title"><div class="tiny-brand"><span class="tiny-brand__mark" aria-hidden="true">{{tinyMark}}</span><span>protected by tinyhost</span></div><p class="tiny-eyebrow">Private app</p><h1 id="denied-title">This account cannot open this app.</h1><div class="tiny-notice" role="status"><p>Signed in as <strong>{{.Email}}</strong>. Ask the app owner for access, or use another email.</p></div><form class="tiny-stack" method="post" action="/_tiny/identity/use-another"><input type="hidden" name="handoff" value="{{.Handoff}}"><button class="tiny-button tiny-button--secondary tiny-button--full" type="submit">Use another email</button></form><p class="tiny-auth-card__footer">Changing email signs this browser out of TinyHost apps after verification. We do not reveal the app’s access rules.</p></section></main><script>{{tinyJS}}</script></body></html>`))
var identityRetryTemplate = template.Must(template.New("identity-retry").Funcs(webui.FuncMap()).Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Sign in · TinyHost</title><style>{{tinyCSS}}</style></head><body><main class="tiny-auth" id="main"><section class="tiny-auth-card" aria-labelledby="retry-title"><div class="tiny-brand"><span class="tiny-brand__mark" aria-hidden="true">{{tinyMark}}</span><span>protected by tinyhost</span></div><h1 id="retry-title">Sign-in needs another try.</h1><div class="tiny-notice" role="alert"><p>We could not complete that sign-in step. Your access has not changed.</p></div><p class="tiny-auth-card__footer">Return to the protected app and try again.</p></section></main><script>{{tinyJS}}</script></body></html>`))
var platformEmailTemplate = template.Must(template.New("platform-email").Funcs(webui.FuncMap()).Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Sign in · TinyHost</title><style>{{tinyCSS}}</style></head><body><a class="tiny-skip" href="#main">Skip to content</a><main class="tiny-auth" id="main"><section class="tiny-auth-card" aria-labelledby="platform-title"><div class="tiny-brand"><span class="tiny-brand__mark" aria-hidden="true">{{tinyMark}}</span><span>tinyhost</span></div><p class="tiny-eyebrow">Your tiny cloud</p><h1 id="platform-title">Sign in to TinyHost.</h1><p class="tiny-auth-card__intro">Use your email to continue to the dashboard and any apps you can access.</p><form class="tiny-stack" method="post" action="/login"><label class="tiny-field" for="email"><span class="tiny-label">Email address</span><input class="tiny-input" id="email" name="email" type="email" required autocomplete="email"></label>{{if .Replace}}<input type="hidden" name="replace" value="1">{{end}}<button class="tiny-button tiny-button--primary tiny-button--full" type="submit">Send one-time code</button></form></section></main><script>{{tinyJS}}</script></body></html>`))
var platformCodeTemplate = template.Must(template.New("platform-code").Funcs(webui.FuncMap()).Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Check your email · TinyHost</title><style>{{tinyCSS}}</style></head><body><a class="tiny-skip" href="#main">Skip to content</a><main class="tiny-auth" id="main"><section class="tiny-auth-card" aria-labelledby="platform-code-title"><div class="tiny-brand"><span class="tiny-brand__mark" aria-hidden="true">{{tinyMark}}</span><span>tinyhost</span></div><p class="tiny-eyebrow">One small step</p><h1 id="platform-code-title">Check your email.</h1><p class="tiny-auth-card__intro">If that address can sign in, a one-time code has been sent.</p><form class="tiny-stack" method="post" action="/login/verify"><label class="tiny-field" for="code"><span class="tiny-label">One-time code</span><input class="tiny-input tiny-input--code" id="code" name="code" inputmode="numeric" autocomplete="one-time-code" required></label><input type="hidden" name="email" value="{{.Email}}"><input type="hidden" name="transaction" value="{{.Transaction}}">{{if .Replace}}<input type="hidden" name="replace" value="1">{{end}}<button class="tiny-button tiny-button--primary tiny-button--full" type="submit">Verify and continue</button></form></section></main><script>{{tinyJS}}</script></body></html>`))
var platformNoRoleTemplate = template.Must(template.New("platform-no-role").Funcs(webui.FuncMap()).Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Dashboard unavailable · TinyHost</title><style>{{tinyCSS}}</style></head><body><a class="tiny-skip" href="#main">Skip to content</a><main class="tiny-auth" id="main"><section class="tiny-auth-card" aria-labelledby="no-role-title"><div class="tiny-brand"><span class="tiny-brand__mark" aria-hidden="true">{{tinyMark}}</span><span>tinyhost</span></div><p class="tiny-eyebrow">Dashboard</p><h1 id="no-role-title">This account cannot use the dashboard.</h1><div class="tiny-notice" role="status"><p>Signed in as <strong>{{.Email}}</strong>. You can still open apps where you have access.</p></div><form class="tiny-stack" method="post" action="/logout"><input type="hidden" name="csrf" value="{{.CSRF}}"><button class="tiny-button tiny-button--secondary tiny-button--full" type="submit">Sign out of TinyHost</button></form><p class="tiny-auth-card__footer">Sign out, then use another email if you need dashboard access.</p></section></main><script>{{tinyJS}}</script></body></html>`))
