package ratelimit

import (
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func testLimiter(now *time.Time) *Limiter {
	l := New([]byte("test-key"), Config{Request: Policy{Window: time.Minute, PerIP: 2, PerEmail: 3, PerApp: 4, Global: 20}, Verify: Policy{Window: time.Minute, PerIP: 2, PerEmail: 2, PerApp: 4, Global: 20}, MaxKeys: 50})
	l.SetClock(func() time.Time { return *now })
	return l
}

func TestAllowUsesRemoteAddrNotForwardedValue(t *testing.T) {
	now := time.Unix(100, 0)
	l := testLimiter(&now)
	if !l.Allow(OTPRequest, "203.0.113.1:99", "one@example.com", "a") || !l.Allow(OTPRequest, "203.0.113.1:99", "two@example.com", "b") {
		t.Fatal("initial calls denied")
	}
	if l.Allow(OTPRequest, "203.0.113.1:99", "three@example.com", "c") {
		t.Fatal("remote address IP limit bypassed")
	}
}

func TestAllowEmailIsNormalizedAndExpires(t *testing.T) {
	now := time.Unix(100, 0)
	l := testLimiter(&now)
	if !l.Allow(OTPVerify, "203.0.113.1:1", "a@Example.COM", "a") || !l.Allow(OTPVerify, "203.0.113.2:1", "a@example.com", "a") {
		t.Fatal("normalized email should fit budget")
	}
	if l.Allow(OTPVerify, "203.0.113.3:1", "a@example.com", "a") {
		t.Fatal("email limit was not applied")
	}
	now = now.Add(time.Minute + time.Nanosecond)
	if !l.Allow(OTPVerify, "203.0.113.3:1", "a@example.com", "a") {
		t.Fatal("expired rolling window remained denied")
	}
}

func TestTransactionBindingUsesEmailBudgetWithoutRawEmail(t *testing.T) {
	now := time.Unix(100, 0)
	l := testLimiter(&now)
	l.BindTransactionFingerprint("login_one", "a@example.com", "origin-one")
	l.BindTransactionFingerprint("login_two", "a@example.com", "origin-two")
	if !l.AllowTransactionFingerprint(OTPVerify, "203.0.113.1:1", "changed-browser", "login_one", "control") || !l.AllowTransactionFingerprint(OTPVerify, "203.0.113.2:1", "another-browser", "login_two", "control") {
		t.Fatal("bound transactions denied")
	}
	if l.AllowTransactionFingerprint(OTPVerify, "203.0.113.3:1", "third-browser", "login_two", "control") {
		t.Fatal("email HMAC budget bypassed through transaction")
	}
}

func TestBoundedStateFailsClosedAndConcurrentBudget(t *testing.T) {
	now := time.Unix(100, 0)
	l := New([]byte("test-key"), Config{Request: Policy{Window: time.Minute, PerIP: 100, PerEmail: 100, PerApp: 100, Global: 4}, Verify: Policy{Window: time.Minute, PerIP: 1, PerEmail: 1, PerApp: 1, Global: 1}, MaxKeys: 5})
	l.SetClock(func() time.Time { return now })
	if !l.Allow(OTPRequest, "203.0.113.1:1", "a@example.com", "a") {
		t.Fatal("first call")
	}
	// Five dimensions consumed the bounded key set. A new IP must fail closed.
	if l.Allow(OTPRequest, "203.0.113.2:1", "b@example.com", "b") {
		t.Fatal("new key accepted after capacity")
	}

	l = New([]byte("test-key"), Config{Request: Policy{Window: time.Minute, PerIP: 100, PerEmail: 100, PerApp: 100, Global: 4}, Verify: Policy{Window: time.Minute, PerIP: 1, PerEmail: 1, PerApp: 1, Global: 1}, MaxKeys: 100})
	l.SetClock(func() time.Time { return now })
	var wg sync.WaitGroup
	allowed := 0
	var mu sync.Mutex
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if l.Allow(OTPRequest, "203.0.113."+string(rune('a'+i))+":1", "a@example.com", "a") {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	if allowed != 4 {
		t.Fatalf("allowed=%d want 4", allowed)
	}
}

func TestRequestFingerprintIsolationAndBoundedInputs(t *testing.T) {
	now := time.Unix(100, 0)
	l := New([]byte("test-key"), Config{Request: Policy{Window: time.Minute, PerIP: 10, PerFingerprint: 1, PerEmail: 10, PerApp: 10, Global: 20}, Verify: Policy{Window: time.Minute, PerIP: 10, PerFingerprint: 10, PerEmail: 10, PerApp: 10, Global: 20}, MaxKeys: 100})
	l.SetClock(func() time.Time { return now })
	a := httptest.NewRequest("POST", "https://tiny.test/login", nil)
	a.RemoteAddr = "203.0.113.1:1"
	a.Header.Set("User-Agent", "same-agent")
	a.Header.Set("Accept-Language", "en")
	b := a.Clone(a.Context())
	b.RemoteAddr = "203.0.113.2:1"
	b.Header.Set("X-Forwarded-For", "198.51.100.99")
	if !l.AllowRequest(OTPRequest, a, "one@example.com", "control") || !l.AllowRequest(OTPRequest, b, "two@example.com", "control") {
		t.Fatal("unrelated peers sharing a browser hint must not share a fingerprint budget")
	}
	c := a.Clone(a.Context())
	c.Header.Set("User-Agent", "different-agent")
	if !l.AllowRequest(OTPRequest, c, "three@example.com", "control") {
		t.Fatal("a separate bounded browser hint should have its own fingerprint budget")
	}
	d := a.Clone(a.Context())
	d.Header.Set("User-Agent", "same-agent")
	if l.AllowRequest(OTPRequest, d, "four@example.com", "control") {
		t.Fatal("same peer and browser fingerprint bypassed its budget")
	}
	oversized := httptest.NewRequest("POST", "https://tiny.test/login", nil)
	oversized.RemoteAddr = "203.0.113.3:1"
	oversized.Header.Set("User-Agent", string(make([]byte, 600))+"\x01")
	if len(RequestFingerprint(oversized)) > 600 {
		t.Fatal("fingerprint retained an unbounded header")
	}
}

func TestInvalidConfigurationFailsClosedAtComposition(t *testing.T) {
	if New([]byte("key"), Config{Request: Policy{Window: time.Second, PerIP: 1, PerEmail: 1, PerApp: 1, Global: 1}, Verify: Policy{Window: time.Second, PerIP: 1, PerEmail: 1, PerApp: 1, Global: 1}, MaxKeys: 3}) != nil {
		t.Fatal("undersized state bound accepted")
	}
	if New([]byte("key"), Config{Request: Policy{Window: 0, PerIP: 1, PerEmail: 1, PerApp: 1, Global: 1}, Verify: Policy{Window: time.Second, PerIP: 1, PerEmail: 1, PerApp: 1, Global: 1}, MaxKeys: 4}) != nil {
		t.Fatal("unsafe window accepted")
	}
}
