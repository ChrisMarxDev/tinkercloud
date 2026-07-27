package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEnsureAppCreatesMissingOwnedSlug(t *testing.T) {
	requests := 0
	c := New("https://tiny.test", "secret")
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

func TestEnsureAppForeignSlugConflictRemainsDenied(t *testing.T) {
	requests := 0
	c := New("https://tiny.test", "secret")
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
			w.Write([]byte(`{"token":"secret","email":"a@example.com","api_version":1}`))
		}
	}))
	defer s.Close()
	p := &prompt{[]string{"a@example.com", "123456"}}
	o, e := Login(context.Background(), s.URL, p)
	if e != nil || o.Token != "secret" {
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
	c := New("https://tiny.test", "control-token")
	c.HTTP = &http.Client{Transport: rt(func(r *http.Request) (*http.Response, error) {
		requests = append(requests, r)
		switch r.Method {
		case "GET":
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`[{"id":"tok_1","scopes":["app:read"],"expires_at":"2030-01-01T00:00:00Z","last_used_at":null,"revoked":false}]`)), Header: make(http.Header), Request: r}, nil
		case "POST":
			return &http.Response{StatusCode: 201, Body: io.NopCloser(strings.NewReader(`{"id":"tok_2","token":"tiny_once","scopes":["app:read"],"expires_at":"2030-01-01T00:00:00Z"}`)), Header: make(http.Header), Request: r}, nil
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
	if err != nil || created.Token != "tiny_once" {
		t.Fatal(created, err)
	}
	if err = c.RevokeToken(context.Background(), "demo/name", "tok_2", "key2"); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 3 || requests[0].URL.EscapedPath() != "/api/v1/apps/demo%2Fname/tokens" || requests[1].Header.Get("Idempotency-Key") != "key" || requests[2].URL.EscapedPath() != "/api/v1/apps/demo%2Fname/tokens/tok_2" || requests[2].Header.Get("Idempotency-Key") != "key2" {
		t.Fatalf("unexpected paths=%q,%q,%q keys=%q,%q", requests[0].URL.EscapedPath(), requests[1].URL.EscapedPath(), requests[2].URL.EscapedPath(), requests[1].Header.Get("Idempotency-Key"), requests[2].Header.Get("Idempotency-Key"))
	}
}
