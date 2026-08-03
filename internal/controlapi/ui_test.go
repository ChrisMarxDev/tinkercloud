package controlapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ChrisMarxDev/tinkercloud/internal/browseridentity"
)

func TestPlatformUIInsightsRenderAggregateTextOrUnavailableNeverZero(t *testing.T) {
	day := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	available := DashboardInsights{Available: true, Last7Days: DashboardInsightPeriod{PageViews: 4, ApproximateVisitors: 2}, Last30Days: DashboardInsightPeriod{PageViews: 9, ApproximateVisitors: 3, LastActivity: day, Days: []DashboardInsightDay{{Day: day, PageViews: 4, ApproximateVisitors: 2}}}}
	p := Platform{Auth: uiAuth{actor: Actor{ID: "deployer", Email: "deployer@example.test", Role: "deployer", Active: true}}, Views: &uiViews{value: DashboardView{Apps: []DashboardApp{{Slug: "owned-app", Status: "active", Access: DashboardAccess{Mode: "private", Revision: 1}, Insights: available}}}}}
	w := uiRequest(t, p, http.MethodGet, "/dashboard", "")
	for _, required := range []string{"Approximate visitors", "Last 7 days", "Last 30 days", "Last activity", "Last 30 UTC days raw local insights values", "tinker-insight-chart", "tinker-insight-chart__tooltip", `tabindex="0"`, `data-page-level="3"`, `data-visitor-level="2"`, ">2</td>", ">4</td>"} {
		if !strings.Contains(w.Body.String(), required) {
			t.Fatalf("available insight markup missing %q: %s", required, w.Body.String())
		}
	}
	if strings.Contains(w.Body.String(), `class="tinker-grid tinker-grid--2" id="app-list"`) || strings.Contains(w.Body.String(), `<div class="tinker-table-wrap"><table class="tinker-table"><caption>Last 30 UTC days</caption>`) {
		t.Fatalf("dashboard retained the app grid or visible daily table: %s", w.Body.String())
	}
	viewer := Platform{Auth: uiAuth{actor: Actor{ID: "viewer", Role: "viewer", Active: true}}, Views: &uiViews{value: DashboardView{Apps: []DashboardApp{{Slug: "owned-app", Insights: available}}}}}
	w = uiRequest(t, viewer, http.MethodGet, "/dashboard", "")
	if w.Code != http.StatusForbidden || strings.Contains(w.Body.String(), "Approximate visitors") || strings.Contains(w.Body.String(), `aria-labelledby="insights-owned-app"`) {
		t.Fatalf("viewer received owner analytics markup: %d %s", w.Code, w.Body.String())
	}
	p.Views = &uiViews{value: DashboardView{Apps: []DashboardApp{{Slug: "owned-app", Status: "active", Access: DashboardAccess{Mode: "private", Revision: 1}}}}}
	w = uiRequest(t, p, http.MethodGet, "/dashboard", "")
	if !strings.Contains(w.Body.String(), "Local insights unavailable") || strings.Contains(w.Body.String(), "Approximate visitors</p><p class=\"tinker-stat__value\">0") {
		t.Fatalf("unavailable insight was rendered as zero: %s", w.Body.String())
	}
}

func TestPlatformUIAppRowsUseCompactMoreDialogsWithNativeFallback(t *testing.T) {
	day := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	app := DashboardApp{
		Slug:      "owned-app",
		Status:    "active",
		StableURL: "https://owned-app.apps.example.test/",
		Access:    DashboardAccess{Mode: "private", Revision: 1},
		Releases:  []DashboardRelease{{ID: "release-1", State: "active"}},
		Insights: DashboardInsights{
			Available:  true,
			Last7Days:  DashboardInsightPeriod{PageViews: 4, ApproximateVisitors: 2},
			Last30Days: DashboardInsightPeriod{PageViews: 9, ApproximateVisitors: 3, LastActivity: day, Days: []DashboardInsightDay{{Day: day, PageViews: 4, ApproximateVisitors: 2}}},
		},
	}
	operator := Platform{Auth: uiAuth{actor: Actor{ID: "operator", Role: "operator", Active: true}}, Views: &uiViews{value: DashboardView{Apps: []DashboardApp{app}}}}
	body := dashboardAppCardHTML(t, uiRequest(t, operator, http.MethodGet, "/dashboard", "").Body.String(), "owned-app")
	for _, required := range []string{
		`tinker-app-card__identity`,
		`tinker-app-card__title-row`,
		`tinker-app-card__tools`,
		`data-tinker-app-more hidden`,
		`aria-label="More options for owned-app"`,
		`data-tinker-app-more-option`,
		`data-tinker-app-dialog="access-owned-app"`,
		`data-tinker-app-dialog="releases-owned-app"`,
		`data-tinker-app-dialog="llm-owned-app"`,
		`data-tinker-app-dialog="controls-owned-app"`,
		`data-tinker-app-dialog-source="access-owned-app"`,
		`class="tinker-insight-layout"`,
		`class="tinker-insight-layout__summary"`,
		`class="tinker-insight-layout__chart"`,
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("compact operator app row missing %q: %s", required, body)
		}
	}

	deployer := Platform{Auth: uiAuth{actor: Actor{ID: "deployer", Role: "deployer", Active: true}}, Views: &uiViews{value: DashboardView{Apps: []DashboardApp{app}}}}
	body = dashboardAppCardHTML(t, uiRequest(t, deployer, http.MethodGet, "/dashboard", "").Body.String(), "owned-app")
	for _, forbidden := range []string{`data-tinker-app-more`, `data-tinker-app-dialog=`, `data-tinker-app-dialog-source=`} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("deployer app row rendered operator enhancement %q: %s", forbidden, body)
		}
	}
}

func dashboardAppCardHTML(t *testing.T, page, slug string) string {
	t.Helper()
	needle := `data-tinker-app-slug="` + slug + `"`
	attribute := strings.Index(page, needle)
	if attribute < 0 {
		t.Fatalf("dashboard app %q missing from page", slug)
	}
	start := strings.LastIndex(page[:attribute], "<article")
	end := strings.Index(page[attribute:], "</article>")
	if start < 0 || end < 0 {
		t.Fatalf("dashboard app %q has incomplete article markup", slug)
	}
	return page[start : attribute+end+len("</article>")]
}

type uiAuth struct {
	actor Actor
	err   error
}

func (a uiAuth) AuthenticatePlatform(context.Context, http.ResponseWriter, *http.Request) (Actor, error) {
	return a.actor, a.err
}

type uiViewerAuth struct {
	viewer ViewerIdentity
	err    error
}

func (a uiViewerAuth) AuthenticateViewer(context.Context, http.ResponseWriter, *http.Request) (ViewerIdentity, error) {
	return a.viewer, a.err
}

