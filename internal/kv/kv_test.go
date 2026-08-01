package kv

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ChrisMarxDev/tinkercloud/internal/appauth"
	"github.com/ChrisMarxDev/tinkercloud/internal/apps"
	"github.com/ChrisMarxDev/tinkercloud/internal/identity"
	"github.com/ChrisMarxDev/tinkercloud/internal/policies"
	"github.com/ChrisMarxDev/tinkercloud/internal/sessions"
)

func authorized(t *testing.T, appID string) appauth.AuthorizationContext {
	return authorizedViewerWithKV(t, appID, "u", true)
}

func authorizedWithKV(t *testing.T, appID string, enabled bool) appauth.AuthorizationContext {
	return authorizedViewerWithKV(t, appID, "u", enabled)
}

func authorizedViewer(t *testing.T, appID, viewerID string) appauth.AuthorizationContext {
	return authorizedViewerWithKV(t, appID, viewerID, true)
}

func authorizedViewerWithKV(t *testing.T, appID, viewerID string, enabled bool) appauth.AuthorizationContext {
	t.Helper()
	viewer := identity.Identity{ID: viewerID, Email: viewerID + "@example.com"}
	store := sessions.NewMemoryStore()
	token, _, err := store.Create(appID, viewer, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	policy := &policies.MemoryStore{Policies: map[string]policies.Policy{appID: {AppID: appID, OwnerIdentityID: viewerID, Valid: true}}}
	auth, err := appauth.Authorizer{Sessions: store, Policies: policy}.Authorize(context.Background(), apps.App{ID: appID, KVEnabled: enabled}, token, "req")
	if err != nil {
		t.Fatal(err)
	}
	return auth
}

type rejectingRepo struct {
	mu    sync.Mutex
	calls int
}

func (r *rejectingRepo) call()      { r.mu.Lock(); r.calls++; r.mu.Unlock() }
func (r *rejectingRepo) count() int { r.mu.Lock(); defer r.mu.Unlock(); return r.calls }

func (r *rejectingRepo) Get(context.Context, string, string) (*Entry, error) {
	r.call()
	return nil, nil
}
func (r *rejectingRepo) Set(context.Context, string, string, json.RawMessage, *uint64) (Entry, error) {
	r.call()
	return Entry{}, nil
}
func (r *rejectingRepo) Delete(context.Context, string, string, *uint64) (bool, Mutation, error) {
	r.call()
	return false, Mutation{}, nil
}
func (r *rejectingRepo) List(context.Context, string, string, string, int) (ListResult, error) {
	r.call()
	return ListResult{}, nil
}
func TestIsolationConflictAndQuota(t *testing.T) {
	s := New(Limits{KeyBytes: 10, ValueBytes: 20, KeysPerApp: 1, TotalBytesPerApp: 20, ListLimit: 10}, nil)
	a, b := authorized(t, "a"), authorized(t, "b")
	e, err := s.Set(context.Background(), a, "x", json.RawMessage(`{"a":1}`), nil)
	if err != nil || e.Version != 1 {
		t.Fatal(e, err)
	}
	if got, _ := s.Get(context.Background(), b, "x"); got != nil {
		t.Fatal("cross-app read")
	}
	wrong := uint64(2)
	if _, err := s.Set(context.Background(), a, "x", json.RawMessage(`1`), &wrong); !errors.Is(err, ErrVersionConflict) {
		t.Fatal(err)
	}
	if _, err := s.Set(context.Background(), a, "y", json.RawMessage(`1`), nil); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatal(err)
	}
}
func TestDenyInvalidAuthorizationAndInput(t *testing.T) {
	s := New(DefaultLimits(), nil)
	if _, err := s.Set(context.Background(), nil, "x", json.RawMessage(`1`), nil); err == nil {
		t.Fatal("nil authorization accepted")
	}
	if _, err := s.Set(context.Background(), authorized(t, "a"), "", json.RawMessage(`no`), nil); !errors.Is(err, ErrInvalidKey) {
		t.Fatal(err)
	}
}

