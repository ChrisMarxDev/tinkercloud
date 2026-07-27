package compose

import (
	"context"
	"github.com/tinyhost/tiny/internal/appauth"
	"github.com/tinyhost/tiny/internal/apps"
	"github.com/tinyhost/tiny/internal/identity"
	"github.com/tinyhost/tiny/internal/live"
	"github.com/tinyhost/tiny/internal/policies"
	"github.com/tinyhost/tiny/internal/sessions"
	"testing"
	"time"
)

type tr struct{ code int }

func (*tr) Send(context.Context, live.Envelope) error { return nil }
func (t *tr) Close(c int, _ string)                   { t.code = c }
func TestLiveSessionsRevokesOnlyMatchingConnection(t *testing.T) {
	store := sessions.NewMemoryStore()
	v := identity.Identity{ID: "u", Email: "u@test"}
	raw, _, _ := store.Create("a", v, time.Now().Add(time.Hour))
	p := &policies.MemoryStore{Policies: map[string]policies.Policy{"a": {AppID: "a", OwnerIdentityID: "u", Valid: true}}}
	auth, _ := appauth.Authorizer{Sessions: store, Policies: p}.Authorize(context.Background(), apps.App{ID: "a"}, raw, "r")
	h := live.New(live.DefaultLimits())
	t1 := &tr{}
	_, _ = h.Attach(auth, t1)
	ls := LiveSessions{Sessions: MemorySessions{Store: store}, Hub: h}
	id, e := ls.Revoke(context.Background(), "a", raw)
	if e != nil || id == raw || t1.code != 1008 {
		t.Fatal(id, e, t1.code)
	}
	if _, e = ls.Revoke(context.Background(), "b", raw); e == nil {
		t.Fatal("wrong app revoked")
	}
}
