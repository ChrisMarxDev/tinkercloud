package contract

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/tinyhost/tiny/internal/apps"
	"github.com/tinyhost/tiny/internal/compose"
	"github.com/tinyhost/tiny/internal/config"
	"github.com/tinyhost/tiny/internal/identity"
	"github.com/tinyhost/tiny/internal/kv"
	"github.com/tinyhost/tiny/internal/live"
	"github.com/tinyhost/tiny/internal/policies"
	"github.com/tinyhost/tiny/internal/sessions"
)

// contractKV is deliberately a repository rather than a mocked HTTP surface:
// the SDK test crosses the real composed gateway and service boundary.
type contractKV struct {
	mu   sync.Mutex
	data map[string]map[string]kv.Entry
}

func (r *contractKV) Get(_ context.Context, app, key string) (*kv.Entry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.data[app][key]
	if !ok {
		return nil, nil
	}
	e.Value = append(json.RawMessage(nil), e.Value...)
	return &e, nil
}
func (r *contractKV) Set(_ context.Context, app, key string, value json.RawMessage, expected *uint64) (kv.Entry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.data[app] == nil {
		r.data[app] = map[string]kv.Entry{}
	}
	old, exists := r.data[app][key]
	if expected != nil && (!exists || old.Version != *expected) {
		return kv.Entry{}, kv.ErrVersionConflict
	}
	e := kv.Entry{Key: key, Value: append(json.RawMessage(nil), value...), Version: old.Version + 1, UpdatedAt: time.Now().UTC()}
	r.data[app][key] = e
	return e, nil
}
func (r *contractKV) Delete(_ context.Context, app, key string, expected *uint64) (bool, kv.Mutation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	old, ok := r.data[app][key]
	if expected != nil && (!ok || old.Version != *expected) {
		return false, kv.Mutation{}, kv.ErrVersionConflict
	}
	if !ok {
		return false, kv.Mutation{}, nil
	}
	delete(r.data[app], key)
	return true, kv.Mutation{Key: key, Version: old.Version + 1, Deleted: true}, nil
}
func (r *contractKV) List(_ context.Context, app, prefix, cursor string, limit int) (kv.ListResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	keys := make([]string, 0)
	for key := range r.data[app] {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix && key > cursor {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	out := kv.ListResult{}
	for i, key := range keys {
		if i == limit {
			out.NextCursor = keys[i-1]
			break
		}
		e := r.data[app][key]
		e.Value = append(json.RawMessage(nil), e.Value...)
		out.Entries = append(out.Entries, e)
	}
	return out, nil
}

func TestBuiltSDKAgainstComposedGateway(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is required for browser SDK contract")
	}
	store := sessions.NewMemoryStore()
	token, _, err := store.Create("alpha-id", identity.Identity{ID: "viewer", Email: "viewer@example.com"}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	appsRepo := apps.NewMemoryRepository(
		apps.App{ID: "alpha-id", Slug: "alpha", Status: apps.Active, KVEnabled: true, RealtimeEnabled: true},
		apps.App{ID: "beta-id", Slug: "beta", Status: apps.Active, KVEnabled: true, RealtimeEnabled: true},
	)
	policy := &policies.MemoryStore{Policies: map[string]policies.Policy{
		"alpha-id": {AppID: "alpha-id", OwnerIdentityID: "viewer", Valid: true},
		"beta-id":  {AppID: "beta-id", OwnerIdentityID: "viewer", Valid: true},
	}}
	h := compose.AppPlane(config.Config{PlatformHost: "platform.localhost", AppSuffix: "localhost", SessionCookie: sessions.AppCookieName}, appsRepo, compose.MemorySessions{Store: store}, policy, &contractKV{data: map[string]map[string]kv.Entry{}}, live.New(live.DefaultLimits()), compose.Login{})
	// Node resolves the reserved `*.localhost` test suffix to IPv6 loopback.
	// Keep the listener local while allowing the browser-style fetch URL to
	// retain a host-derived app name.
	listener, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: h}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Shutdown(context.Background()) })
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	build := exec.Command("npm", "run", "build")
	build.Dir = filepath.Join(root, "sdk", "typescript")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build SDK: %v\n%s", err, out)
	}
	cmd := exec.Command("node", "test/real-handler.mjs")
	cmd.Dir = filepath.Join(root, "sdk", "typescript")
	port := listener.Addr().(*net.TCPAddr).Port
	cmd.Env = append(os.Environ(), "TINY_SDK_CONTRACT_ORIGIN=http://[::1]:"+strconv.Itoa(port), "TINY_SDK_CONTRACT_HOST=alpha.localhost:"+strconv.Itoa(port), "TINY_SDK_CONTRACT_COOKIE="+sessions.AppCookieName+"="+token)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("built SDK contract: %v\n%s", err, out)
	}
}
