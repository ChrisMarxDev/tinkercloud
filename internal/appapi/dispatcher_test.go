package appapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ChrisMarxDev/tinkercloud/internal/appauth"
	"github.com/ChrisMarxDev/tinkercloud/internal/apps"
	"github.com/ChrisMarxDev/tinkercloud/internal/collections"
	"github.com/ChrisMarxDev/tinkercloud/internal/identity"
	"github.com/ChrisMarxDev/tinkercloud/internal/kv"
	"github.com/ChrisMarxDev/tinkercloud/internal/llm"
	"github.com/ChrisMarxDev/tinkercloud/internal/policies"
	"github.com/ChrisMarxDev/tinkercloud/internal/sessions"
)

func authFor(t *testing.T, appID string) appauth.AuthorizationContext {
	return authForKV(t, appID, true)
}

func authForKV(t *testing.T, appID string, enabled bool) appauth.AuthorizationContext {
	t.Helper()
	v := identity.Identity{ID: "viewer", Email: "v@example.com"}
	s := sessions.NewMemoryStore()
	token, _, _ := s.Create(appID, v, time.Now().Add(time.Hour))
	a, e := appauth.Authorizer{Sessions: s, Policies: &policies.MemoryStore{Policies: map[string]policies.Policy{appID: {AppID: appID, OwnerIdentityID: "viewer", Valid: true}}}}.Authorize(context.Background(), apps.App{ID: appID, KVEnabled: enabled, RealtimeEnabled: true}, token, "req_safe")
	if e != nil {
		t.Fatal(e)
	}
	return a
}
func authForLLM(t *testing.T, appID string) appauth.AuthorizationContext {
	t.Helper()
	v := identity.Identity{ID: "viewer", Email: "v@example.com"}
	s := sessions.NewMemoryStore()
	token, _, _ := s.Create(appID, v, time.Now().Add(time.Hour))
	a, e := appauth.Authorizer{Sessions: s, Policies: &policies.MemoryStore{Policies: map[string]policies.Policy{appID: {AppID: appID, OwnerIdentityID: "viewer", Valid: true}}}}.Authorize(context.Background(), apps.App{ID: appID, LLMChatRequested: true}, token, "req_safe")
	if e != nil {
		t.Fatal(e)
	}
	return a
}

type llmRepoSpy struct{ calls int }

func (s *llmRepoSpy) AdmissionLimits(context.Context, string) (llm.Limits, error) {
	return llm.Limits{MaxMessages: 4, MaxMessageBytes: 100, MaxInputBytes: 200, MaxOutputTokens: 10, Timeout: time.Second, ViewerRequests: 10, AppRequests: 10, RateWindow: time.Minute}, nil
}
func (s *llmRepoSpy) Admit(_ context.Context, app, viewer string, input, output int, _ time.Time) (llm.Binding, error) {
	s.calls++
	return llm.Binding{AppID: app, ProfileID: "p", ConnectionID: "c", Credential: []byte("secret"), Provider: llm.ProviderAnthropic, Model: "m", ReservationID: "r", ReservedTokens: input + 10, Limits: llm.Limits{MaxMessages: 4, MaxMessageBytes: 100, MaxInputBytes: 200, MaxOutputTokens: 10, Timeout: time.Second, ViewerRequests: 10, AppRequests: 10, RateWindow: time.Minute}}, nil
}
func (s *llmRepoSpy) Reconcile(context.Context, llm.Binding, llm.Usage, llm.Outcome, time.Time) error {
	return nil
}

type llmAdapterSpy struct{}

