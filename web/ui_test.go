package webui

import (
	"crypto/sha256"
	"encoding/base64"
	"html/template"
	"os"
	"strings"
	"testing"
)

func TestEmbeddedDesignSystemIsLocalAccessibleAndTokenized(t *testing.T) {
	css := Stylesheet()
	for _, required := range []string{
		"--tinker-canvas:",
		"--tinker-ink:",
		"--tinker-primary:",
		"--tinker-qr-ink:",
		"--tinker-qr-surface:",
		"--tinker-motion-fast:",
		"--tinker-motion-disclosure:",
		"--tinker-ease-out:",
		".tinker-auth-card",
		".tinker-status",
		".tinker-button--danger",
		".tinker-toast",
		".tinker-dialog",
		".tinker-dialog--compact",
		".tinker-qr",
		".tinker-qr-frame",
		".tinker-disclosure-card",
		".tinker-app-filter[hidden]",
		".tinker-inset",
		".tinker-state--loading",
		".tinker-state--stale",
		".tinker-state--unavailable",
		".tinker-state--error",
		".tinker-progress",
		".tinker-stage-list",
		".tinker-pagination",
		".tinker-button[data-state=\"busy\"]",
		":focus-visible",
		"prefers-reduced-motion",
		"@media (max-width: 560px)",
	} {
		if !strings.Contains(css, required) {
			t.Fatalf("design system missing %q", required)
		}
	}
	for _, forbidden := range []string{"https://", "http://", "@import", "<script", "javascript:"} {
		if strings.Contains(strings.ToLower(css), forbidden) {
			t.Fatalf("stylesheet contains remote or executable content %q", forbidden)
		}
	}
	if strings.Contains(css, "border-left:") {
		t.Fatal("design system must not use AI-style colored side rails")
	}
}

func TestEmbeddedInteractionsStayLocalAndPresentationOnly(t *testing.T) {
	js := strings.ToLower(Interactions())
	for _, required := range []string{
		"tinkerui",
		"showmodal",
		"data-tinker-dialog-open",
		"data-tinker-toast-message",
		"data-tinker-disclosure-state",
		"prefers-reduced-motion",
		".animate(",
		"toggledisclosure",
		"textcontent",
		"data-tinker-app-filter",
		"data-tinker-app-slug",
		"data-tinker-app-description",
		"data-tinker-app-status",
		"data-tinker-catalog-filter",
		"data-tinker-catalog-filter-query",
		"data-tinker-catalog-filter-tag",
		"data-tinker-catalog-filter-count",
		"data-tinker-catalog-list",
		"data-tinker-catalog-card",
		"data-tinker-catalog-slug",
		"data-tinker-catalog-description",
		"data-tinker-catalog-tags",
		"initializecatalogfilters",
		"canvas[data-tinker-qr]",
		"initializeqrcodes",
		"window.atob",
		"var quiet = 4",
		"tolocalelowercase",
		"showing 1 app.",
	} {
		if !strings.Contains(js, required) {
			t.Fatalf("interaction helper missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"http://",
		"https://",
		"fetch(",
		"xmlhttprequest",
		"innerhtml",
		"eval(",
		"localstorage",
		"sessionstorage",
		"document.cookie",
		"window.location",
	} {
		if strings.Contains(js, forbidden) {
			t.Fatalf("interaction helper contains remote, executable, or browser-held state primitive %q", forbidden)
		}
	}
}

func TestEmbeddedAssetsExposeExactCSPHashSources(t *testing.T) {
	for _, asset := range []struct {
		name   string
		body   string
		source string
	}{
		{name: "stylesheet", body: Stylesheet(), source: TinkerStyleCSPSource()},
		{name: "interactions", body: Interactions(), source: TinkerScriptCSPSource()},
	} {
		digest := sha256.Sum256([]byte(asset.body))
		want := "'sha256-" + base64.StdEncoding.EncodeToString(digest[:]) + "'"
		if asset.source != want {
			t.Fatalf("%s CSP source = %q, want %q", asset.name, asset.source, want)
		}
		if strings.Contains(asset.source, "unsafe-inline") || strings.Contains(asset.source, "http") {
			t.Fatalf("%s CSP source is not an exact local hash: %q", asset.name, asset.source)
		}
	}
}

func TestEmbeddedCloudMarkIsStaticAndScriptFree(t *testing.T) {
	mark := strings.ToLower(CloudMark())
	for _, required := range []string{"<svg", "#200675", "#086bfa"} {
		if !strings.Contains(mark, required) {
			t.Fatalf("cloud mark missing %q", required)
		}
	}
	for _, forbidden := range []string{"<script", "javascript:", `href="http`, `src="http`, "onload=", "onclick="} {
		if strings.Contains(mark, forbidden) {
			t.Fatalf("cloud mark contains unsafe content %q", forbidden)
		}
	}
}

func TestFuncMapKeepsUserValuesEscaped(t *testing.T) {
	tpl := template.Must(template.New("page").Funcs(FuncMap()).Parse(
		`<style>{{tinkerCSS}}</style><script>{{tinkerJS}}</script>{{tinkerMark}}<p>{{.}}</p>`,
	))
	var out strings.Builder
	if err := tpl.Execute(&out, `<script>alert("x")</script>`); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), `<script>alert`) || !strings.Contains(out.String(), "&lt;script&gt;") {
		t.Fatalf("untrusted value escaped incorrectly: %s", out.String())
	}
	if !strings.Contains(out.String(), Interactions()) {
		t.Fatal("FuncMap did not embed the trusted interaction helper")
	}
}

