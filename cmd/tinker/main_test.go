package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"github.com/ChrisMarxDev/tinkercloud/internal/client"
	"github.com/ChrisMarxDev/tinkercloud/internal/releases"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

type tokenRoundTrip func(*http.Request) (*http.Response, error)

func (f tokenRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type loginPrompt struct {
	values    []string
	questions []string
	asked     int
}

type spyStore struct {
	values          map[string]string
	defaultURL      string
	getErr          error
	defaultErr      error
	setErr          error
	putErr          error
	deleteErr       error
	defaultReads    int
	setDefaultCalls int
	putCalls        int
	deleteCalls     int
}

func (s *spyStore) Get(server string) (string, error) {
	if s.getErr != nil {
		return "", s.getErr
	}
	v, ok := s.values[server]
	if !ok {
		return "", client.ErrCredentialNotFound
	}
	return v, nil
}
func (s *spyStore) Put(server, token string) error {
	s.putCalls++
	if s.putErr != nil {
		return s.putErr
	}
	s.values[server] = token
	return nil
}
func (s *spyStore) Delete(server string) error {
	s.deleteCalls++
	if s.deleteErr != nil {
		return s.deleteErr
	}
	delete(s.values, server)
	return nil
}
func (s *spyStore) DefaultServer() (string, error) {
	s.defaultReads++
	if s.defaultErr != nil {
		return "", s.defaultErr
	}
	if s.defaultURL == "" {
		return "", client.ErrNoDefaultServer
	}
	return s.defaultURL, nil
}
func (s *spyStore) SetDefaultServer(server string) error {
	s.setDefaultCalls++
	if s.setErr != nil {
		return s.setErr
	}
	s.defaultURL = server
	return nil
}

func (p *loginPrompt) Ask(question string) (string, error) {
	p.asked++
	p.questions = append(p.questions, question)
	if len(p.values) == 0 {
		return "", io.EOF
	}
	value := p.values[0]
	p.values = p.values[1:]
	return value, nil
}

func tokenClient(transport http.RoundTripper) func(string, string) client.Client {
	return func(base, token string) client.Client {
		c := client.New(base, token)
		c.HTTP = &http.Client{Transport: transport}
		return c
	}
}

func jsonResponse(r *http.Request, status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: r}
}

