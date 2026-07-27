package live

import (
	"net/http/httptest"
	"testing"
)

func TestSameOriginRequiresExactResolvedHost(t *testing.T) {
	for _, tc := range []struct {
		origin, host string
		want         bool
	}{
		{"https://app.example.test", "app.example.test", true},
		{"https://app.example.test:8443", "app.example.test:8443", true},
		{"", "app.example.test", false},
		{"https://other.example.test", "app.example.test", false},
		{"https://app.example.test", "app.example.test:8443", false},
	} {
		r := httptest.NewRequest("GET", "https://"+tc.host+"/_tiny/ws/v1", nil)
		r.Host = tc.host
		r.Header.Set("Origin", tc.origin)
		if got := SameOrigin(r, nil); got != tc.want {
			t.Fatalf("%q/%q got %v", tc.origin, tc.host, got)
		}
	}
}
