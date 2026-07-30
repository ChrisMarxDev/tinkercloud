// Package collections defines TinyHost's bounded app-scoped JSON document
// primitive. It is intentionally not a general query or relational API.
package collections

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"regexp"
	"time"
	"unicode/utf8"

	"github.com/tinyhost/tiny/internal/appauth"
	"github.com/tinyhost/tiny/internal/capabilities"
)

var (
	ErrInvalidCollection = errors.New("invalid collection")
	ErrInvalidDocument   = errors.New("invalid document")
	ErrInvalidDocumentID = errors.New("invalid document id")
	ErrInvalidListLimit  = errors.New("invalid collection list limit")
	ErrSnapshotTooLarge  = errors.New("collection snapshot exceeds limit")
	ErrVersionConflict   = errors.New("collection version conflict")
	ErrQuotaExceeded     = errors.New("collection quota exceeded")
)

type Limits struct {
	CollectionBytes, DocumentBytes             int
	CollectionsPerApp, DocumentsPerCollection  int
	TotalBytesPerApp, ListLimit, SnapshotLimit int
}

func DefaultLimits() Limits {
	return Limits{
		CollectionBytes: 64, DocumentBytes: 64 << 10,
		CollectionsPerApp: 100, DocumentsPerCollection: 10_000,
		TotalBytesPerApp: 100 << 20, ListLimit: 100, SnapshotLimit: 1_000,
	}
}