func successfulDeployTransport(t *testing.T, bearer string, archiveNames *[]string, calls *[]string) tokenRoundTrip {
	t.Helper()
	created := false
	return tokenRoundTrip(func(r *http.Request) (*http.Response, error) {
		*calls = append(*calls, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/version":
			return jsonResponse(r, http.StatusOK, `{"api_version":1}`), nil
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/whoami":
			if r.Header.Get("Authorization") != "Bearer "+bearer {
				t.Fatalf("whoami bearer = %q", r.Header.Get("Authorization"))
			}
			return jsonResponse(r, http.StatusOK, `{"email":"dev@example.test"}`), nil
		case r.URL.Host == "demo.tinker.example" && r.URL.Path == "/":
			h := make(http.Header)
			h.Set("Content-Type", "application/json; charset=utf-8")
			h.Set("Cache-Control", "no-store")
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Request-ID", "req_0123456789abcdef01234567")
			return &http.Response{StatusCode: http.StatusUnauthorized, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"not_authorized","message":"This request is not authorized.","request_id":"req_0123456789abcdef01234567"}}`)), Header: h, Request: r}, nil
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps":
			if created {
				return jsonResponse(r, http.StatusOK, `[{"slug":"demo","status":"active"}]`), nil
			}
			return jsonResponse(r, http.StatusOK, `[]`), nil
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/apps":
			created = true
			return jsonResponse(r, http.StatusCreated, `{"slug":"demo"}`), nil
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps/demo/access":
			return jsonResponse(r, http.StatusOK, `{"mode":"private","revision":1,"allow":{"emails":[],"domains":[]}}`), nil
		case r.URL.Path == "/api/v1/apps/demo/deployments":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			*archiveNames = tarNames(t, body)
			return jsonResponse(r, http.StatusAccepted, `{"deployment_id":"deployment","state":"verified"}`), nil
		case r.URL.Path == "/api/v1/apps/demo/deployments/deployment/activate":
			return jsonResponse(r, http.StatusOK, `{"deployment_id":"deployment","url":"https://demo.tinker.example/","domain":"tinker.example","policy_ready":true,"tls_ready":true,"anonymous_denied":true,"authenticated_healthy":true}`), nil
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL)
			return nil, nil
		}
	})
}

func tarNames(t *testing.T, body []byte) []string {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var names []string
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, h.Name)
	}
	sort.Strings(names)
	return names
}

func TestLoginReusesValidStoredCredentialWithoutPromptOrOTP(t *testing.T) {
	stored := &spyStore{values: map[string]string{"https://tinker.example": "saved-token"}}
	prompt := &loginPrompt{}
	var paths []string
	deps := runnerDeps{store: stored, prompt: prompt, newClient: tokenClient(tokenRoundTrip(func(r *http.Request) (*http.Response, error) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/api/v1/version":
			return jsonResponse(r, http.StatusOK, `{"api_version":1}`), nil
		case "/api/v1/whoami":
			if r.Header.Get("Authorization") != "Bearer saved-token" {
				t.Fatalf("unexpected bearer %q", r.Header.Get("Authorization"))
			}
			return jsonResponse(r, http.StatusOK, `{"email":"dev@example.test"}`), nil
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
			return nil, nil
		}
	}))}
	var out, stderr bytes.Buffer
	if code := runWith([]string{"--server", "https://tinker.example", "login"}, &out, &stderr, deps); code != 0 || out.String() != "Logged in as dev@example.test.\n" || stderr.Len() != 0 {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), stderr.String())
	}
	if prompt.asked != 0 || strings.Join(paths, ",") != "GET /api/v1/version,GET /api/v1/whoami" || stored.values["https://tinker.example"] != "saved-token" || stored.putCalls != 0 || stored.defaultURL != "https://tinker.example" {
		t.Fatalf("asked=%d paths=%v store=%q puts=%d default=%q", prompt.asked, paths, stored.values["https://tinker.example"], stored.putCalls, stored.defaultURL)
	}
}

func TestDataKVListReusesSavedCredentialAndWritesDeterministicJSON(t *testing.T) {
	stored := &spyStore{values: map[string]string{"https://tinker.example": "saved-token"}, defaultURL: "https://tinker.example"}
	deps := runnerDeps{store: stored, newClient: tokenClient(tokenRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/apps/demo/data/kv" || r.URL.Query().Get("prefix") != "settings/" || r.Header.Get("Authorization") != "Bearer saved-token" {
			t.Fatalf("request = %s %s authorization=%q", r.Method, r.URL, r.Header.Get("Authorization"))
		}
		return jsonResponse(r, http.StatusOK, `{"entries":[{"key":"settings/theme","value":"dark","version":2}],"next_cursor":"settings/theme"}`), nil
	}))}
	var out, stderr bytes.Buffer
	if code := runWith([]string{"--json", "data", "kv", "list", "demo", "--prefix", "settings/"}, &out, &stderr, deps); code != 0 || out.String() != "{\"entries\":[{\"key\":\"settings/theme\",\"value\":\"dark\",\"version\":2,\"updated_at\":\"\"}],\"next_cursor\":\"settings/theme\"}\n" || stderr.Len() != 0 {
		t.Fatalf("code=%d out=%q stderr=%q", code, out.String(), stderr.String())
	}
}

func TestDataDeleteRequiresAppBoundConfirmationBeforeNetwork(t *testing.T) {
	called := false
	deps := runnerDeps{store: &spyStore{values: map[string]string{"https://tinker.example": "saved-token"}, defaultURL: "https://tinker.example"}, newClient: tokenClient(tokenRoundTrip(func(r *http.Request) (*http.Response, error) {
		called = true
		return jsonResponse(r, http.StatusNoContent, ``), nil
	}))}
	var out, stderr bytes.Buffer
	if code := runWith([]string{"data", "kv", "delete", "demo", "settings/theme", "--expected-version", "2", "--confirm", "delete:other:settings/theme"}, &out, &stderr, deps); code != 2 || called || stderr.String() != dataUsage+"\n" {
		t.Fatalf("code=%d called=%v out=%q stderr=%q", code, called, out.String(), stderr.String())
	}
}

func TestDataDeletePromptsForExactConfirmationAndSendsExpectedVersion(t *testing.T) {
	called := false
	deps := runnerDeps{store: &spyStore{values: map[string]string{"https://tinker.example": "saved-token"}, defaultURL: "https://tinker.example"}, prompt: &loginPrompt{values: []string{"delete:demo:settings/theme"}}, newClient: tokenClient(tokenRoundTrip(func(r *http.Request) (*http.Response, error) {
		called = true
		body, _ := io.ReadAll(r.Body)
		if r.Method != http.MethodDelete || r.URL.Path != "/api/v1/apps/demo/data/kv/settings/theme" || !strings.Contains(string(body), `"expected_version":2`) || r.Header.Get("Idempotency-Key") == "" {
			t.Fatalf("request=%s %s body=%s", r.Method, r.URL, body)
		}
		return jsonResponse(r, http.StatusNoContent, ``), nil
	}))}
	var out, stderr bytes.Buffer
	if code := runWith([]string{"data", "kv", "delete", "demo", "settings/theme", "--expected-version", "2"}, &out, &stderr, deps); code != 0 || !called || !strings.Contains(out.String(), `"deleted":true`) {
		t.Fatalf("code=%d called=%v out=%q stderr=%q", code, called, out.String(), stderr.String())
	}
}

func TestDataDeleteJSONRequiresAndAcceptsTargetBoundConfirmation(t *testing.T) {
	called := false
	deps := runnerDeps{store: &spyStore{values: map[string]string{"https://tinker.example": "saved-token"}, defaultURL: "https://tinker.example"}, newClient: tokenClient(tokenRoundTrip(func(r *http.Request) (*http.Response, error) {
		called = true
		body, _ := io.ReadAll(r.Body)
		if r.Method != http.MethodDelete || r.URL.Path != "/api/v1/apps/demo/data/kv/settings/theme" || !strings.Contains(string(body), `"expected_version":2`) {
			t.Fatalf("request=%s %s body=%s", r.Method, r.URL, body)
		}
		return jsonResponse(r, http.StatusOK, `{"deleted":true}`), nil
	}))}
	var out, stderr bytes.Buffer
	args := []string{"--json", "data", "kv", "delete", "demo", "settings/theme", "--expected-version", "2", "--confirm", "delete:demo:settings/theme"}
	if code := runWith(args, &out, &stderr, deps); code != 0 || !called || out.String() != "{\"key\":\"settings/theme\",\"deleted\":true}\n" || stderr.Len() != 0 {
		t.Fatalf("code=%d called=%v out=%q stderr=%q", code, called, out.String(), stderr.String())
	}
}

func TestDataMutationRejectsInlineJSONBeforeNetwork(t *testing.T) {
	called := false
	deps := runnerDeps{store: &spyStore{values: map[string]string{"https://tinker.example": "saved-token"}, defaultURL: "https://tinker.example"}, newClient: tokenClient(tokenRoundTrip(func(r *http.Request) (*http.Response, error) {
		called = true
		return jsonResponse(r, http.StatusInternalServerError, `{}`), nil
	}))}
	var out, stderr bytes.Buffer
	if code := runWith([]string{"data", "kv", "set", "demo", "settings/theme", "--value", `"dark"`}, &out, &stderr, deps); code != 1 || called || !strings.Contains(stderr.String(), "JSON input") {
		t.Fatalf("code=%d called=%v out=%q stderr=%q", code, called, out.String(), stderr.String())
	}
}

func TestLoginUnauthorizedStoredCredentialFallsBackAndReplacesOnlyAfterWhoami(t *testing.T) {
	stored := client.MemoryStore{"https://tinker.example": "old-token"}
	prompt := &loginPrompt{values: []string{"new@example.test", "123456"}}
	whoamiCalls := 0
	deps := runnerDeps{store: stored, prompt: prompt, newClient: tokenClient(tokenRoundTrip(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/v1/version":
			return jsonResponse(r, http.StatusOK, `{"api_version":1}`), nil
		case "/api/v1/whoami":
			whoamiCalls++
			if whoamiCalls == 1 {
				if r.Header.Get("Authorization") != "Bearer old-token" {
					t.Fatalf("first bearer = %q", r.Header.Get("Authorization"))
				}
				return jsonResponse(r, http.StatusUnauthorized, `{"error":"expired"}`), nil
			}
			if r.Header.Get("Authorization") != "Bearer new-token" {
				t.Fatalf("second bearer = %q", r.Header.Get("Authorization"))
			}
			return jsonResponse(r, http.StatusOK, `{"email":"actual@example.test"}`), nil
		case "/api/v1/auth/otp":
			return jsonResponse(r, http.StatusAccepted, `{"transaction":"otp_1"}`), nil
		case "/api/v1/auth/verify":
			return jsonResponse(r, http.StatusOK, `{"token":"new-token"}`), nil
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
			return nil, nil
		}
	}))}
	var out, stderr bytes.Buffer
	if code := runWith([]string{"--server", "https://tinker.example", "login"}, &out, &stderr, deps); code != 0 || out.String() != "Logged in as actual@example.test.\n" || stderr.Len() != 0 {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), stderr.String())
	}
	if prompt.asked != 2 || whoamiCalls != 2 || stored["https://tinker.example"] != "new-token" || strings.Contains(out.String(), "new-token") {
		t.Fatalf("asked=%d whoami=%d store=%q out=%q", prompt.asked, whoamiCalls, stored["https://tinker.example"], out.String())
	}
}

func TestLoginDependencyFailureDoesNotBypassSavedCredentialToOTP(t *testing.T) {
	stored := client.MemoryStore{"https://tinker.example": "saved-token"}
	prompt := &loginPrompt{}
	deps := runnerDeps{store: stored, prompt: prompt, newClient: tokenClient(tokenRoundTrip(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(r, http.StatusServiceUnavailable, `{"error":"dependency unavailable"}`), nil
	}))}
	var out, stderr bytes.Buffer
	if code := runWith([]string{"--json", "--server", "https://tinker.example", "login"}, &out, &stderr, deps); code != 1 || out.String() != "{\"valid\":false,\"error\":{\"code\":\"login_failed\",\"message\":\"Login could not be completed.\"}}\n" || prompt.asked != 0 {
		t.Fatalf("code=%d out=%q err=%q asked=%d", code, out.String(), stderr.String(), prompt.asked)
	}
	if stored["https://tinker.example"] != "saved-token" {
		t.Fatal("dependency failure replaced saved credential")
	}
}

func TestLoginRateLimitIsDeterministicAndDoesNotLeakDetails(t *testing.T) {
	prompt := &loginPrompt{values: []string{"dev@example.test"}}
	deps := runnerDeps{store: client.MemoryStore{}, prompt: prompt, newClient: tokenClient(tokenRoundTrip(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/v1/version":
			return jsonResponse(r, http.StatusOK, `{"api_version":1}`), nil
		case "/api/v1/auth/otp":
			return jsonResponse(r, http.StatusTooManyRequests, `{"error":{"email":"dev@example.test","token":"server-secret"}}`), nil
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
			return nil, nil
		}
	}))}
	var out, stderr bytes.Buffer
	if code := runWith([]string{"--json", "--server", "https://tinker.example", "login"}, &out, &stderr, deps); code != 1 || out.String() != "{\"valid\":false,\"error\":{\"code\":\"rate_limited\",\"message\":\"Too many sign-in attempts. Wait and try again.\"}}\n" {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), stderr.String())
	}
	if strings.Contains(out.String(), "example") || strings.Contains(out.String(), "secret") || prompt.asked != 1 {
		t.Fatalf("detail leaked or unexpected prompt count: out=%q asked=%d", out.String(), prompt.asked)
	}
}

func TestLoginForceSkipsReuseAndDoesNotReplaceCredentialOnFailure(t *testing.T) {
	stored := client.MemoryStore{"https://tinker.example": "old-token"}
	prompt := &loginPrompt{values: []string{"other@example.test", "123456"}}
	var paths []string
	deps := runnerDeps{store: stored, prompt: prompt, newClient: tokenClient(tokenRoundTrip(func(r *http.Request) (*http.Response, error) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/api/v1/version":
			return jsonResponse(r, http.StatusOK, `{"api_version":1}`), nil
		case "/api/v1/auth/otp":
			return jsonResponse(r, http.StatusAccepted, `{"transaction":"otp_1"}`), nil
		case "/api/v1/auth/verify":
			return jsonResponse(r, http.StatusUnauthorized, `{"error":"bad code"}`), nil
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
			return nil, nil
		}
	}))}
	var out, stderr bytes.Buffer
	if code := runWith([]string{"--server", "https://tinker.example", "login", "--force"}, &out, &stderr, deps); code != 1 || prompt.asked != 2 || stored["https://tinker.example"] != "old-token" || strings.Join(paths, ",") != "GET /api/v1/version,POST /api/v1/auth/otp,POST /api/v1/auth/verify" {
		t.Fatalf("code=%d asked=%d store=%q paths=%v out=%q err=%q", code, prompt.asked, stored["https://tinker.example"], paths, out.String(), stderr.String())
	}
}

func TestLoginForceIsRejectedForNonLoginCommandsBeforeStoreOrNetwork(t *testing.T) {
	called := false
	deps := runnerDeps{store: client.MemoryStore{"https://tinker.example": "saved-token"}, newClient: tokenClient(tokenRoundTrip(func(*http.Request) (*http.Response, error) {
		called = true
		return nil, context.Canceled
	}))}
	var out, stderr bytes.Buffer
	if code := runWith([]string{"--force", "--server", "https://tinker.example", "apps", "list"}, &out, &stderr, deps); code != 2 || stderr.String() != "--force is only valid with login.\n" || called {
		t.Fatalf("code=%d out=%q err=%q called=%v", code, out.String(), stderr.String(), called)
	}
}

func TestConfirmPublicIsRejectedForNonDeployCommandsBeforeStoreOrNetwork(t *testing.T) {
	called := false
	deps := runnerDeps{store: client.MemoryStore{"https://tinker.example": "saved-token"}, newClient: tokenClient(tokenRoundTrip(func(*http.Request) (*http.Response, error) {
		called = true
		return nil, context.Canceled
	}))}
	var out, stderr bytes.Buffer
	if code := runWith([]string{"--confirm-public", "--server", "https://tinker.example", "apps", "list"}, &out, &stderr, deps); code != 2 || stderr.String() != "--confirm-public is only valid with deploy.\n" || called {
		t.Fatalf("code=%d out=%q err=%q called=%v", code, out.String(), stderr.String(), called)
	}
}

func TestPublicDeployConfirmationDependsOnCurrentOwnerPolicy(t *testing.T) {
	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, "dist"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "dist", "index.html"), []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "tinker.yaml"), []byte("version: 2\nname: demo\nbuild:\n  output: dist\naccess:\n  mode: public\n  indexing: false\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, current         string
		json, confirm, accept bool
		wantCode, wantPrompts int
		wantAck               bool
	}{
		{name: "interactive private accepts exact confirmation", current: "private", accept: true, wantCode: 1, wantPrompts: 1, wantAck: true},
		{name: "interactive private rejects wrong confirmation", current: "private", wantCode: 1, wantPrompts: 1},
		{name: "json private requires flag", current: "private", json: true, wantCode: 1},
		{name: "json private sends acknowledgement with flag", current: "private", json: true, confirm: true, wantCode: 1, wantAck: true},
		{name: "json current public continues without flag", current: "public", json: true, wantCode: 1},
		{name: "current public does not prompt or acknowledge", current: "public", wantCode: 1},
		{name: "current public preserves explicit flag for concurrent transition", current: "public", confirm: true, wantCode: 1, wantAck: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deploymentCalled, acknowledged := false, false
			transport := tokenRoundTrip(func(r *http.Request) (*http.Response, error) {
				switch r.URL.Path {
				case "/api/v1/version":
					return jsonResponse(r, http.StatusOK, `{"api_version":1}`), nil
				case "/api/v1/whoami":
					return jsonResponse(r, http.StatusOK, `{"email":"dev@example.test"}`), nil
				case "/api/v1/apps":
					return jsonResponse(r, http.StatusOK, `[{"slug":"demo","status":"active"}]`), nil
				case "/api/v1/apps/demo/access":
					return jsonResponse(r, http.StatusOK, `{"mode":"`+tc.current+`","revision":1,"allow":{"emails":[],"domains":[]}}`), nil
				case "/api/v1/apps/demo/deployments":
					deploymentCalled = true
					acknowledged = r.Header.Get("X-Tinker-Public-Acknowledged") == "true"
					return jsonResponse(r, http.StatusInternalServerError, `{}`), nil
				default:
					t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
					return nil, nil
				}
			})
			prompt := &loginPrompt{}
			if tc.accept {
				prompt.values = []string{"public:demo"}
			} else {
				prompt.values = []string{"public:wrong"}
			}
			args := []string{"--server", "https://tinker.example"}
			if tc.json {
				args = append(args, "--json")
			}
			if tc.confirm {
				args = append(args, "--confirm-public")
			}
			args = append(args, "deploy", project)
			var out, stderr bytes.Buffer
			code := runWith(args, &out, &stderr, runnerDeps{store: client.MemoryStore{"https://tinker.example": "token"}, prompt: prompt, newClient: tokenClient(transport)})
			if code != tc.wantCode || prompt.asked != tc.wantPrompts || acknowledged != tc.wantAck {
				t.Fatalf("code=%d prompts=%d acknowledged=%v output=%q err=%q", code, prompt.asked, acknowledged, out.String(), stderr.String())
			}
			if tc.current == "private" && !tc.accept && !tc.confirm && deploymentCalled {
				t.Fatal("rejected broadening reached deployment")
			}
			if tc.json && tc.current == "private" && !tc.confirm && deploymentCalled {
				t.Fatal("JSON broadening reached deployment without confirmation")
			}
			if tc.current == "public" && !deploymentCalled {
				t.Fatal("public continuity did not reach deployment")
			}
		})
	}
}

func TestPrivateDeployRejectsConfirmPublicBeforeNetwork(t *testing.T) {
	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, "dist"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "dist", "index.html"), []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "tinker.yaml"), []byte("version: 1\nname: demo\nbuild:\n  output: dist\naccess:\n  mode: private\n"), 0600); err != nil {
		t.Fatal(err)
	}
	called := false
	var out, stderr bytes.Buffer
	code := runWith([]string{"--confirm-public", "--server", "https://tinker.example", "deploy", project}, &out, &stderr, runnerDeps{store: client.MemoryStore{"https://tinker.example": "token"}, newClient: tokenClient(tokenRoundTrip(func(*http.Request) (*http.Response, error) { called = true; return nil, context.Canceled }))})
	if code != 2 || called || stderr.String() != "--confirm-public requires a public manifest.\n" {
		t.Fatalf("code=%d called=%v out=%q err=%q", code, called, out.String(), stderr.String())
	}
}

func TestWhoamiUsesAuthenticationSuccessOutput(t *testing.T) {
	stored := client.MemoryStore{"https://tinker.example": "saved-token"}
	if err := stored.SetDefaultServer("https://tinker.example"); err != nil {
		t.Fatal(err)
	}
	deps := runnerDeps{store: stored, newClient: tokenClient(tokenRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/whoami" || r.Header.Get("Authorization") != "Bearer saved-token" {
			t.Fatalf("unexpected whoami request %s %s %q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
		}
		return jsonResponse(r, http.StatusOK, `{"email":"dev@example.test"}`), nil
	}))}
	var out, stderr bytes.Buffer
	if code := runWith([]string{"whoami"}, &out, &stderr, deps); code != 0 || out.String() != "Logged in as dev@example.test.\n" || stderr.Len() != 0 {
		t.Fatalf("human whoami code=%d out=%q err=%q", code, out.String(), stderr.String())
	}
	out.Reset()
	stderr.Reset()
	if code := runWith([]string{"--json", "whoami"}, &out, &stderr, deps); code != 0 || out.String() != "{\"valid\":true,\"name\":\"dev@example.test\"}\n" || stderr.Len() != 0 {
		t.Fatalf("JSON whoami code=%d out=%q err=%q", code, out.String(), stderr.String())
	}
}

func TestSavedDefaultServerResolvesCommandsAndExplicitServerWins(t *testing.T) {
	stored := client.MemoryStore{"https://tinker.example": "saved", "https://override.example": "override"}
	if err := stored.SetDefaultServer("https://tinker.example"); err != nil {
		t.Fatal(err)
	}
	var seen []string
	deps := runnerDeps{store: stored, newClient: tokenClient(tokenRoundTrip(func(r *http.Request) (*http.Response, error) {
		seen = append(seen, r.URL.Host+" "+r.Header.Get("Authorization"))
		return jsonResponse(r, http.StatusOK, `[]`), nil
	}))}
	var out, stderr bytes.Buffer
	if code := runWith([]string{"apps", "list"}, &out, &stderr, deps); code != 0 || !strings.Contains(out.String(), "[]") {
		t.Fatalf("default command code=%d out=%q err=%q", code, out.String(), stderr.String())
	}
	out.Reset()
	stderr.Reset()
	if code := runWith([]string{"--server", "https://override.example", "apps", "list"}, &out, &stderr, deps); code != 0 {
		t.Fatalf("override command code=%d out=%q err=%q", code, out.String(), stderr.String())
	}
	if got := strings.Join(seen, ","); got != "tinker.example Bearer saved,override.example Bearer override" {
		t.Fatalf("requests = %q", got)
	}
	if got, _ := stored.DefaultServer(); got != "https://tinker.example" {
		t.Fatalf("explicit non-login override changed default to %q", got)
	}
}

func TestMissingOrCorruptDefaultServerFailsBeforeNetwork(t *testing.T) {
	called := false
	deps := runnerDeps{store: client.MemoryStore{}, newClient: tokenClient(tokenRoundTrip(func(*http.Request) (*http.Response, error) {
		called = true
		return nil, context.Canceled
	}))}
	var out, stderr bytes.Buffer
	if code := runWith([]string{"apps", "list"}, &out, &stderr, deps); code != 2 || stderr.String() != "Server setup requires a valid HTTPS URL.\n" || called {
		t.Fatalf("missing config code=%d out=%q err=%q called=%v", code, out.String(), stderr.String(), called)
	}
	corrupt := &spyStore{values: map[string]string{}, defaultErr: client.ErrStore}
	out.Reset()
	stderr.Reset()
	if code := runWith([]string{"apps", "list"}, &out, &stderr, runnerDeps{store: corrupt}); code != 2 || stderr.String() != "Saved server could not be read.\n" {
		t.Fatalf("corrupt config code=%d out=%q err=%q", code, out.String(), stderr.String())
	}
}

func TestMissingServerSetupWizardVerifiesCachesAndContinues(t *testing.T) {
	stored := &spyStore{values: map[string]string{}}
	prompt := &loginPrompt{values: []string{"https://tinker.example"}}
	var paths []string
	deps := runnerDeps{store: stored, prompt: prompt, newClient: tokenClient(tokenRoundTrip(func(r *http.Request) (*http.Response, error) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/api/v1/version":
			return jsonResponse(r, http.StatusOK, `{"api_version":1}`), nil
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
			return nil, nil
		}
	}))}
	var out, stderr bytes.Buffer
	if code := runWith([]string{"apps", "list"}, &out, &stderr, deps); code != 1 || stderr.String() != "Login required.\n" {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), stderr.String())
	}
	if prompt.asked != 1 || stored.defaultURL != "https://tinker.example" || strings.Join(paths, ",") != "GET /api/v1/version" {
		t.Fatalf("asked=%d default=%q paths=%v", prompt.asked, stored.defaultURL, paths)
	}
	// The cached platform is used next time without another prompt, even though
	// the original setup command did not create a credential.
	out.Reset()
	stderr.Reset()
	if code := runWith([]string{"apps", "list"}, &out, &stderr, deps); code != 1 || prompt.asked != 1 {
		t.Fatalf("cached code=%d asked=%d out=%q err=%q", code, prompt.asked, out.String(), stderr.String())
	}
}

func TestMissingServerSetupCachesBeforeLoginRequired(t *testing.T) {
	stored := &spyStore{values: map[string]string{}}
	prompt := &loginPrompt{values: []string{"https://tinker.example"}}
	deps := runnerDeps{store: stored, prompt: prompt, newClient: tokenClient(tokenRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/version" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		return jsonResponse(r, http.StatusOK, `{"api_version":1}`), nil
	}))}
	var out, stderr bytes.Buffer
	if code := runWith([]string{"whoami"}, &out, &stderr, deps); code != 1 || stderr.String() != "Login required.\n" || stored.defaultURL != "https://tinker.example" {
		t.Fatalf("code=%d out=%q err=%q default=%q", code, out.String(), stderr.String(), stored.defaultURL)
	}
}

func TestMissingServerSetupWizardDenyPaths(t *testing.T) {
	for _, tt := range []struct {
		name      string
		input     string
		response  func(*http.Request) (*http.Response, error)
		setErr    error
		json      bool
		wantCalls int
	}{
		{"empty", "", nil, nil, false, 0},
		{"oversized", strings.Repeat("x", maxSetupServerInput+1), nil, nil, false, 0},
		{"invalid", "http://tinker.example", nil, nil, false, 0},
		{"incompatible", "https://tinker.example", func(r *http.Request) (*http.Response, error) {
			return jsonResponse(r, http.StatusOK, `{"api_version":2}`), nil
		}, nil, false, 1},
		{"redirect", "https://tinker.example", func(r *http.Request) (*http.Response, error) { return jsonResponse(r, http.StatusFound, ``), nil }, nil, false, 1},
		{"transport", "https://tinker.example", func(*http.Request) (*http.Response, error) { return nil, context.DeadlineExceeded }, nil, false, 1},
		{"store", "https://tinker.example", func(r *http.Request) (*http.Response, error) {
			return jsonResponse(r, http.StatusOK, `{"api_version":1}`), nil
		}, client.ErrStore, false, 1},
		{"json", "https://tinker.example", nil, nil, true, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stored := &spyStore{values: map[string]string{}, setErr: tt.setErr}
			prompt := &loginPrompt{values: []string{tt.input}}
			calls := 0
			deps := runnerDeps{store: stored, prompt: prompt}
			if tt.response != nil {
				deps.newClient = tokenClient(tokenRoundTrip(func(r *http.Request) (*http.Response, error) {
					calls++
					if r.Method != http.MethodGet || r.URL.Path != "/api/v1/version" {
						t.Fatalf("target request ran: %s %s", r.Method, r.URL.Path)
					}
					return tt.response(r)
				}))
			}
			argv := []string{"apps", "list"}
			if tt.json {
				argv = []string{"--json", "apps", "list"}
			}
			var out, stderr bytes.Buffer
			if code := runWith(argv, &out, &stderr, deps); code != 2 || stored.defaultURL != "" || calls != tt.wantCalls {
				t.Fatalf("code=%d out=%q err=%q default=%q calls=%d", code, out.String(), stderr.String(), stored.defaultURL, calls)
			}
			if tt.json {
				if prompt.asked != 0 || out.String() != "{\"valid\":false,\"error\":{\"code\":\"usage\",\"message\":\"No saved server. Run tinker login --server https://your-tinkercloud.example.\"}}\n" {
					t.Fatalf("JSON prompt=%d out=%q", prompt.asked, out.String())
				}
			} else if prompt.asked != 1 || !strings.Contains(stderr.String(), "Server") {
				t.Fatalf("prompt=%d stderr=%q", prompt.asked, stderr.String())
			}
		})
	}
}

func TestExplicitServerNeverStartsWizardOrCachesDefault(t *testing.T) {
	stored := &spyStore{values: map[string]string{}}
	prompt := &loginPrompt{values: []string{"https://other.example"}}
	calls := 0
	deps := runnerDeps{store: stored, prompt: prompt, newClient: tokenClient(tokenRoundTrip(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, context.Canceled
	}))}
	var out, stderr bytes.Buffer
	if code := runWith([]string{"--server", "https://explicit.example", "apps", "list"}, &out, &stderr, deps); code != 1 || prompt.asked != 0 || stored.defaultURL != "" || calls != 0 {
		t.Fatalf("code=%d asked=%d default=%q calls=%d out=%q err=%q", code, prompt.asked, stored.defaultURL, calls, out.String(), stderr.String())
	}
}

func TestInvalidExplicitServerAndPromptEOFNeverCache(t *testing.T) {
	stored := &spyStore{values: map[string]string{}}
	prompt := &loginPrompt{}
	var out, stderr bytes.Buffer
	if code := runWith([]string{"--server", "http://tinker.example", "apps", "list"}, &out, &stderr, runnerDeps{store: stored, prompt: prompt}); code != 2 || prompt.asked != 0 || stored.defaultURL != "" || stderr.String() != "Server must use HTTPS.\n" {
		t.Fatalf("explicit code=%d asked=%d default=%q out=%q err=%q", code, prompt.asked, stored.defaultURL, out.String(), stderr.String())
	}
	out.Reset()
	stderr.Reset()
	if code := runWith([]string{"apps", "list"}, &out, &stderr, runnerDeps{store: stored, prompt: prompt}); code != 2 || prompt.asked != 1 || stored.defaultURL != "" || stderr.String() != "Server setup requires a valid HTTPS URL.\n" {
		t.Fatalf("EOF code=%d asked=%d default=%q out=%q err=%q", code, prompt.asked, stored.defaultURL, out.String(), stderr.String())
	}
}

func TestInitGeneratesStrictManifestAndNeverOverwrites(t *testing.T) {
	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, "dist"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "dist", "index.html"), []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	prompt := &loginPrompt{values: []string{"demo", "A useful app", "dist", "viewer@example.test,example.test", "kv,realtime", "index.html"}}
	var out, stderr bytes.Buffer
	if code := runWith([]string{"init", project}, &out, &stderr, runnerDeps{prompt: prompt}); code != 0 || out.String() != "Created "+filepath.Join(project, "tinker.yaml")+".\n" {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), stderr.String())
	}
	b, err := os.ReadFile(filepath.Join(project, "tinker.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := releases.ParseManifest(b)
	if err != nil || m.Name != "demo" || !m.KV || !m.Realtime || len(m.Emails) != 1 || len(m.Domains) != 1 {
		t.Fatalf("manifest=%#v err=%v", m, err)
	}
	before := string(b)
	out.Reset()
	stderr.Reset()
	if code := runWith([]string{"init", project}, &out, &stderr, runnerDeps{prompt: prompt}); code != 1 || stringMustRead(t, filepath.Join(project, "tinker.yaml")) != before {
		t.Fatalf("overwrite code=%d", code)
	}
}

func TestInitAcceptsDisplayedDefaultsAndKeepsOptionalFieldsEmpty(t *testing.T) {
	project := filepath.Join(t.TempDir(), "review-tool")
	if err := os.MkdirAll(filepath.Join(project, "dist"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "dist", "index.html"), []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	prompt := &loginPrompt{values: []string{"", "", "", "", "", ""}}
	var out, stderr bytes.Buffer
	if code := runWith([]string{"init", project}, &out, &stderr, runnerDeps{prompt: prompt}); code != 0 {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), stderr.String())
	}
	m, err := releases.ParseManifest([]byte(stringMustRead(t, filepath.Join(project, "tinker.yaml"))))
	if err != nil || m.Name != "review-tool" || m.BuildOutput != "dist" || m.Description != "" || len(m.Emails) != 0 || m.KV || m.Blobs || m.Realtime || m.SPAFallback != "" {
		t.Fatalf("manifest=%#v err=%v", m, err)
	}
}

func TestInitWizardRejectsInvalidOversizeAndEOFWithoutWritingManifest(t *testing.T) {
	for _, test := range []struct {
		name   string
		values []string
		setup  func(*testing.T, string)
	}{
		{name: "invalid slug", values: []string{"bad_slug"}},
		{name: "oversize slug", values: []string{strings.Repeat("a", maxSetupServerInput+1)}},
		{name: "EOF", values: nil},
		{name: "escaping output", values: []string{"demo", "", "../outside"}},
		{name: "missing output", values: []string{"demo", "", "missing"}},
		{name: "non-directory output", values: []string{"demo", "", "file-output"}, setup: func(t *testing.T, project string) {
			t.Helper()
			if err := os.WriteFile(filepath.Join(project, "file-output"), []byte("not a directory"), 0600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "overlong description", values: []string{"demo", strings.Repeat("a", 281), "dist", "", "", ""}},
		{name: "control description", values: []string{"demo", "bad\tvalue", "dist", "", "", ""}},
		{name: "invalid email", values: []string{"demo", "", "dist", "viewer@", "", ""}},
		{name: "invalid domain", values: []string{"demo", "", "dist", "bad_domain", "", ""}},
		{name: "invalid feature", values: []string{"demo", "", "dist", "", "wrong", ""}},
		{name: "traversal fallback", values: []string{"demo", "", "dist", "", "", "../index.html"}},
		{name: "missing fallback", values: []string{"demo", "", "dist", "", "", "missing.html"}},
		{name: "symlink fallback", values: []string{"demo", "", "dist", "", "", "link.html"}, setup: func(t *testing.T, project string) {
			t.Helper()
			if err := os.Symlink("index.html", filepath.Join(project, "dist", "link.html")); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			project := t.TempDir()
			if err := os.Mkdir(filepath.Join(project, "dist"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(project, "dist", "index.html"), []byte("ok"), 0600); err != nil {
				t.Fatal(err)
			}
			if test.setup != nil {
				test.setup(t, project)
			}
			prompt := &loginPrompt{values: append([]string(nil), test.values...)}
			var out, stderr bytes.Buffer
			if code := runWith([]string{"init", project}, &out, &stderr, runnerDeps{prompt: prompt}); code != 1 {
				t.Fatalf("code=%d out=%q err=%q", code, out.String(), stderr.String())
			}
			if _, err := os.Lstat(filepath.Join(project, "tinker.yaml")); !os.IsNotExist(err) {
				t.Fatalf("manifest was written: %v", err)
			}
		})
	}
}

func TestInitWizardRejectsReservedAppSlugWithoutWritingManifest(t *testing.T) {
	project := t.TempDir()
	prompt := &loginPrompt{values: []string{"ADMIN"}}
	if _, err := createManifest(project, prompt); err == nil {
		t.Fatal("reserved app slug accepted")
	}
	if _, err := os.Lstat(filepath.Join(project, "tinker.yaml")); !os.IsNotExist(err) {
		t.Fatalf("reserved slug wrote a manifest: %v", err)
	}
}

func TestInitNeverFollowsManifestTargetSymlink(t *testing.T) {
	project := t.TempDir()
	target := filepath.Join(t.TempDir(), "outside.yaml")
	if err := os.WriteFile(target, []byte("do not replace"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(project, "tinker.yaml")); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	if code := runWith([]string{"init", project}, &out, &stderr, runnerDeps{prompt: &loginPrompt{values: []string{"demo"}}}); code != 1 {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), stderr.String())
	}
	if got := stringMustRead(t, target); got != "do not replace" {
		t.Fatalf("symlink target changed: %q", got)
	}
}

func TestInspectManifestIsLocalAndDoesNotRequireServerConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tinker.yaml")
	if err := os.WriteFile(path, []byte("version: 1\nname: demo\naccess:\n  mode: private\n"), 0600); err != nil {
		t.Fatal(err)
	}
	stored := &spyStore{values: map[string]string{}, defaultErr: client.ErrStore}
	var out, stderr bytes.Buffer
	if code := runWith([]string{"inspect-manifest", path}, &out, &stderr, runnerDeps{store: stored}); code != 0 || stored.defaultReads != 0 || out.String() != "Manifest valid: demo\n" {
		t.Fatalf("code=%d reads=%d out=%q err=%q", code, stored.defaultReads, out.String(), stderr.String())
	}
}

func TestManifestSetupRejectsSymlinkedOutputAndFallbackComponents(t *testing.T) {
	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, "real"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "real", "index.html"), []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real", filepath.Join(project, "dist")); err != nil {
		t.Fatal(err)
	}
	if safeOutput(project, "dist") || safeFallback(project, "dist", "index.html") {
		t.Fatal("accepted symlinked output component")
	}
	if err := os.Mkdir(filepath.Join(project, "safe"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../real/index.html", filepath.Join(project, "safe", "index.html")); err != nil {
		t.Fatal(err)
	}
	if safeFallback(project, "safe", "index.html") {
		t.Fatal("accepted symlinked fallback component")
	}
}

func TestDeployCreatesMissingManifestVerifiesSavedBearerAndArchivesOnlyOutput(t *testing.T) {
	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, "dist"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "dist", "index.html"), []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "source.txt"), []byte("not deployed"), 0600); err != nil {
		t.Fatal(err)
	}
	stored := &spyStore{values: map[string]string{"https://tinker.example": "saved-token"}}
	prompt := &loginPrompt{values: []string{"demo", "", "dist", "", "", ""}}
	var archiveNames, calls []string
	deps := runnerDeps{store: stored, prompt: prompt, newClient: tokenClient(successfulDeployTransport(t, "saved-token", &archiveNames, &calls))}
	var out, stderr bytes.Buffer
	if code := runWith([]string{"--server", "https://tinker.example", "deploy", project}, &out, &stderr, deps); code != 0 {
		t.Fatalf("code=%d out=%q err=%q calls=%v", code, out.String(), stderr.String(), calls)
	}
	if prompt.asked != 6 || stored.putCalls != 0 || !strings.Contains(out.String(), "Created "+filepath.Join(project, "tinker.yaml")+".") {
		t.Fatalf("asked=%d puts=%d out=%q", prompt.asked, stored.putCalls, out.String())
	}
	if _, err := releases.ParseManifest([]byte(stringMustRead(t, filepath.Join(project, "tinker.yaml")))); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(archiveNames, ","), "index.html,tinker.yaml"; got != want {
		t.Fatalf("archive entries=%q want=%q", got, want)
	}
	if strings.Join(calls[:2], ",") != "GET /api/v1/version,GET /api/v1/whoami" {
		t.Fatalf("credential was not proven first: %v", calls)
	}
}

func TestFreshDeployPromptsForManifestBeforeEmailAndCode(t *testing.T) {
	project := filepath.Join(t.TempDir(), "demo-project")
	if err := os.MkdirAll(filepath.Join(project, "dist"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "dist", "index.html"), []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	stored := &spyStore{values: map[string]string{}}
	prompt := &loginPrompt{values: []string{"demo", "", "dist", "", "", "", "dev@example.test", "123456"}}
	var archiveNames, calls []string
	transport := tokenRoundTrip(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/v1/version":
			return jsonResponse(r, http.StatusOK, `{"api_version":1}`), nil
		case "/api/v1/auth/otp":
			return jsonResponse(r, http.StatusAccepted, `{"transaction":"tx"}`), nil
		case "/api/v1/auth/verify":
			return jsonResponse(r, http.StatusOK, `{"token":"new-token","email":"dev@example.test","api_version":1}`), nil
		case "/api/v1/whoami":
			return jsonResponse(r, http.StatusOK, `{"email":"dev@example.test"}`), nil
		default:
			return successfulDeployTransport(t, "new-token", &archiveNames, &calls).RoundTrip(r)
		}
	})
	var out, stderr bytes.Buffer
	deps := runnerDeps{store: stored, prompt: prompt, newClient: tokenClient(transport)}
	if code := runWith([]string{"--server", "https://tinker.example", "deploy", project}, &out, &stderr, deps); code != 0 {
		t.Fatalf("code=%d out=%q err=%q questions=%v", code, out.String(), stderr.String(), prompt.questions)
	}
	wantQuestions := []string{
		"App slug (demo-project, Enter to accept): ",
		"Description (optional): ",
		"Build output (dist, Enter to accept): ",
		"Allowed emails or domains, comma-separated (optional): ",
		"Features (kv,blobs,realtime; optional): ",
		"SPA fallback (optional): ",
		"Email: ",
		"Code: ",
	}
	if got, want := strings.Join(prompt.questions, "\n"), strings.Join(wantQuestions, "\n"); got != want {
		t.Fatalf("questions:\n%s\nwant:\n%s", got, want)
	}
	manifest, err := releases.ParseManifest([]byte(stringMustRead(t, filepath.Join(project, "tinker.yaml"))))
	if err != nil || manifest.Name != "demo" {
		t.Fatalf("manifest=%+v err=%v", manifest, err)
	}
	if stored.values["https://tinker.example"] != "new-token" || stored.putCalls != 1 || stored.defaultURL != "https://tinker.example" || stored.setDefaultCalls != 1 {
		t.Fatalf("stored=%q puts=%d default=%q defaultWrites=%d", stored.values["https://tinker.example"], stored.putCalls, stored.defaultURL, stored.setDefaultCalls)
	}
	deployments := 0
	for _, call := range calls {
		if call == "POST /api/v1/apps/demo/deployments" {
			deployments++
		}
	}
	if deployments != 1 {
		t.Fatalf("deployment calls=%d all calls=%v", deployments, calls)
	}
}

func TestDeploySuccessWritesBoundedHumanReceipt(t *testing.T) {
	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, "dist"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "dist", "index.html"), []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "tinker.yaml"), []byte("version: 1\nname: demo\nbuild:\n  output: dist\naccess:\n  mode: private\n"), 0600); err != nil {
		t.Fatal(err)
	}

	var archiveNames, calls []string
	deps := runnerDeps{
		store:     client.MemoryStore{"https://tinker.example": "saved-token"},
		newClient: tokenClient(successfulDeployTransport(t, "saved-token", &archiveNames, &calls)),
	}
	var out, stderr bytes.Buffer
	code := runWith([]string{"--server", "https://tinker.example", "deploy", project}, &out, &stderr, deps)
	want := "Deployment: deployment\nState: active\nURL: https://demo.tinker.example/\n"
	if code != 0 || stderr.Len() != 0 || out.String() != want {
		t.Fatalf("code=%d out=%q stderr=%q", code, out.String(), stderr.String())
	}
	if deploymentCreates(calls) != 1 {
		t.Fatalf("deployment invocations=%v", calls)
	}
	for _, secret := range []string{"saved-token", "tinker.yaml", "viewer_"} {
		if strings.Contains(out.String(), secret) {
			t.Fatalf("human receipt leaked %q: %q", secret, out.String())
		}
	}
}

func TestDeploySuccessWritesBoundedJSONReceipt(t *testing.T) {
	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, "dist"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "dist", "index.html"), []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "tinker.yaml"), []byte("version: 1\nname: demo\nbuild:\n  output: dist\naccess:\n  mode: private\n"), 0600); err != nil {
		t.Fatal(err)
	}

	var archiveNames, calls []string
	deps := runnerDeps{
		store:     client.MemoryStore{"https://tinker.example": "saved-token"},
		newClient: tokenClient(successfulDeployTransport(t, "saved-token", &archiveNames, &calls)),
	}
	var out, stderr bytes.Buffer
	code := runWith([]string{"--json", "--server", "https://tinker.example", "deploy", project}, &out, &stderr, deps)
	want := "{\"valid\":true,\"deployment\":{\"deployment_id\":\"deployment\",\"url\":\"https://demo.tinker.example/\",\"state\":\"active\"}}\n"
	if code != 0 || stderr.Len() != 0 || out.String() != want {
		t.Fatalf("code=%d out=%q stderr=%q", code, out.String(), stderr.String())
	}
	if deploymentCreates(calls) != 1 {
		t.Fatalf("deployment invocations=%v", calls)
	}
	for _, secret := range []string{"saved-token", "tinker.yaml", "viewer_"} {
		if strings.Contains(out.String(), secret) {
			t.Fatalf("JSON receipt leaked %q: %q", secret, out.String())
		}
	}
}

func deploymentCreates(calls []string) int {
	creates := 0
	for _, call := range calls {
		if call == "POST /api/v1/apps/demo/deployments" {
			creates++
		}
	}
	return creates
}

func TestDeployReportsActiveButUnverifiedReceipt(t *testing.T) {
	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, "dist"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "dist", "index.html"), []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "tinker.yaml"), []byte("version: 1\nname: demo\nbuild:\n  output: dist\naccess:\n  mode: private\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var archiveNames, calls []string
	base := successfulDeployTransport(t, "saved-token", &archiveNames, &calls)
	transport := tokenRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "demo.tinker.example" {
			return jsonResponse(r, http.StatusOK, `<html>unsafe public response</html>`), nil
		}
		return base.RoundTrip(r)
	})
	deps := runnerDeps{
		store:     client.MemoryStore{"https://tinker.example": "saved-token"},
		newClient: tokenClient(transport),
	}
	var out, stderr bytes.Buffer
	code := runWith([]string{"--json", "--server", "https://tinker.example", "deploy", project}, &out, &stderr, deps)
	want := "{\"valid\":false,\"deployment\":{\"deployment_id\":\"deployment\",\"url\":\"https://demo.tinker.example/\",\"state\":\"active\",\"verification\":\"public_probe_invalid_response\"},\"error\":{\"code\":\"active_but_unverified\",\"message\":\"Deployment is active, but public verification is incomplete.\"}}\n"
	if code != 1 || stderr.Len() != 0 || out.String() != want {
		t.Fatalf("code=%d out=%q stderr=%q", code, out.String(), stderr.String())
	}
	if strings.Contains(out.String(), "unsafe public response") || strings.Contains(out.String(), "saved-token") {
		t.Fatal("unsafe public evidence or credential leaked")
	}
}

func TestDeployReportsActivationFailureReceipt(t *testing.T) {
	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, "dist"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "dist", "index.html"), []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "tinker.yaml"), []byte("version: 1\nname: demo\nbuild:\n  output: dist\naccess:\n  mode: private\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var archiveNames, calls []string
	base := successfulDeployTransport(t, "saved-token", &archiveNames, &calls)
	transport := tokenRoundTrip(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/activate") {
			h := make(http.Header)
			h.Set("X-Request-ID", "req_0123456789abcdef01234567")
			return &http.Response{StatusCode: http.StatusConflict, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"activation_candidate_probe_failed","message":"secret policy detail","request_id":"req_0123456789abcdef01234567"}}`)), Header: h, Request: r}, nil
		}
		return base.RoundTrip(r)
	})
	deps := runnerDeps{store: client.MemoryStore{"https://tinker.example": "saved-token"}, newClient: tokenClient(transport)}

	var jsonOut, jsonErr bytes.Buffer
	code := runWith([]string{"--json", "--server", "https://tinker.example", "deploy", project}, &jsonOut, &jsonErr, deps)
	wantJSON := "{\"valid\":false,\"deployment\":{\"deployment_id\":\"deployment\",\"state\":\"verified\",\"reason\":\"activation_candidate_probe_failed\",\"request_id\":\"req_0123456789abcdef01234567\"},\"error\":{\"code\":\"activation_failed\",\"message\":\"Deployment activation failed.\"}}\n"
	if code != 1 || jsonErr.Len() != 0 || jsonOut.String() != wantJSON {
		t.Fatalf("code=%d out=%q stderr=%q", code, jsonOut.String(), jsonErr.String())
	}
	if strings.Contains(jsonOut.String(), "secret policy detail") || strings.Contains(jsonOut.String(), "saved-token") {
		t.Fatal("unsafe activation detail leaked")
	}

	var humanOut, humanErr bytes.Buffer
	code = runWith([]string{"--server", "https://tinker.example", "deploy", project}, &humanOut, &humanErr, deps)
	wantHuman := "Deployment activation failed.\nDeployment: deployment\nState: verified\nReason: activation_candidate_probe_failed\nRequest ID: req_0123456789abcdef01234567\n"
	if code != 1 || humanOut.Len() != 0 || humanErr.String() != wantHuman {
		t.Fatalf("code=%d out=%q stderr=%q", code, humanOut.String(), humanErr.String())
	}
}

