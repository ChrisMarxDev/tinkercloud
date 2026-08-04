package persistence

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"github.com/ChrisMarxDev/tinkercloud/internal/apps"
	"github.com/ChrisMarxDev/tinkercloud/internal/identity"
	"github.com/ChrisMarxDev/tinkercloud/internal/releases"
	"github.com/ChrisMarxDev/tinkercloud/internal/sessions"
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
	if json.Unmarshal(manifest, &m) != nil || m.Name != a.Slug || !releases.ValidStoredManifest(m) {
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
	a.DeploymentID = deploymentID
	a.SPAFallback = m.SPAFallback != ""
	a.KVEnabled = m.KV
	a.BlobsEnabled = m.Blobs
	a.RealtimeEnabled = m.Realtime
	a.PublicIndexing = m.Indexing
	return a, nil
}

// FirstActiveAppSlug is the updater's deterministic denial-probe selector. It
// returns only an app with a current active immutable deployment and never
// accepts an operator- or network-supplied app identity.
func (s *SQLiteStore) FirstActiveAppSlug(ctx context.Context) (string, error) {
	var slug string
	err := s.DB.QueryRowContext(ctx, "SELECT a.slug FROM applications a JOIN deployments d ON d.id=a.current_deployment_id AND d.app_id=a.id WHERE a.status='active' AND d.state='active' AND d.release_hash <> '' ORDER BY a.slug LIMIT 1").Scan(&slug)
	if err != nil || !releases.ValidSlug(slug) {
		return "", apps.ErrNotFound
	}
	return slug, nil
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
		// This low-level fixture helper is used by integration tests. Even it
		// creates a global parent first, so no parentless app session can be
		// persisted after the unified-browser cutover.
		if viewer.ID == "" || viewer.Email == "" {
			return sessions.ErrInvalid
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO identities(id,normalized_email,created_at)
			VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET normalized_email=excluded.normalized_email`, viewer.ID, viewer.Email, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
		binding := make([]byte, sha256.Size)
		if _, err := rand.Read(binding); err != nil {
			return err
		}
		_, parent, err := createIdentitySessionTx(ctx, tx, viewer, binding, time.Now())
		if err != nil {
			return err
		}
		raw, session, err = issueChildAppSessionTx(ctx, tx, appID, viewer, parent.ID, expiry)
		return err
	})
	return raw, session, err
}

// issueChildAppSessionTx is the sole child-session insertion path. Every app
// session has one durable global identity parent, created by the admin-host
// browser broker before the handoff is consumed.
func issueChildAppSessionTx(ctx context.Context, tx *sql.Tx, appID string, viewer identity.Identity, identitySessionID string, expiry time.Time) (string, sessions.Session, error) {
	if identitySessionID == "" {
		return "", sessions.Session{}, sessions.ErrInvalid
	}
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		return "", sessions.Session{}, e
	}
	raw := base64.RawURLEncoding.EncodeToString(b)
	h := sha256.Sum256([]byte(raw))
	v := sessions.Session{ID: "ses_" + raw[:12], AppID: appID, Identity: viewer, ExpiresAt: expiry.UTC()}
	_, e := tx.ExecContext(ctx, "INSERT INTO sessions(id,scope,app_id,identity_id,identity_session_id,secret_hash,expires_at,created_at) VALUES(?,?,?,?,?,?,?,?)", v.ID, "app", appID, viewer.ID, identitySessionID, h[:], v.ExpiresAt.Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano))
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

// Create formerly supported direct app-session issuance. That browser path
// was removed; only the handoff flow may create a child session now.
func (s *SQLiteStore) Create(ctx context.Context, appID string, viewer identity.Identity, expiry time.Time) (string, sessions.Session, error) {
	return "", sessions.Session{}, sessions.ErrInvalid
}
func (s *SQLiteStore) Validate(ctx context.Context, appID, raw string, now time.Time) (sessions.Session, error) {
	h := sha256.Sum256([]byte(raw))
	var v sessions.Session
	var email, expires, parentExpires string
	var revoked, parentRevoked sql.NullString
	e := s.DB.QueryRowContext(ctx, `SELECT s.id,s.app_id,i.id,i.normalized_email,s.expires_at,s.revoked_at,
		parent.expires_at,parent.revoked_at
		FROM sessions s
		JOIN identities i ON i.id=s.identity_id
		JOIN identity_sessions parent ON parent.id=s.identity_session_id
		JOIN applications a ON a.id=s.app_id
		WHERE s.app_id=? AND s.scope='app' AND s.secret_hash=? AND a.status='active'`, appID, h[:]).Scan(&v.ID, &v.AppID, &v.Identity.ID, &email, &expires, &revoked, &parentExpires, &parentRevoked)
	if e != nil || revoked.Valid || parentRevoked.Valid {
		return sessions.Session{}, sessions.ErrInvalid
	}
	v.ExpiresAt, e = time.Parse(time.RFC3339Nano, expires)
	if e != nil || !now.Before(v.ExpiresAt) {
		return sessions.Session{}, sessions.ErrInvalid
	}
	parentExpiry, err := time.Parse(time.RFC3339Nano, parentExpires)
	if err != nil || !now.Before(parentExpiry) {
		return sessions.Session{}, sessions.ErrInvalid
	}
	v.Identity.Email = email
	return v, nil
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
