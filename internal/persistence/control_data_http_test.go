package persistence

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ChrisMarxDev/tinkercloud/internal/controlapi"
)

func TestDeployerDataHTTPVerticalPathAndDenials(t *testing.T) {
	svc, store, cleanup := newControlDataService(t)
	defer cleanup()
	ctx := context.Background()
	if _, err := store.DB.Exec("INSERT INTO users(id,normalized_email,role,status,created_at) VALUES('other','other@example.com','deployer','active',datetime('now'))"); err != nil {
		t.Fatal(err)
	}
	owner, err := store.IssueToken(ctx, "u", "", []string{"data:read", "data:write"}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.IssueToken(ctx, "other", "", []string{"data:read", "data:write"}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	wrongScope, err := store.IssueToken(ctx, "u", "", []string{"app:read"}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	dispatcher := controlapi.Dispatcher{Auth: ControlAuthenticator{Store: store}, Service: svc}
	callData := func(method, path, token, key, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if key != "" {
			req.Header.Set("Idempotency-Key", key)
		}
		rec := httptest.NewRecorder()
		dispatcher.ServeHTTP(rec, req)
		return rec
	}

	created := callData(http.MethodPut, "/api/v1/apps/alpha/data/kv/theme", owner, "set-theme", `{"value":"dark"}`)
	if created.Code != http.StatusOK || !bytes.Contains(created.Body.Bytes(), []byte(`"version":1`)) {
		t.Fatalf("owner set = %d %s", created.Code, created.Body.String())
	}
	read := callData(http.MethodGet, "/api/v1/apps/alpha/data/kv/theme", owner, "", "")
	if read.Code != http.StatusOK || !bytes.Contains(read.Body.Bytes(), []byte(`"dark"`)) || read.Header().Get("Cache-Control") != "no-store" || read.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("owner read = %d %s", read.Code, read.Body.String())
	}
	foreign := callData(http.MethodGet, "/api/v1/apps/alpha/data/kv/theme", other, "", "")
	if foreign.Code != http.StatusForbidden || bytes.Contains(foreign.Body.Bytes(), []byte("dark")) {
		t.Fatalf("foreign read = %d %s", foreign.Code, foreign.Body.String())
	}
	if denied := callData(http.MethodGet, "/api/v1/apps/alpha/data/kv/theme", wrongScope, "", ""); denied.Code != http.StatusUnauthorized {
		t.Fatalf("wrong scope = %d %s", denied.Code, denied.Body.String())
	}
	if anonymous := callData(http.MethodGet, "/api/v1/apps/alpha/data/kv/theme", "", "", ""); anonymous.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous = %d %s", anonymous.Code, anonymous.Body.String())
	}
	duplicate := callData(http.MethodPut, "/api/v1/apps/alpha/data/kv/duplicate", owner, "duplicate", `{"value":{"same":1,"same":2}}`)
	if duplicate.Code != http.StatusBadRequest {
		t.Fatalf("duplicate JSON = %d %s", duplicate.Code, duplicate.Body.String())
	}
	missing := callData(http.MethodGet, "/api/v1/apps/alpha/data/kv/duplicate", owner, "", "")
	if missing.Code != http.StatusNotFound {
		t.Fatalf("denied mutation touched data = %d %s", missing.Code, missing.Body.String())
	}

	document := callData(http.MethodPost, "/api/v1/apps/alpha/data/collections/tasks/documents", owner, "create-task", `{"data":{"title":"Ship"}}`)
	if document.Code != http.StatusOK || !bytes.Contains(document.Body.Bytes(), []byte(`"version":1`)) {
		t.Fatalf("document create = %d %s", document.Code, document.Body.String())
	}
	replayed := callData(http.MethodPost, "/api/v1/apps/alpha/data/collections/tasks/documents", owner, "create-task", `{"data":{"title":"Ship"}}`)
	if replayed.Code != http.StatusOK || replayed.Body.String() != document.Body.String() {
		t.Fatalf("document replay = %d %s; want %s", replayed.Code, replayed.Body.String(), document.Body.String())
	}

	// The response bodies above are bounded JSON and safe to discard. Closing
	// is implicit for recorder responses, but consume one through the standard
	// response path to guard against accidental streaming behavior.
	response := replayed.Result()
	defer response.Body.Close()
	if _, err := io.ReadAll(io.LimitReader(response.Body, controlapi.MaxBody+1)); err != nil {
		t.Fatal(err)
	}
}
