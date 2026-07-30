package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tinyhost/tiny/internal/compose"
	"github.com/tinyhost/tiny/internal/config"
	"github.com/tinyhost/tiny/internal/identity"
	"github.com/tinyhost/tiny/internal/live"
	"github.com/tinyhost/tiny/internal/llm"
	"github.com/tinyhost/tiny/internal/persistence"
	"github.com/tinyhost/tiny/internal/releases"
	"github.com/tinyhost/tiny/internal/sessions"
)

// TestLLMGatewaySQLiteTwoAppBoundary exercises the complete protected path:
// gateway host resolution, opaque app session, SQLite policy, encrypted LLM
// repository, and a local fake adapter. The adapter proves no external network
// or caller-supplied tenancy can bypass the server-derived app scope.
func TestLLMGatewaySQLiteTwoAppBoundary(t *testing.T) {
	ctx := context.Background()
	store, err := persistence.OpenSQLite(ctx, filepath.Join(t.TempDir(), "tiny.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	seedLLMGatewayApp(t, store, "app-a", "alpha", true)
	seedLLMGatewayApp(t, store, "app-b", "beta", true)
	viewer := identity.Identity{ID: "viewer", Email: "viewer@example.com"}
	if _, err := store.DB.Exec("INSERT INTO identities(id,normalized_email,created_at) VALUES(?,?,datetime('now'))", viewer.ID, viewer.Email); err != nil {
		t.Fatal(err)
	}
	aToken, _, err := store.CreateAppSession(ctx, "app-a", viewer, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	bToken, _, err := store.CreateAppSession(ctx, "app-b", viewer, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := llm.NewAESGCMEnvelope(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	repo := persistence.LLMRepository{Store: store, Envelope: envelope}
	limits := llm.Limits{MaxMessages: 4, MaxMessageBytes: 100, MaxInputBytes: 200, MaxOutputTokens: 10, Timeout: time.Second, ViewerRequests: 10, AppRequests: 10, RateWindow: time.Minute}
	if err := repo.CreateConnection(ctx, persistence.LLMConnectionInput{ID: "connection", DisplayName: "test connection", Provider: llm.ProviderAnthropic, Secret: []byte("provider-secret"), KeyVersion: 1, ActorID: "owner-app-a"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateProfile(ctx, persistence.LLMProfileInput{ID: "profile", ConnectionID: "connection", Model: "hidden-test-model", Limits: limits, ConcurrencyLimit: 1, MonthlyTokenLimit: 100, ActorID: "owner-app-a"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateGrant(ctx, persistence.LLMGrantInput{AppID: "app-a", ProfileID: "profile", OperatorID: "owner-app-a", Status: "approved"}, 0); err != nil {
		t.Fatal(err)
	}
	adapter := &llmGatewayAdapter{}
	service := llm.New(repo, map[llm.Provider]llm.Adapter{llm.ProviderAnthropic: adapter})
	h := compose.AppPlaneWithPlatformAndBlobsCollectionsAndLLM(
		config.Config{PlatformHost: "tiny.test", AppSuffix: "apps.tiny.test", SessionCookie: sessions.AppCookieName, ListenHTTPS: ":443"},
		store, store, store, nil, nil, nil, live.New(live.DefaultLimits()), compose.Login{}, nil, service,
	)
	server := httptest.NewServer(h)
	defer server.Close()

	request := func(method, host, path, token, body string) (int, string) {
		t.Helper()
		r, err := http.NewRequest(method, server.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		r.Host = host
		if token != "" {
			r.AddCookie(&http.Cookie{Name: sessions.AppCookieName, Value: token})
		}
		if method == http.MethodPost {
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Origin", "https://"+host)
		}
		response, err := server.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		payload, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		return response.StatusCode, string(payload)
	}

	if status, body := request(http.MethodGet, "alpha.apps.tiny.test", "/_tiny/api/v1/capabilities", "", ""); status != http.StatusUnauthorized || strings.Contains(body, "llm.chat") {
		t.Fatalf("anonymous discovery status=%d body=%q", status, body)
	}
	if status, body := request(http.MethodGet, "alpha.apps.tiny.test", "/_tiny/api/v1/capabilities", aToken, ""); status != http.StatusOK || !strings.Contains(body, `"llm.chat"`) || !strings.Contains(body, `"disclosure"`) || !strings.Contains(body, `"max_output_tokens":10`) || strings.Contains(body, "hidden-test-model") || strings.Contains(body, "provider-secret") || strings.Contains(body, "anthropic") {
		t.Fatalf("unsafe app A discovery status=%d body=%q", status, body)
	}
	if status, body := request(http.MethodGet, "beta.apps.tiny.test", "/_tiny/api/v1/capabilities", bToken, ""); status != http.StatusOK || strings.Contains(body, "llm.chat") || strings.Contains(body, "profile") || strings.Contains(body, "connection") {
		t.Fatalf("ungranted app B discovery status=%d body=%q", status, body)
	}

	prompt := "never persist this prompt"
	completion := "never persist this completion"
	payload, err := json.Marshal(map[string]any{"messages": []map[string]string{{"role": "user", "content": prompt}}, "max_output_tokens": 3})
	if err != nil {
		t.Fatal(err)
	}
	if status, body := request(http.MethodPost, "beta.apps.tiny.test", "/_tiny/api/v1/llm/chat", bToken, string(payload)); status != http.StatusForbidden || strings.Contains(body, "app-a") || strings.Contains(body, "grant") || adapter.Count() != 0 {
		t.Fatalf("app B invocation status=%d calls=%d body=%q", status, adapter.Count(), body)
	}
	if status, body := request(http.MethodPost, "alpha.apps.tiny.test", "/_tiny/api/v1/llm/chat", aToken, string(payload)); status != http.StatusOK || !strings.Contains(body, completion) || adapter.Count() != 1 {
		t.Fatalf("app A invocation status=%d calls=%d body=%q", status, adapter.Count(), body)
	}
	var used, reserved, inFlight int
	if err := store.DB.QueryRow("SELECT used_tokens,reserved_tokens,in_flight FROM llm_usage WHERE app_id='app-a' AND profile_id='profile'").Scan(&used, &reserved, &inFlight); err != nil || used != 5 || reserved != 0 || inFlight != 0 {
		t.Fatalf("usage used=%d reserved=%d in_flight=%d err=%v", used, reserved, inFlight, err)
	}
	var visibleAudit string
	if err := store.DB.QueryRow("SELECT COALESCE(metadata_json,'') FROM audit_events WHERE action='llm.chat.complete'").Scan(&visibleAudit); err != nil || strings.Contains(visibleAudit, prompt) || strings.Contains(visibleAudit, completion) || strings.Contains(visibleAudit, "provider-secret") {
		t.Fatalf("unsafe audit=%q err=%v", visibleAudit, err)
	}
	var secretRows int
	if err := store.DB.QueryRow("SELECT COUNT(*) FROM provider_connections WHERE credential_envelope LIKE '%' || ? || '%'", "provider-secret").Scan(&secretRows); err != nil || secretRows != 0 {
		t.Fatalf("plaintext credential rows=%d err=%v", secretRows, err)
	}

	if err := repo.UpdateGrant(ctx, persistence.LLMGrantInput{AppID: "app-a", ProfileID: "profile", OperatorID: "owner-app-a", Status: "revoked"}, 1); err != nil {
		t.Fatal(err)
	}
	if status, body := request(http.MethodGet, "alpha.apps.tiny.test", "/_tiny/api/v1/capabilities", aToken, ""); status != http.StatusOK || strings.Contains(body, "llm.chat") {
		t.Fatalf("revoked discovery status=%d body=%q", status, body)
	}
	if status, body := request(http.MethodPost, "alpha.apps.tiny.test", "/_tiny/api/v1/llm/chat", aToken, string(payload)); status != http.StatusForbidden || adapter.Count() != 1 {
		t.Fatalf("revoked invocation status=%d calls=%d body=%q", status, adapter.Count(), body)
	}
}

type llmGatewayAdapter struct {
	mu    sync.Mutex
	calls int
}

func (a *llmGatewayAdapter) Complete(context.Context, llm.Binding, llm.Request) (llm.Response, error) {
	a.mu.Lock()
	a.calls++
	a.mu.Unlock()
	return llm.Response{Message: llm.MessageResponse{Role: "assistant", Content: "never persist this completion"}, Usage: llm.Usage{InputTokens: 3, OutputTokens: 2}, FinishReason: "stop"}, nil
}
func (a *llmGatewayAdapter) Count() int { a.mu.Lock(); defer a.mu.Unlock(); return a.calls }

func seedLLMGatewayApp(t *testing.T, store *persistence.SQLiteStore, id, slug string, requestLLM bool) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	owner := "owner-" + id
	if _, err := store.DB.Exec("INSERT INTO users(id,normalized_email,role,status,created_at) VALUES(?,?, 'deployer','active',?)", owner, owner+"@example.com", now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB.Exec("INSERT INTO applications(id,owner_user_id,slug,status,policy_revision,created_at,updated_at) VALUES(?,?,?,'active',1,?,?)", id, owner, slug, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB.Exec("INSERT INTO access_policies(app_id,revision,mode,created_by,created_at) VALUES(?,1,'private',?,?)", id, owner, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB.Exec("INSERT INTO access_rules(id,app_id,policy_revision,kind,normalized_value,created_by,created_at) VALUES(?, ?,1,'email','viewer@example.com',?,?)", "rule-"+id, id, owner, now); err != nil {
		t.Fatal(err)
	}
	manifest, err := json.Marshal(releases.Manifest{Version: 1, Name: slug, LLMChat: requestLLM})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB.Exec("INSERT INTO deployments(id,app_id,created_by,idempotency_key,release_hash,manifest_json,state,created_at) VALUES(?,?,?,?,?,?, 'active',?)", "dep-"+id, id, owner, "key-"+id, "hash-"+id, manifest, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB.Exec("UPDATE applications SET current_deployment_id=? WHERE id=?", "dep-"+id, id); err != nil {
		t.Fatal(err)
	}
}
