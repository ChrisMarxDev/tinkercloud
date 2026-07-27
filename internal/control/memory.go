package control

import (
	"context"
	"errors"
	"github.com/tinyhost/tiny/internal/audit"
	"sort"
	"strings"
	"sync"
)

type Memory struct {
	mu       sync.Mutex
	Users    map[string]bool
	Apps     map[string]App
	Policies map[string]Policy
	Replay   map[string]string
	Audit    audit.Recorder
}

func (m *Memory) AuthorizeDeployer(_ context.Context, email string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if email == "" {
		return ErrDenied
	}
	if m.Users == nil {
		m.Users = map[string]bool{}
	}
	m.Users[email] = true
	return nil
}
func (m *Memory) SuspendDeployer(_ context.Context, email string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Users == nil || !m.Users[email] {
		return ErrDenied
	}
	m.Users[email] = false
	return nil
}
func (m *Memory) Owned(actor, app string) (App, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.Apps[app]
	if !ok || a.OwnerID != actor || !a.Active {
		return App{}, ErrDenied
	}
	return a, nil
}

type Policy struct {
	Revision        int
	Emails, Domains []string
}

func (m *Memory) ReplaceAccessPolicy(ctx context.Context, actor, app string, next Policy, key, requestID string) (Policy, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if key == "" || !m.Users[actor] {
		return Policy{}, ErrDenied
	}
	a, ok := m.Apps[app]
	if !ok || !a.Active || a.OwnerID != actor {
		return Policy{}, ErrDenied
	}
	norm := func(v []string) []string {
		out := append([]string(nil), v...)
		for i := range out {
			out[i] = strings.ToLower(strings.TrimSpace(out[i]))
		}
		sort.Strings(out)
		return out
	}
	next.Emails, next.Domains = norm(next.Emails), norm(next.Domains)
	payload := strings.Join(next.Emails, ",") + "|" + strings.Join(next.Domains, ",")
	if m.Replay == nil {
		m.Replay = map[string]string{}
	}
	rk := actor + "\000" + key
	if old, ok := m.Replay[rk]; ok {
		if old != payload {
			return Policy{}, ErrConflict
		}
		return m.Policies[app], nil
	}
	prior := m.Policies[app]
	next.Revision = prior.Revision + 1
	if m.Audit == nil {
		return Policy{}, ErrAudit
	}
	if e := m.Audit.Append(ctx, audit.Event{ActorKind: "deployer", ActorID: actor, AppID: app, Action: "policy.replaced", Outcome: "succeeded", RequestID: requestID}); e != nil {
		return Policy{}, ErrAudit
	}
	if m.Policies == nil {
		m.Policies = map[string]Policy{}
	}
	m.Policies[app] = next
	m.Replay[rk] = payload
	return next, nil
}

var ErrViewerConfusion = errors.New("viewer credentials cannot control")
