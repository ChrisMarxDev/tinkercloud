package persistence

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/tinyhost/tiny/internal/controlapi"
	"github.com/tinyhost/tiny/internal/sessions"
)

// The top-level file is the operator-visible contract; exact normalized bytes
// avoid a partial-table check silently accepting schema drift.
func TestEmbeddedMigrationsMatchCanonicalFiles(t *testing.T) {
	entries, err := os.ReadDir("../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		b, err := os.ReadFile(filepath.Join("../../migrations", entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		embedded, err := migrationFS.ReadFile("sql/" + entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != string(embedded) {
			t.Fatalf("embedded migration differs from canonical migrations/%s", entry.Name())
		}
	}
}

func TestControlCredentialMigrationUpgradesAndDeniesLegacyChallenge(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "legacy.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	legacy, err := migrationFS.ReadFile("sql/0001_v1_schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(legacy)); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	migrations, err := LoadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	var legacyChecksum string
	for _, migration := range migrations {
		if migration.Version == 1 {
			legacyChecksum = migration.Checksum
		}
	}
	if legacyChecksum == "" {
		t.Fatal("missing migration 1")
	}
	if _, err = db.Exec("INSERT INTO schema_migrations(version,checksum,applied_at) VALUES(1,?,?)", legacyChecksum, now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO users(id,normalized_email,role,status,created_at) VALUES('u','owner@example.com','deployer','active',?)", now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	legacyBearer := "tiny_legacy_control_or_cli"
	legacyBearerHash := sha256.Sum256([]byte(legacyBearer))
	if _, err = db.Exec("INSERT INTO api_tokens(id,user_id,secret_hash,scopes,expires_at) VALUES('legacy_token','u',?,'app:read',?)", legacyBearerHash[:], now.Add(time.Hour).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO otp_challenges(id,app_id,purpose,normalized_email,code_hash,expires_at,attempts,created_at) VALUES('legacy',NULL,'control','owner@example.com',x'01',?,0,?)", now.Add(time.Hour).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if err = ApplyMigrations(context.Background(), db, now); err != nil {
		t.Fatal(err)
	}
	var channel sql.NullString
	if err = db.QueryRow("SELECT control_channel FROM otp_challenges WHERE id='legacy'").Scan(&channel); err != nil || channel.Valid {
		t.Fatalf("legacy channel=%#v err=%v", channel, err)
	}
	store := &SQLiteStore{DB: db, write: make(chan struct{}, 1)}
	store.write <- struct{}{}
	if _, err = store.AuthenticateToken(context.Background(), legacyBearer, "app:read", "", now); err != ErrToken {
		t.Fatalf("legacy bearer remained valid after upgrade: %v", err)
	}
	var revokedAt string
	if err = db.QueryRow("SELECT revoked_at FROM api_tokens WHERE id='legacy_token'").Scan(&revokedAt); err != nil || revokedAt == "" {
		t.Fatalf("legacy bearer revocation=%q err=%v", revokedAt, err)
	}
	outbox := &captureOutbox{}
	login := ControlLogin{Store: store, HMACKey: []byte("key"), Outbox: outbox}
	if _, err = login.VerifyOTP(context.Background(), "legacy", "000000", controlapi.BrowserLoginChannel); err != ErrOTP {
		t.Fatalf("legacy challenge authenticated: %v", err)
	}
	var consumed sql.NullString
	if err = db.QueryRow("SELECT consumed_at FROM otp_challenges WHERE id='legacy'").Scan(&consumed); err != nil || consumed.Valid {
		t.Fatalf("legacy challenge mutated: %#v err=%v", consumed, err)
	}
	cliChallenge, err := login.RequestOTP(context.Background(), "owner@example.com", controlapi.CLILoginChannel, "test")
	if err != nil || cliChallenge == "" || outbox.m.Code == "" {
		t.Fatal(err)
	}
	newBearer, err := login.VerifyOTP(context.Background(), cliChallenge, outbox.m.Code, controlapi.CLILoginChannel)
	if err != nil || newBearer == "" {
		t.Fatal(err)
	}
	if _, err = store.AuthenticateToken(context.Background(), newBearer, "app:read", "", now); err != nil {
		t.Fatalf("fresh CLI bearer denied: %v", err)
	}
	browserChallenge, err := login.RequestOTP(context.Background(), "owner@example.com", controlapi.BrowserLoginChannel, "test")
	if err != nil || browserChallenge == "" || outbox.m.Code == "" {
		t.Fatal(err)
	}
	browser, err := login.VerifyOTP(context.Background(), browserChallenge, outbox.m.Code, controlapi.BrowserLoginChannel)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.AuthenticateControlSession(context.Background(), browser, now); err != nil {
		t.Fatalf("fresh browser control session denied: %v", err)
	}
	if err = ApplyMigrations(context.Background(), db, now.Add(time.Minute)); err != nil {
		t.Fatalf("migration engine rerun: %v", err)
	}
	var rerunRevocation string
	if err = db.QueryRow("SELECT revoked_at FROM api_tokens WHERE id='legacy_token'").Scan(&rerunRevocation); err != nil || rerunRevocation != revokedAt {
		t.Fatalf("rerun changed legacy revocation from %q to %q: %v", revokedAt, rerunRevocation, err)
	}
	if _, err = store.AuthenticateToken(context.Background(), newBearer, "app:read", "", now); err != nil {
		t.Fatalf("rerun revoked fresh CLI bearer: %v", err)
	}
	var n int
	if err = db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version IN (2,3,4,5)").Scan(&n); err != nil || n != 4 {
		t.Fatalf("migration 2/3/4/5 evidence n=%d err=%v", n, err)
	}
	var table, index string
	if err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='app_blobs'").Scan(&table); err != nil || table != "app_blobs" {
		t.Fatalf("blob table missing after legacy upgrade: %q err=%v", table, err)
	}
	if err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='index' AND name='idx_app_blobs_ready_order'").Scan(&index); err != nil || index != "idx_app_blobs_ready_order" {
		t.Fatalf("blob index missing after legacy upgrade: %q err=%v", index, err)
	}
	if err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='identity_sessions'").Scan(&table); err != nil || table != "identity_sessions" {
		t.Fatalf("identity session table missing after legacy upgrade: %q err=%v", table, err)
	}
	if err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='identity_handoffs'").Scan(&table); err != nil || table != "identity_handoffs" {
		t.Fatalf("identity handoff table missing after legacy upgrade: %q err=%v", table, err)
	}
}

