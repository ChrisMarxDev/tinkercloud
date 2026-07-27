package update

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPHealthAndAnonymousDenial(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthy":
			w.WriteHeader(http.StatusNoContent)
		case "/private":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Request-ID", "req_0123456789abcdef01234567")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"code":"not_authorized","message":"This request is not authorized.","request_id":"req_0123456789abcdef01234567"}}`))
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer s.Close()
	client := s.Client()
	if err := (HTTPHealth{URL: s.URL + "/healthy", Client: client}).Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := (AnonymousDenyHealth{HTTPHealth: HTTPHealth{URL: s.URL + "/private", Client: client}}).Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := (AnonymousDenyHealth{HTTPHealth: HTTPHealth{URL: s.URL + "/public", Client: client}}).Check(context.Background()); err != ErrHealth {
		t.Fatal(err)
	}
}

type healthDoer func(*http.Request) (*http.Response, error)

func (f healthDoer) Do(r *http.Request) (*http.Response, error) { return f(r) }

func denialResponse(r *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header: http.Header{
			"Content-Type":           []string{"application/json; charset=utf-8"},
			"Cache-Control":          []string{"no-store"},
			"X-Content-Type-Options": []string{"nosniff"},
			"X-Request-ID":           []string{"req_0123456789abcdef01234567"},
		},
		Request: r,
		Body:    io.NopCloser(strings.NewReader(body)),
	}
}

func TestAnonymousDenyHealthRejectsBrokenOrPublicGatewayEvidence(t *testing.T) {
	valid := `{"error":{"code":"not_authorized","message":"This request is not authorized.","request_id":"req_0123456789abcdef01234567"}}`
	for name, do := range map[string]healthDoer{
		"missing app 404": func(r *http.Request) (*http.Response, error) {
			return denialResponse(r, http.StatusNotFound, valid), nil
		},
		"public content 2xx": func(r *http.Request) (*http.Response, error) {
			return denialResponse(r, http.StatusOK, `public release bytes`), nil
		},
		"malformed denial": func(r *http.Request) (*http.Response, error) {
			return denialResponse(r, http.StatusUnauthorized, `{"error":{"code":"not_found"}}`), nil
		},
		"dependency failure": func(*http.Request) (*http.Response, error) { return nil, errors.New("dial failed") },
		"missing evidence": func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusUnauthorized, Request: r, Body: io.NopCloser(strings.NewReader(""))}, nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := (AnonymousDenyHealth{HTTPHealth: HTTPHealth{URL: "https://payroll.apps.example.test/", Client: do}}).Check(context.Background()); err != ErrHealth {
				t.Fatalf("got %v, want ErrHealth", err)
			}
		})
	}
}

func TestAnonymousDenyHealthFailsOnTimeout(t *testing.T) {
	client := healthDoer(func(r *http.Request) (*http.Response, error) {
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	if err := (AnonymousDenyHealth{HTTPHealth: HTTPHealth{URL: "https://payroll.apps.example.test/", Client: client, Timeout: time.Millisecond}}).Check(context.Background()); err != ErrHealth {
		t.Fatalf("got %v, want ErrHealth", err)
	}
}
