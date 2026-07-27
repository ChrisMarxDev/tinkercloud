package persistence

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestSQLiteFreshMigrationAndWriter(t *testing.T) {
	s, e := OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "tiny.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if e = s.Write(context.Background(), func(tx *sql.Tx) error {
		_, e := tx.Exec("INSERT INTO users(id,normalized_email,role,status,created_at) VALUES('u','a@example.com','deployer','active','now')")
		return e
	}); e != nil {
		t.Fatal(e)
	}
}
