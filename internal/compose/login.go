package compose

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"github.com/tinyhost/tiny/internal/apps"
	"github.com/tinyhost/tiny/internal/gateway"
	"github.com/tinyhost/tiny/internal/identity"
	"github.com/tinyhost/tiny/internal/otp"
	"github.com/tinyhost/tiny/internal/policies"
	"github.com/tinyhost/tiny/internal/ratelimit"
	"github.com/tinyhost/tiny/internal/sessions"
	webui "github.com/tinyhost/tiny/web"
	"html/template"
	"mime"
	"net/http"
	"net/url"
	"time"
)

var appLoginTemplate = template.Must(
	template.New("app-login").Funcs(webui.FuncMap()).Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <meta name="color-scheme" content="light">
  <title>Sign in · TinyHost</title>
  <style>{{tinyCSS}}</style>
</head>
<body>
  <a class="tiny-skip" href="#main">Skip to content</a>
  <main class="tiny-auth" id="main">
    <section class="tiny-auth-card" aria-labelledby="app-login-title">
      <div class="tiny-brand">
        <span class="tiny-brand__mark" aria-hidden="true">{{tinyMark}}</span>
        <span>protected by tinyhost</span>
      </div>
      <p class="tiny-eyebrow">Private app</p>
      <h1 id="app-login-title">Sign in to continue.</h1>
      <p class="tiny-auth-card__intro">
        Use an email address the app owner has allowed.
      </p>
      <form class="tiny-stack" method="post" action="/_tiny/auth/otp">
        <label class="tiny-field" for="email">
          <span class="tiny-label">Email address</span>
          <input class="tiny-input" id="email" name="email" type="email" required autocomplete="email">
        </label>
        <input type="hidden" name="return" value="{{.}}">
        <button class="tiny-button tiny-button--primary tiny-button--full" type="submit">
          Send one-time code
        </button>
      </form>
      <form class="tiny-stack" method="post" action="/_tiny/auth/logout">
        <input type="hidden" name="return" value="{{.}}">
        <button class="tiny-button tiny-button--secondary tiny-button--full" type="submit">
          Use a different account
        </button>
      </form>
      <p class="tiny-auth-card__footer">
        This app is on the open internet, securely gated by TinyHost.
      </p>
    </section>
  </main>
<script>{{tinyJS}}</script>
</body>
</html>`),
)

var appCodeTemplate = template.Must(
	template.New("app-code").Funcs(webui.FuncMap()).Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <meta name="color-scheme" content="light">
  <title>Check your email · TinyHost</title>
  <style>{{tinyCSS}}</style>
</head>
<body>
  <a class="tiny-skip" href="#main">Skip to content</a>
  <main class="tiny-auth" id="main">
    <section class="tiny-auth-card" aria-labelledby="app-code-title">
      <div class="tiny-brand">
        <span class="tiny-brand__mark" aria-hidden="true">{{tinyMark}}</span>
        <span>protected by tinyhost</span>
      </div>
      <p class="tiny-eyebrow">One small step</p>
      <h1 id="app-code-title">Check your email.</h1>
      <p class="tiny-auth-card__intro">
        If that address is authorized, a one-time code has been sent.
      </p>
      <div class="tiny-notice" role="status">
        <p>Enter the code to continue. This message is the same for every address.</p>
      </div>
      <form class="tiny-stack" method="post" action="/_tiny/auth/verify">
        <label class="tiny-field" for="code">
          <span class="tiny-label">One-time code</span>
          <input
            class="tiny-input tiny-input--code"
            id="code"
            name="code"
            inputmode="numeric"
            autocomplete="one-time-code"
            required
          >
        </label>
        <input type="hidden" name="email" value="{{.Email}}">
        <input type="hidden" name="transaction" value="{{.Transaction}}">
        <input type="hidden" name="return" value="{{.Return}}">
        <button class="tiny-button tiny-button--primary tiny-button--full" type="submit">
          Verify and continue
        </button>
      </form>
    </section>
  </main>
<script>{{tinyJS}}</script>
</body>
</html>`),
)

