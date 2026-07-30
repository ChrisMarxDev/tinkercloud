package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tinyhost/tiny/internal/blob"
	"github.com/tinyhost/tiny/internal/compose"
	"github.com/tinyhost/tiny/internal/config"
	"github.com/tinyhost/tiny/internal/identity"
	"github.com/tinyhost/tiny/internal/live"
	"github.com/tinyhost/tiny/internal/persistence"
	"github.com/tinyhost/tiny/internal/releases"
	"github.com/tinyhost/tiny/internal/sessions"
)

// TestBlobGatewayTwoAppMatrix deliberately uses the real SQLite catalog and
// local private byte adapter. It proves that gateway-derived tenant scope holds
// through the final storage boundary, not merely through a mock handler.
func TestBlobGatewayTwoAppMatrix(t *testing.T) {
	ctx := context.Background()
	store, err := persistence.OpenSQLite(ctx, filepath.Join(t.TempDir(), "tiny.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	appDatabases, err := persistence.NewAppDatabaseManager(store.DataRoot, persistence.AppDatabaseManagerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer appDatabases.Close()
	seedBlobApp(t, store, "app-a", "alpha", true)
	seedBlobApp(t, store, "app-b", "beta", true)
	seedBlobApp(t, store, "app-c", "charlie", false)
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
	cToken, _, err := store.CreateAppSession(ctx, "app-c", viewer, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	repo := &persistence.BlobRepository{Store: store, Bytes: blob.LocalStore{Root: store.DataRoot}}
	h := compose.AppPlaneWithPlatformAndBlobs(config.Config{PlatformHost: "tiny.test", AppSuffix: "apps.tiny.test", SessionCookie: sessions.AppCookieName}, store, store, store, persistence.KVRepository{Apps: appDatabases}, repo, live.New(live.DefaultLimits()), compose.Login{}, nil)
	server := httptest.NewServer(h)
	defer server.Close()
	client := server.Client()

	request := func(method, host, path, token, origin string, body io.Reader, contentType string) *http.Response {
		t.Helper()
		r, err := http.NewRequest(method, server.URL+path, body)
		if err != nil {
			t.Fatal(err)
		}
		r.Host = host
		if token != "" {
			r.AddCookie(&http.Cookie{Name: sessions.AppCookieName, Value: token})
		}
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		if contentType != "" {
			r.Header.Set("Content-Type", contentType)
		}
		out, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	read := func(r *http.Response) string {
		t.Helper()
		b, err := io.ReadAll(r.Body)
		r.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	upload := func(host, token, origin, name, contents string) *http.Response {
		t.Helper()
		var body bytes.Buffer
		w := multipart.NewWriter(&body)
		part, err := w.CreateFormFile("file", name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = io.WriteString(part, contents); err != nil {
			t.Fatal(err)
		}
		if err = w.Close(); err != nil {
			t.Fatal(err)
		}
		return request(http.MethodPost, host, "/_tiny/api/v1/blobs", token, origin, &body, w.FormDataContentType())
	}
	count := func(app string) int {
		t.Helper()
		var n int
		if err := store.DB.QueryRow("SELECT COUNT(*) FROM app_blobs WHERE app_id=?", app).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	// Authorization and capability checks run before the blob service/catalog.
	for name, tc := range map[string]struct{ host, token string }{
		"anonymous":           {"alpha.apps.tiny.test", ""},
		"capability-disabled": {"charlie.apps.tiny.test", cToken},
	} {
		t.Run(name, func(t *testing.T) {
			r := upload(tc.host, tc.token, "https://"+tc.host, "never.txt", "NEVER")
			body := read(r)
			if r.StatusCode != http.StatusUnauthorized && r.StatusCode != http.StatusForbidden {
				t.Fatalf("status=%d body=%s", r.StatusCode, body)
			}
			if strings.Contains(body, "NEVER") || count("app-a") != 0 || count("app-c") != 0 {
				t.Fatalf("denied request touched blob state: %s", body)
			}
		})
	}
	// A cookie cannot turn a cross-origin form post into an authorized mutation.
	r := upload("alpha.apps.tiny.test", aToken, "https://evil.test", "cross.txt", "CROSS")
	if body := read(r); r.StatusCode != http.StatusForbidden || strings.Contains(body, "CROSS") || count("app-a") != 0 {
		t.Fatalf("cross origin mutation status=%d body=%q rows=%d", r.StatusCode, body, count("app-a"))
	}

	r = upload("alpha.apps.tiny.test", aToken, "https://alpha.apps.tiny.test", "report.txt", "APP-A-BYTES")
	if r.StatusCode != http.StatusCreated {
		t.Fatalf("upload status=%d body=%s", r.StatusCode, read(r))
	}
	var uploaded struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&uploaded); err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	if uploaded.ID == "" || count("app-a") != 1 {
		t.Fatal("ready catalog record was not created")
	}
	// The other app has a valid session but must not learn app A's object.
	r = request(http.MethodGet, "beta.apps.tiny.test", "/_tiny/api/v1/blobs/"+uploaded.ID, bToken, "", nil, "")
	if body := read(r); r.StatusCode != http.StatusNotFound || strings.Contains(body, "APP-A-BYTES") {
		t.Fatalf("cross-app get status=%d body=%q", r.StatusCode, body)
	}

	r = request(http.MethodGet, "alpha.apps.tiny.test", "/_tiny/api/v1/blobs/"+uploaded.ID, aToken, "", nil, "")
	if body := read(r); r.StatusCode != http.StatusOK || body != "APP-A-BYTES" || r.Header.Get("Content-Disposition") != "attachment; filename=report.txt" || r.Header.Get("Cache-Control") != "private, no-store" || r.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("unsafe/incorrect download status=%d body=%q disposition=%q cache=%q nosniff=%q", r.StatusCode, body, r.Header.Get("Content-Disposition"), r.Header.Get("Cache-Control"), r.Header.Get("X-Content-Type-Options"))
	}
	r = request(http.MethodDelete, "alpha.apps.tiny.test", "/_tiny/api/v1/blobs/"+uploaded.ID, aToken, "https://alpha.apps.tiny.test", nil, "")
	if body := read(r); r.StatusCode != http.StatusOK || !strings.Contains(body, `"deleted":true`) || count("app-a") != 0 {
		t.Fatalf("delete status=%d body=%q rows=%d", r.StatusCode, body, count("app-a"))
	}

	// Revocation is checked by the gateway before the catalog can be opened.
	if _, err := store.Revoke(ctx, "app-a", aToken); err != nil {
		t.Fatal(err)
	}
	r = request(http.MethodGet, "alpha.apps.tiny.test", "/_tiny/api/v1/blobs/"+uploaded.ID, aToken, "", nil, "")
	if body := read(r); r.StatusCode != http.StatusUnauthorized || strings.Contains(body, "APP-A-BYTES") {
		t.Fatalf("revoked get status=%d body=%q", r.StatusCode, body)
	}

	// A catalog/byte disagreement fails closed and bounded reconciliation removes
	// it rather than presenting stale metadata as a usable object.
	newToken, _, err := store.CreateAppSession(ctx, "app-a", viewer, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	r = upload("alpha.apps.tiny.test", newToken, "https://alpha.apps.tiny.test", "missing.txt", "TO-BE-REMOVED")
	if r.StatusCode != http.StatusCreated {
		t.Fatalf("mismatch upload status=%d body=%s", r.StatusCode, read(r))
	}
	var mismatch struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&mismatch); err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	if err := repo.Bytes.Delete("app-a", mismatch.ID); err != nil {
		t.Fatal(err)
	}
	r = request(http.MethodGet, "alpha.apps.tiny.test", "/_tiny/api/v1/blobs/"+mismatch.ID, newToken, "", nil, "")
	if body := read(r); r.StatusCode != http.StatusServiceUnavailable || strings.Contains(body, "TO-BE-REMOVED") {
		t.Fatalf("catalog/byte mismatch status=%d body=%q", r.StatusCode, body)
	}
	if err := repo.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	r = request(http.MethodGet, "alpha.apps.tiny.test", "/_tiny/api/v1/blobs", newToken, "", nil, "")
	if body := read(r); r.StatusCode != http.StatusOK || !strings.Contains(body, `"blobs":[]`) || count("app-a") != 0 {
		t.Fatalf("post-reconcile list status=%d body=%q rows=%d", r.StatusCode, body, count("app-a"))
	}
}

func seedBlobApp(t *testing.T, s *persistence.SQLiteStore, id, slug string, blobs bool) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	owner := "owner-" + id
	if _, err := s.DB.Exec("INSERT INTO users(id,normalized_email,role,status,created_at) VALUES(?,?, 'deployer','active',?)", owner, owner+"@example.com", now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec("INSERT INTO applications(id,owner_user_id,slug,status,policy_revision,created_at,updated_at) VALUES(?,?,?,'active',1,?,?)", id, owner, slug, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec("INSERT INTO access_policies(app_id,revision,mode,created_by,created_at) VALUES(?,1,'private',?,?)", id, owner, now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec("INSERT INTO access_rules(id,app_id,policy_revision,kind,normalized_value,created_by,created_at) VALUES(?, ?,1,'email','viewer@example.com',?,?)", "rule-"+id, id, owner, now); err != nil {
		t.Fatal(err)
	}
	manifest, err := json.Marshal(releases.Manifest{Version: 1, Name: slug, Blobs: blobs})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec("INSERT INTO deployments(id,app_id,created_by,idempotency_key,release_hash,manifest_json,state,created_at) VALUES(?,?,?,?,?,?, 'active',?)", "dep-"+id, id, owner, "key-"+id, "hash-"+id, manifest, now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec("UPDATE applications SET current_deployment_id=? WHERE id=?", "dep-"+id, id); err != nil {
		t.Fatal(err)
	}
}