func TestDeployUnauthorizedBearerUsesOTPAndStoresOnlyVerifiedReplacement(t *testing.T) {
	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, "dist"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "dist", "index.html"), []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "tinker.yaml"), []byte("version: 1\nname: demo\nbuild:\n  output: dist\naccess:\n  mode: private\n"), 0600); err != nil {
		t.Fatal(err)
	}
	stored := &spyStore{values: map[string]string{"https://tinker.example": "old-token"}}
	prompt := &loginPrompt{values: []string{"dev@example.test", "123456"}}
	var archiveNames, calls []string
	verifiedNew := false
	transport := tokenRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/api/v1/version":
			return jsonResponse(r, http.StatusOK, `{"api_version":1}`), nil
		case "/api/v1/whoami":
			if r.Header.Get("Authorization") == "Bearer old-token" {
				return jsonResponse(r, http.StatusUnauthorized, `{}`), nil
			}
			if r.Header.Get("Authorization") == "Bearer new-token" {
				verifiedNew = true
				return jsonResponse(r, http.StatusOK, `{"email":"dev@example.test"}`), nil
			}
			t.Fatalf("unexpected whoami bearer %q", r.Header.Get("Authorization"))
		case "/api/v1/auth/otp":
			return jsonResponse(r, http.StatusAccepted, `{"transaction":"tx"}`), nil
		case "/api/v1/auth/verify":
			return jsonResponse(r, http.StatusOK, `{"token":"new-token","email":"dev@example.test","api_version":1}`), nil
		}
		return successfulDeployTransport(t, "new-token", &archiveNames, &calls).RoundTrip(r)
	})
	var out, stderr bytes.Buffer
	deps := runnerDeps{store: stored, prompt: prompt, newClient: tokenClient(transport)}
	if code := runWith([]string{"--server", "https://tinker.example", "deploy", project}, &out, &stderr, deps); code != 0 || !verifiedNew || stored.values["https://tinker.example"] != "new-token" || stored.putCalls != 1 || stored.defaultURL != "https://tinker.example" || stored.setDefaultCalls != 1 || prompt.asked != 2 {
		t.Fatalf("code=%d verified=%v stored=%q puts=%d default=%q defaultWrites=%d asked=%d out=%q err=%q calls=%v", code, verifiedNew, stored.values["https://tinker.example"], stored.putCalls, stored.defaultURL, stored.setDefaultCalls, prompt.asked, out.String(), stderr.String(), calls)
	}
}

