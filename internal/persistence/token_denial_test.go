package persistence

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"sync"
	"testing"
	"time"

	"github.com/tinyhost/tiny/internal/controlapi"
)

func tokenLastUsed(t *testing.T, s *SQLiteStore, raw string) sql.NullString {
	t.Helper()
	h := sha256.Sum256([]byte(raw))
	var used sql.NullString
	if err := s.DB.QueryRow("SELECT last_used_at FROM api_tokens WHERE secret_hash=?", h[:]).Scan(&used); err != nil {
		t.Fatal(err)
	}
	return used
}

func TestAuthenticateTokenRecordsOnlySuccessfulSafeLastUse(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	now := time.Date(2026, 7, 27, 12, 0, 1, 123456789, time.UTC)
	raw, err := s.IssueToken(context.Background(), "u", "a", []string{"app:read"}, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if used := tokenLastUsed(t, s, raw); used.Valid {
		t.Fatalf("last use before authorization = %q", used.String)
	}
	if _, err = s.AuthenticateToken(context.Background(), raw, "app:read", "a", now); err != nil {
		t.Fatal(err)
	}
	used := tokenLastUsed(t, s, raw)
	if !used.Valid || used.String != now.Format(time.RFC3339Nano) {
		t.Fatalf("safe last use = %#v", used)
	}
}

func TestAuthenticateTokenAcceptsLaterSameSecondFractionalExpiry(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	now := time.Date(2026, 7, 27, 12, 0, 1, 100_000_000, time.UTC)
	raw, err := s.IssueToken(context.Background(), "u", "a", []string{"app:read"}, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256([]byte(raw))
	// This is one nanosecond after now, but sorts before now's canonical
	// RFC3339Nano form (`.1Z`) when compared as text.
	expiry := "2026-07-27T12:00:01.100000001Z"
	if _, err = s.DB.Exec("UPDATE api_tokens SET expires_at=? WHERE secret_hash=?", expiry, h[:]); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AuthenticateToken(context.Background(), raw, "app:read", "a", now); err != nil {
		t.Fatalf("valid same-second token denied: %v", err)
	}
	if used := tokenLastUsed(t, s, raw); !used.Valid || used.String != now.Format(time.RFC3339Nano) {
		t.Fatalf("last use = %#v", used)
	}
}

func TestAuthenticateTokenDeniedPathsDoNotRecordLastUse(t *testing.T) {
	tests := []struct {
		name  string
		scope string
		app   string
		setup func(t *testing.T, s *SQLiteStore)
		now   func(time.Time) time.Time
	}{
		{name: "wrong app", scope: "app:read", app: "b"},
		{name: "wrong scope", scope: "app:re", app: "a"},
		{name: "compound scope injection", scope: "app:read,deploy:create", app: "a"},
		{name: "expired", scope: "app:read", app: "a", now: func(n time.Time) time.Time { return n.Add(2 * time.Hour) }},
		{name: "malformed expiry", scope: "app:read", app: "a", setup: func(t *testing.T, s *SQLiteStore) {
			if _, err := s.DB.Exec("UPDATE api_tokens SET expires_at='not-a-time'"); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "revoked", scope: "app:read", app: "a", setup: func(t *testing.T, s *SQLiteStore) {
			if _, err := s.DB.Exec("UPDATE api_tokens SET revoked_at=datetime('now')"); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "suspended user", scope: "app:read", app: "a", setup: func(t *testing.T, s *SQLiteStore) {
			if _, err := s.DB.Exec("UPDATE users SET status='suspended' WHERE id='u'"); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := seeded(t)
			defer s.Close()
			now := time.Now().UTC()
			raw, err := s.IssueToken(context.Background(), "u", "a", []string{"app:read"}, now.Add(time.Hour))
			if err != nil {
				t.Fatal(err)
			}
			if tt.setup != nil {
				tt.setup(t, s)
			}
			attempt := now
			if tt.now != nil {
				attempt = tt.now(now)
			}
			if _, err = s.AuthenticateToken(context.Background(), raw, tt.scope, tt.app, attempt); err != ErrToken {
				t.Fatalf("got %v", err)
			}
			if used := tokenLastUsed(t, s, raw); used.Valid {
				t.Fatalf("denied request wrote %q", used.String)
			}
		})
	}
}

func TestAuthenticateTokenMetadataWriteFailureDeniesAndRollsBack(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	raw, err := s.IssueToken(context.Background(), "u", "a", []string{"app:read"}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`CREATE TRIGGER deny_token_last_use BEFORE UPDATE OF last_used_at ON api_tokens BEGIN SELECT RAISE(FAIL, 'test failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AuthenticateToken(context.Background(), raw, "app:read", "a", time.Now()); err != ErrToken {
		t.Fatalf("got %v", err)
	}
	if used := tokenLastUsed(t, s, raw); used.Valid {
		t.Fatalf("failed metadata write committed %q", used.String)
	}
}

func TestAuthenticateTokenRevocationSerialization(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	if _, err := s.DB.Exec("UPDATE applications SET status='active' WHERE id='a'"); err != nil {
		t.Fatal(err)
	}
	raw, err := s.IssueToken(context.Background(), "u", "a", []string{"app:read"}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	var id string
	h := sha256.Sum256([]byte(raw))
	if err = s.DB.QueryRow("SELECT id FROM api_tokens WHERE secret_hash=?", h[:]).Scan(&id); err != nil {
		t.Fatal(err)
	}
	svc := ControlService{Store: s}
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		_, _ = s.AuthenticateToken(context.Background(), raw, "app:read", "a", time.Now())
	}()
	go func() {
		defer wg.Done()
		<-start
		_ = svc.RevokeToken(context.Background(), controlapi.Actor{ID: "u", Active: true}, "alpha", id, "revoke-race")
	}()
	close(start)
	wg.Wait()
	// Once the revoke transaction returns, the next relevant request must deny.
	if _, err = s.AuthenticateToken(context.Background(), raw, "app:read", "a", time.Now()); err != ErrToken {
		t.Fatalf("post-revoke auth = %v", err)
	}
}

func TestAuthenticateTokenDenials(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	raw, e := s.IssueToken(context.Background(), "u", "a", []string{"app:read"}, time.Now().Add(time.Hour))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.AuthenticateToken(context.Background(), raw, "app:read", "b", time.Now()); e == nil {
		t.Fatal("wrong app")
	}
	if _, e = s.AuthenticateToken(context.Background(), raw, "deploy:create", "a", time.Now()); e == nil {
		t.Fatal("scope")
	}
	if _, e = s.AuthenticateToken(context.Background(), raw, "app:read", "a", time.Now().Add(2*time.Hour)); e == nil {
		t.Fatal("expiry")
	}
	if _, e = s.DB.Exec("UPDATE users SET status='suspended' WHERE id='u'"); e != nil {
		t.Fatal(e)
	}
	if _, e = s.AuthenticateToken(context.Background(), raw, "app:read", "a", time.Now()); e == nil {
		t.Fatal("suspended")
	}
}

func TestAuthenticateTokenRevokedImmediately(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	raw, err := s.IssueToken(context.Background(), "u", "a", []string{"app:read"}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec("UPDATE api_tokens SET revoked_at=datetime('now')"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AuthenticateToken(context.Background(), raw, "app:read", "a", time.Now()); err != ErrToken {
		t.Fatalf("got %v", err)
	}
}

func TestCLIControlLoginIssuedTokenHasV1Scopes(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	outbox := &captureOutbox{}
	login := ControlLogin{Store: s, HMACKey: []byte("key"), Outbox: outbox}
	transaction, err := login.RequestOTP(context.Background(), "owner@example.com", controlapi.CLILoginChannel)
	if err != nil || transaction == "" {
		t.Fatal(err)
	}
	raw, err := login.VerifyOTP(context.Background(), transaction, outbox.m.Code, controlapi.CLILoginChannel)
	if err != nil {
		t.Fatal(err)
	}
	for _, scope := range []string{"app:read", "app:create", "deploy:create", "deploy:activate", "access:read", "access:write", "token:create", "token:revoke"} {
		if _, err = s.AuthenticateToken(context.Background(), raw, scope, "", time.Now()); err != nil {
			t.Fatalf("%s: %v", scope, err)
		}
	}
}
