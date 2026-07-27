package live

import (
	"context"
	"encoding/json"
	"github.com/tinyhost/tiny/internal/appauth"
	"github.com/tinyhost/tiny/internal/apps"
	"github.com/tinyhost/tiny/internal/identity"
	"github.com/tinyhost/tiny/internal/kv"
	"github.com/tinyhost/tiny/internal/policies"
	"github.com/tinyhost/tiny/internal/sessions"
	"testing"
	"time"
)

type failingTransport struct{ closed bool }

func (t *failingTransport) Send(context.Context, Envelope) error { return errSlowConsumer }
func (t *failingTransport) Close(int, string)                    { t.closed = true }

func authorized(t *testing.T, appID string) appauth.AuthorizationContext {
	t.Helper()
	viewer := identity.Identity{ID: "u", Email: "u@example.com"}
	store := sessions.NewMemoryStore()
	token, _, _ := store.Create(appID, viewer, time.Now().Add(time.Hour))
	a, err := appauth.Authorizer{Sessions: store, Policies: &policies.MemoryStore{Policies: map[string]policies.Policy{appID: {AppID: appID, OwnerIdentityID: "u", Valid: true}}}}.Authorize(context.Background(), apps.App{ID: appID}, token, "req")
	if err != nil {
		t.Fatal(err)
	}
	return a
}

type transport struct {
	sent   []Envelope
	closed bool
}

func (t *transport) Send(_ context.Context, e Envelope) error { t.sent = append(t.sent, e); return nil }
func (t *transport) Close(int, string)                        { t.closed = true }
func TestAppIsolationAndRevocation(t *testing.T) {
	h := New(DefaultLimits())
	ta, tb := &transport{}, &transport{}
	ca, _ := h.Attach(authorized(t, "a"), ta)
	cb, _ := h.Attach(authorized(t, "b"), tb)
	if err := ca.Subscribe("updates"); err != nil {
		t.Fatal(err)
	}
	if err := cb.Subscribe("updates"); err != nil {
		t.Fatal(err)
	}
	if err := ca.Publish(context.Background(), "updates", "x", json.RawMessage(`1`)); err != nil {
		t.Fatal(err)
	}
	if len(ta.sent) != 1 || len(tb.sent) != 0 {
		t.Fatal("cross-app delivery")
	}
	h.Revoke("a", "")
	if !ta.closed || tb.closed {
		t.Fatal("revocation scope")
	}
}
func TestKVEventAndReservedChannelDenied(t *testing.T) {
	h := New(DefaultLimits())
	t1 := &transport{}
	a := authorized(t, "a")
	c, _ := h.Attach(a, t1)
	if err := c.SubscribeKV("x/"); err != nil {
		t.Fatal(err)
	}
	h.PublishKVChange(context.Background(), a, kv.Mutation{Key: "x/a", Version: 1})
	if len(t1.sent) != 1 {
		t.Fatal("missing kv event")
	}
	if err := c.Subscribe("_tiny"); err == nil {
		t.Fatal("reserved accepted")
	}
}

func TestSlowConsumerIsClosedWithoutBlockingOrAffectingOthers(t *testing.T) {
	h := New(DefaultLimits())
	publisherTransport, healthyTransport, slowTransport := &transport{}, &transport{}, &failingTransport{}
	publisher, err := h.Attach(authorized(t, "a"), publisherTransport)
	if err != nil {
		t.Fatal(err)
	}
	healthy, err := h.Attach(authorized(t, "a"), healthyTransport)
	if err != nil {
		t.Fatal(err)
	}
	slow, err := h.Attach(authorized(t, "a"), slowTransport)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []*Connection{publisher, healthy, slow} {
		if err := c.Subscribe("updates"); err != nil {
			t.Fatal(err)
		}
	}
	if err := publisher.Publish(context.Background(), "updates", "changed", json.RawMessage(`true`)); err != nil {
		t.Fatal(err)
	}
	if !slowTransport.closed {
		t.Fatal("slow consumer was not disconnected")
	}
	if len(healthyTransport.sent) != 1 {
		t.Fatalf("healthy consumer delivery = %d, want 1", len(healthyTransport.sent))
	}
	if err := slow.Publish(context.Background(), "updates", "again", json.RawMessage(`true`)); err != ErrClosed {
		t.Fatalf("closed slow connection published: %v", err)
	}
}