type uiCatalogs struct {
	apps []CatalogApp
	err  error
	got  ViewerIdentity
}

func (c *uiCatalogs) Catalog(_ context.Context, viewer ViewerIdentity) ([]CatalogApp, error) {
	c.got = viewer
	return c.apps, c.err
}

type uiViews struct {
	value DashboardView
	err   error
	got   Actor
}

type uiActions struct {
	calls  []string
	err    error
	access AccessPolicyInput
}

func (a *uiActions) ReplaceActiveDeployers(_ context.Context, actor Actor, emails []string, revision string, confirmed bool, _ string) error {
	a.calls = append(a.calls, "deployers:"+actor.Role+":"+strings.Join(emails, ",")+":"+revision+":"+strconv.FormatBool(confirmed))
	return a.err
}
func (a *uiActions) ReplaceAccess(_ context.Context, _ Actor, slug string, in AccessPolicyInput, _ string) error {
	a.calls = append(a.calls, "access:"+slug)
	a.access = in
	return a.err
}
func (a *uiActions) CreateToken(_ context.Context, _ Actor, slug string, _ TokenInput, _ string) (TokenResult, error) {
	a.calls = append(a.calls, "token:"+slug)
	return TokenResult{ID: "tok_safe", Token: "tinker_only_once", ExpiresAt: "later"}, a.err
}
func (a *uiActions) RevokeToken(_ context.Context, _ Actor, slug, token, _ string) error {
	a.calls = append(a.calls, "revoke:"+slug+":"+token)
	return a.err
}
func (a *uiActions) SetAppStatus(_ context.Context, _ Actor, slug, status, _ string) error {
	a.calls = append(a.calls, "app:"+slug+":"+status)
	return a.err
}
func (a *uiActions) DeleteApp(_ context.Context, _ Actor, slug, _ string) error {
	a.calls = append(a.calls, "delete:"+slug)
	return a.err
}
func (a *uiActions) CreateLLMConnection(_ context.Context, _ Actor, _, _ string) error {
	a.calls = append(a.calls, "llm-create")
	return a.err
}
func (a *uiActions) RotateLLMConnection(_ context.Context, _ Actor, id, _ string) error {
	a.calls = append(a.calls, "llm-rotate:"+id)
	return a.err
}
func (a *uiActions) DisableLLMConnection(_ context.Context, _ Actor, id string) error {
	a.calls = append(a.calls, "llm-disable:"+id)
	return a.err
}
func (a *uiActions) CreateLLMProfile(_ context.Context, _ Actor, _ LLMProfileInput) error {
	a.calls = append(a.calls, "llm-profile-create")
	return a.err
}
func (a *uiActions) UpdateLLMProfile(_ context.Context, _ Actor, id string, _ LLMProfileInput) error {
	a.calls = append(a.calls, "llm-profile-update:"+id)
	return a.err
}
func (a *uiActions) ApproveLLMGrant(_ context.Context, _ Actor, slug, profile string, revision uint64) error {
	a.calls = append(a.calls, "llm-grant-approve:"+slug+":"+profile+":"+strconv.FormatUint(revision, 10))
	return a.err
}
func (a *uiActions) SetLLMGrantStatus(_ context.Context, _ Actor, slug, status string, revision uint64) error {
	a.calls = append(a.calls, "llm-grant-"+status+":"+slug+":"+strconv.FormatUint(revision, 10))
	return a.err
}

func (v *uiViews) Dashboard(_ context.Context, a Actor) (DashboardView, error) {
	v.got = a
	return v.value, v.err
}

func uiRequest(t *testing.T, p Platform, method, path, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, "https://tinker.test"+path, strings.NewReader(body))
	r.Host = "tinker.test"
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range cookies {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	p.ServeHTTP(w, r)
	return w
}

func uiSameOriginRequest(t *testing.T, p Platform, method, path, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, "https://tinker.test"+path, strings.NewReader(body))
	r.Host = "tinker.test"
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", "https://tinker.test")
	for _, c := range cookies {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	p.ServeHTTP(w, r)
	return w
}

func TestPlatformUIAnonymousAndCrossRoleDenials(t *testing.T) {
	p := Platform{Auth: uiAuth{err: errors.New("denied")}}
	if w := uiRequest(t, p, http.MethodGet, "/", ""); w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/login" {
		t.Fatalf("anonymous dashboard: %d %q", w.Code, w.Header().Get("Location"))
	}
	if w := uiRequest(t, p, http.MethodPost, "/logout", "csrf=x"); w.Code != http.StatusNotFound {
		t.Fatalf("anonymous logout: %d", w.Code)
	}
	views := &uiViews{value: DashboardView{ActiveDeployerEmails: []string{"deployer@example.test"}}}
	p = Platform{Auth: uiAuth{actor: Actor{ID: "d", Email: "deployer@example.test", Role: "deployer", Active: true}}, Views: views}
	w := uiRequest(t, p, http.MethodGet, "/", "")
	if w.Code != 200 || strings.Contains(w.Body.String(), "deployer@example.test</td>") || views.got.ID != "d" {
		t.Fatalf("deployer page disclosed operator data: %d %s", w.Code, w.Body.String())
	}
}

