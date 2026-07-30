package anthropic

import (
	"context"
	"github.com/tinyhost/tiny/internal/llm"
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
