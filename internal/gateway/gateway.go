package gateway

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ChrisMarxDev/tinkercloud/internal/analytics"
	"github.com/ChrisMarxDev/tinkercloud/internal/appauth"
	"github.com/ChrisMarxDev/tinkercloud/internal/appnamespace"
	"github.com/ChrisMarxDev/tinkercloud/internal/apps"
	"github.com/ChrisMarxDev/tinkercloud/internal/config"
	"github.com/ChrisMarxDev/tinkercloud/internal/policies"
	"github.com/ChrisMarxDev/tinkercloud/internal/staticruntime"
	webui "github.com/ChrisMarxDev/tinkercloud/web"
)

type HostKind uint8

const (
	Unknown HostKind = iota
	Platform
	App
)

type Host struct {
	Kind HostKind
	Slug string
}

func ClassifyHost(raw string, cfg config.Config) (Host, bool) {
	host, port, err := net.SplitHostPort(raw)
	if err == nil {
		portNumber, portErr := strconv.ParseUint(port, 10, 16)
		if port == "" || portErr != nil || portNumber == 0 {
			return Host{}, false
		}
		raw = host
	} else if strings.Contains(raw, ":") {
		return Host{}, false
	}
	h := strings.TrimSuffix(strings.ToLower(raw), ".")
	if h == "" {
		return Host{}, false
	}
	if h == cfg.PlatformHost() {
		return Host{Kind: Platform}, true
	}
	suffix := "." + cfg.AppSuffix()
	if !strings.HasSuffix(h, suffix) {
		return Host{Kind: Unknown}, true
	}
	slug := strings.TrimSuffix(h, suffix)
	if !appnamespace.Valid(slug) {
		return Host{}, false
	}
	return Host{Kind: App, Slug: slug}, true
}

type Endpoint uint8

const (
	Reserved Endpoint = iota
	AppLogin
	AppLogout
	// AppIdentityCallback consumes an app-bound broker handoff. It is pre-auth
	// because the host-only app session does not exist until the handoff is
	// consumed, but the app itself still comes exclusively from this gateway's
	// validated host resolution.
	AppIdentityCallback
	CurrentUser
	AppInfo
	Capabilities
	KV
	Collections
	Blobs
	LLMChat
	Live
	ProtectedStatic
)

func ClassifyRoute(method, p string) Endpoint {
	if strings.HasPrefix(p, "/_tinker/") {
		switch {
		case method == "GET" && p == "/_tinker/auth/login":
			return AppLogin
		case method == "POST" && p == "/_tinker/auth/logout":
			return AppLogout
		case method == "GET" && p == "/_tinker/auth/callback":
			return AppIdentityCallback
		case method == "GET" && p == "/_tinker/api/v1/me":
			return CurrentUser
		case method == "GET" && p == "/_tinker/api/v1/app":
			return AppInfo
		case method == "GET" && p == "/_tinker/api/v1/capabilities":
			return Capabilities
		case strings.HasPrefix(p, "/_tinker/api/v1/kv"):
			return KV
		case p == "/_tinker/api/v1/db" || strings.HasPrefix(p, "/_tinker/api/v1/db/"):
			return Collections
		case p == "/_tinker/api/v1/blobs" || strings.HasPrefix(p, "/_tinker/api/v1/blobs/"):
			return Blobs
		case method == "POST" && p == "/_tinker/api/v1/llm/chat":
			return LLMChat
		case method == "GET" && p == "/_tinker/ws/v1":
			return Live
		}
		return Reserved
	}
	return ProtectedStatic
}
func Registry() map[Endpoint]string {
	return map[Endpoint]string{Reserved: "deny", AppLogin: "pre-auth", AppLogout: "pre-auth", AppIdentityCallback: "pre-auth", CurrentUser: "protected", AppInfo: "protected", Capabilities: "protected", KV: "protected", Collections: "protected", Blobs: "protected", LLMChat: "protected", Live: "protected", ProtectedStatic: "protected"}
}

// ProtectedDispatcher is the only extension point for protected app surfaces.
// Every method receives the same sealed context produced by Authorizer.
type ProtectedDispatcher interface {
	Dispatch(appauth.AuthorizationContext, Endpoint, http.ResponseWriter, *http.Request)
}
type PreAuthDispatcher interface {
	DispatchPreAuth(apps.App, Endpoint, http.ResponseWriter, *http.Request)
}

type Gateway struct {
	Config      config.Config
	Apps        apps.Repository
	Authorizer  appauth.Authorizer
	Protected   ProtectedDispatcher
	PreAuth     PreAuthDispatcher
	Platform    http.Handler
	Insights    *analytics.Recorder
	InsightsKey []byte
	Clock       func() time.Time
}