func TestPlatformUIOperatorCodingAgentPromptUsesConfiguredHostOnly(t *testing.T) {
	operator := Platform{
		Auth:         uiAuth{actor: Actor{ID: "op", Role: "operator", Active: true}},
		Views:        &uiViews{},
		PlatformHost: "admin.testing.tinkercloud.example",
	}
	body := uiRequest(t, operator, http.MethodGet, "/dashboard?endpoint=https://attacker.example", "").Body.String()
	for _, want := range []string{
		"Get started with a coding agent",
		"https://github.com/ChrisMarxDev/tinkercloud",
		"Tinkercloud endpoint: https://admin.testing.tinkercloud.example",
		"skills/tinkercloud-deployer/SKILL.md",
		"Initially ask me only for the deployer email; the bounded OTP exception below is the only later credential question.",
		"Run the normal Tinker CLI and deployer skill flow.",
		"First check for and reuse the exact server-scoped saved identity for that deployer.",
		"Request at most one CLI OTP only when that saved identity is missing, unauthorized, or expired.",
		"Only a trusted human-supervised coding agent may ask once for the short-lived emailed OTP after endpoint, email, and manifest validation and the same live CLI process reaches `Code:`.",
		"The agent provider may retain the interaction.",
		"Immediately submit it only to that same CLI process.",
		"Do not deliberately restate or copy it into project files, source, argv, command/tool logs, summaries, or final output.",
		"If the CLI OTP fails, stop and report the failure.",
		"Never retry the OTP, switch deployer identity, clear, log out, or delete saved CLI authentication, or create another OTP path.",
		"Never ask for bearer tokens, tokens, secrets, or operator access.",
		`id="operator-get-started-prompt"`,
		"readonly",
		`data-tinker-copy-target="operator-get-started-prompt"`,
		"Select the prompt to copy it manually.",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("operator prompt missing %q: %s", want, body)
		}
	}
	for _, forbidden := range []string{"attacker.example", "name=\"endpoint\"", "tinker_only_once", "Never ask me to paste or share a one-time code in chat.", "Do not request, read, copy, paste, relay, or handle the code in chat."} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("operator prompt rendered unsafe input %q: %s", forbidden, body)
		}
	}

	deployer := Platform{Auth: uiAuth{actor: Actor{ID: "deployer", Role: "deployer", Active: true}}, Views: &uiViews{}, PlatformHost: "admin.testing.tinkercloud.example"}
	if body := uiRequest(t, deployer, http.MethodGet, "/dashboard", "").Body.String(); strings.Contains(body, "Get started with a coding agent") || strings.Contains(body, "operator-get-started-prompt") {
		t.Fatalf("deployer received operator prompt: %s", body)
	}
	for _, malformed := range []string{"https://admin.testing.tinkercloud.example", "admin.testing.tinkercloud.example:443", "admin.testing.tinkercloud.example/path", "admin.testing.tinkercloud.example\nattacker.example", "ADMIN.testing.tinkercloud.example", "platform.testing.tinkercloud.example", "localhost"} {
		p := Platform{Auth: uiAuth{actor: Actor{ID: "op", Role: "operator", Active: true}}, Views: &uiViews{}, PlatformHost: malformed}
		if body := uiRequest(t, p, http.MethodGet, "/dashboard", "").Body.String(); strings.Contains(body, "operator-get-started-prompt") || strings.Contains(body, "Get started with a coding agent") {
			t.Fatalf("malformed platform host %q rendered a prompt: %s", malformed, body)
		}
	}
}

func TestPlatformUICatalogRequiresViewerIdentityAndRendersOnlyCatalogReadModel(t *testing.T) {
	denied := Platform{ViewerAuth: uiViewerAuth{err: errors.New("denied")}, Catalogs: &uiCatalogs{apps: []CatalogApp{{Slug: "secret-app"}}}}
	if w := uiRequest(t, denied, http.MethodGet, "/apps", ""); w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/login" || strings.Contains(w.Body.String(), "secret-app") {
		t.Fatalf("anonymous catalog leaked metadata: %d %q %s", w.Code, w.Header().Get("Location"), w.Body.String())
	}
	catalogs := &uiCatalogs{apps: []CatalogApp{{Slug: "allowed-app", Description: `<script>alert("x")</script>`, Tags: []string{"demo", "team"}, StableURL: "https://allowed-app.apps.example.test/"}}}
	p := Platform{ViewerAuth: uiViewerAuth{viewer: ViewerIdentity{IdentityID: "viewer", Email: "viewer@example.test", IdentitySessionID: "gis"}}, Catalogs: catalogs}
	w := uiRequest(t, p, http.MethodGet, "/apps", "")
	page := w.Body.String()
	if w.Code != http.StatusOK || catalogs.got.Email != "viewer@example.test" || !strings.Contains(page, "allowed-app") || strings.Contains(page, `<script>alert`) || !strings.Contains(page, "&lt;script&gt;") {
		t.Fatalf("catalog page=%d got=%#v body=%s", w.Code, catalogs.got, page)
	}
	for _, required := range []string{"data-tinker-catalog-filter", "data-tinker-catalog-filter-query", "data-tinker-catalog-filter-tag", "data-tinker-catalog-filter-count", "data-tinker-catalog-filter-empty", "data-tinker-catalog-description", "All authorized apps are shown.", `method="post" action="/logout"`, `name="csrf"`, "Sign out of Tinkercloud", "Private"} {
		if !strings.Contains(page, required) {
			t.Fatalf("catalog missing %q", required)
		}
	}
	if strings.Contains(page, "deployer") || strings.Contains(page, "operator") || strings.Contains(page, "dashboard") || strings.Contains(page, ">Public<") {
		t.Fatalf("catalog rendered control-plane role data: %s", page)
	}
}

func TestPlatformUICatalogUnavailableDoesNotSubstituteEmptyCatalog(t *testing.T) {
	p := Platform{ViewerAuth: uiViewerAuth{viewer: ViewerIdentity{IdentityID: "viewer", Email: "viewer@example.test"}}, Catalogs: &uiCatalogs{err: errors.New("unavailable")}}
	w := uiRequest(t, p, http.MethodGet, "/apps", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "The app catalog is unavailable.") || strings.Contains(w.Body.String(), "No apps available yet.") {
		t.Fatalf("catalog unavailable state=%d %s", w.Code, w.Body.String())
	}
}

func TestPlatformUICatalogRendersServerDerivedPublicPostureOnly(t *testing.T) {
	p := Platform{ViewerAuth: uiViewerAuth{viewer: ViewerIdentity{IdentityID: "viewer", Email: "viewer@example.test"}}, Catalogs: &uiCatalogs{apps: []CatalogApp{{Slug: "public-proof", StableURL: "https://public-proof.apps.example.test/", Posture: "public"}}}}
	body := uiRequest(t, p, http.MethodGet, "/apps", "").Body.String()
	if !strings.Contains(body, ">Public<") || strings.Contains(body, "access_rules") || strings.Contains(body, "deployment_id") {
		t.Fatalf("catalog public posture=%s", body)
	}
}

func TestPlatformUIDeployerOverviewRendersOnlyProvidedOwnedAppCards(t *testing.T) {
	views := &uiViews{value: DashboardView{Apps: []DashboardApp{{Slug: "owned-app", Status: "active", Access: DashboardAccess{Mode: "private", Revision: 1}}}}}
	p := Platform{Auth: uiAuth{actor: Actor{ID: "deployer", Email: "deployer@example.test", Role: "deployer", Active: true}}, Views: views}
	w := uiRequest(t, p, http.MethodGet, "/dashboard", "")
	body := w.Body.String()
	for _, required := range []string{"Your apps", "owned-app", `action="/logout"`, "Sign out"} {
		if !strings.Contains(body, required) {
			t.Fatalf("deployer overview missing %q: %s", required, body)
		}
	}
	for _, forbidden := range []string{"Active deployer allowlist", "Recent activity", "LLM chat capability", "LLM chat", "API keys", "Server health", "Access policy", "Create display-once token", "App controls", "Allowed emails"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("deployer overview disclosed operator control %q: %s", forbidden, body)
		}
	}
	if views.got.ID != "deployer" || views.got.Role != "deployer" {
		t.Fatalf("dashboard read model actor = %#v", views.got)
	}
}

