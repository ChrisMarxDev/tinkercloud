package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/ChrisMarxDev/tinkercloud/internal/deployments"
	"github.com/ChrisMarxDev/tinkercloud/internal/identity"
	"github.com/ChrisMarxDev/tinkercloud/internal/releases"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"
)

type DeploymentRepository struct{ Store *SQLiteStore }

// RecoveryRecords is database-led: it enumerates only durable deployment rows,
// never the release filesystem. Callers verify just their derived immutable
// evidence locations.
func (s DeploymentRepository) RecoveryRecords(ctx context.Context) ([]deployments.Record, error) {
	// Do not call Get here. Get intentionally rejects malformed release
	// metadata for request-time callers, but startup recovery must classify a
	// corrupt historical row as failed and continue serving unrelated apps.
	// Casting the durable columns keeps type corruption local to a record; only
	// query/scan/transaction failures remain startup errors.
	rows, err := s.Store.DB.QueryContext(ctx, `SELECT
        CAST(d.id AS TEXT), CAST(d.app_id AS TEXT), CAST(a.slug AS TEXT),
        COALESCE(CAST(d.created_by AS TEXT), ''),
        COALESCE(CAST(d.idempotency_key AS TEXT), ''),
        COALESCE(CAST(d.release_hash AS TEXT), ''),
        COALESCE(CAST(d.manifest_json AS BLOB), X''), CAST(d.state AS TEXT)
        FROM deployments d JOIN applications a ON a.id=d.app_id
        ORDER BY d.app_id,d.id`)
	if err != nil {
		return nil, err
	}
	// SQLiteStore intentionally has one connection. Read the complete raw
	// deployment cursor before asking for any per-record file evidence.
	type rawRecord struct {
		record   deployments.Record
		manifest []byte
	}
	var raw []rawRecord
	for rows.Next() {
		var r deployments.Record
		var manifest []byte
		var state string
		if err := rows.Scan(&r.ID, &r.AppID, &r.AppSlug, &r.OwnerID, &r.IdempotencyKey, &r.ReleaseHash, &manifest, &state); err != nil {
			return nil, err
		}
		r.State = releases.State(state)
		raw = append(raw, rawRecord{record: r, manifest: manifest})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	out := make([]deployments.Record, 0, len(raw))
	for _, item := range raw {
		r := item.record
		if r.State == releases.Verified || r.State == releases.Active || r.State == releases.Superseded {
			if len(item.manifest) == 0 || json.Unmarshal(item.manifest, &r.Manifest) != nil || r.Manifest.Name == "" {
				r.RecoveryCorrupt = true
			}
			files, corrupt, err := s.recoveryFiles(ctx, r.ID)
			if err != nil {
				return nil, err
			}
			r.Files = files
			r.RecoveryCorrupt = r.RecoveryCorrupt || corrupt
		}
		out = append(out, r)
	}
	return out, nil
}

// recoveryFiles reads only the manifest evidence for one database record. A
// malformed row is evidence corruption, not a database outage, so it is
// returned as corrupt for per-record failure classification.
func (s DeploymentRepository) recoveryFiles(ctx context.Context, deploymentID string) ([]releases.File, bool, error) {
	rows, err := s.Store.DB.QueryContext(ctx, `SELECT
        CAST(relative_path AS TEXT), CAST(size AS TEXT), CAST(content_hash AS TEXT)
        FROM deployment_files WHERE deployment_id=? ORDER BY relative_path`, deploymentID)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var files []releases.File
	corrupt := false
	for rows.Next() {
		var path, sizeText, hash string
		if err := rows.Scan(&path, &sizeText, &hash); err != nil {
			return nil, false, err
		}
		size, err := strconv.ParseInt(sizeText, 10, 64)
		if err != nil {
			corrupt = true
			continue
		}
		files = append(files, releases.File{Path: path, Size: size, Hash: hash})
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	return files, corrupt, nil
}

// ApplyRecovery is deliberately idempotent. If the active release cannot be
// verified, it also disables the app and clears its pointer so the gateway can
// never serve bytes from an unverifiable active release.
func (s DeploymentRepository) ApplyRecovery(ctx context.Context, id string, next releases.State) error {
	return s.Store.Write(ctx, func(tx *sql.Tx) error {
		var state, appID string
		if err := tx.QueryRowContext(ctx, "SELECT state,app_id FROM deployments WHERE id=?", id).Scan(&state, &appID); err != nil {
			return err
		}
		if next != releases.Failed {
			return errors.New("invalid recovery classification")
		}
		if state != string(releases.Failed) {
			if _, err := tx.ExecContext(ctx, "UPDATE deployments SET state='failed' WHERE id=?", id); err != nil {
				return err
			}
		}
		// Clear a dangling current pointer regardless of the row's claimed state.
		// This is stricter than trusting "active": a corrupt row pointing at a
		// supposedly verified release must not leave an app available either.
		_, err := tx.ExecContext(ctx, "UPDATE applications SET status='failed',current_deployment_id=NULL,updated_at=datetime('now') WHERE id=? AND current_deployment_id=?", appID, id)
		return err
	})
}

func (s DeploymentRepository) DeploymentAttempts(ctx context.Context, ownerID string, since time.Time) (int, error) {
	if s.Store == nil || ownerID == "" {
		return 0, errors.New("deployment attempt store unavailable")
	}
	var count int
	// datetime() accepts both the existing SQLite timestamps and the RFC3339
	// value provided by the service clock. Counting records rather than only
	// completed releases ensures malformed uploads cannot bypass the budget.
	err := s.Store.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM deployments WHERE created_by=? AND datetime(created_at)>=datetime(?)", ownerID, since.UTC().Format(time.RFC3339Nano)).Scan(&count)
	return count, err
}

func (s DeploymentRepository) Create(ctx context.Context, r deployments.Record) error {
	manifest, err := json.Marshal(r.Manifest)
	if err != nil {
		return err
	}
	return s.Store.Write(ctx, func(tx *sql.Tx) error {
		var oldID string
		e := tx.QueryRowContext(ctx, "SELECT id FROM deployments WHERE app_id=? AND idempotency_key=?", r.AppID, r.IdempotencyKey).Scan(&oldID)
		if e == nil {
			if oldID != r.ID {
				return deployments.ErrIdempotency
			}
			_, e = tx.ExecContext(ctx, "UPDATE deployments SET created_by=?,archive_hash=?,release_hash=?,manifest_json=?,state=? WHERE id=? AND app_id=?", r.OwnerID, r.ArchiveHash[:], r.ReleaseHash, manifest, string(r.State), r.ID, r.AppID)
			if e != nil {
				return e
			}
			return persistDeploymentFiles(ctx, tx, r)
		}
		if e != sql.ErrNoRows {
			return e
		}
		_, e = tx.ExecContext(ctx, "INSERT INTO deployments(id,app_id,created_by,idempotency_key,archive_hash,release_hash,manifest_json,state,created_at) VALUES(?,?,?,?,?,?,?,?,datetime('now'))", r.ID, r.AppID, r.OwnerID, r.IdempotencyKey, r.ArchiveHash[:], r.ReleaseHash, manifest, string(r.State))
		if e != nil {
			return e
		}
		return persistDeploymentFiles(ctx, tx, r)
	})
}

// persistDeploymentFiles writes filesystem evidence in the same transaction as
// the verified deployment record. A nil slice means this intermediate state has
// no immutable release yet; a verified record must carry a non-empty complete
// manifest (enforced by the service and read path).
func persistDeploymentFiles(ctx context.Context, tx *sql.Tx, r deployments.Record) error {
	if r.Files == nil {
		return nil
	}
	if r.State != releases.Verified && r.State != releases.Active && r.State != releases.Superseded {
		return errors.New("file evidence before verified release")
	}
	if len(r.Files) == 0 {
		return errors.New("empty release file evidence")
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM deployment_files WHERE deployment_id=?", r.ID); err != nil {
		return err
	}
	for _, f := range r.Files {
		if f.Size < 0 || len(f.Hash) != 64 || path.Clean(f.Path) != f.Path || strings.HasPrefix(f.Path, "../") || strings.Contains(f.Path, "\\") || f.Path == "." {
			return errors.New("invalid deployment file evidence")
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO deployment_files(deployment_id,relative_path,size,content_hash) VALUES(?,?,?,?)", r.ID, f.Path, f.Size, f.Hash); err != nil {
			return err
		}
	}
	return nil
}
func (s DeploymentRepository) Get(ctx context.Context, id string) (deployments.Record, error) {
	var r deployments.Record
	var hash []byte
	var state string
	var manifest []byte
	e := s.Store.DB.QueryRowContext(ctx, "SELECT d.id,d.app_id,a.slug,COALESCE(d.created_by,''),COALESCE(d.idempotency_key,''),COALESCE(d.archive_hash,X''),COALESCE(d.release_hash,''),COALESCE(d.manifest_json,X''),d.state FROM deployments d JOIN applications a ON a.id=d.app_id WHERE d.id=?", id).Scan(&r.ID, &r.AppID, &r.AppSlug, &r.OwnerID, &r.IdempotencyKey, &hash, &r.ReleaseHash, &manifest, &state)
	if e == sql.ErrNoRows {
		return deployments.Record{}, os.ErrNotExist
	}
	if e != nil {
		return deployments.Record{}, e
	}
	copy(r.ArchiveHash[:], hash)
	r.State = releases.State(state)
	if r.State == releases.Verified || r.State == releases.Active || r.State == releases.Superseded {
		if len(manifest) == 0 || json.Unmarshal(manifest, &r.Manifest) != nil || r.Manifest.Name == "" {
			return deployments.Record{}, errors.New("corrupt deployment manifest")
		}
		rows, err := s.Store.DB.QueryContext(ctx, "SELECT relative_path,size,content_hash FROM deployment_files WHERE deployment_id=? ORDER BY relative_path", id)
		if err != nil {
			return deployments.Record{}, err
		}
		for rows.Next() {
			var f releases.File
			if err := rows.Scan(&f.Path, &f.Size, &f.Hash); err != nil {
				rows.Close()
				return deployments.Record{}, err
			}
			r.Files = append(r.Files, f)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return deployments.Record{}, err
		}
		if err := rows.Close(); err != nil {
			return deployments.Record{}, err
		}
	}
	return r, nil
}
func (s DeploymentRepository) Active(ctx context.Context, appID string) (*deployments.Record, error) {
	var id sql.NullString
	if err := s.Store.DB.QueryRowContext(ctx, "SELECT current_deployment_id FROM applications WHERE id=?", appID).Scan(&id); err != nil {
		return nil, err
	}
	if !id.Valid || id.String == "" {
		return nil, nil
	}
	r, err := s.Get(ctx, id.String)
	if err != nil || r.AppID != appID || r.State != releases.Active {
		return nil, os.ErrNotExist
	}
	return &r, nil
}

// ActivationReplay consults durable audit evidence before any verified-state
// planning. A completed exact retry must never create another policy revision,
// audit row, or live side effect. Any reused key for another target or a state
// that no longer proves the committed result fails closed.
func (s DeploymentRepository) ActivationReplay(ctx context.Context, next deployments.Record, requestID string) (bool, error) {
	if requestID == "" || next.ID == "" || next.AppID == "" || next.OwnerID == "" {
		return false, deployments.ErrDenied
	}
	var target string
	err := s.Store.DB.QueryRowContext(ctx, "SELECT target_id FROM audit_events WHERE action='deployment.activated' AND actor_id=? AND request_id=?", next.OwnerID, requestID).Scan(&target)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if target != next.ID {
		return false, deployments.ErrIdempotency
	}
	var current, state string
	if err := s.Store.DB.QueryRowContext(ctx, "SELECT a.current_deployment_id,d.state FROM applications a JOIN deployments d ON d.id=a.current_deployment_id WHERE a.id=?", next.AppID).Scan(&current, &state); err != nil {
		return false, deployments.ErrIdempotency
	}
	if current != next.ID || state != string(releases.Active) || next.State != releases.Active {
		return false, deployments.ErrIdempotency
	}
	return true, nil
}

// CandidatePolicyReady validates the policy carried by an immutable candidate,
// rather than consulting the app's ambient current policy. It is used before
// activation as a fail-closed gate; CommitActivation repeats the checks inside
// its transaction before it changes either pointer.
func (s *SQLiteStore) CandidatePolicyReady(ctx context.Context, r deployments.Record) bool {
	if _, _, err := canonicalManifestPolicy(r); err != nil || r.AppID == "" || r.OwnerID == "" {
		return false
	}
	var owner, status string
	err := s.DB.QueryRowContext(ctx, "SELECT owner_user_id,status FROM applications WHERE id=?", r.AppID).Scan(&owner, &status)
	return err == nil && owner == r.OwnerID && status == "active"
}

// canonicalManifestPolicy validates the server-persisted deployment manifest.
// ParseManifest already produces this shape during upload. Keeping the check at
// the repository boundary prevents a corrupt database row or a test adapter
// from activating against an unrelated current policy.
func canonicalManifestPolicy(r deployments.Record) ([]string, []string, error) {
	m := r.Manifest
	if m.Version != 1 || m.Name == "" || !releases.ValidSlug(m.Name) || (r.AppSlug != "" && m.Name != r.AppSlug) {
		return nil, nil, errors.New("invalid candidate policy")
	}
	emails := append([]string(nil), m.Emails...)
	domains := append([]string(nil), m.Domains...)
	if !slices.IsSorted(emails) || !slices.IsSorted(domains) {
		return nil, nil, errors.New("noncanonical candidate policy")
	}
	for i, value := range emails {
		normalized, err := identity.Normalize(value)
		if err != nil || normalized != value || (i > 0 && emails[i-1] == value) {
			return nil, nil, errors.New("invalid candidate email")
		}
	}
	for i, value := range domains {
		if !validPolicyDomain(value) || (i > 0 && domains[i-1] == value) {
			return nil, nil, errors.New("invalid candidate domain")
		}
	}
	return emails, domains, nil
}

func validPolicyDomain(value string) bool {
	if value == "" || len(value) > 253 || strings.ContainsAny(value, "@/ \t\r\n") {
		return false
	}
	for _, label := range strings.Split(value, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
				return false
			}
		}
	}
	return true
}

