package persistence

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/tinyhost/tiny/internal/jobs"
)

func TestCleanupCandidatesAndExecutionPreserveActiveAndRecovery(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	// An active release and 3 inactive releases: with retention 1 only the two
	// oldest may be reclaimed. SQLite remains the authority, not directory age.
	for _, r := range []struct{ id, hash, state, created string }{
		{"active", "ha", "active", "2026-01-04 00:00:00"},
		{"r3", "h3", "superseded", "2026-01-03 00:00:00"},
		{"r2", "h2", "superseded", "2026-01-02 00:00:00"},
		{"r1", "h1", "superseded", "2026-01-01 00:00:00"},
	} {
		if _, err := s.DB.Exec("INSERT INTO deployments(id,app_id,created_by,release_hash,state,created_at) VALUES(?,?,?,?,?,?)", r.id, "a", "u", r.hash, r.state, r.created); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(s.DataRoot, "releases", "a", r.hash), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.DB.Exec("UPDATE applications SET current_deployment_id='active' WHERE id='a'"); err != nil {
		t.Fatal(err)
	}
	candidates, err := s.CleanupCandidates(context.Background(), 1)
	if err != nil || len(candidates) != 2 || candidates[0].ReleaseHash != "h1" || candidates[1].ReleaseHash != "h2" {
		t.Fatalf("%#v %v", candidates, err)
	}
	removed, err := s.ExecuteCleanup(context.Background(), s.DataRoot, 1, jobs.Executor{})
	if err != nil || len(removed) != 2 {
		t.Fatalf("%#v %v", removed, err)
	}
	for _, hash := range []string{"ha", "h3"} {
		if _, err := os.Stat(filepath.Join(s.DataRoot, "releases", "a", hash)); err != nil {
			t.Fatalf("preserved %s: %v", hash, err)
		}
	}
	for _, hash := range []string{"h1", "h2"} {
		if _, err := os.Stat(filepath.Join(s.DataRoot, "releases", "a", hash)); !os.IsNotExist(err) {
			t.Fatalf("removed %s: %v", hash, err)
		}
	}
}

func TestCleanupFailureLeavesDatabaseAndDirectoryForRetry(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	for _, r := range []struct{ id, hash, created string }{{"r", "h", "2026-01-01 00:00:00"}, {"new", "n", "2026-01-02 00:00:00"}} {
		if _, err := s.DB.Exec("INSERT INTO deployments(id,app_id,created_by,release_hash,state,created_at) VALUES(?,?,? ,?,'superseded',?)", r.id, "a", "u", r.hash, r.created); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(s.DataRoot, "releases", "a", r.hash), 0755); err != nil {
			t.Fatal(err)
		}
	}
	_, err := s.ExecuteCleanup(context.Background(), s.DataRoot, 1, jobs.Executor{Remove: func(_, _, _ string) error { return os.ErrPermission }})
	if err == nil {
		t.Fatal("failure accepted")
	}
	if _, err := s.DB.Query("SELECT id FROM deployments WHERE id='r'"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.DataRoot, "releases", "a", "h")); err != nil {
		t.Fatal(err)
	}
}

func TestRecordCleanupOutcomeIsBoundedAndPathFree(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	if err := s.RecordCleanupOutcome(context.Background(), "succeeded", 2); err != nil {
		t.Fatal(err)
	}
	var action, outcome, target, metadata string
	if err := s.DB.QueryRow("SELECT action,outcome,target_kind,metadata_json FROM audit_events WHERE action='release.cleanup'").Scan(&action, &outcome, &target, &metadata); err != nil {
		t.Fatal(err)
	}
	if action != "release.cleanup" || outcome != "succeeded" || target != "maintenance" || metadata != `{"count":2}` {
		t.Fatalf("unsafe cleanup audit: %q %q %q %q", action, outcome, target, metadata)
	}
	if err := s.RecordCleanupOutcome(context.Background(), "succeeded", -1); err == nil {
		t.Fatal("negative count accepted")
	}
}
