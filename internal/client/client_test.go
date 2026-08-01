package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEnsureAppCreatesMissingOwnedSlug(t *testing.T) {
	requests := 0
	c := New("https://tinker.test", "secret")
	c.HTTP = &http.Client{Transport: rt(func(r *http.Request) (*http.Response, error) {
		requests++
		switch requests {
		case 1:
			if r.Method != "GET" || r.URL.Path != "/api/v1/apps" {
				t.Fatal(r.Method, r.URL.Path)
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`[]`)), Header: make(http.Header), Request: r}, nil
		case 2:
			if r.Method != "POST" || r.URL.Path != "/api/v1/apps" || r.Header.Get("Idempotency-Key") == "" {
				t.Fatal(r.Method, r.URL.Path, r.Header)
			}
			return &http.Response{StatusCode: 201, Body: io.NopCloser(strings.NewReader(`{"slug":"demo"}`)), Header: make(http.Header), Request: r}, nil
		default:
			t.Fatal("unexpected request")
			return nil, nil
		}
	})}
	if err := c.EnsureApp(context.Background(), "demo"); err != nil {
		t.Fatal(err)
	}
}

func TestAccessReadsOnlyValidCurrentPolicy(t *testing.T) {
	c := New("https://tinker.test", "secret")
	c.HTTP = &http.Client{Transport: rt(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/apps/demo/access" {
			t.Fatalf("request %s %s", r.Method, r.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"mode":"public","revision":2,"allow":{"emails":["viewer@example.test"],"domains":[]}}`)), Header: make(http.Header), Request: r}, nil
	})}
	policy, err := c.Access(context.Background(), "demo")
	if err != nil || policy.Mode != "public" || policy.Revision != 2 || len(policy.Allow.Emails) != 1 {
		t.Fatalf("policy=%+v err=%v", policy, err)
	}
}

func TestReleasedClientSendsCompatibilityHeaders(t *testing.T) {
	old := BuildVersion
	BuildVersion = "0.1.0"
	t.Cleanup(func() { BuildVersion = old })
	c := New("https://tinker.test", "secret")
	c.HTTP = &http.Client{Transport: rt(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("X-Tinker-CLI-Version") != "0.1.0" || r.Header.Get("X-Tinker-Control-API-Version") != "1" {
			t.Fatalf("compatibility headers = %#v", r.Header)
		}
		return &http.Response{StatusCode: http.StatusNoContent, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header), Request: r}, nil
	})}
	if err := c.Do(context.Background(), http.MethodPost, "/api/v1/auth/logout", "", nil, nil); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureAppForeignSlugConflictRemainsDenied(t *testing.T) {
	requests := 0
	c := New("https://tinker.test", "secret")
	c.HTTP = &http.Client{Transport: rt(func(r *http.Request) (*http.Response, error) {
		requests++
		switch requests {
		case 1, 3:
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`[]`)), Header: make(http.Header), Request: r}, nil
		case 2:
			return &http.Response{StatusCode: 409, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"conflict"}}`)), Header: make(http.Header), Request: r}, nil
		default:
			t.Fatal("unexpected request")
			return nil, nil
		}
	})}
	if err := c.EnsureApp(context.Background(), "foreign"); !errors.Is(err, ErrAppUnavailable) {
		t.Fatalf("foreign conflict result = %v", err)
	}
}

type prompt struct{ v []string }

