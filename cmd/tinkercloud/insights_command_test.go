package main

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ChrisMarxDev/tinkercloud/internal/persistence"
)

func TestInsightsCommandDurablyMutatesThenRefreshesRunningService(t *testing.T) {
	oldUID, oldPrepare, oldRunner := effectiveUID, prepareDeployerDatabaseOwnership, insightsMutationRunner
	oldActive, oldRestart := deployerServiceIsActive, deployerServiceTryRestart
	effectiveUID = func() int { return 0 }
	prepareDeployerDatabaseOwnership = func(string) error { return nil }
	t.Cleanup(func() {
		effectiveUID, prepareDeployerDatabaseOwnership, insightsMutationRunner = oldUID, oldPrepare, oldRunner
		deployerServiceIsActive, deployerServiceTryRestart = oldActive, oldRestart
	})
	configPath := deployerCommandConfig(t)
	var calls []string
	insightsMutationRunner = func(ctx context.Context, path string, enabled bool) error {
		calls = append(calls, "mutation_closed")
		store, err := persistence.OpenSQLite(ctx, path)
		if err != nil {
			return err
		}
		err = store.SetInsightsEnabled(ctx, enabled)
		closeErr := store.Close()
		if err != nil {
			return err
		}
		return closeErr
	}
	deployerServiceIsActive = func() (bool, error) { calls = append(calls, "is_active"); return true, nil }
	deployerServiceTryRestart = func() error { calls = append(calls, "try_restart"); return nil }

	err, output := runDeployerCommand(t, "insights", "disable", "--config", configPath)
	if err != nil || output != "local insights disabled\n" {
		t.Fatalf("disable err=%v output=%q", err, output)
	}
	if got, want := strings.Join(calls, ","), "mutation_closed,is_active,try_restart,is_active"; got != want {
		t.Fatalf("refresh ordering=%q want=%q", got, want)
	}
	store, err := persistence.OpenSQLite(context.Background(), filepath.Join(filepath.Dir(configPath), "data", "tinkercloud.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if enabled, err := store.InsightsEnabled(context.Background()); err != nil || enabled {
		t.Fatalf("durable setting enabled=%v err=%v", enabled, err)
	}

	calls = nil
	insightsMutationRunner = func(context.Context, string, bool) error {
		calls = append(calls, "mutation_failed")
		return errors.New("no")
	}
	err, output = runDeployerCommand(t, "insights", "enable", "--config", configPath)
	if err == nil || err.Error() != "tinkercloud: insights_failed" || output != "" {
		t.Fatalf("failure err=%v output=%q", err, output)
	}
	if got := strings.Join(calls, ","); got != "mutation_failed" {
		t.Fatalf("service refresh after failed mutation=%q", got)
	}
}

func TestInsightsCommandRejectsBadGrammarAndNonRoot(t *testing.T) {
	oldUID := effectiveUID
	effectiveUID = func() int { return 1 }
	t.Cleanup(func() { effectiveUID = oldUID })
	if err, output := runDeployerCommand(t, "insights", "disable", "--bad"); err == nil || err.Error() != "tinkercloud: root_required" || output != "" {
		t.Fatalf("non-root err=%v output=%q", err, output)
	}
}

func TestPublicCommandMutatesWithoutServiceRefresh(t *testing.T) {
	oldUID, oldPrepare, oldRunner := effectiveUID, prepareDeployerDatabaseOwnership, publicMutationRunner
	oldActive, oldRestart := deployerServiceIsActive, deployerServiceTryRestart
	effectiveUID = func() int { return 0 }
	prepareDeployerDatabaseOwnership = func(string) error { return nil }
	t.Cleanup(func() {
		effectiveUID, prepareDeployerDatabaseOwnership, publicMutationRunner = oldUID, oldPrepare, oldRunner
		deployerServiceIsActive, deployerServiceTryRestart = oldActive, oldRestart
	})
	configPath := deployerCommandConfig(t)
	var calls []string
	publicMutationRunner = func(context.Context, string, bool) error { calls = append(calls, "mutation_closed"); return nil }
	deployerServiceIsActive = func() (bool, error) { calls = append(calls, "is_active"); return true, nil }
	deployerServiceTryRestart = func() error { calls = append(calls, "try_restart"); return nil }
	err, output := runDeployerCommand(t, "public", "enable", "--config", configPath)
	if err != nil || output != "public static access enabled\n" {
		t.Fatalf("enable err=%v output=%q", err, output)
	}
	if got, want := strings.Join(calls, ","), "mutation_closed"; got != want {
		t.Fatalf("public command side effects=%q want=%q", got, want)
	}
	calls = nil
	publicMutationRunner = func(context.Context, string, bool) error {
		calls = append(calls, "mutation_failed")
		return errors.New("no")
	}
	err, output = runDeployerCommand(t, "public", "disable", "--config", configPath)
	if err == nil || err.Error() != "tinkercloud: public_failed" || output != "" {
		t.Fatalf("failed mutation err=%v output=%q", err, output)
	}
	if got := strings.Join(calls, ","); got != "mutation_failed" {
		t.Fatalf("failed mutation side effects=%q", got)
	}
}

func TestPublicCommandDurableEnableDisableRoundTripAndRootAudit(t *testing.T) {
	oldUID, oldPrepare, oldRunner := effectiveUID, prepareDeployerDatabaseOwnership, publicMutationRunner
	oldActive, oldRestart := deployerServiceIsActive, deployerServiceTryRestart
	effectiveUID = func() int { return 0 }
	prepareDeployerDatabaseOwnership = func(string) error { return nil }
	publicMutationRunner = func(ctx context.Context, path string, enabled bool) error {
		store, err := persistence.OpenSQLite(ctx, path)
		if err != nil {
			return err
		}
		gate, err := store.CurrentPublicGate(ctx)
		if err == nil {
			err = store.SetPublicGate(ctx, "root", enabled, gate.Revision, "test-public-"+strconv.FormatBool(enabled)+"-"+strconv.FormatUint(gate.Revision, 10))
		}
		closeErr := store.Close()
		if err != nil {
			return err
		}
		return closeErr
	}
	deployerServiceIsActive = func() (bool, error) { t.Fatal("public command queried systemd"); return false, nil }
	deployerServiceTryRestart = func() error { t.Fatal("public command restarted service"); return nil }
	t.Cleanup(func() {
		effectiveUID, prepareDeployerDatabaseOwnership, publicMutationRunner = oldUID, oldPrepare, oldRunner
		deployerServiceIsActive, deployerServiceTryRestart = oldActive, oldRestart
	})
	configPath := deployerCommandConfig(t)
	for _, action := range []string{"enable", "disable"} {
		err, output := runDeployerCommand(t, "public", action, "--config", configPath)
		if err != nil || output != "public static access "+action+"d\n" {
			t.Fatalf("%s err=%v output=%q", action, err, output)
		}
	}
	store, err := persistence.OpenSQLite(context.Background(), filepath.Join(filepath.Dir(configPath), "data", "tinkercloud.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	gate, err := store.CurrentPublicGate(context.Background())
	if err != nil || gate.Enabled || gate.Revision != 3 {
		t.Fatalf("round-trip gate=%#v err=%v", gate, err)
	}
	var audits int
	if err := store.DB.QueryRow("SELECT COUNT(*) FROM audit_events WHERE action='public_static_gate.set' AND actor_kind='root' AND actor_id IS NULL").Scan(&audits); err != nil || audits != 2 {
		t.Fatalf("root audits=%d err=%v", audits, err)
	}
}
