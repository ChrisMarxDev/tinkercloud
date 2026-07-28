package persistence

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
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

// approvedGlobalViewerIdentityMigrationV5SQLChecksum pins the corrected,
// additive migration bytes. It prevents a future edit from borrowing the
// historical persisted checksum and silently changing the migration's effect.
const approvedGlobalViewerIdentityMigrationV5SQLChecksum = "5ab4c3b21e9255ab887b8c4f23596cf304fe58b09a3d4df83c86ed900d0b8076"

// legacyGlobalViewerIdentityMigrationChecksum is the checksum persisted by the
// short-lived destructive v5 release. The corrected, additive v5 deliberately
// retains that persisted checksum: updater rollback restores only the binary,
// so the prior binary must be able to open a database first migrated by the
// corrected binary. It is a one-time migration identity compatibility shim;
// it never causes the historical SQL to execute.
const legacyGlobalViewerIdentityMigrationChecksum = "297183f181f09ade031290d9acf60dddc76ed2f35fc655d22a14207d989194de"

func acceptedMigrationChecksum(version int, got, current string) bool {
	return got == current || (version == 5 && got == legacyGlobalViewerIdentityMigrationChecksum)
}

type migrationSource interface {
	fs.FS
	fs.ReadDirFS
}

func LoadMigrations() ([]EmbeddedMigration, error) {
	return loadMigrations(migrationFS)
}

func loadMigrations(source migrationSource) ([]EmbeddedMigration, error) {
	entries, e := fs.ReadDir(source, "sql")
	if e != nil {
		return nil, e
	}
	out := make([]EmbeddedMigration, 0, len(entries))
	for _, x := range entries {
		b, e := fs.ReadFile(source, "sql/"+x.Name())
		if e != nil {
			return nil, e
		}
		var v int
		if _, e = fmt.Sscanf(x.Name(), "%04d_", &v); e != nil {
			return nil, e
		}
		checksum, e := migrationChecksum(v, b)
		if e != nil {
			return nil, e
		}
		out = append(out, EmbeddedMigration{v, x.Name(), string(b), checksum})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}

func migrationChecksum(version int, sql []byte) (string, error) {
	h := sha256.Sum256(sql)
	computed := fmt.Sprintf("%x", h)
	if version != 5 {
		return computed, nil
	}
	if computed != approvedGlobalViewerIdentityMigrationV5SQLChecksum {
		return "", fmt.Errorf("unapproved v5 migration SQL checksum: %s", computed)
	}
	return legacyGlobalViewerIdentityMigrationChecksum, nil
}

func ApplyMigrations(ctx context.Context, db *sql.DB, now time.Time) error {
	return applyMigrations(ctx, db, now, migrationFS)
}

func applyMigrations(ctx context.Context, db *sql.DB, now time.Time, source migrationSource) error {
	ms, e := loadMigrations(source)
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
			if !acceptedMigrationChecksum(m.Version, got, m.Checksum) {
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
