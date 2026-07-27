package persistence

import (
	"context"
	"errors"
	"github.com/tinyhost/tiny/internal/controlapi"
	"github.com/tinyhost/tiny/internal/operations"
	"testing"
)

type stoppedAppGate struct{}

func (stoppedAppGate) AllowWrite(context.Context, operations.WriteKind) error {
	return operations.ErrWriteDisabled
}

func TestControlCreateAppReplayAndValidation(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	svc := ControlService{Store: s}
	a := controlapi.Actor{ID: "u", Email: "owner@example.com", Active: true}
	if e := svc.CreateApp(context.Background(), a, "demo", "k"); e != nil {
		t.Fatal(e)
	}
	if e := svc.CreateApp(context.Background(), a, "demo", "k"); e != nil {
		t.Fatal(e)
	}
	var n int
	if e := s.DB.QueryRow("SELECT COUNT(*) FROM applications WHERE slug='demo'").Scan(&n); e != nil || n != 1 {
		t.Fatal(e, n)
	}
	if e := svc.CreateApp(context.Background(), a, "other", "k"); e == nil {
		t.Fatal("idempotency conflict")
	}
	if e := svc.CreateApp(context.Background(), a, "BAD", "x"); e == nil {
		t.Fatal("invalid slug")
	}
}
func TestControlCreateAppAuditRollback(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	if _, e := s.DB.Exec("CREATE TRIGGER deny_audit BEFORE INSERT ON audit_events BEGIN SELECT RAISE(ABORT,'deny'); END"); e != nil {
		t.Fatal(e)
	}
	svc := ControlService{Store: s}
	e := svc.CreateApp(context.Background(), controlapi.Actor{ID: "u", Email: "owner@example.com", Active: true}, "rollback", "r")
	if e == nil {
		t.Fatal("audit accepted")
	}
	var n int
	_ = s.DB.QueryRow("SELECT COUNT(*) FROM applications WHERE slug='rollback'").Scan(&n)
	if n != 0 {
		t.Fatal("app committed")
	}
}
func TestControlCreateAppCrossOwnerSlug(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	if _, e := s.DB.Exec("INSERT INTO users(id,normalized_email,role,status,created_at) VALUES('u2','two@example.com','deployer','active',datetime('now'))"); e != nil {
		t.Fatal(e)
	}
	svc := ControlService{Store: s}
	if e := svc.CreateApp(context.Background(), controlapi.Actor{ID: "u", Email: "owner@example.com", Active: true}, "shared", "one"); e != nil {
		t.Fatal(e)
	}
	if e := svc.CreateApp(context.Background(), controlapi.Actor{ID: "u2", Email: "two@example.com", Active: true}, "shared", "two"); e == nil {
		t.Fatal("cross owner accepted")
	}
	var owner string
	if e := s.DB.QueryRow("SELECT owner_user_id FROM applications WHERE slug='shared'").Scan(&owner); e != nil || owner != "u" {
		t.Fatal(e, owner)
	}
	var audits int
	_ = s.DB.QueryRow("SELECT COUNT(*) FROM audit_events WHERE target_id='shared'").Scan(&audits)
	if audits != 1 {
		t.Fatal(audits)
	}
}

func TestControlCreateAppRespectsDiskAndPerDeployerLimits(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	a := controlapi.Actor{ID: "u", Email: "owner@example.com", Active: true}
	if err := (ControlService{Store: s, WriteGate: stoppedAppGate{}}).CreateApp(context.Background(), a, "blocked", "k"); !errors.Is(err, operations.ErrWriteDisabled) {
		t.Fatal(err)
	}
	if err := (ControlService{Store: s, AppsPerDeployer: 2}).CreateApp(context.Background(), a, "third", "k2"); err == nil {
		t.Fatal("app quota accepted")
	}
	var count int
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM applications WHERE owner_user_id='u'").Scan(&count); err != nil || count != 2 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}
