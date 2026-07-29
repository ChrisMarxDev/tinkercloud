// Package appapi provides protected HTTP capability dispatching. It deliberately
// has no public http.Handler root: the gateway authorizes first, then invokes
// Dispatch with its sealed authorization context.
package appapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/tinyhost/tiny/internal/appauth"
	"github.com/tinyhost/tiny/internal/blob"
	"github.com/tinyhost/tiny/internal/compatibility"
	"github.com/tinyhost/tiny/internal/kv"
)

const apiPrefix = "/_tiny/api/v1"

type KV interface {
	Get(rctx context.Context, auth appauth.AuthorizationContext, key string) (*kv.Entry, error)
	Set(rctx context.Context, auth appauth.AuthorizationContext, key string, value json.RawMessage, expected *uint64) (kv.Entry, error)
	Delete(rctx context.Context, auth appauth.AuthorizationContext, key string, expected *uint64) (bool, error)
	List(rctx context.Context, auth appauth.AuthorizationContext, prefix, cursor string, limit int) (kv.ListResult, error)
}
type Blobs interface {
	Upload(context.Context, appauth.AuthorizationContext, string, string, io.Reader) (blob.Metadata, error)
	Get(context.Context, appauth.AuthorizationContext, string) (io.ReadCloser, blob.Metadata, error)
	List(context.Context, appauth.AuthorizationContext, string, int) (blob.ListResult, error)
	Delete(context.Context, appauth.AuthorizationContext, string) (bool, error)
}
type Dispatcher struct {
	KV           KV
	Blobs        Blobs
	BlobMaxBytes int64
	Capabilities []Capability
	AppSlug      func(appauth.AuthorizationContext) string
	// Origin is injected by composition. Production uses exact HTTPS origin;
	// in-memory HTTP harnesses must opt in explicitly rather than weakening it.
	Origin func(*http.Request) bool
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
	if !compatibleSDKVersion(r.Header.Get("X-Tiny-SDK-Version")) ||
		!compatibleAPIVersion(r.Header.Get("X-Tiny-App-API-Version")) {
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
			if c.Name == "blobs" && !auth.BlobsEnabled() {
				continue
			}
			caps = append(caps, c)
		}
		writeJSON(w, http.StatusOK, map[string]any{"capabilities": caps})
	case path == apiPrefix+"/kv" && r.Method == http.MethodGet:
		d.list(auth, w, r)
	case strings.HasPrefix(path, apiPrefix+"/kv/"):
		d.key(auth, w, r)
	case path == apiPrefix+"/blobs":
		d.blobListOrUpload(auth, w, r)
	case strings.HasPrefix(path, apiPrefix+"/blobs/"):
		d.blobObject(auth, w, r)
	default:
		writeError(w, http.StatusNotFound, "not_found", "This resource is not available.", auth.RequestID())
	}
}

// compatibleSDKVersion keeps raw HTTP and previously shipped clients working
// when they do not send a version header. A supplied header must be complete
// semver and fall inside the release compatibility range.
func compatibleSDKVersion(version string) bool {
	if version == "" {
		return true
	}
	return compatibility.Current("0.1.0").AppAPI.Client.Contains(version)
}

func compatibleAPIVersion(version string) bool {
	return version == "" || version == compatibility.AppAPIVersion
}
func (d Dispatcher) app(auth appauth.AuthorizationContext, w http.ResponseWriter) {
	if d.AppSlug == nil || d.AppSlug(auth) == "" {
		writeError(w, 503, "temporarily_unavailable", "TinyHost is temporarily unavailable.", auth.RequestID())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"slug": d.AppSlug(auth), "features": map[string]bool{"kv": auth.KVEnabled(), "blobs": auth.BlobsEnabled() && d.Blobs != nil, "realtime": auth.RealtimeEnabled()}})
}

func (d Dispatcher) blobListOrUpload(auth appauth.AuthorizationContext, w http.ResponseWriter, r *http.Request) {
	if !auth.BlobsEnabled() {
		CapabilityUnavailable(w, auth.RequestID())
		return
	}
	if d.Blobs == nil {
		d.err(w, auth, blob.ErrUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query()
		for k, v := range q {
			if (k != "limit" && k != "cursor") || len(v) != 1 {
				writeError(w, 400, "validation_failed", "The request is invalid.", auth.RequestID())
				return
			}
		}
		limit := 100
		if q.Get("limit") != "" {
			n, e := strconv.Atoi(q.Get("limit"))
			if e != nil {
				writeError(w, 400, "validation_failed", "The request is invalid.", auth.RequestID())
				return
			}
			limit = n
		}
		out, e := d.Blobs.List(r.Context(), auth, q.Get("cursor"), limit)
		if e != nil {
			d.err(w, auth, e)
			return
		}
		if out.Blobs == nil {
			out.Blobs = []blob.Metadata{}
		}
		writeJSON(w, 200, map[string]any{"blobs": out.Blobs, "next_cursor": out.NextCursor})
	case http.MethodPost:
		if !d.sameOrigin(r) {
			writeError(w, 403, "not_authorized", "This request is not authorized.", auth.RequestID())
			return
		}
		max := d.BlobMaxBytes
		if max == 0 {
			max = blob.DefaultLimits().BlobBytes
		}
		// Multipart framing is bounded independently from bytes, while the file
		// limit is enforced again by the storage service.
		r.Body = http.MaxBytesReader(w, r.Body, max+(1<<20))
		mr, e := r.MultipartReader()
		if e != nil {
			writeError(w, 400, "validation_failed", "The request is invalid.", auth.RequestID())
			return
		}
		part, e := mr.NextPart()
		if e != nil || part.FormName() != "file" || part.FileName() == "" {
			writeError(w, 400, "validation_failed", "The request is invalid.", auth.RequestID())
			return
		}
		m, e := d.Blobs.Upload(r.Context(), auth, part.FileName(), part.Header.Get("Content-Type"), &strictMultipartPart{part: part, mr: mr})
		if e != nil {
			d.err(w, auth, e)
			return
		}
		writeJSON(w, http.StatusCreated, m)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, 405, "validation_failed", "The request is invalid.", auth.RequestID())
	}
}

