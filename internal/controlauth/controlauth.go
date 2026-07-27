// Package controlauth keeps platform authority separate from viewer sessions.
package controlauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"github.com/tinyhost/tiny/internal/controlapi"
	"github.com/tinyhost/tiny/internal/identity"
	"github.com/tinyhost/tiny/internal/otp"
	"net/http"
	"strings"
	"sync"
	"time"
)

const CookieName = "__Host-tiny_control"

var ErrDenied = errors.New("control authority denied")

type User struct {
	ID, Email string
	Active    bool
	Role      string
}
type Credential struct {
	UserID  string
	Hash    [32]byte
	Expires time.Time
	Revoked bool
	Scopes  map[string]struct{}
}
type Store struct {
	mu          sync.RWMutex
	Users       map[string]User
	Credentials map[[32]byte]Credential
}
type Outbox interface {
	EnqueueOTP(context.Context, otp.Message) error
}

// OTPService is purpose-separated from app login. Callers always return a
// generic accepted response; only active authorized users receive mail.
type OTPService struct {
	Users      *Store
	Challenges otp.Service
	Outbox     Outbox
}

func (s OTPService) Request(ctx context.Context, email string) error {
	normalized, e := identity.Normalize(email)
	if e != nil {
		return nil
	}
	s.Users.mu.RLock()
	eligible := false
	for _, u := range s.Users.Users {
		if u.Active && u.Email == normalized && (u.Role == "operator" || u.Role == "deployer") {
			eligible = true
			break
		}
	}
	s.Users.mu.RUnlock()
	return s.Challenges.Request(ctx, "control", normalized, eligible)
}
func (s OTPService) Verify(ctx context.Context, email, transaction, code string, issue func(User) error) error {
	normalized, e := identity.Normalize(email)
	if e != nil {
		return ErrDenied
	}
	return s.Challenges.Verify(ctx, "control", normalized, transaction, code, func(otp.Challenge) error {
		s.Users.mu.RLock()
		defer s.Users.mu.RUnlock()
		for _, u := range s.Users.Users {
			if u.Active && u.Email == normalized && (u.Role == "operator" || u.Role == "deployer") {
				return issue(u)
			}
		}
		return ErrDenied
	})
}

func NewStore() *Store {
	return &Store{Users: map[string]User{}, Credentials: map[[32]byte]Credential{}}
}
func (s *Store) Put(u User) { s.mu.Lock(); defer s.mu.Unlock(); s.Users[u.ID] = u }
func (s *Store) Issue(userID string, expiry time.Time, scopes ...string) (string, error) {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	raw := "tinyctl_" + base64.RawURLEncoding.EncodeToString(b)
	c := Credential{UserID: userID, Hash: sha256.Sum256([]byte(raw)), Expires: expiry, Scopes: map[string]struct{}{}}
	for _, x := range scopes {
		c.Scopes[x] = struct{}{}
	}
	s.mu.Lock()
	s.Credentials[c.Hash] = c
	s.mu.Unlock()
	return raw, nil
}
func (s *Store) AuthenticateControl(_ context.Context, r *http.Request) (controlapi.Actor, error) {
	raw := ""
	if c, e := r.Cookie(CookieName); e == nil {
		raw = c.Value
	}
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		raw = strings.TrimPrefix(h, "Bearer ")
	}
	if raw == "" {
		return controlapi.Actor{}, ErrDenied
	}
	hash := sha256.Sum256([]byte(raw))
	s.mu.RLock()
	c, ok := s.Credentials[hash]
	u, exists := s.Users[c.UserID]
	s.mu.RUnlock()
	if !ok || !exists || c.Revoked || !time.Now().Before(c.Expires) || subtle.ConstantTimeCompare(c.Hash[:], hash[:]) != 1 || !u.Active {
		return controlapi.Actor{}, ErrDenied
	}
	return controlapi.Actor{ID: u.ID, Email: u.Email, Active: true}, nil
}
func NormalizeAuthorized(raw string, users []User) (User, bool) {
	e, err := identity.Normalize(raw)
	if err != nil {
		return User{}, false
	}
	for _, u := range users {
		if u.Active && u.Email == e {
			return u, true
		}
	}
	return User{}, false
}
func ControlCookie(token string, expires time.Time) *http.Cookie {
	return &http.Cookie{Name: CookieName, Value: token, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, Expires: expires}
}
