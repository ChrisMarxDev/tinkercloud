package update

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type unknownHostDoer func(*http.Request) (*http.Response, error)

func (f unknownHostDoer) Do(r *http.Request) (*http.Response, error) { return f(r) }

func TestUnknownHostDenyHealthFailsClosed(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusUnauthorized, http.StatusMovedPermanently} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			h := UnknownHostDenyHealth{HTTPHealth: HTTPHealth{URL: "https://unknown.example/_tinker/api/v1/capabilities", Client: unknownHostDoer(func(r *http.Request) (*http.Response, error) {
				body := `{"error":{"code":"not_found","message":"This request is not authorized.","request_id":"req_0123456789abcdef01234567"}}`
				if !gatewayRequestID.MatchString("req_0123456789abcdef01234567") {
					t.Fatal("fixture request id invalid")
				}
				header := make(http.Header)
				header.Set("Cache-Control", "no-store")
				header.Set("X-Content-Type-Options", "nosniff")
				header.Set("Content-Type", "application/json")
				header.Set("X-Request-ID", "req_0123456789abcdef01234567")
				resp := &http.Response{StatusCode: status, Request: r, Body: io.NopCloser(strings.NewReader(body)), Header: header}
				if status == http.StatusMovedPermanently {
					resp.Header = http.Header{"Location": []string{"https://elsewhere"}}
				}
				return resp, nil
			})}}
			err := h.Check(context.Background())
			if (status == http.StatusNotFound) != (err == nil) {
				t.Fatalf("status %d: %v", status, err)
			}
		})
	}
}
