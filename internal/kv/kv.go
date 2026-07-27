// Package kv provides the bounded, app-scoped V1 JSON KV capability.
package kv

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/tinyhost/tiny/internal/appauth"
	"github.com/tinyhost/tiny/internal/capabilities"
)

var (
	ErrInvalidKey            = errors.New("invalid kv key")
	ErrInvalidValue          = errors.New("invalid json value")
	ErrVersionConflict       = errors.New("kv version conflict")
	ErrQuotaExceeded         = errors.New("kv quota exceeded")
	ErrInvalidListLimit      = errors.New("invalid list limit")
	ErrCapabilityUnavailable = errors.New("kv capability unavailable")
)

type Limits struct{ KeyBytes, ValueBytes, KeysPerApp, TotalBytesPerApp, ListLimit int }

func DefaultLimits() Limits { return Limits{256, 64 << 10, 10000, 100 << 20, 100} }

type Entry struct {
	Key       string          `json:"key"`
	Value     json.RawMessage `json:"value"`
	Version   uint64          `json:"version"`
	UpdatedAt time.Time       `json:"updated_at"`
}
type ListResult struct {
	Entries    []Entry `json:"entries"`
	NextCursor string  `json:"next_cursor,omitempty"`
}
type Mutation struct {
	Key     string
	Value   json.RawMessage
	Version uint64
	Deleted bool
}
type ChangeSink interface {
	PublishKVChange(context.Context, appauth.AuthorizationContext, Mutation)
}

// Repository receives only the server-derived app ID from Service.
type Repository interface {
	Get(context.Context, string, string) (*Entry, error)
	Set(context.Context, string, string, json.RawMessage, *uint64) (Entry, error)
	Delete(context.Context, string, string, *uint64) (bool, Mutation, error)
	List(context.Context, string, string, string, int) (ListResult, error)
}

type Service struct {
	mu     sync.RWMutex
	limits Limits
	now    func() time.Time
	apps   map[string]map[string]Entry
	sink   ChangeSink
	repo   Repository
}

func New(limits Limits, sink ChangeSink) *Service {
	if limits.KeyBytes == 0 {
		limits = DefaultLimits()
	}
	s := &Service{limits: limits, now: time.Now, apps: map[string]map[string]Entry{}, sink: sink}
	s.repo = memoryRepo{s}
	return s
}
func NewWithRepository(limits Limits, sink ChangeSink, repo Repository) *Service {
	if limits.KeyBytes == 0 {
		limits = DefaultLimits()
	}
	return &Service{limits: limits, now: time.Now, sink: sink, repo: repo}
}

type memoryRepo struct{ s *Service }

