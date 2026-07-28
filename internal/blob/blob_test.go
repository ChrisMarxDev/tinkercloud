package blob

import (
	"context"
	"io"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tinyhost/tiny/internal/appauth"
	"github.com/tinyhost/tiny/internal/apps"
	"github.com/tinyhost/tiny/internal/identity"
	"github.com/tinyhost/tiny/internal/policies"
	"github.com/tinyhost/tiny/internal/sessions"
)

func blobTestAuth(t *testing.T, app string) appauth.AuthorizationContext {
	t.Helper()
	s := sessions.NewMemoryStore()
	tok, _, _ := s.Create(app, identity.Identity{ID: "v", Email: "v@example.com"}, time.Now().Add(time.Hour))
	a, e := appauth.Authorizer{Sessions: s, Policies: &policies.MemoryStore{Policies: map[string]policies.Policy{app: {AppID: app, OwnerIdentityID: "v", Valid: true}}}}.Authorize(context.Background(), apps.App{ID: app, BlobsEnabled: true}, tok, "r")
	if e != nil {
		t.Fatal(e)
	}
	return a
}

type blockingRepo struct {
	started chan struct{}
	once    sync.Once
	release chan struct{}
	mu      sync.Mutex
	apps    []string
}

func (r *blockingRepo) Upload(_ context.Context, app, _ string, m Metadata, src io.Reader, _ Limits) (Metadata, error) {
	r.mu.Lock()
	r.apps = append(r.apps, app)
	r.mu.Unlock()
	r.once.Do(func() { close(r.started) })
	<-r.release
	_, e := io.ReadAll(src)
	return m, e
}
func (*blockingRepo) Open(context.Context, string, string) (io.ReadCloser, Metadata, error) {
	return nil, Metadata{}, nil
}
func (*blockingRepo) List(context.Context, string, string, int) (ListResult, error) {
	return ListResult{}, nil
}
func (*blockingRepo) Delete(context.Context, string, string) (bool, error) { return false, nil }
func (*blockingRepo) Reconcile(context.Context) error                      { return nil }
func TestUploadLimiterIsAppScoped(t *testing.T) {
	r := &blockingRepo{started: make(chan struct{}), release: make(chan struct{})}
	s := New(Limits{BlobBytes: 10, BlobsPerApp: 1, TotalBytesPerApp: 10, ListLimit: 1, UploadsPerMinute: 10, ConcurrentUploads: 1, UploadDuration: time.Second}, r)
	a, b := blobTestAuth(t, "a"), blobTestAuth(t, "b")
	done := make(chan error, 1)
	go func() {
		_, e := s.Upload(context.Background(), a, "x", "text/plain", strings.NewReader("x"))
		done <- e
	}()
	<-r.started
	if _, e := s.Upload(context.Background(), a, "x", "text/plain", strings.NewReader("x")); e != ErrRateLimited {
		t.Fatalf("same app=%v", e)
	}
	other := make(chan error, 1)
	go func() {
		_, e := s.Upload(context.Background(), b, "x", "text/plain", strings.NewReader("x"))
		other <- e
	}()
	time.Sleep(10 * time.Millisecond)
	close(r.release)
	<-done
	<-other
}
func TestUploadRateWindowUsesInjectedClockAndPrunesActiveState(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s := New(Limits{BlobBytes: 1, BlobsPerApp: 1, TotalBytesPerApp: 1, ListLimit: 1, UploadsPerMinute: 1, ConcurrentUploads: 1, UploadDuration: time.Second}, nil)
	s.now = func() time.Time { return now }
	if !s.acquire("a") {
		t.Fatal("first denied")
	}
	s.release("a")
	if _, ok := s.active["a"]; ok {
		t.Fatal("active state retained")
	}
	if s.acquire("a") {
		t.Fatal("rate limit missing")
	}
	now = now.Add(time.Minute)
	if !s.acquire("a") {
		t.Fatal("new rate window denied")
	}
}

func TestMetadataRejectsPathControlAndMalformedMIME(t *testing.T) {
	for _, name := range []string{".", "..", ".hidden", "C:evil.txt", "a\u202eb.txt", "a\n.txt", "a/b"} {
		if validMeta(Metadata{Name: name, ContentType: "text/plain"}) {
			t.Fatalf("accepted unsafe name %q", name)
		}
	}
	for _, typ := range []string{"text", "text/pla in", "text/plain; charset=", "text/plain\r\nX: y"} {
		if validMeta(Metadata{Name: "safe.txt", ContentType: typ}) {
			t.Fatalf("accepted unsafe type %q", typ)
		}
	}
	m, ok := normalizeMeta(Metadata{Name: "safe.txt", ContentType: "Text/Plain; charset=utf-8"})
	if !ok || m.ContentType != "text/plain" {
		t.Fatalf("MIME normalization failed: %#v ok=%t", m, ok)
	}
}

func TestUploadLimiterFailsClosedWhenRateBookIsFull(t *testing.T) {
	s := New(DefaultLimits(), nil)
	s.now = func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
	for i := 0; i < 1024; i++ {
		s.rates["app"+strconv.Itoa(i)] = rateState{since: s.now()}
	}
	if s.acquire("new-app") {
		t.Fatal("new app evicted a live rate bucket")
	}
}
