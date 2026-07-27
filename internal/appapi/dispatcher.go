// Package appapi provides protected HTTP capability dispatching. It deliberately
// has no public http.Handler root: the gateway authorizes first, then invokes
// Dispatch with its sealed authorization context.
package appapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/tinyhost/tiny/internal/appauth"
	"github.com/tinyhost/tiny/internal/kv"
)

const apiPrefix = "/_tiny/api/v1"

const supportedSDKMajor = "0"

type KV interface {
	Get(rctx context.Context, auth appauth.AuthorizationContext, key string) (*kv.Entry, error)
	Set(rctx context.Context, auth appauth.AuthorizationContext, key string, value json.RawMessage, expected *uint64) (kv.Entry, error)
	Delete(rctx context.Context, auth appauth.AuthorizationContext, key string, expected *uint64) (bool, error)
	List(rctx context.Context, auth appauth.AuthorizationContext, prefix, cursor string, limit int) (kv.ListResult, error)
}
type Dispatcher struct {
	KV           KV
	Capabilities []Capability
	AppSlug      func(appauth.AuthorizationContext) string
}
type Capability struct {
	Name    string         `json:"name"`
	Version int            `json:"version"`
	Limits  map[string]int `json:"limits,omitempty"`
}

// Dispatch is the gateway registration seam. It rejects malformed and unknown
// routes without attempting any capability operation.
func (d Dispatcher) Dispatch(auth appauth.AuthorizationContext, w http.ResponseWriter, r *http.Request) {
	if auth == nil {
		writeError(w, http.StatusUnauthorized, "not_authenticated", "This request is not authenticated.", "")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	if !compatibleSDKVersion(r.Header.Get("X-Tiny-SDK-Version")) {
		writeError(w, http.StatusUpgradeRequired, "sdk_version_incompatible", "Update @tinyhost/sdk to a supported version and retry.", auth.RequestID())
		return
	}
	path := r.URL.EscapedPath()
	switch {
	case path == apiPrefix+"/me" && r.Method == http.MethodGet:
		d.me(auth, w)
	case path == apiPrefix+"/app" && r.Method == http.MethodGet:
		d.app(auth, w)
	case path == apiPrefix+"/capabilities" && r.Method == http.MethodGet:
		caps := []Capability{}
		for _, c := range d.Capabilities {
			if c.Name == "kv" && !auth.KVEnabled() {
				continue
			}
			if c.Name == "live" && !auth.RealtimeEnabled() {
				continue
			}
			caps = append(caps, c)
		}
		writeJSON(w, http.StatusOK, map[string]any{"capabilities": caps})
	case path == apiPrefix+"/kv" && r.Method == http.MethodGet:
		d.list(auth, w, r)
	case strings.HasPrefix(path, apiPrefix+"/kv/"):
		d.key(auth, w, r)
	default:
		writeError(w, http.StatusNotFound, "not_found", "This resource is not available.", auth.RequestID())
	}
}

// compatibleSDKVersion keeps raw HTTP and previously shipped clients working
// when they do not send a version header. A supplied header must be complete
// semver and use the currently supported pre-1.0 major.
func compatibleSDKVersion(version string) bool {
	if version == "" {
		return true
	}
	parts := strings.Split(version, ".")
	if len(parts) != 3 || parts[0] != supportedSDKMajor {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}
func (d Dispatcher) app(auth appauth.AuthorizationContext, w http.ResponseWriter) {
	if d.AppSlug == nil || d.AppSlug(auth) == "" {
		writeError(w, 503, "temporarily_unavailable", "TinyHost is temporarily unavailable.", auth.RequestID())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"slug": d.AppSlug(auth), "features": map[string]bool{"kv": auth.KVEnabled(), "realtime": auth.RealtimeEnabled()}})
}
func (d Dispatcher) me(auth appauth.AuthorizationContext, w http.ResponseWriter) {
	if d.AppSlug == nil || d.AppSlug(auth) == "" {
		writeError(w, 503, "temporarily_unavailable", "TinyHost is temporarily unavailable.", auth.RequestID())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"identity": auth.Identity(), "app": map[string]string{"slug": d.AppSlug(auth)}})
}

