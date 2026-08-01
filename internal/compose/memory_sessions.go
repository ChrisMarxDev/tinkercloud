package compose

import (
	"context"
	"github.com/ChrisMarxDev/tinkercloud/internal/identity"
	"github.com/ChrisMarxDev/tinkercloud/internal/sessions"
	"time"
)

// MemorySessions adapts the test-only in-memory session store to the
// context-aware production composition contract.
type MemorySessions struct{ Store *sessions.MemoryStore }

func (m MemorySessions) Validate(ctx context.Context, app, token string, now time.Time) (sessions.Session, error) {
	return m.Store.Validate(ctx, app, token, now)
}
func (m MemorySessions) Create(ctx context.Context, app string, id identity.Identity, expiry time.Time) (string, sessions.Session, error) {
	if err := ctx.Err(); err != nil {
		return "", sessions.Session{}, err
	}
	return m.Store.Create(app, id, expiry)
}
func (m MemorySessions) Revoke(ctx context.Context, app, raw string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	v, e := m.Store.Validate(ctx, app, raw, time.Now())
	if e != nil {
		return "", e
	}
	m.Store.Revoke(raw)
	return v.ID, nil
}