var appRetryTemplate = template.Must(
	template.New("app-retry").Funcs(webui.FuncMap()).Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <meta name="color-scheme" content="light">
  <title>Sign in · TinyHost</title>
  <style>{{tinyCSS}}</style>
</head>
<body>
  <a class="tiny-skip" href="#main">Skip to content</a>
  <main class="tiny-auth" id="main">
    <section class="tiny-auth-card" aria-labelledby="retry-title">
      <div class="tiny-brand">
        <span class="tiny-brand__mark" aria-hidden="true">{{tinyMark}}</span>
        <span>protected by tinyhost</span>
      </div>
      <p class="tiny-eyebrow">Private app</p>
      <h1 id="retry-title">Sign-in needs another try.</h1>
      <div class="tiny-notice" role="alert">
        <p>We could not complete that sign-in step. Your access has not changed.</p>
      </div>
      <a class="tiny-button tiny-button--primary tiny-button--full" href="/_tiny/auth/login?return={{.}}">Start again</a>
      <p class="tiny-auth-card__footer">Use a different email address if needed. We do not reveal who has access.</p>
    </section>
  </main>
  <script>{{tinyJS}}</script>
</body>
</html>`),
)

// Login is an in-memory/provider-neutral app login dispatcher for local M1 use.
type Login struct {
	OTP        OTPRequesterVerifier
	Sessions   SessionIssuerValidatorRevoker
	Policies   policies.Store
	SessionTTL time.Duration
	Atomic     AtomicAppLogin
	RateLimits *ratelimit.Limiter
	// IdentityBroker is optional only for isolated legacy/local harnesses. A
	// deployed broker owns every browser viewer-session issuance path; direct
	// app OTP endpoints are retired so they cannot mint parentless sessions.
	IdentityBroker *IdentityBroker
}
type AtomicAppLogin interface {
	Request(context.Context, string, string, bool, string) (string, error)
	VerifyAndCreateSession(context.Context, string, string, string, string, time.Time) (string, error)
}
type OTPRequesterVerifier interface {
	Request(context.Context, string, string, bool) error
	Verify(context.Context, string, string, string, string, func(otp.Challenge) error) error
}
type SessionIssuerValidatorRevoker interface {
	sessions.Store
	Create(context.Context, string, identity.Identity, time.Time) (string, sessions.Session, error)
	Revoke(context.Context, string, string) (string, error)
}

func (l Login) DispatchPreAuth(app apps.App, ep gateway.Endpoint, w http.ResponseWriter, r *http.Request) {
	switch ep {
	case gateway.AppLogin:
		ret, ok := gateway.ValidReturnPath(r.URL.Query().Get("return"))
		if !ok {
			ret = "/"
		}
		if l.IdentityBroker != nil {
			if l.IdentityBroker.AppLogin(app.ID, ret, w, r) {
				return
			}
			// Once global identity is configured, browser navigation must not
			// silently fall back to the legacy per-app OTP path on broker failure.
			writeFormRetry(w, http.StatusServiceUnavailable, ret)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_ = appLoginTemplate.Execute(w, ret)
	case gateway.AppIdentityCallback:
		if l.IdentityBroker != nil && l.IdentityBroker.AppCallback(app.ID, w, r) {
			return
		}
		writeFormRetry(w, http.StatusUnauthorized, "/")
	case gateway.AppOTPRequest:
		if l.IdentityBroker != nil {
			l.retiredDirectOTP(w, r)
			return
		}
		var b struct {
			Email string `json:"email"`
		}
		form := hasMediaType(r, "application/x-www-form-urlencoded")
		ret := "/"
		if form {
			r.Body = http.MaxBytesReader(w, r.Body, 4096)
			if r.ParseForm() == nil {
				b.Email = r.PostForm.Get("email")
				if x, ok := gateway.ValidReturnPath(r.PostForm.Get("return")); ok {
					ret = x
				}
			}
		} else if hasMediaType(r, "application/json") {
			_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&b)
		} else {
			writePreAuthDenied(w, http.StatusUnauthorized)
			return
		}
		allowed := l.RateLimits == nil || l.RateLimits.AllowRequest(ratelimit.OTPRequest, r, b.Email, app.ID)
		email, e := identity.Normalize(b.Email)
		eligible := false
		if e == nil && l.Policies != nil {
			if p, x := l.Policies.Current(r.Context(), app.ID); x == nil {
				eligible = policies.Evaluate(p, identity.Identity{ID: email, Email: email}) == policies.Allow
			}
		}
		transaction := opaqueTransaction()
		if allowed && e == nil {
			if l.Atomic != nil {
				if id, err := l.Atomic.Request(r.Context(), app.ID, email, eligible, ratelimit.RequestFingerprint(r)); err == nil && id != "" {
					transaction = id
					if l.RateLimits != nil {
						l.RateLimits.BindTransactionRequest(transaction, email, r)
					}
				}
			} else {
				_ = l.OTP.Request(r.Context(), app.ID, email, eligible)
			}
		}
		if form {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_ = appCodeTemplate.Execute(w, map[string]string{"Email": b.Email, "Transaction": transaction, "Return": ret})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"status":"accepted","transaction":"` + transaction + `"}`))
	case gateway.AppOTPVerify:
		if l.IdentityBroker != nil {
			l.retiredDirectOTP(w, r)
			return
		}
		var b struct {
			Email       string `json:"email"`
			Transaction string `json:"transaction"`
			Code        string `json:"code"`
			Return      string `json:"return"`
		}
		form := hasMediaType(r, "application/x-www-form-urlencoded")
		valid := false
		if form {
			r.Body = http.MaxBytesReader(w, r.Body, 4096)
			if r.ParseForm() == nil {
				b.Email = r.PostForm.Get("email")
				b.Transaction = r.PostForm.Get("transaction")
				b.Code = r.PostForm.Get("code")
				b.Return = r.PostForm.Get("return")
				valid = true
			}
		} else if hasMediaType(r, "application/json") {
			valid = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&b) == nil
		}
		if !valid {
			if form {
				writeFormRetry(w, http.StatusUnauthorized, b.Return)
				return
			}
			writePreAuthDenied(w, http.StatusUnauthorized)
			return
		}
		if l.RateLimits != nil && !l.RateLimits.AllowTransactionRequest(ratelimit.OTPVerify, r, b.Transaction, app.ID) {
			if !form {
				writeRateLimited(w)
				return
			}
			if form {
				writeFormRetry(w, http.StatusTooManyRequests, b.Return)
				return
			}
			writeRateLimited(w)
			return
		}
		email, e := identity.Normalize(b.Email)
		if e != nil || l.Policies == nil || (l.Atomic == nil && l.Sessions == nil) {
			if form {
				writeFormRetry(w, http.StatusUnauthorized, b.Return)
				return
			}
			writePreAuthDenied(w, http.StatusUnauthorized)
			return
		}
		var token string
		ttl := l.SessionTTL
		if ttl <= 0 {
			ttl = 24 * time.Hour
		}
		if l.Atomic != nil {
			token, e = l.Atomic.VerifyAndCreateSession(r.Context(), app.ID, email, b.Transaction, b.Code, time.Now().Add(ttl))
			err := e
			if err == nil {
				http.SetCookie(w, sessions.AppCookie(token, time.Now().Add(ttl)))
				ret, ok := gateway.ValidReturnPath(b.Return)
				if !ok {
					ret = "/"
				}
				http.Redirect(w, r, ret, http.StatusSeeOther)
				return
			}
			if form {
				writeFormRetry(w, http.StatusUnauthorized, b.Return)
				return
			}
			writePreAuthDenied(w, http.StatusUnauthorized)
			return
		}
		err := l.OTP.Verify(r.Context(), app.ID, email, b.Transaction, b.Code, func(_ otp.Challenge) error {
			p, x := l.Policies.Current(r.Context(), app.ID)
			if x != nil || policies.Evaluate(p, identity.Identity{ID: email, Email: email}) != policies.Allow {
				return otp.ErrInvalid
			}
			var z error
			token, _, z = l.Sessions.Create(r.Context(), app.ID, identity.Identity{ID: email, Email: email}, time.Now().Add(ttl))
			return z
		})
		if err != nil {
			if form {
				writeFormRetry(w, http.StatusUnauthorized, b.Return)
				return
			}
			writePreAuthDenied(w, http.StatusUnauthorized)
			return
		}
		http.SetCookie(w, sessions.AppCookie(token, time.Now().Add(ttl)))
		ret, ok := gateway.ValidReturnPath(b.Return)
		if !ok {
			ret = "/"
		}
		http.Redirect(w, r, ret, http.StatusSeeOther)
	case gateway.AppLogout:
		if !gateway.SameOrigin(r) {
			http.Error(w, "not authorized", http.StatusUnauthorized)
			return
		}
		form := hasMediaType(r, "application/x-www-form-urlencoded")
		ret := "/"
		if form {
			r.Body = http.MaxBytesReader(w, r.Body, 4096)
			if r.ParseForm() != nil {
				http.Error(w, "not authorized", http.StatusUnauthorized)
				return
			}
			x, ok := gateway.ValidReturnPath(r.PostForm.Get("return"))
			if !ok {
				http.Error(w, "not authorized", http.StatusUnauthorized)
				return
			}
			ret = x
		}
		if c, e := r.Cookie(sessions.AppCookieName); e == nil && l.Sessions != nil {
			_, _ = l.Sessions.Revoke(r.Context(), app.ID, c.Value)
		}
		http.SetCookie(w, &http.Cookie{Name: sessions.AppCookieName, Value: "", Path: "/", MaxAge: -1, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
		if form {
			http.Redirect(w, r, "/_tiny/auth/login?return="+url.QueryEscape(ret), http.StatusSeeOther)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// retiredDirectOTP keeps the old paths safe for stale forms and clients while
// ensuring the configured global-identity deployment never issues an app
// session without a revocable parent identity. Form callers retain the native
// generic retry screen; JSON callers retain a bounded machine-safe denial.
func (l Login) retiredDirectOTP(w http.ResponseWriter, r *http.Request) {
	if hasMediaType(r, "application/x-www-form-urlencoded") {
		writeFormRetry(w, http.StatusGone, "/")
		return
	}
	writePreAuthDenied(w, http.StatusUnauthorized)
}

func hasMediaType(r *http.Request, want string) bool {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && media == want
}

func writeFormRetry(w http.ResponseWriter, status int, rawReturn string) {
	ret, ok := gateway.ValidReturnPath(rawReturn)
	if !ok {
		ret = "/"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = appRetryTemplate.Execute(w, ret)
}

func writePreAuthDenied(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"error":{"code":"not_authorized","message":"Request not authorized."}}`))
}

func writeRateLimited(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)
	_, _ = w.Write([]byte(`{"error":{"code":"rate_limited","message":"Request could not be completed."}}`))
}
func opaqueTransaction() string {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		return "otp_unavailable000000000000000000000000"
	}
	return "otp_" + hex.EncodeToString(b)
}