func (llmAdapterSpy) Complete(context.Context, llm.Binding, llm.Request) (llm.Response, error) {
	return llm.Response{Message: llm.MessageResponse{Role: "assistant", Content: "ok"}, Usage: llm.Usage{InputTokens: 1, OutputTokens: 2}, FinishReason: "stop"}, nil
}
func TestLLMChatWireContractAndStrictJSON(t *testing.T) {
	repo := &llmRepoSpy{}
	s := llm.New(repo, map[llm.Provider]llm.Adapter{llm.ProviderAnthropic: llmAdapterSpy{}})
	d := Dispatcher{LLM: s, Origin: func(*http.Request) bool { return true }}
	a := authForLLM(t, "a")
	w := request(d, a, http.MethodPost, "/_tinker/api/v1/llm/chat", `{"messages":[{"role":"user","content":"hi"}],"max_output_tokens":3}`, true)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"input_tokens":1`) || strings.Contains(w.Body.String(), "InputTokens") {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request(d, a, http.MethodPost, "/_tinker/api/v1/llm/chat", `{"messages":[],"messages":[]}`, true)
	if w.Code != 400 {
		t.Fatal(w.Code, w.Body.String())
	}
	raw := []byte(`{"messages":[{"role":"user","content":"`)
	raw = append(raw, 0xff)
	raw = append(raw, []byte(`"}]}`)...)
	r := httptest.NewRequest(http.MethodPost, "/_tinker/api/v1/llm/chat", bytes.NewReader(raw))
	r.Host = "example.com"
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "http://example.com")
	w = httptest.NewRecorder()
	d.Dispatch(a, w, r)
	if w.Code != http.StatusBadRequest || repo.calls != 1 {
		t.Fatalf("invalid utf8 status=%d durable admissions=%d body=%s", w.Code, repo.calls, w.Body.String())
	}
}

func TestLLMDiscoveryExposesOnlyEffectiveSafeLimits(t *testing.T) {
	d := Dispatcher{Capabilities: []Capability{{Name: "llm.chat", Version: 1, Limits: map[string]int{"model": 99}}}, LLM: llm.New(&llmRepoSpy{}, nil)}
	w := request(d, authForLLM(t, "a"), http.MethodGet, "/_tinker/api/v1/capabilities", "", false)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"disclosure"`) || !strings.Contains(w.Body.String(), `"max_output_tokens":10`) || strings.Contains(w.Body.String(), `"model":99`) {
		t.Fatalf("discovery=%d %s", w.Code, w.Body.String())
	}
	w = request(d, authFor(t, "a"), http.MethodGet, "/_tinker/api/v1/capabilities", "", false)
	if strings.Contains(w.Body.String(), "llm.chat") {
		t.Fatalf("unrequested llm leaked into discovery: %s", w.Body.String())
	}
}

type listSpy struct{ calls int }

func (s *listSpy) Get(context.Context, appauth.AuthorizationContext, string) (*kv.Entry, error) {
	s.calls++
	return nil, nil
}

type collectionSpy struct {
	calls int
	doc   collections.Document
}

func (s *collectionSpy) Create(_ context.Context, _ appauth.AuthorizationContext, collection string, data json.RawMessage) (collections.Document, collections.Mutation, error) {
	s.calls++
	s.doc = collections.Document{ID: "doc_abcdefghijklmnopqrstuv", Data: data, Version: 1, CreatedAt: time.Unix(1, 0).UTC(), UpdatedAt: time.Unix(1, 0).UTC()}
	return s.doc, collections.Mutation{Collection: collection, ID: s.doc.ID, Version: 1, Revision: 1}, nil
}
func (s *collectionSpy) Get(context.Context, appauth.AuthorizationContext, string, string) (*collections.Document, error) {
	s.calls++
	if s.doc.ID == "" {
		return nil, nil
	}
	doc := s.doc
	return &doc, nil
}
func (s *collectionSpy) Update(_ context.Context, _ appauth.AuthorizationContext, collection, id string, data json.RawMessage, expected *uint64) (collections.Document, collections.Mutation, error) {
	s.calls++
	if expected != nil && *expected != s.doc.Version {
		return collections.Document{}, collections.Mutation{}, collections.ErrVersionConflict
	}
	s.doc.Data, s.doc.Version, s.doc.UpdatedAt = data, s.doc.Version+1, time.Unix(2, 0).UTC()
	return s.doc, collections.Mutation{Collection: collection, ID: id, Version: s.doc.Version, Revision: 2}, nil
}
func (s *collectionSpy) Delete(_ context.Context, _ appauth.AuthorizationContext, collection, id string, expected *uint64) (bool, collections.Mutation, error) {
	s.calls++
	if expected != nil && *expected != s.doc.Version {
		return false, collections.Mutation{}, collections.ErrVersionConflict
	}
	s.doc = collections.Document{}
	return true, collections.Mutation{Collection: collection, ID: id, Version: 3, Deleted: true, Revision: 3}, nil
}
func (s *collectionSpy) List(context.Context, appauth.AuthorizationContext, string, string, int) (collections.ListResult, error) {
	s.calls++
	if s.doc.ID == "" {
		return collections.ListResult{Documents: []collections.Document{}, Revision: 0}, nil
	}
	return collections.ListResult{Documents: []collections.Document{s.doc}, Revision: s.doc.Version}, nil
}
func (s *collectionSpy) Snapshot(context.Context, appauth.AuthorizationContext, string) (collections.Snapshot, error) {
	s.calls++
	if s.doc.ID == "" {
		return collections.Snapshot{Documents: []collections.Document{}}, nil
	}
	return collections.Snapshot{Collection: "tasks", Documents: []collections.Document{s.doc}, Revision: s.doc.Version}, nil
}
func (s *listSpy) Set(context.Context, appauth.AuthorizationContext, string, json.RawMessage, *uint64) (kv.Entry, error) {
	s.calls++
	return kv.Entry{}, nil
}
func (s *listSpy) Delete(context.Context, appauth.AuthorizationContext, string, *uint64) (bool, error) {
	s.calls++
	return false, nil
}
func (s *listSpy) List(context.Context, appauth.AuthorizationContext, string, string, int) (kv.ListResult, error) {
	s.calls++
	return kv.ListResult{}, nil
}
func request(d Dispatcher, a appauth.AuthorizationContext, method, path, body string, contentType bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Host = "example.com"
	if contentType {
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", "http://"+r.Host)
	}
	w := httptest.NewRecorder()
	d.Dispatch(a, w, r)
	return w
}
func TestTwoAppIsolationVersionAndMalformedRequest(t *testing.T) {
	d := Dispatcher{KV: kv.New(kv.DefaultLimits(), nil), AppSlug: func(a appauth.AuthorizationContext) string { return "app-" + a.AppID() }}
	a, b := authFor(t, "a"), authFor(t, "b")
	w := request(d, a, http.MethodPut, "/_tinker/api/v1/kv/state%2Fx", `{"value":{"ok":true}}`, true)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request(d, b, http.MethodGet, "/_tinker/api/v1/kv/state%2Fx", "", false)
	if w.Code != 404 {
		t.Fatal("cross-app isolation", w.Code)
	}
	w = request(d, a, http.MethodPut, "/_tinker/api/v1/kv/state%2Fx", `{"value":1,"expected_version":9}`, true)
	if w.Code != 409 || !strings.Contains(w.Body.String(), "req_safe") {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request(d, a, http.MethodPut, "/_tinker/api/v1/kv/x", `{"unknown":1}`, true)
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
	w = request(d, a, http.MethodPut, "/_tinker/api/v1/kv/x", `{"value":1}`, false)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
}
func TestCurrentIdentityAndAppAreServerDerived(t *testing.T) {
	d := Dispatcher{KV: kv.New(kv.DefaultLimits(), nil), AppSlug: func(appauth.AuthorizationContext) string { return "demo" }}
	w := request(d, authFor(t, "a"), http.MethodGet, "/_tinker/api/v1/me", "", false)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "demo") || strings.Contains(w.Body.String(), "app_id") {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request(d, nil, http.MethodGet, "/_tinker/api/v1/me", "", false)
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
}

func TestDisabledKVListDeniedBeforeDispatcher(t *testing.T) {
	spy := &listSpy{}
	d := Dispatcher{KV: spy}
	w := request(d, authForKV(t, "a", false), http.MethodGet, "/_tinker/api/v1/kv?prefix=state", "", false)
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "capability_unavailable") {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if spy.calls != 0 {
		t.Fatalf("disabled list reached dispatcher capability %d times", spy.calls)
	}
}

func TestCollectionRoutesAreProtectedStrictAndVersioned(t *testing.T) {
	spy := &collectionSpy{}
	d := Dispatcher{Collections: spy}
	a := authFor(t, "a")
	w := request(d, a, http.MethodPost, "/_tinker/api/v1/db/tasks", `{"data":{"title":"one"}}`, true)
	if w.Code != http.StatusCreated || !strings.Contains(w.Body.String(), `"doc_abcdefghijklmnopqrstuv"`) {
		t.Fatalf("create=%d %s", w.Code, w.Body.String())
	}
	w = request(d, a, http.MethodGet, "/_tinker/api/v1/db/tasks?cursor=x", "", false)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid cursor=%d %s", w.Code, w.Body.String())
	}
	w = request(d, a, http.MethodGet, "/_tinker/api/v1/db/Tasks", "", false)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid collection=%d %s", w.Code, w.Body.String())
	}
	w = request(d, a, http.MethodPut, "/_tinker/api/v1/db/tasks/doc_abcdefghijklmnopqrstuv", `{"data":{"title":"two"},"expected_version":9}`, true)
	if w.Code != http.StatusConflict {
		t.Fatalf("stale write=%d %s", w.Code, w.Body.String())
	}
	w = request(d, a, http.MethodGet, "/_tinker/api/v1/db/tasks?snapshot=1", "", false)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"documents"`) {
		t.Fatalf("snapshot=%d %s", w.Code, w.Body.String())
	}
	w = request(d, a, http.MethodPost, "/_tinker/api/v1/db/tasks", `{"data":{}}`, false)
	if w.Code != http.StatusForbidden {
		t.Fatalf("origin denial=%d", w.Code)
	}
}

func TestDisabledCollectionDoesNotReachRepository(t *testing.T) {
	spy := &collectionSpy{}
	d := Dispatcher{Collections: spy}
	w := request(d, authForKV(t, "a", false), http.MethodGet, "/_tinker/api/v1/db/tasks", "", false)
	if w.Code != http.StatusForbidden || spy.calls != 0 {
		t.Fatalf("status=%d calls=%d body=%s", w.Code, spy.calls, w.Body.String())
	}
}

// An empty page is a successful collection response, not a nullable value.
// SDK clients must be able to iterate entries without special-casing it.
func TestKVListEmptyPageSerializesEntriesAsArray(t *testing.T) {
	d := Dispatcher{KV: kv.New(kv.DefaultLimits(), nil)}
	w := request(d, authFor(t, "a"), http.MethodGet, "/_tinker/api/v1/kv?prefix=missing/", "", false)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var response struct {
		Entries    []kv.Entry `json:"entries"`
		NextCursor string     `json:"next_cursor"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Entries == nil || len(response.Entries) != 0 {
		t.Fatalf("entries must be an empty array, got %#v; body=%s", response.Entries, w.Body.String())
	}
	if response.NextCursor != "" {
		t.Fatalf("empty page cursor=%q", response.NextCursor)
	}
	if strings.Contains(w.Body.String(), `"entries":null`) {
		t.Fatalf("empty page serialized null entries: %s", w.Body.String())
	}
}

func TestSDKVersionNegotiation(t *testing.T) {
	d := Dispatcher{AppSlug: func(appauth.AuthorizationContext) string { return "alpha" }}
	a := authFor(t, "a")
	for _, version := range []string{"", "0.1.0", "0.999.12"} {
		r := httptest.NewRequest(http.MethodGet, "/_tinker/api/v1/me", nil)
		r.Header.Set("X-Tinker-SDK-Version", version)
		w := httptest.NewRecorder()
		d.Dispatch(a, w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("version %q: got %d", version, w.Code)
		}
	}
	for _, version := range []string{"1.0.0", "not-semver", "0.1", "0.1.0-beta"} {
		r := httptest.NewRequest(http.MethodGet, "/_tinker/api/v1/me", nil)
		r.Header.Set("X-Tinker-SDK-Version", version)
		w := httptest.NewRecorder()
		d.Dispatch(a, w, r)
		if w.Code != http.StatusUpgradeRequired || !strings.Contains(w.Body.String(), "sdk_version_incompatible") {
			t.Fatalf("version %q: got %d %s", version, w.Code, w.Body.String())
		}
	}
}
