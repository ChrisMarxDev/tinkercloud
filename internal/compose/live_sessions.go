package compose

import (
	"context"
	"github.com/tinyhost/tiny/internal/live"
	"github.com/tinyhost/tiny/internal/sessions"
	"time"
)

type LiveSessions struct {
	Sessions SessionValidatorRevoker
	Hub      *live.Hub
}

func (l LiveSessions) Validate(ctx context.Context, a, t string, n time.Time) (sessions.Session, error) {
	return l.Sessions.Validate(ctx, a, t, n)
}
func (l LiveSessions) Revoke(ctx context.Context, a, t string) (string, error) {
	id, e := l.Sessions.Revoke(ctx, a, t)
	if e == nil && l.Hub != nil {
		l.Hub.Revoke(a, id)
	}
	return id, e
}
