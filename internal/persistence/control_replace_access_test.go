package persistence

import (
	"context"
	"errors"
	"github.com/ChrisMarxDev/tinkercloud/internal/controlapi"
	"testing"
)

type revokeSpy struct{ n int }

func (r *revokeSpy) Revoke(string, string) { r.n++ }
func TestReplaceAccessReplay(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	_, _ = s.DB.Exec("INSERT INTO access_policies(app_id,revision,mode,created_at) VALUES('a',1,'private',datetime('now'))")
	spy := &revokeSpy{}
	svc := ControlService{Store: s, Live: spy}
	in := controlapi.AccessPolicyInput{Mode: "private", ExpectedRevision: 1, ConfirmBroadening: true}
	in.Allow.Emails = []string{"a@example.com"}
	if e := svc.ReplaceAccess(context.Background(), controlapi.Actor{ID: "u", Active: true}, "alpha", in, "k"); e != nil {
		t.Fatal(e)
	}
	if e := svc.ReplaceAccess(context.Background(), controlapi.Actor{ID: "u", Active: true}, "alpha", in, "k"); e != nil {
		t.Fatal(e)
	}
	var rev, n int
	_ = s.DB.QueryRow("SELECT policy_revision FROM applications WHERE id='a'").Scan(&rev)
	_ = s.DB.QueryRow("SELECT COUNT(*) FROM audit_events WHERE action='policy.replaced'").Scan(&n)
	if rev != 2 || n != 1 || spy.n != 1 {
		t.Fatal(rev, n, spy.n)
	}
}
func TestReplaceAccessDenials(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	_, _ = s.DB.Exec("INSERT INTO access_policies(app_id,revision,mode,created_at) VALUES('a',1,'private',datetime('now'))")
	spy := &revokeSpy{}
	svc := ControlService{Store: s, Live: spy}
	in := controlapi.AccessPolicyInput{Mode: "private", ExpectedRevision: 1}
	if e := svc.ReplaceAccess(context.Background(), controlapi.Actor{ID: "other", Active: true}, "alpha", in, "x"); e == nil {
		t.Fatal("owner")
	}
	if e := svc.ReplaceAccess(context.Background(), controlapi.Actor{ID: "u", Active: true}, "missing", in, "x"); e == nil {
		t.Fatal("missing")
	}
	if spy.n != 0 {
		t.Fatal("revoke")
	}
}

func TestReplaceAccessRevisionConflictAndBroadeningConfirmationDoNotMutate(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	if _, err := s.DB.Exec("INSERT INTO access_policies(app_id,revision,mode,created_at) VALUES('a',1,'private',datetime('now'))"); err != nil {
		t.Fatal(err)
	}
	spy := &revokeSpy{}
	svc := ControlService{Store: s, Live: spy}
	broadening := controlapi.AccessPolicyInput{Mode: "private", ExpectedRevision: 1}
	broadening.Allow.Emails = []string{"viewer@example.com"}
	if err := svc.ReplaceAccess(context.Background(), controlapi.Actor{ID: "u", Active: true}, "alpha", broadening, "without-confirmation"); err == nil {
		t.Fatal("unconfirmed broadening accepted")
	}
	confirmed := broadening
	confirmed.ConfirmBroadening = true
	if err := svc.ReplaceAccess(context.Background(), controlapi.Actor{ID: "u", Active: true}, "alpha", confirmed, "first"); err != nil {
		t.Fatal(err)
	}
	if err := svc.ReplaceAccess(context.Background(), controlapi.Actor{ID: "u", Active: true}, "alpha", confirmed, "stale"); !errors.Is(err, controlapi.ErrPolicyRevision) {
		t.Fatalf("stale replacement = %v", err)
	}
	var rev, audits int
	if err := s.DB.QueryRow("SELECT policy_revision FROM applications WHERE id='a'").Scan(&rev); err != nil || rev != 2 {
		t.Fatalf("revision=%d err=%v", rev, err)
	}
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM audit_events WHERE action='policy.replaced'").Scan(&audits); err != nil || audits != 1 || spy.n != 1 {
		t.Fatalf("audits=%d revocations=%d err=%v", audits, spy.n, err)
	}
}
