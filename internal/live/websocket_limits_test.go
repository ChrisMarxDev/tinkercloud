package live

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestWebSocketDefaultFrameLimitDeniesOversizeFrame(t *testing.T) {
	a := WebSocketAdapter{Hub: New(DefaultLimits()), Origin: SameOrigin}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.Upgrade(r.Context(), authorized(t, "a"), w, r)
	}))
	defer s.Close()

	c, _, err := websocket.Dial(context.Background(), "ws"+strings.TrimPrefix(s.URL, "http")+"/_tinker/ws/v1", &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {s.URL}}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	// This is deliberately larger than the protocol's default 32 KiB inbound
	// limit. The server must close before parsing or dispatching it.
	if err := c.Write(context.Background(), websocket.MessageText, []byte(strings.Repeat("x", defaultFrameBytes+1))); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, _, err := c.Read(ctx); err == nil {
		t.Fatal("oversize frame left the socket usable")
	}
}

func TestWebSocketPublishRateLimitUsesInjectedClock(t *testing.T) {
	fixed := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	a := WebSocketAdapter{Hub: New(DefaultLimits()), Origin: SameOrigin, Now: func() time.Time { return fixed }}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.Upgrade(r.Context(), authorized(t, "a"), w, r)
	}))
	defer s.Close()

	c, _, err := websocket.Dial(context.Background(), "ws"+strings.TrimPrefix(s.URL, "http")+"/_tinker/ws/v1", &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {s.URL}}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	frame := []byte(`{"v":1,"type":"publish","channel":"updates","event":"changed","payload":true}`)
	for i := 0; i < defaultPublishRate+1; i++ {
		if err := c.Write(context.Background(), websocket.MessageText, frame); err != nil {
			t.Fatalf("publish %d: %v", i, err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, _, err := c.Read(ctx); err == nil {
		t.Fatal("21 publishes in a one-second window left socket usable")
	}
}

func TestWebSocketSilentPeerClosesWithinLivenessBound(t *testing.T) {
	adapter := WebSocketAdapter{
		Hub:          New(DefaultLimits()),
		Origin:       SameOrigin,
		IdleTimeout:  120 * time.Millisecond,
		PingInterval: 30 * time.Millisecond,
		PongTimeout:  20 * time.Millisecond,
	}
	handlerExited := make(chan struct{})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		adapter.Upgrade(r.Context(), authorized(t, "a"), w, r)
		close(handlerExited)
	}))
	defer s.Close()

	// The client deliberately never calls Read. coder/websocket can only
	// service server pings while reading, so this models a peer retaining an
	// otherwise-open TCP connection without responding to a ping.
	c, _, err := websocket.Dial(context.Background(), "ws"+strings.TrimPrefix(s.URL, "http")+"/_tinker/ws/v1", &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {s.URL}}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	select {
	case <-handlerExited:
	case <-time.After(300 * time.Millisecond):
		t.Fatal("silent peer retained websocket handler beyond liveness bound")
	}
}

func TestWebSocketPongResponsivePeerStaysAlive(t *testing.T) {
	adapter := WebSocketAdapter{
		Hub:          New(DefaultLimits()),
		Origin:       SameOrigin,
		IdleTimeout:  120 * time.Millisecond,
		PingInterval: 30 * time.Millisecond,
		PongTimeout:  20 * time.Millisecond,
	}
	handlerExited := make(chan struct{})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		adapter.Upgrade(r.Context(), authorized(t, "a"), w, r)
		close(handlerExited)
	}))
	defer s.Close()

	c, _, err := websocket.Dial(context.Background(), "ws"+strings.TrimPrefix(s.URL, "http")+"/_tinker/ws/v1", &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {s.URL}}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	readCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	readErr := make(chan error, 1)
	go func() {
		_, _, err := c.Read(readCtx)
		readErr <- err
	}()

	// Read processes ping control frames and sends pongs. Four heartbeat
	// intervals must not turn a responsive, otherwise-idle peer into a timeout.
	select {
	case err := <-readErr:
		t.Fatalf("pong-responsive peer closed: %v", err)
	case <-time.After(130 * time.Millisecond):
	}
	if err := c.Write(context.Background(), websocket.MessageText, []byte(`{"v":1,"type":"subscribe","channel":"updates"}`)); err != nil {
		t.Fatalf("responsive connection unusable: %v", err)
	}
	c.CloseNow()
	select {
	case <-handlerExited:
	case <-time.After(time.Second):
		t.Fatal("responsive connection leaked websocket handler on close")
	}
}
