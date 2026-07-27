package controlauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSuspensionAndViewerCookieDeny(t *testing.T) {
	s := NewStore()
	s.Put(User{ID: "u", Email: "a@example.com", Active: true, Role: "deployer"})
	raw, _ := s.Issue("u", time.Now().Add(time.Hour), "app:read")
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Bearer "+raw)
	if _, e := s.AuthenticateControl(context.Background(), r); e != nil {
		t.Fatal(e)
	}
	s.Put(User{ID: "u", Email: "a@example.com", Active: false})
	if _, e := s.AuthenticateControl(context.Background(), r); e == nil {
		t.Fatal("suspended accepted")
	}
	v := httptest.NewRequest("GET", "/", nil)
	v.AddCookie(&http.Cookie{Name: "__Host-tiny_app", Value: raw})
	if _, e := s.AuthenticateControl(context.Background(), v); e == nil {
		t.Fatal("viewer cookie accepted")
	}
}
