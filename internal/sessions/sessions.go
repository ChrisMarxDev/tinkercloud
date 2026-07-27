package sessions

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"github.com/tinyhost/tiny/internal/identity"
	"net/http"
	"sync"
	"time"
)

const AppCookieName = "__Host-tiny_app"

func AppCookie(token string, expiry time.Time) *http.Cookie {
	return &http.Cookie{Name: AppCookieName, Value: token, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, Expires: expiry}
}

type Session struct {
	ID, AppID string
	Identity  identity.Identity
	ExpiresAt time.Time
	Revoked   bool
}
type Store interface {
	Validate(context.Context, string, string, time.Time) (Session, error)
}

var ErrInvalid = errors.New("invalid session")

type MemoryStore struct {
	mu     sync.RWMutex
	tokens map[[32]byte]Session
}

func NewMemoryStore() *MemoryStore { return &MemoryStore{tokens: map[[32]byte]Session{}} }
func (s *MemoryStore) Create(appID string, viewer identity.Identity, expiry time.Time) (string, Session, error) {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		return "", Session{}, e
	}
	token := base64.RawURLEncoding.EncodeToString(b)
	v := Session{ID: token[:12], AppID: appID, Identity: viewer, ExpiresAt: expiry}
	s.mu.Lock()
	s.tokens[sha256.Sum256([]byte(token))] = v
	s.mu.Unlock()
	return token, v, nil
}
func (s *MemoryStore) Validate(_ context.Context, appID, token string, now time.Time) (Session, error) {
	if token == "" {
		return Session{}, ErrInvalid
	}
	s.mu.RLock()
	v, ok := s.tokens[sha256.Sum256([]byte(token))]
	s.mu.RUnlock()
	if !ok || v.Revoked || v.AppID != appID || !now.Before(v.ExpiresAt) {
		return Session{}, ErrInvalid
	}
	return v, nil
}
func (s *MemoryStore) Revoke(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h := sha256.Sum256([]byte(token))
	v, ok := s.tokens[h]
	if ok {
		v.Revoked = true
		s.tokens[h] = v
	}
}
