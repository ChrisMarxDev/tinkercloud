package persistence

import (
	"context"
	"github.com/tinyhost/tiny/internal/controlapi"
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
