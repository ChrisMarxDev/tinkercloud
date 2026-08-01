package live

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
)

func TestRevokeRacesWithConnectionOperations(t *testing.T) {
	h := New(DefaultLimits())
	c, err := h.Attach(authorized(t, "a"), &transport{})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = c.Publish(context.Background(), "updates", "changed", json.RawMessage(`1`))
			_ = c.Subscribe("updates")
			_ = c.Unsubscribe("updates")
		}()
	}
	h.Revoke("a", "")
	wg.Wait()
	if err := c.Publish(context.Background(), "updates", "changed", json.RawMessage(`1`)); !errors.Is(err, ErrClosed) {
		t.Fatalf("publish after revoke = %v", err)
	}
	if err := c.Subscribe("updates"); !errors.Is(err, ErrClosed) {
		t.Fatalf("subscribe after revoke = %v", err)
	}
	if err := c.Unsubscribe("updates"); !errors.Is(err, ErrClosed) {
		t.Fatalf("unsubscribe after revoke = %v", err)
	}
}

func TestDoubleCloseIsHarmless(t *testing.T) {
	h := New(DefaultLimits())
	tr := &transport{}
	c, err := h.Attach(authorized(t, "a"), tr)
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	c.Close()
	if !tr.closed {
		t.Fatal("transport was not closed")
	}
	if err := c.Subscribe("updates"); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed connection accepted subscribe: %v", err)
	}
}
