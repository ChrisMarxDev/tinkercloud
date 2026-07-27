package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/tinyhost/tiny/internal/controlapi"
)

func TestEnsureInitialOperator(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	ctx := context.Background()
	if e := s.EnsureInitialOperator(ctx, "root@example.com", "r1"); e != nil {
		t.Fatal(e)
	}
	if e := s.EnsureInitialOperator(ctx, "root@example.com", "r1"); e != nil {
		t.Fatal(e)
	}
	if e := s.EnsureInitialOperator(ctx, "other@example.com", "r2"); e == nil {
		t.Fatal("replaced operator")
	}
}

func TestRecoverOperatorRevokesPriorControlCredentialsIncludingSameEmail(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	ctx := context.Background()
	if err := s.EnsureInitialOperator(ctx, "root@example.com", "r1"); err != nil {
		t.Fatal(err)
	}
	var operatorID string
	if err := s.DB.QueryRow("SELECT id FROM users WHERE normalized_email='root@example.com'").Scan(&operatorID); err != nil {
		t.Fatal(err)
	}
	before, err := s.IssueToken(ctx, operatorID, "", []string{"app:read"}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	o := &captureOutbox{}
	login := ControlLogin{Store: s, HMACKey: []byte("key"), Outbox: o}
	browserChallenge, err := login.RequestOTP(ctx, "root@example.com", controlapi.BrowserLoginChannel)
	if err != nil || browserChallenge == "" || o.m.Code == "" {
		t.Fatal(err)
	}
	browserBefore, err := login.VerifyOTP(ctx, browserChallenge, o.m.Code, controlapi.BrowserLoginChannel)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RecoverOperator(ctx, "root@example.com", "r2"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AuthenticateToken(ctx, before, "app:read", "", time.Now()); err == nil {
		t.Fatal("pre-recovery control credential remained valid")
	}
	if _, err = s.AuthenticateControlSession(ctx, browserBefore, time.Now()); err == nil {
		t.Fatal("pre-recovery browser control session remained valid")
	}
	challenge, err := login.RequestOTP(ctx, "root@example.com", controlapi.BrowserLoginChannel)
	if err != nil || challenge == "" || o.m.Code == "" {
		t.Fatal(err)
	}
	after, err := login.VerifyOTP(ctx, challenge, o.m.Code, controlapi.BrowserLoginChannel)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AuthenticateControlSession(ctx, after, time.Now()); err != nil {
		t.Fatalf("recovered operator could not authenticate: %v", err)
	}
}
func TestRecoverOperatorChangesActive(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	ctx := context.Background()
	if e := s.EnsureInitialOperator(ctx, "old@example.com", "r1"); e != nil {
		t.Fatal(e)
	}
	if e := s.RecoverOperator(ctx, "new@example.com", "r2"); e != nil {
		t.Fatal(e)
	}
	var n int
	_ = s.DB.QueryRow("SELECT COUNT(*) FROM users WHERE role='operator' AND status='active' AND normalized_email='new@example.com'").Scan(&n)
	if n != 1 {
		t.Fatal(n)
	}
}