func TestDeployPrerequisiteFailuresDoNotReachAppOrDeployment(t *testing.T) {
	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, "dist"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "dist", "index.html"), []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "tinker.yaml"), []byte("version: 1\nname: demo\nbuild:\n  output: dist\naccess:\n  mode: private\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		store  *spyStore
		prompt *loginPrompt
		client tokenRoundTrip
	}{
		{
			name:  "dependency failure",
			store: &spyStore{values: map[string]string{"https://tinker.example": "saved-token"}},
			client: tokenRoundTrip(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/api/v1/version" {
					return nil, context.DeadlineExceeded
				}
				t.Fatalf("unexpected request %s", r.URL.Path)
				return nil, nil
			}),
		},
		{
			name:   "OTP failure",
			store:  &spyStore{values: map[string]string{}},
			prompt: &loginPrompt{values: []string{"dev@example.test", "123456"}},
			client: tokenRoundTrip(func(r *http.Request) (*http.Response, error) {
				switch r.URL.Path {
				case "/api/v1/version":
					return jsonResponse(r, http.StatusOK, `{"api_version":1}`), nil
				case "/api/v1/auth/otp":
					return jsonResponse(r, http.StatusAccepted, `{"transaction":"tx"}`), nil
				case "/api/v1/auth/verify":
					return jsonResponse(r, http.StatusUnauthorized, `{}`), nil
				default:
					t.Fatalf("unexpected request %s", r.URL.Path)
					return nil, nil
				}
			}),
		},
		{
			name:   "credential store failure",
			store:  &spyStore{values: map[string]string{}, putErr: client.ErrStore},
			prompt: &loginPrompt{values: []string{"dev@example.test", "123456"}},
			client: tokenRoundTrip(func(r *http.Request) (*http.Response, error) {
				switch r.URL.Path {
				case "/api/v1/auth/otp":
					return jsonResponse(r, http.StatusAccepted, `{"transaction":"tx"}`), nil
				case "/api/v1/auth/verify":
					return jsonResponse(r, http.StatusOK, `{"token":"new-token","email":"dev@example.test","api_version":1}`), nil
				case "/api/v1/version":
					return jsonResponse(r, http.StatusOK, `{"api_version":1}`), nil
				case "/api/v1/whoami":
					return jsonResponse(r, http.StatusOK, `{"email":"dev@example.test"}`), nil
				default:
					t.Fatalf("unexpected request %s", r.URL.Path)
					return nil, nil
				}
			}),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var out, stderr bytes.Buffer
			code := runWith([]string{"--server", "https://tinker.example", "deploy", project}, &out, &stderr, runnerDeps{store: test.store, prompt: test.prompt, newClient: tokenClient(test.client)})
			if code != 1 || strings.Contains(out.String()+stderr.String(), "/api/v1/apps") {
				t.Fatalf("code=%d out=%q err=%q", code, out.String(), stderr.String())
			}
		})
	}
}

