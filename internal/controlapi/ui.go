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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ChrisMarxDev/tinkercloud/internal/browseridentity"
	"github.com/ChrisMarxDev/tinkercloud/internal/gateway"
	"github.com/ChrisMarxDev/tinkercloud/internal/identity"
	"github.com/ChrisMarxDev/tinkercloud/internal/releases"
	webui "github.com/ChrisMarxDev/tinkercloud/web"
)

//go:embed templates/*.html
var templateFiles embed.FS

// Platform serves the platform host only. API routes remain bearer-token
// authenticated through API; browser forms never proxy into that API.
type Platform struct {
	API        http.Handler
	Auth       PlatformAuthenticator
	ViewerAuth ViewerAuthenticator
	Views      DashboardReader
	Catalogs   CatalogReader
	// Actions is deliberately separate from API. HTML forms call the same
	// typed service boundary, with the actor always derived from the control
	// cookie. They never replay browser credentials into the bearer API.
	Actions UIActions
	Now     func() time.Time
}

// UIActions is the narrow, audited mutation surface used by the admin-host
// forms. Implementations must re-check role and ownership; route parameters
// and form fields are untrusted input.
type UIActions interface {
	ReplaceActiveDeployers(context.Context, Actor, []string, string, bool, string) error
	ReplaceAccess(context.Context, Actor, string, AccessPolicyInput, string) error
	CreateToken(context.Context, Actor, string, TokenInput, string) (TokenResult, error)
	RevokeToken(context.Context, Actor, string, string, string) error
	SetAppStatus(context.Context, Actor, string, string, string) error
	DeleteApp(context.Context, Actor, string, string) error
	CreateLLMConnection(context.Context, Actor, string, string) error
	RotateLLMConnection(context.Context, Actor, string, string) error
	DisableLLMConnection(context.Context, Actor, string) error
	CreateLLMProfile(context.Context, Actor, LLMProfileInput) error
	UpdateLLMProfile(context.Context, Actor, string, LLMProfileInput) error
	ApproveLLMGrant(context.Context, Actor, string, string, uint64) error
	SetLLMGrantStatus(context.Context, Actor, string, string, uint64) error
}

const maxUIFormBytes int64 = 16 << 10

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
	case "/apps":
		if r.Method != http.MethodGet {
			p.methodNotAllowed(w)
			return
		}
		p.catalog(w, r)
	default:
		if r.Method == http.MethodPost && p.formAction(w, r) {
			return
		}
		p.errorPage(w, http.StatusNotFound, "Page not found", "This Tinkercloud page is unavailable or has moved.", "/login", "Go to sign in")
	}
}