func TestGlobalIdentityMigrationUpgradesExistingV4Database(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "v4.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	migrations, err := LoadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, checksum TEXT NOT NULL UNIQUE, applied_at TEXT NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	for _, migration := range migrations {
		if migration.Version > 4 {
			continue
		}
		if _, err = db.Exec(migration.SQL); err != nil {
			t.Fatalf("apply v%d: %v", migration.Version, err)
		}
		if _, err = db.Exec("INSERT INTO schema_migrations(version,checksum,applied_at) VALUES(?,?,?)", migration.Version, migration.Checksum, now.Format(time.RFC3339Nano)); err != nil {
			t.Fatal(err)
		}
	}
	legacyRaw := "legacy-v4-app-session"
	legacyHash := sha256.Sum256([]byte(legacyRaw))
	if _, err = db.Exec(`INSERT INTO users(id,normalized_email,role,status,created_at) VALUES('u','owner@example.com','deployer','active',?);
		INSERT INTO identities(id,normalized_email,created_at) VALUES('viewer','viewer@example.com',?);
		INSERT INTO applications(id,owner_user_id,slug,status,policy_revision,created_at,updated_at) VALUES('app','u','legacy-app','active',1,?,?);
		INSERT INTO sessions(id,scope,app_id,identity_id,secret_hash,expires_at,created_at) VALUES('legacy-app-session','app','app','viewer',?,?,?)`,
		now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), legacyHash[:], now.Add(time.Hour).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("seed valid v4 app session: %v", err)
	}
	if err = ApplyMigrations(context.Background(), db, now); err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	store := &SQLiteStore{DB: db, write: make(chan struct{}, 1)}
	store.write <- struct{}{}
	if _, err = store.Validate(context.Background(), "app", legacyRaw, now); err != sessions.ErrInvalid {
		t.Fatalf("legacy v4 app session remained valid after v5: %v", err)
	}
	var legacyRevoked sql.NullString
	if err = db.QueryRow("SELECT revoked_at FROM sessions WHERE id='legacy-app-session'").Scan(&legacyRevoked); err != nil || legacyRevoked.Valid {
		t.Fatalf("legacy v4 app session was mutated by additive v5 migration: %#v err=%v", legacyRevoked, err)
	}
	compatibilityRaw := "post-v5-brokerless-app-session"
	compatibilityHash := sha256.Sum256([]byte(compatibilityRaw))
	if _, err = db.Exec(`INSERT INTO sessions(id,scope,app_id,identity_id,secret_hash,expires_at,created_at)
		VALUES('post-v5-brokerless','app','app','viewer',?,?,?)`, compatibilityHash[:], now.Add(time.Hour).Format(time.RFC3339Nano), now.Add(time.Second).Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("seed post-v5 brokerless compatibility session: %v", err)
	}
	if _, err = store.Validate(context.Background(), "app", compatibilityRaw, now); err != nil {
		t.Fatalf("post-v5 brokerless compatibility session denied: %v", err)
	}
	corruptRaw := "corrupt-created-at-app-session"
	corruptHash := sha256.Sum256([]byte(corruptRaw))
	if _, err = db.Exec(`INSERT INTO sessions(id,scope,app_id,identity_id,secret_hash,expires_at,created_at)
		VALUES('corrupt-created-at','app','app','viewer',?,?,?)`, corruptHash[:], now.Add(time.Hour).Format(time.RFC3339Nano), "not-a-timestamp"); err != nil {
		t.Fatalf("seed corrupt app session: %v", err)
	}
	if _, err = store.Validate(context.Background(), "app", corruptRaw, now); err != sessions.ErrInvalid {
		t.Fatalf("corrupt app-session timestamp accepted: %v", err)
	}
	var column string
	rows, err := db.Query("PRAGMA table_info(sessions)")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var typ string
		var notNull, pk int
		var dflt any
		if err = rows.Scan(&cid, &column, &typ, &notNull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		if column == "identity_session_id" {
			break
		}
	}
	if column != "identity_session_id" {
		t.Fatal("sessions identity_session_id column missing after v4 upgrade")
	}
	rows.Close()
	rows, err = db.Query("PRAGMA table_info(identity_handoffs)")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	column = ""
	for rows.Next() {
		var cid int
		var typ string
		var notNull, pk int
		var dflt any
		if err = rows.Scan(&cid, &column, &typ, &notNull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		if column == "browser_binding_hash" {
			break
		}
	}
	if column != "browser_binding_hash" {
		t.Fatal("identity_handoffs browser_binding_hash column missing after v4 upgrade")
	}
	if err = rows.Close(); err != nil {
		t.Fatal(err)
	}
	var index string
	if err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='index' AND name='idx_identity_sessions_browser_binding_active'").Scan(&index); err != nil || index != "idx_identity_sessions_browser_binding_active" {
		t.Fatalf("active browser binding uniqueness index missing after v4 upgrade: %q err=%v", index, err)
	}
	binding := sha256.Sum256([]byte("browser-binding"))
	firstSecret := sha256.Sum256([]byte("first-global"))
	secondSecret := sha256.Sum256([]byte("second-global"))
	if _, err = db.Exec(`INSERT INTO identity_sessions(id,identity_id,family_id,secret_hash,browser_binding_hash,expires_at,last_seen_at,rotated_at,created_at)
		VALUES('binding-one','viewer','family-one',?,?,?,?,?,?)`, firstSecret[:], binding[:], now.Add(time.Hour).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("seed first browser binding family: %v", err)
	}
	childRaw := "parent-linked-app-session"
	childHash := sha256.Sum256([]byte(childRaw))
	if _, err = db.Exec(`INSERT INTO sessions(id,scope,app_id,identity_id,identity_session_id,secret_hash,expires_at,created_at)
		VALUES('parent-linked','app','app','viewer','binding-one',?,?,?)`, childHash[:], now.Add(time.Hour).Format(time.RFC3339Nano), now.Add(-time.Hour).Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("seed parent-linked app session: %v", err)
	}
	if _, err = db.Exec("UPDATE schema_migrations SET applied_at='not-a-timestamp' WHERE version=5"); err != nil {
		t.Fatalf("corrupt v5 cutoff: %v", err)
	}
	if _, err = store.Validate(context.Background(), "app", compatibilityRaw, now); err != sessions.ErrInvalid {
		t.Fatalf("corrupt v5 cutoff accepted parentless session: %v", err)
	}
	if _, err = store.Validate(context.Background(), "app", childRaw, now); err != nil {
		t.Fatalf("parent-linked session depends on corrupt parentless cutoff: %v", err)
	}
	if _, err = db.Exec(`INSERT INTO identity_sessions(id,identity_id,family_id,secret_hash,browser_binding_hash,expires_at,last_seen_at,rotated_at,created_at)
		VALUES('binding-two','viewer','family-two',?,?,?,?,?,?)`, secondSecret[:], binding[:], now.Add(time.Hour).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err == nil {
		t.Fatal("active browser binding uniqueness index allowed a second family")
	}
	var n int
	if err = db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version=5").Scan(&n); err != nil || n != 1 {
		t.Fatalf("v5 evidence n=%d err=%v", n, err)
	}
}

