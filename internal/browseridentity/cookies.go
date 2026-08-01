// Package browseridentity owns the admin-host-only browser credential
// names. Keeping these outside HTTP composition prevents a persistence or
// dashboard adapter from inventing a second cookie contract.
package browseridentity

import (
	"net/http"
	"time"
)

const (
	IdentityCookieName = "__Host-tinker_identity"
	BindingCookieName  = "__Host-tinker_browser"
	CSRFCookieName     = "__Host-tinker_identity_csrf"
)

func IdentityCookie(raw string, expires time.Time) *http.Cookie {
	return &http.Cookie{Name: IdentityCookieName, Value: raw, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, Expires: expires}
}

func ExpiredCookie(name string) *http.Cookie {
	return &http.Cookie{Name: name, Value: "", Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1}
}