func (m memoryRepo) Get(_ context.Context, app, key string) (*Entry, error) {
	m.s.mu.RLock()
	defer m.s.mu.RUnlock()
	e, ok := m.s.apps[app][key]
	if !ok {
		return nil, nil
	}
	c := clone(e)
	return &c, nil
}
func (m memoryRepo) Set(_ context.Context, app, key string, value json.RawMessage, expected *uint64) (Entry, error) {
	s := m.s
	s.mu.Lock()
	defer s.mu.Unlock()
	entries := s.apps[app]
	if entries == nil {
		entries = map[string]Entry{}
		s.apps[app] = entries
	}
	old, exists := entries[key]
	if expected != nil && (!exists || old.Version != *expected) {
		return Entry{}, ErrVersionConflict
	}
	if !exists && len(entries) >= s.limits.KeysPerApp {
		return Entry{}, ErrQuotaExceeded
	}
	total := 0
	for _, v := range entries {
		total += len(v.Value)
	}
	if total-len(old.Value)+len(value) > s.limits.TotalBytesPerApp {
		return Entry{}, ErrQuotaExceeded
	}
	e := Entry{Key: key, Value: append(json.RawMessage(nil), value...), Version: old.Version + 1, UpdatedAt: s.now().UTC()}
	entries[key] = e
	return clone(e), nil
}
func (m memoryRepo) Delete(_ context.Context, app, key string, expected *uint64) (bool, Mutation, error) {
	s := m.s
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.apps[app][key]
	if expected != nil && (!ok || old.Version != *expected) {
		return false, Mutation{}, ErrVersionConflict
	}
	if !ok {
		return false, Mutation{}, nil
	}
	delete(s.apps[app], key)
	return true, Mutation{Key: key, Version: old.Version + 1, Deleted: true}, nil
}
func (m memoryRepo) List(_ context.Context, app, prefix, cursor string, limit int) (ListResult, error) {
	s := m.s
	s.mu.RLock()
	defer s.mu.RUnlock()
	keys := []string{}
	for key := range s.apps[app] {
		if strings.HasPrefix(key, prefix) && key > cursor {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	out := ListResult{Entries: []Entry{}}
	for i, key := range keys {
		if i == limit {
			out.NextCursor = keys[i-1]
			break
		}
		out.Entries = append(out.Entries, clone(s.apps[app][key]))
	}
	return out, nil
}

func validKey(key string, max int) bool {
	return key != "" && utf8.ValidString(key) && len([]byte(key)) <= max && !strings.ContainsRune(key, '\x00') && json.Valid([]byte(`"`+strings.ReplaceAll(strings.ReplaceAll(key, `\\`, `\\\\`), `"`, `\\"`)+`"`))
}
func validateValue(value json.RawMessage, max int) bool {
	return len(value) > 0 && len(value) <= max && json.Valid(value)
}
func (s *Service) app(auth appauth.AuthorizationContext) (string, error) {
	// Check the server-derived manifest capability before resolving scope or
	// reaching a repository. A disabled capability must never become a storage
	// oracle through any operation, including prefix listing.
	if auth == nil || !auth.KVEnabled() {
		return "", ErrCapabilityUnavailable
	}
	app, _, _, err := capabilities.Scope(auth)
	return app, err
}
func clone(e Entry) Entry { e.Value = append(json.RawMessage(nil), e.Value...); return e }

func (s *Service) Get(ctx context.Context, auth appauth.AuthorizationContext, key string) (*Entry, error) {
	app, err := s.app(auth)
	if err != nil {
		return nil, err
	}
	if !validKey(key, s.limits.KeyBytes) {
		return nil, ErrInvalidKey
	}
	return s.repo.Get(ctx, app, key)
}
func (s *Service) Set(ctx context.Context, auth appauth.AuthorizationContext, key string, value json.RawMessage, expected *uint64) (Entry, error) {
	app, err := s.app(auth)
	if err != nil {
		return Entry{}, err
	}
	if !validKey(key, s.limits.KeyBytes) {
		return Entry{}, ErrInvalidKey
	}
	if !validateValue(value, s.limits.ValueBytes) {
		return Entry{}, ErrInvalidValue
	}
	e, err := s.repo.Set(ctx, app, key, value, expected)
	if err != nil {
		return Entry{}, err
	}
	if s.sink != nil {
		s.sink.PublishKVChange(ctx, auth, Mutation{Key: key, Value: append(json.RawMessage(nil), value...), Version: e.Version})
	}
	return clone(e), nil
}
func (s *Service) Delete(ctx context.Context, auth appauth.AuthorizationContext, key string, expected *uint64) (bool, error) {
	app, err := s.app(auth)
	if err != nil {
		return false, err
	}
	if !validKey(key, s.limits.KeyBytes) {
		return false, ErrInvalidKey
	}
	deleted, mutation, err := s.repo.Delete(ctx, app, key, expected)
	if err != nil {
		return false, err
	}
	if !deleted {
		return false, nil
	}
	if s.sink != nil {
		s.sink.PublishKVChange(ctx, auth, mutation)
	}
	return true, nil
}
func (s *Service) List(ctx context.Context, auth appauth.AuthorizationContext, prefix, cursor string, limit int) (ListResult, error) {
	app, err := s.app(auth)
	if err != nil {
		return ListResult{}, err
	}
	if len([]byte(prefix)) > s.limits.KeyBytes || len([]byte(cursor)) > s.limits.KeyBytes {
		return ListResult{}, ErrInvalidKey
	}
	if limit < 1 || limit > s.limits.ListLimit {
		return ListResult{}, ErrInvalidListLimit
	}
	result, err := s.repo.List(ctx, app, prefix, cursor, limit)
	if err != nil {
		return ListResult{}, err
	}
	// The public collection contract distinguishes an empty page from null.
	// Repositories are an internal seam, so normalize even third-party/test
	// implementations that return the zero value.
	if result.Entries == nil {
		result.Entries = []Entry{}
	}
	return result, nil
}