// catalog is a global-viewer-identity surface, not a less restrictive control
// dashboard. Its authenticator deliberately never derives a role.
func (p Platform) catalog(w http.ResponseWriter, r *http.Request) {
	if p.ViewerAuth == nil || p.Catalogs == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	viewer, err := p.ViewerAuth.AuthenticateViewer(r.Context(), w, r)
	if err != nil || viewer.IdentityID == "" || viewer.Email == "" {
		// No catalog template or read model is rendered before identity proof.
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	apps, err := p.Catalogs.Catalog(r.Context(), viewer)
	if err != nil {
		// A failed safe read model is unavailable, never a partially rendered
		// catalog. This also makes an adapter contract violation non-disclosing.
		apps = nil
	}
	csrf := p.csrf(w, r, Actor{IdentitySessionID: viewer.IdentitySessionID})
	p.render(w, "catalog.html", catalogPage{Apps: apps, Tags: catalogTags(apps), CSRF: csrf, Unavailable: err != nil})
}

func catalogTags(apps []CatalogApp) []string {
	seen := make(map[string]struct{})
	for _, app := range apps {
		for _, tag := range app.Tags {
			seen[tag] = struct{}{}
		}
	}
	result := make([]string, 0, len(seen))
	for tag := range seen {
		result = append(result, tag)
	}
	sort.Strings(result)
	return result
}

// formAction handles only fixed route shapes. It enforces the browser trust
// checks before parsing any action payload, then invokes an audited service
// with a server-derived actor and a server-generated idempotency key.
func (p Platform) formAction(w http.ResponseWriter, r *http.Request) bool {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 2 || (parts[0] != "apps" && parts[0] != "deployers" && parts[0] != "dashboard") {
		return false
	}
	a, ok := p.actor(w, r)
	if !ok || !gateway.SameOrigin(r) || !p.validCSRF(r, a) || p.Actions == nil {
		p.errorPage(w, http.StatusForbidden, "Action not authorized", "This request could not be completed. Return to the dashboard and try again.", "/dashboard", "Return to dashboard")
		return true
	}
	key := newUIRequestKey()
	if key == "" {
		p.errorPage(w, http.StatusServiceUnavailable, "Action unavailable", "Tinkercloud could not prepare this action. Retry from the dashboard.", "/dashboard", "Return to dashboard")
		return true
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUIFormBytes)
	if err := r.ParseForm(); err != nil {
		p.errorPage(w, http.StatusBadRequest, "Form needs review", "Use the controls on the dashboard and try again.", "/dashboard", "Return to dashboard")
		return true
	}
	var err error
	if len(parts) == 3 && parts[0] == "dashboard" && parts[1] == "llm" && parts[2] == "connections" {
		if a.Role != "operator" {
			p.errorPage(w, http.StatusForbidden, "Action not authorized", "This request could not be completed. Return to the dashboard and try again.", "/dashboard", "Return to dashboard")
			return true
		}
		provider, secret := r.FormValue("provider"), r.FormValue("secret")
		_, browserName := r.Form["display_name"]
		_, browserID := r.Form["id"]
		_, browserConnectionID := r.Form["connection_id"]
		if browserName || browserID || browserConnectionID || (provider != "anthropic" && provider != "gemini") || len(secret) < 1 || len(secret) > 4096 {
			p.errorPage(w, http.StatusBadRequest, "API key needs review", "Choose a supported provider and enter a valid API key.", "/dashboard", "Return to dashboard")
			return true
		}
		err = p.Actions.CreateLLMConnection(r.Context(), a, provider, secret)
		return p.actionResult(w, r, err, "llm_connection_created")
	}
	if len(parts) == 5 && parts[0] == "dashboard" && parts[1] == "llm" && parts[2] == "connections" && parts[4] == "rotate" {
		if a.Role != "operator" || !safeLLMID(parts[3]) {
			p.errorPage(w, 403, "Action not authorized", "This request could not be completed.", "/dashboard", "Return to dashboard")
			return true
		}
		secret := r.FormValue("secret")
		if len(secret) < 1 || len(secret) > 4096 || r.FormValue("provider") != "" {
			p.errorPage(w, 400, "Connection needs review", "Use a valid replacement credential.", "/dashboard", "Return to dashboard")
			return true
		}
		err = p.Actions.RotateLLMConnection(r.Context(), a, parts[3], secret)
		return p.actionResult(w, r, err, "llm_connection_rotated")
	}
	if len(parts) == 5 && parts[0] == "dashboard" && parts[1] == "llm" && parts[2] == "connections" && parts[4] == "disable" {
		if a.Role != "operator" || !safeLLMID(parts[3]) || r.FormValue("confirmation") != "disable:"+parts[3] {
			p.errorPage(w, 400, "Confirmation required", "Type the exact connection target before disabling it.", "/dashboard", "Return to dashboard")
			return true
		}
		err = p.Actions.DisableLLMConnection(r.Context(), a, parts[3])
		return p.actionResult(w, r, err, "llm_connection_disabled")
	}
	if len(parts) == 3 && parts[0] == "dashboard" && parts[1] == "llm" && parts[2] == "profiles" {
		if a.Role != "operator" {
			p.errorPage(w, 403, "Action not authorized", "This request could not be completed.", "/dashboard", "Return to dashboard")
			return true
		}
		in, ok := parseLLMProfile(r)
		if !ok || in.ExpectedRevision != 0 {
			p.errorPage(w, 400, "Profile needs review", "Choose an active connection, model, and all bounded limits.", "/dashboard", "Return to dashboard")
			return true
		}
		err = p.Actions.CreateLLMProfile(r.Context(), a, in)
		return p.actionResult(w, r, err, "llm_profile_created")
	}
	if len(parts) == 4 && parts[0] == "dashboard" && parts[1] == "llm" && parts[2] == "profiles" {
		if a.Role != "operator" || !safeLLMID(parts[3]) {
			p.errorPage(w, 403, "Action not authorized", "This request could not be completed.", "/dashboard", "Return to dashboard")
			return true
		}
		in, ok := parseLLMProfile(r)
		if !ok || in.ExpectedRevision == 0 {
			p.errorPage(w, 400, "Profile needs review", "Refresh the dashboard and review every profile limit before saving.", "/dashboard", "Return to dashboard")
			return true
		}
		err = p.Actions.UpdateLLMProfile(r.Context(), a, parts[3], in)
		return p.actionResult(w, r, err, "llm_profile_updated")
	}
	if len(parts) == 2 && parts[0] == "deployers" && parts[1] == "active" {
		if a.Role != "operator" {
			p.errorPage(w, http.StatusForbidden, "Action not authorized", "This request could not be completed. Return to the dashboard and try again.", "/dashboard", "Return to dashboard")
			return true
		}
		emails, listErr := normalizedEmailList(r.FormValue("emails"), 100, 8192)
		if listErr != nil {
			p.errorPage(w, http.StatusBadRequest, "Deployer allowlist needs review", "Use at most 100 unique, valid email addresses—one per line—then try again.", "/dashboard", "Return to dashboard")
			return true
		}
		err = p.Actions.ReplaceActiveDeployers(r.Context(), a, emails, r.FormValue("expected_revision"), r.FormValue("confirm_broadening") == "confirm", key)
		return p.actionResult(w, r, err, "")
	}
	if parts[0] != "apps" || len(parts) < 3 {
		return false
	}
	slug, action := parts[1], parts[2]
	switch {
	case len(parts) == 4 && action == "llm" && parts[3] == "grant":
		if a.Role != "operator" || !releases.ValidSlug(slug) || !safeLLMID(r.FormValue("profile_id")) {
			p.errorPage(w, http.StatusBadRequest, "Grant needs review", "Choose a current profile from the dashboard and try again.", "/dashboard", "Return to dashboard")
			return true
		}
		expected, ok := parsePositiveOrZero(r.FormValue("expected_revision"))
		if !ok {
			p.errorPage(w, 400, "Grant needs review", "Refresh the dashboard and review the current app grant before saving.", "/dashboard", "Refresh dashboard")
			return true
		}
		err = p.Actions.ApproveLLMGrant(r.Context(), a, slug, r.FormValue("profile_id"), expected)
		return p.actionResult(w, r, err, "llm_grant_approved")
	case len(parts) == 5 && action == "llm" && parts[3] == "grant" && (parts[4] == "disable" || parts[4] == "revoke"):
		if a.Role != "operator" || !releases.ValidSlug(slug) {
			p.errorPage(w, 403, "Action not authorized", "This request could not be completed.", "/dashboard", "Return to dashboard")
			return true
		}
		expected, ok := parsePositiveOrZero(r.FormValue("expected_revision"))
		status := parts[4] + "d"
		if !ok || expected == 0 || r.FormValue("confirmation") != status+":grant:"+slug {
			p.errorPage(w, 400, "Confirmation required", "Type the exact app grant target before changing it.", "/dashboard", "Return to dashboard")
			return true
		}
		err = p.Actions.SetLLMGrantStatus(r.Context(), a, slug, status, expected)
		return p.actionResult(w, r, err, "llm_grant_"+status)
	case len(parts) == 3 && action == "access":
		mode := r.FormValue("mode")
		// Older rendered pages remain safe: omission can only request the
		// historical private mode, and the service still compares it with the
		// current server policy before writing anything.
		if mode == "" {
			mode = "private"
		}
		v := AccessPolicyInput{Mode: mode}
		revision, scan := strconv.ParseUint(r.FormValue("expected_revision"), 10, 64)
		if scan != nil || revision == 0 {
			p.errorPage(w, http.StatusBadRequest, "Policy needs review", "Refresh the dashboard and review the current access policy before trying again.", "/dashboard", "Refresh dashboard")
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

func normalizedEmailList(raw string, maxEntries, maxBytes int) ([]string, error) {
	if len(raw) > maxBytes {
		return nil, errors.New("too large")
	}
	items := strings.FieldsFunc(raw, func(r rune) bool { return r == '\n' || r == '\r' || r == ',' })
	if len(items) > maxEntries {
		return nil, errors.New("too many")
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(items))
	for _, item := range items {
		email, err := identity.Normalize(strings.TrimSpace(item))
		if err != nil || seen[email] {
			return nil, errors.New("invalid email list")
		}
		seen[email] = true
		out = append(out, email)
	}
	sort.Strings(out)
	return out, nil
}

func (p Platform) actionResult(w http.ResponseWriter, r *http.Request, err error, notice string) bool {
	if err != nil {
		if errors.Is(err, ErrDeployerRevision) {
			p.errorPage(w, http.StatusConflict, "Deployer list changed", "Refresh the dashboard and review the current active deployer list before saving.", "/dashboard", "Refresh dashboard")
			return true
		}
		if errors.Is(err, ErrPolicyRevision) {
			p.errorPage(w, http.StatusConflict, "Policy changed", "Someone or a deployment changed this policy. Refresh and review the current revision before retrying.", "/dashboard", "Refresh dashboard")
			return true
		}
		if errors.Is(err, ErrLLMRevision) {
			p.errorPage(w, http.StatusConflict, "LLM settings changed", "Refresh the dashboard and review the current profile or app grant before trying again.", "/dashboard", "Refresh dashboard")
			return true
		}
		p.errorPage(w, http.StatusForbidden, "Action unavailable", "Tinkercloud could not complete this action. Return to the dashboard and review the current state.", "/dashboard", "Return to dashboard")
		return true
	}
	location := "/dashboard"
	if notice == "policy_replaced" || strings.HasPrefix(notice, "llm_") {
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

// safeLLMID admits only opaque IDs issued by Tinkercloud. Browser forms never
// create IDs and routes cannot be used to select arbitrary database rows.
func safeLLMID(value string) bool {
	if len(value) != 32 {
		return false
	}
	for _, c := range value {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func parsePositiveOrZero(value string) (uint64, bool) {
	if value == "" || len(value) > 20 {
		return 0, false
	}
	v, err := strconv.ParseUint(value, 10, 64)
	return v, err == nil
}

func parseLLMProfile(r *http.Request) (LLMProfileInput, bool) {
	input := LLMProfileInput{ConnectionID: r.FormValue("connection_id"), Model: strings.TrimSpace(r.FormValue("model"))}
	if !safeLLMID(input.ConnectionID) || input.Model == "" || len(input.Model) > 128 || strings.ContainsAny(input.Model, "\x00\r\n") {
		return LLMProfileInput{}, false
	}
	intField := func(name string, destination *int) bool {
		value := r.FormValue(name)
		if value == "" || len(value) > 12 {
			return false
		}
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 {
			return false
		}
		*destination = parsed
		return true
	}
	int64Field := func(name string, destination *int64) bool {
		value := r.FormValue(name)
		if value == "" || len(value) > 16 {
			return false
		}
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil || parsed < 1 {
			return false
		}
		*destination = parsed
		return true
	}
	if !intField("max_messages", &input.MaxMessages) || !intField("max_message_bytes", &input.MaxMessageBytes) || !intField("max_input_bytes", &input.MaxInputBytes) || !intField("max_output_tokens", &input.MaxOutputTokens) || !int64Field("timeout_ms", &input.TimeoutMS) || !intField("viewer_requests", &input.ViewerRequests) || !intField("app_requests", &input.AppRequests) || !int64Field("rate_window_ms", &input.RateWindowMS) || !intField("concurrency_limit", &input.ConcurrencyLimit) || !intField("monthly_token_limit", &input.MonthlyTokenLimit) {
		return LLMProfileInput{}, false
	}
	revision, ok := parsePositiveOrZero(r.FormValue("expected_revision"))
	if !ok {
		return LLMProfileInput{}, false
	}
	input.ExpectedRevision = revision
	return input, true
}

func (p Platform) dashboard(w http.ResponseWriter, r *http.Request) {
	a, ok := p.actor(w, r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	csrf := p.csrf(w, r, a)
	v := DashboardView{Health: []DashboardHealth{{Name: "host diagnostics", State: "local", Detail: "Run tinkercloud doctor on the VPS for database, disk, DNS, TLS, and email diagnostics."}}}
	unavailable := false
	if p.Views != nil {
		var err error
		v, err = p.Views.Dashboard(r.Context(), a)
		if err != nil {
			v = DashboardView{Health: []DashboardHealth{{Name: "dashboard read model", State: "unavailable", Detail: "Retry or run tinkercloud doctor locally."}}}
			unavailable = true
		}
	}
	notice := ""
	switch r.URL.Query().Get("notice") {
	case "policy_replaced":
		notice = "Access policy replaced. Review the current revision and additional viewers below."
	case "llm_connection_created":
		notice = "Provider connection verified and stored. Its credential remains write-only."
	case "llm_connection_rotated":
		notice = "Provider credential verified and rotated. The replacement remains write-only."
	case "llm_connection_disabled":
		notice = "Provider connection disabled. Future calls using it are denied."
	case "llm_profile_created":
		notice = "LLM profile created with its fixed limits."
	case "llm_profile_updated":
		notice = "LLM profile limits updated."
	case "llm_grant_approved":
		notice = "LLM chat grant approved for the selected app."
	case "llm_grant_disabled":
		notice = "LLM chat grant disabled. Future calls are denied."
	case "llm_grant_revoked":
		notice = "LLM chat grant revoked. Future calls are denied."
	}
	p.render(w, "dashboard.html", dashboardPage{Actor: a, View: v, CSRF: csrf, Notice: notice, Unavailable: unavailable})
}
func (p Platform) actor(w http.ResponseWriter, r *http.Request) (Actor, bool) {
	if p.Auth == nil {
		return Actor{}, false
	}
	a, err := p.Auth.AuthenticatePlatform(r.Context(), w, r)
	return a, err == nil && a.Active
}
func (p Platform) csrf(w http.ResponseWriter, r *http.Request, actor Actor) string {
	subject := actor.IdentitySessionID
	// Test-only authenticators do not carry a browser identity session. A real
	// ControlAuthenticator always supplies one; keep isolated UI rendering
	// tests deterministic without weakening the production path.
	if subject == "" && actor.ID != "" {
		subject = "test-" + actor.ID
	}
	if subject == "" {
		return ""
	}
	prefix := subject + "."
	if c, err := r.Cookie(browseridentity.CSRFCookieName); err == nil && strings.HasPrefix(c.Value, prefix) && len(c.Value) >= len(prefix)+32 {
		return c.Value
	}
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	v := prefix + base64.RawURLEncoding.EncodeToString(b)
	http.SetCookie(w, &http.Cookie{Name: browseridentity.CSRFCookieName, Value: v, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	return v
}
func (p Platform) validCSRF(r *http.Request, actor Actor) bool {
	c, e := r.Cookie(browseridentity.CSRFCookieName)
	subject := actor.IdentitySessionID
	if subject == "" && actor.ID != "" {
		subject = "test-" + actor.ID
	}
	if e != nil || c.Value == "" || subject == "" || !strings.HasPrefix(c.Value, subject+".") {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(c.Value), []byte(r.FormValue("csrf"))) == 1
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

type dashboardPage struct {
	Actor       Actor
	View        DashboardView
	CSRF        string
	Notice      string
	Unavailable bool
}
type tokenPage struct {
	Actor Actor
	Token TokenResult
}
type errorPage struct{ Title, Message, Href, Label string }
type catalogPage struct {
	Apps        []CatalogApp
	Tags        []string
	CSRF        string
	Unavailable bool
}
