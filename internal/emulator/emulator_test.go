package emulator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func app(t *testing.T, state string) *Server {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>app</h1>"), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := New(context.Background(), Config{AppDir: dir, StateDir: state, Listen: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}
func request(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func TestRefusesPublicListenerAndTraversal(t *testing.T) {
	dir := t.TempDir()
	_, err := New(context.Background(), Config{AppDir: dir, Listen: "0.0.0.0:8787"})
	if err == nil || !strings.Contains(err.Error(), "non-loopback") {
		t.Fatalf("err=%v", err)
	}
	s := app(t, "")
	w := request(t, s.Handler(), "GET", "/../../etc/passwd", "")
	if w.Code == 200 && strings.Contains(w.Body.String(), "root:") {
		t.Fatal("traversal exposed file")
	}
	if err := os.Symlink("/etc/passwd", filepath.Join(s.AppDir(), "outside")); err == nil {
		w = request(t, s.Handler(), "GET", "/outside", "")
		if w.Code == 200 && strings.Contains(w.Body.String(), "root:") {
			t.Fatal("symlink exposed file")
		}
	}
}
func TestKVAndCollectionCRUDAndPersistence(t *testing.T) {
	state := t.TempDir()
	s := app(t, state)
	h := s.Handler()
	w := request(t, h, "PUT", "/_tinker/api/v1/kv/x", `{"value":{"ok":true}}`)
	if w.Code != 200 {
		t.Fatalf("set: %d %s", w.Code, w.Body.String())
	}
	w = request(t, h, "GET", "/_tinker/api/v1/kv/x", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"ok":true`) {
		t.Fatalf("get: %d %s", w.Code, w.Body.String())
	}
	w = request(t, h, "PUT", "/_tinker/api/v1/kv/x", `{"value":1,"expected_version":99}`)
	if w.Code != 409 {
		t.Fatalf("conflict: %d", w.Code)
	}
	w = request(t, h, "POST", "/_tinker/api/v1/db/tasks", `{"data":{"title":"test"}}`)
	if w.Code != 201 || !strings.Contains(w.Body.String(), `"id":"doc_`) || !strings.Contains(w.Body.String(), "created_at") {
		t.Fatalf("collection create: %d %s", w.Code, w.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	w = request(t, h, "GET", "/_tinker/api/v1/db/tasks?snapshot=1", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"revision":1`) {
		t.Fatalf("snapshot: %d %s", w.Code, w.Body.String())
	}
	_ = s.Close()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	next, err := New(context.Background(), Config{AppDir: dir, StateDir: state, Listen: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	w = request(t, next.Handler(), "GET", "/_tinker/api/v1/db/tasks/"+created.ID, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "test") {
		t.Fatalf("persist: %d %s", w.Code, w.Body.String())
	}
}
func TestAPIIdentityAndUnknownMethodDenial(t *testing.T) {
	s := app(t, "")
	w := request(t, s.Handler(), "GET", "/_tinker/api/v1/me", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "local-viewer") {
		t.Fatalf("identity: %d %s", w.Code, w.Body.String())
	}
	w = request(t, s.Handler(), "POST", "/_tinker/api/v1/me", "")
	if w.Code != 404 {
		t.Fatalf("method: %d", w.Code)
	}
	w = request(t, s.Handler(), "PUT", "/_tinker/api/v1/kv/a%2Fb", `{"value":1}`)
	if w.Code != 400 {
		t.Fatalf("escaped slash: %d", w.Code)
	}
}
func TestLiveKVNotification(t *testing.T) {
	s := app(t, "")
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	ws := "ws" + strings.TrimPrefix(ts.URL, "http") + "/_tinker/ws/v1"
	c, _, err := websocket.Dial(context.Background(), ws, &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {ts.URL}}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	if err = c.Write(context.Background(), websocket.MessageText, []byte(`{"v":1,"type":"subscribe_kv","prefix":"x"}`)); err != nil {
		t.Fatal(err)
	}
	waitForEmulatorSubscription(t, s, "kv:x")
	w := request(t, s.Handler(), "PUT", "/_tinker/api/v1/kv/x", `{"value":1}`)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, raw, err := c.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil || m["type"] != "kv.changed" {
		t.Fatalf("frame %s", raw)
	}
}
func TestCollectionWebSocketCompatibility(t *testing.T) {
	s := app(t, "")
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	ws := "ws" + strings.TrimPrefix(ts.URL, "http") + "/_tinker/ws/v1"
	c, _, e := websocket.Dial(context.Background(), ws, &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {ts.URL}}})
	if e != nil {
		t.Fatal(e)
	}
	defer c.CloseNow()
	if e = c.Write(context.Background(), websocket.MessageText, []byte(`{"v":1,"type":"subscribe_collection","collection":"tasks"}`)); e != nil {
		t.Fatal(e)
	}
	waitForEmulatorSubscription(t, s, "collection:tasks")
	w := request(t, s.Handler(), "POST", "/_tinker/api/v1/db/tasks", `{"data":{"title":"test"}}`)
	if w.Code != 201 {
		t.Fatal(w.Code)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, raw, e := c.Read(ctx)
	if e != nil {
		t.Fatal(e)
	}
	var event struct {
		Type, Collection, ID string
		Version, Revision    int
	}
	if json.Unmarshal(raw, &event) != nil || event.Type != "collection.changed" || event.Collection != "tasks" || !strings.HasPrefix(event.ID, "doc_") || event.Version != 1 || event.Revision != 1 {
		t.Fatalf("event %s", raw)
	}
}

// WebSocket writes complete after the client has handed a frame to its
// transport, not after the server's handler has applied it. Mutating state
// immediately after a subscribe is therefore a test race, not a delivery
// guarantee. Wait for the emulator's server-side subscription state so this
// test verifies the actual live-notification contract deterministically.
func waitForEmulatorSubscription(t *testing.T, s *Server, key string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		s.mu.Lock()
		found := false
		for _, subscriptions := range s.clients {
			_, found = subscriptions[key]
			if found {
				break
			}
		}
		s.mu.Unlock()
		if found {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("server did not apply websocket subscription %q", key)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestWebSocketRejectsNonLoopbackOrigin(t *testing.T) {
	s := app(t, "")
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	ws := "ws" + strings.TrimPrefix(ts.URL, "http") + "/_tinker/ws/v1"
	_, response, err := websocket.Dial(context.Background(), ws, &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {"https://example.test"}}})
	if err == nil || response == nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("non-loopback websocket origin accepted: response=%#v err=%v", response, err)
	}
}

func TestWebSocketBoundsFramesAndSubscriptions(t *testing.T) {
	s := app(t, "")
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	dial := func(t *testing.T) *websocket.Conn {
		t.Helper()
		ws := "ws" + strings.TrimPrefix(ts.URL, "http") + "/_tinker/ws/v1"
		c, _, err := websocket.Dial(context.Background(), ws, &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {ts.URL}}})
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	t.Run("oversized frame", func(t *testing.T) {
		c := dial(t)
		defer c.CloseNow()
		if err := c.Write(context.Background(), websocket.MessageText, []byte(strings.Repeat("x", emulatorMaxFrame+1))); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if _, _, err := c.Read(ctx); err == nil {
			t.Fatal("oversized websocket frame remained usable")
		}
	})
	t.Run("subscriptions", func(t *testing.T) {
		c := dial(t)
		defer c.CloseNow()
		for i := 0; i <= emulatorMaxSubscriptions; i++ {
			frame := fmt.Sprintf(`{"v":1,"type":"subscribe","channel":"c%d"}`, i)
			if err := c.Write(context.Background(), websocket.MessageText, []byte(frame)); err != nil {
				t.Fatal(err)
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if _, _, err := c.Read(ctx); err == nil {
			t.Fatal("unbounded subscriptions remained usable")
		}
	})
}

func TestCollectionWritesAreAtomicAndFailedWritesDoNotPublish(t *testing.T) {
	s := app(t, "")
	h := s.Handler()
	w := request(t, h, "POST", "/_tinker/api/v1/db/tasks", `{"data":{"title":"first"}}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil || !validDocumentID(created.ID) {
		t.Fatalf("invalid generated id %q: %v", created.ID, err)
	}

	ts := httptest.NewServer(h)
	defer ts.Close()
	ws := "ws" + strings.TrimPrefix(ts.URL, "http") + "/_tinker/ws/v1"
	c, _, err := websocket.Dial(context.Background(), ws, &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {ts.URL}}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	if err = c.Write(context.Background(), websocket.MessageText, []byte(`{"v":1,"type":"subscribe_collection","collection":"tasks"}`)); err != nil {
		t.Fatal(err)
	}
	waitForEmulatorSubscription(t, s, "collection:tasks")

	// Both updates deliberately use the same stale version. SQLite serializes
	// the complete read/check/write/revision transaction: exactly one wins.
	start := make(chan struct{})
	codes := make(chan int, 2)
	var wg sync.WaitGroup
	for _, title := range []string{"left", "right"} {
		wg.Add(1)
		go func(title string) {
			defer wg.Done()
			<-start
			response := request(t, h, http.MethodPut, "/_tinker/api/v1/db/tasks/"+created.ID, `{"data":{"title":"`+title+`"},"expected_version":1}`)
			codes <- response.Code
		}(title)
	}
	close(start)
	wg.Wait()
	close(codes)
	var ok, conflict int
	for code := range codes {
		switch code {
		case http.StatusOK:
			ok++
		case http.StatusConflict:
			conflict++
		default:
			t.Fatalf("unexpected concurrent update code %d", code)
		}
	}
	if ok != 1 || conflict != 1 {
		t.Fatalf("winners=%d conflicts=%d", ok, conflict)
	}

	// The successful mutation sends one freshness hint. A later rejected stale
	// write must not alter persistent state or produce another event.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, raw, err := c.Read(ctx)
	if err != nil || !strings.Contains(string(raw), `"version":2`) {
		t.Fatalf("successful change event: %v %s", err, raw)
	}
	w = request(t, h, http.MethodPut, "/_tinker/api/v1/db/tasks/"+created.ID, `{"data":{"title":"stale"},"expected_version":1}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("stale write: %d %s", w.Code, w.Body.String())
	}
	noEvent, stop := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer stop()
	if _, raw, err = c.Read(noEvent); err == nil {
		t.Fatalf("failed write published event %s", raw)
	}
}

func TestMutationDecoderRejectsExtraJSONAndInvalidDocumentID(t *testing.T) {
	s := app(t, "")
	h := s.Handler()
	for _, path := range []string{
		"/_tinker/api/v1/db/tasks/not-a-server-issued-id",
		"/_tinker/api/v1/db/tasks/doc_abcdefghijklmnopqrstuvw", // 23 suffix bytes
	} {
		if w := request(t, h, http.MethodGet, path, ""); w.Code != http.StatusBadRequest {
			t.Fatalf("id %s: %d", path, w.Code)
		}
	}
	if w := request(t, h, http.MethodPut, "/_tinker/api/v1/kv/strict", `{"value":1}{"value":2}`); w.Code != http.StatusBadRequest {
		t.Fatalf("multi-value kv body: %d %s", w.Code, w.Body.String())
	}
	if w := request(t, h, http.MethodPost, "/_tinker/api/v1/db/tasks", `{"data":{"title":"x"}} {}`); w.Code != http.StatusBadRequest {
		t.Fatalf("multi-value document body: %d %s", w.Code, w.Body.String())
	}
}

func TestKVConcurrentStaleVersionHasOneWinner(t *testing.T) {
	s := app(t, "")
	h := s.Handler()
	if w := request(t, h, http.MethodPut, "/_tinker/api/v1/kv/counter", `{"value":0}`); w.Code != http.StatusOK {
		t.Fatalf("seed: %d %s", w.Code, w.Body.String())
	}
	start := make(chan struct{})
	codes := make(chan int, 2)
	var wg sync.WaitGroup
	for _, value := range []int{1, 2} {
		wg.Add(1)
		go func(value int) {
			defer wg.Done()
			<-start
			w := request(t, h, http.MethodPut, "/_tinker/api/v1/kv/counter", fmt.Sprintf(`{"value":%d,"expected_version":1}`, value))
			codes <- w.Code
		}(value)
	}
	close(start)
	wg.Wait()
	close(codes)
	var ok, conflict int
	for code := range codes {
		if code == http.StatusOK {
			ok++
		} else if code == http.StatusConflict {
			conflict++
		} else {
			t.Fatalf("unexpected kv concurrent update code %d", code)
		}
	}
	if ok != 1 || conflict != 1 {
		t.Fatalf("kv winners=%d conflicts=%d", ok, conflict)
	}
}
func TestStaticSPA(t *testing.T) {
	s := app(t, "")
	w := request(t, s.Handler(), "GET", "/client/route", "")
	if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte("app")) {
		t.Fatalf("spa: %d %s", w.Code, w.Body.String())
	}
	w = request(t, s.Handler(), "POST", "/client/route", "")
	if w.Code != 405 {
		t.Fatalf("static method: %d", w.Code)
	}
	_, _ = io.Copy(io.Discard, w.Result().Body)
}
