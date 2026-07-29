package live

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/tinyhost/tiny/internal/appauth"
	"github.com/tinyhost/tiny/internal/compatibility"
)

// WebSocketAdapter is invoked only by the already-authorized gateway dispatcher.
// It owns no listener and derives no identity from client frames.
type WebSocketAdapter struct {
	Hub          *Hub
	Origin       func(*http.Request, appauth.AuthorizationContext) bool
	MaxFrame     int64
	Now          func() time.Time // injectable for deterministic rate-limit tests
	IdleTimeout  time.Duration
	PingInterval time.Duration
	PongTimeout  time.Duration
	WriteTimeout time.Duration
	OutboundSize int
}

const (
	defaultFrameBytes    = 32 << 10
	defaultPublishRate   = 20
	defaultIdleTimeout   = time.Minute
	defaultPingInterval  = 20 * time.Second
	defaultPongTimeout   = 10 * time.Second
	defaultWriteTimeout  = 3 * time.Second
	defaultOutboundQueue = 16
)

var errSlowConsumer = errors.New("live slow consumer")

func (a WebSocketAdapter) Upgrade(ctx context.Context, auth appauth.AuthorizationContext, w http.ResponseWriter, r *http.Request) {
	if a.Hub == nil || a.Origin == nil || !a.Origin(r, auth) {
		http.Error(w, "not authorized", http.StatusForbidden)
		return
	}
	protocol, ok := compatibleSubprotocol(r.Header.Get("Sec-WebSocket-Protocol"))
	if !ok {
		http.Error(w, "SDK version incompatible", http.StatusUpgradeRequired)
		return
	}
	options := &websocket.AcceptOptions{InsecureSkipVerify: true}
	if protocol != "" {
		options.Subprotocols = []string{protocol}
	}
	c, err := websocket.Accept(w, r, options)
	if err != nil {
		return
	}
	defer c.CloseNow()
	limit := a.MaxFrame
	if limit <= 0 {
		limit = defaultFrameBytes
	}
	c.SetReadLimit(limit)
	t := newWSTransport(c, a.writeTimeout(), a.outboundSize())
	livenessDone := make(chan struct{})
	go func() {
		defer close(livenessDone)
		a.watchLiveness(c, t.done, t)
	}()
	defer func() {
		t.Close(1000, "closed")
		<-livenessDone
	}()
	conn, err := a.Hub.Attach(auth, t)
	if err != nil {
		_ = c.Close(websocket.StatusPolicyViolation, "denied")
		return
	}
	defer conn.Close()
	now := a.now()
	publishedAt := make([]time.Time, 0, defaultPublishRate)
	for {
		_, raw, err := c.Read(ctx)
		if err != nil {
			return
		}
		f, err := ParseClientFrame(raw, int(limit))
		if err != nil {
			_ = c.Close(websocket.StatusPolicyViolation, "invalid frame")
			return
		}
		switch f.Type {
		case "subscribe":
			if conn.Subscribe(f.Channel) != nil {
				_ = c.Close(websocket.StatusPolicyViolation, "denied")
				return
			}
		case "unsubscribe":
			if conn.Unsubscribe(f.Channel) != nil {
				_ = c.Close(websocket.StatusPolicyViolation, "denied")
				return
			}
		case "subscribe_kv":
			if conn.SubscribeKV(f.Prefix) != nil {
				_ = c.Close(websocket.StatusPolicyViolation, "denied")
				return
			}
		case "publish":
			current := now()
			cutoff := current.Add(-time.Second)
			kept := publishedAt[:0]
			for _, at := range publishedAt {
				if at.After(cutoff) {
					kept = append(kept, at)
				}
			}
			publishedAt = kept
			if len(publishedAt) >= defaultPublishRate {
				_ = c.Close(websocket.StatusPolicyViolation, "rate limited")
				return
			}
			publishedAt = append(publishedAt, current)
			if conn.Publish(ctx, f.Channel, f.Event, f.Payload) != nil {
				_ = c.Close(websocket.StatusPolicyViolation, "denied")
				return
			}
		}
	}
}

