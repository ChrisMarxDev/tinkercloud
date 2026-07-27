package appapi

import (
	"github.com/tinyhost/tiny/internal/appauth"
	"github.com/tinyhost/tiny/internal/kv"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMutationRequiresExactOrigin(t *testing.T) {
	d := Dispatcher{KV: kv.New(kv.DefaultLimits(), nil), AppSlug: func(appauth.AuthorizationContext) string { return "demo" }}
	a := authFor(t, "a")
	r := httptest.NewRequest(http.MethodPut, "/_tiny/api/v1/kv/x", strings.NewReader(`{"value":1}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	d.Dispatch(a, w, r)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
	r.Header.Set("Origin", "https://evil.test")
	w = httptest.NewRecorder()
	d.Dispatch(a, w, r)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
}