func (g Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := requestID()
	securityHeaders(w, id)
	host, ok := ClassifyHost(r.Host, g.Config)
	if !ok || host.Kind == Unknown {
		denyJSON(w, http.StatusNotFound, "not_found", id)
		return
	}
	if host.Kind == Platform {
		if g.Platform == nil {
			denyJSON(w, http.StatusNotFound, "not_found", id)
			return
		}
		g.Platform.ServeHTTP(w, r)
		return
	}
	if g.Apps == nil {
		denyJSON(w, http.StatusNotFound, "not_found", id)
		return
	}
	app, err := g.Apps.ResolveActive(r.Context(), host.Slug)
	if err != nil {
		denyJSON(w, http.StatusNotFound, "not_found", id)
		return
	}
	endpoint := ClassifyRoute(r.Method, r.URL.Path)
	// Public apps never expose a gateway-owned reserved/auth surface to an
	// anonymous caller.  Resolve current policy/gate before any dispatcher.
	if endpoint != ProtectedStatic {
		if g.isEffectivePublic(r.Context(), app) {
			denyJSON(w, http.StatusNotFound, "not_found", id)
			return
		}
	}
	switch endpoint {
	case AppLogin, AppLogout, AppIdentityCallback:
		if g.PreAuth != nil {
			g.PreAuth.DispatchPreAuth(app, ClassifyRoute(r.Method, r.URL.Path), w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted", "request_id": id})
		return
	case Reserved:
		denyJSON(w, http.StatusNotFound, "not_found", id)
		return
	}
	token := ""
	if c, e := r.Cookie(g.Config.SessionCookie); e == nil {
		token = c.Value
	}
	if endpoint == ProtectedStatic {
		staticAuth, err := g.Authorizer.AuthorizeStatic(r.Context(), app, token, id)
		if err == nil {
			observed := &responseObserver{ResponseWriter: w}
			var event analytics.Event
			var record bool
			var callback func()
			if IsDocumentNavigation(r) {
				callback = func() { event, record = g.prepareInsight(staticAuth, observed, r) }
			}
			outcome := staticruntime.Serve(staticAuth, observed, r, callback)
			if outcome.DocumentCandidate && IsDocumentNavigation(r) && observed.status() == http.StatusOK && record {
				_ = g.Insights.Offer(event)
			}
			return
		}
		if IsDocumentNavigation(r) {
			if ret, valid := ValidReturnPath(r.URL.RequestURI()); valid {
				http.Redirect(w, r, "/_tinker/auth/login?return="+url.QueryEscape(ret), http.StatusSeeOther)
				return
			}
		}
		denyJSON(w, http.StatusUnauthorized, "not_authorized", id)
		return
	}
	auth, err := g.Authorizer.Authorize(r.Context(), app, token, id)
	if err != nil {
		if ClassifyRoute(r.Method, r.URL.Path) == ProtectedStatic && IsDocumentNavigation(r) {
			if ret, valid := ValidReturnPath(r.URL.RequestURI()); valid {
				http.Redirect(w, r, "/_tinker/auth/login?return="+url.QueryEscape(ret), http.StatusSeeOther)
				return
			}
		}
		denyJSON(w, http.StatusUnauthorized, "not_authorized", id)
		return
	}
	if g.Protected == nil {
		denyJSON(w, http.StatusNotFound, "not_found", id)
		return
	}
	g.Protected.Dispatch(auth, ClassifyRoute(r.Method, r.URL.Path), w, r)
}

func (g Gateway) isEffectivePublic(ctx context.Context, app apps.App) bool {
	if g.Authorizer.Policies == nil {
		return false
	}
	if app.DeploymentID == "" || app.KVEnabled || app.BlobsEnabled || app.RealtimeEnabled {
		return false
	}
	p, err := g.Authorizer.Policies.Current(ctx, app.ID)
	if err != nil || !p.Valid || p.Mode != "public" {
		return false
	}
	gs, ok := g.Authorizer.Policies.(policies.PublicGateStore)
	if !ok {
		return false
	}
	gate, err := gs.CurrentPublicGate(ctx)
	return err == nil && gate.Valid && gate.Enabled && gate.Revision > 0
}

type responseObserver struct {
	http.ResponseWriter
	code          int
	pendingCookie *http.Cookie
}

func (w *responseObserver) WriteHeader(code int) {
	if w.code == 0 {
		w.code = code
		if code == http.StatusOK && w.pendingCookie != nil {
			http.SetCookie(w.ResponseWriter, w.pendingCookie)
		}
	}
	w.ResponseWriter.WriteHeader(code)
}
func (w *responseObserver) Write(b []byte) (int, error) {
	if w.code == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}
func (w *responseObserver) status() int {
	if w.code == 0 {
		return http.StatusOK
	}
	return w.code
}

// recordInsight runs only after staticruntime has served an authorized 200 HTML
// document. The recorder's offer is non-blocking, and a cookie is an analytics
// header only; a failed random source or recorder can never change the body.
func (g Gateway) prepareInsight(auth appauth.StaticAccessContext, w http.ResponseWriter, r *http.Request) (analytics.Event, bool) {
	if g.Insights == nil || len(g.InsightsKey) == 0 || !g.Insights.Available() {
		return analytics.Event{}, false
	}
	now := time.Now().UTC()
	if g.Clock != nil {
		now = g.Clock().UTC()
	}
	event := analytics.Event{AppID: auth.AppID(), Occurred: now}
	if marker, ok := analytics.ReadMarker(r); ok {
		event.Marker, event.HasMarker = analytics.Digest(g.InsightsKey, auth.AppID(), marker), true
		// Conditional static requests may be turned into a 304/412 by ServeContent
		// after this callback. Never introduce a fresh marker on that path; an
		// existing valid marker can still be counted only after the observed 200.
	} else if !hasConditionalRequest(r) {
		if cookie, err := analytics.NewMarkerCookie(now); err == nil {
			if observed, ok := w.(*responseObserver); ok {
				observed.pendingCookie = cookie
			}
		}
	}
	return event, true
}

func hasConditionalRequest(r *http.Request) bool {
	for _, header := range []string{"If-Match", "If-Modified-Since", "If-None-Match", "If-Unmodified-Since"} {
		if r.Header.Get(header) != "" {
			return true
		}
	}
	return false
}

// IsDocumentNavigation is intentionally narrow. Browser navigation metadata
// plus an HTML Accept value distinguishes a user asking for an app document
// from an SDK/API call, asset load, range request, or WebSocket attempt. Those
// non-document surfaces retain the JSON gateway denial.
func IsDocumentNavigation(r *http.Request) bool {
	if r == nil || r.Method != http.MethodGet || r.URL == nil || !acceptsHTML(r.Header.Get("Accept")) {
		return false
	}
	dest := strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Dest")))
	mode := strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Mode")))
	return dest == "document" || (dest == "" && mode == "navigate")
}

func acceptsHTML(raw string) bool {
	for _, value := range strings.Split(raw, ",") {
		media := strings.TrimSpace(strings.SplitN(value, ";", 2)[0])
		if strings.EqualFold(media, "text/html") {
			return true
		}
	}
	return false
}

// SameOrigin rejects an absent, malformed, cross-host, or non-HTTPS Origin
// before a state-changing app-host form can revoke a session. The gateway has
// already classified the Host, so this comparison never accepts a client
// supplied app identity.
func SameOrigin(r *http.Request) bool {
	if r == nil {
		return false
	}
	u, err := url.Parse(r.Header.Get("Origin"))
	if err != nil || u.Scheme != "https" || u.User != nil || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}
func securityHeaders(w http.ResponseWriter, id string) {
	w.Header().Set("X-Request-ID", id)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "same-origin")
	w.Header().Set("Cache-Control", "no-store")
	// `self` is intentionally the only connect target. In particular, do not add
	// a scheme source such as `wss:` here: every app is untrusted JavaScript and
	// a scheme source would let it exfiltrate ambient app data to any WebSocket
	// host. Browsers permit a same-origin WebSocket under this policy.
	w.Header().Set("Content-Security-Policy", "default-src 'self'; connect-src 'self'; style-src 'self' "+webui.TinkerStyleCSPSource()+"; script-src 'self' "+webui.TinkerScriptCSPSource()+"; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
}
func denyJSON(w http.ResponseWriter, status int, code, id string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": "This request is not authorized.", "request_id": id}})
}
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// ValidReturnPath accepts only same-host relative navigation paths.
func ValidReturnPath(raw string) (string, bool) {
	if raw == "" {
		return "/", true
	}
	u, err := url.Parse(raw)
	if err != nil || u.IsAbs() || u.Host != "" || strings.HasPrefix(raw, "//") || !strings.HasPrefix(u.Path, "/") || strings.ContainsAny(raw, "\\\r\n\x00") || strings.Contains(u.Path, "\\") {
		return "", false
	}
	return u.RequestURI(), true
}
func requestID() string {
	b := make([]byte, 12)
	if _, e := rand.Read(b); e != nil {
		return "req_unavailable"
	}
	return "req_" + hex.EncodeToString(b)
}
