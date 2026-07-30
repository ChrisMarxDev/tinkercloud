package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// AppDatabase is a single app's private SQLite database. It deliberately has
// no control-plane tables or foreign keys: app identity was resolved before
// this boundary and physical isolation is the tenancy mechanism here.
type AppDatabase struct {
	DB    *sql.DB
	write chan struct{}
}

func (d *AppDatabase) Write(ctx context.Context, fn func(*sql.Tx) error) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-d.write:
	}
	defer func() { d.write <- struct{}{} }()
	tx, err := d.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (d *AppDatabase) Close() error { return d.DB.Close() }

type AppDatabaseManagerOptions struct {
	// MaxOpen bounds app databases held open concurrently. A request may still
	// use its database while another idle database is closed.
	MaxOpen int
	// IdleTTL releases unused app database handles without a background worker.
	// Cleanup is performed on Acquire and CloseIdle.
	IdleTTL time.Duration
	Clock   func() time.Time
}

// AppDatabaseManager lazily owns apps/{server-derived immutable id}/data.db.
// It never accepts a path or browser-derived app selection.
type AppDatabaseManager struct {
	root    string
	maxOpen int
	idleTTL time.Duration
	clock   func() time.Time

	mu      sync.Mutex
	closed  bool
	apps    map[string]*managedAppDatabase
	removed map[string]bool
}

type managedAppDatabase struct {
	db       *AppDatabase
	refs     int
	lastUsed time.Time
}

var (
	ErrInvalidAppDatabaseID = errors.New("invalid app database id")
	ErrAppDatabaseClosed    = errors.New("app database manager closed")
	ErrAppDatabaseRemoving  = errors.New("app database is being removed")
	appDatabaseID           = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
)

func NewAppDatabaseManager(dataRoot string, options AppDatabaseManagerOptions) (*AppDatabaseManager, error) {
	if dataRoot == "" || !filepath.IsAbs(dataRoot) {
		return nil, fmt.Errorf("app database root: %w", ErrInvalidAppDatabaseID)
	}
	if options.MaxOpen <= 0 {
		options.MaxOpen = 32
	}
	if options.IdleTTL <= 0 {
		options.IdleTTL = 2 * time.Minute
	}
	if options.Clock == nil {
		options.Clock = time.Now
	}
	return &AppDatabaseManager{
		root: dataRoot, maxOpen: options.MaxOpen, idleTTL: options.IdleTTL,
		clock: options.Clock, apps: map[string]*managedAppDatabase{}, removed: map[string]bool{},
	}, nil
}

func (m *AppDatabaseManager) databasePath(appID string) (string, error) {
	if !appDatabaseID.MatchString(appID) {
		return "", ErrInvalidAppDatabaseID
	}
	return filepath.Join(m.root, "apps", appID, "data.db"), nil
}

// Path is intentionally only for server-owned lifecycle and verification
// code. It validates the immutable ID before joining it below the data root.
func (m *AppDatabaseManager) Path(appID string) (string, error) { return m.databasePath(appID) }

// Acquire returns a database and an idempotent release function. Callers must
// release it before request completion so bounded eviction can make progress.
func (m *AppDatabaseManager) Acquire(ctx context.Context, appID string) (*AppDatabase, func(), error) {
	path, err := m.databasePath(appID)
	if err != nil {
		return nil, nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, nil, ErrAppDatabaseClosed
	}
	if m.removed[appID] {
		return nil, nil, ErrAppDatabaseRemoving
	}
	now := m.clock().UTC()
	m.closeIdleLocked(now)
	entry := m.apps[appID]
	if entry == nil {
		if err := m.makeRoomLocked(); err != nil {
			return nil, nil, err
		}
		db, err := openAppDatabase(ctx, path)
		if err != nil {
			return nil, nil, err
		}
		entry = &managedAppDatabase{db: db, lastUsed: now}
		m.apps[appID] = entry
	}
	entry.refs++
	entry.lastUsed = now
	var once sync.Once
	release := func() {
		once.Do(func() {
			m.mu.Lock()
			defer m.mu.Unlock()
			if current := m.apps[appID]; current == entry && current.refs > 0 {
				current.refs--
				current.lastUsed = m.clock().UTC()
			}
		})
	}
	return entry.db, release, nil
}

// CloseIdle is exposed for deterministic maintenance/tests; request handling
// also invokes it lazily, so no unbounded cleaner goroutine is required.
func (m *AppDatabaseManager) CloseIdle() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closeIdleLocked(m.clock().UTC())
}

