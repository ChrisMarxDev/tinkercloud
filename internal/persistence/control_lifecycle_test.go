package persistence

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tinyhost/tiny/internal/blob"
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

func (*lifecycleBlobCleanupSpy) RemoveAppNamespace(string) error { return nil }

type lifecyclePurgeSpy struct {
	calls int
	err   error
}

func (s *lifecyclePurgeSpy) Purge(context.Context, string) error {
	s.calls++
	return s.err
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

func TestControlDeleteAppPurgesOwnedStateAndPrivateBytes(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	seedActiveRelease(t, s)
	for _, statement := range []string{
		"INSERT INTO provider_connections(id,display_name,provider_kind,credential_envelope,credential_key_version,status,created_at,updated_at) VALUES('conn','test','anthropic',X'01',1,'active',datetime('now'),datetime('now'))",
		"INSERT INTO llm_chat_profiles(id,connection_id,model,max_messages,max_message_bytes,max_input_bytes,max_output_tokens,timeout_ms,viewer_requests,app_requests,rate_window_ms,concurrency_limit,monthly_token_limit,status,revision,created_at,updated_at) VALUES('profile','conn','test',1,1,1,1,1000,1,1,1000,1,1,'active',1,datetime('now'),datetime('now'))",
		"INSERT INTO app_capability_grants(app_id,capability,version,profile_id,status,revision,created_at,updated_at) VALUES('a','llm.chat',1,'profile','approved',1,datetime('now'),datetime('now'))",
		"INSERT INTO llm_usage(app_id,profile_id,period_start,used_tokens,reserved_tokens,in_flight,updated_at) VALUES('a','profile','2026-07-01T00:00:00Z',0,1,1,datetime('now'))",
		"INSERT INTO llm_reservations(id,app_id,profile_id,identity_id,period_start,reserved_tokens,status,created_at) VALUES('reservation','a','profile','i','2026-07-01T00:00:00Z',1,'calling',datetime('now'))",
		"INSERT INTO access_policies(app_id,revision,mode,created_at) VALUES('a',1,'private',datetime('now'))",
		"INSERT INTO access_rules(id,app_id,policy_revision,kind,normalized_value,created_at) VALUES('rule','a',1,'email','viewer@example.com',datetime('now'))",
		"INSERT INTO otp_challenges(id,app_id,purpose,normalized_email,code_hash,expires_at,attempts,created_at) VALUES('otp','a','viewer','viewer@example.com',X'01',datetime('now','+1 hour'),0,datetime('now'))",
		"INSERT INTO app_quota_usage(app_id,metric,used,limit_value,measured_at) VALUES('a','blob_bytes',1,2,datetime('now'))",
		"INSERT INTO app_blobs(id,app_id,state,display_name,content_type,size_bytes,content_hash,created_by_identity_id,created_at,updated_at) VALUES('blb_0123456789abcdef0123456789abcdef','a','ready','x','text/plain',1,'hash','i',datetime('now'),datetime('now'))",
		"INSERT INTO identity_handoffs(id,app_id,state_hash,return_path,force_login,expires_at,created_at) VALUES('handoff','a',randomblob(32),'/',0,datetime('now','+1 hour'),datetime('now'))",
		"INSERT INTO audit_events(id,occurred_at,actor_kind,app_id,action,outcome) VALUES('audit',datetime('now'),'user','a','app.updated','success')",
	} {
		if _, err := s.DB.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
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
	svc := ControlService{Store: s, Live: live, BlobCleanup: cleanup, AppDataCleanup: AppDataPurger{DataRoot: s.DataRoot, Store: s, BlobCleanup: cleanup}}
	actor := controlapi.Actor{ID: "u", Active: true}
	if err = svc.DeleteApp(context.Background(), actor, "alpha", "delete-one"); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		"SELECT COUNT(*) FROM applications WHERE id='a'",
		"SELECT COUNT(*) FROM llm_reservations WHERE app_id='a'",
		"SELECT COUNT(*) FROM llm_usage WHERE app_id='a'",
		"SELECT COUNT(*) FROM app_capability_grants WHERE app_id='a'",
		"SELECT COUNT(*) FROM api_tokens WHERE app_id='a'",
		"SELECT COUNT(*) FROM sessions WHERE app_id='a'",
		"SELECT COUNT(*) FROM deployment_files WHERE deployment_id='d'",
		"SELECT COUNT(*) FROM deployments WHERE app_id='a'",
		"SELECT COUNT(*) FROM access_rules WHERE app_id='a'",
		"SELECT COUNT(*) FROM access_policies WHERE app_id='a'",
		"SELECT COUNT(*) FROM otp_challenges WHERE app_id='a'",
		"SELECT COUNT(*) FROM app_quota_usage WHERE app_id='a'",
		"SELECT COUNT(*) FROM app_blobs WHERE app_id='a'",
		"SELECT COUNT(*) FROM identity_handoffs WHERE app_id='a'",
		"SELECT COUNT(*) FROM audit_events WHERE app_id='a'",
	} {
		var count int
		if err = s.DB.QueryRow(statement).Scan(&count); err != nil || count != 0 {
			t.Fatalf("owned row remained count=%d err=%v query=%s", count, err, statement)
		}
	}
	if _, err = os.Stat(filepath.Join(s.DataRoot, "releases", "a")); !os.IsNotExist(err) {
		t.Fatalf("release namespace remained: %v", err)
	}
	if _, err = s.AuthenticateToken(context.Background(), appToken, "app:read", "a", time.Now()); err == nil {
		t.Fatal("revoked app token remained usable")
	}
	if live.calls != 1 || live.app != "a" || live.session != "" {
		t.Fatalf("live revocation = %#v", live)
	}
	select {
	case <-cleanup.calls:
	default:
		t.Fatal("deletion did not synchronously purge blob bytes")
	}
	if _, err = s.ResolveActive(context.Background(), "alpha"); err == nil {
		t.Fatal("deleted app remained gateway-resolvable")
	}
	if err = svc.DeleteApp(context.Background(), actor, "alpha", "delete-one"); err == nil {
		t.Fatal("hard-deleted app accepted a retry")
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
	if _, err = s.DB.Exec("CREATE TRIGGER deny_delete_app BEFORE DELETE ON applications WHEN OLD.id='a' BEGIN SELECT RAISE(ABORT,'deny'); END"); err != nil {
		t.Fatal(err)
	}
	if err = (ControlService{Store: s}).DeleteApp(context.Background(), controlapi.Actor{ID: "u", Active: true}, "alpha", "rollback"); err == nil {
		t.Fatal("app-delete failure accepted")
	}
	var status string
	var revoked sql.NullString
	if err = s.DB.QueryRow("SELECT status FROM applications WHERE id='a'").Scan(&status); err != nil || status != "deleting" {
		t.Fatal(status, err)
	}
	if err = s.DB.QueryRow("SELECT revoked_at FROM api_tokens WHERE app_id='a'").Scan(&revoked); err != nil || !revoked.Valid {
		t.Fatal(revoked, err)
	}
	if _, err = s.AuthenticateToken(context.Background(), raw, "app:read", "a", time.Now()); err == nil {
		t.Fatal("deleting app token remained usable")
	}
}

func TestControlDeleteAppPurgeFailureLeavesDeletingAndRetryable(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	raw, err := s.IssueToken(context.Background(), "u", "a", []string{"app:read"}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	purger := &lifecyclePurgeSpy{err: errors.New("private bytes unavailable")}
	svc := ControlService{Store: s, AppDataCleanup: purger}
	actor := controlapi.Actor{ID: "u", Active: true}
	if err := svc.DeleteApp(context.Background(), actor, "alpha", "first"); err == nil {
		t.Fatal("byte cleanup failure accepted")
	}
	var status string
	if err := s.DB.QueryRow("SELECT status FROM applications WHERE id='a'").Scan(&status); err != nil || status != "deleting" {
		t.Fatalf("status=%q err=%v", status, err)
	}
	if _, err := s.AuthenticateToken(context.Background(), raw, "app:read", "a", time.Now()); err == nil {
		t.Fatal("failed purge restored token access")
	}
	if purger.calls != 1 {
		t.Fatalf("purge calls=%d", purger.calls)
	}
	purger.err = nil
	if err := svc.DeleteApp(context.Background(), actor, "alpha", "retry"); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM applications WHERE id='a'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("app remained count=%d err=%v", count, err)
	}
	if purger.calls != 2 {
		t.Fatalf("retry did not purge calls=%d", purger.calls)
	}
}

func TestControlDeleteAppPurgesLegacyDeletedRow(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	if _, err := s.DB.Exec("UPDATE applications SET status='deleted' WHERE id='a'"); err != nil {
		t.Fatal(err)
	}
	if err := (ControlService{Store: s, AppDataCleanup: &lifecyclePurgeSpy{}}).DeleteApp(context.Background(), controlapi.Actor{ID: "u", Active: true}, "alpha", "legacy"); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM applications WHERE id='a'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("legacy row remained count=%d err=%v", count, err)
	}
}

func TestAppDataPurgerRemovesOnlyDeletedAppBlobNamespace(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	bytes := blob.LocalStore{Root: s.DataRoot}
	id := "blb_0123456789abcdef0123456789abcdef"
	for _, app := range []string{"a", "b"} {
		if _, _, err := bytes.Put(app, id, strings.NewReader("x"), 10); err != nil {
			t.Fatal(err)
		}
	}
	repo := &BlobRepository{Store: s, Bytes: bytes}
	if err := (AppDataPurger{DataRoot: s.DataRoot, Store: s, BlobCleanup: repo}).Purge(context.Background(), "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.DataRoot, "blobs", "a")); !os.IsNotExist(err) {
		t.Fatalf("target blob namespace remained: %v", err)
	}
	if _, err := os.Stat(filepath.Join(s.DataRoot, "blobs", "b")); err != nil {
		t.Fatalf("other blob namespace removed: %v", err)
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

func TestControlReplaceActiveDeployersReconcilesAndRevokes(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	_, err := s.DB.Exec("INSERT INTO users(id,normalized_email,role,status,created_at) VALUES('op','operator@example.com','operator','active',datetime('now')),('keep','keep@example.com','deployer','active',datetime('now')),('remove','remove@example.com','deployer','active',datetime('now')),('wake','wake@example.com','deployer','revoked',datetime('now')); UPDATE applications SET owner_user_id='remove' WHERE id='a'; INSERT INTO otp_challenges(id,app_id,purpose,normalized_email,code_hash,expires_at,attempts,created_at) VALUES('wake-otp',NULL,'control','wake@example.com',X'01',datetime('now','+1 hour'),0,datetime('now')),('remove-otp',NULL,'control','remove@example.com',X'01',datetime('now','+1 hour'),0,datetime('now'))")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := s.IssueToken(context.Background(), "remove", "", []string{"app:read"}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec("INSERT INTO sessions(id,scope,user_id,secret_hash,expires_at,created_at) VALUES('remove-session','control','remove',X'01',datetime('now','+1 hour'),datetime('now'))"); err != nil {
		t.Fatal(err)
	}
	svc := ControlService{Store: s}
	actor := controlapi.Actor{ID: "op", Role: "operator", Active: true}
	revision := deployerRevision([]string{"keep@example.com", "owner@example.com", "remove@example.com"})
	if err = svc.ReplaceActiveDeployers(context.Background(), actor, []string{"keep@example.com", "new@example.com", "wake@example.com"}, revision, true, "replace"); err != nil {
		t.Fatal(err)
	}
	var status, owner string
	if err = s.DB.QueryRow("SELECT status FROM users WHERE id='remove'").Scan(&status); err != nil || status != "revoked" {
		t.Fatal(status, err)
	}
	if err = s.DB.QueryRow("SELECT owner_user_id FROM applications WHERE id='a'").Scan(&owner); err != nil || owner != "remove" {
		t.Fatal(owner, err)
	}
	if _, err = s.AuthenticateToken(context.Background(), raw, "app:read", "", time.Now()); err == nil {
		t.Fatal("removed token remained valid")
	}
	var n int
	if err = s.DB.QueryRow("SELECT COUNT(*) FROM sessions WHERE id='remove-session' AND revoked_at IS NOT NULL").Scan(&n); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	if err = s.DB.QueryRow("SELECT COUNT(*) FROM otp_challenges WHERE id IN ('wake-otp','remove-otp') AND invalidated_at IS NOT NULL").Scan(&n); err != nil || n != 2 {
		t.Fatal(n, err)
	}
	if err = svc.ReplaceActiveDeployers(context.Background(), actor, []string{"keep@example.com", "new@example.com", "wake@example.com"}, revision, true, "replace"); err != nil {
		t.Fatal(err)
	}
	if err = svc.ReplaceActiveDeployers(context.Background(), actor, []string{"keep@example.com"}, revision, false, "replace"); err == nil {
		t.Fatal("idempotency mismatch accepted")
	}
}

func TestControlReplaceActiveDeployersDenialsAndAtomicAudit(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	if _, err := s.DB.Exec("INSERT INTO users(id,normalized_email,role,status,created_at) VALUES('op','operator@example.com','operator','active',datetime('now')),('d','old@example.com','deployer','active',datetime('now')); INSERT INTO api_tokens(id,user_id,secret_hash,scopes,expires_at) VALUES('token','d',X'01','app:read',datetime('now','+1 hour')); INSERT INTO sessions(id,scope,user_id,secret_hash,expires_at,created_at) VALUES('session','control','d',X'02',datetime('now','+1 hour'),datetime('now'))"); err != nil {
		t.Fatal(err)
	}
	svc := ControlService{Store: s}
	op := controlapi.Actor{ID: "op", Role: "operator", Active: true}
	revision := deployerRevision([]string{"old@example.com", "owner@example.com"})
	overBytes := make([]string, 100)
	for i := range overBytes {
		overBytes[i] = strings.Repeat("a", 90) + fmt.Sprintf("%03d@example.test", i)
	}
	for _, tc := range []struct {
		actor    controlapi.Actor
		emails   []string
		revision string
		confirm  bool
	}{{controlapi.Actor{ID: "d", Role: "deployer", Active: true}, []string{"old@example.com"}, revision, false}, {op, []string{"bad"}, revision, false}, {op, []string{"old@example.com", "old@example.com"}, revision, false}, {op, []string{"operator@example.com"}, revision, true}, {op, []string{"old@example.com", "new@example.com"}, revision, false}, {op, make([]string, 101), revision, true}, {op, overBytes, revision, true}} {
		if err := svc.ReplaceActiveDeployers(context.Background(), tc.actor, tc.emails, tc.revision, tc.confirm, "deny"+fmt.Sprint(len(tc.emails))); err == nil {
			t.Fatalf("accepted denied input %#v", tc)
		}
	}
	if err := svc.ReplaceActiveDeployers(context.Background(), op, []string{"old@example.com"}, "stale", false, "stale"); !errors.Is(err, controlapi.ErrDeployerRevision) {
		t.Fatalf("stale=%v", err)
	}
	var status string
	var tokenRevoked, sessionRevoked sql.NullString
	if err := s.DB.QueryRow("SELECT status FROM users WHERE id='d'").Scan(&status); err != nil || status != "active" {
		t.Fatalf("stale changed status %q %v", status, err)
	}
	if err := s.DB.QueryRow("SELECT revoked_at FROM api_tokens WHERE id='token'").Scan(&tokenRevoked); err != nil || tokenRevoked.Valid {
		t.Fatalf("stale changed token %v %v", tokenRevoked, err)
	}
	if err := s.DB.QueryRow("SELECT revoked_at FROM sessions WHERE id='session'").Scan(&sessionRevoked); err != nil || sessionRevoked.Valid {
		t.Fatalf("stale changed session %v %v", sessionRevoked, err)
	}
	if _, err := s.DB.Exec("CREATE TRIGGER deny_allowlist_audit BEFORE INSERT ON audit_events WHEN NEW.action='deployers.reconciled' BEGIN SELECT RAISE(ABORT,'deny'); END"); err != nil {
		t.Fatal(err)
	}
	if err := svc.ReplaceActiveDeployers(context.Background(), op, []string{}, revision, false, "audit"); err == nil {
		t.Fatal("audit failure accepted")
	}
	if err := s.DB.QueryRow("SELECT status FROM users WHERE id='d'").Scan(&status); err != nil || status != "active" {
		t.Fatalf("audit changed status %q %v", status, err)
	}
	if err := s.DB.QueryRow("SELECT revoked_at FROM api_tokens WHERE id='token'").Scan(&tokenRevoked); err != nil || tokenRevoked.Valid {
		t.Fatalf("audit changed token %v %v", tokenRevoked, err)
	}
	if err := s.DB.QueryRow("SELECT revoked_at FROM sessions WHERE id='session'").Scan(&sessionRevoked); err != nil || sessionRevoked.Valid {
		t.Fatalf("audit changed session %v %v", sessionRevoked, err)
	}
}