func TestGlobalIdentityMigrationPreservesHistoricalChecksumForBinaryRollback(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "historical-v5.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	if err = ApplyMigrations(context.Background(), db, now); err != nil {
		t.Fatalf("apply corrected migrations: %v", err)
	}
	if err = ApplyMigrations(context.Background(), db, now.Add(time.Minute)); err != nil {
		t.Fatalf("corrected runtime rejected its persisted v5 compatibility checksum: %v", err)
	}
	var checksum string
	if err = db.QueryRow("SELECT checksum FROM schema_migrations WHERE version=5").Scan(&checksum); err != nil || checksum != legacyGlobalViewerIdentityMigrationChecksum {
		t.Fatalf("additive v5 did not retain the previous-binary checksum %q: %v", checksum, err)
	}
}

func TestGlobalIdentityMigrationV5PinsApprovedAdditiveSQL(t *testing.T) {
	approved, err := migrationFS.ReadFile("sql/0005_global_viewer_identity.sql")
	if err != nil {
		t.Fatal(err)
	}
	checksum, err := migrationChecksum(5, approved)
	if err != nil {
		t.Fatalf("approved additive v5 migration rejected: %v", err)
	}
	if checksum != legacyGlobalViewerIdentityMigrationChecksum {
		t.Fatalf("approved additive v5 did not retain rollback checksum: %q", checksum)
	}
	altered := append(append([]byte(nil), approved...), []byte("\n-- unauthorized change\n")...)
	source := fstest.MapFS{
		"sql/0005_global_viewer_identity.sql": &fstest.MapFile{Data: altered},
	}
	if _, err = loadMigrations(source); err == nil {
		t.Fatal("altered v5 SQL loaded under the historical checksum")
	}
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "altered-v5.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = applyMigrations(context.Background(), db, time.Now().UTC(), source); err == nil {
		t.Fatal("altered v5 SQL executed under the historical checksum")
	}
	var created int
	if err = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='schema_migrations'").Scan(&created); err != nil || created != 0 {
		t.Fatalf("altered v5 SQL reached migration transaction: tables=%d err=%v", created, err)
	}
}
