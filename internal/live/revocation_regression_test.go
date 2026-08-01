package live

import (
	"context"
	"encoding/json"
	"testing"
)

// A revoked transport can still have a buffered inbound frame. The hub must
// reject it itself rather than relying on a WebSocket adapter to discard it.
func TestRevokedConnectionCannotPublishBufferedFrame(t *testing.T) {
	h := New(DefaultLimits())
	publisherTransport, recipientTransport := &transport{}, &transport{}
	publisher, err := h.Attach(authorized(t, "a"), publisherTransport)
	if err != nil {
		t.Fatal(err)
	}
	recipient, err := h.Attach(authorized(t, "a"), recipientTransport)
	if err != nil {
		t.Fatal(err)
	}
	if err := recipient.Subscribe("updates"); err != nil {
		t.Fatal(err)
	}
	h.Revoke("a", "")
	if err := publisher.Publish(context.Background(), "updates", "changed", json.RawMessage(`{"stale":true}`)); err != ErrClosed {
		t.Fatalf("revoked connection published buffered event: got %v, want %v", err, ErrClosed)
	}
	if len(recipientTransport.sent) != 0 {
		t.Fatal("revoked publisher delivered an event")
	}
}
