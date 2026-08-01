package persistence

import (
	"context"
	"database/sql"
	"errors"
	"github.com/ChrisMarxDev/tinkercloud/internal/kv"
	"github.com/ChrisMarxDev/tinkercloud/internal/operations"
	"sync"
	"testing"
)

func newKVRepository(t *testing.T, limits kv.Limits, gate operations.WriteGate) (KVRepository, *AppDatabaseManager) {
	t.Helper()
	apps, err := NewAppDatabaseManager(t.TempDir(), AppDatabaseManagerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return KVRepository{Apps: apps, Limits: limits, WriteGate: gate}, apps
}

type stoppedKVGate struct{}

func (stoppedKVGate) AllowWrite(context.Context, operations.WriteKind) error {
	return operations.ErrWriteDisabled
}

func TestSQLiteKVIsolationAndVersioning(t *testing.T) {
	r, apps := newKVRepository(t, kv.Limits{}, nil)
	defer apps.Close()
	ctx := context.Background()
	a, e := r.Set(ctx, "a", "p/one", []byte(`1`), nil)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = r.Set(ctx, "b", "p/one", []byte(`2`), nil); e != nil {
		t.Fatal(e)
	}
	b, _ := r.Get(ctx, "b", "p/one")
	if string(b.Value) != "2" {
		t.Fatal("tenant leak")
	}
	if _, e = r.Set(ctx, "a", "p/one", []byte(`3`), ptr(a.Version+1)); e != kv.ErrVersionConflict {
		t.Fatal(e)
	}
	a, e = r.Set(ctx, "a", "p/one", []byte(`3`), ptr(a.Version))
	if e != nil || a.Version != 2 {
		t.Fatal(e, a.Version)
	}
	if ok, _, e := r.Delete(ctx, "a", "p/one", ptr(2)); e != nil || !ok {
		t.Fatal(e)
	}
	if x, _ := r.Get(ctx, "a", "p/one"); x != nil {
		t.Fatal("delete failed")
	}
}
func TestSQLiteKVConcurrentExpectedOneWinner(t *testing.T) {
	r, apps := newKVRepository(t, kv.Limits{}, nil)
	defer apps.Close()
	e, _ := r.Set(context.Background(), "a", "x", []byte(`1`), nil)
	var wg sync.WaitGroup
	wins := 0
	var mu sync.Mutex
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, x := r.Set(context.Background(), "a", "x", []byte(`2`), ptr(e.Version)); x == nil {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Fatalf("wins=%d", wins)
	}
}
func ptr(x uint64) *uint64 { return &x }
func TestSQLiteKVQuotaAndList(t *testing.T) {
	r, apps := newKVRepository(t, kv.Limits{KeyBytes: 10, ValueBytes: 10, KeysPerApp: 2, TotalBytesPerApp: 4, ListLimit: 2}, nil)
	defer apps.Close()
	ctx := context.Background()
	if _, e := r.Set(ctx, "a", "a", []byte(`1`), nil); e != nil {
		t.Fatal(e)
	}
	if _, e := r.Set(ctx, "a", "b", []byte(`22`), nil); e != nil {
		t.Fatal(e)
	}
	if _, e := r.Set(ctx, "a", "c", []byte(`1`), nil); e != kv.ErrQuotaExceeded {
		t.Fatal(e)
	}
	a, _ := r.Get(ctx, "a", "a")
	if _, e := r.Set(ctx, "a", "a", []byte(`999`), ptr(a.Version)); e != kv.ErrQuotaExceeded {
		t.Fatal(e)
	}
	if ok, _, e := r.Delete(ctx, "a", "b", nil); e != nil || !ok {
		t.Fatal(e)
	}
	if _, e := r.Set(ctx, "a", "c", []byte(`1`), nil); e != nil {
		t.Fatal(e)
	}
	if _, e := r.Set(ctx, "b", "a", []byte(`999`), nil); e != nil {
		t.Fatal("cross-app quota", e)
	}
	list, e := r.List(ctx, "a", "", "", 1)
	if e != nil || len(list.Entries) != 1 || list.Entries[0].Key != "a" {
		t.Fatal(e, list)
	}
	list, e = r.List(ctx, "a", "", list.Entries[0].Key, 2)
	if e != nil || len(list.Entries) != 1 || list.Entries[0].Key != "c" {
		t.Fatal(e, list)
	}
}

func TestSQLiteKVListEmptyPageHasNonNilEntries(t *testing.T) {
	r, apps := newKVRepository(t, kv.Limits{}, nil)
	defer apps.Close()
	list, err := r.List(context.Background(), "a", "missing/", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if list.Entries == nil || len(list.Entries) != 0 || list.NextCursor != "" {
		t.Fatalf("unexpected empty page: %#v", list)
	}
}

func TestKVRepositoryDiskStopDeniesMutationsButNotReads(t *testing.T) {
	r, apps := newKVRepository(t, kv.Limits{}, stoppedKVGate{})
	defer apps.Close()
	if _, err := r.Set(context.Background(), "a", "key", []byte(`1`), nil); !errors.Is(err, operations.ErrWriteDisabled) {
		t.Fatal(err)
	}
	if _, _, err := r.Delete(context.Background(), "a", "key", nil); !errors.Is(err, operations.ErrWriteDisabled) {
		t.Fatal(err)
	}
	if got, err := r.Get(context.Background(), "a", "key"); err != nil || got != nil {
		t.Fatal(got, err)
	}
}

func TestKVRepositoryWithoutAppDatabaseManagerFailsClosed(t *testing.T) {
	r := KVRepository{}
	if _, err := r.Get(context.Background(), "app", "key"); !errors.Is(err, sql.ErrConnDone) {
		t.Fatalf("missing app database manager err=%v", err)
	}
}