func installManifestPolicy(ctx context.Context, tx *sql.Tx, appID, actor string, revision uint64, r deployments.Record) error {
	emails, domains, err := canonicalManifestPolicy(r)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO access_policies(app_id,revision,mode,created_by,created_at) VALUES(?,?, 'private', ?, datetime('now'))", appID, revision, actor); err != nil {
		return err
	}
	for _, value := range emails {
		if _, err = tx.ExecContext(ctx, "INSERT INTO access_rules(id,app_id,policy_revision,kind,normalized_value,created_by,created_at) VALUES(lower(hex(randomblob(16))),?,?, 'email', ?, ?, datetime('now'))", appID, revision, value, actor); err != nil {
			return err
		}
	}
	for _, value := range domains {
		if _, err = tx.ExecContext(ctx, "INSERT INTO access_rules(id,app_id,policy_revision,kind,normalized_value,created_by,created_at) VALUES(lower(hex(randomblob(16))),?,?, 'domain', ?, ?, datetime('now'))", appID, revision, value, actor); err != nil {
			return err
		}
	}
	return nil
}
func (s DeploymentRepository) CommitActivation(ctx context.Context, next deployments.Record, old *deployments.Record, requestID string) error {
	if requestID == "" {
		return deployments.ErrDenied
	}
	planned, err := releases.PlanActivation(nil, next.Deployment, releases.ActivationRequirements{PolicyReady: true, CertificateReady: true, DenialProbePassed: true})
	if err != nil {
		return err
	}
	_ = planned
	return s.Store.Write(ctx, func(tx *sql.Tx) error {
		var target string
		e := tx.QueryRowContext(ctx, "SELECT target_id FROM audit_events WHERE action='deployment.activated' AND actor_id=? AND request_id=?", next.OwnerID, requestID).Scan(&target)
		if e == nil {
			if target == next.ID {
				return nil
			}
			return deployments.ErrIdempotency
		}
		if e != sql.ErrNoRows {
			return e
		}
		var current sql.NullString
		var policyRevision uint64
		var owner, status string
		if err := tx.QueryRowContext(ctx, "SELECT current_deployment_id,policy_revision,owner_user_id,status FROM applications WHERE id=?", next.AppID).Scan(&current, &policyRevision, &owner, &status); err != nil {
			return err
		}
		if owner != next.OwnerID || status != "active" {
			return deployments.ErrDenied
		}
		if old == nil && current.Valid && current.String != "" {
			return releases.ErrTransition
		}
		if old != nil && (!current.Valid || current.String != old.ID) {
			return releases.ErrTransition
		}
		if old != nil && (old.AppID != next.AppID || old.State != releases.Active) {
			return releases.ErrTransition
		}
		if err := installManifestPolicy(ctx, tx, next.AppID, next.OwnerID, policyRevision+1, next); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, "UPDATE deployments SET state='active',activated_at=datetime('now') WHERE id=? AND app_id=? AND state='verified'", next.ID, next.AppID)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return releases.ErrTransition
		}
		if old != nil {
			res, err = tx.ExecContext(ctx, "UPDATE deployments SET state='superseded' WHERE id=? AND app_id=? AND state='active'", old.ID, next.AppID)
			if err != nil {
				return err
			}
			n, _ = res.RowsAffected()
			if n != 1 {
				return releases.ErrTransition
			}
		}
		res, err = tx.ExecContext(ctx, "UPDATE applications SET current_deployment_id=?,policy_revision=?,updated_at=datetime('now') WHERE id=?", next.ID, policyRevision+1, next.AppID)
		if err != nil {
			return err
		}
		n, _ = res.RowsAffected()
		if n != 1 {
			return releases.ErrTransition
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO audit_events(id,occurred_at,actor_kind,actor_id,app_id,action,outcome,target_id,request_id) VALUES(lower(hex(randomblob(16))),datetime('now'),'deployer',?,?,'deployment.activated','success',?,?)", next.OwnerID, next.AppID, next.ID, requestID)
		return err
	})
}
func (s DeploymentRepository) Fail(ctx context.Context, id string) error {
	return s.Store.Write(ctx, func(tx *sql.Tx) error {
		var state string
		if err := tx.QueryRowContext(ctx, "SELECT state FROM deployments WHERE id=?", id).Scan(&state); err != nil {
			return err
		}
		if state == string(releases.Failed) {
			return nil
		}
		if state == string(releases.Active) || state == string(releases.Superseded) || state == string(releases.Rejected) {
			return releases.ErrTransition
		}
		res, err := tx.ExecContext(ctx, "UPDATE deployments SET state='failed' WHERE id=? AND state=?", id, state)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return releases.ErrTransition
		}
		return nil
	})
}
