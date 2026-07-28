package persistence

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"github.com/tinyhost/tiny/internal/apps"
	"github.com/tinyhost/tiny/internal/identity"
	"github.com/tinyhost/tiny/internal/releases"
	"github.com/tinyhost/tiny/internal/sessions"
	"path/filepath"
	"time"
)

func (s *SQLiteStore) ResolveActive(ctx context.Context, slug string) (apps.App, error) {
	var a apps.App
	var hash string
	var manifest []byte
	var deploymentID string
	err := s.DB.QueryRowContext(ctx, "SELECT a.id,a.slug,a.status,a.owner_user_id,d.id,d.release_hash,d.manifest_json FROM applications a JOIN deployments d ON d.id=a.current_deployment_id AND d.app_id=a.id WHERE a.slug=? AND a.status='active' AND d.state='active'", slug).Scan(&a.ID, &a.Slug, &a.Status, &a.OwnerIdentityID, &deploymentID, &hash, &manifest)
	if err != nil || hash == "" {
		return apps.App{}, apps.ErrNotFound
	}
	var m releases.Manifest
	if json.Unmarshal(manifest, &m) != nil {
		return apps.App{}, apps.ErrNotFound
	}
	rows, e := s.DB.QueryContext(ctx, "SELECT relative_path,size,content_hash FROM deployment_files WHERE deployment_id=? ORDER BY relative_path", deploymentID)
	if e != nil {
		return apps.App{}, apps.ErrNotFound
	}
	var expected []releases.File
	for rows.Next() {
		var f releases.File
		if e := rows.Scan(&f.Path, &f.Size, &f.Hash); e != nil {
			rows.Close()
			return apps.App{}, apps.ErrNotFound
		}
		expected = append(expected, f)
	}
	if e := rows.Err(); e != nil {
		rows.Close()
		return apps.App{}, apps.ErrNotFound
	}
	if e := rows.Close(); e != nil {
		return apps.App{}, apps.ErrNotFound
	}
	// This method is intentionally metadata-only. Login and reserved routes
	// need an app ID and manifest features, but must not open untrusted release
	// files before authentication. staticruntime verifies this evidence after a
	// sealed AuthorizationContext has been issued.
	a.ReleaseRoot = filepath.Join(s.DataRoot, "releases", a.ID, hash)
	a.ReleaseEvidence = releases.FileManifest{Files: expected, Hash: hash}
	a.SPAFallback = m.SPAFallback != ""
	a.KVEnabled = m.KV
	a.BlobsEnabled = m.Blobs
	a.RealtimeEnabled = m.Realtime
	return a, nil
}

// EligibleAppHost is deliberately narrower than ResolveActive: ACME may issue
// for an active app before its first immutable release exists.
func (s *SQLiteStore) EligibleAppHost(ctx context.Context, slug string) bool {
	if !releases.ValidSlug(slug) {
		return false
	}
	var n int
	if s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM applications WHERE slug=? AND status='active'", slug).Scan(&n) != nil {
		return false
	}
	return n == 1
}
func (s *SQLiteStore) CreateAppSession(ctx context.Context, appID string, viewer identity.Identity, expiry time.Time) (string, sessions.Session, error) {
	var raw string
	var session sessions.Session
	err := s.Write(ctx, func(tx *sql.Tx) error {
		var err error
		raw, session, err = issueAppSessionTx(ctx, tx, appID, viewer, expiry)
		return err
	})
	return raw, session, err
}
func issueAppSessionTx(ctx context.Context, tx *sql.Tx, appID string, viewer identity.Identity, expiry time.Time) (string, sessions.Session, error) {
	return issueChildAppSessionTx(ctx, tx, appID, viewer, "", expiry)
}

// issueChildAppSessionTx records a platform identity parent only for the
// global-login broker. Direct app OTP sessions remain valid independent app
// credentials and therefore have no parent.
func issueChildAppSessionTx(ctx context.Context, tx *sql.Tx, appID string, viewer identity.Identity, identitySessionID string, expiry time.Time) (string, sessions.Session, error) {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		return "", sessions.Session{}, e
	}
	raw := base64.RawURLEncoding.EncodeToString(b)
	h := sha256.Sum256([]byte(raw))
	v := sessions.Session{ID: "ses_" + raw[:12], AppID: appID, Identity: viewer, ExpiresAt: expiry.UTC()}
	_, e := tx.ExecContext(ctx, "INSERT INTO sessions(id,scope,app_id,identity_id,identity_session_id,secret_hash,expires_at,created_at) VALUES(?,?,?,?,?,?,?,?)", v.ID, "app", appID, viewer.ID, nullIfEmpty(identitySessionID), h[:], v.ExpiresAt.Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano))
	if e != nil {
		return "", sessions.Session{}, e
	}
	return raw, v, nil
}