func TestPlatformUIDoesNotOwnBrowserLoginOrLogout(t *testing.T) {
	p := Platform{}
	for _, path := range []string{"/login", "/login/verify", "/logout"} {
		w := uiRequest(t, p, http.MethodPost, path, "")
		if w.Code != http.StatusNotFound {
			t.Fatalf("%s status=%d; identity broker must own browser authentication", path, w.Code)
		}
	}
}

func TestPlatformUIOperatorCanReplaceActiveDeployersOnlyWithBrowserGuards(t *testing.T) {
	actions := &uiActions{}
	p := Platform{
		Auth:    uiAuth{actor: Actor{ID: "op", Email: "operator@example.test", Role: "operator", Active: true}},
		Views:   &uiViews{value: DashboardView{ActiveDeployerEmails: []string{"old@example.test"}, ActiveDeployerRevision: "revision"}},
		Actions: actions,
	}
	page := uiRequest(t, p, http.MethodGet, "/dashboard", "")
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `action="/deployers/active"`) || !strings.Contains(page.Body.String(), `name="expected_revision" value="revision"`) || !strings.Contains(page.Body.String(), "removed addresses are signed out") || strings.Contains(page.Body.String(), "Change deployer email") {
		t.Fatalf("operator allowlist form missing: %d %s", page.Code, page.Body.String())
	}
	var csrf *http.Cookie
	for _, c := range page.Result().Cookies() {
		if c.Name == browseridentity.CSRFCookieName {
			csrf = c
		}
	}
	if csrf == nil {
		t.Fatal("dashboard did not issue csrf cookie")
	}
	body := "csrf=" + url.QueryEscape(csrf.Value) + "&expected_revision=revision&emails=old%40example.test%0Anew%40example.test&confirm_broadening=confirm"
	w := uiSameOriginRequest(t, p, http.MethodPost, "/deployers/active", body, csrf)
	if w.Code != http.StatusSeeOther || len(actions.calls) != 1 || actions.calls[0] != "deployers:operator:new@example.test,old@example.test:revision:true" {
		t.Fatalf("operator allowlist save: status=%d calls=%v", w.Code, actions.calls)
	}

	actions.calls = nil
	p.Auth = uiAuth{actor: Actor{ID: "d", Email: "deployer@example.test", Role: "deployer", Active: true}}
	w = uiSameOriginRequest(t, p, http.MethodPost, "/deployers/active", body, csrf)
	if w.Code != http.StatusForbidden || len(actions.calls) != 0 {
		t.Fatalf("deployer changed email: status=%d calls=%v", w.Code, actions.calls)
	}
	p.Auth = uiAuth{actor: Actor{ID: "op", Email: "operator@example.test", Role: "operator", Active: true}}
	w = uiRequest(t, p, http.MethodPost, "/deployers/active", "expected_revision=revision&emails=new%40example.test", csrf)
	if w.Code != http.StatusForbidden || len(actions.calls) != 0 {
		t.Fatalf("missing csrf accepted: status=%d calls=%v", w.Code, actions.calls)
	}
	w = uiSameOriginRequest(t, p, http.MethodPost, "/deployers/active", "csrf="+url.QueryEscape(csrf.Value)+"&expected_revision=revision&emails=not-an-email", csrf)
	if w.Code != http.StatusBadRequest || len(actions.calls) != 0 {
		t.Fatalf("malformed email reached action: status=%d calls=%v", w.Code, actions.calls)
	}
}

func TestPlatformUIActiveDeployerRevisionConflictIsActionable(t *testing.T) {
	actions := &uiActions{err: ErrDeployerRevision}
	p := Platform{Auth: uiAuth{actor: Actor{ID: "op", Role: "operator", Active: true}}, Views: &uiViews{value: DashboardView{ActiveDeployerRevision: "rev"}}, Actions: actions}
	page := uiRequest(t, p, http.MethodGet, "/dashboard", "")
	var csrf *http.Cookie
	for _, c := range page.Result().Cookies() {
		if c.Name == browseridentity.CSRFCookieName {
			csrf = c
		}
	}
	if csrf == nil {
		t.Fatal("missing csrf")
	}
	w := uiSameOriginRequest(t, p, http.MethodPost, "/deployers/active", "csrf="+url.QueryEscape(csrf.Value)+"&expected_revision=rev&emails=&confirm_broadening=confirm", csrf)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "Deployer list changed") || !strings.Contains(w.Body.String(), "Refresh dashboard") || w.Header().Get("Location") != "" {
		t.Fatalf("revision conflict: %d %q", w.Code, w.Body.String())
	}
}

func TestPlatformUIUsesEmbeddedNativeDesignSystem(t *testing.T) {
	p := Platform{}
	w := uiRequest(t, p, http.MethodGet, "/login", "")
	body := w.Body.String()
	for _, want := range []string{
		`class="tinker-auth-card"`,
		`class="tinker-brand__mark"`,
		`--tinker-canvas:`,
		`--tinker-primary:`,
		`TinkerUI`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("login missing design-system contract %q: %s", want, body)
		}
	}
	for _, forbidden := range []string{`src="http`, `href="http`, `fetch(`, `document.cookie`} {
		if strings.Contains(strings.ToLower(body), forbidden) {
			t.Fatalf("login contains remote or executable dependency %q", forbidden)
		}
	}

	views := &uiViews{value: DashboardView{
		Apps: []DashboardApp{{
			Slug:   "alpha",
			Status: "active",
			Access: DashboardAccess{Mode: "private", Revision: 2},
		}},
		Health: []DashboardHealth{{
			Name:   "email",
			State:  "degraded",
			Detail: "Existing sessions continue; new sign-ins are unavailable.",
		}},
	}}
	p = Platform{
		Auth:  uiAuth{actor: Actor{ID: "u", Email: "owner@example.test", Role: "operator", Active: true}},
		Views: views,
	}
	w = uiRequest(t, p, http.MethodGet, "/dashboard", "")
	body = w.Body.String()
	for _, want := range []string{
		`data-state="active"`,
		`data-state="degraded"`,
		`data-tinker-app-filter hidden`,
		`aria-controls="app-list"`,
		`id="app-list"`,
		`data-tinker-app-list`,
		`data-tinker-app-card`,
		`data-tinker-app-slug="alpha"`,
		`data-tinker-app-status="active"`,
		`data-tinker-app-filter-count role="status" aria-live="polite"`,
		`No matching apps.`,
		`<details class="tinker-details" data-tinker-app-dialog-source=`,
		`Replace current policy`,
		`Type <code>delete:alpha</code>`,
		`Your VPS remains the recovery authority`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("dashboard missing design-system contract %q: %s", want, body)
		}
	}
	for _, forbidden := range []string{`fetch(`, `innerHTML`, `localStorage`, `sessionStorage`, `document.cookie`} {
		if strings.Contains(strings.ToLower(body), strings.ToLower(forbidden)) {
			t.Fatalf("dashboard filter contains unsafe browser state or request primitive %q", forbidden)
		}
	}
	if strings.Contains(body, `<option value="deleted">`) {
		t.Fatalf("dashboard exposes deleted apps as an operational filter: %s", body)
	}
	for _, forbidden := range []string{`/rollback`, `Roll back`, `Releases and rollback`} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("dashboard exposes deferred rollback control %q: %s", forbidden, body)
		}
	}
}