func TestJSONDeployMissingManifestNeverPromptsWritesOrRequestsNetwork(t *testing.T) {
	project := t.TempDir()
	prompt := &loginPrompt{values: []string{"must-not-be-read"}}
	stored := &spyStore{values: map[string]string{}}
	called := false
	var out, stderr bytes.Buffer
	code := runWith([]string{"--json", "--server", "https://tinker.example", "deploy", project}, &out, &stderr, runnerDeps{store: stored, prompt: prompt, newClient: tokenClient(tokenRoundTrip(func(*http.Request) (*http.Response, error) {
		called = true
		return nil, context.Canceled
	}))})
	if code != 1 || prompt.asked != 0 || stored.putCalls != 0 || called || out.String() != `{"valid":false,"error":{"code":"manifest_required","message":"Create a valid tinker.yaml with tinker init."}}`+"\n" {
		t.Fatalf("code=%d asked=%d puts=%d called=%v out=%q err=%q", code, prompt.asked, stored.putCalls, called, out.String(), stderr.String())
	}
	if _, err := os.Lstat(filepath.Join(project, "tinker.yaml")); !os.IsNotExist(err) {
		t.Fatalf("manifest was created: %v", err)
	}
}

func TestDeployExistingManifestSkipsManifestWizard(t *testing.T) {
	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, "dist"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "dist", "index.html"), []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "tinker.yaml"), []byte("version: 1\nname: demo\nbuild:\n  output: dist\naccess:\n  mode: private\n"), 0600); err != nil {
		t.Fatal(err)
	}
	prompt := &loginPrompt{}
	stored := &spyStore{values: map[string]string{"https://tinker.example": "saved-token"}}
	var archiveNames, calls []string
	var out, stderr bytes.Buffer
	code := runWith([]string{"--server", "https://tinker.example", "deploy", project}, &out, &stderr, runnerDeps{store: stored, prompt: prompt, newClient: tokenClient(successfulDeployTransport(t, "saved-token", &archiveNames, &calls))})
	if code != 0 || prompt.asked != 0 || len(archiveNames) == 0 {
		t.Fatalf("code=%d asked=%d archive=%v out=%q err=%q", code, prompt.asked, archiveNames, out.String(), stderr.String())
	}
}

