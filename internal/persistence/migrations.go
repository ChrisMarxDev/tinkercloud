package persistence

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"fmt"
	"sort"
	"strings"
	"time"
)

//go:embed sql/*.sql
var migrationFS embed.FS

type EmbeddedMigration struct {
	Version             int
	Name, SQL, Checksum string
}

func LoadMigrations() ([]EmbeddedMigration, error) {
	entries, e := migrationFS.ReadDir("sql")
	if e != nil {
		return nil, e
	}
	out := make([]EmbeddedMigration, 0, len(entries))
	for _, x := range entries {
		b, e := migrationFS.ReadFile("sql/" + x.Name())
		if e != nil {
			return nil, e
		}
		var v int
		if _, e = fmt.Sscanf(x.Name(), "%04d_", &v); e != nil {
			return nil, e
		}
		h := sha256.Sum256(b)
		out = append(out, EmbeddedMigration{v, x.Name(), string(b), fmt.Sprintf("%x", h)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}
func ApplyMigrations(ctx context.Context, db *sql.DB, now time.Time) error {
	ms, e := LoadMigrations()
	if e != nil {
		return e
	}
	tx, e := db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, checksum TEXT NOT NULL UNIQUE, applied_at TEXT NOT NULL)"); e != nil {
		return e
	}
	for _, m := range ms {
		var got string
		e = tx.QueryRowContext(ctx, "SELECT checksum FROM schema_migrations WHERE version=?", m.Version).Scan(&got)
		if e == nil {
			if got != m.Checksum {
				return fmt.Errorf("migration checksum mismatch: %d", m.Version)
			}
			continue
		}
		if e != sql.ErrNoRows {
			return e
		}
		if _, e = tx.ExecContext(ctx, m.SQL); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, "INSERT INTO schema_migrations(version,checksum,applied_at) VALUES(?,?,?)", m.Version, m.Checksum, now.UTC().Format(time.RFC3339Nano)); e != nil {
			return e
		}
	}
	return tx.Commit()
}

var _ = strings.TrimSpace
