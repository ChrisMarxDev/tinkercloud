// Package analytics records the deliberately small, local app-insights signal.
// It never receives an HTTP request, identity, URL, or raw browser marker.
package analytics

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

const (
	CookieName       = "__Host-tinker-insights"
	markerBytes      = 32
	MarkerLifetime   = 30 * 24 * time.Hour
	DefaultQueueSize = 256
)

// Event is intentionally limited to server-derived app identity, a keyed
// marker digest, and an occurrence time. It must not grow request metadata.
type Event struct {
	AppID     string
	Marker    [sha256.Size]byte
	HasMarker bool
	Occurred  time.Time
}

// Sink performs persistence outside the public request path.
type Sink interface {
	RecordInsight(context.Context, Event) error
}

// Recorder has one fixed queue. Offer is non-blocking and starts no goroutine;
// the composition root owns the one worker by calling Start.
type Recorder struct {
	sink    Sink
	queue   chan Event
	running atomic.Bool
	enabled atomic.Bool
	once    sync.Once
}

func NewRecorder(sink Sink, capacity int) *Recorder {
	if capacity < 1 {
		capacity = DefaultQueueSize
	}
	r := &Recorder{sink: sink, queue: make(chan Event, capacity)}
	r.enabled.Store(true)
	return r
}

func (r *Recorder) Start(ctx context.Context) {
	if r == nil || r.sink == nil {
		return
	}
	r.once.Do(func() {
		r.running.Store(true)
		go func() {
			defer r.running.Store(false)
			for {
				select {
				case <-ctx.Done():
					return
				case event := <-r.queue:
					// A write failure is an intentional drop. Retrying here could
					// make a failed database a source of unbounded retained work.
					_ = r.sink.RecordInsight(ctx, event)
				}
			}
		}()
	})
}

// SetEnabled is the non-blocking gateway gate, synchronized by composition
// with the durable operator setting before readiness and after any mutation.
func (r *Recorder) SetEnabled(enabled bool) {
	if r != nil {
		r.enabled.Store(enabled)
	}
}

func (r *Recorder) Available() bool { return r != nil && r.running.Load() && r.enabled.Load() }

// Offer never waits for SQLite or a worker. A stopped, full, or unavailable
// recorder drops the event and cannot influence the app response.
func (r *Recorder) Offer(event Event) bool {
	if !r.Available() || event.AppID == "" || event.Occurred.IsZero() {
		return false
	}
	select {
	case r.queue <- event:
		return true
	default:
		return false
	}
}

// ReadMarker accepts only the exact fixed-size, URL-safe value issued here.
func ReadMarker(r *http.Request) ([]byte, bool) {
	if r == nil {
		return nil, false
	}
	c, err := r.Cookie(CookieName)
	if err != nil || len(c.Value) != base64.RawURLEncoding.EncodedLen(markerBytes) {
		return nil, false
	}
	b, err := base64.RawURLEncoding.DecodeString(c.Value)
	if err != nil || len(b) != markerBytes {
		return nil, false
	}
	return b, true
}

func NewMarkerCookie(now time.Time) (*http.Cookie, error) {
	b := make([]byte, markerBytes)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return &http.Cookie{Name: CookieName, Value: base64.RawURLEncoding.EncodeToString(b), Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, Expires: now.UTC().Add(MarkerLifetime), MaxAge: int(MarkerLifetime.Seconds())}, nil
}

// Digest scopes a marker to an app before it leaves gateway memory.
func Digest(key []byte, appID string, marker []byte) [sha256.Size]byte {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write([]byte("tinkercloud.local-insights.v1\x00"))
	_, _ = h.Write([]byte(appID))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write(marker)
	var out [sha256.Size]byte
	copy(out[:], h.Sum(nil))
	return out
}
