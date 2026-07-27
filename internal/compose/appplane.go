package compose

import (
	"github.com/tinyhost/tiny/internal/appapi"
	"github.com/tinyhost/tiny/internal/appauth"
	"github.com/tinyhost/tiny/internal/apps"
	"github.com/tinyhost/tiny/internal/config"
	"github.com/tinyhost/tiny/internal/gateway"
	"github.com/tinyhost/tiny/internal/kv"
	"github.com/tinyhost/tiny/internal/live"
	"github.com/tinyhost/tiny/internal/policies"
	"net/http"
)

// AppPlane composes the only app ingress; it never exposes a second handler.
func AppPlane(cfg config.Config, appsRepo apps.Repository, sessions SessionIssuerValidatorRevoker, policy policies.Store, repo kv.Repository, hub *live.Hub, login Login) http.Handler {
	return AppPlaneWithPlatform(cfg, appsRepo, sessions, policy, repo, hub, login, nil)
}
func AppPlaneWithPlatform(cfg config.Config, appsRepo apps.Repository, sessions SessionIssuerValidatorRevoker, policy policies.Store, repo kv.Repository, hub *live.Hub, login Login, platform http.Handler) http.Handler {
	service := kv.NewWithRepository(kv.DefaultLimits(), hub, repo)
	d := NewAppDispatcher(appapi.Dispatcher{KV: service})
	realtime := cfg.EffectiveRealtimeLimits()
	d.Live = &live.WebSocketAdapter{Hub: hub, Origin: live.SameOrigin, IdleTimeout: realtime.IdleTimeout, PingInterval: realtime.PingInterval, PongTimeout: realtime.PongTimeout, WriteTimeout: realtime.WriteTimeout, OutboundSize: realtime.OutboundQueue}
	return gateway.Gateway{Config: cfg, Apps: appsRepo, Authorizer: appauth.Authorizer{Sessions: sessions, Policies: policy}, Protected: d, PreAuth: login, Platform: platform}
}