func TestHumanDeployPersistsVerifiedExplicitServerWithoutOverwritingExistingDefault(t *testing.T) {
	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, "dist"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "dist", "index.html"), []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "tinker.yaml"), []byte("version: 1\nname: demo\nbuild:\n  output: dist\naccess:\n  mode: private\n"), 0600); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name              string
		defaultURL        string
		wantDefault       string
		wantDefaultWrites int
	}{
		{name: "missing default is saved", wantDefault: "https://tinker.example", wantDefaultWrites: 1},
		{name: "existing other default is preserved", defaultURL: "https://other.example", wantDefault: "https://other.example", wantDefaultWrites: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			stored := &spyStore{values: map[string]string{"https://tinker.example": "saved-token"}, defaultURL: test.defaultURL}
			var archiveNames, calls []string
			var out, stderr bytes.Buffer
			code := runWith([]string{"--server", "https://tinker.example", "deploy", project}, &out, &stderr, runnerDeps{store: stored, newClient: tokenClient(successfulDeployTransport(t, "saved-token", &archiveNames, &calls))})
			if code != 0 || stored.defaultURL != test.wantDefault || stored.setDefaultCalls != test.wantDefaultWrites || len(archiveNames) == 0 {
				t.Fatalf("code=%d default=%q setCalls=%d archive=%v out=%q err=%q", code, stored.defaultURL, stored.setDefaultCalls, archiveNames, out.String(), stderr.String())
			}
		})
	}
}

func TestHumanDeployDefaultStoreFailuresStopBeforeAppMutation(t *testing.T) {
	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, "dist"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "dist", "index.html"), []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "tinker.yaml"), []byte("version: 1\nname: demo\nbuild:\n  output: dist\naccess:\n  mode: private\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name     string
		stored   *spyStore
		message  string
		setCalls int
	}{
		{name: "write failure", stored: &spyStore{values: map[string]string{"https://tinker.example": "saved-token"}, setErr: client.ErrStore}, message: "Server could not be saved.", setCalls: 1},
		{name: "corrupt read", stored: &spyStore{values: map[string]string{"https://tinker.example": "saved-token"}, defaultErr: client.ErrStore}, message: "Saved server could not be read.", setCalls: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			mutated := false
			deps := runnerDeps{store: test.stored, newClient: tokenClient(tokenRoundTrip(func(r *http.Request) (*http.Response, error) {
				switch r.URL.Path {
				case "/api/v1/version":
					return jsonResponse(r, http.StatusOK, `{"api_version":1}`), nil
				case "/api/v1/whoami":
					return jsonResponse(r, http.StatusOK, `{"email":"dev@example.test"}`), nil
				default:
					mutated = true
					return nil, context.Canceled
				}
			}))}
			var out, stderr bytes.Buffer
			if code := runWith([]string{"--server", "https://tinker.example", "deploy", project}, &out, &stderr, deps); code != 1 || !strings.Contains(stderr.String(), test.message) || mutated || test.stored.setDefaultCalls != test.setCalls {
				t.Fatalf("code=%d mutated=%v setCalls=%d out=%q err=%q", code, mutated, test.stored.setDefaultCalls, out.String(), stderr.String())
			}
		})
	}
}

func stringMustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestUnknownOrEmptyCommandDoesNotReadDefaultServer(t *testing.T) {
	deps := runnerDeps{store: &spyStore{values: map[string]string{}, defaultErr: client.ErrStore}}
	for _, argv := range [][]string{nil, {"unknown"}} {
		var out, stderr bytes.Buffer
		if code := runWith(argv, &out, &stderr, deps); code != 2 || stderr.String() != "usage: tinker [--json] [--server URL] <command>\n" {
			t.Fatalf("argv=%v code=%d out=%q err=%q", argv, code, out.String(), stderr.String())
		}
	}
}

