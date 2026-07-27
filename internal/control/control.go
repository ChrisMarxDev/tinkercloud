// Package control contains transport-neutral deployer authorization and command
// orchestration. Persistence implementations must enforce target ownership too.
package control

import (
	"context"
	"errors"
	"github.com/tinyhost/tiny/internal/audit"
	"github.com/tinyhost/tiny/internal/tokens"
	"time"
)

var (
	ErrDenied   = errors.New("control action denied")
	ErrConflict = errors.New("idempotency conflict")
	ErrAudit    = errors.New("audit failed")
)

type Actor struct {
	ID     string
	Active bool
	Token  tokens.Record
}
type App struct {
	ID, OwnerID, Slug string
	Active            bool
}
type Repository interface {
	FindOwnedApp(ctx context.Context, actorID, appID string) (App, error)
	RunInTransaction(ctx context.Context, fn func(context.Context) error) error
}
type Service struct {
	Repo  Repository
	Audit audit.Recorder
	Now   func() time.Time
}

func (s Service) Authorize(ctx context.Context, a Actor, scope tokens.Scope, appID string) (App, error) {
	if s.Repo == nil || !a.Active || a.ID == "" {
		return App{}, ErrDenied
	}
	now := time.Now()
	if s.Now != nil {
		now = s.Now()
	}
	// The caller has authenticated the raw credential against this current record;
	// this check keeps revocation/expiry enforcement at every command boundary.
	if a.Token.RevokedAt != nil || !now.Before(a.Token.ExpiresAt) {
		return App{}, ErrDenied
	}
	if err := a.Token.Allows(scope, appID); err != nil {
		return App{}, ErrDenied
	}
	app, err := s.Repo.FindOwnedApp(ctx, a.ID, appID)
	if err != nil || !app.Active {
		return App{}, ErrDenied
	}
	return app, nil
}

// Mutate performs an owned mutation and its audit append atomically where the
// repository adapter provides a real transaction boundary.
func (s Service) Mutate(ctx context.Context, a Actor, scope tokens.Scope, appID, action, requestID string, fn func(context.Context, App) error) error {
	app, err := s.Authorize(ctx, a, scope, appID)
	if err != nil {
		return err
	}
	if s.Audit == nil {
		return ErrAudit
	}
	return s.Repo.RunInTransaction(ctx, func(tx context.Context) error {
		if err := fn(tx, app); err != nil {
			return err
		}
		if err := s.Audit.Append(tx, audit.Event{ActorKind: "deployer", ActorID: a.ID, AppID: app.ID, Action: action, Outcome: "succeeded", RequestID: requestID, OccurredAt: s.now()}); err != nil {
			return ErrAudit
		}
		return nil
	})
}
func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