func (p *prompt) Ask(string) (string, error) { x := p.v[0]; p.v = p.v[1:]; return x, nil }
func TestLoginSuccess(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/version":
			w.Write([]byte(`{"api_version":1}`))
		case "/api/v1/auth/otp":
			w.Write([]byte(`{"transaction":"x"}`))
		case "/api/v1/auth/verify":
			w.Write([]byte(`{"token":"secret","api_version":1}`))
		case "/api/v1/whoami":
			if r.Header.Get("Authorization") != "Bearer secret" {
				t.Fatal("whoami was not authenticated")
			}
			w.Write([]byte(`{"email":"a@example.com"}`))
		}
	}))
	defer s.Close()
	p := &prompt{[]string{"a@example.com", "123456"}}
	o, e := Login(context.Background(), s.URL, p)
	if e != nil || o.Token != "secret" || o.Email != "a@example.com" {
		t.Fatal(o, e)
	}
}
func TestLoginVersionMismatch(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"api_version":2}`)) }))
	defer s.Close()
	_, e := Login(context.Background(), s.URL, &prompt{})
	if e == nil {
		t.Fatal("accepted incompatible API")
	}
}

func TestDoRateLimitDoesNotExposeResponseBody(t *testing.T) {
	c := New("https://tinker.test", "control-token")
	c.HTTP = &http.Client{Transport: rt(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusTooManyRequests, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"a@example.test control-token"}}`)), Header: make(http.Header), Request: r}, nil
	})}
	err := c.Do(context.Background(), "POST", "/api/v1/auth/otp", "", map[string]string{"email": "a@example.test"}, nil)
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(err.Error(), "example") || strings.Contains(err.Error(), "token") {
		t.Fatalf("rate limit detail leaked: %v", err)
	}
}

func TestVerifyLoginSessionDependencyFailureIsNotUnauthorized(t *testing.T) {
	c := New("https://tinker.test", "saved-token")
	c.HTTP = &http.Client{Transport: rt(func(*http.Request) (*http.Response, error) {
		return nil, context.DeadlineExceeded
	})}
	_, err := VerifyLoginSession(context.Background(), c)
	if err == nil || errors.Is(err, ErrUnauthorized) {
		t.Fatalf("dependency error must fail closed, got %v", err)
	}
}

func TestVerifyServerCompatibilityFailsClosed(t *testing.T) {
	for _, tt := range []struct {
		name   string
		status int
		body   string
		err    error
	}{
		{"valid", http.StatusOK, `{"api_version":1}`, nil},
		{"wrong version", http.StatusOK, `{"api_version":2}`, nil},
		{"malformed", http.StatusOK, `{`, nil},
		{"redirect", http.StatusFound, ``, nil},
		{"dependency", 0, ``, context.DeadlineExceeded},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := New("https://tinker.test", "")
			c.HTTP = &http.Client{Transport: rt(func(r *http.Request) (*http.Response, error) {
				if r.Method != http.MethodGet || r.URL.Path != "/api/v1/version" || r.Header.Get("Authorization") != "" {
					t.Fatalf("unexpected proof request %s %s", r.Method, r.URL.Path)
				}
				if tt.err != nil {
					return nil, tt.err
				}
				return &http.Response{StatusCode: tt.status, Body: io.NopCloser(strings.NewReader(tt.body)), Header: make(http.Header), Request: r}, nil
			})}
			err := VerifyServerCompatibility(context.Background(), c)
			if tt.name == "valid" {
				if err != nil {
					t.Fatal(err)
				}
			} else if !errors.Is(err, ErrIncompatibleServer) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}
func TestCrossOriginRedirectDenied(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://evil.example", http.StatusFound)
	}))
	defer s.Close()
	c := New(s.URL, "")
	if e := c.Do(context.Background(), "GET", "/api/v1/version", "", nil, &struct{}{}); e == nil {
		t.Fatal("redirect accepted")
	}
}