func TestMalformedRecognizedCommandsDoNotStartServerSetup(t *testing.T) {
	for _, argv := range [][]string{
		{"apps", "nonsense"},
		{"access", "get"},
		{"access", "set", "demo", "--wrong", "policy.json"},
		{"tokens", "create", "demo", "--scope", "invalid", "--expires-in", "60"},
		{"tokens", "list", "bad_slug"},
		{"releases", "list", "demo", "extra"},
		{"releases", "list", "bad_slug"},
		{"rollback", "demo", "deployment"},
		{"deploy", "one", "two"},
	} {
		stored := &spyStore{values: map[string]string{}, defaultErr: client.ErrStore}
		prompt := &loginPrompt{values: []string{"https://tinker.example"}}
		called := false
		deps := runnerDeps{store: stored, prompt: prompt, newClient: tokenClient(tokenRoundTrip(func(*http.Request) (*http.Response, error) {
			called = true
			return nil, context.Canceled
		}))}
		var out, stderr bytes.Buffer
		if code := runWith(argv, &out, &stderr, deps); code != 2 || stderr.String() != "usage: tinker [--json] [--server URL] <command>\n" || stored.defaultReads != 0 || prompt.asked != 0 || called {
			t.Fatalf("argv=%v code=%d out=%q err=%q reads=%d asked=%d called=%v", argv, code, out.String(), stderr.String(), stored.defaultReads, prompt.asked, called)
		}
	}
}

func TestLogoutRevokesOnlyCurrentBearerAndHandlesFailures(t *testing.T) {
	for _, tt := range []struct {
		name      string
		status    int
		wantCode  int
		wantOut   string
		keepToken bool
		wantCalls int
	}{
		{"success", http.StatusNoContent, 0, "Logged out.\n", false, 1},
		{"unauthorized cleanup", http.StatusUnauthorized, 0, "Logged out.\n", false, 1},
		{"server failure keeps token", http.StatusServiceUnavailable, 1, "", true, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stored := client.MemoryStore{"https://tinker.example": "current-token"}
			if err := stored.SetDefaultServer("https://tinker.example"); err != nil {
				t.Fatal(err)
			}
			calls := 0
			deps := runnerDeps{store: stored, newClient: tokenClient(tokenRoundTrip(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Method != http.MethodPost || r.URL.Path != "/api/v1/auth/logout" || r.Header.Get("Authorization") != "Bearer current-token" {
					t.Fatalf("unexpected logout request %s %s %q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
				}
				return jsonResponse(r, tt.status, `{}`), nil
			}))}
			var out, stderr bytes.Buffer
			code := runWith([]string{"logout"}, &out, &stderr, deps)
			if code != tt.wantCode || out.String() != tt.wantOut || calls != tt.wantCalls {
				t.Fatalf("code=%d out=%q err=%q calls=%d", code, out.String(), stderr.String(), calls)
			}
			_, exists := stored["https://tinker.example"]
			if exists != tt.keepToken {
				t.Fatalf("token exists=%v, want %v", exists, tt.keepToken)
			}
			if got, err := stored.DefaultServer(); err != nil || got != "https://tinker.example" {
				t.Fatalf("default = %q, %v", got, err)
			}
		})
	}
}

func TestLogoutWithoutLocalTokenIsIdempotent(t *testing.T) {
	stored := client.MemoryStore{}
	if err := stored.SetDefaultServer("https://tinker.example"); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	if code := runWith([]string{"logout"}, &out, &stderr, runnerDeps{store: stored}); code != 0 || out.String() != "Already logged out.\n" || stderr.Len() != 0 {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), stderr.String())
	}
}

func TestLogoutTransportDeleteAndUnsafeCredentialFailuresKeepLocalState(t *testing.T) {
	for _, tt := range []struct {
		name      string
		store     *spyStore
		transport http.RoundTripper
	}{
		{"transport", &spyStore{values: map[string]string{"https://tinker.example": "token"}, defaultURL: "https://tinker.example"}, tokenRoundTrip(func(*http.Request) (*http.Response, error) { return nil, context.DeadlineExceeded })},
		{"local delete", &spyStore{values: map[string]string{"https://tinker.example": "token"}, defaultURL: "https://tinker.example", deleteErr: client.ErrStore}, tokenRoundTrip(func(r *http.Request) (*http.Response, error) { return jsonResponse(r, http.StatusNoContent, ""), nil })},
		{"unsafe credential", &spyStore{values: map[string]string{"https://tinker.example": "token"}, defaultURL: "https://tinker.example", getErr: client.ErrStore}, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			deps := runnerDeps{store: tt.store}
			if tt.transport != nil {
				deps.newClient = tokenClient(tt.transport)
			}
			var out, stderr bytes.Buffer
			if code := runWith([]string{"logout"}, &out, &stderr, deps); code != 1 || out.Len() != 0 {
				t.Fatalf("code=%d out=%q err=%q", code, out.String(), stderr.String())
			}
			if got := tt.store.values["https://tinker.example"]; got != "token" {
				t.Fatalf("credential changed to %q", got)
			}
		})
	}
}

func TestAccessSetRejectsInvalidPolicyBeforeNetwork(t *testing.T) {
	p := filepath.Join(t.TempDir(), "policy.json")
	if e := os.WriteFile(p, []byte("{"), 0600); e != nil {
		t.Fatal(e)
	}
	var out, err bytes.Buffer
	code := runWith([]string{"--server", "https://tinker.example", "access", "set", "app", "--file", p}, &out, &err, runnerDeps{store: client.MemoryStore{"https://tinker.example": "secret"}})
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
	deps := runnerDeps{store: client.MemoryStore{"https://tinker.example": "token"}, newClient: tokenClient(tokenRoundTrip(func(r *http.Request) (*http.Response, error) {
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
	if code := runWith([]string{"--server", "https://tinker.example", "access", "set", "app", "--file", p}, &out, &stderr, deps); code != 0 || !strings.Contains(putBody, `"expected_revision":7`) {
		t.Fatalf("code=%d out=%q err=%q body=%q", code, out.String(), stderr.String(), putBody)
	}
}

func TestAccessSetJSONBroadeningRequiresExplicitFileConfirmationBeforePUT(t *testing.T) {
	p := filepath.Join(t.TempDir(), "policy.json")
	if err := os.WriteFile(p, []byte(`{"mode":"private","allow":{"emails":["viewer@example.test"],"domains":[]}}`), 0600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	deps := runnerDeps{store: client.MemoryStore{"https://tinker.example": "token"}, newClient: tokenClient(tokenRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/apps/app/access" {
			t.Fatalf("unexpected mutation/request: %s %s", r.Method, r.URL.Path)
		}
		return jsonResponse(r, http.StatusOK, `{"mode":"private","revision":7,"allow":{"emails":[],"domains":[]}}`), nil
	}))}
	var out, stderr bytes.Buffer
	code := runWith([]string{"--json", "--server", "https://tinker.example", "access", "set", "app", "--file", p}, &out, &stderr, deps)
	if code != 1 || stderr.Len() != 0 || calls != 1 || !strings.Contains(out.String(), `"code":"confirmation_required"`) {
		t.Fatalf("code=%d calls=%d out=%q err=%q", code, calls, out.String(), stderr.String())
	}
}

func TestAccessSetInteractiveBroadeningConfirmsAndPreservesServerEnforcement(t *testing.T) {
	p := filepath.Join(t.TempDir(), "policy.json")
	if err := os.WriteFile(p, []byte(`{"mode":"private","allow":{"emails":["viewer@example.test"],"domains":[]}}`), 0600); err != nil {
		t.Fatal(err)
	}
	var putBody string
	prompt := &loginPrompt{values: []string{"yes"}}
	deps := runnerDeps{store: client.MemoryStore{"https://tinker.example": "token"}, prompt: prompt, newClient: tokenClient(tokenRoundTrip(func(r *http.Request) (*http.Response, error) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/apps/app/access":
			return jsonResponse(r, http.StatusOK, `{"mode":"private","revision":7,"allow":{"emails":[],"domains":[]}}`), nil
		case "PUT /api/v1/apps/app/access":
			body, _ := io.ReadAll(r.Body)
			putBody = string(body)
			return jsonResponse(r, http.StatusOK, `{}`), nil
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
			return nil, nil
		}
	}))}
	var out, stderr bytes.Buffer
	if code := runWith([]string{"--server", "https://tinker.example", "access", "set", "app", "--file", p}, &out, &stderr, deps); code != 0 || prompt.asked != 1 || !strings.Contains(putBody, `"confirm_broadening":true`) {
		t.Fatalf("code=%d asked=%d out=%q err=%q body=%q", code, prompt.asked, out.String(), stderr.String(), putBody)
	}
}

func TestAccessSetStrictlyRejectsReadOnlyRevisionBeforeNetwork(t *testing.T) {
	p := filepath.Join(t.TempDir(), "policy.json")
	if err := os.WriteFile(p, []byte(`{"mode":"private","revision":7,"allow":{"emails":[],"domains":[]}}`), 0600); err != nil {
		t.Fatal(err)
	}
	deps := runnerDeps{store: client.MemoryStore{"https://tinker.example": "token"}, newClient: tokenClient(tokenRoundTrip(func(r *http.Request) (*http.Response, error) {
		t.Fatal("policy validation made a network request")
		return nil, nil
	}))}
	var out, stderr bytes.Buffer
	code := runWith([]string{"--json", "--server", "https://tinker.example", "access", "set", "app", "--file", p}, &out, &stderr, deps)
	if code != 1 || stderr.Len() != 0 || !strings.Contains(out.String(), `"code":"invalid_policy"`) {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), stderr.String())
	}
}

func TestAccessSetStaleRevisionRemainsServerConflictWithoutPrompt(t *testing.T) {
	p := filepath.Join(t.TempDir(), "policy.json")
	if err := os.WriteFile(p, []byte(`{"mode":"private","expected_revision":6,"allow":{"emails":["viewer@example.test"],"domains":[]}}`), 0600); err != nil {
		t.Fatal(err)
	}
	prompt := &loginPrompt{values: []string{"yes"}}
	deps := runnerDeps{store: client.MemoryStore{"https://tinker.example": "token"}, prompt: prompt, newClient: tokenClient(tokenRoundTrip(func(r *http.Request) (*http.Response, error) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/apps/app/access":
			return jsonResponse(r, http.StatusOK, `{"mode":"private","revision":7,"allow":{"emails":[],"domains":[]}}`), nil
		case "PUT /api/v1/apps/app/access":
			return jsonResponse(r, http.StatusConflict, `{"error":{"code":"conflict","message":"Conflict.","request_id":"req_0123456789abcdef01234567"}}`), nil
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
			return nil, nil
		}
	}))}
	var out, stderr bytes.Buffer
	code := runWith([]string{"--json", "--server", "https://tinker.example", "access", "set", "app", "--file", p}, &out, &stderr, deps)
	if code != 1 || prompt.asked != 0 || !strings.Contains(out.String(), `"code":"policy_conflict"`) {
		t.Fatalf("code=%d asked=%d out=%q err=%q", code, prompt.asked, out.String(), stderr.String())
	}
}

