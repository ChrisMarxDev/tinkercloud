package anthropic

import (
	"context"
	"github.com/ChrisMarxDev/tinkercloud/internal/llm"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func binding() llm.Binding {
	return llm.Binding{Provider: llm.ProviderAnthropic, Credential: []byte("never-log"), Model: "claude-test", Limits: llm.Limits{MaxOutputTokens: 8, Timeout: time.Second}}
}
func TestConformanceAndFixedHeaders(t *testing.T) {
	h := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "never-log" || r.Header.Get("anthropic-version") == "" {
			t.Error("fixed credential headers missing")
		}
		w.Header().Set("content-type", "application/json")
		w.Write([]byte(`{"content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":2,"output_tokens":3}}`))
	}))
	defer h.Close()
	out, e := NewForTest(h.URL, http.DefaultClient).Complete(context.Background(), binding(), llm.Request{Messages: []llm.Message{{Role: "user", Content: "hello"}}})
	if e != nil || out.Message.Content != "ok" || out.FinishReason != "stop" {
		t.Fatal(out, e)
	}
}
func TestRedirectIsRejected(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("redirect followed") }))
	defer target.Close()
	h := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	defer h.Close()
	if _, e := NewForTest(h.URL, http.DefaultClient).Complete(context.Background(), binding(), llm.Request{Messages: []llm.Message{{Role: "user", Content: "x"}}}); e == nil {
		t.Fatal("redirect accepted")
	}
}

func TestCompletionRejectsOversizeAndTrailingResponses(t *testing.T) {
	valid := `{"content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":2,"output_tokens":3}}`
	for _, tc := range []struct {
		name, body string
	}{
		{"oversize", valid + strings.Repeat(" ", maxProviderResponseBytes)},
		{"trailing json", valid + `{}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(tc.body))
			}))
			defer h.Close()
			if _, err := NewForTest(h.URL, http.DefaultClient).Complete(context.Background(), binding(), llm.Request{Messages: []llm.Message{{Role: "user", Content: "x"}}}); err == nil {
				t.Fatal("unsafe provider response accepted")
			}
		})
	}
}

func TestCredentialValidationRejectsOversizeAndTrailingResponses(t *testing.T) {
	valid := `{"data":[]}`
	for _, tc := range []struct {
		name, body string
	}{
		{"oversize", valid + strings.Repeat(" ", maxProviderResponseBytes)},
		{"trailing json", valid + `{}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(tc.body))
			}))
			defer h.Close()
			if err := NewForTest(h.URL, http.DefaultClient).ValidateCredential(context.Background(), []byte("key")); err == nil {
				t.Fatal("unsafe validation response accepted")
			}
		})
	}
}

func TestListModelsUsesCredentialAndReturnsBoundedIdentifiers(t *testing.T) {
	h := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "catalog-key" || r.Header.Get("anthropic-version") == "" || r.URL.Query().Get("limit") != "1000" {
			t.Error("catalog request did not use the fixed authenticated model endpoint contract")
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"claude-z"},{"id":"claude-a"},{"id":"claude-a"}]}`))
	}))
	defer h.Close()
	models, err := NewForTest(h.URL, http.DefaultClient).ListModels(context.Background(), []byte("catalog-key"))
	if err != nil || len(models) != 2 || models[0] != "claude-a" || models[1] != "claude-z" {
		t.Fatalf("unexpected catalog: %#v, %v", models, err)
	}
}

func TestListModelsRejectsMalformedIdentifier(t *testing.T) {
	h := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"claude-safe"},{"id":"bad\nmodel"}]}`))
	}))
	defer h.Close()
	adapter := NewForTest(h.URL, http.DefaultClient)
	if err := adapter.ValidateCredential(context.Background(), []byte("key")); err != nil {
		t.Fatalf("usable key was rejected only because its catalog cannot render: %v", err)
	}
	if models, err := adapter.ListModels(context.Background(), []byte("key")); err == nil || models != nil {
		t.Fatalf("malformed catalog accepted: %#v", models)
	}
}
