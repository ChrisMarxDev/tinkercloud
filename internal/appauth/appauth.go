package appauth

import (
	"context"
	"errors"
	"time"

	"github.com/tinyhost/tiny/internal/apps"
	"github.com/tinyhost/tiny/internal/identity"
	"github.com/tinyhost/tiny/internal/policies"
	"github.com/tinyhost/tiny/internal/releases"
	"github.com/tinyhost/tiny/internal/sessions"
)

// AuthorizationContext is sealed: protected packages can consume it but cannot manufacture one.
type AuthorizationContext interface {
	AppID() string
	AppSlug() string
	ReleaseRoot() string
	ReleaseEvidence() releases.FileManifest
	SPAFallback() bool
	KVEnabled() bool
	BlobsEnabled() bool
	RealtimeEnabled() bool
	LLMChatRequested() bool
	Identity() identity.Identity
	SessionID() string
	PolicyRevision() uint64
	RequestID() string
	DataActorKind() string
	DataActorID() string
	dataAuthorizedContext()
	authorizedContext()
}

// DataAuthorizationContext is the narrower, sealed trust seam for app-local
// persistence and best-effort freshness hints. It deliberately has no viewer
// identity or session: a deployer acting through the control plane is not
// turned into a synthetic app viewer.
type DataAuthorizationContext interface {
	AppID() string
	AppSlug() string
	RequestID() string
	DataActorKind() string
	DataActorID() string
	dataAuthorizedContext()
}
type authorizationContext struct {
	app       apps.App
	session   sessions.Session
	revision  uint64
	requestID string
}

func (c authorizationContext) AppID() string       { return c.app.ID }
func (c authorizationContext) AppSlug() string     { return c.app.Slug }
func (c authorizationContext) ReleaseRoot() string { return c.app.ReleaseRoot }
func (c authorizationContext) ReleaseEvidence() releases.FileManifest {
	return c.app.ReleaseEvidence
}
func (c authorizationContext) SPAFallback() bool           { return c.app.SPAFallback }
func (c authorizationContext) KVEnabled() bool             { return c.app.KVEnabled }
func (c authorizationContext) BlobsEnabled() bool          { return c.app.BlobsEnabled }
func (c authorizationContext) RealtimeEnabled() bool       { return c.app.RealtimeEnabled }
func (c authorizationContext) LLMChatRequested() bool      { return c.app.LLMChatRequested }
func (c authorizationContext) Identity() identity.Identity { return c.session.Identity }
func (c authorizationContext) SessionID() string           { return c.session.ID }
func (c authorizationContext) PolicyRevision() uint64      { return c.revision }
func (c authorizationContext) RequestID() string           { return c.requestID }
func (c authorizationContext) authorizedContext()          {}
func (c authorizationContext) DataActorKind() string       { return "viewer" }
func (c authorizationContext) DataActorID() string         { return c.session.Identity.ID }
func (c authorizationContext) dataAuthorizedContext()      {}

type deployerDataAuthorizationContext struct {
	appID, slug, actorID, requestID string
}

func (c deployerDataAuthorizationContext) AppID() string          { return c.appID }
func (c deployerDataAuthorizationContext) AppSlug() string        { return c.slug }
func (c deployerDataAuthorizationContext) RequestID() string      { return c.requestID }
func (c deployerDataAuthorizationContext) DataActorKind() string  { return "deployer" }
func (c deployerDataAuthorizationContext) DataActorID() string    { return c.actorID }
func (c deployerDataAuthorizationContext) dataAuthorizedContext() {}

// NewDeployerDataAuthorizationContext is called only after the control plane
// has authenticated the bearer, checked its data scope, and derived ownership.
// It intentionally cannot satisfy AuthorizationContext, so it can never reach
// viewer/static/WebSocket dispatchers.
func NewDeployerDataAuthorizationContext(appID, slug, actorID, requestID string) DataAuthorizationContext {
	if appID == "" || slug == "" || actorID == "" {
		return nil
	}
	return deployerDataAuthorizationContext{appID: appID, slug: slug, actorID: actorID, requestID: requestID}
}

var ErrDenied = errors.New("app authorization denied")

type Authorizer struct {
	Sessions sessions.Store
	Policies policies.Store
	Clock    func() time.Time
}

func (a Authorizer) Authorize(ctx context.Context, app apps.App, token, requestID string) (AuthorizationContext, error) {
	if a.Sessions == nil || a.Policies == nil {
		return nil, ErrDenied
	}
	now := time.Now
	if a.Clock != nil {
		now = a.Clock
	}
	s, err := a.Sessions.Validate(ctx, app.ID, token, now())
	if err != nil {
		return nil, ErrDenied
	}
	p, err := a.Policies.Current(ctx, app.ID)
	if err != nil || !p.Valid || p.AppID != app.ID {
		return nil, ErrDenied
	}
	if policies.Evaluate(p, s.Identity) != policies.Allow {
		return nil, ErrDenied
	}
	return authorizationContext{app: app, session: s, revision: p.Revision, requestID: requestID}, nil
}
