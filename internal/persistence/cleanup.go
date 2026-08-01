package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/ChrisMarxDev/tinkercloud/internal/jobs"
)

// CleanupRelease is database-derived immutable-release metadata. Hashes are
// grouped because content-addressed uploads can legitimately share one release
// directory across multiple deployment records for the same app.
type CleanupRelease struct {
	AppID       string
	ReleaseHash string
	Created     time.Time
	Active      bool
}

// CleanupCandidates treats SQLite's current pointer as authority. Any corrupt
// or ambiguous active state is retained, never reclaimed. Only verified release
// rows with a content hash can name an immutable directory.
func (s *SQLiteStore) CleanupCandidates(ctx context.Context, retention int) ([]CleanupRelease, error) {
	if s == nil || s.DB == nil {
		return nil, errors.New("cleanup store unavailable")
	}
	if retention < 1 {
		retention = 1
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT d.app_id,d.id,d.release_hash,d.state,d.created_at,COALESCE(a.current_deployment_id,'')
		FROM deployments d JOIN applications a ON a.id=d.app_id
		WHERE d.release_hash IS NOT NULL AND d.release_hash<>'' AND d.state IN ('verified','active','superseded')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type group struct {
		release     CleanupRelease
		stateActive bool
	}
	grouped := map[string]group{}
	for rows.Next() {
		var appID, id, hash, state, created, current string
		if err := rows.Scan(&appID, &id, &hash, &state, &created, &current); err != nil {
			return nil, err
		}
		when, err := parseSQLiteTime(created)
		if err != nil {
			return nil, err
		}
		key := appID + "\x00" + hash
		g, exists := grouped[key]
		wasActive := g.release.Active
		if !exists || when.After(g.release.Created) {
			g.release = CleanupRelease{AppID: appID, ReleaseHash: hash, Created: when}
			g.release.Active = wasActive
		}
		// Retain both an active pointer and any row claiming active. The latter
		// fails closed if an interrupted transaction ever leaves inconsistent
		// metadata behind.
		if current == id || state == "active" {
			g.release.Active = true
			g.stateActive = true
		}
		grouped[key] = g
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	byApp := map[string][]CleanupRelease{}
	for _, g := range grouped {
		byApp[g.release.AppID] = append(byApp[g.release.AppID], g.release)
	}
	var out []CleanupRelease
	for _, releases := range byApp {
		plan := make([]jobs.Release, 0, len(releases))
		lookup := map[string]CleanupRelease{}
		for _, r := range releases {
			plan = append(plan, jobs.Release{ID: r.ReleaseHash, Created: r.Created.UnixNano(), Active: r.Active})
			lookup[r.ReleaseHash] = r
		}
		for _, r := range jobs.Cleanup(plan, retention) {
			out = append(out, lookup[r.ID])
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].AppID == out[j].AppID {
			return out[i].Created.Before(out[j].Created)
		}
		return out[i].AppID < out[j].AppID
	})
	return out, nil
}

func parseSQLiteTime(v string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02 15:04:05.999999999"} {
		if t, err := time.Parse(layout, v); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, errors.New("invalid deployment timestamp")
}

// ExecuteCleanup selects first, then removes the private directory. Database
// history is deliberately retained: a failed deletion is retryable and no
// state transition can make a recovery release disappear from the audit trail.
func (s *SQLiteStore) ExecuteCleanup(ctx context.Context, dataRoot string, retention int, executor jobs.Executor) ([]CleanupRelease, error) {
	candidates, err := s.CleanupCandidates(ctx, retention)
	if err != nil {
		return nil, err
	}
	for _, c := range candidates {
		if c.Active {
			return nil, errors.New("cleanup attempted active release")
		}
		if err := executor.Execute(ctx, dataRoot, c.AppID, c.ReleaseHash); err != nil {
			return nil, err
		}
	}
	return candidates, nil
}

// RecordCleanupOutcome appends bounded maintenance evidence. No release hash,
// filesystem path, user input, or secret is retained: the normal deployment
// audit trail remains the source for per-release history.
func (s *SQLiteStore) RecordCleanupOutcome(ctx context.Context, outcome string, count int) error {
	if s == nil || s.DB == nil || (outcome != "succeeded" && outcome != "failed") || count < 0 {
		return errors.New("cleanup audit unavailable")
	}
	return s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO audit_events(id,occurred_at,actor_kind,action,outcome,target_kind,request_id,metadata_json)
			VALUES(lower(hex(randomblob(16))),datetime('now'),'system','release.cleanup',?,'maintenance','maintenance_cleanup',?)`, outcome, fmt.Sprintf(`{"count":%d}`, count))
		return err
	})
}
