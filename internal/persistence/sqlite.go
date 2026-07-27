package persistence

import (
	"context"
	"database/sql"
	"fmt"
	_ "modernc.org/sqlite"
	"path/filepath"
	"time"
)

// SQLiteStore is the single-process persistence adapter. All callers should
// use Write for mutations so SQLite contention remains bounded and observable.
type SQLiteStore struct {
	DB       *sql.DB
	DataRoot string
	write    chan struct{}
}

func OpenSQLite(ctx context.Context, dsn string) (*SQLiteStore, error) {
	db, e := sql.Open("sqlite", dsn)
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	for _, pragma := range []string{"PRAGMA foreign_keys=ON", "PRAGMA journal_mode=WAL", "PRAGMA busy_timeout=5000"} {
		if _, e = db.ExecContext(ctx, pragma); e != nil {
			db.Close()
			return nil, fmt.Errorf("sqlite setup: %w", e)
		}
	}
	s := &SQLiteStore{DB: db, DataRoot: filepath.Dir(dsn), write: make(chan struct{}, 1)}
	s.write <- struct{}{}
	if e := ApplyMigrations(ctx, db, time.Now()); e != nil {
		db.Close()
		return nil, e
	}
	return s, nil
}
func (s *SQLiteStore) Close() error { return s.DB.Close() }
func (s *SQLiteStore) Write(ctx context.Context, fn func(*sql.Tx) error) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.write:
	}
	defer func() { s.write <- struct{}{} }()
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if e = fn(tx); e != nil {
		return e
	}
	return tx.Commit()
}

// CurrentPolicy loads a private policy revision by tenant key; missing/corrupt
// state is an error for callers, which preserves fail-closed authorization.
func (s *SQLiteStore) CurrentPolicy(ctx context.Context, appID string) (uint64, error) {
	var revision uint64
	e := s.DB.QueryRowContext(ctx, "SELECT policy_revision FROM applications WHERE id=?", appID).Scan(&revision)
	if e != nil {
		return 0, e
	}
	var mode string
	e = s.DB.QueryRowContext(ctx, "SELECT mode FROM access_policies WHERE app_id=? AND revision=?", appID, revision).Scan(&mode)
	if e != nil || mode != "private" {
		return 0, fmt.Errorf("policy unavailable")
	}
	return revision, nil
}
