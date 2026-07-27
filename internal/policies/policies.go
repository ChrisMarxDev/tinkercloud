package policies

import (
	"context"
	"errors"
	"github.com/tinyhost/tiny/internal/identity"
)

type Policy struct {
	AppID, OwnerIdentityID string
	Revision               uint64
	Emails, Domains        map[string]struct{}
	Valid                  bool
}
type Store interface {
	Current(context.Context, string) (Policy, error)
}

var ErrUnavailable = errors.New("policy unavailable")

type Decision bool

const (
	Deny  Decision = false
	Allow Decision = true
)

func Evaluate(p Policy, viewer identity.Identity) Decision {
	if !p.Valid || viewer.ID == "" {
		return Deny
	}
	if viewer.ID == p.OwnerIdentityID {
		return Allow
	}
	if _, ok := p.Emails[viewer.Email]; ok {
		return Allow
	}
	_, ok := p.Domains[identity.Domain(viewer.Email)]
	return Decision(ok)
}

type MemoryStore struct {
	Policies map[string]Policy
	Err      error
}

func (s *MemoryStore) Current(_ context.Context, appID string) (Policy, error) {
	if s.Err != nil {
		return Policy{}, s.Err
	}
	p, ok := s.Policies[appID]
	if !ok {
		return Policy{}, ErrUnavailable
	}
	return p, nil
}
