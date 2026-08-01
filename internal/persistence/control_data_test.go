package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/ChrisMarxDev/tinkercloud/internal/appauth"
	"github.com/ChrisMarxDev/tinkercloud/internal/collections"
	"github.com/ChrisMarxDev/tinkercloud/internal/controlapi"
	"github.com/ChrisMarxDev/tinkercloud/internal/kv"
)

type dataEventSpy struct{ kvApps, collectionApps []string }

func (s *dataEventSpy) PublishKVChange(_ context.Context, auth appauth.DataAuthorizationContext, _ kv.Mutation) {
	s.kvApps = append(s.kvApps, auth.AppID())
}
func (s *dataEventSpy) PublishCollectionChange(_ context.Context, auth appauth.DataAuthorizationContext, _ collections.Mutation) {
	s.collectionApps = append(s.collectionApps, auth.AppID())
}

func newControlDataService(t *testing.T) (ControlService, *SQLiteStore, func()) {
	t.Helper()
	store := seeded(t)
	apps, err := NewAppDatabaseManager(t.TempDir(), AppDatabaseManagerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	svc := ControlService{Store: store, AppDatabases: apps, DataKV: KVRepository{Apps: apps}, DataDocuments: CollectionRepository{Apps: apps}}
	return svc, store, func() { _ = apps.Close(); _ = store.Close() }
}

func TestDeployerDataOwnerScopeVersionsAndIdempotency(t *testing.T) {
	svc, store, cleanup := newControlDataService(t)
	defer cleanup()
	ctx := context.Background()
	actor := controlapi.Actor{ID: "u", Role: "deployer", Active: true}
	entry, err := svc.DataKVSet(ctx, actor, "alpha", "settings/theme", json.RawMessage(`"dark"`), nil, "set-1")
	if err != nil || entry.(kv.Entry).Version != 1 {
		t.Fatalf("set=%#v err=%v", entry, err)
	}
	replay, err := svc.DataKVSet(ctx, actor, "alpha", "settings/theme", json.RawMessage(`"dark"`), nil, "set-1")
	if err != nil || replay.(*kv.Entry).Version != 1 {
		t.Fatalf("replay=%#v err=%v", replay, err)
	}
	if _, err = svc.DataKVSet(ctx, actor, "alpha", "settings/theme", json.RawMessage(`"light"`), nil, "set-1"); err == nil {
		t.Fatal("mismatched replay accepted")
	}
	if _, err = svc.DataKVDelete(ctx, actor, "alpha", "settings/theme", nil, "delete-1"); !errors.Is(err, kv.ErrVersionConflict) {
		t.Fatal(err)
	}
	if _, err = store.DB.Exec("INSERT INTO users(id,normalized_email,role,status,created_at) VALUES('other','other@example.com','deployer','active',datetime('now'))"); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.DataKVGet(ctx, controlapi.Actor{ID: "other", Role: "deployer", Active: true}, "alpha", "settings/theme"); err == nil {
		t.Fatal("cross-owner read accepted")
	}
}

func TestDeployerDataSuspendedReadActiveWriteAndAuditFailure(t *testing.T) {
	svc, store, cleanup := newControlDataService(t)
	defer cleanup()
	ctx := context.Background()
	actor := controlapi.Actor{ID: "u", Role: "deployer", Active: true}
	if _, err := svc.DataKVSet(ctx, actor, "alpha", "x", json.RawMessage(`1`), nil, "one"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DataKV.(KVRepository).Set(ctx, "b", "x", json.RawMessage(`1`), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DataKVGet(ctx, actor, "beta", "x"); err != nil {
		t.Fatalf("suspended read denied: %v", err)
	}
	if _, err := svc.DataKVSet(ctx, actor, "beta", "x", json.RawMessage(`2`), nil, "two"); err == nil {
		t.Fatal("suspended write accepted")
	}
	if _, err := store.DB.Exec("CREATE TRIGGER deny_data_audit BEFORE INSERT ON audit_events WHEN NEW.action='data.kv.set' BEGIN SELECT RAISE(ABORT,'deny'); END"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DataKVSet(ctx, actor, "alpha", "blocked", json.RawMessage(`1`), nil, "blocked"); err == nil {
		t.Fatal("audit failure accepted mutation")
	}
	if value, err := svc.DataKVGet(ctx, actor, "alpha", "blocked"); err == nil || value != nil {
		t.Fatal("unaudited data write")
	}
}

func TestDeployerDataMutationPublishesOnlyAfterCommitForDerivedApp(t *testing.T) {
	svc, _, cleanup := newControlDataService(t)
	defer cleanup()
	spy := &dataEventSpy{}
	svc.DataEvents = spy
	actor := controlapi.Actor{ID: "u", Role: "deployer", Active: true}
	if _, err := svc.DataKVSet(context.Background(), actor, "alpha", "updates/one", json.RawMessage(`1`), nil, "event-1"); err != nil {
		t.Fatal(err)
	}
	if len(spy.kvApps) != 1 || spy.kvApps[0] != "a" {
		t.Fatalf("unexpected hints %#v", spy.kvApps)
	}
	// A completed idempotent retry returns a receipt but is not a new change.
	if _, err := svc.DataKVSet(context.Background(), actor, "alpha", "updates/one", json.RawMessage(`1`), nil, "event-1"); err != nil {
		t.Fatal(err)
	}
	if len(spy.kvApps) != 1 {
		t.Fatalf("replay re-published %#v", spy.kvApps)
	}
	if _, err := svc.DataKVSet(context.Background(), actor, "alpha", "bad\x00key", json.RawMessage(`1`), nil, "event-2"); err == nil {
		t.Fatal("invalid mutation accepted")
	}
	if len(spy.kvApps) != 1 {
		t.Fatal("failed mutation emitted hint")
	}
}

func TestDeployerDataReconcilesCommittedMutationAfterOutcomeFailure(t *testing.T) {
	svc, store, cleanup := newControlDataService(t)
	defer cleanup()
	spy := &dataEventSpy{}
	svc.DataEvents = spy
	ctx := context.Background()
	actor := controlapi.Actor{ID: "u", Role: "deployer", Active: true}
	initial, err := svc.DataKVSet(ctx, actor, "alpha", "recover", json.RawMessage(`"before"`), nil, "recover-initial")
	if err != nil {
		t.Fatal(err)
	}
	expected := initial.(kv.Entry).Version
	if _, err = store.DB.Exec(`CREATE TRIGGER deny_data_outcome
		BEFORE UPDATE OF outcome ON audit_events
		WHEN NEW.action='data.kv.set' AND NEW.outcome='succeeded'
		BEGIN SELECT RAISE(ABORT,'deny'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.DataKVSet(ctx, actor, "alpha", "recover", json.RawMessage(`"after"`), &expected, "recover-set"); err == nil {
		t.Fatal("outcome failure reported mutation success")
	}
	if len(spy.kvApps) != 1 {
		t.Fatalf("failed completion emitted hint: %#v", spy.kvApps)
	}
	if _, err = store.DB.Exec("DROP TRIGGER deny_data_outcome"); err != nil {
		t.Fatal(err)
	}
	replayed, err := svc.DataKVSet(ctx, actor, "alpha", "recover", json.RawMessage(`"after"`), &expected, "recover-set")
	if err != nil || replayed.(*kv.Entry).Version != expected+1 {
		t.Fatalf("reconciled replay=%#v err=%v", replayed, err)
	}
	if len(spy.kvApps) != 2 || spy.kvApps[1] != "a" {
		t.Fatalf("recovery hint missing: %#v", spy.kvApps)
	}
	if _, err = svc.DataKVSet(ctx, actor, "alpha", "recover", json.RawMessage(`"different"`), &expected, "recover-set"); err == nil {
		t.Fatal("mismatched incomplete replay accepted")
	}
}

func TestDeployerDataCreateReconcilesReservedDocumentAfterOutcomeFailure(t *testing.T) {
	svc, store, cleanup := newControlDataService(t)
	defer cleanup()
	spy := &dataEventSpy{}
	svc.DataEvents = spy
	ctx := context.Background()
	actor := controlapi.Actor{ID: "u", Role: "deployer", Active: true}
	if _, err := store.DB.Exec(`CREATE TRIGGER deny_document_outcome
		BEFORE UPDATE OF outcome ON audit_events
		WHEN NEW.action='data.document.create' AND NEW.outcome='succeeded'
		BEGIN SELECT RAISE(ABORT,'deny'); END`); err != nil {
		t.Fatal(err)
	}
	body := json.RawMessage(`{"title":"recover"}`)
	if _, err := svc.DataDocumentCreate(ctx, actor, "alpha", "tasks", body, "recover-create"); err == nil {
		t.Fatal("outcome failure reported document success")
	}
	if len(spy.collectionApps) != 0 {
		t.Fatalf("failed completion emitted hint: %#v", spy.collectionApps)
	}
	if _, err := store.DB.Exec("DROP TRIGGER deny_document_outcome"); err != nil {
		t.Fatal(err)
	}
	replayed, err := svc.DataDocumentCreate(ctx, actor, "alpha", "tasks", body, "recover-create")
	doc, ok := replayed.(*collections.Document)
	if err != nil || !ok || doc.ID == "" || doc.Version != 1 {
		t.Fatalf("reconciled document=%#v err=%v", replayed, err)
	}
	if len(spy.collectionApps) != 1 || spy.collectionApps[0] != "a" {
		t.Fatalf("document recovery hint missing: %#v", spy.collectionApps)
	}
}
