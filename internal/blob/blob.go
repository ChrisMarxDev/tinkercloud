// Package blob provides the bounded, app-scoped V1 local blob capability.
package blob

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"mime"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/ChrisMarxDev/tinkercloud/internal/appauth"
	"github.com/ChrisMarxDev/tinkercloud/internal/capabilities"
)

var (
	ErrCapabilityUnavailable = errors.New("blob capability unavailable")
	ErrInvalidMetadata       = errors.New("invalid blob metadata")
	ErrInvalidID             = errors.New("invalid blob id")
	ErrInvalidList           = errors.New("invalid blob list")
	ErrQuotaExceeded         = errors.New("blob quota exceeded")
	ErrRateLimited           = errors.New("blob rate limited")
	ErrUnavailable           = errors.New("blob unavailable")
)

type Limits struct {
	BlobBytes         int64
	BlobsPerApp       int
	TotalBytesPerApp  int64
	ListLimit         int
	UploadsPerMinute  int
	ConcurrentUploads int
	UploadDuration    time.Duration
}

func DefaultLimits() Limits {
	return Limits{BlobBytes: 25000000, BlobsPerApp: 1000, TotalBytesPerApp: 250000000, ListLimit: 100, UploadsPerMinute: 20, ConcurrentUploads: 2, UploadDuration: 2 * time.Minute}
}

type Metadata struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Size        int64     `json:"size"`
	ContentType string    `json:"content_type"`
	CreatedAt   time.Time `json:"created_at"`
}
type ListResult struct {
	Blobs      []Metadata
	NextCursor string
}
type Key struct{ App, ID string }

// Repository is intentionally app-ID-first. It is reached only after the
// sealed gateway context has established the active app and viewer.
type Repository interface {
	Upload(context.Context, string, string, Metadata, io.Reader, Limits) (Metadata, error)
	Open(context.Context, string, string) (io.ReadCloser, Metadata, error)
	List(context.Context, string, string, int) (ListResult, error)
	Delete(context.Context, string, string) (bool, error)
	Reconcile(context.Context) error
}
type Service struct {
	limits Limits
	repo   Repository
	mu     sync.Mutex
	active map[string]int
	rates  map[string]rateState
	now    func() time.Time
}
type rateState struct {
	since time.Time
	count int
}

func New(l Limits, r Repository) *Service {
	if l.BlobBytes == 0 {
		l = DefaultLimits()
	}
	return &Service{limits: l, repo: r, active: map[string]int{}, rates: map[string]rateState{}, now: time.Now}
}
func (s *Service) app(a appauth.AuthorizationContext) (string, string, error) {
	if a == nil || !a.BlobsEnabled() {
		return "", "", ErrCapabilityUnavailable
	}
	app, identity, _, err := capabilities.Scope(a)
	return app, identity, err
}
func opaqueID() (string, error) {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return "blb_" + hex.EncodeToString(b), nil
}
func validID(v string) bool {
	if len(v) != 36 || !strings.HasPrefix(v, "blb_") {
		return false
	}
	for _, r := range v[4:] {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}
func validMeta(m Metadata) bool {
	_, ok := normalizeMeta(m)
	return ok
}
func normalizeMeta(m Metadata) (Metadata, bool) {
	if !utf8.ValidString(m.Name) || m.Name == "" || m.Name == "." || m.Name == ".." || strings.HasPrefix(m.Name, ".") || len([]byte(m.Name)) > 255 || strings.ContainsAny(m.Name, "/\\\x00") {
		return Metadata{}, false
	}
	if len(m.Name) >= 2 && m.Name[1] == ':' && ((m.Name[0] >= 'A' && m.Name[0] <= 'Z') || (m.Name[0] >= 'a' && m.Name[0] <= 'z')) {
		return Metadata{}, false
	}
	for _, r := range m.Name {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return Metadata{}, false
		}
	}
	if !utf8.ValidString(m.ContentType) || m.ContentType == "" || len([]byte(m.ContentType)) > 127 || strings.ContainsAny(m.ContentType, "\x00\r\n") {
		return Metadata{}, false
	}
	if !strings.Contains(strings.SplitN(m.ContentType, ";", 2)[0], "/") {
		return Metadata{}, false
	}
	media, _, err := mime.ParseMediaType(m.ContentType)
	if err != nil || media == "" {
		return Metadata{}, false
	}
	m.ContentType = media
	return m, true
}
func (s *Service) Upload(ctx context.Context, a appauth.AuthorizationContext, name, contentType string, r io.Reader) (Metadata, error) {
	app, identity, err := s.app(a)
	if err != nil {
		return Metadata{}, err
	}
	if s.repo == nil {
		return Metadata{}, ErrUnavailable
	}
	if !s.acquire(app) {
		return Metadata{}, ErrRateLimited
	}
	defer s.release(app)
	ctx, cancel := context.WithTimeout(ctx, s.limits.UploadDuration)
	defer cancel()
	id, err := opaqueID()
	if err != nil {
		return Metadata{}, err
	}
	m, ok := normalizeMeta(Metadata{ID: id, Name: name, ContentType: contentType})
	if !ok {
		return Metadata{}, ErrInvalidMetadata
	}
	return s.repo.Upload(ctx, app, identity, m, contextReader{ctx: ctx, r: r}, s.limits)
}
func (s *Service) acquire(app string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if len(s.rates) >= 1024 {
		for key, old := range s.rates {
			if now.Sub(old.since) >= time.Minute {
				delete(s.rates, key)
			}
		}
		if _, present := s.rates[app]; !present && len(s.rates) >= 1024 {
			return false
		}
	}
	state := s.rates[app]
	if state.since.IsZero() || now.Sub(state.since) >= time.Minute {
		state = rateState{since: now}
	}
	if state.count >= s.limits.UploadsPerMinute || s.active[app] >= s.limits.ConcurrentUploads {
		return false
	}
	state.count++
	s.rates[app] = state
	s.active[app]++
	return true
}
func (s *Service) release(app string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active[app]--
	if s.active[app] <= 0 {
		delete(s.active, app)
	}
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	select {
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	default:
		return r.r.Read(p)
	}
}
func (s *Service) Get(ctx context.Context, a appauth.AuthorizationContext, id string) (io.ReadCloser, Metadata, error) {
	app, _, err := s.app(a)
	if err != nil {
		return nil, Metadata{}, err
	}
	if !validID(id) {
		return nil, Metadata{}, ErrInvalidID
	}
	if s.repo == nil {
		return nil, Metadata{}, ErrUnavailable
	}
	return s.repo.Open(ctx, app, id)
}
func (s *Service) List(ctx context.Context, a appauth.AuthorizationContext, cursor string, limit int) (ListResult, error) {
	app, _, err := s.app(a)
	if err != nil {
		return ListResult{}, err
	}
	if cursor != "" && !validID(cursor) {
		return ListResult{}, ErrInvalidList
	}
	if limit < 1 || limit > s.limits.ListLimit {
		return ListResult{}, ErrInvalidList
	}
	if s.repo == nil {
		return ListResult{}, ErrUnavailable
	}
	out, err := s.repo.List(ctx, app, cursor, limit)
	if out.Blobs == nil {
		out.Blobs = []Metadata{}
	}
	return out, err
}
func (s *Service) Delete(ctx context.Context, a appauth.AuthorizationContext, id string) (bool, error) {
	app, _, err := s.app(a)
	if err != nil {
		return false, err
	}
	if !validID(id) {
		return false, ErrInvalidID
	}
	if s.repo == nil {
		return false, ErrUnavailable
	}
	return s.repo.Delete(ctx, app, id)
}
