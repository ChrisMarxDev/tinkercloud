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
	ErrRateLimited           = errors.New("kv rate limited")
)

type Limits struct {
	KeyBytes, ValueBytes, KeysPerApp, TotalBytesPerApp, ListLimit int
	RequestWindow                                                 time.Duration
	// RequestsPerWindow is retained as the default for the viewer bucket so
	// existing callers keep their per-viewer budget when upgrading. New code
	// should set the three explicit bucket limits below.
	RequestsPerWindow                             int
	ViewerRequestsPerWindow, AppRequestsPerWindow int
	GlobalRequestsPerWindow, MaxRateScopes        int
}

func DefaultLimits() Limits {
	return Limits{KeyBytes: 256, ValueBytes: 64 << 10, KeysPerApp: 10000, TotalBytesPerApp: 100 << 20, ListLimit: 100, RequestWindow: time.Minute, RequestsPerWindow: 120, ViewerRequestsPerWindow: 120, AppRequestsPerWindow: 600, GlobalRequestsPerWindow: 10000, MaxRateScopes: 10000}
}

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
	PublishKVChange(context.Context, appauth.DataAuthorizationContext, Mutation)
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
	rates  map[string][]time.Time
	global []time.Time
}

func New(limits Limits, sink ChangeSink) *Service {
	if limits.KeyBytes == 0 {
		limits = DefaultLimits()
	}
	limits = normalizedLimits(limits)
	s := &Service{limits: limits, now: time.Now, apps: map[string]map[string]Entry{}, rates: map[string][]time.Time{}, sink: sink}
	s.repo = memoryRepo{s}
	return s
}
func NewWithRepository(limits Limits, sink ChangeSink, repo Repository) *Service {
	if limits.KeyBytes == 0 {
		limits = DefaultLimits()
	}
	limits = normalizedLimits(limits)
	return &Service{limits: limits, now: time.Now, rates: map[string][]time.Time{}, sink: sink, repo: repo}
}

func normalizedLimits(l Limits) Limits {
	d := DefaultLimits()
	if l.RequestWindow <= 0 {
		l.RequestWindow = d.RequestWindow
	}
	if l.RequestsPerWindow <= 0 {
		l.RequestsPerWindow = d.RequestsPerWindow
	}
	if l.ViewerRequestsPerWindow <= 0 {
		l.ViewerRequestsPerWindow = l.RequestsPerWindow
	}
	if l.AppRequestsPerWindow <= 0 {
		l.AppRequestsPerWindow = d.AppRequestsPerWindow
	}
	if l.GlobalRequestsPerWindow <= 0 {
		l.GlobalRequestsPerWindow = d.GlobalRequestsPerWindow
	}
	if l.MaxRateScopes <= 0 {
		l.MaxRateScopes = d.MaxRateScopes
	}
	return l
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

// ValidKey exposes the stable control-plane grammar without granting data
// access. Authorization remains the responsibility of its caller.
func ValidKey(key string) bool { return validKey(key, DefaultLimits().KeyBytes) }
func validateValue(value json.RawMessage, max int) bool {
	return len(value) > 0 && len(value) <= max && json.Valid(value)
}

// ValidValue exposes the bounded JSON value grammar to the deployer data API.
func ValidValue(value json.RawMessage) bool { return validateValue(value, DefaultLimits().ValueBytes) }
func (s *Service) app(auth appauth.AuthorizationContext) (string, string, error) {
	// Check the server-derived manifest capability before resolving scope or
	// reaching a repository. A disabled capability must never become a storage
	// oracle through any operation, including prefix listing.
	if auth == nil || !auth.KVEnabled() {
		return "", "", ErrCapabilityUnavailable
	}
	app, viewer, _, err := capabilities.Scope(auth)
	return app, viewer, err
}

func (s *Service) allow(app, viewer string) error {
	now := s.now()
	cutoff := now.Add(-s.limits.RequestWindow)
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, events := range s.rates {
		i := 0
		for i < len(events) && !events[i].After(cutoff) {
			i++
		}
		if i == len(events) {
			delete(s.rates, k)
		} else if i > 0 {
			s.rates[k] = append([]time.Time(nil), events[i:]...)
		}
	}
	s.global = retainRecent(s.global, cutoff)

	// The viewer bucket is deliberately app-scoped: an identity's activity in
	// one app cannot consume its budget in another. The app and global buckets
	// prevent a large allowlist from multiplying the available request rate.
	viewerKey := "viewer\x00" + app + "\x00" + viewer
	appKey := "app\x00" + app
	viewerEvents, viewerExists := s.rates[viewerKey]
	appEvents, appExists := s.rates[appKey]
	if len(viewerEvents) >= s.limits.ViewerRequestsPerWindow ||
		len(appEvents) >= s.limits.AppRequestsPerWindow ||
		len(s.global) >= s.limits.GlobalRequestsPerWindow {
		return ErrRateLimited
	}

	newScopes := 0
	if !viewerExists {
		newScopes++
	}
	if !appExists {
		newScopes++
	}
	// MaxRateScopes bounds only dynamic app/viewer scopes. The one global
	// bucket is a fixed-size part of every service and cannot be used to grow
	// state with untrusted app or viewer identifiers.
	if len(s.rates)+newScopes > s.limits.MaxRateScopes {
		return ErrRateLimited
	}

	// All three checks happen before any append. A denied request therefore
	// consumes no bucket and cannot starve another authorized scope.
	s.rates[viewerKey] = append(viewerEvents, now)
	s.rates[appKey] = append(appEvents, now)
	s.global = append(s.global, now)
	return nil
}

func retainRecent(events []time.Time, cutoff time.Time) []time.Time {
	i := 0
	for i < len(events) && !events[i].After(cutoff) {
		i++
	}
	if i == len(events) {
		return nil
	}
	if i == 0 {
		return events
	}
	return append([]time.Time(nil), events[i:]...)
}
func clone(e Entry) Entry { e.Value = append(json.RawMessage(nil), e.Value...); return e }

func (s *Service) Get(ctx context.Context, auth appauth.AuthorizationContext, key string) (*Entry, error) {
	app, viewer, err := s.app(auth)
	if err != nil {
		return nil, err
	}
	if !validKey(key, s.limits.KeyBytes) {
		return nil, ErrInvalidKey
	}
	if err := s.allow(app, viewer); err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, app, key)
}
func (s *Service) Set(ctx context.Context, auth appauth.AuthorizationContext, key string, value json.RawMessage, expected *uint64) (Entry, error) {
	app, viewer, err := s.app(auth)
	if err != nil {
		return Entry{}, err
	}
	if !validKey(key, s.limits.KeyBytes) {
		return Entry{}, ErrInvalidKey
	}
	if !validateValue(value, s.limits.ValueBytes) {
		return Entry{}, ErrInvalidValue
	}
	if err := s.allow(app, viewer); err != nil {
		return Entry{}, err
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
	app, viewer, err := s.app(auth)
	if err != nil {
		return false, err
	}
	if !validKey(key, s.limits.KeyBytes) {
		return false, ErrInvalidKey
	}
	if err := s.allow(app, viewer); err != nil {
		return false, err
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
	app, viewer, err := s.app(auth)
	if err != nil {
		return ListResult{}, err
	}
	if len([]byte(prefix)) > s.limits.KeyBytes || len([]byte(cursor)) > s.limits.KeyBytes {
		return ListResult{}, ErrInvalidKey
	}
	if limit < 1 || limit > s.limits.ListLimit {
		return ListResult{}, ErrInvalidListLimit
	}
	if err := s.allow(app, viewer); err != nil {
		return ListResult{}, err
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
