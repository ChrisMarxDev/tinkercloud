package persistence

import (
	"context"
	"github.com/ChrisMarxDev/tinkercloud/internal/controlapi"
	"testing"
	"time"
)

func TestControlCreateTokenBoundAndOpaque(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	if _, e := s.DB.Exec("UPDATE applications SET status='active' WHERE id='a'"); e != nil {
		t.Fatal(e)
	}
	svc := ControlService{Store: s}
	in := controlapi.TokenInput{Scopes: []string{"app:read"}, ExpiresInSeconds: 3600}
	out, e := svc.CreateToken(context.Background(), controlapi.Actor{ID: "u", Active: true}, "alpha", in, "k")
	if e != nil || out.Token == "" {
		t.Fatal(e)
	}
	if _, e = s.AuthenticateToken(context.Background(), out.Token, "app:read", "a", time.Now()); e != nil {
		t.Fatal(e)
	}
	if _, e = s.AuthenticateToken(context.Background(), out.Token, "app:read", "b", time.Now()); e == nil {
		t.Fatal("cross app")
	}
	var stored []byte
	_ = s.DB.QueryRow("SELECT secret_hash FROM api_tokens WHERE id=?", out.ID).Scan(&stored)
	if string(stored) == out.Token {
		t.Fatal("raw persisted")
	}
	if _, e = svc.CreateToken(context.Background(), controlapi.Actor{ID: "u", Active: true}, "alpha", in, "k"); e == nil {
		t.Fatal("replay")
	}
}
