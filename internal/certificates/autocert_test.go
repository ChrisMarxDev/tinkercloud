package certificates

import (
	"context"
	"errors"
	"testing"
)

type hosts map[string]bool

func (h hosts) ActiveAppHost(v string) bool { return h[v] }
func TestAutocertHostPolicyFailClosed(t *testing.T) {
	m := NewAutocert(t.TempDir(), "ops@example.test", "tiny.example.test", hosts{"app.apps.example.test": true}, func(string) bool { return true })
	for _, host := range []string{"tiny.example.test", "TINY.EXAMPLE.TEST.", "app.apps.example.test"} {
		if err := m.hostPolicy(context.Background(), host); err != nil {
			t.Fatalf("%s: %v", host, err)
		}
	}
	for _, host := range []string{"unknown.apps.example.test", "suspended.apps.example.test", "deleting.apps.example.test", "evil.example.test"} {
		if err := m.hostPolicy(context.Background(), host); !errors.Is(err, ErrHostDenied) {
			t.Fatalf("%s accepted: %v", host, err)
		}
	}
}
func TestAutocertNilResolverDeniesApps(t *testing.T) {
	m := NewAutocert(t.TempDir(), "ops@example.test", "tiny.example.test", nil, func(string) bool { return true })
	if err := m.hostPolicy(context.Background(), "app.apps.example.test"); !errors.Is(err, ErrHostDenied) {
		t.Fatal(err)
	}
}