func compatibleSubprotocol(raw string) (string, bool) {
	if raw == "" {
		return "", true
	}
	if strings.Contains(raw, ",") {
		return "", false
	}
	const prefix = "tiny.sdk."
	const separator = ".api."
	if !strings.HasPrefix(raw, prefix) {
		return "", false
	}
	parts := strings.Split(strings.TrimPrefix(raw, prefix), separator)
	if len(parts) != 2 || parts[1] != compatibility.AppAPIVersion {
		return "", false
	}
	if !compatibility.Current("0.1.0").AppAPI.Client.Contains(parts[0]) {
		return "", false
	}
	return raw, true
}

func (a WebSocketAdapter) now() func() time.Time {
	if a.Now != nil {
		return a.Now
	}
	return time.Now
}
func (a WebSocketAdapter) writeTimeout() time.Duration {
	if a.WriteTimeout > 0 {
		return a.WriteTimeout
	}
	return defaultWriteTimeout
}
func (a WebSocketAdapter) idleTimeout() time.Duration {
	if a.IdleTimeout > 0 {
		return a.IdleTimeout
	}
	return defaultIdleTimeout
}
func (a WebSocketAdapter) pingInterval() time.Duration {
	if a.PingInterval > 0 {
		return a.PingInterval
	}
	return defaultPingInterval
}
func (a WebSocketAdapter) pongTimeout() time.Duration {
	if a.PongTimeout > 0 {
		return a.PongTimeout
	}
	return defaultPongTimeout
}
func (a WebSocketAdapter) outboundSize() int {
	if a.OutboundSize > 0 {
		return a.OutboundSize
	}
	return defaultOutboundQueue
}

// watchLiveness keeps a connection only while its peer services ping/pong
// control frames. It does not treat application messages as an authorization
// signal: gateway authorization occurred before Upgrade and revocation closes
// t.done immediately. c.Ping is deliberately bounded, so a silent peer cannot
// retain a handler or transport goroutine indefinitely.
func (a WebSocketAdapter) watchLiveness(c *websocket.Conn, done <-chan struct{}, t *wsTransport) {
	idle := a.idleTimeout()
	timer := time.NewTimer(idle)
	defer timer.Stop()
	ticker := time.NewTicker(a.pingInterval())
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-timer.C:
			t.Close(1001, "idle timeout")
			return
		case <-ticker.C:
			pongCtx, cancel := context.WithTimeout(context.Background(), a.pongTimeout())
			err := c.Ping(pongCtx)
			cancel()
			if err != nil {
				t.Close(1001, "idle timeout")
				return
			}
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(idle)
		}
	}
}

type wsTransport struct {
	c        *websocket.Conn
	outbound chan []byte
	done     chan struct{}
	once     sync.Once
	timeout  time.Duration
}

func newWSTransport(c *websocket.Conn, timeout time.Duration, size int) *wsTransport {
	t := &wsTransport{c: c, outbound: make(chan []byte, size), done: make(chan struct{}), timeout: timeout}
	go t.writeLoop()
	return t
}

func (t *wsTransport) Send(_ context.Context, e Envelope) error {
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	// Hub fan-out must never wait behind the network. A full per-socket queue
	// is a deterministic slow-consumer signal; the hub closes that socket.
	select {
	case <-t.done:
		return errSlowConsumer
	case t.outbound <- b:
		return nil
	default:
		return errSlowConsumer
	}
}
func (t *wsTransport) Close(code int, reason string) {
	t.once.Do(func() {
		close(t.done)
		// Do not wait for a hostile peer to complete a close handshake. This is
		// particularly important for revocation and liveness timeouts: closing
		// must promptly release the reader, writer, and hub membership.
		_ = t.c.CloseNow()
	})
}

func (t *wsTransport) writeLoop() {
	for {
		select {
		case <-t.done:
			return
		case b := <-t.outbound:
			ctx, cancel := context.WithTimeout(context.Background(), t.timeout)
			err := t.c.Write(ctx, websocket.MessageText, b)
			cancel()
			if err != nil {
				t.Close(1013, "slow consumer")
				return
			}
		}
	}
}

// SameOrigin requires a concrete host origin; no wildcard cross-origin upgrades.
func SameOrigin(r *http.Request, _ appauth.AuthorizationContext) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return false
	}
	return strings.EqualFold(origin, "https://"+r.Host) || strings.EqualFold(origin, "http://"+r.Host)
}
