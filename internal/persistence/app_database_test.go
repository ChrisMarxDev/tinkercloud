package persistence

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAppDatabaseManagerCreatesIsolatedLazyDatabases(t *testing.T) {
	root := t.TempDir()
	m, err := NewAppDatabaseManager(root, AppDatabaseManagerOptions{MaxOpen: 1, IdleTTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	first, releaseFirst, err := m.Acquire(context.Background(), "app_a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = first.DB.Exec(`INSERT INTO app_kv(key,value_json,version,size_bytes,created_at,updated_at) VALUES('key','1',1,1,'now','now')`); err != nil {
		t.Fatal(err)
	}
	releaseFirst()
	second, releaseSecond, err := m.Acquire(context.Background(), "app_b")
	if err != nil {
		t.Fatal(err)
	}
	defer releaseSecond()
	var count int
	if err := second.DB.QueryRow(`SELECT COUNT(*) FROM app_kv`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("isolated app database count=%d err=%v", count, err)
	}
	for _, app := range []string{"app_a", "app_b"} {
		path, err := m.Path(app)
		if err != nil {
			t.Fatal(err)
		}
		if filepath.Base(path) != "data.db" {
			t.Fatalf("unexpected database path: %q", path)
		}
	}
}

func TestAppDataPurgerRemovesOnlyTargetDatabaseAfterRequestsFinish(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	m, err := NewAppDatabaseManager(s.DataRoot, AppDatabaseManagerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	_, releaseA, err := m.Acquire(context.Background(), "a")
	if err != nil {
		t.Fatal(err)
	}
	_, releaseB, err := m.Acquire(context.Background(), "b")
	if err != nil {
		t.Fatal(err)
	}
	releaseB()
	purger := AppDataPurger{DataRoot: s.DataRoot, Apps: m, BlobCleanup: &lifecycleBlobCleanupSpy{calls: make(chan struct{}, 1)}}
	if err := purger.Purge(context.Background(), "a"); err == nil {
		t.Fatal("purge raced an in-flight app database request")
	}
	if _, _, err := m.Acquire(context.Background(), "a"); err != ErrAppDatabaseRemoving {
		t.Fatalf("new app database request accepted during deletion: %v", err)
	}
	releaseA()
	if err := purger.Purge(context.Background(), "a"); err != nil {
		t.Fatal(err)
	}
	aPath, _ := m.Path("a")
	bPath, _ := m.Path("b")
	if _, err := os.Stat(aPath); !os.IsNotExist(err) {
		t.Fatalf("target database remained: %v", err)
	}
	if _, err := os.Stat(bPath); err != nil {
		t.Fatalf("other app database removed: %v", err)
	}
}

func TestAppDatabaseManagerRejectsPathLikeIDsAndClosesIdle(t *testing.T) {
	now := time.Now()
	m, err := NewAppDatabaseManager(t.TempDir(), AppDatabaseManagerOptions{IdleTTL: time.Second, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	for _, id := range []string{"../other", "a/b", "", "app space"} {
		if _, _, err := m.Acquire(context.Background(), id); err != ErrInvalidAppDatabaseID {
			t.Fatalf("id=%q err=%v", id, err)
		}
	}
	_, release, err := m.Acquire(context.Background(), "safe")
	if err != nil {
		t.Fatal(err)
	}
	release()
	now = now.Add(2 * time.Second)
	m.CloseIdle()
	m.mu.Lock()
	count := len(m.apps)
	m.mu.Unlock()
	if count != 0 {
		t.Fatalf("idle app database remained open: %d", count)
	}
}