func TestPlatformUIDashboardEscapesDescriptionsAndUsesOnlyStableGatewayLink(t *testing.T) {
	p := Platform{Auth: uiAuth{actor: Actor{ID: "u", Role: "operator", Active: true}}, Views: &uiViews{value: DashboardView{Apps: []DashboardApp{{Slug: "alpha", Status: "active", Description: `<script>alert("x")</script>`, StableURL: "https://alpha.apps.example.test/", Access: DashboardAccess{Mode: "private", Revision: 1}, Releases: []DashboardRelease{{ID: "d", State: "active", Description: "Immutable summary"}}}}}}}
	w := uiRequest(t, p, http.MethodGet, "/dashboard", "")
	body := w.Body.String()
	for _, want := range []string{`data-tinker-app-description="&lt;script&gt;alert(&#34;x&#34;)&lt;/script&gt;"`, `href="https://alpha.apps.example.test/"`, `target="_blank"`, `rel="noopener noreferrer"`, `aria-label="Open alpha in a new tab"`, `aria-label="Show QR code for alpha"`, `data-tinker-dialog-open="qr-alpha"`, `class="tinker-qr"`, `Scan with your phone`, `normal sign-in and access policy still apply`, `Immutable summary`} {
		if !strings.Contains(body, want) {
			t.Fatalf("dashboard missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, `<script>alert`) || strings.Contains(body, "release_hash") {
		t.Fatalf("unsafe dashboard description/link: %s", body)
	}
}

func TestPlatformUIDashboardOmitsQRControlWhenStableURLCannotBeEncoded(t *testing.T) {
	tooLong := "https://" + strings.Repeat("a", 272) + "/"
	p := Platform{Auth: uiAuth{actor: Actor{ID: "u", Role: "deployer", Active: true}}, Views: &uiViews{value: DashboardView{Apps: []DashboardApp{{Slug: "alpha", Status: "active", StableURL: tooLong, Access: DashboardAccess{Mode: "private", Revision: 1}}}}}}
	body := uiRequest(t, p, http.MethodGet, "/dashboard", "").Body.String()
	if !strings.Contains(body, `aria-label="Open alpha in a new tab"`) {
		t.Fatalf("dashboard unexpectedly removed the independent stable launch link: %s", body)
	}
	for _, forbidden := range []string{`aria-label="Show QR code for alpha"`, `data-tinker-dialog-open="qr-alpha"`, `class="tinker-qr"`} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("dashboard rendered unsupported QR state %q: %s", forbidden, body)
		}
	}
}

func TestPlatformUIErrorStatesAreStyledAndKeepSafeStatusCodes(t *testing.T) {
	p := Platform{}
	for _, tc := range []struct {
		name, method, path string
		want               int
	}{
		{"not found", http.MethodGet, "/missing", http.StatusNotFound},
		{"method", http.MethodDelete, "/login", http.StatusNotFound},
		{"logout delegated", http.MethodPost, "/logout", http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := uiRequest(t, p, tc.method, tc.path, "")
			body := w.Body.String()
			if w.Code != tc.want || !strings.Contains(body, `class="tinker-auth-card"`) || !strings.Contains(body, "Safe next step") || !strings.Contains(body, `role="alert"`) {
				t.Fatalf("status=%d body=%s", w.Code, body)
			}
		})
	}
}

func TestPlatformUIStalePolicyConflictIsStyledAndDoesNotExposeState(t *testing.T) {
	actions := &uiActions{err: ErrPolicyRevision}
	p := Platform{Auth: uiAuth{actor: Actor{ID: "u", Role: "deployer", Active: true}}, Actions: actions}
	w := uiRequest(t, p, http.MethodGet, "/", "")
	var csrf *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == browseridentity.CSRFCookieName {
			csrf = c
		}
	}
	form := url.Values{"csrf": {csrf.Value}, "expected_revision": {"1"}}
	r := httptest.NewRequest(http.MethodPost, "https://tinker.test/apps/alpha/access", strings.NewReader(form.Encode()))
	r.Host = "tinker.test"
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", "https://tinker.test")
	r.AddCookie(&http.Cookie{Name: browseridentity.IdentityCookieName, Value: "opaque"})
	r.AddCookie(csrf)
	w = httptest.NewRecorder()
	p.ServeHTTP(w, r)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "Policy changed") || !strings.Contains(w.Body.String(), "Refresh dashboard") || strings.Contains(w.Body.String(), "alpha") {
		t.Fatalf("stale policy response: %d %s", w.Code, w.Body.String())
	}
}

func TestPlatformUIRollbackFormRouteIsAbsentEvenWithValidBrowserTrustChecks(t *testing.T) {
	actions := &uiActions{}
	p := Platform{Auth: uiAuth{actor: Actor{ID: "u", Role: "deployer", Active: true}}, Actions: actions}
	w := uiRequest(t, p, http.MethodGet, "/", "")
	var csrf *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == browseridentity.CSRFCookieName {
			csrf = c
		}
	}
	if csrf == nil {
		t.Fatal("missing CSRF cookie")
	}
	form := url.Values{"csrf": {csrf.Value}, "deployment": {"old-release"}}
	r := httptest.NewRequest(http.MethodPost, "https://tinker.test/apps/alpha/rollback", strings.NewReader(form.Encode()))
	r.Host = "tinker.test"
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", "https://tinker.test")
	r.AddCookie(&http.Cookie{Name: browseridentity.IdentityCookieName, Value: "opaque"})
	r.AddCookie(csrf)
	w = httptest.NewRecorder()
	p.ServeHTTP(w, r)
	if w.Code != http.StatusNotFound || len(actions.calls) != 0 {
		t.Fatalf("rollback form route status=%d calls=%v", w.Code, actions.calls)
	}
}