func TestTokenLifecycleUsesBoundPathsAndNeverDecodesSecretFromList(t *testing.T) {
	var requests []*http.Request
	c := New("https://tinker.test", "control-token")
	c.HTTP = &http.Client{Transport: rt(func(r *http.Request) (*http.Response, error) {
		requests = append(requests, r)
		switch r.Method {
		case "GET":
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`[{"id":"tok_1","scopes":["app:read"],"expires_at":"2030-01-01T00:00:00Z","last_used_at":null,"revoked":false}]`)), Header: make(http.Header), Request: r}, nil
		case "POST":
			return &http.Response{StatusCode: 201, Body: io.NopCloser(strings.NewReader(`{"id":"tok_2","token":"tinker_once","scopes":["app:read"],"expires_at":"2030-01-01T00:00:00Z"}`)), Header: make(http.Header), Request: r}, nil
		case "DELETE":
			return &http.Response{StatusCode: 204, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header), Request: r}, nil
		default:
			t.Fatalf("unexpected method %s", r.Method)
			return nil, nil
		}
	})}
	items, err := c.ListTokens(context.Background(), "demo/name")
	if err != nil || len(items) != 1 || items[0].ID != "tok_1" || items[0].LastUsedAt != nil {
		t.Fatal(items, err)
	}
	created, err := c.CreateToken(context.Background(), "demo/name", TokenInput{Scopes: []string{"app:read"}, ExpiresInSeconds: 3600}, "key")
	if err != nil || created.Token != "tinker_once" {
		t.Fatal(created, err)
	}
	if err = c.RevokeToken(context.Background(), "demo/name", "tok_2", "key2"); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 3 || requests[0].URL.EscapedPath() != "/api/v1/apps/demo%2Fname/tokens" || requests[1].Header.Get("Idempotency-Key") != "key" || requests[2].URL.EscapedPath() != "/api/v1/apps/demo%2Fname/tokens/tok_2" || requests[2].Header.Get("Idempotency-Key") != "key2" {
		t.Fatalf("unexpected paths=%q,%q,%q keys=%q,%q", requests[0].URL.EscapedPath(), requests[1].URL.EscapedPath(), requests[2].URL.EscapedPath(), requests[1].Header.Get("Idempotency-Key"), requests[2].Header.Get("Idempotency-Key"))
	}
}

func TestDataClientUsesBoundedControlPathsAndVersions(t *testing.T) {
	var requests []*http.Request
	c := New("https://tinker.test", "control-token")
	c.HTTP = &http.Client{Transport: rt(func(r *http.Request) (*http.Response, error) {
		requests = append(requests, r)
		switch r.Method {
		case "GET":
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"entries":[]}`)), Header: make(http.Header), Request: r}, nil
		case "PUT":
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `"expected_version":4`) || !strings.Contains(string(body), `"value":{"done":true}`) {
				t.Fatalf("update body = %s", body)
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"key":"a/b","value":{"done":true},"version":5}`)), Header: make(http.Header), Request: r}, nil
		default:
			t.Fatalf("unexpected method %s", r.Method)
			return nil, nil
		}
	})}
	if _, err := c.ListDataKV(context.Background(), "owned/app", "settings/", "cursor", 10); err != nil {
		t.Fatal(err)
	}
	expected := uint64(4)
	if _, err := c.SetDataKV(context.Background(), "owned/app", "a/b", json.RawMessage(`{"done":true}`), &expected, "idem"); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 2 || requests[0].URL.EscapedPath() != "/api/v1/apps/owned%2Fapp/data/kv" || requests[0].URL.Query().Get("prefix") != "settings/" || requests[0].URL.Query().Get("cursor") != "cursor" || requests[1].URL.EscapedPath() != "/api/v1/apps/owned%2Fapp/data/kv/a%2Fb" || requests[1].Header.Get("Idempotency-Key") != "idem" || requests[1].Header.Get("Authorization") != "Bearer control-token" {
		t.Fatalf("unexpected requests: %#v", requests)
	}
}

func TestControlClientKeepsQuotaDistinctFromRequestRateLimit(t *testing.T) {
	for _, test := range []struct {
		body string
		want error
	}{
		{`{"error":{"code":"quota_exceeded","message":"safe"}}`, ErrQuotaExceeded},
		{`{"error":{"code":"rate_limited","message":"safe"}}`, ErrRateLimited},
		{`not-json`, ErrRateLimited},
	} {
		c := New("https://tinker.test", "control-token")
		c.HTTP = &http.Client{Transport: rt(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusTooManyRequests, Body: io.NopCloser(strings.NewReader(test.body)), Header: make(http.Header), Request: r}, nil
		})}
		if err := c.Do(context.Background(), http.MethodGet, "/api/v1/apps/demo/data/kv", "", nil, nil); !errors.Is(err, test.want) {
			t.Fatalf("body=%q error=%v want=%v", test.body, err, test.want)
		}
	}
}
