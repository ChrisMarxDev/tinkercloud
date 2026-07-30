package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/tinyhost/tiny/internal/config"
	"github.com/tinyhost/tiny/internal/persistence"
)

func fakeTinyhostIdentity(t *testing.T) {
	t.Helper()
	oldDrop, oldPrepare, oldMutation := dropToTinyhostIdentity, prepareDeployerDatabaseOwnership, deployerMutationRunner
	dropToTinyhostIdentity = func() error { return nil }
	prepareDeployerDatabaseOwnership = func(string) error { return nil }
	deployerMutationRunner = runDeployerMutationInProcess
	t.Cleanup(func() {
		dropToTinyhostIdentity = oldDrop
		prepareDeployerDatabaseOwnership = oldPrepare
		deployerMutationRunner = oldMutation
	})
}

func fakeDeployerServiceInactive(t *testing.T) {
	t.Helper()
	oldActive, oldRestart := deployerServiceIsActive, deployerServiceTryRestart
	deployerServiceIsActive = func() (bool, error) { return false, nil }
	deployerServiceTryRestart = func() error {
		t.Fatal("inactive service restarted")
		return nil
	}
	t.Cleanup(func() { deployerServiceIsActive, deployerServiceTryRestart = oldActive, oldRestart })
}

func deployerCommandConfig(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	cfg := config.Config{
		Domain: "apps.example.test", SessionCookie: "__Host-tiny_app",
		ListenHTTP: ":80", ListenHTTPS: ":443", DataDirectory: filepath.Join(root, "data"), ACMECachedir: filepath.Join(root, "acme"),
		ResendAPIKeyRef: "env:RESEND_API_KEY", HMACKeyRef: "env:TINYHOST_HMAC_KEY", EmailFrom: "sender@example.test", ACMEEmail: "operator@example.test",
		OTPExpiry: 10 * time.Minute, OTPMaxAttempts: 5, SessionExpiry: 24 * time.Hour,
	}
	if err := os.Mkdir(cfg.DataDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(cfg.ACMECachedir, 0700); err != nil {
		t.Fatal(err)
	}
	body, err := cfg.RenderYAML()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "config.yaml")
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func runDeployerCommand(t *testing.T, args ...string) (error, string) {
	t.Helper()
	out, err := os.CreateTemp(t.TempDir(), "deployer-output")
	if err != nil {
		t.Fatal(err)
	}
	runErr := run(args, out, out)
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(out.Name())
	if err != nil {
		t.Fatal(err)
	}
	return runErr, string(body)
}

func TestDeployersAuthorizeParsesActionFlagsAndMutatesSQLite(t *testing.T) {
	old := effectiveUID
	effectiveUID = func() int { return 0 }
	t.Cleanup(func() { effectiveUID = old })
	fakeTinyhostIdentity(t)
	fakeDeployerServiceInactive(t)

	configPath := deployerCommandConfig(t)
	err, output := runDeployerCommand(t, "deployers", "authorize", "--config", configPath, "Deployer@Example.COM")
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if output != "deployer authorize: Deployer@Example.COM\n" {
		t.Fatalf("output = %q", output)
	}
	store, err := persistence.OpenSQLite(context.Background(), filepath.Join(filepath.Dir(configPath), "data", "tinyhost.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var status string
	if err = store.DB.QueryRow("SELECT status FROM users WHERE normalized_email='Deployer@example.com' AND role='deployer'").Scan(&status); err != nil || status != "active" {
		t.Fatalf("authorized deployer = %q, %v", status, err)
	}
}

func TestDeployersRefreshesOnlyRunningServiceAfterDurableMutation(t *testing.T) {
	oldUID, oldMutation := effectiveUID, deployerMutationRunner
	oldActive, oldRestart := deployerServiceIsActive, deployerServiceTryRestart
	effectiveUID = func() int { return 0 }
	t.Cleanup(func() {
		effectiveUID, deployerMutationRunner = oldUID, oldMutation
		deployerServiceIsActive, deployerServiceTryRestart = oldActive, oldRestart
	})
	fakeTinyhostIdentity(t)
	configPath := deployerCommandConfig(t)

	var calls []string
	deployerMutationRunner = func(context.Context, string, string, string, string) error {
		calls = append(calls, "mutation_closed")
		return nil
	}
	deployerServiceIsActive = func() (bool, error) {
		calls = append(calls, "is_active")
		return true, nil
	}
	deployerServiceTryRestart = func() error {
		calls = append(calls, "try_restart")
		return nil
	}
	err, output := runDeployerCommand(t, "deployers", "authorize", "--config", configPath, "deployer@example.com")
	if err != nil || output != "deployer authorize: deployer@example.com\n" {
		t.Fatalf("success result err=%v output=%q", err, output)
	}
	if got, want := strings.Join(calls, ","), "mutation_closed,is_active,try_restart,is_active"; got != want {
		t.Fatalf("refresh ordering=%q want=%q", got, want)
	}

	calls = nil
	deployerMutationRunner = func(context.Context, string, string, string, string) error {
		calls = append(calls, "mutation_failed")
		return os.ErrPermission
	}
	err, output = runDeployerCommand(t, "deployers", "authorize", "--config", configPath, "deployer@example.com")
	if err == nil || err.Error() != "tinyhost: deployer_failed" || output != "" {
		t.Fatalf("mutation failure err=%v output=%q", err, output)
	}
	if got := strings.Join(calls, ","); got != "mutation_failed" {
		t.Fatalf("service refresh after failed mutation: %q", got)
	}

	calls = nil
	deployerMutationRunner = func(context.Context, string, string, string, string) error {
		calls = append(calls, "mutation_closed")
		return nil
	}
	deployerServiceIsActive = func() (bool, error) {
		calls = append(calls, "is_active")
		return false, nil
	}
	err, output = runDeployerCommand(t, "deployers", "authorize", "--config", configPath, "deployer@example.com")
	if err != nil || output != "deployer authorize: deployer@example.com\n" {
		t.Fatalf("inactive result err=%v output=%q", err, output)
	}
	if got := strings.Join(calls, ","); got != "mutation_closed,is_active" {
		t.Fatalf("inactive service refresh=%q", got)
	}
}

func TestDeployersDenyMalformedGrammarBeforeMutation(t *testing.T) {
	old := effectiveUID
	effectiveUID = func() int { return 0 }
	t.Cleanup(func() { effectiveUID = old })
	fakeTinyhostIdentity(t)
	fakeDeployerServiceInactive(t)

	configPath := deployerCommandConfig(t)
	secret := "not-a-real-secret-or-path"
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"missing action", []string{"deployers"}, "tinyhost: invalid_arguments"},
		{"invalid action", []string{"deployers", "wat", "--config", configPath, "a@example.com"}, "tinyhost: invalid_action"},
		{"extra argument", []string{"deployers", "authorize", "--config", configPath, "a@example.com", "extra"}, "tinyhost: invalid_arguments"},
		{"misplaced flag", []string{"deployers", "authorize", "a@example.com", "--config", configPath}, "tinyhost: invalid_arguments"},
		{"unknown flag", []string{"deployers", "authorize", "--unknown=" + secret, "a@example.com"}, "tinyhost: invalid_arguments"},
		{"invalid email", []string{"deployers", "authorize", "--config", configPath, "invalid"}, "tinyhost: deployer_failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err, output := runDeployerCommand(t, tc.args...)
			if err == nil || err.Error() != tc.want {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
			if output != "" || strings.Contains(output, secret) || strings.Contains(err.Error(), secret) {
				t.Fatalf("unredacted output/error: output=%q error=%v", output, err)
			}
		})
	}

	store, err := persistence.OpenSQLite(context.Background(), filepath.Join(filepath.Dir(configPath), "data", "tinyhost.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var count int
	if err = store.DB.QueryRow("SELECT COUNT(*) FROM users").Scan(&count); err != nil || count != 0 {
		t.Fatalf("denied command mutated users: count=%d err=%v", count, err)
	}
}

func TestDeployersRequireRootBeforeParsing(t *testing.T) {
	old := effectiveUID
	effectiveUID = func() int { return 1 }
	t.Cleanup(func() { effectiveUID = old })
	err, output := runDeployerCommand(t, "deployers", "authorize", "--unknown=not-a-real-secret-or-path", "a@example.com")
	if err == nil || err.Error() != "tinyhost: root_required" || output != "" {
		t.Fatalf("non-root result: output=%q error=%v", output, err)
	}
}

func TestDeployersCreatesDatabaseAndSidecarsAsServiceIdentity(t *testing.T) {
	oldUID, oldOpen := effectiveUID, openDeployerSQLite
	effectiveUID = func() int { return 0 }
	t.Cleanup(func() {
		effectiveUID = oldUID
		openDeployerSQLite = oldOpen
	})
	fakeTinyhostIdentity(t)
	fakeDeployerServiceInactive(t)

	configPath := deployerCommandConfig(t)
	opened := false
	openDeployerSQLite = func(ctx context.Context, path string) (*persistence.SQLiteStore, error) {
		s, err := persistence.OpenSQLite(ctx, path)
		if err != nil {
			return nil, err
		}
		opened = true
		for _, artifact := range []string{path, path + "-wal", path + "-shm"} {
			info, statErr := os.Stat(artifact)
			if statErr != nil {
				_ = s.Close()
				t.Fatalf("SQLite artifact %q unavailable: %v", artifact, statErr)
			}
			stat, ok := info.Sys().(*syscall.Stat_t)
			if !ok || stat.Uid != uint32(os.Geteuid()) || info.Mode().Perm()&0022 != 0 {
				_ = s.Close()
				t.Fatalf("SQLite artifact ownership/mode unsafe: %q %#v", artifact, info)
			}
		}
		return s, nil
	}

	err, output := runDeployerCommand(t, "deployers", "authorize", "--config", configPath, "service@example.com")
	if err != nil || output != "deployer authorize: service@example.com\n" || !opened {
		t.Fatalf("result err=%v output=%q opened=%v", err, output, opened)
	}
}

func TestDeployersIdentityOrDatabaseFailureDoesNotReportSuccess(t *testing.T) {
	oldUID, oldDrop, oldPrepare, oldOpen, oldMutation := effectiveUID, dropToTinyhostIdentity, prepareDeployerDatabaseOwnership, openDeployerSQLite, deployerMutationRunner
	effectiveUID = func() int { return 0 }
	t.Cleanup(func() {
		effectiveUID = oldUID
		dropToTinyhostIdentity = oldDrop
		prepareDeployerDatabaseOwnership = oldPrepare
		openDeployerSQLite = oldOpen
		deployerMutationRunner = oldMutation
	})

	configPath := deployerCommandConfig(t)
	databasePath := filepath.Join(filepath.Dir(configPath), "data", "tinyhost.db")
	prepareDeployerDatabaseOwnership = func(string) error { return nil }
	dropToTinyhostIdentity = func() error { return os.ErrPermission }
	deployerMutationRunner = runDeployerMutationInProcess
	openDeployerSQLite = func(context.Context, string) (*persistence.SQLiteStore, error) {
		t.Fatal("SQLite opened after service-identity failure")
		return nil, nil
	}
	err, output := runDeployerCommand(t, "deployers", "authorize", "--config", configPath, "service@example.com")
	if err == nil || err.Error() != "tinyhost: deployer_failed" || output != "" {
		t.Fatalf("identity failure result: err=%v output=%q", err, output)
	}
	if _, statErr := os.Stat(databasePath); !os.IsNotExist(statErr) {
		t.Fatalf("identity failure created database artifact: %v", statErr)
	}

	fakeTinyhostIdentity(t)
	fakeDeployerServiceInactive(t)
	openDeployerSQLite = func(context.Context, string) (*persistence.SQLiteStore, error) {
		return nil, os.ErrPermission
	}
	err, output = runDeployerCommand(t, "deployers", "authorize", "--config", configPath, "service@example.com")
	if err == nil || err.Error() != "tinyhost: deployer_failed" || output != "" {
		t.Fatalf("database failure result: err=%v output=%q", err, output)
	}
}

func TestDeployerDatabaseOwnershipHandoffCoversSQLiteSidecarsAndFailsClosed(t *testing.T) {
	oldLookup, oldOwner, oldChown := lookupTinyhostIdentity, databaseArtifactOwner, chownDeployerDatabaseArtifact
	t.Cleanup(func() {
		lookupTinyhostIdentity = oldLookup
		databaseArtifactOwner = oldOwner
		chownDeployerDatabaseArtifact = oldChown
	})
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	databasePath := filepath.Join(root, "tinyhost.db")
	for _, path := range []string{databasePath, databasePath + "-wal", databasePath + "-shm"} {
		if err := os.WriteFile(path, []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	lookupTinyhostIdentity = func() (serviceIdentity, error) { return serviceIdentity{uid: 123, gid: 456}, nil }
	databaseArtifactOwner = func(os.FileInfo) (uint32, uint32, bool) { return 0, 0, true }
	var got []string
	chownDeployerDatabaseArtifact = func(path string, uid, gid int) error {
		if uid != 123 || gid != 456 {
			t.Fatalf("handoff target = %d:%d", uid, gid)
		}
		got = append(got, path)
		return nil
	}
	if err := prepareDeployerDatabaseOwnership(databasePath); err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != strings.Join([]string{databasePath, databasePath + "-wal", databasePath + "-shm"}, ",") {
		t.Fatalf("handoff artifacts = %#v", got)
	}
	chownDeployerDatabaseArtifact = func(string, int, int) error { return os.ErrPermission }
	if err := prepareDeployerDatabaseOwnership(databasePath); err == nil {
		t.Fatal("ownership handoff accepted failed chown")
	}
}
