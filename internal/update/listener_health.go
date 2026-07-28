package update

import (
	"context"
	"net"
	"time"
)

const (
	defaultListenerReadinessWindow = 10 * time.Second
	defaultListenerReadinessTrys   = 12
	defaultListenerInitialBackoff  = 100 * time.Millisecond
	defaultListenerMaximumBackoff  = time.Second
)

// ListenerHealth waits only for the replacement service's configured local
// listeners to accept TCP connections. It is deliberately separate from
// doctor and public gateway checks so retries cannot mask their failures.
type ListenerHealth struct {
	Addresses      []string
	Window         time.Duration
	Attempts       int
	InitialBackoff time.Duration
	MaximumBackoff time.Duration
	Dial           func(context.Context, string, string) (net.Conn, error)
	Wait           func(context.Context, time.Duration) error
	Now            func() time.Time
}

func (h ListenerHealth) Check(ctx context.Context) error {
	if ctx.Err() != nil || len(h.Addresses) == 0 {
		return ErrHealth
	}
	addresses := make([]string, 0, len(h.Addresses))
	for _, address := range h.Addresses {
		local, ok := localListenerAddress(address)
		if !ok {
			return ErrHealth
		}
		addresses = append(addresses, local)
	}
	window := h.Window
	if window <= 0 {
		window = defaultListenerReadinessWindow
	}
	attempts := h.Attempts
	if attempts <= 0 {
		attempts = defaultListenerReadinessTrys
	}
	backoff := h.InitialBackoff
	if backoff <= 0 {
		backoff = defaultListenerInitialBackoff
	}
	maximum := h.MaximumBackoff
	if maximum <= 0 {
		maximum = defaultListenerMaximumBackoff
	}
	if maximum < backoff {
		maximum = backoff
	}
	dial := h.Dial
	if dial == nil {
		dial = (&net.Dialer{}).DialContext
	}
	wait := h.Wait
	if wait == nil {
		wait = waitForListenerReadiness
	}
	now := h.Now
	if now == nil {
		now = time.Now
	}
	deadline := now().Add(window)
	bounded, cancel := context.WithTimeout(ctx, window)
	defer cancel()
	for attempt := 0; attempt < attempts; attempt++ {
		if bounded.Err() != nil {
			return ErrHealth
		}
		if listenersAccept(bounded, addresses, dial) {
			return nil
		}
		if attempt == attempts-1 || bounded.Err() != nil {
			return ErrHealth
		}
		remaining := deadline.Sub(now())
		if remaining <= 0 {
			return ErrHealth
		}
		if wait(bounded, min(backoff, remaining)) != nil || bounded.Err() != nil {
			return ErrHealth
		}
		if backoff < maximum {
			backoff = min(backoff*2, maximum)
		}
	}
	return ErrHealth
}

func localListenerAddress(address string) (string, bool) {
	host, port, err := net.SplitHostPort(address)
	if err != nil || port == "" {
		return "", false
	}
	if host == "" || host == "0.0.0.0" || host == "::" || host == "[::]" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port), true
}

func listenersAccept(ctx context.Context, addresses []string, dial func(context.Context, string, string) (net.Conn, error)) bool {
	for _, address := range addresses {
		conn, err := dial(ctx, "tcp", address)
		if err != nil {
			return false
		}
		if conn == nil || conn.Close() != nil {
			return false
		}
	}
	return true
}

func waitForListenerReadiness(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
