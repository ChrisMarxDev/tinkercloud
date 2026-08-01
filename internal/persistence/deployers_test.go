package persistence

import (
	"context"
	"testing"
)

func TestSetDeployerStatus(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	ctx := context.Background()
	if e := s.SetDeployerStatus(ctx, "new@example.com", "active", "r1"); e != nil {
		t.Fatal(e)
	}
	var role, status string
	_ = s.DB.QueryRow("SELECT role,status FROM users WHERE normalized_email='new@example.com'").Scan(&role, &status)
	if role != "deployer" || status != "active" {
		t.Fatal(role, status)
	}
	if e := s.SetDeployerStatus(ctx, "new@example.com", "active", "r1"); e != nil {
		t.Fatal(e)
	}
	if e := s.SetDeployerStatus(ctx, "bad", "active", "r"); e == nil {
		t.Fatal("bad email")
	}
	if e := s.SetDeployerStatus(ctx, "x@example.com", "bad", "r"); e == nil {
		t.Fatal("bad status")
	}
}
func TestSetDeployerStatusPreservesOperatorAndRollsBackAudit(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	ctx := context.Background()
	if e := s.SetDeployerStatus(ctx, "owner@example.com", "suspended", "r2"); e != nil {
		t.Fatal(e)
	}
	var role string
	_ = s.DB.QueryRow("SELECT role FROM users WHERE normalized_email='owner@example.com'").Scan(&role)
	if role != "deployer" {
		t.Fatal(role)
	}
	if _, e := s.DB.Exec("CREATE TRIGGER deny_audit BEFORE INSERT ON audit_events BEGIN SELECT RAISE(ABORT,'deny'); END"); e != nil {
		t.Fatal(e)
	}
	if e := s.SetDeployerStatus(ctx, "rollback@example.com", "active", "r3"); e == nil {
		t.Fatal("audit failure accepted")
	}
	var n int
	_ = s.DB.QueryRow("SELECT COUNT(*) FROM users WHERE normalized_email='rollback@example.com'").Scan(&n)
	if n != 0 {
		t.Fatal(n)
	}
}
