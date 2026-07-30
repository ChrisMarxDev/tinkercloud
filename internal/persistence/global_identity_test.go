package persistence

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ChrisMarxDev/tinkercloud/internal/sessions"
)

func seedViewerPolicy(t *testing.T, s *SQLiteStore) {
	seedViewerPolicyFor(t, s, "a")
}

func seedViewerPolicyFor(t *testing.T, s *SQLiteStore, appID string) {
	t.Helper()
	if _, err := s.DB.Exec(`INSERT INTO access_policies(app_id,revision,mode,created_by,created_at) VALUES(?,1,'private','u',datetime('now'));
		INSERT INTO access_rules(id,app_id,policy_revision,kind,normalized_value,created_by,created_at) VALUES(? || '-viewer',?,1,'email','viewer@example.com','u',datetime('now'))`, appID, appID, appID); err != nil {
		t.Fatal(err)
	}
}

func TestGlobalIdentityOTPToAppHandoffAndDenials(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	seedViewerPolicy(t, s)
	now := time.Now().UTC()
	h, state, err := s.CreateIdentityHandoff(context.Background(), "a", "/report?tab=one", false, now)
	if err != nil || h.AppID != "a" || state == "" {
		t.Fatalf("create handoff: %#v %q %v", h, state, err)
	}
	msg, err := s.RequestIdentityOTP(context.Background(), h.ID, "browser", "viewer@example.com", "fingerprint", []byte("key"), now, time.Minute)
	if err != nil || msg == nil || msg.Code == "" || msg.AppID != "a" {
		t.Fatalf("request: %#v %v", msg, err)
	}
	verified, err := s.VerifyIdentityOTP(context.Background(), h.ID, "browser", "viewer@example.com", msg.ID, msg.Code, "", []byte("key"), now, 5)
	if err != nil || verified.Token == "" || verified.Session.Identity.Email != "viewer@example.com" {
		t.Fatalf("verify: %#v %v", verified, err)
	}
	gotHandoff, appRaw, app, err := s.ConsumeIdentityHandoff(context.Background(), "a", h.ID, state, now, now.Add(time.Hour))
	if err != nil || appRaw == "" || gotHandoff.ReturnPath != "/report?tab=one" || app.Identity.Email != "viewer@example.com" {
		t.Fatalf("consume: %#v %#v %v", gotHandoff, app, err)
	}
	if _, err = s.Validate(context.Background(), "a", appRaw, now); err != nil {
		t.Fatalf("derived app session invalid: %v", err)
	}
	if _, _, _, err = s.ConsumeIdentityHandoff(context.Background(), "a", h.ID, state, now, now.Add(time.Hour)); !errors.Is(err, ErrIdentity) {
		t.Fatalf("replay consume: %v", err)
	}
	if _, _, err = s.AuthorizeIdentityHandoff(context.Background(), h.ID, verified.Token, now); !errors.Is(err, ErrIdentity) {
		t.Fatalf("consumed handoff reauthorized: %v", err)
	}

	wrong, wrongState, err := s.CreateIdentityHandoff(context.Background(), "a", "/", false, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.AuthorizeIdentityHandoff(context.Background(), wrong.ID, verified.Token, now); err != nil {
		t.Fatal(err)
	}
	if repeat, err := s.RequestIdentityOTP(context.Background(), wrong.ID, "browser", "viewer@example.com", "fingerprint", []byte("key"), now, time.Minute); err != nil || repeat != nil {
		t.Fatalf("already-authorized handoff sent OTP: %#v %v", repeat, err)
	}
	if _, _, _, err = s.ConsumeIdentityHandoff(context.Background(), "b", wrong.ID, wrongState, now, now.Add(time.Hour)); !errors.Is(err, ErrIdentity) {
		t.Fatalf("wrong app accepted: %v", err)
	}
	if _, _, _, err = s.ConsumeIdentityHandoff(context.Background(), "a", wrong.ID, "not-the-state", now, now.Add(time.Hour)); !errors.Is(err, ErrIdentity) {
		t.Fatalf("wrong state accepted: %v", err)
	}
}

func TestDashboardIdentityUsesCurrentActiveRoleAndNeverViewerPermission(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	seedViewerPolicy(t, s)
	now := time.Now().UTC()

	// The app owner is an implicit viewer, so this creates the one global
	// browser identity without granting dashboard authority through the app
	// policy itself.
	h, _, err := s.CreateIdentityHandoff(context.Background(), "a", "/", false, now)
	if err != nil {
		t.Fatal(err)
	}
	msg, err := s.RequestIdentityOTP(context.Background(), h.ID, "browser-binding", "owner@example.com", "fp", []byte("key"), now, time.Minute)
	if err != nil || msg == nil {
		t.Fatalf("request global identity: %#v %v", msg, err)
	}
	issued, err := s.VerifyIdentityOTP(context.Background(), h.ID, "browser-binding", "owner@example.com", msg.ID, msg.Code, "", []byte("key"), now, 5)
	if err != nil {
		t.Fatalf("issue global identity: %v", err)
	}

	got, err := s.AuthenticateDashboardIdentity(context.Background(), issued.Token, now.Add(time.Second))
	if err != nil || got.Actor.ID != "u" || got.Actor.Role != "deployer" || !got.Actor.Active {
		t.Fatalf("dashboard actor=%#v err=%v", got.Actor, err)
	}

	// Role/status are intentionally not cached in the identity family. The
	// next request observes a suspension immediately.
	if _, err = s.DB.Exec("UPDATE users SET status='suspended' WHERE id='u'"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AuthenticateDashboardIdentity(context.Background(), issued.Token, now.Add(2*time.Second)); !errors.Is(err, ErrIdentity) {
		t.Fatalf("inactive deployer received dashboard authority: %v", err)
	}
	if _, err = s.DB.Exec("UPDATE users SET status='active', role='operator' WHERE id='u'"); err != nil {
		t.Fatal(err)
	}
	got, err = s.AuthenticateDashboardIdentity(context.Background(), issued.Token, now.Add(3*time.Second))
	if err != nil || got.Actor.Role != "operator" {
		t.Fatalf("current role not applied actor=%#v err=%v", got.Actor, err)
	}

	// A viewer identity with no active operator/deployer row can complete app
	// handoffs, but is never dashboard authority merely because its email is a
	// valid identity.
	viewerHandoff, _, err := s.CreateIdentityHandoff(context.Background(), "a", "/", false, now.Add(4*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	viewerMsg, err := s.RequestIdentityOTP(context.Background(), viewerHandoff.ID, "viewer-binding", "viewer@example.com", "fp", []byte("key"), now.Add(4*time.Second), time.Minute)
	if err != nil || viewerMsg == nil {
		t.Fatalf("request viewer identity: %#v %v", viewerMsg, err)
	}
	viewer, err := s.VerifyIdentityOTP(context.Background(), viewerHandoff.ID, "viewer-binding", "viewer@example.com", viewerMsg.ID, viewerMsg.Code, "", []byte("key"), now.Add(4*time.Second), 5)
	if err != nil {
		t.Fatal(err)
	}
	viewerDashboard, err := s.AuthenticateDashboardIdentity(context.Background(), viewer.Token, now.Add(5*time.Second))
	if !errors.Is(err, ErrIdentity) {
		t.Fatalf("viewer identity granted dashboard access: %v", err)
	}
	if viewerDashboard.Identity.ID == "" || viewerDashboard.Identity.Identity.Email != "viewer@example.com" {
		t.Fatalf("dashboard role denial discarded valid viewer identity: %#v", viewerDashboard.Identity)
	}
}

func TestDashboardIdentityDeniesExpiredAndRevokedGlobalCredentials(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	seedViewerPolicy(t, s)
	now := time.Now().UTC()
	h, _, err := s.CreateIdentityHandoff(context.Background(), "a", "/", false, now)
	if err != nil {
		t.Fatal(err)
	}
	msg, err := s.RequestIdentityOTP(context.Background(), h.ID, "browser-binding", "owner@example.com", "fp", []byte("key"), now, time.Minute)
	if err != nil || msg == nil {
		t.Fatal(err)
	}
	issued, err := s.VerifyIdentityOTP(context.Background(), h.ID, "browser-binding", "owner@example.com", msg.ID, msg.Code, "", []byte("key"), now, 5)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec("UPDATE identity_sessions SET expires_at=? WHERE id=?", now.Add(-time.Minute).Format(time.RFC3339Nano), issued.Session.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AuthenticateDashboardIdentity(context.Background(), issued.Token, now); !errors.Is(err, ErrIdentity) {
		t.Fatalf("expired global identity authorized dashboard: %v", err)
	}
	if _, err = s.DB.Exec("UPDATE identity_sessions SET expires_at=? WHERE id=?", now.Add(time.Hour).Format(time.RFC3339Nano), issued.Session.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.RevokeIdentitySession(context.Background(), issued.Token, now); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AuthenticateDashboardIdentity(context.Background(), issued.Token, now.Add(time.Second)); !errors.Is(err, ErrIdentity) {
		t.Fatalf("revoked global identity authorized dashboard: %v", err)
	}
}

func TestPlatformIdentityOTPIsRoleAgnosticAndDashboardLookupRemainsSeparate(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	now := time.Now().UTC()
	challenge, err := s.RequestPlatformIdentityOTP(context.Background(), "platform-browser", "viewer@example.com", "fp", []byte("key"), now, time.Minute)
	if err != nil || challenge == nil || challenge.Code == "" || challenge.Email != "viewer@example.com" {
		t.Fatalf("request platform identity: %#v %v", challenge, err)
	}
	issued, err := s.VerifyPlatformIdentityOTP(context.Background(), "platform-browser", "viewer@example.com", challenge.ID, challenge.Code, "", false, []byte("key"), now, 5)
	if err != nil || issued.Token == "" || issued.Session.Identity.Email != "viewer@example.com" {
		t.Fatalf("verify platform identity: %#v %v", issued, err)
	}
	if _, err = s.AuthenticateDashboardIdentity(context.Background(), issued.Token, now.Add(time.Second)); !errors.Is(err, ErrIdentity) {
		t.Fatalf("viewer received dashboard authority: %v", err)
	}
	var appID sql.NullString
	if err = s.DB.QueryRow("SELECT app_id FROM platform_identity_challenges WHERE id=?", challenge.ID).Scan(&appID); err == nil {
		t.Fatal("platform challenge unexpectedly has app column")
	}
}

func TestPlatformIdentityOTPDenyAndSwitchRevokesFamily(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	seedViewerPolicy(t, s)
	now := time.Now().UTC()
	first, err := s.RequestPlatformIdentityOTP(context.Background(), "platform-browser", "owner@example.com", "fp", []byte("key"), now, time.Minute)
	if err != nil || first == nil {
		t.Fatal(err)
	}
	if _, err = s.VerifyPlatformIdentityOTP(context.Background(), "wrong-browser", "owner@example.com", first.ID, first.Code, "", false, []byte("key"), now, 5); !errors.Is(err, ErrOTP) && !errors.Is(err, ErrIdentity) {
		t.Fatalf("mismatched binding accepted: %v", err)
	}
	issued, err := s.VerifyPlatformIdentityOTP(context.Background(), "platform-browser", "owner@example.com", first.ID, first.Code, "", false, []byte("key"), now, 5)
	if err != nil {
		t.Fatal(err)
	}
	// Establish a child app session; a forced platform account switch must
	// return it for post-commit live-connection closure and revoke it durably.
	h, state, err := s.CreateIdentityHandoff(context.Background(), "a", "/", false, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.AuthorizeIdentityHandoff(context.Background(), h.ID, issued.Token, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err = s.ConsumeIdentityHandoff(context.Background(), "a", h.ID, state, now.Add(time.Second), now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	switchChallenge, err := s.RequestPlatformIdentityOTP(context.Background(), "platform-browser", "viewer@example.com", "fp", []byte("key"), now.Add(2*time.Second), time.Minute)
	if err != nil || switchChallenge == nil {
		t.Fatal(err)
	}
	switched, err := s.VerifyPlatformIdentityOTP(context.Background(), "platform-browser", "viewer@example.com", switchChallenge.ID, switchChallenge.Code, issued.Token, true, []byte("key"), now.Add(2*time.Second), 5)
	if err != nil || switched.Token == "" || len(switched.Revoked) != 1 {
		t.Fatalf("switch=%#v err=%v", switched, err)
	}
	if _, err = s.ValidateIdentitySession(context.Background(), issued.Token, now.Add(3*time.Second)); !errors.Is(err, ErrIdentity) {
		t.Fatalf("old family survived platform switch: %v", err)
	}
}

func TestPlatformIdentityOTPConcurrentCompletionsYieldOneFamily(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	now := time.Now().UTC()
	first, err := s.RequestPlatformIdentityOTP(context.Background(), "race-browser", "one@example.com", "one", []byte("key"), now, time.Minute)
	if err != nil || first == nil {
		t.Fatal(err)
	}
	second, err := s.RequestPlatformIdentityOTP(context.Background(), "race-browser", "two@example.com", "two", []byte("key"), now, time.Minute)
	if err != nil || second == nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	go func() {
		_, e := s.VerifyPlatformIdentityOTP(context.Background(), "race-browser", "one@example.com", first.ID, first.Code, "", false, []byte("key"), now.Add(time.Second), 5)
		results <- e
	}()
	go func() {
		_, e := s.VerifyPlatformIdentityOTP(context.Background(), "race-browser", "two@example.com", second.ID, second.Code, "", false, []byte("key"), now.Add(time.Second), 5)
		results <- e
	}()
	ok := 0
	for range 2 {
		if err := <-results; err == nil {
			ok++
		} else if !errors.Is(err, ErrOTP) {
			t.Fatalf("unexpected concurrent verification error: %v", err)
		}
	}
	if ok != 1 {
		t.Fatalf("concurrent platform completions issued %d identities", ok)
	}
	var families int
	binding := sha256.Sum256([]byte("race-browser"))
	if err = s.DB.QueryRow("SELECT COUNT(DISTINCT family_id) FROM identity_sessions WHERE browser_binding_hash=? AND revoked_at IS NULL", binding[:]).Scan(&families); err != nil || families != 1 {
		t.Fatalf("active families=%d err=%v", families, err)
	}
}

func TestGlobalIdentityRotationOverlapAndRevokeChildren(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	seedViewerPolicy(t, s)
	now := time.Now().UTC()
	h, state, err := s.CreateIdentityHandoff(context.Background(), "a", "/", false, now)
	if err != nil {
		t.Fatal(err)
	}
	msg, _ := s.RequestIdentityOTP(context.Background(), h.ID, "browser", "viewer@example.com", "fp", []byte("key"), now, time.Minute)
	verified, err := s.VerifyIdentityOTP(context.Background(), h.ID, "browser", "viewer@example.com", msg.ID, msg.Code, "", []byte("key"), now, 5)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err = s.ConsumeIdentityHandoff(context.Background(), "a", h.ID, state, now, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec("UPDATE identity_sessions SET rotated_at=? WHERE id=?", now.Add(-25*time.Hour).Format(time.RFC3339Nano), verified.Session.ID); err != nil {
		t.Fatal(err)
	}
	validation, err := s.ValidateIdentitySession(context.Background(), verified.Token, now)
	replacement := validation.ReplacementToken
	if err != nil || replacement == "" {
		t.Fatalf("rotation result=%#v err=%v", validation, err)
	}
	if _, err = s.ValidateIdentitySession(context.Background(), verified.Token, now.Add(30*time.Second)); err != nil {
		t.Fatalf("previous token during overlap: %v", err)
	}
	stale, err := s.ValidateIdentitySession(context.Background(), verified.Token, now.Add(2*time.Minute))
	if !errors.Is(err, ErrIdentity) || !stale.FamilyRevoked || len(stale.Revoked) != 1 || stale.Revoked[0].AppID != "a" {
		t.Fatalf("stale previous result=%#v err=%v", stale, err)
	}
	if _, err = s.ValidateIdentitySession(context.Background(), replacement, now.Add(2*time.Minute)); !errors.Is(err, ErrIdentity) {
		t.Fatalf("current sibling survived stale previous replay: %v", err)
	}
	var n int
	if err = s.DB.QueryRow("SELECT COUNT(*) FROM sessions WHERE identity_session_id=? AND revoked_at IS NOT NULL", verified.Session.ID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("child revoke n=%d err=%v", n, err)
	}
}

func TestRotatedOutIdentityReplayRevokesWholeFamilyAndPendingGrants(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	seedViewerPolicy(t, s)
	now := time.Now().UTC()

	first, firstState, err := s.CreateIdentityHandoff(context.Background(), "a", "/", false, now)
	if err != nil {
		t.Fatal(err)
	}
	msg, err := s.RequestIdentityOTP(context.Background(), first.ID, "browser", "viewer@example.com", "fp", []byte("key"), now, time.Minute)
	if err != nil || msg == nil {
		t.Fatalf("request initial OTP: %#v %v", msg, err)
	}
	issued, err := s.VerifyIdentityOTP(context.Background(), first.ID, "browser", "viewer@example.com", msg.ID, msg.Code, "", []byte("key"), now, 5)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err = s.ConsumeIdentityHandoff(context.Background(), "a", first.ID, firstState, now, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	// A family must remain the revocation boundary even if a future session
	// strategy creates more than one backing row for it.
	var siblingRaw string
	var sibling IdentitySession
	if err = s.Write(context.Background(), func(tx *sql.Tx) error {
		var e error
		siblingBinding := sha256.Sum256([]byte("browser-sibling"))
		siblingRaw, sibling, e = createIdentitySessionTx(context.Background(), tx, issued.Session.Identity, siblingBinding[:], now)
		if e != nil {
			return e
		}
		if _, e = tx.ExecContext(context.Background(), "UPDATE identity_sessions SET family_id=? WHERE id=?", issued.Session.FamilyID, sibling.ID); e != nil {
			return e
		}
		_, _, e = issueChildAppSessionTx(context.Background(), tx, "a", issued.Session.Identity, sibling.ID, now.Add(time.Hour))
		return e
	}); err != nil {
		t.Fatal(err)
	}
	pending, _, err := s.CreateIdentityHandoff(context.Background(), "a", "/", false, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec("UPDATE identity_handoffs SET identity_session_id=?,authorized_at=? WHERE id=?", sibling.ID, now.Format(time.RFC3339Nano), pending.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec("UPDATE identity_sessions SET rotated_at=? WHERE id=?", now.Add(-identityRotateAfter-time.Second).Format(time.RFC3339Nano), issued.Session.ID); err != nil {
		t.Fatal(err)
	}
	rotation, err := s.ValidateIdentitySession(context.Background(), issued.Token, now)
	if err != nil || rotation.ReplacementToken == "" {
		t.Fatalf("rotation=%#v err=%v", rotation, err)
	}
	replay, err := s.ValidateIdentitySession(context.Background(), issued.Token, now.Add(identityPreviousOverlap+time.Second))
	if !errors.Is(err, ErrIdentity) || !replay.FamilyRevoked || len(replay.Revoked) != 2 {
		t.Fatalf("replay=%#v err=%v", replay, err)
	}
	for _, token := range []string{rotation.ReplacementToken, siblingRaw} {
		if _, err = s.ValidateIdentitySession(context.Background(), token, now.Add(identityPreviousOverlap+time.Second)); !errors.Is(err, ErrIdentity) {
			t.Fatalf("family credential survived replay: %v", err)
		}
	}
	if _, err = s.GetIdentityHandoff(context.Background(), pending.ID, now.Add(identityPreviousOverlap+time.Second)); !errors.Is(err, ErrIdentity) {
		t.Fatalf("authorized pending grant survived replay: %v", err)
	}
	var revokedSessions, revokedHandoffs int
	if err = s.DB.QueryRow("SELECT COUNT(*) FROM identity_sessions WHERE family_id=? AND revoked_at IS NOT NULL", issued.Session.FamilyID).Scan(&revokedSessions); err != nil || revokedSessions != 2 {
		t.Fatalf("family revocations=%d err=%v", revokedSessions, err)
	}
	if err = s.DB.QueryRow("SELECT COUNT(*) FROM identity_handoffs WHERE id=? AND revoked_at IS NOT NULL", pending.ID).Scan(&revokedHandoffs); err != nil || revokedHandoffs != 1 {
		t.Fatalf("pending handoff revocations=%d err=%v", revokedHandoffs, err)
	}
}

func TestCreateIdentityHandoffCleansBoundedExpiredRecords(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	now := time.Now().UTC()
	for i := 0; i < handoffCleanupLimit+3; i++ {
		id := fmt.Sprintf("expired-%03d", i)
		stateHash := sha256.Sum256([]byte(id))
		if _, err := s.DB.Exec(`INSERT INTO identity_handoffs(id,app_id,state_hash,return_path,force_login,expires_at,created_at)
			VALUES(?,?,?,?,?,?,?)`, id, "a", stateHash[:], "/", 0, now.Add(-time.Minute).Format(time.RFC3339Nano), now.Add(time.Duration(i)*time.Nanosecond).Format(time.RFC3339Nano)); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := s.CreateIdentityHandoff(context.Background(), "a", "/", false, now); err != nil {
		t.Fatalf("create after cleanup: %v", err)
	}
	var remaining int
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM identity_handoffs WHERE expires_at <= ?", now.Format(time.RFC3339Nano)).Scan(&remaining); err != nil || remaining != 3 {
		t.Fatalf("first bounded cleanup left=%d err=%v", remaining, err)
	}
	if _, _, err := s.CreateIdentityHandoff(context.Background(), "a", "/", false, now); err != nil {
		t.Fatalf("second create after cleanup: %v", err)
	}
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM identity_handoffs WHERE expires_at <= ?", now.Format(time.RFC3339Nano)).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatalf("second bounded cleanup left=%d err=%v", remaining, err)
	}
}

func TestRevokeIdentitySessionPropagatesPersistenceFailure(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	// This is deliberate failure injection: a missing backing table must not
	// be flattened into ErrIdentity, because the HTTP layer would otherwise
	// clear the browser cookie and falsely report a completed logout.
	if _, err := s.DB.Exec("DROP TABLE identity_sessions"); err != nil {
		t.Fatal(err)
	}
	_, err := s.RevokeIdentitySession(context.Background(), "gid_unavailable", time.Now().UTC())
	if err == nil || errors.Is(err, ErrIdentity) {
		t.Fatalf("persistence failure was flattened to identity denial: %v", err)
	}
}

func TestGlobalIdentityRevocationInvalidatesAuthorizedPendingHandoff(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	seedViewerPolicy(t, s)
	now := time.Now().UTC()
	first, _, err := s.CreateIdentityHandoff(context.Background(), "a", "/", false, now)
	if err != nil {
		t.Fatal(err)
	}
	msg, _ := s.RequestIdentityOTP(context.Background(), first.ID, "browser", "viewer@example.com", "fp", []byte("key"), now, time.Minute)
	verified, err := s.VerifyIdentityOTP(context.Background(), first.ID, "browser", "viewer@example.com", msg.ID, msg.Code, "", []byte("key"), now, 5)
	if err != nil {
		t.Fatal(err)
	}
	pending, state, err := s.CreateIdentityHandoff(context.Background(), "a", "/", false, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.AuthorizeIdentityHandoff(context.Background(), pending.ID, verified.Token, now); err != nil {
		t.Fatal(err)
	}
	if _, err = s.RevokeIdentitySession(context.Background(), verified.Token, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err = s.ConsumeIdentityHandoff(context.Background(), "a", pending.ID, state, now.Add(2*time.Second), now.Add(time.Hour)); !errors.Is(err, ErrIdentity) {
		t.Fatalf("revoked identity consumed pending handoff: %v", err)
	}
}

func TestGlobalIdentityConcurrentHandoffConsumeOnce(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	seedViewerPolicy(t, s)
	now := time.Now().UTC()
	h, state, _ := s.CreateIdentityHandoff(context.Background(), "a", "/", false, now)
	msg, _ := s.RequestIdentityOTP(context.Background(), h.ID, "browser", "viewer@example.com", "fp", []byte("key"), now, time.Minute)
	verified, err := s.VerifyIdentityOTP(context.Background(), h.ID, "browser", "viewer@example.com", msg.ID, msg.Code, "", []byte("key"), now, 5)
	if err != nil {
		t.Fatal(err)
	}
	_ = verified
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, _, err := s.ConsumeIdentityHandoff(context.Background(), "a", h.ID, state, now, now.Add(time.Hour))
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	good := 0
	for err := range results {
		if err == nil {
			good++
		} else if !errors.Is(err, ErrIdentity) {
			t.Fatalf("unexpected concurrent consume: %v", err)
		}
	}
	if good != 1 {
		t.Fatalf("consume successes=%d", good)
	}
}

func TestGlobalIdentityConcurrentRotationIssuesOneReplacement(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	seedViewerPolicy(t, s)
	now := time.Now().UTC()
	h, _, err := s.CreateIdentityHandoff(context.Background(), "a", "/", false, now)
	if err != nil {
		t.Fatal(err)
	}
	msg, _ := s.RequestIdentityOTP(context.Background(), h.ID, "browser", "viewer@example.com", "fp", []byte("key"), now, time.Minute)
	verified, err := s.VerifyIdentityOTP(context.Background(), h.ID, "browser", "viewer@example.com", msg.ID, msg.Code, "", []byte("key"), now, 5)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec("UPDATE identity_sessions SET rotated_at=? WHERE id=?", now.Add(-25*time.Hour).Format(time.RFC3339Nano), verified.Session.ID); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	tokens := make(chan string, 2)
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := s.ValidateIdentitySession(context.Background(), verified.Token, now)
			tokens <- result.ReplacementToken
			errs <- err
		}()
	}
	wg.Wait()
	close(tokens)
	close(errs)
	rotated := 0
	for err := range errs {
		if err != nil {
			t.Fatalf("rotation error: %v", err)
		}
	}
	for token := range tokens {
		if token != "" {
			rotated++
		}
	}
	if rotated != 1 {
		t.Fatalf("rotation replacements=%d", rotated)
	}
}

func seedSecondAllowedApp(t *testing.T, s *SQLiteStore) {
	t.Helper()
	if _, err := s.DB.Exec("UPDATE applications SET status='active' WHERE id='b'"); err != nil {
		t.Fatal(err)
	}
	seedViewerPolicyFor(t, s, "b")
}

func TestGlobalIdentitySequentialStaleOTPFirstCompletionWins(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	seedViewerPolicy(t, s)
	seedSecondAllowedApp(t, s)
	base := time.Date(2026, time.July, 28, 12, 0, 0, 0, time.UTC)

	first, _, err := s.CreateIdentityHandoff(context.Background(), "a", "/", false, base)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := s.CreateIdentityHandoff(context.Background(), "b", "/", false, base)
	if err != nil {
		t.Fatal(err)
	}
	firstMessage, err := s.RequestIdentityOTP(context.Background(), first.ID, "browser", "viewer@example.com", "first", []byte("key"), base, time.Minute)
	if err != nil || firstMessage == nil {
		t.Fatalf("first OTP: %#v %v", firstMessage, err)
	}
	secondMessage, err := s.RequestIdentityOTP(context.Background(), second.ID, "browser", "viewer@example.com", "second", []byte("key"), base, time.Minute)
	if err != nil || secondMessage == nil {
		t.Fatalf("second OTP: %#v %v", secondMessage, err)
	}

	if _, err = s.VerifyIdentityOTP(context.Background(), first.ID, "browser", "viewer@example.com", firstMessage.ID, firstMessage.Code, "", []byte("key"), base.Add(time.Second), 5); err != nil {
		t.Fatalf("first completion: %v", err)
	}
	if _, err = s.VerifyIdentityOTP(context.Background(), second.ID, "browser", "viewer@example.com", secondMessage.ID, secondMessage.Code, "", []byte("key"), base.Add(2*time.Second), 5); !errors.Is(err, ErrOTP) {
		t.Fatalf("stale completion=%v, want generic denial", err)
	}

	var activeSessions, activeFamilies, authorized int
	if err = s.DB.QueryRow("SELECT COUNT(*) FROM identity_sessions WHERE revoked_at IS NULL").Scan(&activeSessions); err != nil || activeSessions != 1 {
		t.Fatalf("active global sessions=%d err=%v", activeSessions, err)
	}
	if err = s.DB.QueryRow("SELECT COUNT(DISTINCT family_id) FROM identity_sessions WHERE revoked_at IS NULL").Scan(&activeFamilies); err != nil || activeFamilies != 1 {
		t.Fatalf("active global families=%d err=%v", activeFamilies, err)
	}
	if err = s.DB.QueryRow("SELECT COUNT(*) FROM identity_handoffs WHERE id=? AND authorized_at IS NOT NULL", second.ID).Scan(&authorized); err != nil || authorized != 0 {
		t.Fatalf("stale handoff authorization=%d err=%v", authorized, err)
	}
}

func TestGlobalIdentityConcurrentOTPFirstCompletionWins(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	seedViewerPolicy(t, s)
	seedSecondAllowedApp(t, s)
	base := time.Date(2026, time.July, 28, 12, 30, 0, 0, time.UTC)

	first, _, err := s.CreateIdentityHandoff(context.Background(), "a", "/", false, base)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := s.CreateIdentityHandoff(context.Background(), "b", "/", false, base)
	if err != nil {
		t.Fatal(err)
	}
	firstMessage, err := s.RequestIdentityOTP(context.Background(), first.ID, "browser", "viewer@example.com", "first", []byte("key"), base, time.Minute)
	if err != nil || firstMessage == nil {
		t.Fatalf("first OTP: %#v %v", firstMessage, err)
	}
	secondMessage, err := s.RequestIdentityOTP(context.Background(), second.ID, "browser", "viewer@example.com", "second", []byte("key"), base, time.Minute)
	if err != nil || secondMessage == nil {
		t.Fatalf("second OTP: %#v %v", secondMessage, err)
	}

	type completion struct {
		handoffID string
		err       error
	}
	results := make(chan completion, 2)
	complete := func(h IdentityHandoff, message *ChallengeMessage) {
		_, e := s.VerifyIdentityOTP(context.Background(), h.ID, "browser", "viewer@example.com", message.ID, message.Code, "", []byte("key"), base.Add(time.Second), 5)
		results <- completion{handoffID: h.ID, err: e}
	}
	go complete(first, firstMessage)
	go complete(second, secondMessage)
	firstResult := <-results
	secondResult := <-results

	successes := 0
	loserID := ""
	for _, result := range []completion{firstResult, secondResult} {
		if result.err == nil {
			successes++
			continue
		}
		if !errors.Is(result.err, ErrOTP) {
			t.Fatalf("concurrent completion %s: %v", result.handoffID, result.err)
		}
		loserID = result.handoffID
	}
	if successes != 1 || loserID == "" {
		t.Fatalf("concurrent completions successes=%d loser=%q", successes, loserID)
	}
	var activeSessions, activeFamilies, authorized int
	if err = s.DB.QueryRow("SELECT COUNT(*) FROM identity_sessions WHERE revoked_at IS NULL").Scan(&activeSessions); err != nil || activeSessions != 1 {
		t.Fatalf("active global sessions=%d err=%v", activeSessions, err)
	}
	if err = s.DB.QueryRow("SELECT COUNT(DISTINCT family_id) FROM identity_sessions WHERE revoked_at IS NULL").Scan(&activeFamilies); err != nil || activeFamilies != 1 {
		t.Fatalf("active global families=%d err=%v", activeFamilies, err)
	}
	if err = s.DB.QueryRow("SELECT COUNT(*) FROM identity_handoffs WHERE id=? AND authorized_at IS NOT NULL", loserID).Scan(&authorized); err != nil || authorized != 0 {
		t.Fatalf("losing handoff authorization=%d err=%v", authorized, err)
	}
}

func TestGlobalIdentityNonForceOTPRejectsCurrentPresentedIdentity(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	seedViewerPolicy(t, s)
	base := time.Date(2026, time.July, 28, 13, 0, 0, 0, time.UTC)

	initial, _, err := s.CreateIdentityHandoff(context.Background(), "a", "/", false, base)
	if err != nil {
		t.Fatal(err)
	}
	initialMessage, _ := s.RequestIdentityOTP(context.Background(), initial.ID, "browser", "viewer@example.com", "initial", []byte("key"), base, time.Minute)
	current, err := s.VerifyIdentityOTP(context.Background(), initial.ID, "browser", "viewer@example.com", initialMessage.ID, initialMessage.Code, "", []byte("key"), base.Add(time.Second), 5)
	if err != nil {
		t.Fatal(err)
	}

	pending, _, err := s.CreateIdentityHandoff(context.Background(), "a", "/", false, base.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	pendingMessage, _ := s.RequestIdentityOTP(context.Background(), pending.ID, "browser", "viewer@example.com", "pending", []byte("key"), base.Add(2*time.Second), time.Minute)
	if _, err = s.VerifyIdentityOTP(context.Background(), pending.ID, "browser", "viewer@example.com", pendingMessage.ID, pendingMessage.Code, current.Token, []byte("key"), base.Add(3*time.Second), 5); !errors.Is(err, ErrOTP) {
		t.Fatalf("non-force completion replaced current identity: %v", err)
	}
	var active, authorized int
	if err = s.DB.QueryRow("SELECT COUNT(*) FROM identity_sessions WHERE revoked_at IS NULL").Scan(&active); err != nil || active != 1 {
		t.Fatalf("active globals=%d err=%v", active, err)
	}
	if err = s.DB.QueryRow("SELECT COUNT(*) FROM identity_handoffs WHERE id=? AND authorized_at IS NOT NULL", pending.ID).Scan(&authorized); err != nil || authorized != 0 {
		t.Fatalf("non-force handoff authorization=%d err=%v", authorized, err)
	}
}

func TestGlobalIdentityNonForceOTPRejectsRotatedOutPresentedIdentity(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	seedViewerPolicy(t, s)
	base := time.Date(2026, time.July, 28, 13, 30, 0, 0, time.UTC)

	initial, _, err := s.CreateIdentityHandoff(context.Background(), "a", "/", false, base)
	if err != nil {
		t.Fatal(err)
	}
	initialMessage, _ := s.RequestIdentityOTP(context.Background(), initial.ID, "browser", "viewer@example.com", "initial", []byte("key"), base, 10*time.Minute)
	current, err := s.VerifyIdentityOTP(context.Background(), initial.ID, "browser", "viewer@example.com", initialMessage.ID, initialMessage.Code, "", []byte("key"), base.Add(time.Second), 5)
	if err != nil {
		t.Fatal(err)
	}

	pending, _, err := s.CreateIdentityHandoff(context.Background(), "a", "/", false, base.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	pendingMessage, _ := s.RequestIdentityOTP(context.Background(), pending.ID, "browser", "viewer@example.com", "pending", []byte("key"), base.Add(2*time.Second), 10*time.Minute)
	if _, err = s.DB.Exec("UPDATE identity_sessions SET rotated_at=? WHERE id=?", base.Add(-identityRotateAfter).Format(time.RFC3339Nano), current.Session.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ValidateIdentitySession(context.Background(), current.Token, base.Add(3*time.Second)); err != nil {
		t.Fatalf("rotate identity: %v", err)
	}
	if _, err = s.VerifyIdentityOTP(context.Background(), pending.ID, "browser", "viewer@example.com", pendingMessage.ID, pendingMessage.Code, current.Token, []byte("key"), base.Add(2*time.Minute), 5); !errors.Is(err, ErrOTP) {
		t.Fatalf("rotated-out token minted a replacement global identity: %v", err)
	}
	var active, authorized int
	if err = s.DB.QueryRow("SELECT COUNT(*) FROM identity_sessions WHERE revoked_at IS NULL").Scan(&active); err != nil || active != 1 {
		t.Fatalf("active globals=%d err=%v", active, err)
	}
	if err = s.DB.QueryRow("SELECT COUNT(*) FROM identity_handoffs WHERE id=? AND authorized_at IS NOT NULL", pending.ID).Scan(&authorized); err != nil || authorized != 0 {
		t.Fatalf("rotated-out non-force handoff authorization=%d err=%v", authorized, err)
	}
}

func TestGlobalIdentityForceOTPRejectsNewerBrowserCompletion(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	seedViewerPolicy(t, s)
	seedSecondAllowedApp(t, s)
	base := time.Date(2026, time.July, 28, 14, 0, 0, 0, time.UTC)

	force, _, err := s.CreateIdentityHandoff(context.Background(), "a", "/", true, base)
	if err != nil {
		t.Fatal(err)
	}
	forceMessage, _ := s.RequestIdentityOTP(context.Background(), force.ID, "browser", "viewer@example.com", "force", []byte("key"), base, 10*time.Minute)
	other, _, err := s.CreateIdentityHandoff(context.Background(), "b", "/", false, base)
	if err != nil {
		t.Fatal(err)
	}
	otherMessage, _ := s.RequestIdentityOTP(context.Background(), other.ID, "browser", "viewer@example.com", "other", []byte("key"), base, 10*time.Minute)
	current, err := s.VerifyIdentityOTP(context.Background(), other.ID, "browser", "viewer@example.com", otherMessage.ID, otherMessage.Code, "", []byte("key"), base.Add(time.Second), 5)
	if err != nil {
		t.Fatalf("current identity: %v", err)
	}
	if _, err = s.VerifyIdentityOTP(context.Background(), force.ID, "browser", "viewer@example.com", forceMessage.ID, forceMessage.Code, current.Token, []byte("key"), base.Add(2*time.Second), 5); !errors.Is(err, ErrOTP) {
		t.Fatalf("stale force form replaced newer browser identity: %v", err)
	}
	if _, err = s.ValidateIdentitySession(context.Background(), current.Token, base.Add(3*time.Second)); err != nil {
		t.Fatalf("newer browser identity did not survive stale force form: %v", err)
	}
}

func TestGlobalIdentityHandoffRejectsUnsafeReturnAndUnauthorizedEmail(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	seedViewerPolicy(t, s)
	now := time.Now()
	if _, _, err := s.CreateIdentityHandoff(context.Background(), "a", "https://attacker.invalid", false, now); !errors.Is(err, ErrIdentity) {
		t.Fatalf("unsafe return: %v", err)
	}
	h, _, err := s.CreateIdentityHandoff(context.Background(), "a", "/", false, now)
	if err != nil {
		t.Fatal(err)
	}
	msg, err := s.RequestIdentityOTP(context.Background(), h.ID, "browser", "outsider@example.com", "fp", []byte("key"), now, time.Minute)
	if err != nil || msg != nil {
		t.Fatalf("ineligible request msg=%#v err=%v", msg, err)
	}
	if err = s.ForceIdentityHandoff(context.Background(), h.ID, now); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetIdentityHandoff(context.Background(), h.ID, now)
	if err != nil || !got.ForceLogin || got.AppSlug != "alpha" {
		t.Fatalf("force/get %#v %v", got, err)
	}
}

func TestForceSwitchRevokesPriorIdentityChildrenAndReturnsRefs(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	seedViewerPolicy(t, s)
	now := time.Now().UTC()
	first, firstState, err := s.CreateIdentityHandoff(context.Background(), "a", "/", false, now)
	if err != nil {
		t.Fatal(err)
	}
	msg, _ := s.RequestIdentityOTP(context.Background(), first.ID, "browser", "viewer@example.com", "fp", []byte("key"), now, time.Minute)
	old, err := s.VerifyIdentityOTP(context.Background(), first.ID, "browser", "viewer@example.com", msg.ID, msg.Code, "", []byte("key"), now, 5)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err = s.ConsumeIdentityHandoff(context.Background(), "a", first.ID, firstState, now, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	switchHandoff, _, err := s.CreateIdentityHandoff(context.Background(), "a", "/", true, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	switchMessage, _ := s.RequestIdentityOTP(context.Background(), switchHandoff.ID, "browser", "viewer@example.com", "fp", []byte("key"), now.Add(time.Second), time.Minute)
	fresh, err := s.VerifyIdentityOTP(context.Background(), switchHandoff.ID, "browser", "viewer@example.com", switchMessage.ID, switchMessage.Code, old.Token, []byte("key"), now.Add(time.Second), 5)
	if err != nil || fresh.Token == "" || len(fresh.Revoked) != 1 || fresh.Revoked[0].AppID != "a" {
		t.Fatalf("switch=%#v err=%v", fresh, err)
	}
	if _, err = s.ValidateIdentitySession(context.Background(), old.Token, now.Add(2*time.Second)); !errors.Is(err, ErrIdentity) {
		t.Fatalf("old global identity survived switch: %v", err)
	}
}

func TestIdentityHandoffBindsOTPToOnePlatformBrowser(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	seedViewerPolicy(t, s)
	now := time.Now().UTC()
	h, _, err := s.CreateIdentityHandoff(context.Background(), "a", "/", false, now)
	if err != nil {
		t.Fatal(err)
	}
	message, err := s.RequestIdentityOTP(context.Background(), h.ID, "browser-one", "viewer@example.com", "fp", []byte("key"), now, time.Minute)
	if err != nil || message == nil {
		t.Fatalf("bound request=%#v err=%v", message, err)
	}
	if _, err = s.RequestIdentityOTP(context.Background(), h.ID, "browser-two", "viewer@example.com", "fp", []byte("key"), now, time.Minute); !errors.Is(err, ErrIdentity) {
		t.Fatalf("second browser overwrote handoff binding: %v", err)
	}
	if _, err = s.VerifyIdentityOTP(context.Background(), h.ID, "browser-two", "viewer@example.com", message.ID, message.Code, "", []byte("key"), now, 5); !errors.Is(err, ErrIdentity) {
		t.Fatalf("mismatched browser consumed OTP: %v", err)
	}
	var authorized, consumed int
	if err = s.DB.QueryRow("SELECT COUNT(*),COUNT(consumed_at) FROM identity_handoffs WHERE id=?", h.ID).Scan(&authorized, &consumed); err != nil || authorized != 1 || consumed != 0 {
		t.Fatalf("mismatch mutated handoff rows=%d consumed=%d err=%v", authorized, consumed, err)
	}
	if _, err = s.VerifyIdentityOTP(context.Background(), h.ID, "browser-one", "viewer@example.com", message.ID, message.Code, "", []byte("key"), now, 5); err != nil {
		t.Fatalf("matching browser did not verify: %v", err)
	}
}

func seedIdentityEmailRule(t *testing.T, s *SQLiteStore, appID, email string) {
	t.Helper()
	if _, err := s.DB.Exec(`INSERT INTO access_rules(id,app_id,policy_revision,kind,normalized_value,created_by,created_at)
		VALUES(? || '-email',?,1,'email',?,'u',datetime('now'))`, appID+"-"+email, appID, email); err != nil {
		t.Fatal(err)
	}
}

func TestGlobalIdentityDifferentEmailsFirstCompletionWinsPerBrowserBinding(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	seedViewerPolicy(t, s)
	seedSecondAllowedApp(t, s)
	seedIdentityEmailRule(t, s, "b", "other@example.com")
	base := time.Date(2026, time.July, 28, 16, 0, 0, 0, time.UTC)

	first, _, err := s.CreateIdentityHandoff(context.Background(), "a", "/", false, base)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := s.CreateIdentityHandoff(context.Background(), "b", "/", false, base)
	if err != nil {
		t.Fatal(err)
	}
	firstMessage, err := s.RequestIdentityOTP(context.Background(), first.ID, "same-browser", "viewer@example.com", "one", []byte("key"), base, time.Minute)
	if err != nil || firstMessage == nil {
		t.Fatalf("first request=%#v err=%v", firstMessage, err)
	}
	secondMessage, err := s.RequestIdentityOTP(context.Background(), second.ID, "same-browser", "other@example.com", "two", []byte("key"), base, time.Minute)
	if err != nil || secondMessage == nil {
		t.Fatalf("second request=%#v err=%v", secondMessage, err)
	}

	results := make(chan error, 2)
	go func() {
		_, e := s.VerifyIdentityOTP(context.Background(), first.ID, "same-browser", "viewer@example.com", firstMessage.ID, firstMessage.Code, "", []byte("key"), base.Add(time.Second), 5)
		results <- e
	}()
	go func() {
		_, e := s.VerifyIdentityOTP(context.Background(), second.ID, "same-browser", "other@example.com", secondMessage.ID, secondMessage.Code, "", []byte("key"), base.Add(time.Second), 5)
		results <- e
	}()
	firstErr, secondErr := <-results, <-results
	successes := 0
	for _, result := range []error{firstErr, secondErr} {
		if result == nil {
			successes++
		} else if !errors.Is(result, ErrOTP) {
			t.Fatalf("different-email completion error=%v", result)
		}
	}
	if successes != 1 {
		t.Fatalf("different-email winners=%d", successes)
	}
	var sessions, families, handoffs int
	if err = s.DB.QueryRow("SELECT COUNT(*),COUNT(DISTINCT family_id) FROM identity_sessions WHERE revoked_at IS NULL").Scan(&sessions, &families); err != nil || sessions != 1 || families != 1 {
		t.Fatalf("active families sessions=%d families=%d err=%v", sessions, families, err)
	}
	if err = s.DB.QueryRow("SELECT COUNT(*) FROM identity_handoffs WHERE id IN (?,?) AND authorized_at IS NOT NULL", first.ID, second.ID).Scan(&handoffs); err != nil || handoffs != 1 {
		t.Fatalf("authorized handoff winners=%d err=%v", handoffs, err)
	}
}

func TestGlobalIdentityConcurrentForceSwitchFirstCompletionWinsPerBrowserBinding(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	seedViewerPolicy(t, s)
	seedSecondAllowedApp(t, s)
	base := time.Date(2026, time.July, 28, 17, 0, 0, 0, time.UTC)

	initial, _, err := s.CreateIdentityHandoff(context.Background(), "a", "/", false, base)
	if err != nil {
		t.Fatal(err)
	}
	initialMessage, _ := s.RequestIdentityOTP(context.Background(), initial.ID, "switch-browser", "viewer@example.com", "initial", []byte("key"), base, time.Minute)
	old, err := s.VerifyIdentityOTP(context.Background(), initial.ID, "switch-browser", "viewer@example.com", initialMessage.ID, initialMessage.Code, "", []byte("key"), base.Add(time.Second), 5)
	if err != nil {
		t.Fatalf("initial global identity: %v", err)
	}
	first, _, err := s.CreateIdentityHandoff(context.Background(), "a", "/", true, base.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := s.CreateIdentityHandoff(context.Background(), "b", "/", true, base.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	firstMessage, _ := s.RequestIdentityOTP(context.Background(), first.ID, "switch-browser", "viewer@example.com", "one", []byte("key"), base.Add(2*time.Second), time.Minute)
	secondMessage, _ := s.RequestIdentityOTP(context.Background(), second.ID, "switch-browser", "viewer@example.com", "two", []byte("key"), base.Add(2*time.Second), time.Minute)
	if firstMessage == nil || secondMessage == nil {
		t.Fatal("force OTP requests were unexpectedly denied")
	}

	type completion struct {
		result IdentityOTPResult
		err    error
	}
	results := make(chan completion, 2)
	go func() {
		result, e := s.VerifyIdentityOTP(context.Background(), first.ID, "switch-browser", "viewer@example.com", firstMessage.ID, firstMessage.Code, old.Token, []byte("key"), base.Add(3*time.Second), 5)
		results <- completion{result: result, err: e}
	}()
	go func() {
		result, e := s.VerifyIdentityOTP(context.Background(), second.ID, "switch-browser", "viewer@example.com", secondMessage.ID, secondMessage.Code, old.Token, []byte("key"), base.Add(3*time.Second), 5)
		results <- completion{result: result, err: e}
	}()
	resultOne, resultTwo := <-results, <-results
	successes := 0
	for i, result := range []completion{resultOne, resultTwo} {
		if result.err == nil && result.result.Token != "" {
			successes++
		} else if !errors.Is(result.err, ErrOTP) {
			t.Fatalf("force completion %d error=%v", i, result.err)
		}
	}
	if successes != 1 {
		t.Fatalf("forced-switch winners=%d", successes)
	}
	if _, err = s.ValidateIdentitySession(context.Background(), old.Token, base.Add(4*time.Second)); !errors.Is(err, ErrIdentity) {
		t.Fatalf("old family survived successful force switch: %v", err)
	}
	var sessions, families, handoffs int
	if err = s.DB.QueryRow("SELECT COUNT(*),COUNT(DISTINCT family_id) FROM identity_sessions WHERE revoked_at IS NULL").Scan(&sessions, &families); err != nil || sessions != 1 || families != 1 {
		t.Fatalf("force active families sessions=%d families=%d err=%v", sessions, families, err)
	}
	if err = s.DB.QueryRow("SELECT COUNT(*) FROM identity_handoffs WHERE id IN (?,?) AND authorized_at IS NOT NULL", first.ID, second.ID).Scan(&handoffs); err != nil || handoffs != 1 {
		t.Fatalf("force authorized handoffs=%d err=%v", handoffs, err)
	}
}

func TestIdentitySessionMalformedTimestampsArePersistenceFailures(t *testing.T) {
	fields := []string{"expires_at", "last_seen_at", "rotated_at", "previous_valid_until"}
	for _, field := range fields {
		t.Run(field, func(t *testing.T) {
			s := seeded(t)
			defer s.Close()
			seedViewerPolicy(t, s)
			now := time.Now().UTC()
			h, _, err := s.CreateIdentityHandoff(context.Background(), "a", "/", false, now)
			if err != nil {
				t.Fatal(err)
			}
			message, _ := s.RequestIdentityOTP(context.Background(), h.ID, "corrupt-browser", "viewer@example.com", "fp", []byte("key"), now, time.Minute)
			verified, err := s.VerifyIdentityOTP(context.Background(), h.ID, "corrupt-browser", "viewer@example.com", message.ID, message.Code, "", []byte("key"), now, 5)
			if err != nil {
				t.Fatal(err)
			}
			if field == "previous_valid_until" {
				prior := sha256.Sum256([]byte("prior-token"))
				if _, err = s.DB.Exec("UPDATE identity_sessions SET previous_secret_hash=?,previous_valid_until=? WHERE id=?", prior[:], "not-a-time", verified.Session.ID); err != nil {
					t.Fatal(err)
				}
			} else if _, err = s.DB.Exec("UPDATE identity_sessions SET "+field+"=? WHERE id=?", "not-a-time", verified.Session.ID); err != nil {
				t.Fatal(err)
			}
			if _, err = s.ValidateIdentitySession(context.Background(), verified.Token, now.Add(time.Second)); !errors.Is(err, ErrIdentityCorrupt) {
				t.Fatalf("malformed %s classified as ordinary identity denial: %v", field, err)
			}
		})
	}
}

func TestRevokeIdentityBrowserBindingRevokesOnlyBoundFamily(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	seedViewerPolicy(t, s)
	now := time.Now().UTC()
	h, state, err := s.CreateIdentityHandoff(context.Background(), "a", "/", false, now)
	if err != nil {
		t.Fatal(err)
	}
	message, err := s.RequestIdentityOTP(context.Background(), h.ID, "revoke-browser", "viewer@example.com", "fp", []byte("key"), now, time.Minute)
	if err != nil || message == nil {
		t.Fatalf("OTP request=%#v err=%v", message, err)
	}
	issued, err := s.VerifyIdentityOTP(context.Background(), h.ID, "revoke-browser", "viewer@example.com", message.ID, message.Code, "", []byte("key"), now, 5)
	if err != nil {
		t.Fatal(err)
	}
	_, appRaw, child, err := s.ConsumeIdentityHandoff(context.Background(), "a", h.ID, state, now, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	refs, err := s.RevokeIdentityBrowserBinding(context.Background(), "revoke-browser", now.Add(time.Second))
	if err != nil || len(refs) != 1 || refs[0] != (AppSessionRef{AppID: "a", SessionID: child.ID}) {
		t.Fatalf("binding revoke refs=%#v err=%v", refs, err)
	}
	if _, err = s.ValidateIdentitySession(context.Background(), issued.Token, now.Add(2*time.Second)); !errors.Is(err, ErrIdentity) {
		t.Fatalf("binding revoke left identity valid: %v", err)
	}
	if _, err = s.Validate(context.Background(), "a", appRaw, now.Add(2*time.Second)); !errors.Is(err, sessions.ErrInvalid) {
		t.Fatalf("binding revoke left child session valid: %v", err)
	}
	if _, err = s.RevokeIdentityBrowserBinding(context.Background(), "unknown-browser", now); !errors.Is(err, ErrIdentity) {
		t.Fatalf("unknown browser binding=%v", err)
	}
}

func TestRevokeIdentityBrowserBindingRevokesExpiredAndRejectsCorruptState(t *testing.T) {
	for _, scenario := range []string{"expired", "corrupt"} {
		t.Run(scenario, func(t *testing.T) {
			s := seeded(t)
			defer s.Close()
			seedViewerPolicy(t, s)
			now := time.Now().UTC()
			h, _, err := s.CreateIdentityHandoff(context.Background(), "a", "/", false, now)
			if err != nil {
				t.Fatal(err)
			}
			message, _ := s.RequestIdentityOTP(context.Background(), h.ID, "binding-state", "viewer@example.com", "fp", []byte("key"), now, time.Minute)
			issued, err := s.VerifyIdentityOTP(context.Background(), h.ID, "binding-state", "viewer@example.com", message.ID, message.Code, "", []byte("key"), now, 5)
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "expired" {
				if _, err = s.RevokeIdentityBrowserBinding(context.Background(), "binding-state", issued.Session.ExpiresAt); err != nil {
					t.Fatalf("expired binding revoke=%v", err)
				}
				var revoked sql.NullString
				if err = s.DB.QueryRow("SELECT revoked_at FROM identity_sessions WHERE id=?", issued.Session.ID).Scan(&revoked); err != nil || !revoked.Valid {
					t.Fatalf("expired binding family remained unrevoked=%#v err=%v", revoked, err)
				}
				return
			}
			if _, err = s.DB.Exec("UPDATE identity_sessions SET expires_at=? WHERE id=?", "malformed", issued.Session.ID); err != nil {
				t.Fatal(err)
			}
			if _, err = s.RevokeIdentityBrowserBinding(context.Background(), "binding-state", now); !errors.Is(err, ErrIdentityCorrupt) {
				t.Fatalf("corrupt binding revoke=%v", err)
			}
		})
	}
}

func TestRevokeIdentityBrowserBindingPropagatesPersistenceFailure(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	if _, err := s.DB.Exec("DROP TABLE identity_sessions"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RevokeIdentityBrowserBinding(context.Background(), "unavailable-browser", time.Now().UTC()); err == nil || errors.Is(err, ErrIdentity) {
		t.Fatalf("binding revoke flattened persistence failure into identity denial: %v", err)
	}
}