func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}
func (s *SQLiteStore) Create(ctx context.Context, appID string, viewer identity.Identity, expiry time.Time) (string, sessions.Session, error) {
	var raw string
	var out sessions.Session
	e := s.Write(ctx, func(tx *sql.Tx) error {
		var status, mode string
		var rev uint64
		if e := tx.QueryRowContext(ctx, "SELECT a.status,a.policy_revision,p.mode FROM applications a JOIN access_policies p ON p.app_id=a.id AND p.revision=a.policy_revision WHERE a.id=?", appID).Scan(&status, &rev, &mode); e != nil || status != "active" || mode != "private" {
			return sessions.ErrInvalid
		}
		if _, e := tx.ExecContext(ctx, "INSERT INTO identities(id,normalized_email,created_at) VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET normalized_email=excluded.normalized_email", viewer.ID, viewer.Email, time.Now().UTC().Format(time.RFC3339Nano)); e != nil {
			return e
		}
		var allow int
		_ = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM access_rules WHERE app_id=? AND policy_revision=? AND ((kind='email' AND normalized_value=?) OR (kind='domain' AND normalized_value=substr(?,instr(?,'@')+1)))", appID, rev, viewer.Email, viewer.Email, viewer.Email).Scan(&allow)
		if allow == 0 {
			return sessions.ErrInvalid
		}
		var issueErr error
		raw, out, issueErr = issueAppSessionTx(ctx, tx, appID, viewer, expiry)
		return issueErr
	})
	return raw, out, e
}
func (s *SQLiteStore) Validate(ctx context.Context, appID, raw string, now time.Time) (sessions.Session, error) {
	h := sha256.Sum256([]byte(raw))
	var v sessions.Session
	var email, expires, created string
	var revoked sql.NullString
	var identitySessionID sql.NullString
	e := s.DB.QueryRowContext(ctx, "SELECT s.id,s.app_id,i.id,i.normalized_email,s.expires_at,s.revoked_at,s.identity_session_id,s.created_at FROM sessions s JOIN identities i ON i.id=s.identity_id JOIN applications a ON a.id=s.app_id WHERE s.app_id=? AND s.scope='app' AND s.secret_hash=? AND a.status='active'", appID, h[:]).Scan(&v.ID, &v.AppID, &v.Identity.ID, &email, &expires, &revoked, &identitySessionID, &created)
	if e != nil || revoked.Valid {
		return sessions.Session{}, sessions.ErrInvalid
	}
	v.ExpiresAt, e = time.Parse(time.RFC3339Nano, expires)
	if e != nil || !now.Before(v.ExpiresAt) {
		return sessions.Session{}, sessions.ErrInvalid
	}
	createdAt, e := time.Parse(time.RFC3339Nano, created)
	if e != nil {
		return sessions.Session{}, sessions.ErrInvalid
	}
	if identitySessionID.Valid {
		if identitySessionID.String == "" {
			return sessions.Session{}, sessions.ErrInvalid
		}
	} else if !s.parentlessAppSessionPostV5(ctx, createdAt) {
		return sessions.Session{}, sessions.ErrInvalid
	}
	v.Identity.Email = email
	return v, nil
}

// parentlessAppSessionPostV5 quarantines only sessions that predate the v5
// identity-link schema. Newer brokerless compatibility sessions deliberately
// remain parentless and retain their existing app-local semantics. SQLite
// timestamp text is untrusted persisted state here: a missing or malformed v5
// cutoff, or a malformed session creation time, must deny.
func (s *SQLiteStore) parentlessAppSessionPostV5(ctx context.Context, createdAt time.Time) bool {
	var cutoff string
	if err := s.DB.QueryRowContext(ctx, "SELECT applied_at FROM schema_migrations WHERE version=5").Scan(&cutoff); err != nil {
		return false
	}
	cutoffAt, err := time.Parse(time.RFC3339Nano, cutoff)
	if err != nil {
		return false
	}
	return !createdAt.Before(cutoffAt)
}
func (s *SQLiteStore) Revoke(ctx context.Context, appID, raw string) (string, error) {
	h := sha256.Sum256([]byte(raw))
	var id string
	e := s.Write(ctx, func(tx *sql.Tx) error {
		e := tx.QueryRowContext(ctx, "UPDATE sessions SET revoked_at=? WHERE secret_hash=? AND app_id=? AND scope='app' AND revoked_at IS NULL RETURNING id", time.Now().UTC().Format(time.RFC3339Nano), h[:], appID).Scan(&id)
		if e == sql.ErrNoRows {
			return sessions.ErrInvalid
		}
		return e
	})
	return id, e
}