func TestPlatformUICSRFAndSecretRedaction(t *testing.T) {
	views := &uiViews{value: DashboardView{Apps: []DashboardApp{{Slug: "alpha", Tokens: []DashboardToken{{ID: "tok_safe", Scopes: []string{"app:read"}}}}}, Health: []DashboardHealth{{Name: "email", State: "degraded", Detail: "provider unavailable"}}}}
	p := Platform{Auth: uiAuth{actor: Actor{ID: "u", Email: "owner@example.test", Role: "operator", Active: true}}, Views: views}
	w := uiRequest(t, p, http.MethodGet, "/", "")
	var csrf *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == browseridentity.CSRFCookieName {
			csrf = c
		}
	}
	if csrf == nil || !csrf.Secure || !csrf.HttpOnly || strings.Contains(w.Body.String(), "secret_hash") || strings.Contains(w.Body.String(), "tinker_") {
		t.Fatalf("unsafe dashboard: cookies=%#v body=%s", w.Result().Cookies(), w.Body.String())
	}
	control := &http.Cookie{Name: browseridentity.IdentityCookieName, Value: "opaque"}
	if w = uiRequest(t, p, http.MethodPost, "/logout", "csrf="+url.QueryEscape(csrf.Value), control, csrf); w.Code != http.StatusNotFound {
		t.Fatalf("originless csrf accepted: %d", w.Code)
	}
	r := httptest.NewRequest(http.MethodPost, "https://tinker.test/logout", strings.NewReader("csrf="+url.QueryEscape(csrf.Value)))
	r.Host = "tinker.test"
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", "https://evil.test")
	r.AddCookie(control)
	r.AddCookie(csrf)
	w = httptest.NewRecorder()
	p.ServeHTTP(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-origin csrf accepted: %d", w.Code)
	}
	r.Header.Set("Origin", "https://tinker.test")
	w = httptest.NewRecorder()
	p.ServeHTTP(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("dashboard unexpectedly handled global logout: %d", w.Code)
	}
}

func TestPlatformUIDashboardOmitsTokenManagementUntilOverhaul(t *testing.T) {
	views := &uiViews{value: DashboardView{Apps: []DashboardApp{{
		Slug:   "alpha",
		Status: "active",
		Access: DashboardAccess{Mode: "private", Revision: 1},
		Tokens: []DashboardToken{{
			ID:        "tok_safe",
			Scopes:    []string{"app:read", "deploy:create"},
			ExpiresAt: "2026-08-02T00:00:00Z",
		}},
	}}}}
	p := Platform{Auth: uiAuth{actor: Actor{ID: "u", Email: "owner@example.test", Role: "operator", Active: true}}, Views: views}
	body := uiRequest(t, p, http.MethodGet, "/dashboard", "").Body.String()
	for _, forbidden := range []string{
		"<summary>Tokens</summary>",
		"tok_safe",
		"deploy:create",
		"expires_in_seconds",
		"Create display-once token",
		`action="/apps/alpha/tokens"`,
		`action="/apps/alpha/tokens/tok_safe/revoke"`,
		"tokens: 1",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("dashboard rendered deferred token management %q: %s", forbidden, body)
		}
	}
}

func TestPlatformUIMutationsRequireActorOriginCSRFAndConfirmation(t *testing.T) {
	actions := &uiActions{}
	p := Platform{Auth: uiAuth{actor: Actor{ID: "u", Email: "owner@example.test", Role: "deployer", Active: true}}, Actions: actions}
	// Bootstrap the double-submit token through the authenticated page.
	w := uiRequest(t, p, http.MethodGet, "/", "")
	var csrf *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == browseridentity.CSRFCookieName {
			csrf = c
		}
	}
	if csrf == nil {
		t.Fatal("missing csrf")
	}
	control := &http.Cookie{Name: browseridentity.IdentityCookieName, Value: "opaque"}
	request := func(path, form, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "https://tinker.test"+path, strings.NewReader(form+"&csrf="+url.QueryEscape(csrf.Value)))
		r.Host = "tinker.test"
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Origin", origin)
		r.AddCookie(control)
		r.AddCookie(csrf)
		x := httptest.NewRecorder()
		p.ServeHTTP(x, r)
		return x
	}
	if w = request("/apps/alpha/delete", "confirmation=delete%3Awrong", "https://tinker.test"); w.Code != http.StatusBadRequest || len(actions.calls) != 0 {
		t.Fatalf("wrong delete confirmation: %d %#v", w.Code, actions.calls)
	}
	if w = request("/apps/alpha/delete", "confirmation=delete%3Aalpha", "https://evil.test"); w.Code != http.StatusForbidden || len(actions.calls) != 0 {
		t.Fatalf("cross origin mutation: %d %#v", w.Code, actions.calls)
	}
	if w = request("/apps/alpha/delete", "confirmation=delete%3Aalpha", "https://tinker.test"); w.Code != http.StatusSeeOther || len(actions.calls) != 1 || actions.calls[0] != "delete:alpha" {
		t.Fatalf("valid deletion form: %d %#v", w.Code, actions.calls)
	}
	if w = request("/deployers/a%40example.test/suspend", "confirmation=suspend%3Aa%40example.test", "https://tinker.test"); w.Code != http.StatusNotFound || len(actions.calls) != 1 {
		t.Fatalf("deployer escalated: %d %#v", w.Code, actions.calls)
	}
}

func TestPlatformUIOperatorUsesOnlyGlobalDeployerAllowlist(t *testing.T) {
	actions := &uiActions{}
	p := Platform{Auth: uiAuth{actor: Actor{ID: "operator", Email: "root@example.test", Role: "operator", Active: true}}, Actions: actions}
	w := uiRequest(t, p, http.MethodGet, "/dashboard", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `action="/deployers/active"`) || strings.Contains(w.Body.String(), `/deployers/authorize`) {
		t.Fatalf("global editor missing or legacy route present: %d %s", w.Code, w.Body.String())
	}
}

func TestPlatformUITokenIsDisplayedOnlyByCreateResponse(t *testing.T) {
	actions := &uiActions{}
	p := Platform{Auth: uiAuth{actor: Actor{ID: "u", Role: "deployer", Active: true}}, Actions: actions, Views: &uiViews{value: DashboardView{Apps: []DashboardApp{{Slug: "alpha"}}}}}
	w := uiRequest(t, p, http.MethodGet, "/", "")
	var csrf *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == browseridentity.CSRFCookieName {
			csrf = c
		}
	}
	r := httptest.NewRequest(http.MethodPost, "https://tinker.test/apps/alpha/tokens", strings.NewReader("scopes=app%3Aread&expires_in_seconds=3600&csrf="+url.QueryEscape(csrf.Value)))
	r.Host = "tinker.test"
	r.Header.Set("Origin", "https://tinker.test")
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(&http.Cookie{Name: browseridentity.IdentityCookieName, Value: "opaque"})
	r.AddCookie(csrf)
	w = httptest.NewRecorder()
	p.ServeHTTP(w, r)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "tinker_only_once") {
		t.Fatalf("create token response: %d %s", w.Code, w.Body.String())
	}
	w = uiRequest(t, p, http.MethodGet, "/", "")
	if strings.Contains(w.Body.String(), "tinker_only_once") {
		t.Fatal("raw token persisted in dashboard")
	}
}

