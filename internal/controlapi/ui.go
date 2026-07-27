package controlapi

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tinyhost/tiny/internal/ratelimit"
	webui "github.com/tinyhost/tiny/web"
)

const ControlCookieName = "__Host-tiny_control"
const controlCSRFCookie = "__Host-tiny_control_csrf"

//go:embed templates/*.html
var templateFiles embed.FS

// Platform serves the platform host only. API routes remain bearer-token
// authenticated through API; browser forms never proxy into that API.
type Platform struct {
	API   http.Handler
	Auth  PlatformAuthenticator
	Login Login
	Views DashboardReader
	// Actions is deliberately separate from API. HTML forms call the same
	// typed service boundary, with the actor always derived from the control
	// cookie. They never replay browser credentials into the bearer API.
	Actions    UIActions
	Now        func() time.Time
	RateLimits *ratelimit.Limiter
}

// UIActions is the narrow, audited mutation surface used by the platform-host
// forms. Implementations must re-check role and ownership; route parameters
// and form fields are untrusted input.
type UIActions interface {
	SetDeployerStatus(context.Context, Actor, string, string, string) error
	ReplaceAccess(context.Context, Actor, string, AccessPolicyInput, string) error
	CreateToken(context.Context, Actor, string, TokenInput, string) (TokenResult, error)
	RevokeToken(context.Context, Actor, string, string, string) error
	Rollback(context.Context, Actor, string, string, string) error
	SetAppStatus(context.Context, Actor, string, string, string) error
	DeleteApp(context.Context, Actor, string, string) error
}

type controlCredentialRevoker interface {
	RevokeControlCredential(context.Context, string) error
}

func (p Platform) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		if p.API == nil {
			http.NotFound(w, r)
			return
		}
		p.API.ServeHTTP(w, r)
		return
	}
	switch r.URL.Path {
	case "/", "/dashboard":
		if r.Method != http.MethodGet {
			p.methodNotAllowed(w)
			return
		}
		p.dashboard(w, r)
	case "/login":
		switch r.Method {
		case http.MethodGet:
			p.loginForm(w, "", "")
		case http.MethodPost:
			p.requestOTP(w, r)
		default:
			p.methodNotAllowed(w)
		}
	case "/login/verify":
		switch r.Method {
		case http.MethodGet:
			p.verifyForm(w, r.URL.Query().Get("transaction"), "")
		case http.MethodPost:
			p.verifyOTP(w, r)
		default:
			p.methodNotAllowed(w)
		}
	case "/logout":
		if r.Method != http.MethodPost {
			p.methodNotAllowed(w)
			return
		}
		p.logout(w, r)
	default:
		if r.Method == http.MethodPost && p.formAction(w, r) {
			return
		}
		p.errorPage(w, http.StatusNotFound, "Page not found", "This TinyHost page is unavailable or has moved.", "/login", "Go to sign in")
	}
}

