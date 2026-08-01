package gateway_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ChrisMarxDev/tinkercloud/internal/analytics"
	"github.com/ChrisMarxDev/tinkercloud/internal/appauth"
	"github.com/ChrisMarxDev/tinkercloud/internal/apps"
	"github.com/ChrisMarxDev/tinkercloud/internal/config"
	"github.com/ChrisMarxDev/tinkercloud/internal/gateway"
	"github.com/ChrisMarxDev/tinkercloud/internal/identity"
	"github.com/ChrisMarxDev/tinkercloud/internal/policies"
	"github.com/ChrisMarxDev/tinkercloud/internal/releases"
	"github.com/ChrisMarxDev/tinkercloud/internal/sessions"
)

type insightSink struct {
	events chan analytics.Event
	err    error
}

func (s insightSink) RecordInsight(_ context.Context, e analytics.Event) error {
	if s.err != nil {
		return s.err
	}
	s.events <- e
	return nil
}

func TestGatewayCountsOnlySuccessfulHTMLAndIssuesHostOnlyMarker(t *testing.T) {
	sink := insightSink{events: make(chan analytics.Event, 2)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	recorder := analytics.NewRecorder(sink, 4)
	recorder.Start(ctx)
	h, token := analyticsGateway(t, recorder)

	first := httptest.NewRequest(http.MethodGet, "https://alpha.apps.tinker.test/", nil)
	first.Host = "alpha.apps.tinker.test"
	first.AddCookie(&http.Cookie{Name: sessions.AppCookieName, Value: token})
	first.Header.Set("Accept", "text/html")
	first.Header.Set("Sec-Fetch-Dest", "document")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, first)
	if w.Code != http.StatusOK || w.Body.String() != "<h1>private</h1>" {
		t.Fatalf("document response changed: %d %q", w.Code, w.Body.String())
	}
	cookie := w.Result().Cookies()
	if len(cookie) != 1 || cookie[0].Name != analytics.CookieName || !cookie[0].Secure || !cookie[0].HttpOnly || cookie[0].Domain != "" || cookie[0].Path != "/" {
		t.Fatalf("unsafe insights cookie: %#v", cookie)
	}
	select {
	case event := <-sink.events:
		if event.HasMarker || event.AppID != "app" {
			t.Fatalf("first event=%+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("document event was not recorded")
	}

	second := httptest.NewRequest(http.MethodGet, "https://alpha.apps.tinker.test/", nil)
	second.Host = "alpha.apps.tinker.test"
	second.AddCookie(&http.Cookie{Name: sessions.AppCookieName, Value: token})
	second.AddCookie(cookie[0])
	second.Header.Set("Accept", "text/html")
	second.Header.Set("Sec-Fetch-Dest", "document")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, second)
	select {
	case event := <-sink.events:
		if !event.HasMarker {
			t.Fatal("valid marker was not digested")
		}
	case <-time.After(time.Second):
		t.Fatal("returning document event was not recorded")
	}

	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "https://alpha.apps.tinker.test/app.js", nil),
		httptest.NewRequest(http.MethodHead, "https://alpha.apps.tinker.test/", nil),
		httptest.NewRequest(http.MethodGet, "https://alpha.apps.tinker.test/", nil),
	} {
		request.Host = "alpha.apps.tinker.test"
		request.AddCookie(&http.Cookie{Name: sessions.AppCookieName, Value: token})
		if request.URL.Path == "/" && request.Method == http.MethodGet {
			request.Header.Set("Range", "bytes=0-1")
		}
		h.ServeHTTP(httptest.NewRecorder(), request)
	}
	select {
	case event := <-sink.events:
		t.Fatalf("non-document counted: %+v", event)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestGatewayAnalyticsFailurePreservesStaticStatusAndBody(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	recorder := analytics.NewRecorder(insightSink{err: errors.New("database failed")}, 1)
	recorder.Start(ctx)
	h, token := analyticsGateway(t, recorder)
	r := httptest.NewRequest(http.MethodGet, "https://alpha.apps.tinker.test/", nil)
	r.Host = "alpha.apps.tinker.test"
	r.AddCookie(&http.Cookie{Name: sessions.AppCookieName, Value: token})
	r.Header.Set("Accept", "text/html")
	r.Header.Set("Sec-Fetch-Dest", "document")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK || w.Body.String() != "<h1>private</h1>" {
		t.Fatalf("analytics failure changed static response: %d %q", w.Code, w.Body.String())
	}
}

func TestGatewayDisabledInsightsDoesNotIssueCookie(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	recorder := analytics.NewRecorder(insightSink{events: make(chan analytics.Event)}, 1)
	recorder.SetEnabled(false)
	recorder.Start(ctx)
	h, token := analyticsGateway(t, recorder)
	r := httptest.NewRequest(http.MethodGet, "https://alpha.apps.tinker.test/", nil)
	r.Host = "alpha.apps.tinker.test"
	r.AddCookie(&http.Cookie{Name: sessions.AppCookieName, Value: token})
	r.Header.Set("Accept", "text/html")
	r.Header.Set("Sec-Fetch-Dest", "document")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK || len(w.Result().Cookies()) != 0 {
		t.Fatalf("disabled analytics wrote cookie/status=%d cookies=%v", w.Code, w.Result().Cookies())
	}
}

func TestGatewayConditionalDocumentDoesNotIssueCookieOrCount(t *testing.T) {
	sink := insightSink{events: make(chan analytics.Event, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	recorder := analytics.NewRecorder(sink, 1)
	recorder.Start(ctx)
	h, token := analyticsGateway(t, recorder)
	r := httptest.NewRequest(http.MethodGet, "https://alpha.apps.tinker.test/", nil)
	r.Host = "alpha.apps.tinker.test"
	r.AddCookie(&http.Cookie{Name: sessions.AppCookieName, Value: token})
	r.Header.Set("Accept", "text/html")
	r.Header.Set("Sec-Fetch-Dest", "document")
	r.Header.Set("If-None-Match", `W/"14-0"`)
	// A current ETag is added by staticruntime; repeat with that value after a
	// normal response so the test does not depend on filesystem timestamps.
	normal := httptest.NewRecorder()
	normalRequest := r.Clone(context.Background())
	normalRequest.Header.Del("If-None-Match")
	h.ServeHTTP(normal, normalRequest)
	<-sink.events
	r.Header.Set("If-None-Match", normal.Header().Get("ETag"))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusNotModified || len(w.Result().Cookies()) != 0 {
		t.Fatalf("conditional response status=%d cookies=%v", w.Code, w.Result().Cookies())
	}
	select {
	case event := <-sink.events:
		t.Fatalf("304 counted: %+v", event)
	case <-time.After(50 * time.Millisecond):
	}
}

func analyticsGateway(t *testing.T, recorder *analytics.Recorder) (gateway.Gateway, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("<h1>private</h1>"), 0600); err != nil {
		t.Fatal(err)
	}
	evidence, err := releases.Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	viewer := identity.Identity{ID: "viewer", Email: "viewer@example.com"}
	store := sessions.NewMemoryStore()
	token, _, err := store.Create("app", viewer, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	app := apps.App{ID: "app", Slug: "alpha", Status: apps.Active, ReleaseRoot: root, ReleaseEvidence: evidence}
	return gateway.Gateway{Config: config.Config{Domain: "apps.tinker.test", SessionCookie: sessions.AppCookieName}, Apps: apps.NewMemoryRepository(app), Authorizer: appauth.Authorizer{Sessions: store, Policies: &policies.MemoryStore{Policies: map[string]policies.Policy{"app": {AppID: "app", Valid: true, Emails: map[string]struct{}{viewer.Email: {}}}}}}, Insights: recorder, InsightsKey: []byte("analytics-test-key")}, token
}
