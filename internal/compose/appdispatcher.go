// Package compose wires protected capability services to the gateway. It has no
// listener of its own: the gateway remains the sole public ingress.
package compose

import (
	"net/http"

	"github.com/ChrisMarxDev/tinkercloud/internal/appapi"
	"github.com/ChrisMarxDev/tinkercloud/internal/appauth"
	"github.com/ChrisMarxDev/tinkercloud/internal/gateway"
	"github.com/ChrisMarxDev/tinkercloud/internal/live"
)

type AppDispatcher struct {
	API  appapi.Dispatcher
	Live *live.WebSocketAdapter
}

func (d AppDispatcher) Dispatch(auth appauth.AuthorizationContext, endpoint gateway.Endpoint, w http.ResponseWriter, r *http.Request) {
	switch endpoint {
	case gateway.CurrentUser, gateway.AppInfo, gateway.Capabilities, gateway.KV, gateway.Collections, gateway.Blobs, gateway.LLMChat:
		d.API.Dispatch(auth, w, r)
	case gateway.Live:
		if !auth.RealtimeEnabled() {
			appapi.CapabilityUnavailable(w, auth.RequestID())
			return
		}
		if d.Live == nil {
			http.NotFound(w, r)
			return
		}
		d.Live.Upgrade(r.Context(), auth, w, r)
	default:
		http.NotFound(w, r) // live stays deliberately unimplemented; no upgrade occurs.
	}
}

func NewAppDispatcher(api appapi.Dispatcher) AppDispatcher {
	if api.AppSlug == nil {
		api.AppSlug = func(auth appauth.AuthorizationContext) string { return auth.AppSlug() }
	}
	if api.Capabilities == nil {
		api.Capabilities = []appapi.Capability{{Name: "user", Version: 1}, {Name: "kv", Version: 1, Limits: map[string]int{"value_bytes": 65536, "keys_per_app": 10000}}, {Name: "live", Version: 1, Limits: map[string]int{"connections_per_app": 100, "subscriptions_per_connection": 32}}}
		if api.Collections != nil {
			api.Capabilities = append(api.Capabilities, appapi.Capability{Name: "db", Version: 1, Limits: map[string]int{"document_bytes": 65536, "documents_per_collection": 10000, "list_limit": 100, "snapshot_limit": 1000}})
		}
		if api.Blobs != nil {
			api.Capabilities = append(api.Capabilities, appapi.Capability{Name: "blobs", Version: 1, Limits: map[string]int{"blob_bytes": 25000000, "blobs_per_app": 1000, "total_bytes_per_app": 250000000, "list_limit": 100}})
		}
	}
	return AppDispatcher{API: api}
}