func TestTokenCreatePrintsSecretExactlyOnce(t *testing.T) {
	var gotBody string
	deps := runnerDeps{
		store: client.MemoryStore{"https://tinker.example": "control-token"},
		newClient: tokenClient(tokenRoundTrip(func(r *http.Request) (*http.Response, error) {
			if r.Method != "POST" || r.URL.Path != "/api/v1/apps/demo/tokens" || r.Header.Get("Authorization") != "Bearer control-token" || r.Header.Get("Idempotency-Key") == "" {
				t.Fatalf("unexpected request: %s %s %#v", r.Method, r.URL, r.Header)
			}
			body, _ := io.ReadAll(r.Body)
			gotBody = string(body)
			return &http.Response{StatusCode: 201, Body: io.NopCloser(strings.NewReader(`{"id":"tok_1","token":"tinker_display_once","scopes":["app:read"],"expires_at":"2030-01-01T00:00:00Z"}`)), Header: make(http.Header), Request: r}, nil
		})),
	}
	var out, err bytes.Buffer
	code := runWith([]string{"--server", "https://tinker.example", "tokens", "create", "demo", "--scope", "app:read", "--expires-in", "3600"}, &out, &err, deps)
	if code != 0 || err.Len() != 0 || out.String() != "tinker_display_once\n" {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), err.String())
	}
	if gotBody != "{\"scopes\":[\"app:read\"],\"expires_in_seconds\":3600}\n" {
		t.Fatalf("unexpected body %q", gotBody)
	}
}

func TestTokenListNeverPrintsSecret(t *testing.T) {
	deps := runnerDeps{
		store: client.MemoryStore{"https://tinker.example": "control-token"},
		newClient: tokenClient(tokenRoundTrip(func(r *http.Request) (*http.Response, error) {
			if r.Method != "GET" || r.URL.Path != "/api/v1/apps/demo/tokens" {
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL)
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`[{"id":"tok_1","scopes":["app:read"],"expires_at":"2030-01-01T00:00:00Z","last_used_at":null,"revoked":false}]`)), Header: make(http.Header), Request: r}, nil
		})),
	}
	var out, err bytes.Buffer
	code := runWith([]string{"--json", "--server", "https://tinker.example", "tokens", "list", "demo"}, &out, &err, deps)
	if code != 0 || err.Len() != 0 || out.String() != "[{\"id\":\"tok_1\",\"scopes\":[\"app:read\"],\"expires_at\":\"2030-01-01T00:00:00Z\",\"last_used_at\":null,\"revoked\":false}]\n" {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), err.String())
	}
	if strings.Contains(out.String(), "tinker_") || strings.Contains(out.String(), "control-token") {
		t.Fatal("credential leaked in token list")
	}
}

func TestTokenRevokeUsesDeleteAndDoesNotExposeCredential(t *testing.T) {
	deps := runnerDeps{
		store: client.MemoryStore{"https://tinker.example": "control-token"},
		newClient: tokenClient(tokenRoundTrip(func(r *http.Request) (*http.Response, error) {
			if r.Method != "DELETE" || r.URL.Path != "/api/v1/apps/demo/tokens/tok_1" || r.Header.Get("Idempotency-Key") == "" {
				t.Fatalf("unexpected request: %s %s %#v", r.Method, r.URL, r.Header)
			}
			return &http.Response{StatusCode: 204, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header), Request: r}, nil
		})),
	}
	var out, err bytes.Buffer
	code := runWith([]string{"--json", "--server", "https://tinker.example", "tokens", "revoke", "demo", "tok_1"}, &out, &err, deps)
	if code != 0 || err.Len() != 0 || out.String() != "{\"id\":\"tok_1\",\"revoked\":true}\n" {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), err.String())
	}
}

func TestTokenCreateRejectsMalformedInputBeforeNetwork(t *testing.T) {
	called := false
	deps := runnerDeps{
		store: client.MemoryStore{"https://tinker.example": "control-token"},
		newClient: tokenClient(tokenRoundTrip(func(*http.Request) (*http.Response, error) {
			called = true
			return nil, context.Canceled
		})),
	}
	var out, err bytes.Buffer
	code := runWith([]string{"--server", "https://tinker.example", "tokens", "create", "demo", "--scope", "app:read", "--expires-in", "1"}, &out, &err, deps)
	if code != 2 || called {
		t.Fatalf("code=%d called=%v out=%q err=%q", code, called, out.String(), err.String())
	}
}

func TestReleasesListUsesInjectedClientAndWritesDeterministicJSON(t *testing.T) {
	deps := runnerDeps{
		store: client.MemoryStore{"https://tinker.example": "control-token"},
		newClient: tokenClient(tokenRoundTrip(func(r *http.Request) (*http.Response, error) {
			if r.Method != "GET" || r.URL.Path != "/api/v1/apps/demo/releases" || r.Header.Get("Authorization") != "Bearer control-token" {
				t.Fatalf("unexpected request: %s %s %#v", r.Method, r.URL, r.Header)
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`[{"id":"dep_1","release_hash":"abc","state":"active","created_at":"2030-01-01T00:00:00Z","verified_at":"2030-01-01T00:01:00Z","activated_at":"2030-01-01T00:02:00Z"}]`)), Header: make(http.Header), Request: r}, nil
		})),
	}
	var out, err bytes.Buffer
	code := runWith([]string{"--json", "--server", "https://tinker.example", "releases", "list", "demo"}, &out, &err, deps)
	want := "[{\"id\":\"dep_1\",\"release_hash\":\"abc\",\"state\":\"active\",\"created_at\":\"2030-01-01T00:00:00Z\",\"verified_at\":\"2030-01-01T00:01:00Z\",\"activated_at\":\"2030-01-01T00:02:00Z\"}]\n"
	if code != 0 || err.Len() != 0 || out.String() != want {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), err.String())
	}
}

func TestReleasesUnauthorizedDoesNotExposeCredentialOrServerBody(t *testing.T) {
	deps := runnerDeps{
		store: client.MemoryStore{"https://tinker.example": "control-token"},
		newClient: tokenClient(tokenRoundTrip(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 403, Body: io.NopCloser(strings.NewReader(`{"id":"other-app","token":"server-secret"}`)), Header: make(http.Header)}, nil
		})),
	}
	var out, err bytes.Buffer
	code := runWith([]string{"--json", "--server", "https://tinker.example", "releases", "demo"}, &out, &err, deps)
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
		store: client.MemoryStore{"https://tinker.example": "control-token"},
		newClient: tokenClient(tokenRoundTrip(func(*http.Request) (*http.Response, error) {
			called = true
			return nil, context.Canceled
		})),
	}
	var out, err bytes.Buffer
	code := runWith([]string{"--server", "https://tinker.example", "apps", "delete", "demo", "--confirm", "delete:other"}, &out, &err, deps)
	if code != 2 || called {
		t.Fatalf("code=%d called=%v out=%q err=%q", code, called, out.String(), err.String())
	}
}

func TestAppDeleteUsesBoundConfirmationAndDoesNotLeakUnauthorizedCredential(t *testing.T) {
	deps := runnerDeps{
		store: client.MemoryStore{"https://tinker.example": "control-token"},
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
	code := runWith([]string{"--json", "--server", "https://tinker.example", "apps", "delete", "demo", "--confirm", "delete:demo"}, &out, &err, deps)
	if code != 1 || err.Len() != 0 || out.String() != "{\"valid\":false,\"error\":{\"code\":\"not_authorized\",\"message\":\"Request not authorized.\"}}\n" {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), err.String())
	}
	if strings.Contains(out.String(), "control-token") || strings.Contains(out.String(), "forbidden") {
		t.Fatal("credential or server detail leaked")
	}
}

func TestAppDeleteSuccessWritesStableResult(t *testing.T) {
	deps := runnerDeps{
		store: client.MemoryStore{"https://tinker.example": "control-token"},
		newClient: tokenClient(tokenRoundTrip(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 204, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header), Request: r}, nil
		})),
	}
	var out, err bytes.Buffer
	code := runWith([]string{"--json", "--server", "https://tinker.example", "apps", "delete", "demo", "--confirm", "delete:demo"}, &out, &err, deps)
	if code != 0 || err.Len() != 0 || out.String() != "{\"slug\":\"demo\",\"deleted\":true}\n" {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), err.String())
	}
}

func TestDeployUsesFreshIdempotencyKeyForEachInvocationAndStagesArchive(t *testing.T) {
	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, "dist"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "tinker.yaml"), []byte("version: 1\nname: demo\nbuild:\n  output: dist\naccess:\n  mode: private\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "dist", "index.html"), []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	var keys []string
	deployments := 0
	appCreated := false
	deps := runnerDeps{
		store: client.MemoryStore{"https://tinker.example": "control-token"},
		newClient: tokenClient(tokenRoundTrip(func(r *http.Request) (*http.Response, error) {
			if r.Method == "GET" && r.URL.Path == "/api/v1/version" {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"api_version":1}`)), Header: make(http.Header), Request: r}, nil
			}
			if r.Method == "GET" && r.URL.Path == "/api/v1/whoami" {
				if r.Header.Get("Authorization") != "Bearer control-token" {
					t.Fatalf("credential verification lost bearer: %q", r.Header.Get("Authorization"))
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"email":"dev@example.test","api_version":1}`)), Header: make(http.Header), Request: r}, nil
			}
			if r.URL.Host == "demo.tinker.example" && r.URL.Path == "/" {
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
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"deployment_id":"d","url":"https://demo.tinker.example/","domain":"tinker.example","policy_ready":true,"tls_ready":true,"anonymous_denied":true,"authenticated_healthy":true}`)), Header: make(http.Header), Request: r}, nil
		})),
	}
	for range 2 {
		var out, stderr bytes.Buffer
		if code := runWith([]string{"--server", "https://tinker.example", "deploy", project}, &out, &stderr, deps); code != 0 {
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