// formAction handles only fixed route shapes. It enforces the browser trust
// checks before parsing any action payload, then invokes an audited service
// with a server-derived actor and a server-generated idempotency key.
func (p Platform) formAction(w http.ResponseWriter, r *http.Request) bool {
	a, ok := p.actor(r)
	if !ok || !sameOrigin(r) || !p.validCSRF(r) || p.Actions == nil {
		p.errorPage(w, http.StatusForbidden, "Action not authorized", "This request could not be completed. Return to the dashboard and try again.", "/dashboard", "Return to dashboard")
		return true
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 {
		return false
	}
	key := newUIRequestKey()
	if key == "" {
		p.errorPage(w, http.StatusServiceUnavailable, "Action unavailable", "TinyHost could not prepare this action. Retry from the dashboard.", "/dashboard", "Return to dashboard")
		return true
	}
	var err error
	if len(parts) == 3 && parts[0] == "deployers" && (parts[2] == "authorize" || parts[2] == "suspend" || parts[2] == "revoke") {
		if a.Role != "operator" || r.FormValue("confirmation") != parts[2]+":"+strings.ToLower(strings.TrimSpace(parts[1])) {
			p.errorPage(w, http.StatusForbidden, "Action not authorized", "This request could not be completed. Return to the dashboard and try again.", "/dashboard", "Return to dashboard")
			return true
		}
		err = p.Actions.SetDeployerStatus(r.Context(), a, parts[1], map[string]string{"authorize": "active", "suspend": "suspended", "revoke": "revoked"}[parts[2]], key)
		return p.actionResult(w, r, err, "")
	}
	if parts[0] != "apps" || len(parts) < 3 {
		return false
	}
	slug, action := parts[1], parts[2]
	switch {
	case len(parts) == 3 && action == "access":
		v := AccessPolicyInput{Mode: "private"}
		revision, scan := strconv.ParseUint(r.FormValue("expected_revision"), 10, 64)
		if scan != nil || revision == 0 {
			p.errorPage(w, http.StatusBadRequest, "Policy needs review", "Refresh the dashboard and review the current private policy before trying again.", "/dashboard", "Refresh dashboard")
			return true
		}
		v.ExpectedRevision = revision
		v.ConfirmBroadening = r.FormValue("confirm_broadening") == "confirm"
		v.Allow.Emails = splitFormList(r.FormValue("emails"))
		v.Allow.Domains = splitFormList(r.FormValue("domains"))
		if !validAccess(&v) {
			p.errorPage(w, http.StatusBadRequest, "Policy needs review", "Use valid, unique email addresses and domains, then try again.", "/dashboard", "Return to dashboard")
			return true
		}
		err = p.Actions.ReplaceAccess(r.Context(), a, slug, v, key)
		return p.actionResult(w, r, err, "policy_replaced")
	case len(parts) == 3 && action == "tokens":
		v := TokenInput{Scopes: splitFormList(r.FormValue("scopes"))}
		if _, scan := fmt.Sscan(r.FormValue("expires_in_seconds"), &v.ExpiresInSeconds); scan != nil || !validTokenInput(v) {
			p.errorPage(w, http.StatusBadRequest, "Token settings need review", "Choose valid scopes and a permitted lifetime, then try again.", "/dashboard", "Return to dashboard")
			return true
		}
		out, e := p.Actions.CreateToken(r.Context(), a, slug, v, key)
		if e != nil {
			return p.actionResult(w, r, e, "")
		}
		p.render(w, "token.html", tokenPage{Actor: a, Token: out})
		return true
	case len(parts) == 5 && action == "tokens" && parts[3] != "" && parts[4] == "revoke":
		err = p.Actions.RevokeToken(r.Context(), a, slug, parts[3], key)
	case len(parts) == 3 && action == "rollback":
		id := r.FormValue("deployment")
		if id == "" {
			p.errorPage(w, http.StatusBadRequest, "Release needs review", "Choose an immutable release from the dashboard before trying again.", "/dashboard", "Return to dashboard")
			return true
		}
		err = p.Actions.Rollback(r.Context(), a, slug, id, key)
	case len(parts) == 3 && (action == "suspend" || action == "resume"):
		err = p.Actions.SetAppStatus(r.Context(), a, slug, map[string]string{"suspend": "suspended", "resume": "active"}[action], key)
	case len(parts) == 3 && action == "delete":
		if r.FormValue("confirmation") != "delete:"+slug {
			p.errorPage(w, http.StatusBadRequest, "Confirmation required", "Type the exact app target shown on the dashboard before deletion can continue.", "/dashboard", "Return to dashboard")
			return true
		}
		err = p.Actions.DeleteApp(r.Context(), a, slug, key)
	default:
		return false
	}
	return p.actionResult(w, r, err, "")
}

func (p Platform) actionResult(w http.ResponseWriter, r *http.Request, err error, notice string) bool {
	if err != nil {
		if errors.Is(err, ErrPolicyRevision) {
			p.errorPage(w, http.StatusConflict, "Policy changed", "Someone or a deployment changed this policy. Refresh and review the current revision before retrying.", "/dashboard", "Refresh dashboard")
			return true
		}
		p.errorPage(w, http.StatusForbidden, "Action unavailable", "TinyHost could not complete this action. Return to the dashboard and review the current state.", "/dashboard", "Return to dashboard")
		return true
	}
	location := "/dashboard"
	if notice == "policy_replaced" {
		location += "?notice=" + notice
	}
	http.Redirect(w, r, location, http.StatusSeeOther)
	return true
}

func newUIRequestKey() string {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return "ui_" + base64.RawURLEncoding.EncodeToString(b)
}

func splitFormList(s string) []string {
	items := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '\n' || r == '\r' || r == '\t' })
	for i := range items {
		items[i] = strings.TrimSpace(items[i])
	}
	sort.Strings(items)
	return items
}