func TestPlatformUIDashboardShowsAndRoundTripsCurrentAccessPolicy(t *testing.T) {
	access := DashboardAccess{Mode: "private", Revision: 7, Emails: []string{"alice@example.test"}, Domains: []string{"example.test"}}
	actions := &uiActions{}
	p := Platform{Auth: uiAuth{actor: Actor{ID: "u", Role: "operator", Active: true}}, Views: &uiViews{value: DashboardView{Apps: []DashboardApp{{Slug: "alpha", Status: "active", Access: access}}}}, Actions: actions}
	w := uiRequest(t, p, http.MethodGet, "/", "")
	if w.Code != http.StatusOK {
		t.Fatal(w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{"Current posture: private (policy revision 7)", "alice@example.test", "example.test", "You are always an implicit viewer.", "A future deployment can replace this policy", "name=\"mode\" value=\"private\"", "name=\"emails\"", "name=\"domains\""} {
		if !strings.Contains(body, want) {
			t.Fatalf("dashboard missing %q: %s", want, body)
		}
	}
	var csrf *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == browseridentity.CSRFCookieName {
			csrf = c
		}
	}
	if csrf == nil {
		t.Fatal("missing csrf")
	}
	form := url.Values{"emails": {"Alice@Example.test\nbob@example.test"}, "domains": {"Example.test"}, "expected_revision": {"7"}, "confirm_broadening": {"confirm"}, "csrf": {csrf.Value}}
	r := httptest.NewRequest(http.MethodPost, "https://tinker.test/apps/alpha/access", strings.NewReader(form.Encode()))
	r.Host = "tinker.test"
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", "https://tinker.test")
	r.AddCookie(&http.Cookie{Name: browseridentity.IdentityCookieName, Value: "opaque"})
	r.AddCookie(csrf)
	w = httptest.NewRecorder()
	p.ServeHTTP(w, r)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/dashboard?notice=policy_replaced" || len(actions.calls) != 1 || actions.calls[0] != "access:alpha" {
		t.Fatalf("replace access = %d %#v", w.Code, actions.calls)
	}
	if actions.access.Mode != "private" || actions.access.ExpectedRevision != 7 || !actions.access.ConfirmBroadening || strings.Join(actions.access.Allow.Emails, ",") != "Alice@example.test,bob@example.test" || strings.Join(actions.access.Allow.Domains, ",") != "example.test" {
		t.Fatalf("access input = %#v", actions.access)
	}
}

func TestPlatformUIDashboardLabelsEmptyAccessAsOwnerOnly(t *testing.T) {
	p := Platform{Auth: uiAuth{actor: Actor{ID: "u", Role: "operator", Active: true}}, Views: &uiViews{value: DashboardView{Apps: []DashboardApp{{Slug: "alpha", Access: DashboardAccess{Mode: "private", Revision: 1}}}}}}
	w := uiRequest(t, p, http.MethodGet, "/", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "This app is owner-only: no additional email or domain viewers are allowed.") {
		t.Fatalf("owner-only dashboard = %d %s", w.Code, w.Body.String())
	}
}

