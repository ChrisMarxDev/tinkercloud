package persistence

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestSQLiteFreshMigrationAndWriter(t *testing.T) {
	s, e := OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "tinker.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	var legacyTable string
	if err := s.DB.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='app_kv'").Scan(&legacyTable); err != sql.ErrNoRows {
		t.Fatalf("legacy control app_kv survived the pre-release transition: table=%q err=%v", legacyTable, err)
	}
	if e = s.Write(context.Background(), func(tx *sql.Tx) error {
		_, e := tx.Exec("INSERT INTO users(id,normalized_email,role,status,created_at) VALUES('u','a@example.com','deployer','active','now')")
		return e
	}); e != nil {
		t.Fatal(e)
	}
}
