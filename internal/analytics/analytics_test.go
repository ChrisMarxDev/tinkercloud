package analytics

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"
)

type sinkSpy struct{ events chan Event }

func (s sinkSpy) RecordInsight(_ context.Context, event Event) error { s.events <- event; return nil }

func TestMarkerCookieIsHostOnlyAndDigestIsAppScoped(t *testing.T) {
	cookie, err := NewMarkerCookie(time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC))
	if err != nil || !cookie.Secure || !cookie.HttpOnly || cookie.Path != "/" || cookie.Domain != "" || cookie.SameSite != 2 {
		t.Fatalf("unsafe cookie: %#v err=%v", cookie, err)
	}
	r := httptest.NewRequest("GET", "https://alpha.test/", nil)
	r.AddCookie(cookie)
	marker, ok := ReadMarker(r)
	if !ok {
		t.Fatal("issued marker rejected")
	}
	if Digest([]byte("key"), "a", marker) == Digest([]byte("key"), "b", marker) {
		t.Fatal("digest not app scoped")
	}
	r.Header.Set("Cookie", CookieName+"=too-short")
	if _, ok := ReadMarker(r); ok {
		t.Fatal("malformed marker accepted")
	}
}

func TestRecorderDropsBeforeStartAndWhenFull(t *testing.T) {
	r := NewRecorder(sinkSpy{events: make(chan Event)}, 1)
	e := Event{AppID: "a", Occurred: time.Now()}
	if r.Offer(e) {
		t.Fatal("stopped recorder accepted event")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.Start(ctx)
	// A sink which never reads blocks the only worker; the fixed queue must
	// eventually drop rather than make a request wait.
	deadline := time.Now().Add(time.Second)
	for !r.Offer(e) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if r.Offer(e) && r.Offer(e) {
		t.Fatal("full recorder accepted unbounded events")
	}
}
