package gateway

import (
	"testing"

	"github.com/tinyhost/tiny/internal/config"
)

func FuzzClassifyHostNeverPanics(f *testing.F) {
	cfg := config.Config{PlatformHost: "tiny.example.test", AppSuffix: "apps.example.test"}
	for _, seed := range []string{"demo.apps.example.test", "demo.apps.example.test:443", "..apps.example.test", "[::1]:443", "evilapps.example.test", "demo.apps.example.test."} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, host string) { ClassifyHost(host, cfg) })
}
