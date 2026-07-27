package persistence

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tinyhost/tiny/internal/controlapi"
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
	if err = db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version IN (2,3,4)").Scan(&n); err != nil || n != 3 {
		t.Fatalf("migration 2/3/4 evidence n=%d err=%v", n, err)
	}
	var table, index string
	if err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='app_blobs'").Scan(&table); err != nil || table != "app_blobs" {
		t.Fatalf("blob table missing after legacy upgrade: %q err=%v", table, err)
	}
	if err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='index' AND name='idx_app_blobs_ready_order'").Scan(&index); err != nil || index != "idx_app_blobs_ready_order" {
		t.Fatalf("blob index missing after legacy upgrade: %q err=%v", index, err)
	}
}