func TestShowcaseUsesCanonicalLocalAssetsAndSemanticStates(t *testing.T) {
	raw, err := os.ReadFile("showcase.html")
	if err != nil {
		t.Fatal(err)
	}
	page := strings.ToLower(string(raw))
	for _, required := range []string{
		`href="assets/tinkercloud.css?v=1"`,
		`src="assets/tinkercloud.js?v=1"`,
		`src="assets/tinkercloud-mark.svg"`,
		`class="tinker-skip"`,
		`method="post" action="/logout"`,
		`name="csrf"`,
		`sign out of tinkercloud`,
		`data-state="active"`,
		`class="tinker-dialog"`,
		`data-tinker-dialog-open="showcase-qr-dialog"`,
		`class="tinker-qr"`,
		`scan with your phone`,
		`class="tinker-disclosure-card"`,
		`class="tinker-state tinker-state--loading"`,
		`class="tinker-state tinker-state--stale"`,
		`class="tinker-state tinker-state--unavailable"`,
		`class="tinker-state tinker-state--error"`,
		`class="tinker-progress"`,
		`class="tinker-stage-list"`,
		`class="tinker-pagination"`,
		`aria-invalid="true"`,
		`aria-busy="true"`,
		`aria-current="page"`,
		`data-tinker-toast-region`,
		`role="alert"`,
		`<th scope="col">`,
		`delete:payroll-preview`,
		`data-tinker-app-filter`,
		`data-tinker-app-filter-query`,
		`data-tinker-app-filter-status`,
		`data-tinker-app-filter-count`,
		`data-tinker-app-filter-empty`,
		`data-tinker-catalog-filter`,
		`data-tinker-catalog-filter-query`,
		`data-tinker-catalog-filter-tag`,
		`data-tinker-catalog-filter-count`,
		`data-tinker-catalog-filter-empty`,
		`data-tinker-catalog-list`,
		`data-tinker-catalog-card`,
		`data-tinker-catalog-description`,
		`data-tinker-catalog-tags`,
		`aria-controls="showcase-app-list"`,
		`id="showcase-app-list"`,
		`no matching apps.`,
		`after successful irreversible deletion, this app and its owned data are permanently removed`,
		`it cannot be restored`,
		`active deployer allowlist`,
		`approximate visitors`,
		`last 30 utc days`,
		`local insights unavailable`,
		`removed addresses are signed out`,
		`api keys`,
		`llm chat`,
		`api key management is unavailable`,
		`tinkercloud llm enable`,
		`type="password"`,
		`never displayed or recovered`,
		`disable:0123456789abcdef0123456789abcdef`,
		`only verified, active, and superseded immutable releases provide descriptions`,
		`uploading, uploaded, validating, staged, rejected, and failed candidates remain description-less`,
	} {
		if !strings.Contains(page, required) {
			t.Fatalf("showcase missing %q", required)
		}
	}
	for _, forbidden := range []string{`<script>`, `style=`, `src="http`, `href="http`, `javascript:`, `<option value="deleted">`, `releases and rollback`, `roll back`, `provider credential value=`, `display name`} {
		if strings.Contains(page, forbidden) {
			t.Fatalf("showcase contains remote or executable dependency %q", forbidden)
		}
	}
	start := strings.Index(page, `<article class="tinker-state tinker-state--unavailable tinker-section__spaced" role="status"><h3>api key management is unavailable</h3>`)
	if start < 0 {
		t.Fatal("showcase API-key unavailable state missing")
	}
	if end := strings.Index(page[start:], `</article>`); end < 0 || strings.Contains(page[start:start+end], `<input`) || strings.Contains(page[start:start+end], `<button`) || strings.Contains(page[start:start+end], `<form`) {
		t.Fatal("showcase API-key unavailable state exposes a mutation control")
	}
}

func TestOperationsUsesOnlyCanonicalEmbeddedAssets(t *testing.T) {
	templateSource, err := os.ReadFile("templates/operations.html")
	if err != nil {
		t.Fatal(err)
	}
	page := strings.ToLower(string(templateSource))
	for _, required := range []string{"{{tinkercss}}", "{{tinkermark}}", "tinker-auth-card", "exact target"} {
		if !strings.Contains(page, required) {
			t.Fatalf("operations template missing %q", required)
		}
	}
	for _, forbidden := range []string{"operations.css", "<link", "src=\"http", "href=\"http", "style="} {
		if strings.Contains(page, forbidden) {
			t.Fatalf("operations template has divergent or remote asset %q", forbidden)
		}
	}
}
