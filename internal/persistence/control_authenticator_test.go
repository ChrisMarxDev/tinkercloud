package persistence

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tinyhost/tiny/internal/controlapi"
	"github.com/tinyhost/tiny/internal/identity"
)

func TestControlAuthenticatorAppBoundToken(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	if _, e := s.DB.Exec("UPDATE applications SET status='active' WHERE id='b'"); e != nil {
		t.Fatal(e)
	}
	raw, e := s.IssueToken(context.Background(), "u", "a", []string{"access:read", "deploy:create", "deploy:activate"}, time.Now().Add(time.Hour))
	if e != nil {
		t.Fatal(e)
	}
	auth := ControlAuthenticator{Store: s}
	req := func(path string) error {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r.Header.Set("Authorization", "Bearer "+raw)
		_, e := auth.AuthenticateControl(context.Background(), r)
		return e
	}
	if e := req("/api/v1/apps/alpha/access"); e != nil {
		t.Fatal(e)
	}
	if e := req("/api/v1/apps/beta/access"); e == nil {
		t.Fatal("cross app")
	}
	if e := req("/api/v1/apps/unknown/access"); e == nil {
		t.Fatal("unknown")
	}
	if e := req("/api/v1/apps/alpha%2faccess/access"); e == nil {
		t.Fatal("encoded")
	}
}

func TestControlAuthenticatorSeparatesBrowserBearerAndViewerCredentials(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	ctx := context.Background()
	auth := ControlAuthenticator{Store: s}
	platform := func(raw string) error {
		r := httptest.NewRequest(http.MethodGet, "https://tiny.test/dashboard", nil)
		r.AddCookie(&http.Cookie{Name: controlapi.ControlCookieName, Value: raw})
		_, err := auth.AuthenticatePlatform(ctx, r)
		return err
	}
	bearer := func(raw string) error {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
		r.Header.Set("Authorization", "Bearer "+raw)
		_, err := auth.AuthenticateControl(ctx, r)
		return err
	}
	outbox := &captureOutbox{}
	login := ControlLogin{Store: s, HMACKey: []byte("key"), Outbox: outbox}
	tx, err := login.RequestOTP(ctx, "owner@example.com", controlapi.BrowserLoginChannel)
	if err != nil || tx == "" || outbox.m.Code == "" {
		t.Fatal(err)
	}
	browser, err := login.VerifyOTP(ctx, tx, outbox.m.Code, controlapi.BrowserLoginChannel)
	if err != nil || browser == "" {
		t.Fatal(err)
	}
	if err = platform(browser); err != nil {
		t.Fatalf("browser control session denied: %v", err)
	}
	if err = bearer(browser); err == nil {
		t.Fatal("browser control session accepted as bearer")
	}
	cli, err := s.IssueToken(ctx, "u", "", []string{"app:read"}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err = bearer(cli); err != nil {
		t.Fatalf("CLI bearer denied: %v", err)
	}
	if err = platform(cli); err == nil {
		t.Fatal("CLI bearer accepted as control cookie")
	}
	if _, err = s.DB.Exec("INSERT INTO identities(id,normalized_email,created_at) VALUES('viewer','viewer@example.com',datetime('now'))"); err != nil {
		t.Fatal(err)
	}
	viewer, _, err := s.CreateAppSession(ctx, "a", identity.Identity{ID: "viewer", Email: "viewer@example.com"}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err = bearer(viewer); err == nil {
		t.Fatal("app viewer session accepted as bearer")
	}
	if err = platform(viewer); err == nil {
		t.Fatal("app viewer session accepted as control cookie")
	}
}

func TestControlAuthenticatorDeleteAllowsSuspendedTargetOnly(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	raw, err := s.IssueToken(context.Background(), "u", "b", []string{"app:delete"}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	auth := ControlAuthenticator{Store: s}
	r := httptest.NewRequest(http.MethodDelete, "/api/v1/apps/beta", nil)
	r.Header.Set("Authorization", "Bearer "+raw)
	if _, err = auth.AuthenticateControl(context.Background(), r); err != nil {
		t.Fatal("suspended deletion authorization denied: ", err)
	}
	r = httptest.NewRequest(http.MethodGet, "/api/v1/apps/beta/releases", nil)
	r.Header.Set("Authorization", "Bearer "+raw)
	if _, err = auth.AuthenticateControl(context.Background(), r); err == nil {
		t.Fatal("suspended non-delete route accepted")
	}
}