func TestListEmptyPageHasNonNilEntries(t *testing.T) {
	s := New(DefaultLimits(), nil)
	result, err := s.List(context.Background(), authorized(t, "a"), "missing/", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if result.Entries == nil || len(result.Entries) != 0 || result.NextCursor != "" {
		t.Fatalf("unexpected empty page: %#v", result)
	}
}

func TestDisabledCapabilityNeverTouchesRepositoryIncludingList(t *testing.T) {
	repo := &rejectingRepo{}
	s := NewWithRepository(DefaultLimits(), nil, repo)
	auth := authorizedWithKV(t, "a", false)
	if _, err := s.Get(context.Background(), auth, "key"); !errors.Is(err, ErrCapabilityUnavailable) {
		t.Fatal(err)
	}
	if _, err := s.Set(context.Background(), auth, "key", json.RawMessage(`1`), nil); !errors.Is(err, ErrCapabilityUnavailable) {
		t.Fatal(err)
	}
	if _, err := s.Delete(context.Background(), auth, "key", nil); !errors.Is(err, ErrCapabilityUnavailable) {
		t.Fatal(err)
	}
	if _, err := s.List(context.Background(), auth, "", "", 1); !errors.Is(err, ErrCapabilityUnavailable) {
		t.Fatal(err)
	}
	if repo.count() != 0 {
		t.Fatalf("disabled capability touched repository %d times", repo.count())
	}
}

func TestRequestRateLimitDeniesBeforeRepositoryAndIsScopeIsolated(t *testing.T) {
	repo := &rejectingRepo{}
	s := NewWithRepository(Limits{KeyBytes: 10, ValueBytes: 20, KeysPerApp: 10, TotalBytesPerApp: 100, ListLimit: 10, RequestWindow: time.Minute, RequestsPerWindow: 1, MaxRateScopes: 4}, nil, repo)
	now := time.Unix(100, 0)
	s.now = func() time.Time { return now }
	a := authorized(t, "a")
	b := authorized(t, "b")
	if _, err := s.Get(context.Background(), a, "key"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(context.Background(), a, "key"); !errors.Is(err, ErrRateLimited) || repo.count() != 1 {
		t.Fatalf("rate denial touched repo: %v calls=%d", err, repo.count())
	}
	if _, err := s.Get(context.Background(), b, "key"); err != nil || repo.count() != 2 {
		t.Fatalf("cross-app budget leaked: %v calls=%d", err, repo.count())
	}
	now = now.Add(time.Minute + time.Nanosecond)
	if _, err := s.Get(context.Background(), a, "key"); err != nil || repo.count() != 3 {
		t.Fatalf("window did not recover: %v calls=%d", err, repo.count())
	}
}

func TestRequestRateLimitBoundedStateFailsClosed(t *testing.T) {
	repo := &rejectingRepo{}
	s := NewWithRepository(Limits{KeyBytes: 10, ValueBytes: 20, KeysPerApp: 10, TotalBytesPerApp: 100, ListLimit: 10, RequestWindow: time.Minute, RequestsPerWindow: 5, MaxRateScopes: 2}, nil, repo)
	if _, err := s.Get(context.Background(), authorized(t, "a"), "key"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(context.Background(), authorized(t, "b"), "key"); !errors.Is(err, ErrRateLimited) || repo.count() != 1 {
		t.Fatalf("state bound did not fail closed: %v calls=%d", err, repo.count())
	}
}

func TestRequestRateLimitAppBucketCannotBeBypassedByViewers(t *testing.T) {
	repo := &rejectingRepo{}
	s := NewWithRepository(Limits{KeyBytes: 10, ValueBytes: 20, KeysPerApp: 10, TotalBytesPerApp: 100, ListLimit: 10, RequestWindow: time.Minute, ViewerRequestsPerWindow: 10, AppRequestsPerWindow: 2, GlobalRequestsPerWindow: 10, MaxRateScopes: 10}, nil, repo)
	a := authorizedViewer(t, "app", "viewer-a")
	b := authorizedViewer(t, "app", "viewer-b")
	c := authorizedViewer(t, "app", "viewer-c")
	if _, err := s.Get(context.Background(), a, "key"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(context.Background(), b, "key"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(context.Background(), c, "key"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("third viewer bypassed app budget: %v", err)
	}
	if repo.count() != 2 {
		t.Fatalf("app-budget denial touched repository: %d", repo.count())
	}
	if _, exists := s.rates["viewer\x00app\x00viewer-c"]; exists {
		t.Fatal("denied request consumed its viewer bucket")
	}
}

func TestRequestRateLimitGlobalBucketFailsClosedWithoutConsumingScope(t *testing.T) {
	repo := &rejectingRepo{}
	s := NewWithRepository(Limits{KeyBytes: 10, ValueBytes: 20, KeysPerApp: 10, TotalBytesPerApp: 100, ListLimit: 10, RequestWindow: time.Minute, ViewerRequestsPerWindow: 10, AppRequestsPerWindow: 10, GlobalRequestsPerWindow: 2, MaxRateScopes: 10}, nil, repo)
	if _, err := s.Get(context.Background(), authorizedViewer(t, "a", "one"), "key"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(context.Background(), authorizedViewer(t, "b", "two"), "key"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(context.Background(), authorizedViewer(t, "c", "three"), "key"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("global budget did not deny: %v", err)
	}
	if repo.count() != 2 || len(s.global) != 2 {
		t.Fatalf("global denial touched protected work: calls=%d global=%d", repo.count(), len(s.global))
	}
	if _, exists := s.rates["viewer\x00c\x00three"]; exists {
		t.Fatal("global denial consumed a viewer bucket")
	}
}

func TestRequestRateLimitViewerScopeIsAppScoped(t *testing.T) {
	repo := &rejectingRepo{}
	s := NewWithRepository(Limits{KeyBytes: 10, ValueBytes: 20, KeysPerApp: 10, TotalBytesPerApp: 100, ListLimit: 10, RequestWindow: time.Minute, ViewerRequestsPerWindow: 1, AppRequestsPerWindow: 10, GlobalRequestsPerWindow: 10, MaxRateScopes: 10}, nil, repo)
	if _, err := s.Get(context.Background(), authorizedViewer(t, "a", "same-viewer"), "key"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(context.Background(), authorizedViewer(t, "b", "same-viewer"), "key"); err != nil {
		t.Fatalf("viewer budget leaked across apps: %v", err)
	}
	if _, err := s.Get(context.Background(), authorizedViewer(t, "a", "same-viewer"), "key"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("viewer scope did not limit: %v", err)
	}
}

func TestRequestRateLimitConcurrentBudget(t *testing.T) {
	repo := &rejectingRepo{}
	s := NewWithRepository(Limits{KeyBytes: 10, ValueBytes: 20, KeysPerApp: 10, TotalBytesPerApp: 100, ListLimit: 10, RequestWindow: time.Minute, RequestsPerWindow: 3, MaxRateScopes: 10}, nil, repo)
	auth := authorized(t, "a")
	var wg sync.WaitGroup
	var allowed int
	var mu sync.Mutex
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.Get(context.Background(), auth, "key"); err == nil {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if allowed != 3 || repo.count() != 3 {
		t.Fatalf("allowed=%d calls=%d", allowed, repo.count())
	}
}
