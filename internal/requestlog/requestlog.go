// Package requestlog emits privacy-safe operational JSON. It deliberately
// records route classes instead of URLs, hosts, query strings, or payloads.
package requestlog

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// Middleware captures only bounded request facts. Authentication material,
// email addresses, host names, filesystem paths, query strings, and bodies are
// intentionally absent from the log schema.
func Middleware(next http.Handler, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.New(slog.NewJSONHandler(io.Discard, nil))
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		outcome := outcome(rec.status)
		logger.LogAttrs(r.Context(), slog.LevelInfo, "request.complete",
			slog.String("request_id", safeRequestID(rec.Header().Get("X-Request-ID"))),
			slog.String("route_class", RouteClass(r.Method, r.URL.Path)),
			slog.Int("status", rec.status),
			slog.String("outcome", outcome),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
		)
	})
}

// Service logs are similarly restricted to a fixed event vocabulary and safe
// opaque IDs/counts supplied by the server. It must never receive paths,
// secrets, or user-provided strings.
func Service(logger *slog.Logger, event, outcome string, count int) {
	if logger == nil {
		return
	}
	logger.LogAttrs(context.Background(), slog.LevelInfo, "service."+safeEvent(event),
		slog.String("outcome", safeOutcome(outcome)), slog.Int("count", max(count, 0)))
}

func RouteClass(method, path string) string {
	if len(path) >= len("/.well-known/acme-challenge/") && path[:len("/.well-known/acme-challenge/")] == "/.well-known/acme-challenge/" {
		return "gateway_acme"
	}
	if path == "/_tiny/ws/v1" {
		return "app_websocket"
	}
	if len(path) >= len("/_tiny/auth/") && path[:len("/_tiny/auth/")] == "/_tiny/auth/" {
		return "app_auth"
	}
	if len(path) >= len("/_tiny/api/") && path[:len("/_tiny/api/")] == "/_tiny/api/" {
		return "app_api"
	}
	if len(path) >= len("/api/v1/") && path[:len("/api/v1/")] == "/api/v1/" {
		return "platform_api"
	}
	if path == "/login" || path == "/" {
		return "platform_ui"
	}
	if len(path) >= len("/_tiny/") && path[:len("/_tiny/")] == "/_tiny/" {
		return "app_reserved"
	}
	_ = method // method is deliberately not emitted; class is sufficient.
	return "app_static"
}

func safeRequestID(v string) string {
	if len(v) < 5 || len(v) > 80 {
		return "req_unavailable"
	}
	for _, r := range v {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_') {
			return "req_unavailable"
		}
	}
	return v
}
func safeOutcome(v string) string {
	if v == "succeeded" || v == "failed" || v == "denied" {
		return v
	}
	return "failed"
}
func safeEvent(v string) string {
	if v == "release_cleanup" {
		return v
	}
	return "unknown"
}
func outcome(status int) string {
	if status >= 500 {
		return "failed"
	}
	if status >= 400 {
		return "denied"
	}
	return "succeeded"
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

type responseRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (r *responseRecorder) WriteHeader(status int) {
	if !r.wrote {
		r.status = status
		r.wrote = true
	}
	r.ResponseWriter.WriteHeader(status)
}
func (r *responseRecorder) Write(b []byte) (int, error) {
	if !r.wrote {
		r.WriteHeader(http.StatusOK)
	}
	return r.ResponseWriter.Write(b)
}
func (r *responseRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
func (r *responseRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	return h.Hijack()
}
func (r *responseRecorder) ReadFrom(src io.Reader) (int64, error) {
	if rf, ok := r.ResponseWriter.(io.ReaderFrom); ok {
		if !r.wrote {
			r.WriteHeader(http.StatusOK)
		}
		return rf.ReadFrom(src)
	}
	return io.Copy(r.ResponseWriter, src)
}
func (r *responseRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }
