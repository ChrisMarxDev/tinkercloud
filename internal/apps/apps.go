package apps

import (
	"context"
	"errors"
	"sync"

	"github.com/ChrisMarxDev/tinkercloud/internal/releases"
)

type Status string

const (
	Active    Status = "active"
	Suspended Status = "suspended"
	Deleting  Status = "deleting"
	Failed    Status = "failed"
)

type App struct {
	ID, Slug, OwnerIdentityID, ReleaseRoot string
	// ReleaseEvidence is database-derived metadata for the currently active
	// immutable release. Resolving an app intentionally does not inspect this
	// path: the static dispatcher verifies it only after authorization.
	ReleaseEvidence                                            releases.FileManifest
	Status                                                     Status
	SPAFallback                                                bool
	KVEnabled, BlobsEnabled, RealtimeEnabled, LLMChatRequested bool
}

var ErrNotFound = errors.New("app not found")

type Repository interface {
	ResolveActive(context.Context, string) (App, error)
}
type MemoryRepository struct {
	mu     sync.RWMutex
	bySlug map[string]App
	Err    error
}

func NewMemoryRepository(apps ...App) *MemoryRepository {
	r := &MemoryRepository{bySlug: map[string]App{}}
	for _, a := range apps {
		r.bySlug[a.Slug] = a
	}
	return r
}
func (r *MemoryRepository) ResolveActive(_ context.Context, slug string) (App, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return App{}, r.Err
	}
	a, ok := r.bySlug[slug]
	if !ok || a.Status != Active {
		return App{}, ErrNotFound
	}
	return a, nil
}
func (r *MemoryRepository) Put(a App) { r.mu.Lock(); defer r.mu.Unlock(); r.bySlug[a.Slug] = a }