func (d Dispatcher) key(auth appauth.AuthorizationContext, w http.ResponseWriter, r *http.Request) {
	if !auth.KVEnabled() {
		writeError(w, http.StatusForbidden, "capability_unavailable", "This capability is unavailable.", auth.RequestID())
		return
	}
	if d.KV == nil {
		writeError(w, 503, "temporarily_unavailable", "TinyHost is temporarily unavailable.", auth.RequestID())
		return
	}
	raw := strings.TrimPrefix(r.URL.EscapedPath(), apiPrefix+"/kv/")
	key, err := url.PathUnescape(raw)
	if err != nil || key == "" {
		writeError(w, 400, "validation_failed", "The request is invalid.", auth.RequestID())
		return
	}
	switch r.Method {
	case http.MethodGet:
		if r.URL.RawQuery != "" {
			writeError(w, 400, "validation_failed", "The request is invalid.", auth.RequestID())
			return
		}
		entry, err := d.KV.Get(r.Context(), auth, key)
		if err != nil {
			d.err(w, auth, err)
			return
		}
		if entry == nil {
			writeError(w, 404, "not_found", "This resource is not available.", auth.RequestID())
			return
		}
		writeJSON(w, 200, entry)
	case http.MethodPut:
		if !sameOrigin(r) {
			writeError(w, 403, "not_authorized", "This request is not authorized.", auth.RequestID())
			return
		}
		var body struct {
			Value           json.RawMessage `json:"value"`
			ExpectedVersion *uint64         `json:"expected_version"`
		}
		if !decode(w, r, auth, &body) {
			return
		}
		entry, err := d.KV.Set(r.Context(), auth, key, body.Value, body.ExpectedVersion)
		if err != nil {
			d.err(w, auth, err)
			return
		}
		writeJSON(w, 200, entry)
	case http.MethodDelete:
		if !sameOrigin(r) {
			writeError(w, 403, "not_authorized", "This request is not authorized.", auth.RequestID())
			return
		}
		var body struct {
			ExpectedVersion *uint64 `json:"expected_version"`
		}
		if !decode(w, r, auth, &body) {
			return
		}
		deleted, err := d.KV.Delete(r.Context(), auth, key, body.ExpectedVersion)
		if err != nil {
			d.err(w, auth, err)
			return
		}
		writeJSON(w, 200, map[string]bool{"deleted": deleted})
	default:
		w.Header().Set("Allow", "GET, PUT, DELETE")
		writeError(w, 405, "validation_failed", "The request is invalid.", auth.RequestID())
	}
}
func sameOrigin(r *http.Request) bool {
	return r.Header.Get("Origin") == "http://"+r.Host || r.Header.Get("Origin") == "https://"+r.Host
}
func (d Dispatcher) list(auth appauth.AuthorizationContext, w http.ResponseWriter, r *http.Request) {
	if !auth.KVEnabled() {
		writeError(w, http.StatusForbidden, "capability_unavailable", "This capability is unavailable.", auth.RequestID())
		return
	}
	if d.KV == nil {
		d.err(w, auth, errors.New("unavailable"))
		return
	}
	q := r.URL.Query()
	for key, values := range q {
		if (key != "prefix" && key != "cursor" && key != "limit") || len(values) != 1 {
			writeError(w, 400, "validation_failed", "The request is invalid.", auth.RequestID())
			return
		}
	}
	limit := 100
	if q.Get("limit") != "" {
		n, err := strconv.Atoi(q.Get("limit"))
		if err != nil {
			writeError(w, 400, "validation_failed", "The request is invalid.", auth.RequestID())
			return
		}
		limit = n
	}
	result, err := d.KV.List(r.Context(), auth, q.Get("prefix"), q.Get("cursor"), limit)
	if err != nil {
		d.err(w, auth, err)
		return
	}
	// Keep the wire contract stable even when a KV implementation returns its
	// Go zero value for an empty result.
	entries := result.Entries
	if entries == nil {
		entries = []kv.Entry{}
	}
	writeJSON(w, 200, map[string]any{"entries": entries, "next_cursor": result.NextCursor})
}
func decode(w http.ResponseWriter, r *http.Request, auth appauth.AuthorizationContext, dst any) bool {
	if r.Header.Get("Content-Type") != "application/json" {
		writeError(w, 415, "validation_failed", "The request is invalid.", auth.RequestID())
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 66<<10)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, 400, "validation_failed", "The request is invalid.", auth.RequestID())
		return false
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		writeError(w, 400, "validation_failed", "The request is invalid.", auth.RequestID())
		return false
	}
	return true
}
func (d Dispatcher) err(w http.ResponseWriter, a appauth.AuthorizationContext, err error) {
	switch {
	case errors.Is(err, kv.ErrVersionConflict):
		writeError(w, 409, "version_conflict", "The value changed. Read it and try again.", a.RequestID())
	case errors.Is(err, kv.ErrQuotaExceeded):
		writeError(w, 429, "quota_exceeded", "The app storage limit was reached.", a.RequestID())
	case errors.Is(err, kv.ErrInvalidKey), errors.Is(err, kv.ErrInvalidValue), errors.Is(err, kv.ErrInvalidListLimit):
		writeError(w, 400, "validation_failed", "The request is invalid.", a.RequestID())
	case errors.Is(err, kv.ErrCapabilityUnavailable):
		writeError(w, http.StatusForbidden, "capability_unavailable", "This capability is unavailable.", a.RequestID())
	default:
		writeError(w, 503, "temporarily_unavailable", "TinyHost is temporarily unavailable.", a.RequestID())
	}
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, status int, code, message, requestID string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message, "request_id": requestID}})
}
func CapabilityUnavailable(w http.ResponseWriter, requestID string) {
	writeError(w, http.StatusForbidden, "capability_unavailable", "This capability is unavailable.", requestID)
}
