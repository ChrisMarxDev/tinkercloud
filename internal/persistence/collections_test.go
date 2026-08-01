package persistence

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ChrisMarxDev/tinkercloud/internal/collections"
)

func newCollectionRepo(t *testing.T, limits collections.Limits) (CollectionRepository, *AppDatabaseManager) {
	t.Helper()
	m, err := NewAppDatabaseManager(t.TempDir(), AppDatabaseManagerOptions{MaxOpen: 2, IdleTTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	return CollectionRepository{Apps: m, Limits: limits}, m
}

func testDocument(id, json string) collections.Document {
	return collections.Document{ID: id, Data: []byte(json)}
}

func TestCollectionRepositoryCRUDIsolationAndRevision(t *testing.T) {
	r, manager := newCollectionRepo(t, collections.DefaultLimits())
	defer manager.Close()
	ctx := context.Background()
	created, mutation, err := r.Create(ctx, "app_a", "tasks", testDocument("doc_abcdefghijklmnopqrstuv", `{"title":"ship"}`))
	if err != nil || created.Version != 1 || mutation.Revision != 1 || mutation.Deleted {
		t.Fatalf("create doc=%#v mutation=%#v err=%v", created, mutation, err)
	}
	if other, err := r.Get(ctx, "app_b", "tasks", created.ID); err != nil || other != nil {
		t.Fatalf("cross-app disclosure: doc=%#v err=%v", other, err)
	}
	updated, mutation, err := r.Update(ctx, "app_a", "tasks", created.ID, []byte(`{"title":"done"}`), &created.Version)
	if err != nil || updated.Version != 2 || mutation.Revision != 2 {
		t.Fatalf("update doc=%#v mutation=%#v err=%v", updated, mutation, err)
	}
	page, err := r.List(ctx, "app_a", "tasks", "", 10)
	if err != nil || page.Revision != 2 || len(page.Documents) != 1 || page.Documents[0].ID != created.ID {
		t.Fatalf("list=%#v err=%v", page, err)
	}
	deleted, mutation, err := r.Delete(ctx, "app_a", "tasks", created.ID, &updated.Version)
	if err != nil || !deleted || !mutation.Deleted || mutation.Version != 3 || mutation.Revision != 3 {
		t.Fatalf("delete=%v mutation=%#v err=%v", deleted, mutation, err)
	}
}

func TestCollectionRepositoryConcurrentVersionAndQuotaDenials(t *testing.T) {
	limits := collections.DefaultLimits()
	limits.DocumentsPerCollection = 1
	limits.TotalBytesPerApp = 20
	r, manager := newCollectionRepo(t, limits)
	defer manager.Close()
	ctx := context.Background()
	doc, _, err := r.Create(ctx, "app", "tasks", testDocument("doc_abcdefghijklmnopqrstuv", `{"x":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.Create(ctx, "app", "tasks", testDocument("doc_bcdefghijklmnopqrstuvw", `{"x":2}`)); !errors.Is(err, collections.ErrQuotaExceeded) {
		t.Fatalf("document quota err=%v", err)
	}
	var winners int
	var lock sync.Mutex
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if _, _, err := r.Update(ctx, "app", "tasks", doc.ID, []byte(`{"x":3}`), &doc.Version); err == nil {
				lock.Lock()
				winners++
				lock.Unlock()
			}
		}()
	}
	wait.Wait()
	if winners != 1 {
		t.Fatalf("optimistic update winners=%d", winners)
	}
}

func TestCollectionRepositoryFailedTransactionHasNoMutation(t *testing.T) {
	r, manager := newCollectionRepo(t, collections.DefaultLimits())
	defer manager.Close()
	ctx := context.Background()
	db, release, err := manager.Acquire(ctx, "app")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.DB.Exec(`CREATE TRIGGER deny_document BEFORE INSERT ON documents BEGIN SELECT RAISE(ABORT,'deny'); END`)
	release()
	if err != nil {
		t.Fatal(err)
	}
	_, mutation, err := r.Create(ctx, "app", "tasks", testDocument("doc_abcdefghijklmnopqrstuv", `{}`))
	if err == nil || mutation != (collections.Mutation{}) {
		t.Fatalf("err=%v mutation=%#v", err, mutation)
	}
	got, err := r.Get(ctx, "app", "tasks", "doc_abcdefghijklmnopqrstuv")
	if err != nil || got != nil {
		t.Fatalf("failed write persisted: doc=%#v err=%v", got, err)
	}
}

func TestCollectionRepositoryDeletedNamesDoNotConsumeCollectionCap(t *testing.T) {
	limits := collections.DefaultLimits()
	limits.CollectionsPerApp = 2
	r, manager := newCollectionRepo(t, limits)
	defer manager.Close()
	ctx := context.Background()

	create := func(collection, id string) collections.Document {
		t.Helper()
		doc, _, err := r.Create(ctx, "app", collection, testDocument(id, `{}`))
		if err != nil {
			t.Fatalf("create %s: %v", collection, err)
		}
		return doc
	}
	remove := func(collection string, doc collections.Document) {
		t.Helper()
		if deleted, _, err := r.Delete(ctx, "app", collection, doc.ID, &doc.Version); err != nil || !deleted {
			t.Fatalf("delete %s: deleted=%t err=%v", collection, deleted, err)
		}
	}

	alpha := create("alpha", "doc_abcdefghijklmnopqrstuv")
	remove("alpha", alpha)
	beta := create("beta", "doc_bcdefghijklmnopqrstuvw")
	remove("beta", beta)
	gamma := create("gamma", "doc_cdefghijklmnopqrstuvwx")
	remove("gamma", gamma)
	// alpha's historical revision remains, but its inactive name cannot count
	// against the active two-collection budget.
	alpha = create("alpha", "doc_defghijklmnopqrstuvwxy")
	page, err := r.List(ctx, "app", "alpha", "", 10)
	if err != nil || page.Revision != 3 || len(page.Documents) != 1 || page.Documents[0].ID != alpha.ID {
		t.Fatalf("recreated alpha=%#v err=%v", page, err)
	}
	_ = create("beta", "doc_efghijklmnopqrstuvwxyz")
	if _, _, err := r.Create(ctx, "app", "delta", testDocument("doc_fghijklmnopqrstuvwxyza", `{}`)); !errors.Is(err, collections.ErrQuotaExceeded) {
		t.Fatalf("third active collection err=%v", err)
	}
}

func TestCollectionRepositoryConcurrentCreateNearCollectionCapHasOneWinner(t *testing.T) {
	limits := collections.DefaultLimits()
	limits.CollectionsPerApp = 1
	r, manager := newCollectionRepo(t, limits)
	defer manager.Close()
	var wait sync.WaitGroup
	var lock sync.Mutex
	winners := 0
	for _, input := range []struct{ collection, id string }{
		{"alpha", "doc_abcdefghijklmnopqrstuv"},
		{"beta", "doc_bcdefghijklmnopqrstuvw"},
	} {
		input := input
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, _, err := r.Create(context.Background(), "app", input.collection, testDocument(input.id, `{}`))
			if err == nil {
				lock.Lock()
				winners++
				lock.Unlock()
				return
			}
			if !errors.Is(err, collections.ErrQuotaExceeded) {
				t.Errorf("create %s err=%v", input.collection, err)
			}
		}()
	}
	wait.Wait()
	if winners != 1 {
		t.Fatalf("concurrent collection-cap winners=%d", winners)
	}
}
