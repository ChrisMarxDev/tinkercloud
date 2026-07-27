package gateway_test

import (
	"context"
	"github.com/coder/websocket"
	"github.com/tinyhost/tiny/internal/appapi"
	"github.com/tinyhost/tiny/internal/appauth"
	"github.com/tinyhost/tiny/internal/apps"
	"github.com/tinyhost/tiny/internal/compose"
	"github.com/tinyhost/tiny/internal/config"
	"github.com/tinyhost/tiny/internal/gateway"
	"github.com/tinyhost/tiny/internal/identity"
	"github.com/tinyhost/tiny/internal/live"
	"github.com/tinyhost/tiny/internal/policies"
	"github.com/tinyhost/tiny/internal/sessions"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWebSocketGatewayTwoAppIsolation(t *testing.T) {
	v := identity.Identity{ID: "u", Email: "u@test"}
	ss := sessions.NewMemoryStore()
	ta, _, _ := ss.Create("a", v, time.Now().Add(time.Hour))
	tb, _, _ := ss.Create("b", v, time.Now().Add(time.Hour))
	repo := apps.NewMemoryRepository(apps.App{ID: "a", Slug: "alpha", Status: apps.Active, RealtimeEnabled: true}, apps.App{ID: "b", Slug: "beta", Status: apps.Active, RealtimeEnabled: true})
	ps := &policies.MemoryStore{Policies: map[string]policies.Policy{"a": {AppID: "a", OwnerIdentityID: "u", Valid: true}, "b": {AppID: "b", OwnerIdentityID: "u", Valid: true}}}
	hub := live.New(live.DefaultLimits())
	ad := live.WebSocketAdapter{Hub: hub, Origin: live.SameOrigin}
	d := compose.NewAppDispatcher(appapi.Dispatcher{})
	d.Live = &ad
	g := gateway.Gateway{Config: config.Config{PlatformHost: "tiny.test", AppSuffix: "apps.tiny.test", SessionCookie: "__Host-tiny_app"}, Apps: repo, Authorizer: appauth.Authorizer{Sessions: ss, Policies: ps}, Protected: d}
	s := httptest.NewServer(g)
	defer s.Close()
	addr := s.Listener.Addr().String()
	dial := func(host, token string) (*websocket.Conn, error) {
		u := "ws://" + host + "/_tiny/ws/v1"
		tr := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "tcp", addr)
		}}
		h := http.Header{"Origin": []string{"http://" + host}, "Cookie": []string{"__Host-tiny_app=" + token}}
		c, _, e := websocket.Dial(context.Background(), u, &websocket.DialOptions{HTTPClient: &http.Client{Transport: tr}, HTTPHeader: h})
		return c, e
	}
	if c, e := dial("alpha.apps.tiny.test", ""); e == nil {
		c.CloseNow()
		t.Fatal("anonymous upgraded")
	}
	a, e := dial("alpha.apps.tiny.test", ta)
	if e != nil {
		t.Fatal(e)
	}
	defer a.CloseNow()
	b, e := dial("beta.apps.tiny.test", tb)
	if e != nil {
		t.Fatal(e)
	}
	defer b.CloseNow()
	ctx := context.Background()
	_ = a.Write(ctx, websocket.MessageText, []byte(`{"v":1,"type":"subscribe","channel":"x"}`))
	_ = b.Write(ctx, websocket.MessageText, []byte(`{"v":1,"type":"subscribe","channel":"x"}`))
	_ = a.Write(ctx, websocket.MessageText, []byte(`{"v":1,"type":"publish","channel":"x","event":"ok","payload":1}`))
	_, raw, e := a.Read(ctx)
	if e != nil || !strings.Contains(string(raw), `"event":"ok"`) {
		t.Fatal(e, string(raw))
	}
	ctx2, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	_, _, e = b.Read(ctx2)
	if e == nil {
		t.Fatal("cross app event delivered")
	}
	hub.Revoke("a", "")
	revokedCtx, cancelRevoked := context.WithTimeout(context.Background(), time.Second)
	defer cancelRevoked()
	_, _, e = a.Read(revokedCtx)
	if e == nil {
		t.Fatal("revoked socket remained usable")
	}
}
