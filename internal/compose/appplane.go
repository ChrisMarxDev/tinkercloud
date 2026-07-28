package compose

import (
	"net/http"

	"github.com/tinyhost/tiny/internal/appapi"
	"github.com/tinyhost/tiny/internal/appauth"
	"github.com/tinyhost/tiny/internal/apps"
	"github.com/tinyhost/tiny/internal/blob"
	"github.com/tinyhost/tiny/internal/config"
	"github.com/tinyhost/tiny/internal/gateway"
	"github.com/tinyhost/tiny/internal/kv"
	"github.com/tinyhost/tiny/internal/live"
	"github.com/tinyhost/tiny/internal/policies"
)

// AppPlane composes the only app ingress; it never exposes a second handler.
func AppPlane(cfg config.Config, appsRepo apps.Repository, sessions SessionIssuerValidatorRevoker, policy policies.Store, repo kv.Repository, hub *live.Hub, login Login) http.Handler {
	return AppPlaneWithPlatform(cfg, appsRepo, sessions, policy, repo, hub, login, nil)
}
func AppPlaneWithPlatform(cfg config.Config, appsRepo apps.Repository, sessions SessionIssuerValidatorRevoker, policy policies.Store, repo kv.Repository, hub *live.Hub, login Login, platform http.Handler) http.Handler {
	return AppPlaneWithPlatformAndBlobs(cfg, appsRepo, sessions, policy, repo, nil, hub, login, platform)
}
func AppPlaneWithPlatformAndBlobs(cfg config.Config, appsRepo apps.Repository, sessions SessionIssuerValidatorRevoker, policy policies.Store, repo kv.Repository, blobs blob.Repository, hub *live.Hub, login Login, platform http.Handler) http.Handler {
	// The broker owns only its fixed platform-host namespace and delegates every
	// control/dashboard route to the existing platform handler. Its interface is
	// deliberately independent from the gateway's app authorization context.
	if login.IdentityBroker != nil && platform != nil {
		platform = login.IdentityBroker.PlatformHandler(platform)
	}
	service := kv.NewWithRepository(kv.DefaultLimits(), hub, repo)
	bl := cfgBlobLimits(cfg)
	caps := []appapi.Capability{
		{Name: "user", Version: 1},
		{Name: "kv", Version: 1, Limits: map[string]int{"value_bytes": 65536, "keys_per_app": 10000}},
	}
	if blobs != nil {
		caps = append(caps, appapi.Capability{Name: "blobs", Version: 1, Limits: map[string]int{"blob_bytes": int(bl.BlobBytes), "blobs_per_app": bl.BlobsPerApp, "total_bytes_per_app": int(bl.TotalBytesPerApp), "list_limit": bl.ListLimit}})
	}
	caps = append(caps, appapi.Capability{Name: "live", Version: 1, Limits: map[string]int{"connections_per_app": 100, "subscriptions_per_connection": 32}})
	origin := func(r *http.Request) bool { return r.Header.Get("Origin") == "https://"+r.Host }
	// Local unit composition intentionally has no TLS listener. It remains an
	// explicit harness mode; deployed configurations always have ListenHTTPS.
	if cfg.ListenHTTPS == "" {
		origin = func(r *http.Request) bool {
			return r.Header.Get("Origin") == "http://"+r.Host || r.Header.Get("Origin") == "https://"+r.Host
		}
	}
	d := NewAppDispatcher(appapi.Dispatcher{KV: service, Blobs: blob.New(bl, blobs), BlobMaxBytes: bl.BlobBytes, Origin: origin, Capabilities: caps})
	realtime := cfg.EffectiveRealtimeLimits()
	d.Live = &live.WebSocketAdapter{Hub: hub, Origin: live.SameOrigin, IdleTimeout: realtime.IdleTimeout, PingInterval: realtime.PingInterval, PongTimeout: realtime.PongTimeout, WriteTimeout: realtime.WriteTimeout, OutboundSize: realtime.OutboundQueue}
	return gateway.Gateway{Config: cfg, Apps: appsRepo, Authorizer: appauth.Authorizer{Sessions: sessions, Policies: policy}, Protected: d, PreAuth: login, Platform: platform}
}
func cfgBlobLimits(cfg config.Config) blob.Limits {
	l := cfg.EffectiveLimits()
	return blob.Limits{BlobBytes: l.BlobBytes, BlobsPerApp: l.BlobsPerApp, TotalBytesPerApp: l.TotalBlobBytesPerApp, ListLimit: l.BlobListLimit, UploadsPerMinute: l.BlobUploadsPerMinute, ConcurrentUploads: l.BlobConcurrentUploads, UploadDuration: l.BlobUploadDuration}
}