func (m *AppDatabaseManager) closeIdleLocked(now time.Time) {
	for id, entry := range m.apps {
		if entry.refs == 0 && now.Sub(entry.lastUsed) >= m.idleTTL {
			_ = entry.db.Close()
			delete(m.apps, id)
		}
	}
}

func (m *AppDatabaseManager) makeRoomLocked() error {
	if len(m.apps) < m.maxOpen {
		return nil
	}
	var oldestID string
	var oldest time.Time
	for id, entry := range m.apps {
		if entry.refs != 0 || (!oldest.IsZero() && !entry.lastUsed.Before(oldest)) {
			continue
		}
		oldestID, oldest = id, entry.lastUsed
	}
	if oldestID == "" {
		return fmt.Errorf("app database capacity reached")
	}
	entry := m.apps[oldestID]
	if err := entry.db.Close(); err != nil {
		return err
	}
	delete(m.apps, oldestID)
	return nil
}

func (m *AppDatabaseManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil
	}
	m.closed = true
	var first error
	for id, entry := range m.apps {
		if err := entry.db.Close(); err != nil && first == nil {
			first = err
		}
		delete(m.apps, id)
	}
	return first
}

// Remove closes and removes exactly one validated app namespace. It is for the
// deletion lifecycle after control-plane revocation, never request handling.
func (m *AppDatabaseManager) Remove(appID string) error {
	path, err := m.databasePath(appID)
	if err != nil {
		return err
	}
	m.mu.Lock()
	// Stop any newly authorized/in-flight request from opening this database.
	// Existing holders make deletion retryable rather than racing a close and
	// recursive removal against their request.
	m.removed[appID] = true
	if entry := m.apps[appID]; entry != nil {
		if entry.refs != 0 {
			m.mu.Unlock()
			return fmt.Errorf("app database is in use")
		}
		if err := entry.db.Close(); err != nil {
			m.mu.Unlock()
			return err
		}
		delete(m.apps, appID)
	}
	m.mu.Unlock()
	return os.RemoveAll(filepath.Dir(path))
}

func openAppDatabase(ctx context.Context, path string) (*AppDatabase, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	for _, pragma := range []string{"PRAGMA foreign_keys=ON", "PRAGMA journal_mode=WAL", "PRAGMA busy_timeout=5000"} {
		if _, err = db.ExecContext(ctx, pragma); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("app sqlite setup: %w", err)
		}
	}
	if err = applyAppMigrations(ctx, db, time.Now()); err != nil {
		_ = db.Close()
		return nil, err
	}
	app := &AppDatabase{DB: db, write: make(chan struct{}, 1)}
	app.write <- struct{}{}
	return app, nil
}

const appSchemaVersion = 1

func applyAppMigrations(ctx context.Context, db *sql.DB, now time.Time) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS platform_metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		return err
	}
	var version int
	err = tx.QueryRowContext(ctx, `SELECT CAST(value AS INTEGER) FROM platform_metadata WHERE key='schema_version'`).Scan(&version)
	if err == sql.ErrNoRows {
		version = 0
	} else if err != nil {
		return err
	}
	if version > appSchemaVersion {
		return fmt.Errorf("app schema version %d is newer than this binary", version)
	}
	if version < 1 {
		for _, statement := range []string{
			`CREATE TABLE app_kv (key TEXT PRIMARY KEY, value_json BLOB NOT NULL, version INTEGER NOT NULL CHECK(version>0), size_bytes INTEGER NOT NULL CHECK(size_bytes>=0), created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
			`CREATE TABLE documents (collection TEXT NOT NULL, id TEXT NOT NULL, data_json BLOB NOT NULL CHECK(json_valid(data_json)), version INTEGER NOT NULL CHECK(version>0), size_bytes INTEGER NOT NULL CHECK(size_bytes>=0), created_at TEXT NOT NULL, updated_at TEXT NOT NULL, PRIMARY KEY(collection,id))`,
			`CREATE INDEX documents_by_collection_updated ON documents(collection,updated_at,id)`,
			`CREATE TABLE collection_revisions (collection TEXT PRIMARY KEY, revision INTEGER NOT NULL CHECK(revision>0))`,
		} {
			if _, err = tx.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO platform_metadata(key,value) VALUES('schema_version',?)`, fmt.Sprint(appSchemaVersion)); err != nil {
			return err
		}
	}
	_, _ = now, tx
	return tx.Commit()
}
