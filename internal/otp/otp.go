// Package otp implements the provider-neutral, in-memory M1 challenge domain.
package otp

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"
)

var (
	ErrInvalid     = errors.New("invalid challenge")
	ErrUnavailable = errors.New("otp unavailable")
)

type Challenge struct {
	ID, AppID, Email          string
	CodeHash                  [32]byte
	ExpiresAt                 time.Time
	Attempts, MaxAttempts     int
	ConsumedAt, InvalidatedAt *time.Time
}
type Outbox interface {
	EnqueueOTP(context.Context, Message) error
}
type Message struct{ AppID, Email, Code, ChallengeID string }

// Store's Consume is transaction-capable: durable implementations must consume
// the challenge and create the app session in one database transaction.
type Store interface {
	Create(context.Context, Challenge) error
	InvalidateActive(context.Context, string, string, time.Time) error
	Consume(context.Context, string, string, string, time.Time, func(Challenge) error) error
}
type Service struct {
	Store       Store
	Outbox      Outbox
	Key         []byte
	Clock       func() time.Time
	TTL         time.Duration
	MaxAttempts int
}

func (s Service) now() time.Time {
	if s.Clock != nil {
		return s.Clock()
	}
	return time.Now()
}
func (s Service) Request(ctx context.Context, appID, email string, eligible bool) error {
	if s.Store == nil || len(s.Key) == 0 {
		return ErrUnavailable
	}
	code, err := numericCode()
	if err != nil {
		return ErrUnavailable
	}
	now := s.now()
	id, err := randomID()
	if err != nil {
		return ErrUnavailable
	}
	if err = s.Store.InvalidateActive(ctx, appID, email, now); err != nil {
		return ErrUnavailable
	}
	ttl := s.TTL
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	c := Challenge{ID: id, AppID: appID, Email: email, CodeHash: hash(s.Key, id, code), ExpiresAt: now.Add(ttl), MaxAttempts: s.MaxAttempts}
	if c.MaxAttempts == 0 {
		c.MaxAttempts = 5
	}
	if err = s.Store.Create(ctx, c); err != nil {
		return ErrUnavailable
	}
	if eligible && s.Outbox != nil {
		if err = s.Outbox.EnqueueOTP(ctx, Message{AppID: appID, Email: email, Code: code, ChallengeID: id}); err != nil {
			// A delivery outage must not disclose eligibility. No usable code was
			// delivered, so new authentication remains fail-closed.
			return nil
		}
	}
	return nil
}
func (s Service) Verify(ctx context.Context, appID, email, id, code string, createSession func(Challenge) error) error {
	if s.Store == nil || len(s.Key) == 0 || createSession == nil {
		return ErrInvalid
	}
	return s.Store.Consume(ctx, appID, email, id, s.now(), func(c Challenge) error {
		actual := hash(s.Key, id, code)
		if subtle.ConstantTimeCompare(c.CodeHash[:], actual[:]) != 1 {
			return ErrInvalid
		}
		return createSession(c)
	})
}
func hash(key []byte, id, code string) [32]byte {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write([]byte(id + ":" + code))
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}
func numericCode() (string, error) {
	n, e := rand.Int(rand.Reader, big.NewInt(1000000))
	if e != nil {
		return "", e
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}
func randomID() (string, error) {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return fmt.Sprintf("otp_%x", b), nil
}

type MemoryStore struct {
	mu         sync.Mutex
	challenges map[string]Challenge
}

func NewMemoryStore() *MemoryStore { return &MemoryStore{challenges: map[string]Challenge{}} }
func (m *MemoryStore) Create(_ context.Context, c Challenge) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.challenges[c.ID] = c
	return nil
}
func (m *MemoryStore) InvalidateActive(_ context.Context, app, email string, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, c := range m.challenges {
		if c.AppID == app && c.Email == email && c.ConsumedAt == nil && c.InvalidatedAt == nil {
			c.InvalidatedAt = &now
			m.challenges[id] = c
		}
	}
	return nil
}
func (m *MemoryStore) Consume(_ context.Context, app, email, id string, now time.Time, fn func(Challenge) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.challenges[id]
	if !ok || c.AppID != app || c.Email != email || c.ConsumedAt != nil || c.InvalidatedAt != nil || !now.Before(c.ExpiresAt) || c.Attempts >= c.MaxAttempts {
		return ErrInvalid
	}
	if err := fn(c); err != nil {
		c.Attempts++
		m.challenges[id] = c
		return ErrInvalid
	}
	c.ConsumedAt = &now
	m.challenges[id] = c
	return nil
}
