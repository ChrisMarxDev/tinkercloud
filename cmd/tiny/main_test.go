package main

import (
	"bytes"
	"context"
	"github.com/tinyhost/tiny/internal/client"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type tokenRoundTrip func(*http.Request) (*http.Response, error)

func (f tokenRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func tokenClient(transport http.RoundTripper) func(string, string) client.Client {
	return func(base, token string) client.Client {
		c := client.New(base, token)
		c.HTTP = &http.Client{Transport: transport}
		return c
	}
}

func TestAccessSetRejectsInvalidPolicyBeforeNetwork(t *testing.T) {
	p := filepath.Join(t.TempDir(), "policy.json")
	if e := os.WriteFile(p, []byte("{"), 0600); e != nil {
		t.Fatal(e)
	}
	var out, err bytes.Buffer
	code := runWith([]string{"--server", "https://tiny.example", "access", "set", "app", "--file", p}, &out, &err, runnerDeps{store: client.MemoryStore{"https://tiny.example": "secret"}})
	if code != 1 || out.String()+err.String() == "" {
		t.Fatalf("code=%d output=%q", code, out.String()+err.String())
	}
	if bytes.Contains([]byte(out.String()+err.String()), []byte("secret")) {
		t.Fatal("secret leaked")
	}
}

func TestAccessSetFetchesCurrentRevisionBeforeReplacement(t *testing.T) {
	p := filepath.Join(t.TempDir(), "policy.json")
	if err := os.WriteFile(p, []byte(`{"mode":"private","confirm_broadening":true,"allow":{"emails":["viewer@example.test"],"domains":[]}}`), 0600); err != nil {
		t.Fatal(err)
	}
	var putBody string
	deps := runnerDeps{store: client.MemoryStore{"https://tiny.example": "token"}, newClient: tokenClient(tokenRoundTrip(func(r *http.Request) (*http.Response, error) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/apps/app/access":
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"mode":"private","revision":7,"allow":{"emails":[],"domains":[]}}`)), Header: make(http.Header), Request: r}, nil
		case "PUT /api/v1/apps/app/access":
			body, _ := io.ReadAll(r.Body)
			putBody = string(body)
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header), Request: r}, nil
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
			return nil, nil
		}
	}))}
	var out, stderr bytes.Buffer
	if code := runWith([]string{"--server", "https://tiny.example", "access", "set", "app", "--file", p}, &out, &stderr, deps); code != 0 || !strings.Contains(putBody, `"expected_revision":7`) {
		t.Fatalf("code=%d out=%q err=%q body=%q", code, out.String(), stderr.String(), putBody)
	}
}

func TestTokenCreatePrintsSecretExactlyOnce(t *testing.T) {
	var gotBody string
	deps := runnerDeps{
		store: client.MemoryStore{"https://tiny.example": "control-token"},
		newClient: tokenClient(tokenRoundTrip(func(r *http.Request) (*http.Response, error) {
			if r.Method != "POST" || r.URL.Path != "/api/v1/apps/demo/tokens" || r.Header.Get("Authorization") != "Bearer control-token" || r.Header.Get("Idempotency-Key") == "" {
				t.Fatalf("unexpected request: %s %s %#v", r.Method, r.URL, r.Header)
			}
			body, _ := io.ReadAll(r.Body)
			gotBody = string(body)
			return &http.Response{StatusCode: 201, Body: io.NopCloser(strings.NewReader(`{"id":"tok_1","token":"tiny_display_once","scopes":["app:read"],"expires_at":"2030-01-01T00:00:00Z"}`)), Header: make(http.Header), Request: r}, nil
		})),
	}
	var out, err bytes.Buffer
	code := runWith([]string{"--server", "https://tiny.example", "tokens", "create", "demo", "--scope", "app:read", "--expires-in", "3600"}, &out, &err, deps)
	if code != 0 || err.Len() != 0 || out.String() != "tiny_display_once\n" {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), err.String())
	}
	if gotBody != "{\"scopes\":[\"app:read\"],\"expires_in_seconds\":3600}\n" {
		t.Fatalf("unexpected body %q", gotBody)
	}
}

func TestTokenListNeverPrintsSecret(t *testing.T) {
	deps := runnerDeps{
		store: client.MemoryStore{"https://tiny.example": "control-token"},
		newClient: tokenClient(tokenRoundTrip(func(r *http.Request) (*http.Response, error) {
			if r.Method != "GET" || r.URL.Path != "/api/v1/apps/demo/tokens" {
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL)
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`[{"id":"tok_1","scopes":["app:read"],"expires_at":"2030-01-01T00:00:00Z","last_used_at":null,"revoked":false}]`)), Header: make(http.Header), Request: r}, nil
		})),
	}
	var out, err bytes.Buffer
	code := runWith([]string{"--json", "--server", "https://tiny.example", "tokens", "list", "demo"}, &out, &err, deps)
	if code != 0 || err.Len() != 0 || out.String() != "[{\"id\":\"tok_1\",\"scopes\":[\"app:read\"],\"expires_at\":\"2030-01-01T00:00:00Z\",\"last_used_at\":null,\"revoked\":false}]\n" {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), err.String())
	}
	if strings.Contains(out.String(), "tiny_") || strings.Contains(out.String(), "control-token") {
		t.Fatal("credential leaked in token list")
	}
}

func TestTokenRevokeUsesDeleteAndDoesNotExposeCredential(t *testing.T) {
	deps := runnerDeps{
		store: client.MemoryStore{"https://tiny.example": "control-token"},
		newClient: tokenClient(tokenRoundTrip(func(r *http.Request) (*http.Response, error) {
			if r.Method != "DELETE" || r.URL.Path != "/api/v1/apps/demo/tokens/tok_1" || r.Header.Get("Idempotency-Key") == "" {
				t.Fatalf("unexpected request: %s %s %#v", r.Method, r.URL, r.Header)
			}
			return &http.Response{StatusCode: 204, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header), Request: r}, nil
		})),
	}
	var out, err bytes.Buffer
	code := runWith([]string{"--json", "--server", "https://tiny.example", "tokens", "revoke", "demo", "tok_1"}, &out, &err, deps)
	if code != 0 || err.Len() != 0 || out.String() != "{\"id\":\"tok_1\",\"revoked\":true}\n" {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), err.String())
	}
}

func TestTokenCreateRejectsMalformedInputBeforeNetwork(t *testing.T) {
	called := false
	deps := runnerDeps{
		store: client.MemoryStore{"https://tiny.example": "control-token"},
		newClient: tokenClient(tokenRoundTrip(func(*http.Request) (*http.Response, error) {
			called = true
			return nil, context.Canceled
		})),
	}
	var out, err bytes.Buffer
	code := runWith([]string{"--server", "https://tiny.example", "tokens", "create", "demo", "--scope", "app:read", "--expires-in", "1"}, &out, &err, deps)
	if code != 2 || called {
		t.Fatalf("code=%d called=%v out=%q err=%q", code, called, out.String(), err.String())
	}
}

func TestReleasesListUsesInjectedClientAndWritesDeterministicJSON(t *testing.T) {
	deps := runnerDeps{
		store: client.MemoryStore{"https://tiny.example": "control-token"},
		newClient: tokenClient(tokenRoundTrip(func(r *http.Request) (*http.Response, error) {
			if r.Method != "GET" || r.URL.Path != "/api/v1/apps/demo/releases" || r.Header.Get("Authorization") != "Bearer control-token" {
				t.Fatalf("unexpected request: %s %s %#v", r.Method, r.URL, r.Header)
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`[{"id":"dep_1","release_hash":"abc","state":"active","created_at":"2030-01-01T00:00:00Z","verified_at":"2030-01-01T00:01:00Z","activated_at":"2030-01-01T00:02:00Z"}]`)), Header: make(http.Header), Request: r}, nil
		})),
	}
	var out, err bytes.Buffer
	code := runWith([]string{"--json", "--server", "https://tiny.example", "releases", "list", "demo"}, &out, &err, deps)
	want := "[{\"id\":\"dep_1\",\"release_hash\":\"abc\",\"state\":\"active\",\"created_at\":\"2030-01-01T00:00:00Z\",\"verified_at\":\"2030-01-01T00:01:00Z\",\"activated_at\":\"2030-01-01T00:02:00Z\"}]\n"
	if code != 0 || err.Len() != 0 || out.String() != want {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), err.String())
	}
}

func TestReleasesUnauthorizedDoesNotExposeCredentialOrServerBody(t *testing.T) {
	deps := runnerDeps{
		store: client.MemoryStore{"https://tiny.example": "control-token"},
		newClient: tokenClient(tokenRoundTrip(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 403, Body: io.NopCloser(strings.NewReader(`{"id":"other-app","token":"server-secret"}`)), Header: make(http.Header)}, nil
		})),
	}
	var out, err bytes.Buffer
	code := runWith([]string{"--json", "--server", "https://tiny.example", "releases", "demo"}, &out, &err, deps)
	if code != 1 || err.Len() != 0 || out.String() != "{\"valid\":false,\"error\":{\"code\":\"not_authorized\",\"message\":\"Request not authorized.\"}}\n" {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), err.String())
	}
	if strings.Contains(out.String(), "control-token") || strings.Contains(out.String(), "other-app") || strings.Contains(out.String(), "server-secret") {
		t.Fatal("credential or server detail leaked")
	}
}

func TestAppDeleteRequiresExactAppBoundConfirmationBeforeNetwork(t *testing.T) {
	called := false
	deps := runnerDeps{
		store: client.MemoryStore{"https://tiny.example": "control-token"},
		newClient: tokenClient(tokenRoundTrip(func(*http.Request) (*http.Response, error) {
			called = true
			return nil, context.Canceled
		})),
	}
	var out, err bytes.Buffer
	code := runWith([]string{"--server", "https://tiny.example", "apps", "delete", "demo", "--confirm", "delete:other"}, &out, &err, deps)
	if code != 2 || called {
		t.Fatalf("code=%d called=%v out=%q err=%q", code, called, out.String(), err.String())
	}
}

func TestAppDeleteUsesBoundConfirmationAndDoesNotLeakUnauthorizedCredential(t *testing.T) {
	deps := runnerDeps{
		store: client.MemoryStore{"https://tiny.example": "control-token"},
		newClient: tokenClient(tokenRoundTrip(func(r *http.Request) (*http.Response, error) {
			if r.Method != "DELETE" || r.URL.Path != "/api/v1/apps/demo" || r.Header.Get("Idempotency-Key") == "" {
				t.Fatalf("unexpected request: %s %s %#v", r.Method, r.URL, r.Header)
			}
			body, _ := io.ReadAll(r.Body)
			if string(body) != "{\"confirmation\":\"delete:demo\"}\n" {
				t.Fatalf("unexpected body %q", body)
			}
			return &http.Response{StatusCode: 403, Body: io.NopCloser(strings.NewReader(`{"error":"forbidden"}`)), Header: make(http.Header), Request: r}, nil
		})),
	}
	var out, err bytes.Buffer
	code := runWith([]string{"--json", "--server", "https://tiny.example", "apps", "delete", "demo", "--confirm", "delete:demo"}, &out, &err, deps)
	if code != 1 || err.Len() != 0 || out.String() != "{\"valid\":false,\"error\":{\"code\":\"not_authorized\",\"message\":\"Request not authorized.\"}}\n" {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), err.String())
	}
	if strings.Contains(out.String(), "control-token") || strings.Contains(out.String(), "forbidden") {
		t.Fatal("credential or server detail leaked")
	}
}

func TestAppDeleteSuccessWritesStableResult(t *testing.T) {
	deps := runnerDeps{
		store: client.MemoryStore{"https://tiny.example": "control-token"},
		newClient: tokenClient(tokenRoundTrip(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 204, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header), Request: r}, nil
		})),
	}
	var out, err bytes.Buffer
	code := runWith([]string{"--json", "--server", "https://tiny.example", "apps", "delete", "demo", "--confirm", "delete:demo"}, &out, &err, deps)
	if code != 0 || err.Len() != 0 || out.String() != "{\"slug\":\"demo\",\"deleted\":true}\n" {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), err.String())
	}
}

func TestDeployUsesFreshIdempotencyKeyForEachInvocationAndStagesArchive(t *testing.T) {
	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, "dist"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "tiny.yaml"), []byte("version: 1\nname: demo\nbuild:\n  output: dist\naccess:\n  mode: private\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "dist", "index.html"), []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	var keys []string
	deployments := 0
	appCreated := false
	deps := runnerDeps{
		store: client.MemoryStore{"https://tiny.example": "control-token"},
		newClient: tokenClient(tokenRoundTrip(func(r *http.Request) (*http.Response, error) {
			if r.URL.Host == "demo.tiny.example" && r.URL.Path == "/" {
				if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
					t.Fatalf("credential leaked to public probe: %v", r.Header)
				}
				h := make(http.Header)
				h.Set("Content-Type", "application/json; charset=utf-8")
				h.Set("Cache-Control", "no-store")
				h.Set("X-Content-Type-Options", "nosniff")
				h.Set("X-Request-ID", "req_0123456789abcdef01234567")
				return &http.Response{StatusCode: http.StatusUnauthorized, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"not_authorized","message":"This request is not authorized.","request_id":"req_0123456789abcdef01234567"}}`)), Header: h, Request: r}, nil
			}
			if r.Method == "GET" && r.URL.Path == "/api/v1/apps" {
				if appCreated {
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`[{"slug":"demo","status":"active"}]`)), Header: make(http.Header), Request: r}, nil
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`[]`)), Header: make(http.Header), Request: r}, nil
			}
			if r.Method == "POST" && r.URL.Path == "/api/v1/apps" {
				appCreated = true
				return &http.Response{StatusCode: 201, Body: io.NopCloser(strings.NewReader(`{"slug":"demo"}`)), Header: make(http.Header), Request: r}, nil
			}
			if r.URL.Path == "/api/v1/apps/demo/deployments" {
				deployments++
				keys = append(keys, r.Header.Get("Idempotency-Key"))
				body, err := io.ReadAll(r.Body)
				if err != nil || len(body) == 0 {
					t.Fatalf("archive body = %d bytes, %v", len(body), err)
				}
				return &http.Response{StatusCode: 202, Body: io.NopCloser(strings.NewReader(`{"deployment_id":"d` + strconv.Itoa(deployments) + `","state":"verified"}`)), Header: make(http.Header), Request: r}, nil
			}
			if !strings.HasSuffix(r.URL.Path, "/activate") || r.Header.Get("Idempotency-Key") == "" {
				t.Fatalf("unexpected activation request %s", r.URL.Path)
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"deployment_id":"d","url":"https://demo.tiny.example/","app_suffix":"tiny.example","policy_ready":true,"tls_ready":true,"anonymous_denied":true,"authenticated_healthy":true}`)), Header: make(http.Header), Request: r}, nil
		})),
	}
	for range 2 {
		var out, stderr bytes.Buffer
		if code := runWith([]string{"--server", "https://tiny.example", "deploy", project}, &out, &stderr, deps); code != 0 {
			t.Fatalf("deploy code=%d output=%q stderr=%q", code, out.String(), stderr.String())
		}
	}
	if len(keys) != 2 || keys[0] == "" || keys[0] == keys[1] {
		t.Fatalf("deployment idempotency keys = %#v", keys)
	}
}

func TestStagedArchiveIsPrivateAndRemoved(t *testing.T) {
	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, "dist"), 0700); err != nil {
		t.Fatal(err)
	}
	manifest := []byte("version: 1\nname: demo\nbuild:\n  output: dist\naccess:\n  mode: private\n")
	if err := os.WriteFile(filepath.Join(project, "dist", "index.html"), []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	f, cleanup, err := stagedArchive(project, manifest)
	if err != nil {
		t.Fatal(err)
	}
	name := f.Name()
	info, err := os.Stat(name)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("temporary archive mode=%v err=%v", info.Mode(), err)
	}
	cleanup()
	if _, err := os.Stat(name); !os.IsNotExist(err) {
		t.Fatalf("temporary archive remains: %v", err)
	}
}
