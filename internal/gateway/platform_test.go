package gateway_test

import (
	"github.com/tinyhost/tiny/internal/config"
	"github.com/tinyhost/tiny/internal/gateway"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPlatformHostUsesOnlyPlatformHandler(t *testing.T) {
	called := false
	g := gateway.Gateway{Config: config.Config{PlatformHost: "tiny.test", AppSuffix: "apps.tiny.test"}, Platform: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true; w.WriteHeader(204) })}
	r := httptest.NewRequest("GET", "http://tiny.test/api/v1/whoami", nil)
	r.Host = "tiny.test"
	w := httptest.NewRecorder()
	g.ServeHTTP(w, r)
	if !called || w.Code != 204 {
		t.Fatal(called, w.Code)
	}
	r = httptest.NewRequest("GET", "http://unknown.test/", nil)
	r.Host = "unknown.test"
	w = httptest.NewRecorder()
	g.ServeHTTP(w, r)
	if w.Code != 404 {
		t.Fatal(w.Code)
	}
}