func TestPlatformUIDashboardShowsPublicPostureAndGateWithoutBrowserMutation(t *testing.T) {
	p := Platform{Auth: uiAuth{actor: Actor{ID: "op", Role: "operator", Active: true}}, Views: &uiViews{value: DashboardView{PublicGate: DashboardPublicGate{Available: true, Enabled: true, Revision: 4}, Apps: []DashboardApp{{Slug: "alpha", Access: DashboardAccess{Mode: "public", Revision: 2, Indexing: false}}}}}}
	body := uiRequest(t, p, http.MethodGet, "/dashboard", "").Body.String()
	for _, want := range []string{"Current posture: public", "indexing disabled", "Public static gate: enabled", "tinkercloud public enable", `name="mode" value="public"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("dashboard missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, "public/enable") || strings.Contains(body, "public/disable") {
		t.Fatalf("dashboard exposed public gate mutation: %s", body)
	}
}

func TestPlatformUIDashboardUnavailablePolicyIsNotEditableEmptyState(t *testing.T) {
	p := Platform{Auth: uiAuth{actor: Actor{ID: "u", Role: "deployer", Active: true}}, Views: &uiViews{err: errors.New("policy unavailable")}}
	w := uiRequest(t, p, http.MethodGet, "/", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Your app overview is unavailable.") || strings.Contains(w.Body.String(), "Replace current policy") {
		t.Fatalf("unavailable dashboard = %d %s", w.Code, w.Body.String())
	}
}

func TestPlatformUIAccessRejectsMissingOrMalformedExpectedRevision(t *testing.T) {
	actions := &uiActions{}
	p := Platform{Auth: uiAuth{actor: Actor{ID: "u", Role: "deployer", Active: true}}, Actions: actions}
	w := uiRequest(t, p, http.MethodGet, "/", "")
	var csrf *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == browseridentity.CSRFCookieName {
			csrf = c
		}
	}
	for _, revision := range []string{"", "1st", "0"} {
		form := url.Values{"csrf": {csrf.Value}, "expected_revision": {revision}}
		r := httptest.NewRequest(http.MethodPost, "https://tinker.test/apps/alpha/access", strings.NewReader(form.Encode()))
		r.Host = "tinker.test"
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Origin", "https://tinker.test")
		r.AddCookie(&http.Cookie{Name: browseridentity.IdentityCookieName, Value: "opaque"})
		r.AddCookie(csrf)
		w = httptest.NewRecorder()
		p.ServeHTTP(w, r)
		if w.Code != http.StatusBadRequest || len(actions.calls) != 0 {
			t.Fatalf("revision %q = %d %#v", revision, w.Code, actions.calls)
		}
	}
}

func TestPlatformUILLMOperatorFormsStayWriteOnlyAndUseServerTargets(t *testing.T) {
	connectionID := "0123456789abcdef0123456789abcdef"
	profileID := "fedcba9876543210fedcba9876543210"
	actions := &uiActions{}
	p := Platform{Auth: uiAuth{actor: Actor{ID: "op", Role: "operator", Active: true}}, Actions: actions, Views: &uiViews{value: DashboardView{
		Apps:                  []DashboardApp{{Slug: "alpha", LLMGrant: &LLMGrant{AppSlug: "alpha", ProfileID: profileID, Status: "approved", Revision: 7}}},
		LLMKeyManagementReady: true,
		LLMConnections:        []LLMConnection{{ID: connectionID, DisplayName: "Team provider", Provider: "anthropic", Status: "active"}},
		LLMProfiles:           []LLMProfile{{ID: profileID, ConnectionID: connectionID, Model: "model", Status: "active", Revision: 3, MaxMessages: 2, MaxMessageBytes: 10, MaxInputBytes: 20, MaxOutputTokens: 4, TimeoutMS: 1000, ViewerRequests: 1, AppRequests: 1, RateWindowMS: 1000, ConcurrencyLimit: 1, MonthlyTokenLimit: 10}},
	}}}
	page := uiRequest(t, p, http.MethodGet, "/dashboard", "")
	if page.Code != http.StatusOK || strings.Contains(page.Body.String(), "super-secret") || !strings.Contains(page.Body.String(), "API keys") || !strings.Contains(page.Body.String(), "LLM chat") || strings.Contains(page.Body.String(), `name="display_name"`) || !strings.Contains(page.Body.String(), `action="/dashboard/llm/connections/`+connectionID+`/rotate"`) || !strings.Contains(page.Body.String(), `action="/apps/alpha/llm/grant/revoke"`) {
		t.Fatalf("llm UI missing/write-only: %d %s", page.Code, page.Body.String())
	}
	var csrf *http.Cookie
	for _, c := range page.Result().Cookies() {
		if c.Name == browseridentity.CSRFCookieName {
			csrf = c
		}
	}
	if csrf == nil {
		t.Fatal("csrf missing")
	}
	// The create form accepts only the fixed provider and one write-only key;
	// browser-chosen names and opaque IDs are refused rather than ignored.
	w := uiSameOriginRequest(t, p, http.MethodPost, "/dashboard/llm/connections", url.Values{"csrf": {csrf.Value}, "provider": {"anthropic"}, "secret": {"new-secret"}, "display_name": {"browser name"}}.Encode(), csrf)
	if w.Code != http.StatusBadRequest || len(actions.calls) != 0 {
		t.Fatalf("browser display name = %d %#v", w.Code, actions.calls)
	}
	w = uiSameOriginRequest(t, p, http.MethodPost, "/dashboard/llm/connections", url.Values{"csrf": {csrf.Value}, "provider": {"anthropic"}, "secret": {"new-secret"}, "id": {connectionID}}.Encode(), csrf)
	if w.Code != http.StatusBadRequest || len(actions.calls) != 0 {
		t.Fatalf("browser connection id = %d %#v", w.Code, actions.calls)
	}
	// Rotation has no provider parameter. A submitted provider field is denied
	// rather than trusted to validate a key for the wrong stored connection.
	w = uiSameOriginRequest(t, p, http.MethodPost, "/dashboard/llm/connections/"+connectionID+"/rotate", url.Values{"csrf": {csrf.Value}, "secret": {"new-secret"}, "provider": {"gemini"}}.Encode(), csrf)
	if w.Code != http.StatusBadRequest || len(actions.calls) != 0 {
		t.Fatalf("provider-selected rotation = %d %#v", w.Code, actions.calls)
	}
	w = uiSameOriginRequest(t, p, http.MethodPost, "/dashboard/llm/connections/"+connectionID+"/rotate", url.Values{"csrf": {csrf.Value}, "secret": {"new-secret"}}.Encode(), csrf)
	if w.Code != http.StatusSeeOther || strings.Join(actions.calls, ",") != "llm-rotate:"+connectionID {
		t.Fatalf("safe rotation = %d %#v", w.Code, actions.calls)
	}
	w = uiSameOriginRequest(t, p, http.MethodPost, "/dashboard/llm/connections/"+connectionID+"/disable", url.Values{"csrf": {csrf.Value}, "confirmation": {"disable:wrong"}}.Encode(), csrf)
	if w.Code != http.StatusBadRequest || len(actions.calls) != 1 {
		t.Fatalf("disable confirmation = %d %#v", w.Code, actions.calls)
	}
	w = uiSameOriginRequest(t, p, http.MethodPost, "/apps/alpha/llm/grant/revoke", url.Values{"csrf": {csrf.Value}, "expected_revision": {"7"}, "confirmation": {"revoked:grant:alpha"}}.Encode(), csrf)
	if w.Code != http.StatusSeeOther || strings.Join(actions.calls, ",") != "llm-rotate:"+connectionID+",llm-grant-revoked:alpha:7" {
		t.Fatalf("grant revoke = %d %#v", w.Code, actions.calls)
	}
}

func TestPlatformUIAPIKeysUnavailableHasNoMutationForm(t *testing.T) {
	connectionID := "0123456789abcdef0123456789abcdef"
	p := Platform{Auth: uiAuth{actor: Actor{ID: "op", Role: "operator", Active: true}}, Views: &uiViews{value: DashboardView{
		LLMConnections: []LLMConnection{{ID: connectionID, DisplayName: "Legacy provider", Provider: "anthropic", Status: "active"}},
	}}}
	w := uiRequest(t, p, http.MethodGet, "/dashboard", "")
	body := w.Body.String()
	if w.Code != http.StatusOK || !strings.Contains(body, "API key management is unavailable") || !strings.Contains(body, "tinkercloud llm enable") || strings.Contains(body, `action="/dashboard/llm/connections"`) {
		t.Fatalf("unavailable api keys = %d %s", w.Code, body)
	}
	if !strings.Contains(body, "Legacy provider") {
		t.Fatalf("existing safe connection metadata missing: %s", body)
	}
}

func TestPlatformUILLMProfileRejectsBrowserChosenIDsAndMalformedBounds(t *testing.T) {
	actions := &uiActions{}
	p := Platform{Auth: uiAuth{actor: Actor{ID: "op", Role: "operator", Active: true}}, Actions: actions}
	page := uiRequest(t, p, http.MethodGet, "/dashboard", "")
	var csrf *http.Cookie
	for _, c := range page.Result().Cookies() {
		if c.Name == browseridentity.CSRFCookieName {
			csrf = c
		}
	}
	if csrf == nil {
		t.Fatal("csrf missing")
	}
	form := url.Values{"csrf": {csrf.Value}, "expected_revision": {"0"}, "connection_id": {"browser-chosen"}, "model": {"model"}, "max_messages": {"2"}, "max_message_bytes": {"10"}, "max_input_bytes": {"20"}, "max_output_tokens": {"4"}, "timeout_ms": {"1000"}, "viewer_requests": {"1"}, "app_requests": {"1"}, "rate_window_ms": {"1000"}, "concurrency_limit": {"1"}, "monthly_token_limit": {"10"}}
	w := uiSameOriginRequest(t, p, http.MethodPost, "/dashboard/llm/profiles", form.Encode(), csrf)
	if w.Code != http.StatusBadRequest || len(actions.calls) != 0 {
		t.Fatalf("untrusted connection id = %d %#v", w.Code, actions.calls)
	}
}

func TestPlatformUILLMSuccessNoticesAreSafeAndCredentialFree(t *testing.T) {
	p := Platform{Auth: uiAuth{actor: Actor{ID: "op", Role: "operator", Active: true}}, Views: &uiViews{}}
	w := uiRequest(t, p, http.MethodGet, "/dashboard?notice=llm_connection_rotated", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "credential verified and rotated") || strings.Contains(w.Body.String(), "secret-value") {
		t.Fatalf("llm notice = %d %s", w.Code, w.Body.String())
	}
}
