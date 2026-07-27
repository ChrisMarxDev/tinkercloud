package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/tinyhost/tiny/internal/apps"
	"github.com/tinyhost/tiny/internal/identity"
	"github.com/tinyhost/tiny/internal/releases"
	"github.com/tinyhost/tiny/internal/sessions"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func seeded(t *testing.T) *SQLiteStore {
	t.Helper()
	s, e := OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "x.db"))
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, q := range []string{"INSERT INTO users VALUES('u','owner@example.com','deployer','active','" + now + "')", "INSERT INTO identities(id,normalized_email,created_at) VALUES('i','a@example.com','" + now + "')", "INSERT INTO applications(id,owner_user_id,slug,status,policy_revision,created_at,updated_at) VALUES('a','u','alpha','active',1,'" + now + "','" + now + "')", "INSERT INTO applications(id,owner_user_id,slug,status,policy_revision,created_at,updated_at) VALUES('b','u','beta','suspended',1,'" + now + "','" + now + "')"} {
		if _, e = s.DB.Exec(q); e != nil {
			t.Fatal(e)
		}
	}
	return s
}
func seedActiveRelease(t *testing.T, s *SQLiteStore) {
	t.Helper()
	m, _ := json.Marshal(releases.Manifest{Version: 1, Name: "alpha"})
	staging := filepath.Join(s.DataRoot, "seed-release")
	if e := os.MkdirAll(staging, 0755); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(staging, "index.html"), []byte("seed"), 0644); e != nil {
		t.Fatal(e)
	}
	files, e := releases.Inspect(staging)
	if e != nil {
		t.Fatal(e)
	}
	root := filepath.Join(s.DataRoot, "releases", "a", files.Hash)
	if e := os.MkdirAll(root, 0755); e != nil {
		t.Fatal(e)
	}
	if e := os.Rename(filepath.Join(staging, "index.html"), filepath.Join(root, "index.html")); e != nil {
		t.Fatal(e)
	}
	if _, e := s.DB.Exec("INSERT INTO deployments(id,app_id,created_by,idempotency_key,release_hash,manifest_json,state,created_at) VALUES('d','a','u','k',?,?,'active',datetime('now'))", files.Hash, m); e != nil {
		t.Fatal(e)
	}
	for _, f := range files.Files {
		if _, e := s.DB.Exec("INSERT INTO deployment_files(deployment_id,relative_path,size,content_hash) VALUES('d',?,?,?)", f.Path, f.Size, f.Hash); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := s.DB.Exec("UPDATE applications SET current_deployment_id='d' WHERE id='a'"); e != nil {
		t.Fatal(e)
	}
}
func TestSQLiteAppsAndSessions(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	seedActiveRelease(t, s)
	if _, e := s.ResolveActive(context.Background(), "alpha"); e != nil {
		t.Fatal(e)
	}
	if _, e := s.ResolveActive(context.Background(), "beta"); e != apps.ErrNotFound {
		t.Fatal(e)
	}
	raw, v, e := s.CreateAppSession(context.Background(), "a", identity.Identity{ID: "i", Email: "a@example.com"}, time.Now().Add(time.Hour))
	if e != nil {
		t.Fatal(e)
	}
	var hash []byte
	if e = s.DB.QueryRow("SELECT secret_hash FROM sessions WHERE id=?", v.ID).Scan(&hash); e != nil || string(hash) == raw {
		t.Fatal("raw secret persisted")
	}
	if _, e = s.Validate(context.Background(), "a", raw, time.Now()); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Validate(context.Background(), "b", raw, time.Now()); e != sessions.ErrInvalid {
		t.Fatal(e)
	}
	if _, e = s.Revoke(context.Background(), "a", raw); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Validate(context.Background(), "a", raw, time.Now()); e != sessions.ErrInvalid {
		t.Fatal(e)
	}
}

func TestAppSessionsPersistIndependentlyAcrossSQLiteRestart(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	dbPath := filepath.Join(t.TempDir(), "sessions.db")
	open := func() *SQLiteStore {
		s, err := OpenSQLite(ctx, dbPath)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	s := open()
	stamp := now.Format(time.RFC3339Nano)
	for _, q := range []string{
		"INSERT INTO users VALUES('u','owner@example.com','deployer','active','" + stamp + "')",
		"INSERT INTO identities(id,normalized_email,created_at) VALUES('i1','one@example.com','" + stamp + "')",
		"INSERT INTO identities(id,normalized_email,created_at) VALUES('i2','two@example.com','" + stamp + "')",
		"INSERT INTO applications(id,owner_user_id,slug,status,policy_revision,created_at,updated_at) VALUES('a','u','alpha','active',1,'" + stamp + "','" + stamp + "')",
		"INSERT INTO applications(id,owner_user_id,slug,status,policy_revision,created_at,updated_at) VALUES('b','u','beta','active',1,'" + stamp + "','" + stamp + "')",
	} {
		if _, err := s.DB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	first, _, err := s.CreateAppSession(ctx, "a", identity.Identity{ID: "i1", Email: "one@example.com"}, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := s.CreateAppSession(ctx, "a", identity.Identity{ID: "i1", Email: "one@example.com"}, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	third, _, err := s.CreateAppSession(ctx, "a", identity.Identity{ID: "i2", Email: "two@example.com"}, now.Add(time.Hour))
	if err != nil || first == second || second == third {
		t.Fatalf("independent opaque credentials were not issued: %v", err)
	}
	expired, _, err := s.CreateAppSession(ctx, "a", identity.Identity{ID: "i1", Email: "one@example.com"}, now.Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = open()
	validate := func(app, raw string, wantErr bool) {
		t.Helper()
		_, got := s.Validate(ctx, app, raw, now)
		if (got != nil) != wantErr {
			t.Fatalf("validate app=%q credential=%q err=%v", app, "[redacted]", got)
		}
	}
	validate("a", first, false)
	validate("a", second, false)
	validate("a", third, false)
	validate("b", first, true)
	validate("a", expired, true)
	if _, err := s.Revoke(ctx, "a", first); err != nil {
		t.Fatal(err)
	}
	validate("a", first, true)
	validate("a", second, false)
	validate("a", third, false)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = open()
	defer s.Close()
	validate("a", first, true)
	validate("a", second, false)
	validate("a", third, false)
}

var _ *sql.DB
