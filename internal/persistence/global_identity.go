package persistence

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"strings"
	"time"

	"github.com/ChrisMarxDev/tinkercloud/internal/controlapi"
	"github.com/ChrisMarxDev/tinkercloud/internal/identity"
	"github.com/ChrisMarxDev/tinkercloud/internal/policies"
	"github.com/ChrisMarxDev/tinkercloud/internal/sessions"
)

// ErrIdentity is deliberately generic: callers must not distinguish a stale
// browser identity, an expired handoff, an unauthorized policy, or bad state.
var ErrIdentity = errors.New("identity denied")

// ErrIdentityCorrupt is deliberately distinct from ErrIdentity. A known but
// malformed persisted identity must fail closed as an operational fault rather
// than being treated like an ordinary stale browser cookie: doing the latter
// could mint an unrelated replacement family while descendants survive.
var ErrIdentityCorrupt = errors.New("identity session persistence corrupt")

// errRotatedIdentityReplay never crosses the persistence boundary. It marks a
// matched prior credential that is outside the explicitly allowed overlap.
var errRotatedIdentityReplay = errors.New("rotated identity token replay")

const (
	identityLifetime        = 30 * 24 * time.Hour
	identityRotateAfter     = 24 * time.Hour
	identityPreviousOverlap = 60 * time.Second
	handoffLifetime         = 5 * time.Minute
	handoffCleanupLimit     = 128
)

type IdentitySession struct {
	ID, FamilyID string
	Identity     identity.Identity
	ExpiresAt    time.Time
	LastSeenAt   time.Time
	RotatedAt    time.Time
}

type IdentityHandoff struct {
	ID, AppID, AppSlug, ReturnPath string
	ForceLogin                     bool
	ExpiresAt                      time.Time
	AuthorizedAt                   *time.Time
	ConsumedAt                     *time.Time
	RevokedAt                      *time.Time
}

type AppSessionRef struct{ AppID, SessionID string }

// IdentityValidationResult is intentionally returned even when validation
// fails. A replay of a rotated-out global token is an intrusion signal: its
// transaction revokes the entire credential family and its child sessions.
// The caller must close the returned live app sessions before replying with a
// generic authentication denial.
type IdentityValidationResult struct {
	Session          IdentitySession
	ReplacementToken string
	Revoked          []AppSessionRef
	FamilyRevoked    bool
}

type IdentityOTPResult struct {
	Token   string
	Session IdentitySession
	Revoked []AppSessionRef
}

// PlatformIdentityChallenge is delivery data for the platform-only identity
// broker. It carries no app, role, or authorization result.
type PlatformIdentityChallenge struct{ ID, Code, Email string }

// DashboardIdentityResult is the only persistence result that turns a global
// browser identity into dashboard authority. The identity credential itself
// conveys no role: every call joins its normalized email to the current active
// operator/deployer record in the same transaction that records identity use.
//
// Revoked contains app sessions which the gateway must close after the
// transaction commits. It is populated for rotated-token replay, just as it is
// for ValidateIdentitySession.
type DashboardIdentityResult struct {
	Actor            controlapi.Actor
	Identity         IdentitySession
	ReplacementToken string
	Revoked          []AppSessionRef
	FamilyRevoked    bool
}

func randomOpaque(prefix string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(b), nil
}

func safeRelativePath(raw string) (string, bool) {
	if raw == "" {
		return "/", true
	}
	u, err := url.Parse(raw)
	if err != nil || u.IsAbs() || u.Host != "" || !strings.HasPrefix(u.Path, "/") || strings.HasPrefix(u.Path, "//") || strings.ContainsAny(raw, "\\\r\n") {
		return "", false
	}
	return raw, true
}

