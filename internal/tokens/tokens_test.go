package tokens

import (
	"testing"
	"time"
)

func TestRevokedTokenDenied(t *testing.T) {
	now := time.Now()
	r, raw, e := Issue("tok", "usr", "app", []Scope{ScopeDeployCreate}, now.Add(time.Hour))
	if e != nil {
		t.Fatal(e)
	}
	if e = r.Authenticate(raw, now); e != nil {
		t.Fatal(e)
	}
	r.RevokedAt = &now
	if e = r.Authenticate(raw, now); e != ErrRevoked {
		t.Fatalf("%v", e)
	}
}
func TestScopedTokenCannotCrossApp(t *testing.T) {
	r, _, _ := Issue("tok", "usr", "app-a", []Scope{ScopeDeployCreate}, time.Now().Add(time.Hour))
	if r.Allows(ScopeDeployCreate, "app-b") == nil {
		t.Fatal("cross app accepted")
	}
}