type Document struct {
	ID        string          `json:"id"`
	Data      json.RawMessage `json:"data"`
	Version   uint64          `json:"version"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type ListResult struct {
	Documents  []Document `json:"documents"`
	NextCursor string     `json:"next_cursor,omitempty"`
	Revision   uint64     `json:"revision"`
}

type Snapshot struct {
	Collection string     `json:"collection"`
	Documents  []Document `json:"documents"`
	Revision   uint64     `json:"revision"`
}

// Mutation is returned only after the persistence transaction committed. A
// caller may therefore use it as an in-memory WebSocket freshness hint.
type Mutation struct {
	Collection string `json:"collection"`
	ID         string `json:"id"`
	Version    uint64 `json:"version"`
	Deleted    bool   `json:"deleted"`
	Revision   uint64 `json:"revision"`
}

// Repository is an internal transport seam. Its app argument is supplied only
// by Service after capabilities.Scope has checked the sealed gateway context.
type Repository interface {
	Create(context.Context, string, string, Document) (Document, Mutation, error)
	Get(context.Context, string, string, string) (*Document, error)
	Update(context.Context, string, string, string, json.RawMessage, *uint64) (Document, Mutation, error)
	Delete(context.Context, string, string, string, *uint64) (bool, Mutation, error)
	List(context.Context, string, string, string, int) (ListResult, error)
	Snapshot(context.Context, string, string, int) (Snapshot, error)
}

// ChangeSink receives only a successful committed mutation. It deliberately
// carries a freshness hint, not document data or a durable replay record.
type ChangeSink interface {
	PublishCollectionChange(context.Context, appauth.DataAuthorizationContext, Mutation)
}

type Service struct {
	repo   Repository
	limits Limits
	newID  func() (string, error)
	sink   ChangeSink
}

func New(repo Repository, limits Limits, sinks ...ChangeSink) *Service {
	if limits.CollectionBytes == 0 {
		limits = DefaultLimits()
	}
	s := &Service{repo: repo, limits: limits, newID: newDocumentID}
	if len(sinks) != 0 {
		s.sink = sinks[0]
	}
	return s
}

func (s *Service) scope(auth appauth.AuthorizationContext) (string, error) {
	app, _, _, err := capabilities.Scope(auth)
	return app, err
}

func (s *Service) Create(ctx context.Context, auth appauth.AuthorizationContext, collection string, data json.RawMessage) (Document, Mutation, error) {
	app, err := s.scope(auth)
	if err != nil {
		return Document{}, Mutation{}, err
	}
	if !validCollection(collection, s.limits.CollectionBytes) {
		return Document{}, Mutation{}, ErrInvalidCollection
	}
	if !validDocument(data, s.limits.DocumentBytes) {
		return Document{}, Mutation{}, ErrInvalidDocument
	}
	id, err := s.newID()
	if err != nil {
		return Document{}, Mutation{}, err
	}
	doc, mutation, err := s.repo.Create(ctx, app, collection, Document{ID: id, Data: copyJSON(data)})
	if err == nil && s.sink != nil {
		s.sink.PublishCollectionChange(ctx, auth, mutation)
	}
	return doc, mutation, err
}

func (s *Service) Get(ctx context.Context, auth appauth.AuthorizationContext, collection, id string) (*Document, error) {
	app, err := s.scope(auth)
	if err != nil {
		return nil, err
	}
	if !validCollection(collection, s.limits.CollectionBytes) {
		return nil, ErrInvalidCollection
	}
	if !validID(id) {
		return nil, ErrInvalidDocumentID
	}
	return s.repo.Get(ctx, app, collection, id)
}

func (s *Service) Update(ctx context.Context, auth appauth.AuthorizationContext, collection, id string, data json.RawMessage, expected *uint64) (Document, Mutation, error) {
	app, err := s.scope(auth)
	if err != nil {
		return Document{}, Mutation{}, err
	}
	if !validCollection(collection, s.limits.CollectionBytes) {
		return Document{}, Mutation{}, ErrInvalidCollection
	}
	if !validID(id) {
		return Document{}, Mutation{}, ErrInvalidDocumentID
	}
	if !validDocument(data, s.limits.DocumentBytes) {
		return Document{}, Mutation{}, ErrInvalidDocument
	}
	doc, mutation, err := s.repo.Update(ctx, app, collection, id, copyJSON(data), expected)
	if err == nil && s.sink != nil {
		s.sink.PublishCollectionChange(ctx, auth, mutation)
	}
	return doc, mutation, err
}

func (s *Service) Delete(ctx context.Context, auth appauth.AuthorizationContext, collection, id string, expected *uint64) (bool, Mutation, error) {
	app, err := s.scope(auth)
	if err != nil {
		return false, Mutation{}, err
	}
	if !validCollection(collection, s.limits.CollectionBytes) {
		return false, Mutation{}, ErrInvalidCollection
	}
	if !validID(id) {
		return false, Mutation{}, ErrInvalidDocumentID
	}
	deleted, mutation, err := s.repo.Delete(ctx, app, collection, id, expected)
	if err == nil && deleted && s.sink != nil {
		s.sink.PublishCollectionChange(ctx, auth, mutation)
	}
	return deleted, mutation, err
}

func (s *Service) List(ctx context.Context, auth appauth.AuthorizationContext, collection, cursor string, limit int) (ListResult, error) {
	app, err := s.scope(auth)
	if err != nil {
		return ListResult{}, err
	}
	if !validCollection(collection, s.limits.CollectionBytes) {
		return ListResult{}, ErrInvalidCollection
	}
	if cursor != "" && !validID(cursor) {
		return ListResult{}, ErrInvalidDocumentID
	}
	if limit < 1 || limit > s.limits.ListLimit {
		return ListResult{}, ErrInvalidListLimit
	}
	result, err := s.repo.List(ctx, app, collection, cursor, limit)
	if result.Documents == nil {
		result.Documents = []Document{}
	}
	return result, err
}

func (s *Service) Snapshot(ctx context.Context, auth appauth.AuthorizationContext, collection string) (Snapshot, error) {
	app, err := s.scope(auth)
	if err != nil {
		return Snapshot{}, err
	}
	if !validCollection(collection, s.limits.CollectionBytes) {
		return Snapshot{}, ErrInvalidCollection
	}
	result, err := s.repo.Snapshot(ctx, app, collection, s.limits.SnapshotLimit)
	if result.Documents == nil {
		result.Documents = []Document{}
	}
	return result, err
}

func validCollection(value string, maximum int) bool {
	return value != "" && utf8.ValidString(value) && len([]byte(value)) <= maximum && collectionName.MatchString(value)
}

// ValidCollection exposes the stable wire grammar to transport adapters.
func ValidCollection(value string) bool {
	return validCollection(value, DefaultLimits().CollectionBytes)
}

func validID(value string) bool {
	return documentID.MatchString(value)
}

// ValidDocumentID exposes the server-issued opaque ID grammar to transport
// adapters. It does not authorize access to an otherwise valid ID.
func ValidDocumentID(value string) bool { return validID(value) }

// ValidDocument exposes the bounded JSON-object grammar to the deployer data
// control API. It does not authorize an app or collection.
func ValidDocument(value json.RawMessage) bool {
	return validDocument(value, DefaultLimits().DocumentBytes)
}

// NewDocumentID keeps server-assigned document identifiers identical across
// the viewer SDK and owner control-plane paths.
func NewDocumentID() (string, error) { return newDocumentID() }

var (
	collectionName = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9_-]{0,62}[a-z0-9])?$`)
	documentID     = regexp.MustCompile(`^doc_[A-Za-z0-9_-]{22}$`)
)

func validDocument(value json.RawMessage, maximum int) bool {
	if len(value) == 0 || len(value) > maximum || !json.Valid(value) {
		return false
	}
	var object map[string]json.RawMessage
	return json.Unmarshal(value, &object) == nil && object != nil
}

func newDocumentID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "doc_" + base64.RawURLEncoding.EncodeToString(b), nil
}

func copyJSON(value json.RawMessage) json.RawMessage { return append(json.RawMessage(nil), value...) }