// CreateIdentityHandoff is called by the app-host gateway after it has derived
// appID from the canonical hostname. State never leaves that app host.
func (s *SQLiteStore) CreateIdentityHandoff(ctx context.Context, appID, returnPath string, forceLogin bool, now time.Time) (IdentityHandoff, string, error) {
	returnPath, ok := safeRelativePath(returnPath)
	if !ok {
		return IdentityHandoff{}, "", ErrIdentity
	}
	id, err := randomOpaque("ih_")
	if err != nil {
		return IdentityHandoff{}, "", err
	}
	state, err := randomOpaque("is_")
	if err != nil {
		return IdentityHandoff{}, "", err
	}
	stateHash := sha256.Sum256([]byte(state))
	h := IdentityHandoff{ID: id, AppID: appID, ReturnPath: returnPath, ForceLogin: forceLogin, ExpiresAt: now.Add(handoffLifetime).UTC()}
	err = s.Write(ctx, func(tx *sql.Tx) error {
		if err := cleanupIdentityHandoffsTx(ctx, tx, now); err != nil {
			return err
		}
		var status string
		if err := tx.QueryRowContext(ctx, "SELECT status FROM applications WHERE id=?", appID).Scan(&status); err != nil || status != "active" {
			return ErrIdentity
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO identity_handoffs(id,app_id,state_hash,return_path,force_login,expires_at,created_at)
			VALUES(?,?,?,?,?,?,?)`, h.ID, h.AppID, stateHash[:], h.ReturnPath, boolInt(h.ForceLogin), h.ExpiresAt.Format(time.RFC3339Nano), now.UTC().Format(time.RFC3339Nano))
		return err
	})
	if err != nil {
		return IdentityHandoff{}, "", err
	}
	return h, state, nil
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func parseTime(value string) (time.Time, error) { return time.Parse(time.RFC3339Nano, value) }

func browserBindingHash(raw string) ([sha256.Size]byte, error) {
	if raw == "" {
		return [sha256.Size]byte{}, ErrIdentity
	}
	return sha256.Sum256([]byte(raw)), nil
}

func identityCorruption(field string, err error) error {
	if err == nil {
		return fmt.Errorf("%w: %s", ErrIdentityCorrupt, field)
	}
	return fmt.Errorf("%w: %s: %v", ErrIdentityCorrupt, field, err)
}

func scanHandoff(row *sql.Row) (IdentityHandoff, error) {
	var h IdentityHandoff
	var force int
	var expiry string
	var authorized, consumed, revoked sql.NullString
	err := row.Scan(&h.ID, &h.AppID, &h.AppSlug, &h.ReturnPath, &force, &expiry, &authorized, &consumed, &revoked)
	if err != nil {
		return IdentityHandoff{}, err
	}
	h.ForceLogin = force == 1
	h.ExpiresAt, err = parseTime(expiry)
	if err != nil {
		return IdentityHandoff{}, err
	}
	if authorized.Valid {
		t, e := parseTime(authorized.String)
		if e != nil {
			return IdentityHandoff{}, e
		}
		h.AuthorizedAt = &t
	}
	if consumed.Valid {
		t, e := parseTime(consumed.String)
		if e != nil {
			return IdentityHandoff{}, e
		}
		h.ConsumedAt = &t
	}
	if revoked.Valid {
		t, e := parseTime(revoked.String)
		if e != nil {
			return IdentityHandoff{}, e
		}
		h.RevokedAt = &t
	}
	return h, nil
}

func handoffTx(ctx context.Context, tx *sql.Tx, id string) (IdentityHandoff, error) {
	h, err := scanHandoff(tx.QueryRowContext(ctx, `SELECT h.id,h.app_id,a.slug,h.return_path,h.force_login,h.expires_at,h.authorized_at,h.consumed_at
		,h.revoked_at FROM identity_handoffs h JOIN applications a ON a.id=h.app_id WHERE h.id=?`, id))
	if err != nil {
		return IdentityHandoff{}, ErrIdentity
	}
	return h, nil
}

// cleanupIdentityHandoffsTx bounds retained one-time browser transaction
// records without a background worker. It deliberately leaves active,
// unconsumed handoffs alone and removes a small deterministic batch only.
func cleanupIdentityHandoffsTx(ctx context.Context, tx *sql.Tx, now time.Time) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM identity_handoffs WHERE id IN (
		SELECT id FROM identity_handoffs
		WHERE expires_at <= ? OR consumed_at IS NOT NULL OR revoked_at IS NOT NULL
		ORDER BY created_at ASC, id ASC
		LIMIT ?
	)`, now.UTC().Format(time.RFC3339Nano), handoffCleanupLimit)
	return err
}

func policyAllowsTx(ctx context.Context, tx *sql.Tx, appID string, viewer identity.Identity) bool {
	var revision uint64
	var mode string
	var owner string
	if err := tx.QueryRowContext(ctx, `SELECT a.policy_revision,p.mode,u.normalized_email
		FROM applications a JOIN access_policies p ON p.app_id=a.id AND p.revision=a.policy_revision
		JOIN users u ON u.id=a.owner_user_id WHERE a.id=? AND a.status='active'`, appID).Scan(&revision, &mode, &owner); err != nil || (mode != "private" && mode != "public") {
		return false
	}
	p := policies.Policy{AppID: appID, OwnerIdentityID: owner, Revision: revision, Valid: true, Emails: map[string]struct{}{}, Domains: map[string]struct{}{}}
	rows, err := tx.QueryContext(ctx, "SELECT kind,normalized_value FROM access_rules WHERE app_id=? AND policy_revision=?", appID, revision)
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var kind, value string
		if rows.Scan(&kind, &value) != nil {
			return false
		}
		switch kind {
		case "email":
			p.Emails[value] = struct{}{}
		case "domain":
			p.Domains[value] = struct{}{}
		case "owner":
			// Owner authority is derived from the application owner row, not an
			// app-controlled rule. The migration may retain this canonical marker.
		default:
			return false
		}
	}
	return rows.Err() == nil && policies.Evaluate(p, viewer) == policies.Allow
}

func identitySessionTx(ctx context.Context, tx *sql.Tx, raw string, now time.Time) (IdentitySession, bool, error) {
	if raw == "" {
		return IdentitySession{}, false, ErrIdentity
	}
	h := sha256.Sum256([]byte(raw))
	var revoked sql.NullString
	var v IdentitySession
	var email, expiry, seen, rotated string
	var current, previous []byte
	var previousUntil sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT s.id,s.family_id,i.id,i.normalized_email,s.secret_hash,s.previous_secret_hash,s.previous_valid_until,s.expires_at,s.last_seen_at,s.rotated_at,s.revoked_at
		FROM identity_sessions s JOIN identities i ON i.id=s.identity_id WHERE (s.secret_hash=? OR s.previous_secret_hash=?)`, h[:], h[:]).Scan(&v.ID, &v.FamilyID, &v.Identity.ID, &email, &current, &previous, &previousUntil, &expiry, &seen, &rotated, &revoked)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return IdentitySession{}, false, ErrIdentity
		}
		return IdentitySession{}, false, err
	}
	if revoked.Valid {
		return IdentitySession{}, false, ErrIdentity
	}
	v.Identity.Email = email
	if v.ExpiresAt, err = parseTime(expiry); err != nil {
		return IdentitySession{}, false, identityCorruption("expires_at", err)
	}
	if v.LastSeenAt, err = parseTime(seen); err != nil {
		return IdentitySession{}, false, identityCorruption("last_seen_at", err)
	}
	if v.RotatedAt, err = parseTime(rotated); err != nil {
		return IdentitySession{}, false, identityCorruption("rotated_at", err)
	}
	if len(current) != sha256.Size || (len(previous) != 0 && len(previous) != sha256.Size) || (len(previous) == 0) != !previousUntil.Valid {
		return IdentitySession{}, false, identityCorruption("credential hash state", nil)
	}
	if previousUntil.Valid {
		if _, e := parseTime(previousUntil.String); e != nil {
			return IdentitySession{}, false, identityCorruption("previous_valid_until", e)
		}
	}
	if !now.Before(v.ExpiresAt) {
		return IdentitySession{}, false, ErrIdentity
	}
	usedPrevious := len(previous) > 0 && string(h[:]) == string(previous)
	if usedPrevious {
		until, e := parseTime(previousUntil.String)
		if e != nil {
			return IdentitySession{}, false, identityCorruption("previous_valid_until", e)
		}
		if !now.Before(until) {
			return v, true, errRotatedIdentityReplay
		}
	}
	return v, usedPrevious, nil
}

// ValidateIdentitySession validates a global opaque credential and rotates a
// current token at most once per 24 hours. The prior hash is accepted only to
// smooth concurrent browser tabs for sixty seconds. Replaying that prior hash
// afterwards revokes the entire family in the same transaction.
func (s *SQLiteStore) ValidateIdentitySession(ctx context.Context, raw string, now time.Time) (IdentityValidationResult, error) {
	var out IdentityValidationResult
	denied := false
	err := s.Write(ctx, func(tx *sql.Tx) error {
		v, prior, err := identitySessionTx(ctx, tx, raw, now)
		if err != nil {
			// identitySessionTx intentionally fails closed, but a previous
			// credential outside its short concurrency window proves a
			// rotated-out token was replayed. Revoke by family, not row, so a
			// future family representation cannot accidentally survive.
			if !errors.Is(err, errRotatedIdentityReplay) {
				return err
			}
			if revokeErr := revokeIdentityFamilyTx(ctx, tx, v.FamilyID, now, &out.Revoked); revokeErr != nil {
				return revokeErr
			}
			out.FamilyRevoked = true
			// Returning ErrIdentity here would roll the committed revocations
			// back. Commit first, then expose the generic denial below.
			denied = true
			return nil
		}
		out.Session = v
		if !prior && now.Sub(v.RotatedAt) >= identityRotateAfter {
			replacement, err := randomOpaque("gid_")
			if err != nil {
				return err
			}
			h := sha256.Sum256([]byte(replacement))
			old := sha256.Sum256([]byte(raw))
			result, err := tx.ExecContext(ctx, `UPDATE identity_sessions SET previous_secret_hash=?,previous_valid_until=?,secret_hash=?,rotated_at=?,last_seen_at=?
				WHERE id=? AND secret_hash=? AND revoked_at IS NULL`, old[:], now.Add(identityPreviousOverlap).UTC().Format(time.RFC3339Nano), h[:], now.UTC().Format(time.RFC3339Nano), now.UTC().Format(time.RFC3339Nano), v.ID, old[:])
			if err != nil {
				return err
			}
			n, _ := result.RowsAffected()
			if n != 1 {
				return ErrIdentity
			}
			out.ReplacementToken = replacement
			out.Session.RotatedAt = now.UTC()
			out.Session.LastSeenAt = now.UTC()
			return nil
		}
		_, err = tx.ExecContext(ctx, "UPDATE identity_sessions SET last_seen_at=? WHERE id=? AND revoked_at IS NULL", now.UTC().Format(time.RFC3339Nano), v.ID)
		out.Session.LastSeenAt = now.UTC()
		return err
	})
	if err == nil && denied {
		err = ErrIdentity
	}
	return out, err
}

// AuthenticateDashboardIdentity validates the platform-only global identity
// cookie and derives dashboard authority from the current users row. It never
// accepts an app session or bearer token, and it never copies a role into
// identity_sessions. Suspending/revoking a deployer or changing a role thus
// takes effect on the next dashboard request.
func (s *SQLiteStore) AuthenticateDashboardIdentity(ctx context.Context, raw string, now time.Time) (DashboardIdentityResult, error) {
	var out DashboardIdentityResult
	denied := false
	err := s.Write(ctx, func(tx *sql.Tx) error {
		v, prior, err := identitySessionTx(ctx, tx, raw, now)
		if err != nil {
			if !errors.Is(err, errRotatedIdentityReplay) {
				return err
			}
			if revokeErr := revokeIdentityFamilyTx(ctx, tx, v.FamilyID, now, &out.Revoked); revokeErr != nil {
				return revokeErr
			}
			out.FamilyRevoked = true
			denied = true
			return nil
		}
		// Preserve proof that the browser identity itself is valid even when the
		// subsequent dashboard-role lookup denies it. Callers must not clear a
		// viewer's global identity merely because that viewer has no dashboard
		// role; they may still have app policy access.
		out.Identity = v

		var actor controlapi.Actor
		if err := tx.QueryRowContext(ctx, `SELECT id,normalized_email,role
			FROM users
			WHERE normalized_email=? AND status='active' AND role IN ('operator','deployer')`, v.Identity.Email).
			Scan(&actor.ID, &actor.Email, &actor.Role); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrIdentity
			}
			return err
		}
		actor.Active = true
		actor.IdentitySessionID = v.ID
		out.Actor = actor

		if !prior && now.Sub(v.RotatedAt) >= identityRotateAfter {
			replacement, err := randomOpaque("gid_")
			if err != nil {
				return err
			}
			newHash := sha256.Sum256([]byte(replacement))
			oldHash := sha256.Sum256([]byte(raw))
			result, err := tx.ExecContext(ctx, `UPDATE identity_sessions
				SET previous_secret_hash=?,previous_valid_until=?,secret_hash=?,rotated_at=?,last_seen_at=?
				WHERE id=? AND secret_hash=? AND revoked_at IS NULL`, oldHash[:], now.Add(identityPreviousOverlap).UTC().Format(time.RFC3339Nano), newHash[:], now.UTC().Format(time.RFC3339Nano), now.UTC().Format(time.RFC3339Nano), v.ID, oldHash[:])
			if err != nil {
				return err
			}
			if n, _ := result.RowsAffected(); n != 1 {
				return ErrIdentity
			}
			out.ReplacementToken = replacement
			out.Identity.RotatedAt = now.UTC()
			out.Identity.LastSeenAt = now.UTC()
			return nil
		}
		if _, err := tx.ExecContext(ctx, "UPDATE identity_sessions SET last_seen_at=? WHERE id=? AND revoked_at IS NULL", now.UTC().Format(time.RFC3339Nano), v.ID); err != nil {
			return err
		}
		out.Identity.LastSeenAt = now.UTC()
		return nil
	})
	if err == nil && denied {
		err = ErrIdentity
	}
	return out, err
}

// GetIdentityHandoff returns only a live unconsumed server-created handoff.
// It exists for the platform's “use another email” action; it never trusts an
// app, return path, or state value provided by the browser.
func (s *SQLiteStore) GetIdentityHandoff(ctx context.Context, handoffID string, now time.Time) (IdentityHandoff, error) {
	var h IdentityHandoff
	err := s.Write(ctx, func(tx *sql.Tx) error {
		var e error
		h, e = handoffTx(ctx, tx, handoffID)
		if e != nil || h.ConsumedAt != nil || h.RevokedAt != nil || !now.Before(h.ExpiresAt) {
			return ErrIdentity
		}
		return nil
	})
	return h, err
}

// ForceIdentityHandoff is the platform-side account-switch operation. The app
// state cookie is deliberately retained on its own host; this only clears an
// already-authorized identity so the next OTP must select a fresh one.
func (s *SQLiteStore) ForceIdentityHandoff(ctx context.Context, handoffID string, now time.Time) error {
	return s.Write(ctx, func(tx *sql.Tx) error {
		h, err := handoffTx(ctx, tx, handoffID)
		if err != nil || h.ConsumedAt != nil || h.RevokedAt != nil || !now.Before(h.ExpiresAt) {
			return ErrIdentity
		}
		result, err := tx.ExecContext(ctx, `UPDATE identity_handoffs
			SET force_login=1, identity_session_id=NULL, authorized_at=NULL
			WHERE id=? AND consumed_at IS NULL AND revoked_at IS NULL`, handoffID)
		if err != nil {
			return err
		}
		n, _ := result.RowsAffected()
		if n != 1 {
			return ErrIdentity
		}
		return nil
	})
}

// RequestIdentityOTP returns an eligible message only. The platform's
// host-only browser binding is hashed and attached to the handoff before a
// challenge exists; a later request from another browser cannot overwrite it.
// Callers must emit the same opaque transaction whether this returns nil or
// delivery fails.
func (s *SQLiteStore) RequestIdentityOTP(ctx context.Context, handoffID, rawBrowserBinding, rawEmail, fingerprint string, key []byte, now time.Time, ttl time.Duration) (*ChallengeMessage, error) {
	bindingHash, err := browserBindingHash(rawBrowserBinding)
	if err != nil {
		return nil, err
	}
	email, err := identity.Normalize(rawEmail)
	if err != nil {
		return nil, nil
	}
	var appID string
	var eligible bool
	bindingDenied := false
	err = s.Write(ctx, func(tx *sql.Tx) error {
		h, e := handoffTx(ctx, tx, handoffID)
		if e != nil || h.ConsumedAt != nil || h.RevokedAt != nil || h.AuthorizedAt != nil || !now.Before(h.ExpiresAt) {
			return ErrIdentity
		}
		result, e := tx.ExecContext(ctx, `UPDATE identity_handoffs
			SET browser_binding_hash=?
			WHERE id=? AND browser_binding_hash IS NULL AND consumed_at IS NULL AND revoked_at IS NULL AND authorized_at IS NULL`, bindingHash[:], h.ID)
		if e != nil {
			return e
		}
		n, _ := result.RowsAffected()
		if n == 0 {
			var bound []byte
			if e = tx.QueryRowContext(ctx, "SELECT browser_binding_hash FROM identity_handoffs WHERE id=?", h.ID).Scan(&bound); e != nil || len(bound) != sha256.Size || subtle.ConstantTimeCompare(bound, bindingHash[:]) != 1 {
				bindingDenied = true
				return ErrIdentity
			}
		}
		appID = h.AppID
		eligible = policyAllowsTx(ctx, tx, appID, identity.Identity{ID: email, Email: email})
		return nil
	})
	if err != nil {
		if bindingDenied {
			return nil, ErrIdentity
		}
		return nil, nil
	}
	return s.CreateChallenge(ctx, appID, "viewer", email, fingerprintHash(key, fingerprint), key, eligible, now, ttl)
}

// RequestPlatformIdentityOTP creates a generic, platform-only identity
// challenge. It deliberately does not inspect users, roles, app policies, or
// handoffs: any syntactically valid email may establish an identity, while
// dashboard authorization later performs the current-role lookup.
func (s *SQLiteStore) RequestPlatformIdentityOTP(ctx context.Context, rawBrowserBinding, rawEmail, fingerprint string, key []byte, now time.Time, ttl time.Duration) (*PlatformIdentityChallenge, error) {
	binding, err := browserBindingHash(rawBrowserBinding)
	if err != nil {
		return nil, ErrIdentity
	}
	email, err := identity.Normalize(rawEmail)
	if err != nil {
		return nil, nil
	}
	if len(key) == 0 {
		return nil, ErrIdentity
	}
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	b := make([]byte, 16)
	if _, err = rand.Read(b); err != nil {
		return nil, err
	}
	id := fmt.Sprintf("pidotp_%x", b)
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return nil, err
	}
	code := fmt.Sprintf("%06d", n.Int64())
	h := hmac.New(sha256.New, key)
	h.Write([]byte(id + ":" + code))
	err = s.Write(ctx, func(tx *sql.Tx) error {
		// New forms supersede only the same browser/email tuple. A different
		// submitted email must still race at verification, where the binding's
		// one-family invariant chooses exactly one completion.
		if _, err := tx.ExecContext(ctx, `UPDATE platform_identity_challenges
			SET invalidated_at=?
			WHERE browser_binding_hash=? AND normalized_email=?
			  AND consumed_at IS NULL AND invalidated_at IS NULL`, now.UTC().Format(time.RFC3339Nano), binding[:], email); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO platform_identity_challenges
			(id,browser_binding_hash,normalized_email,code_hash,expires_at,attempts,request_fingerprint_hash,created_at)
			VALUES(?,?,?,?,?,?,?,?)`, id, binding[:], email, h.Sum(nil), now.Add(ttl).UTC().Format(time.RFC3339Nano), 0, fingerprintHash(key, fingerprint), now.UTC().Format(time.RFC3339Nano))
		return err
	})
	if err != nil {
		return nil, err
	}
	return &PlatformIdentityChallenge{ID: id, Code: code, Email: email}, nil
}

