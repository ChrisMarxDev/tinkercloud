// Package ratelimit provides the small, bounded, single-node abuse-control
// primitive used by public OTP endpoints. It deliberately stores only keyed
// digests for personal request attributes.
package ratelimit

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/tinyhost/tiny/internal/identity"
)

// Kind separates request and verification budgets so a request flood cannot
// consume the verification budget (or vice versa).
type Kind string

const (
	OTPRequest Kind = "otp_request"
	OTPVerify  Kind = "otp_verify"
)

// Policy is a rolling-window limit. All four dimensions must allow a request.
type Policy struct {
	Window                  time.Duration
	PerIP, PerEmail, PerApp int
	Global                  int
}

// Config is purpose-specific. MaxKeys bounds memory even under a distributed
// address/email flood. When the bound is reached, a new key is denied rather
// than evicting an active key and letting the caller bypass a budget.
type Config struct {
	Request, Verify Policy
	MaxKeys         int
}

func DefaultConfig() Config {
	return Config{
		Request: Policy{Window: 10 * time.Minute, PerIP: 10, PerEmail: 3, PerApp: 100, Global: 1000},
		Verify:  Policy{Window: 10 * time.Minute, PerIP: 20, PerEmail: 10, PerApp: 200, Global: 2000},
		MaxKeys: 10000,
	}
}

type bucket struct{ events []time.Time }
type transactionBinding struct {
	emailDigest string
	expiresAt   time.Time
}

// Limiter is concurrency-safe. It has no persistence by design: this is a
// bounded single-node V1 guard, not an identity or audit store.
type Limiter struct {
	key          []byte
	cfg          Config
	now          func() time.Time
	mu           sync.Mutex
	buckets      map[string]bucket
	transactions map[string]transactionBinding
}

func New(key []byte, cfg Config) *Limiter {
	if len(key) == 0 {
		return nil
	}
	if !validPolicy(cfg.Request) || !validPolicy(cfg.Verify) || cfg.MaxKeys < 4 || cfg.MaxKeys > 100000 {
		return nil
	}
	return &Limiter{key: append([]byte(nil), key...), cfg: cfg, now: time.Now, buckets: make(map[string]bucket), transactions: make(map[string]transactionBinding)}
}

func validPolicy(p Policy) bool {
	if p.Window < time.Second || p.Window > time.Hour {
		return false
	}
	for _, n := range []int{p.PerIP, p.PerEmail, p.PerApp, p.Global} {
		if n < 1 || n > 1000000 {
			return false
		}
	}
	return true
}

// SetClock is test-only composition support. Calls must not race with Check.
func (l *Limiter) SetClock(now func() time.Time) {
	if l != nil && now != nil {
		l.now = now
	}
}

// Allow derives the remote address only from RemoteAddr. Forwarded headers are
// intentionally never consulted; accepting them would give callers control of
// the IP bucket. Raw IP and raw email are used only during this call.
func (l *Limiter) Allow(kind Kind, remoteAddr, email, app string) bool {
	if l == nil { // nil is an intentional development/test bypass, never production composition.
		return true
	}
	_, ok := l.policy(kind)
	if !ok {
		return false
	}
	return l.allow(kind, remoteAddr, l.digest(normalizedOrOpaque(email)), app)
}

// BindTransaction associates an opaque login transaction with the email HMAC
// used by VerifyTransaction. The raw email is never retained. Call this only
// after a request has passed its request budget and a transaction was issued.
func (l *Limiter) BindTransaction(transaction, email string) {
	if l == nil || transaction == "" {
		return
	}
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.pruneTransactionsLocked(now)
	if _, exists := l.transactions[transaction]; !exists && len(l.transactions) >= l.cfg.MaxKeys {
		return
	}
	l.transactions[transaction] = transactionBinding{emailDigest: l.digest(normalizedOrOpaque(email)), expiresAt: now.Add(l.cfg.Request.Window)}
}

// AllowTransaction uses the email HMAC previously associated with an opaque
// transaction. Unknown transactions use an HMAC of the transaction instead:
// they remain rate-limited without learning whether a transaction exists.
func (l *Limiter) AllowTransaction(kind Kind, remoteAddr, transaction, app string) bool {
	if l == nil {
		return true
	}
	now := l.now()
	l.mu.Lock()
	binding, ok := l.transactions[transaction]
	if ok && !binding.expiresAt.After(now) {
		delete(l.transactions, transaction)
		ok = false
	}
	l.mu.Unlock()
	mail := l.digest("transaction:" + transaction)
	if ok {
		mail = binding.emailDigest
	}
	return l.allow(kind, remoteAddr, mail, app)
}

func (l *Limiter) allow(kind Kind, remoteAddr, emailDigest, app string) bool {
	p, ok := l.policy(kind)
	if !ok {
		return false
	}
	ip := remoteIP(remoteAddr)
	keys := []string{
		string(kind) + ":ip:" + l.digest(ip),
		string(kind) + ":email:" + emailDigest,
		string(kind) + ":app:" + safeApp(app),
		string(kind) + ":global",
	}
	limits := []int{p.PerIP, p.PerEmail, p.PerApp, p.Global}
	now := l.now()
	cutoff := now.Add(-p.Window)

	l.mu.Lock()
	defer l.mu.Unlock()
	l.pruneLocked(cutoff)
	l.pruneTransactionsLocked(now)
	for i, k := range keys {
		b, exists := l.buckets[k]
		if !exists && len(l.buckets) >= l.cfg.MaxKeys {
			return false
		}
		if len(b.events) >= limits[i] {
			return false
		}
	}
	for _, k := range keys {
		b := l.buckets[k]
		b.events = append(b.events, now)
		l.buckets[k] = b
	}
	return true
}

func (l *Limiter) policy(kind Kind) (Policy, bool) {
	switch kind {
	case OTPRequest:
		return l.cfg.Request, true
	case OTPVerify:
		return l.cfg.Verify, true
	default:
		return Policy{}, false
	}
}

func (l *Limiter) pruneLocked(cutoff time.Time) {
	for k, b := range l.buckets {
		i := 0
		for i < len(b.events) && !b.events[i].After(cutoff) {
			i++
		}
		if i == len(b.events) {
			delete(l.buckets, k)
			continue
		}
		if i > 0 {
			b.events = append([]time.Time(nil), b.events[i:]...)
			l.buckets[k] = b
		}
	}
}

func (l *Limiter) pruneTransactionsLocked(now time.Time) {
	for tx, binding := range l.transactions {
		if !binding.expiresAt.After(now) {
			delete(l.transactions, tx)
		}
	}
}

func (l *Limiter) digest(value string) string {
	h := hmac.New(sha256.New, l.key)
	_, _ = h.Write([]byte(value))
	return hex.EncodeToString(h.Sum(nil))
}

func remoteIP(remote string) string {
	host, _, err := net.SplitHostPort(remote)
	if err == nil && net.ParseIP(host) != nil {
		return host
	}
	if net.ParseIP(remote) != nil {
		return remote
	}
	return "invalid"
}

func normalizedOrOpaque(email string) string {
	if normalized, err := identity.Normalize(email); err == nil {
		return normalized
	}
	// Invalid addresses cannot cause outbound email, but keeping a separate
	// HMAC bucket avoids turning all malformed input into one shared bucket.
	return "invalid:" + strings.TrimSpace(strings.ToLower(email))
}

func safeApp(app string) string {
	app = strings.TrimSpace(strings.ToLower(app))
	if app == "" {
		return "invalid"
	}
	return app
}
