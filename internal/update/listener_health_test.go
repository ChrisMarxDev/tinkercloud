package update

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

type fakeListenerConn struct{ closed bool }

func (c *fakeListenerConn) Read([]byte) (int, error)         { return 0, nil }
func (c *fakeListenerConn) Write([]byte) (int, error)        { return 0, nil }
func (c *fakeListenerConn) Close() error                     { c.closed = true; return nil }
func (c *fakeListenerConn) LocalAddr() net.Addr              { return nil }
func (c *fakeListenerConn) RemoteAddr() net.Addr             { return nil }
func (c *fakeListenerConn) SetDeadline(time.Time) error      { return nil }
func (c *fakeListenerConn) SetReadDeadline(time.Time) error  { return nil }
func (c *fakeListenerConn) SetWriteDeadline(time.Time) error { return nil }

func TestListenerHealthRetriesOnlyUntilConfiguredListenersAccept(t *testing.T) {
	now := time.Unix(0, 0)
	calls := 0
	var waits []time.Duration
	err := (ListenerHealth{
		Addresses:      []string{":80", ":443"},
		Attempts:       4,
		InitialBackoff: time.Millisecond,
		MaximumBackoff: 4 * time.Millisecond,
		Window:         20 * time.Millisecond,
		Now:            func() time.Time { return now },
		Dial: func(_ context.Context, network, address string) (net.Conn, error) {
			if network != "tcp" || (address != "127.0.0.1:80" && address != "127.0.0.1:443") {
				t.Fatalf("dial %s %s", network, address)
			}
			calls++
			if calls <= 2 {
				return nil, errors.New("not listening")
			}
			return &fakeListenerConn{}, nil
		},
		Wait: func(_ context.Context, delay time.Duration) error {
			waits = append(waits, delay)
			now = now.Add(delay)
			return nil
		},
	}).Check(context.Background())
	if err != nil || calls != 4 || len(waits) != 2 || waits[0] != time.Millisecond || waits[1] != 2*time.Millisecond {
		t.Fatalf("err=%v calls=%d waits=%v", err, calls, waits)
	}
}

func TestListenerHealthFailsClosedForInvalidAddressAndCancellation(t *testing.T) {
	if err := (ListenerHealth{Addresses: []string{"not-an-address"}}).Check(context.Background()); err != ErrHealth {
		t.Fatalf("invalid address error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	err := (ListenerHealth{Addresses: []string{":80"}, Dial: func(context.Context, string, string) (net.Conn, error) { calls++; return nil, nil }}).Check(ctx)
	if err != ErrHealth || calls != 0 {
		t.Fatalf("cancelled err=%v calls=%d", err, calls)
	}
}

func TestListenerHealthStopsAtBoundedAttempts(t *testing.T) {
	calls := 0
	err := (ListenerHealth{
		Addresses:      []string{":80"},
		Attempts:       2,
		InitialBackoff: time.Millisecond,
		Dial: func(context.Context, string, string) (net.Conn, error) {
			calls++
			return nil, errors.New("not listening")
		},
		Wait: func(context.Context, time.Duration) error { return nil },
	}).Check(context.Background())
	if err != ErrHealth || calls != 2 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}
