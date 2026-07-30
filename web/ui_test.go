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
		"--tiny-canvas:",
		"--tiny-ink:",
		"--tiny-primary:",
		"--tiny-motion-fast:",
		"--tiny-motion-disclosure:",
		"--tiny-ease-out:",
		".tiny-auth-card",
		".tiny-status",
		".tiny-button--danger",
		".tiny-toast",
		".tiny-dialog",
		".tiny-disclosure-card",
		".tiny-app-filter[hidden]",
		".tiny-inset",
		".tiny-state--loading",
		".tiny-state--stale",
		".tiny-state--unavailable",
		".tiny-state--error",
		".tiny-progress",
		".tiny-stage-list",
		".tiny-pagination",
		".tiny-button[data-state=\"busy\"]",
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
		"tinyui",
		"showmodal",
		"data-tiny-dialog-open",
		"data-tiny-toast-message",
		"data-tiny-disclosure-state",
		"prefers-reduced-motion",
		".animate(",
		"toggledisclosure",
		"textcontent",
		"data-tiny-app-filter",
		"data-tiny-app-slug",
		"data-tiny-app-description",
		"data-tiny-app-status",
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
		{name: "stylesheet", body: Stylesheet(), source: TinyStyleCSPSource()},
		{name: "interactions", body: Interactions(), source: TinyScriptCSPSource()},
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
		`<style>{{tinyCSS}}</style><script>{{tinyJS}}</script>{{tinyMark}}<p>{{.}}</p>`,
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
		`href="assets/tinyhost.css?v=1"`,
		`src="assets/tinyhost.js"`,
		`src="assets/tiny-cloud-mark.svg"`,
		`class="tiny-skip"`,
		`data-state="active"`,
		`class="tiny-dialog"`,
		`class="tiny-disclosure-card"`,
		`class="tiny-state tiny-state--loading"`,
		`class="tiny-state tiny-state--stale"`,
		`class="tiny-state tiny-state--unavailable"`,
		`class="tiny-state tiny-state--error"`,
		`class="tiny-progress"`,
		`class="tiny-stage-list"`,
		`class="tiny-pagination"`,
		`aria-invalid="true"`,
		`aria-busy="true"`,
		`aria-current="page"`,
		`data-tiny-toast-region`,
		`role="alert"`,
		`<th scope="col">`,
		`delete:payroll-preview`,
		`data-tiny-app-filter`,
		`data-tiny-app-filter-query`,
		`data-tiny-app-filter-status`,
		`data-tiny-app-filter-count`,
		`data-tiny-app-filter-empty`,
		`aria-controls="showcase-app-list"`,
		`id="showcase-app-list"`,
		`no matching apps.`,
		`after successful irreversible deletion, this app and its owned data are permanently removed`,
		`it cannot be restored`,
		`active deployer allowlist`,
		`removed addresses are signed out`,
		`write-only llm connection`,
		`type="password"`,
		`never displayed or recovered`,
		`disable:0123456789abcdef0123456789abcdef`,
	} {
		if !strings.Contains(page, required) {
			t.Fatalf("showcase missing %q", required)
		}
	}
	for _, forbidden := range []string{`<script>`, `style=`, `src="http`, `href="http`, `javascript:`, `<option value="deleted">`, `releases and rollback`, `roll back`, `provider credential value=`} {
		if strings.Contains(page, forbidden) {
			t.Fatalf("showcase contains remote or executable dependency %q", forbidden)
		}
	}
}

func TestOperationsUsesOnlyCanonicalEmbeddedAssets(t *testing.T) {
	templateSource, err := os.ReadFile("templates/operations.html")
	if err != nil {
		t.Fatal(err)
	}
	page := strings.ToLower(string(templateSource))
	for _, required := range []string{"{{tinycss}}", "{{tinymark}}", "tiny-auth-card", "exact target"} {
		if !strings.Contains(page, required) {
			t.Fatalf("operations template missing %q", required)
		}
	}
	for _, forbidden := range []string{"operations.css", "<link", "src=\"http", "href=\"http", "style="} {
		if strings.Contains(page, forbidden) {
			t.Fatalf("operations template has divergent or remote asset %q", forbidden)
		}
	}

	legacy, err := os.ReadFile("assets/operations.css")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(legacy), "{") && !strings.HasPrefix(strings.TrimSpace(string(legacy)), "/*") {
		t.Fatal("obsolete operations stylesheet must not contain active rules")
	}
}
