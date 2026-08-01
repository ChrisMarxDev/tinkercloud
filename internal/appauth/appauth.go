package appauth

import (
	"context"
	"errors"
	"time"

	"github.com/ChrisMarxDev/tinkercloud/internal/apps"
	"github.com/ChrisMarxDev/tinkercloud/internal/identity"
	"github.com/ChrisMarxDev/tinkercloud/internal/policies"
	"github.com/ChrisMarxDev/tinkercloud/internal/releases"
	"github.com/ChrisMarxDev/tinkercloud/internal/sessions"
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

// StaticAccessContext is the only authority accepted by staticruntime.  Its
// two implementations are deliberately private to this package: a private
// viewer context and a capability-free public static context.
type StaticAccessContext interface {
	AppID() string
	DeploymentID() string
	AppSlug() string
	ReleaseRoot() string
	ReleaseEvidence() releases.FileManifest
	SPAFallback() bool
	PolicyRevision() uint64
	PublicGateRevision() uint64
	Public() bool
	Indexing() bool
	staticAuthorizedContext()
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

type privateStaticContext struct{ AuthorizationContext }

func (privateStaticContext) DeploymentID() string { return "" }

func (privateStaticContext) PublicGateRevision() uint64 { return 0 }
func (privateStaticContext) Public() bool               { return false }
func (privateStaticContext) Indexing() bool             { return false }
func (privateStaticContext) staticAuthorizedContext()   {}

// PrivateStatic wraps an already gateway-produced private context for the
// static dispatcher. It cannot manufacture viewer authority because its input
// is itself sealed.
func PrivateStatic(auth AuthorizationContext) StaticAccessContext {
	if auth == nil {
		return nil
	}
	return privateStaticContext{auth}
}

type publicStaticContext struct {
	app                          apps.App
	policyRevision, gateRevision uint64
	indexing                     bool
}

func (c publicStaticContext) AppID() string                          { return c.app.ID }
func (c publicStaticContext) DeploymentID() string                   { return c.app.DeploymentID }
func (c publicStaticContext) AppSlug() string                        { return c.app.Slug }
func (c publicStaticContext) ReleaseRoot() string                    { return c.app.ReleaseRoot }
func (c publicStaticContext) ReleaseEvidence() releases.FileManifest { return c.app.ReleaseEvidence }
func (c publicStaticContext) SPAFallback() bool                      { return c.app.SPAFallback }
func (c publicStaticContext) PolicyRevision() uint64                 { return c.policyRevision }
func (c publicStaticContext) PublicGateRevision() uint64             { return c.gateRevision }
func (c publicStaticContext) Public() bool                           { return true }
func (c publicStaticContext) Indexing() bool                         { return c.indexing }
func (c publicStaticContext) staticAuthorizedContext()               {}

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
	if err != nil || !p.Valid || p.AppID != app.ID || (p.Mode != "" && p.Mode != "private" && p.Mode != "public") {
		return nil, ErrDenied
	}
	if policies.Evaluate(p, s.Identity) != policies.Allow {
		return nil, ErrDenied
	}
	return authorizationContext{app: app, session: s, revision: p.Revision, requestID: requestID}, nil
}

// AuthorizeStatic first preserves private viewer access.  Anonymous public
// access is possible only for a current public policy, a current enabled gate,
// and a release with no browser capability.  All failures are denial.
func (a Authorizer) AuthorizeStatic(ctx context.Context, app apps.App, token, requestID string) (StaticAccessContext, error) {
	gateStore, ok := a.Policies.(policies.PublicGateStore)
	if ok && !app.KVEnabled && !app.BlobsEnabled && !app.RealtimeEnabled && !app.LLMChatRequested && app.DeploymentID != "" && app.ReleaseRoot != "" && app.ReleaseEvidence.Hash != "" {
		p, err := a.Policies.Current(ctx, app.ID)
		if err == nil && p.Valid && p.AppID == app.ID && p.Mode == "public" {
			gate, err := gateStore.CurrentPublicGate(ctx)
			if err == nil && gate.Valid && gate.Enabled && gate.Revision > 0 {
				return publicStaticContext{app: app, policyRevision: p.Revision, gateRevision: gate.Revision, indexing: app.PublicIndexing}, nil
			}
		}
	}
	if private, err := a.Authorize(ctx, app, token, requestID); err == nil {
		return privateStaticContext{private}, nil
	}
	return nil, ErrDenied
}