// VerifyPlatformIdentityOTP atomically consumes a platform-only challenge and
// creates the same global identity family used by app handoffs. force is the
// explicit account-switch path; a non-force completion cannot replace a valid
// presented identity. Concurrent completions for one browser binding serialize
// on the active-family uniqueness constraint and the generation check.
func (s *SQLiteStore) VerifyPlatformIdentityOTP(ctx context.Context, rawBrowserBinding, rawEmail, challengeID, code, oldGlobalRaw string, force bool, key []byte, now time.Time, maxAttempts int) (IdentityOTPResult, error) {
	binding, err := browserBindingHash(rawBrowserBinding)
	if err != nil {
		return IdentityOTPResult{}, ErrIdentity
	}
	email, err := identity.Normalize(rawEmail)
	if err != nil || challengeID == "" || len(key) == 0 {
		return IdentityOTPResult{}, ErrOTP
	}
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	var raw string
	var out IdentitySession
	var revoked []AppSessionRef
	denied := false
	err = s.Write(ctx, func(tx *sql.Tx) error {
		var hash []byte
		var expires, created string
		var attempts int
		var consumed, invalid sql.NullString
		if err := tx.QueryRowContext(ctx, `SELECT code_hash,expires_at,attempts,consumed_at,invalidated_at,created_at
			FROM platform_identity_challenges
			WHERE id=? AND browser_binding_hash=? AND normalized_email=?`, challengeID, binding[:], email).
			Scan(&hash, &expires, &attempts, &consumed, &invalid, &created); err != nil || consumed.Valid || invalid.Valid {
			denied = true
			return nil
		}
		expiresAt, parseErr := parseTime(expires)
		createdAt, createdErr := parseTime(created)
		if parseErr != nil || createdErr != nil || !now.Before(expiresAt) || attempts >= maxAttempts {
			denied = true
			return nil
		}
		h := hmac.New(sha256.New, key)
		h.Write([]byte(challengeID + ":" + code))
		if !hmac.Equal(hash, h.Sum(nil)) {
			if _, err := tx.ExecContext(ctx, "UPDATE platform_identity_challenges SET attempts=attempts+1 WHERE id=? AND attempts<?", challengeID, maxAttempts); err != nil {
				return err
			}
			denied = true
			return nil
		}

		bound, hasBound, err := activeBrowserBindingSessionTx(ctx, tx, binding[:])
		if err != nil {
			return err
		}
		if hasBound && now.Before(bound.ExpiresAt) && !bound.CreatedAt.Before(createdAt) {
			denied = true
			return nil
		}
		if !force && oldGlobalRaw != "" {
			if _, _, oldErr := identitySessionTx(ctx, tx, oldGlobalRaw, now); oldErr == nil || errors.Is(oldErr, errRotatedIdentityReplay) {
				denied = true
				return nil
			} else if !errors.Is(oldErr, ErrIdentity) {
				return oldErr
			}
		}
		if hasBound {
			if err := revokeIdentityFamilyTx(ctx, tx, bound.FamilyID, now, &revoked); err != nil {
				return err
			}
		}
		if force && oldGlobalRaw != "" {
			if err := revokeIdentitySessionTx(ctx, tx, oldGlobalRaw, now, &revoked); err != nil && !errors.Is(err, ErrIdentity) {
				return err
			}
		}
		viewer := identity.Identity{ID: email, Email: email}
		if _, err := tx.ExecContext(ctx, `INSERT INTO identities(id,normalized_email,created_at,last_authenticated_at)
			VALUES(?,?,?,?) ON CONFLICT(id) DO UPDATE SET normalized_email=excluded.normalized_email,last_authenticated_at=excluded.last_authenticated_at`, viewer.ID, viewer.Email, now.UTC().Format(time.RFC3339Nano), now.UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
		raw, out, err = createIdentitySessionTx(ctx, tx, viewer, binding[:], now)
		if err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE platform_identity_challenges SET consumed_at=?
			WHERE id=? AND consumed_at IS NULL AND invalidated_at IS NULL`, now.UTC().Format(time.RFC3339Nano), challengeID)
		if err != nil {
			return err
		}
		if n, _ := result.RowsAffected(); n != 1 {
			return ErrOTP
		}
		return nil
	})
	if err != nil {
		return IdentityOTPResult{}, err
	}
	if denied {
		return IdentityOTPResult{}, ErrOTP
	}
	return IdentityOTPResult{Token: raw, Session: out, Revoked: revoked}, nil
}

func createIdentitySessionTx(ctx context.Context, tx *sql.Tx, viewer identity.Identity, browserBindingHash []byte, now time.Time) (string, IdentitySession, error) {
	if len(browserBindingHash) != sha256.Size {
		return "", IdentitySession{}, ErrIdentity
	}
	raw, err := randomOpaque("gid_")
	if err != nil {
		return "", IdentitySession{}, err
	}
	family, err := randomOpaque("fam_")
	if err != nil {
		return "", IdentitySession{}, err
	}
	h := sha256.Sum256([]byte(raw))
	v := IdentitySession{ID: "gis_" + raw[4:16], FamilyID: family, Identity: viewer, ExpiresAt: now.Add(identityLifetime).UTC(), LastSeenAt: now.UTC(), RotatedAt: now.UTC()}
	_, err = tx.ExecContext(ctx, `INSERT INTO identity_sessions(id,identity_id,family_id,secret_hash,browser_binding_hash,expires_at,last_seen_at,rotated_at,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, v.ID, viewer.ID, v.FamilyID, h[:], browserBindingHash, v.ExpiresAt.Format(time.RFC3339Nano), v.LastSeenAt.Format(time.RFC3339Nano), v.RotatedAt.Format(time.RFC3339Nano), now.UTC().Format(time.RFC3339Nano))
	return raw, v, err
}

type browserBindingSession struct {
	FamilyID  string
	ExpiresAt time.Time
	CreatedAt time.Time
}

// activeBrowserBindingSessionTx finds the one unrevoked family currently
// associated with a browser binding. The partial unique index is the durable
// backstop; treating multiple rows as corruption makes an old malformed
// database fail closed instead of selecting an arbitrary family to revoke.
func activeBrowserBindingSessionTx(ctx context.Context, tx *sql.Tx, bindingHash []byte) (browserBindingSession, bool, error) {
	rows, err := tx.QueryContext(ctx, `SELECT family_id,expires_at,created_at
		FROM identity_sessions WHERE browser_binding_hash=? AND revoked_at IS NULL`, bindingHash)
	if err != nil {
		return browserBindingSession{}, false, err
	}
	var found browserBindingSession
	count := 0
	for rows.Next() {
		var expiry, created string
		if err = rows.Scan(&found.FamilyID, &expiry, &created); err != nil {
			return browserBindingSession{}, false, err
		}
		if found.ExpiresAt, err = parseTime(expiry); err != nil {
			return browserBindingSession{}, false, identityCorruption("expires_at", err)
		}
		if found.CreatedAt, err = parseTime(created); err != nil {
			return browserBindingSession{}, false, identityCorruption("created_at", err)
		}
		count++
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return browserBindingSession{}, false, err
	}
	if err = rows.Close(); err != nil {
		return browserBindingSession{}, false, err
	}
	if count > 1 {
		return browserBindingSession{}, false, identityCorruption("multiple active browser binding families", nil)
	}
	return found, count == 1, nil
}

// VerifyIdentityOTP consumes the app-bound generic challenge, rechecks the
// handoff's current policy, creates a global identity, and authorizes exactly
// that handoff in one transaction. A stale non-force form cannot replace an
// identity that was established after its OTP was requested: the first
// serialized completion wins. force-login revokes an optional prior global
// identity and its derived app sessions in the same transaction.
func (s *SQLiteStore) VerifyIdentityOTP(ctx context.Context, handoffID, rawBrowserBinding, rawEmail, challengeID, code, oldGlobalRaw string, key []byte, now time.Time, maxAttempts int) (IdentityOTPResult, error) {
	bindingHash, err := browserBindingHash(rawBrowserBinding)
	if err != nil {
		return IdentityOTPResult{}, err
	}
	email, err := identity.Normalize(rawEmail)
	if err != nil {
		return IdentityOTPResult{}, ErrOTP
	}
	var appID string
	if err = s.DB.QueryRowContext(ctx, "SELECT app_id FROM identity_handoffs WHERE id=?", handoffID).Scan(&appID); err != nil {
		return IdentityOTPResult{}, ErrOTP
	}
	var raw string
	var out IdentitySession
	var revoked []AppSessionRef
	err = s.VerifyAndIssue(ctx, appID, "viewer", email, challengeID, code, key, now, maxAttempts, func(tx *sql.Tx) error {
		h, e := handoffTx(ctx, tx, handoffID)
		if e != nil || h.ConsumedAt != nil || h.RevokedAt != nil || !now.Before(h.ExpiresAt) || !policyAllowsTx(ctx, tx, h.AppID, identity.Identity{ID: email, Email: email}) {
			return ErrOTP
		}
		var boundHash []byte
		if e = tx.QueryRowContext(ctx, "SELECT browser_binding_hash FROM identity_handoffs WHERE id=?", h.ID).Scan(&boundHash); e != nil || len(boundHash) != sha256.Size || subtle.ConstantTimeCompare(boundHash, bindingHash[:]) != 1 {
			return ErrIdentity
		}
		viewer := identity.Identity{ID: email, Email: email}
		// The selected challenge's timestamp is the browser-flow generation
		// boundary. Once another completion has created an active identity
		// after this form was issued, accepting this form would create a
		// second family that the browser can no longer reliably revoke.
		var challengeCreated string
		if e = tx.QueryRowContext(ctx, `SELECT created_at FROM otp_challenges
			WHERE id=? AND app_id=? AND purpose='viewer' AND normalized_email=?`, challengeID, h.AppID, email).Scan(&challengeCreated); e != nil {
			return ErrOTP
		}
		createdAt, parseErr := parseTime(challengeCreated)
		if parseErr != nil {
			return ErrOTP
		}
		boundSession, hasBoundSession, e := activeBrowserBindingSessionTx(ctx, tx, bindingHash[:])
		if e != nil {
			return e
		}
		// Any completion at or after the selected challenge belongs to this
		// browser's newer login generation. It wins irrespective of email or
		// force-login mode; this check happens before an older force switch can
		// revoke it. Equality is intentionally included because SQLite stores a
		// caller-provided timestamp and equal timestamps are otherwise ambiguous.
		if hasBoundSession && now.Before(boundSession.ExpiresAt) && !boundSession.CreatedAt.Before(createdAt) {
			return ErrOTP
		}
		// A non-force form must not silently replace any valid currently
		// presented browser identity either. Account replacement is explicit
		// through the force-login path, which revokes the old family below.
		if !h.ForceLogin && oldGlobalRaw != "" {
			if _, _, oldErr := identitySessionTx(ctx, tx, oldGlobalRaw, now); oldErr == nil {
				return ErrOTP
			} else if errors.Is(oldErr, errRotatedIdentityReplay) {
				// This endpoint must never turn a replayed prior token into a
				// fresh global family. ValidateIdentitySession performs the
				// corresponding family revocation when it observes that token on
				// an identity-authentication route.
				return ErrOTP
			} else if !errors.Is(oldErr, ErrIdentity) {
				// Do not flatten persistence/corruption faults into an OTP
				// denial that could otherwise mint a replacement credential.
				return oldErr
			}
		}
		// A binding's old family may be expired, or the platform identity cookie
		// may have been lost. Once the OTP is valid, revoking that older family is
		// the safe re-authentication path: it also revokes child sessions and
		// pending grants before the unique binding index admits a replacement.
		// An explicit force switch follows the same path for a pre-challenge
		// family. A newer family was rejected above and therefore cannot lose a
		// race to a stale force form.
		if hasBoundSession {
			if e = revokeIdentityFamilyTx(ctx, tx, boundSession.FamilyID, now, &revoked); e != nil {
				return e
			}
		}
		// The platform identity cookie remains the account-switch credential. A
		// force handoff revokes its family too, even if a browser-binding cookie
		// was regenerated and therefore points at no matching row. ErrIdentity is
		// benign here when the binding-family revocation above already revoked the
		// same credential.
		if h.ForceLogin && oldGlobalRaw != "" {
			if e = revokeIdentitySessionTx(ctx, tx, oldGlobalRaw, now, &revoked); e != nil && !errors.Is(e, ErrIdentity) {
				return e
			}
		}
		if _, e = tx.ExecContext(ctx, "INSERT INTO identities(id,normalized_email,created_at,last_authenticated_at) VALUES(?,?,?,?) ON CONFLICT(id) DO UPDATE SET normalized_email=excluded.normalized_email,last_authenticated_at=excluded.last_authenticated_at", viewer.ID, viewer.Email, now.UTC().Format(time.RFC3339Nano), now.UTC().Format(time.RFC3339Nano)); e != nil {
			return e
		}
		raw, out, e = createIdentitySessionTx(ctx, tx, viewer, bindingHash[:], now)
		if e != nil {
			return e
		}
		result, e := tx.ExecContext(ctx, "UPDATE identity_handoffs SET identity_session_id=?,authorized_at=? WHERE id=? AND identity_session_id IS NULL AND consumed_at IS NULL AND revoked_at IS NULL", out.ID, now.UTC().Format(time.RFC3339Nano), h.ID)
		if e != nil {
			return e
		}
		n, _ := result.RowsAffected()
		if n != 1 {
			return ErrOTP
		}
		return nil
	})
	if err != nil {
		return IdentityOTPResult{}, err
	}
	return IdentityOTPResult{Token: raw, Session: out, Revoked: revoked}, nil
}

// AuthorizeIdentityHandoff uses an existing global identity. The app and
// return target are loaded only from the persisted handoff row.
func (s *SQLiteStore) AuthorizeIdentityHandoff(ctx context.Context, handoffID, globalRaw string, now time.Time) (IdentityHandoff, IdentitySession, error) {
	var h IdentityHandoff
	var viewer IdentitySession
	err := s.Write(ctx, func(tx *sql.Tx) error {
		var e error
		viewer, _, e = identitySessionTx(ctx, tx, globalRaw, now)
		if e != nil {
			return e
		}
		h, e = handoffTx(ctx, tx, handoffID)
		if e != nil || h.ForceLogin || h.ConsumedAt != nil || h.RevokedAt != nil || !now.Before(h.ExpiresAt) || !policyAllowsTx(ctx, tx, h.AppID, viewer.Identity) {
			return ErrIdentity
		}
		result, e := tx.ExecContext(ctx, "UPDATE identity_handoffs SET identity_session_id=?,authorized_at=? WHERE id=? AND identity_session_id IS NULL AND consumed_at IS NULL AND revoked_at IS NULL", viewer.ID, now.UTC().Format(time.RFC3339Nano), h.ID)
		if e != nil {
			return e
		}
		n, _ := result.RowsAffected()
		if n != 1 {
			return ErrIdentity
		}
		t := now.UTC()
		h.AuthorizedAt = &t
		return nil
	})
	return h, viewer, err
}

// ConsumeIdentityHandoff is app-host only: exactAppID is derived from the
// callback hostname and rawState comes from that host's HttpOnly state cookie.
func (s *SQLiteStore) ConsumeIdentityHandoff(ctx context.Context, exactAppID, handoffID, rawState string, now, appExpiry time.Time) (IdentityHandoff, string, sessions.Session, error) {
	var h IdentityHandoff
	var appRaw string
	var appSession sessions.Session
	stateHash := sha256.Sum256([]byte(rawState))
	if rawState == "" || !now.Before(appExpiry) {
		return IdentityHandoff{}, "", sessions.Session{}, ErrIdentity
	}
	err := s.Write(ctx, func(tx *sql.Tx) error {
		var identitySessionID, identityID, email string
		var expiry string
		var revoked sql.NullString
		var appSlug string
		var force int
		var handoffExpiry string
		var authorized, consumed, handoffRevoked sql.NullString
		e := tx.QueryRowContext(ctx, `SELECT h.id,h.app_id,a.slug,h.return_path,h.force_login,h.expires_at,h.authorized_at,h.consumed_at,h.revoked_at,h.identity_session_id,s.identity_id,i.normalized_email,s.expires_at,s.revoked_at
			FROM identity_handoffs h JOIN identity_sessions s ON s.id=h.identity_session_id JOIN identities i ON i.id=s.identity_id
			JOIN applications a ON a.id=h.app_id WHERE h.id=? AND h.app_id=? AND h.state_hash=?`, handoffID, exactAppID, stateHash[:]).Scan(&h.ID, &h.AppID, &appSlug, &h.ReturnPath, &force, &handoffExpiry, &authorized, &consumed, &handoffRevoked, &identitySessionID, &identityID, &email, &expiry, &revoked)
		if e != nil || revoked.Valid || handoffRevoked.Valid {
			return ErrIdentity
		}
		// Reload canonical fields with parsing so malformed persistence denies.
		h, e = handoffTx(ctx, tx, handoffID)
		if e != nil || h.AppID != exactAppID || h.AuthorizedAt == nil || h.ConsumedAt != nil || h.RevokedAt != nil || !now.Before(h.ExpiresAt) {
			return ErrIdentity
		}
		identityExpiry, e := parseTime(expiry)
		if e != nil || !now.Before(identityExpiry) || !policyAllowsTx(ctx, tx, exactAppID, identity.Identity{ID: identityID, Email: email}) {
			return ErrIdentity
		}
		result, e := tx.ExecContext(ctx, "UPDATE identity_handoffs SET consumed_at=? WHERE id=? AND consumed_at IS NULL AND revoked_at IS NULL", now.UTC().Format(time.RFC3339Nano), h.ID)
		if e != nil {
			return e
		}
		n, _ := result.RowsAffected()
		if n != 1 {
			return ErrIdentity
		}
		appRaw, appSession, e = issueChildAppSessionTx(ctx, tx, exactAppID, identity.Identity{ID: identityID, Email: email}, identitySessionID, appExpiry)
		return e
	})
	return h, appRaw, appSession, err
}

// revokeIdentityFamilyTx is the only global-identity revocation primitive.
// A credential family is the security boundary: session rows may be replaced
// or multiplied in a future persistence implementation, while a stolen old
// token must invalidate every descendant authorization grant.
func revokeIdentityFamilyTx(ctx context.Context, tx *sql.Tx, familyID string, now time.Time, refs *[]AppSessionRef) error {
	if familyID == "" {
		return ErrIdentity
	}
	revokedAt := now.UTC().Format(time.RFC3339Nano)

	rows, err := tx.QueryContext(ctx, `SELECT s.app_id,s.id
		FROM sessions s JOIN identity_sessions i ON i.id=s.identity_session_id
		WHERE i.family_id=? AND s.scope='app' AND s.revoked_at IS NULL`, familyID)
	if err != nil {
		return err
	}
	defer rows.Close()
	var got []AppSessionRef
	for rows.Next() {
		var r AppSessionRef
		if err = rows.Scan(&r.AppID, &r.SessionID); err != nil {
			return err
		}
		got = append(got, r)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	if err = rows.Close(); err != nil {
		return err
	}

	result, err := tx.ExecContext(ctx, "UPDATE identity_sessions SET revoked_at=? WHERE family_id=? AND revoked_at IS NULL", revokedAt, familyID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return ErrIdentity
	}
	if _, err = tx.ExecContext(ctx, `UPDATE sessions SET revoked_at=?
		WHERE scope='app' AND revoked_at IS NULL AND identity_session_id IN (
			SELECT id FROM identity_sessions WHERE family_id=?
		)`, revokedAt, familyID); err != nil {
		return err
	}
	// This is a terminal revocation, unlike a normal logout followed by a
	// fresh sign-in. A pending handoff authorized by the compromised family
	// must never become an app session later.
	if _, err = tx.ExecContext(ctx, `UPDATE identity_handoffs SET revoked_at=?
		WHERE consumed_at IS NULL AND revoked_at IS NULL AND identity_session_id IN (
			SELECT id FROM identity_sessions WHERE family_id=?
		)`, revokedAt, familyID); err != nil {
		return err
	}
	if refs != nil {
		*refs = append(*refs, got...)
	}
	return nil
}

func revokeIdentitySessionTx(ctx context.Context, tx *sql.Tx, raw string, now time.Time, refs *[]AppSessionRef) error {
	if raw == "" {
		return ErrIdentity
	}
	hash := sha256.Sum256([]byte(raw))
	var familyID string
	err := tx.QueryRowContext(ctx, "SELECT family_id FROM identity_sessions WHERE (secret_hash=? OR previous_secret_hash=?) AND revoked_at IS NULL", hash[:], hash[:]).Scan(&familyID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrIdentity
		}
		return err
	}
	return revokeIdentityFamilyTx(ctx, tx, familyID, now, refs)
}

func (s *SQLiteStore) RevokeIdentitySession(ctx context.Context, raw string, now time.Time) ([]AppSessionRef, error) {
	var refs []AppSessionRef
	err := s.Write(ctx, func(tx *sql.Tx) error { return revokeIdentitySessionTx(ctx, tx, raw, now, &refs) })
	return refs, err
}

// RevokeIdentityBrowserBinding is deliberately a revocation-only recovery
// primitive. The opaque platform browser-binding value is hashed, used only to
// locate its one unrevoked family, and never returns an identity or authorizes
// a handoff. It also revokes an expired unrevoked family so its descendants and
// pending grants cannot survive a browser-level cleanup.
func (s *SQLiteStore) RevokeIdentityBrowserBinding(ctx context.Context, rawBinding string, now time.Time) ([]AppSessionRef, error) {
	bindingHash, err := browserBindingHash(rawBinding)
	if err != nil {
		return nil, err
	}
	var refs []AppSessionRef
	err = s.Write(ctx, func(tx *sql.Tx) error {
		bound, found, e := activeBrowserBindingSessionTx(ctx, tx, bindingHash[:])
		if e != nil {
			return e
		}
		if !found {
			return ErrIdentity
		}
		return revokeIdentityFamilyTx(ctx, tx, bound.FamilyID, now, &refs)
	})
	return refs, err
}