// strictMultipartPart makes end-of-file contingent on no further parts. This
// lets the storage transition fail before ready state when a request sneaks in
// an extra form field or file; calling NextPart before consuming the file would
// discard the upload body.
type strictMultipartPart struct {
	part io.Reader
	mr   interface {
		NextPart() (*multipart.Part, error)
	}
	checked bool
}

func (p *strictMultipartPart) Read(b []byte) (int, error) {
	n, e := p.part.Read(b)
	if e != io.EOF || p.checked {
		return n, e
	}
	p.checked = true
	next, x := p.mr.NextPart()
	if x == io.EOF && next == nil {
		return n, io.EOF
	}
	if x == nil {
		return n, blob.ErrInvalidMetadata
	}
	return n, x
}
func (d Dispatcher) blobObject(auth appauth.AuthorizationContext, w http.ResponseWriter, r *http.Request) {
	if !auth.BlobsEnabled() {
		CapabilityUnavailable(w, auth.RequestID())
		return
	}
	if d.Blobs == nil {
		d.err(w, auth, blob.ErrUnavailable)
		return
	}
	id := strings.TrimPrefix(r.URL.EscapedPath(), apiPrefix+"/blobs/")
	if strings.Contains(id, "/") {
		writeError(w, 400, "validation_failed", "The request is invalid.", auth.RequestID())
		return
	}
	switch r.Method {
	case http.MethodGet:
		if r.URL.RawQuery != "" || r.Header.Get("Range") != "" {
			writeError(w, 400, "validation_failed", "The request is invalid.", auth.RequestID())
			return
		}
		rd, m, e := d.Blobs.Get(r.Context(), auth, id)
		if e != nil {
			if errors.Is(e, os.ErrNotExist) {
				writeError(w, 404, "not_found", "This resource is not available.", auth.RequestID())
			} else {
				d.err(w, auth, e)
			}
			return
		}
		defer rd.Close()
		w.Header().Set("Content-Type", m.ContentType)
		w.Header().Set("Content-Length", strconv.FormatInt(m.Size, 10))
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": m.Name}))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "private, no-store")
		w.WriteHeader(200)
		_, _ = io.Copy(w, rd)
	case http.MethodDelete:
		if !d.sameOrigin(r) {
			writeError(w, 403, "not_authorized", "This request is not authorized.", auth.RequestID())
			return
		}
		deleted, e := d.Blobs.Delete(r.Context(), auth, id)
		if e != nil {
			d.err(w, auth, e)
			return
		}
		writeJSON(w, 200, map[string]bool{"deleted": deleted})
	default:
		w.Header().Set("Allow", "GET, DELETE")
		writeError(w, 405, "validation_failed", "The request is invalid.", auth.RequestID())
	}
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
		if !d.sameOrigin(r) {
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
		if !d.sameOrigin(r) {
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
func (d Dispatcher) sameOrigin(r *http.Request) bool {
	if d.Origin != nil {
		return d.Origin(r)
	}
	return sameOrigin(r)
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
	case errors.Is(err, kv.ErrRateLimited):
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many requests. Try again shortly.", a.RequestID())
	case errors.Is(err, kv.ErrVersionConflict):
		writeError(w, 409, "version_conflict", "The value changed. Read it and try again.", a.RequestID())
	case errors.Is(err, kv.ErrQuotaExceeded):
		writeError(w, 429, "quota_exceeded", "The app storage limit was reached.", a.RequestID())
	case errors.Is(err, kv.ErrInvalidKey), errors.Is(err, kv.ErrInvalidValue), errors.Is(err, kv.ErrInvalidListLimit):
		writeError(w, 400, "validation_failed", "The request is invalid.", a.RequestID())
	case errors.Is(err, kv.ErrCapabilityUnavailable):
		writeError(w, http.StatusForbidden, "capability_unavailable", "This capability is unavailable.", a.RequestID())
	case errors.Is(err, blob.ErrCapabilityUnavailable):
		writeError(w, http.StatusForbidden, "capability_unavailable", "This capability is unavailable.", a.RequestID())
	case errors.Is(err, blob.ErrQuotaExceeded):
		writeError(w, 429, "quota_exceeded", "The app storage limit was reached.", a.RequestID())
	case errors.Is(err, blob.ErrRateLimited):
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many requests. Try again shortly.", a.RequestID())
	case errors.Is(err, blob.ErrInvalidID), errors.Is(err, blob.ErrInvalidList), errors.Is(err, blob.ErrInvalidMetadata):
		writeError(w, 400, "validation_failed", "The request is invalid.", a.RequestID())
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
