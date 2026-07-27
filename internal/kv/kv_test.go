package kv

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/tinyhost/tiny/internal/appauth"
	"github.com/tinyhost/tiny/internal/apps"
	"github.com/tinyhost/tiny/internal/identity"
	"github.com/tinyhost/tiny/internal/policies"
	"github.com/tinyhost/tiny/internal/sessions"
)

func authorized(t *testing.T, appID string) appauth.AuthorizationContext {
	return authorizedWithKV(t, appID, true)
}

func authorizedWithKV(t *testing.T, appID string, enabled bool) appauth.AuthorizationContext {
	t.Helper()
	viewer := identity.Identity{ID: "u", Email: "u@example.com"}
	store := sessions.NewMemoryStore()
	token, _, err := store.Create(appID, viewer, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	policy := &policies.MemoryStore{Policies: map[string]policies.Policy{appID: {AppID: appID, OwnerIdentityID: "u", Valid: true}}}
	auth, err := appauth.Authorizer{Sessions: store, Policies: policy}.Authorize(context.Background(), apps.App{ID: appID, KVEnabled: enabled}, token, "req")
	if err != nil {
		t.Fatal(err)
	}
	return auth
}

type rejectingRepo struct{ calls int }

func (r *rejectingRepo) Get(context.Context, string, string) (*Entry, error) {
	r.calls++
	return nil, nil
}
func (r *rejectingRepo) Set(context.Context, string, string, json.RawMessage, *uint64) (Entry, error) {
	r.calls++
	return Entry{}, nil
}
func (r *rejectingRepo) Delete(context.Context, string, string, *uint64) (bool, Mutation, error) {
	r.calls++
	return false, Mutation{}, nil
}
func (r *rejectingRepo) List(context.Context, string, string, string, int) (ListResult, error) {
	r.calls++
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
	if repo.calls != 0 {
		t.Fatalf("disabled capability touched repository %d times", repo.calls)
	}
}
