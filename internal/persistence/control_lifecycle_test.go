package persistence

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"testing"
	"time"

	"github.com/tinyhost/tiny/internal/controlapi"
)

type lifecycleLiveSpy struct {
	calls        int
	app, session string
}

func (s *lifecycleLiveSpy) Revoke(app, session string) { s.calls++; s.app, s.session = app, session }

type lifecycleBlobCleanupSpy struct {
	calls chan struct{}
}

func (s *lifecycleBlobCleanupSpy) Reconcile(context.Context) error {
	s.calls <- struct{}{}
	return nil
}

func TestControlReleasesOwnerBoundAndMetadataOnly(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	if _, err := s.DB.Exec("INSERT INTO deployments(id,app_id,created_by,idempotency_key,release_hash,state,created_at) VALUES('old','a','u','old','hash-old','superseded',datetime('now','-1 hour')),('new','a','u','new','hash-new','active',datetime('now'))"); err != nil {
		t.Fatal(err)
	}
	svc := ControlService{Store: s}
	v, err := svc.Releases(context.Background(), controlapi.Actor{ID: "u", Active: true}, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	out := v.([]ReleaseView)
	if len(out) != 2 || out[0].ID != "new" || out[0].ReleaseHash != "hash-new" {
		t.Fatalf("unexpected releases: %#v", out)
	}
	if _, err = svc.Releases(context.Background(), controlapi.Actor{ID: "other", Active: true}, "alpha"); err == nil {
		t.Fatal("cross-owner release list accepted")
	}
}

func TestControlDeleteAppRevokesAtomicallyAndDoesNotDeleteFiles(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	seedActiveRelease(t, s)
	appToken, err := s.IssueToken(context.Background(), "u", "a", []string{"app:read"}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte("viewer-session"))
	if _, err = s.DB.Exec("INSERT INTO sessions(id,scope,app_id,identity_id,secret_hash,expires_at,created_at) VALUES('ses','app','a','i',?,?,datetime('now'))", hash[:], time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	live := &lifecycleLiveSpy{}
	cleanup := &lifecycleBlobCleanupSpy{calls: make(chan struct{}, 2)}
	svc := ControlService{Store: s, Live: live, BlobCleanup: cleanup}
	actor := controlapi.Actor{ID: "u", Active: true}
	if err = svc.DeleteApp(context.Background(), actor, "alpha", "delete-one"); err != nil {
		t.Fatal(err)
	}
	var status string
	if err = s.DB.QueryRow("SELECT status FROM applications WHERE id='a'").Scan(&status); err != nil || status != "deleted" {
		t.Fatal(status, err)
	}
	var tokens, sessions int
	if err = s.DB.QueryRow("SELECT COUNT(*) FROM api_tokens WHERE app_id='a' AND revoked_at IS NULL").Scan(&tokens); err != nil || tokens != 0 {
		t.Fatal(tokens, err)
	}
	if err = s.DB.QueryRow("SELECT COUNT(*) FROM sessions WHERE app_id='a' AND revoked_at IS NULL").Scan(&sessions); err != nil || sessions != 0 {
		t.Fatal(sessions, err)
	}
	if _, err = s.AuthenticateToken(context.Background(), appToken, "app:read", "a", time.Now()); err == nil {
		t.Fatal("revoked app token remained usable")
	}
	if live.calls != 1 || live.app != "a" || live.session != "" {
		t.Fatalf("live revocation = %#v", live)
	}
	select {
	case <-cleanup.calls:
	case <-time.After(time.Second):
		t.Fatal("deleted app did not trigger blob reconciliation")
	}
	// The deployment row and its immutable release root are intentionally left
	// for asynchronous retention cleanup, not this security-sensitive request.
	var deployments int
	if err = s.DB.QueryRow("SELECT COUNT(*) FROM deployments WHERE app_id='a'").Scan(&deployments); err != nil || deployments != 1 {
		t.Fatal(deployments, err)
	}
	if _, err = s.ResolveActive(context.Background(), "alpha"); err == nil {
		t.Fatal("deleted app remained gateway-resolvable")
	}
	if err = svc.DeleteApp(context.Background(), actor, "alpha", "delete-one"); err != nil {
		t.Fatal("matching idempotent retry denied: ", err)
	}
	select {
	case <-cleanup.calls:
		t.Fatal("idempotent deletion retry triggered blob reconciliation")
	default:
	}
	if err = svc.DeleteApp(context.Background(), actor, "alpha", "other-request"); err == nil {
		t.Fatal("deleted app accepted a new deletion request")
	}
	select {
	case <-cleanup.calls:
		t.Fatal("failed deletion triggered blob reconciliation")
	default:
	}
}

func TestControlDeleteAppAuditFailureRollsBackMutation(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	raw, err := s.IssueToken(context.Background(), "u", "a", []string{"app:read"}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec("CREATE TRIGGER deny_delete_audit BEFORE INSERT ON audit_events WHEN NEW.action='app.deletion_requested' BEGIN SELECT RAISE(ABORT,'deny'); END"); err != nil {
		t.Fatal(err)
	}
	if err = (ControlService{Store: s}).DeleteApp(context.Background(), controlapi.Actor{ID: "u", Active: true}, "alpha", "rollback"); err == nil {
		t.Fatal("audit failure accepted")
	}
	var status string
	var revoked sql.NullString
	if err = s.DB.QueryRow("SELECT status FROM applications WHERE id='a'").Scan(&status); err != nil || status != "active" {
		t.Fatal(status, err)
	}
	if err = s.DB.QueryRow("SELECT revoked_at FROM api_tokens WHERE app_id='a'").Scan(&revoked); err != nil || revoked.Valid {
		t.Fatal(revoked, err)
	}
	if _, err = s.AuthenticateToken(context.Background(), raw, "app:read", "a", time.Now()); err != nil {
		t.Fatal("token revoked despite rolled-back deletion: ", err)
	}
}

func TestControlDeleteAppCrossOwnerDenied(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	if _, err := s.DB.Exec("INSERT INTO users(id,normalized_email,role,status,created_at) VALUES('u2','two@example.com','deployer','active',datetime('now'))"); err != nil {
		t.Fatal(err)
	}
	cleanup := &lifecycleBlobCleanupSpy{calls: make(chan struct{}, 1)}
	if err := (ControlService{Store: s, BlobCleanup: cleanup}).DeleteApp(context.Background(), controlapi.Actor{ID: "u2", Active: true}, "alpha", "x"); err == nil {
		t.Fatal("cross-owner deletion accepted")
	}
	select {
	case <-cleanup.calls:
		t.Fatal("unauthorized deletion triggered blob reconciliation")
	default:
	}
	var status string
	if err := s.DB.QueryRow("SELECT status FROM applications WHERE id='a'").Scan(&status); err != nil || status != "active" {
		t.Fatal(status, err)
	}
}

func TestControlReplaceAccessDoesNotTriggerBlobCleanup(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	if _, err := s.DB.Exec("INSERT INTO access_policies(app_id,revision,mode,created_at) VALUES('a',1,'private',datetime('now'))"); err != nil {
		t.Fatal(err)
	}
	live := &lifecycleLiveSpy{}
	cleanup := &lifecycleBlobCleanupSpy{calls: make(chan struct{}, 1)}
	svc := ControlService{Store: s, Live: live, BlobCleanup: cleanup}
	in := controlapi.AccessPolicyInput{Mode: "private", ExpectedRevision: 1, ConfirmBroadening: true}
	in.Allow.Emails = []string{"viewer@example.com"}
	if err := svc.ReplaceAccess(context.Background(), controlapi.Actor{ID: "u", Active: true}, "alpha", in, "replace"); err != nil {
		t.Fatal(err)
	}
	if live.calls != 1 || live.app != "a" || live.session != "" {
		t.Fatalf("access replacement did not immediately revoke live sessions: %#v", live)
	}
	select {
	case <-cleanup.calls:
		t.Fatal("access replacement triggered blob reconciliation")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestControlSuspendAppRevokesImmediatelyAndOperatorMaySuspendAnyApp(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	seedActiveRelease(t, s)
	raw, err := s.IssueToken(context.Background(), "u", "a", []string{"app:read"}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	live := &lifecycleLiveSpy{}
	svc := ControlService{Store: s, Live: live}
	if err = svc.SetAppStatus(context.Background(), controlapi.Actor{ID: "u", Role: "deployer", Active: true}, "alpha", "suspended", "suspend"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ResolveActive(context.Background(), "alpha"); err == nil {
		t.Fatal("suspended app resolved")
	}
	if _, err = s.AuthenticateToken(context.Background(), raw, "app:read", "a", time.Now()); err == nil {
		t.Fatal("suspension left app token valid")
	}
	if live.calls != 1 || live.app != "a" {
		t.Fatalf("live revoke: %#v", live)
	}
	if err = svc.SetAppStatus(context.Background(), controlapi.Actor{ID: "u", Role: "deployer", Active: true}, "alpha", "active", "resume"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ResolveActive(context.Background(), "alpha"); err != nil {
		t.Fatal("resume did not restore routing: ", err)
	}
	if _, err = s.AuthenticateToken(context.Background(), raw, "app:read", "a", time.Now()); err == nil {
		t.Fatal("resume resurrected revoked token")
	}
}

func TestControlDeployerStatusRequiresActiveOperatorAndRevokesCredentials(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	if _, err := s.DB.Exec("INSERT INTO users(id,normalized_email,role,status,created_at) VALUES('op','operator@example.com','operator','active',datetime('now')),('d','deployer@example.com','deployer','active',datetime('now'))"); err != nil {
		t.Fatal(err)
	}
	raw, err := s.IssueToken(context.Background(), "d", "", []string{"app:read"}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	svc := ControlService{Store: s}
	if err = svc.SetDeployerStatus(context.Background(), controlapi.Actor{ID: "d", Role: "deployer", Active: true}, "deployer@example.com", "suspended", "x"); err == nil {
		t.Fatal("deployer could change deployer status")
	}
	if err = svc.SetDeployerStatus(context.Background(), controlapi.Actor{ID: "op", Role: "operator", Active: true}, "deployer@example.com", "suspended", "s"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AuthenticateToken(context.Background(), raw, "app:read", "", time.Now()); err == nil {
		t.Fatal("suspended deployer credential remained active")
	}
	var status string
	if err = s.DB.QueryRow("SELECT status FROM users WHERE id='d'").Scan(&status); err != nil || status != "suspended" {
		t.Fatalf("status=%q err=%v", status, err)
	}
}
