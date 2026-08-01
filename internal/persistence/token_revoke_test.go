package persistence

import (
	"context"
	"github.com/ChrisMarxDev/tinkercloud/internal/controlapi"
	"testing"
	"time"
)

func TestGlobalTokenNullAndRevoke(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	raw, e := s.IssueToken(context.Background(), "u", "", []string{"app:read"}, time.Now().Add(time.Hour))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.AuthenticateToken(context.Background(), raw, "app:read", "", time.Now()); e != nil {
		t.Fatal(e)
	}
	if _, e = s.DB.Exec("UPDATE applications SET status='active' WHERE id='a'"); e != nil {
		t.Fatal(e)
	}
	bound, e := s.IssueToken(context.Background(), "u", "a", []string{"app:read"}, time.Now().Add(time.Hour))
	if e != nil {
		t.Fatal(e)
	}
	var id string
	_ = s.DB.QueryRow("SELECT id FROM api_tokens WHERE secret_hash != (SELECT secret_hash FROM api_tokens WHERE app_id IS NULL LIMIT 1) LIMIT 1").Scan(&id)
	svc := ControlService{Store: s}
	if e = svc.RevokeToken(context.Background(), controlapi.Actor{ID: "u", Active: true}, "alpha", id, "r"); e != nil {
		t.Fatal(e)
	}
	if _, e = s.AuthenticateToken(context.Background(), bound, "app:read", "a", time.Now()); e == nil {
		t.Fatal("revoked usable")
	}
	if e = svc.RevokeToken(context.Background(), controlapi.Actor{ID: "u", Active: true}, "alpha", id, "r"); e != nil {
		t.Fatal(e)
	}
}

func TestLogoutRevokesOnlyAuthenticatedGlobalBearer(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	ctx := context.Background()
	first, err := s.IssueToken(ctx, "u", "", []string{"app:read"}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.IssueToken(ctx, "u", "", []string{"app:read"}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	actor, err := s.AuthenticateToken(ctx, first, "app:read", "", time.Now())
	if err != nil || actor.CredentialID == "" {
		t.Fatalf("authenticate = %#v, %v", actor, err)
	}
	svc := ControlService{Store: s}
	if err = svc.RevokeCurrentBearer(ctx, actor); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AuthenticateToken(ctx, first, "app:read", "", time.Now()); err == nil {
		t.Fatal("revoked current bearer remained usable")
	}
	if _, err = s.AuthenticateToken(ctx, second, "app:read", "", time.Now()); err != nil {
		t.Fatalf("other bearer was revoked: %v", err)
	}
	appBound, err := s.IssueToken(ctx, "u", "a", []string{"app:read"}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	appActor, err := s.AuthenticateToken(ctx, appBound, "app:read", "a", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.RevokeCurrentBearer(ctx, appActor); err == nil {
		t.Fatal("app-bound bearer revoked through CLI logout")
	}
}

func TestLogoutPersistenceFailureRollsBackRevocation(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	ctx := context.Background()
	raw, err := s.IssueToken(ctx, "u", "", []string{"app:read"}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	actor, err := s.AuthenticateToken(ctx, raw, "app:read", "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec("CREATE TRIGGER deny_logout_audit BEFORE INSERT ON audit_events BEGIN SELECT RAISE(FAIL, 'test failure'); END"); err != nil {
		t.Fatal(err)
	}
	if err = (ControlService{Store: s}).RevokeCurrentBearer(ctx, actor); err == nil {
		t.Fatal("persistence failure reported successful logout")
	}
	if _, err = s.AuthenticateToken(ctx, raw, "app:read", "", time.Now()); err != nil {
		t.Fatalf("rollback failed; bearer denied: %v", err)
	}
}