func (p Platform) dashboard(w http.ResponseWriter, r *http.Request) {
	a, ok := p.actor(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	csrf := p.csrf(w, r)
	v := DashboardView{Health: []DashboardHealth{{Name: "host diagnostics", State: "local", Detail: "Run tinyhost doctor on the VPS for database, disk, DNS, TLS, and email diagnostics."}}}
	if p.Views != nil {
		var err error
		v, err = p.Views.Dashboard(r.Context(), a)
		if err != nil {
			v = DashboardView{Health: []DashboardHealth{{Name: "dashboard read model", State: "unavailable", Detail: "Retry or run tinyhost doctor locally."}}}
		}
	}
	notice := ""
	if r.URL.Query().Get("notice") == "policy_replaced" {
		notice = "Access policy replaced. Review the current revision and additional viewers below."
	}
	p.render(w, "dashboard.html", dashboardPage{Actor: a, View: v, CSRF: csrf, Notice: notice})
}
func (p Platform) loginForm(w http.ResponseWriter, tx, message string) {
	p.render(w, "login.html", loginPage{Transaction: tx, Message: message})
}
func (p Platform) requestOTP(w http.ResponseWriter, r *http.Request) {
	if p.Login == nil {
		p.loginForm(w, "", "Unable to continue. Try again later.")
		return
	}
	_ = r.ParseForm()
	email := r.Form.Get("email")
	tx := ""
	if p.RateLimits == nil || p.RateLimits.Allow(ratelimit.OTPRequest, r.RemoteAddr, email, "control") {
		tx, _ = p.Login.RequestOTP(r.Context(), email, BrowserLoginChannel)
		if tx != "" && p.RateLimits != nil {
			p.RateLimits.BindTransaction(tx, email)
		}
	}
	// Always show the same state to prevent account enumeration.
	p.verifyForm(w, tx, "If that address is authorized, a code has been sent.")
}
func (p Platform) verifyForm(w http.ResponseWriter, tx, message string) {
	p.render(w, "verify.html", verifyPage{Transaction: tx, Message: message})
}
func (p Platform) verifyOTP(w http.ResponseWriter, r *http.Request) {
	if p.Login == nil {
		p.verifyForm(w, "", "Unable to verify that code.")
		return
	}
	_ = r.ParseForm()
	if p.RateLimits != nil && !p.RateLimits.AllowTransaction(ratelimit.OTPVerify, r.RemoteAddr, r.Form.Get("transaction"), "control") {
		p.verifyForm(w, r.Form.Get("transaction"), "Unable to verify that code.")
		return
	}
	token, err := p.Login.VerifyOTP(r.Context(), r.Form.Get("transaction"), r.Form.Get("code"), BrowserLoginChannel)
	if err != nil || token == "" {
		p.verifyForm(w, r.Form.Get("transaction"), "Unable to verify that code.")
		return
	}
	now := time.Now()
	if p.Now != nil {
		now = p.Now()
	}
	http.SetCookie(w, &http.Cookie{Name: ControlCookieName, Value: token, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, Expires: now.Add(30 * 24 * time.Hour)})
	p.csrf(w, r)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
func (p Platform) logout(w http.ResponseWriter, r *http.Request) {
	if _, ok := p.actor(r); !ok || !sameOrigin(r) || !p.validCSRF(r) {
		p.errorPage(w, http.StatusForbidden, "Sign out not authorized", "This request could not be completed. Return to the dashboard and try again.", "/dashboard", "Return to dashboard")
		return
	}
	if c, err := r.Cookie(ControlCookieName); err == nil {
		if revoker, ok := p.Login.(controlCredentialRevoker); ok {
			_ = revoker.RevokeControlCredential(r.Context(), c.Value)
		}
	}
	http.SetCookie(w, &http.Cookie{Name: ControlCookieName, Value: "", Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	http.SetCookie(w, &http.Cookie{Name: controlCSRFCookie, Value: "", Path: "/", Secure: true, SameSite: http.SameSiteStrictMode, MaxAge: -1})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
func (p Platform) actor(r *http.Request) (Actor, bool) {
	if p.Auth == nil {
		return Actor{}, false
	}
	a, err := p.Auth.AuthenticatePlatform(r.Context(), r)
	return a, err == nil && a.Active
}
func (p Platform) csrf(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie(controlCSRFCookie); err == nil && len(c.Value) >= 32 {
		return c.Value
	}
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	v := base64.RawURLEncoding.EncodeToString(b)
	http.SetCookie(w, &http.Cookie{Name: controlCSRFCookie, Value: v, Path: "/", Secure: true, SameSite: http.SameSiteStrictMode})
	return v
}
func (p Platform) validCSRF(r *http.Request) bool {
	c, e := r.Cookie(controlCSRFCookie)
	if e != nil || c.Value == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(c.Value), []byte(r.FormValue("csrf"))) == 1
}
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		origin = r.Header.Get("Referer")
	}
	if origin == "" {
		return false
	}
	u, err := url.Parse(origin)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && strings.EqualFold(u.Host, r.Host)
}
func (p Platform) methodNotAllowed(w http.ResponseWriter) {
	w.Header().Set("Allow", "GET, POST")
	p.errorPage(w, http.StatusMethodNotAllowed, "Method not allowed", "Use the controls on this page to continue safely.", "/dashboard", "Return to dashboard")
}

func (p Platform) errorPage(w http.ResponseWriter, status int, title, message, href, label string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	p.render(w, "error.html", errorPage{Title: title, Message: message, Href: href, Label: label})
}
func (p Platform) render(w http.ResponseWriter, name string, data any) {
	t, err := template.New("base.html").Funcs(webui.FuncMap()).ParseFS(
		templateFiles,
		"templates/base.html",
		"templates/"+name,
	)
	if err != nil {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = t.ExecuteTemplate(w, "base", data)
}

type loginPage struct{ Transaction, Message string }
type verifyPage struct{ Transaction, Message string }
type dashboardPage struct {
	Actor  Actor
	View   DashboardView
	CSRF   string
	Notice string
}
type tokenPage struct {
	Actor Actor
	Token TokenResult
}
type errorPage struct{ Title, Message, Href, Label string }
