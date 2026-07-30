// Package tokens handles opaque deployer credentials. Raw token values are
// accepted only at issuance/authentication edges and never persisted or audited.
package tokens

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalid = errors.New("invalid token")
	ErrRevoked = errors.New("token revoked")
	ErrExpired = errors.New("token expired")
	ErrScope   = errors.New("token scope denied")
)

type Scope string

const (
	ScopeAppRead        Scope = "app:read"
	ScopeAppCreate      Scope = "app:create"
	ScopeTokenCreate    Scope = "token:create"
	ScopeTokenRevoke    Scope = "token:revoke"
	ScopeDeployCreate   Scope = "deploy:create"
	ScopeDeployActivate Scope = "deploy:activate"
	ScopeAccessRead     Scope = "access:read"
	ScopeAccessWrite    Scope = "access:write"
)

type Record struct {
	ID, UserID, AppID string
	Hash              [32]byte
	Scopes            map[Scope]struct{}
	ExpiresAt         time.Time
	RevokedAt         *time.Time
}

func Issue(id, userID, appID string, scopes []Scope, expires time.Time) (Record, string, error) {
	if id == "" || userID == "" || expires.IsZero() {
		return Record{}, "", ErrInvalid
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return Record{}, "", err
	}
	raw := "tinker_" + base64.RawURLEncoding.EncodeToString(b)
	r := Record{ID: id, UserID: userID, AppID: appID, Hash: sha256.Sum256([]byte(raw)), Scopes: map[Scope]struct{}{}, ExpiresAt: expires.UTC()}
	for _, s := range scopes {
		if s == "" {
			return Record{}, "", ErrInvalid
		}
		r.Scopes[s] = struct{}{}
	}
	return r, raw, nil
}
func (r Record) Authenticate(raw string, now time.Time) error {
	h := sha256.Sum256([]byte(raw))
	if !strings.HasPrefix(raw, "tinker_") || subtle.ConstantTimeCompare(r.Hash[:], h[:]) != 1 {
		return ErrInvalid
	}
	if r.RevokedAt != nil {
		return ErrRevoked
	}
	if !now.Before(r.ExpiresAt) {
		return ErrExpired
	}
	return nil
}
func (r Record) Allows(scope Scope, appID string) error {
	if _, ok := r.Scopes[scope]; !ok {
		return ErrScope
	}
	if r.AppID != "" && r.AppID != appID {
		return ErrScope
	}
	return nil
}
