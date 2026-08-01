package main

import (
	"context"
	"errors"
	"path/filepath"
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

func TestPublicCommandMutatesBeforeRefreshingAndKeepsStoppedServiceStopped(t *testing.T) {
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
	deployerServiceIsActive = func() (bool, error) { calls = append(calls, "is_active"); return false, nil }
	deployerServiceTryRestart = func() error { calls = append(calls, "try_restart"); return nil }
	err, output := runDeployerCommand(t, "public", "enable", "--config", configPath)
	if err != nil || output != "public static access enabled\n" {
		t.Fatalf("enable err=%v output=%q", err, output)
	}
	if got, want := strings.Join(calls, ","), "mutation_closed,is_active"; got != want {
		t.Fatalf("stopped refresh ordering=%q want=%q", got, want)
	}
}
