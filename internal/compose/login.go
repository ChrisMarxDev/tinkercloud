package compose

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"mime"
	"net/http"
	"net/url"

	"github.com/tinyhost/tiny/internal/apps"
	"github.com/tinyhost/tiny/internal/gateway"
	"github.com/tinyhost/tiny/internal/sessions"
)

// Login implements the only app-host pre-authentication routes left in the
// browser architecture. Identity issuance is deliberately absent here: the
// admin-host IdentityBroker owns every OTP and global-identity transition.
//
// An app login is therefore only a broker handoff. A local app logout only
// revokes this host's child session; it cannot sign a browser out elsewhere.
type Login struct {
	Sessions       AppSessionRevoker
	IdentityBroker *IdentityBroker
}

// AppSessionRevoker is intentionally narrower than the app authorization
// store. The pre-auth handler cannot issue a child session directly.
type AppSessionRevoker interface {
	Revoke(context.Context, string, string) (string, error)
}

// SessionValidatorRevoker is the app-plane capability boundary. App
// authorization needs validation and app-local logout needs revocation; child
// session creation belongs exclusively to the broker persistence transaction.
type SessionValidatorRevoker interface {
	sessions.Store
	Revoke(context.Context, string, string) (string, error)
}

func (l Login) DispatchPreAuth(app apps.App, ep gateway.Endpoint, w http.ResponseWriter, r *http.Request) {
	switch ep {
	case gateway.AppLogin:
		ret, ok := gateway.ValidReturnPath(r.URL.Query().Get("return"))
		if !ok {
			ret = "/"
		}
		if l.IdentityBroker != nil && l.IdentityBroker.AppLogin(app.ID, ret, w, r) {
			return
		}
		// A missing/unavailable broker is never permission to fall back to an
		// app-local OTP. That former path would mint an unparented session.
		writePreAuthRetry(w, http.StatusServiceUnavailable)
	case gateway.AppIdentityCallback:
		if l.IdentityBroker != nil && l.IdentityBroker.AppCallback(app.ID, w, r) {
			return
		}
		writePreAuthRetry(w, http.StatusUnauthorized)
	case gateway.AppLogout:
		l.appLogout(app, w, r)
	default:
		writePreAuthRetry(w, http.StatusNotFound)
	}
}

func (l Login) appLogout(app apps.App, w http.ResponseWriter, r *http.Request) {
	if !gateway.SameOrigin(r) {
		writePreAuthRetry(w, http.StatusUnauthorized)
		return
	}
	form := hasMediaType(r, "application/x-www-form-urlencoded")
	ret := "/"
	if form {
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		if r.ParseForm() != nil {
			writePreAuthRetry(w, http.StatusBadRequest)
			return
		}
		var ok bool
		ret, ok = gateway.ValidReturnPath(r.PostForm.Get("return"))
		if !ok {
			writePreAuthRetry(w, http.StatusBadRequest)
			return
		}
	}
	if c, err := r.Cookie(sessions.AppCookieName); err == nil && c.Value != "" {
		if l.Sessions == nil {
			writePreAuthRetry(w, http.StatusServiceUnavailable)
			return
		}
		if _, err := l.Sessions.Revoke(r.Context(), app.ID, c.Value); err != nil && !errors.Is(err, sessions.ErrInvalid) {
			// Do not claim the session was revoked or clear its cookie when the
			// durable write did not complete.
			writePreAuthRetry(w, http.StatusServiceUnavailable)
			return
		}
	}
	http.SetCookie(w, &http.Cookie{Name: sessions.AppCookieName, Value: "", Path: "/", MaxAge: -1, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	if form {
		http.Redirect(w, r, "/_tiny/auth/login?return="+url.QueryEscape(ret), http.StatusSeeOther)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func hasMediaType(r *http.Request, want string) bool {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && media == want
}

func writePreAuthRetry(w http.ResponseWriter, status int) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"error":{"code":"not_authorized","message":"Request not authorized."}}`))
}

func opaqueTransaction() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "otp_unavailable000000000000000000000000"
	}
	return "otp_" + hex.EncodeToString(b)
}
