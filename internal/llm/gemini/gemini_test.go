package gemini

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
	return llm.Binding{Provider: llm.ProviderGemini, Credential: []byte("never-log"), Model: "gemini-test", Limits: llm.Limits{MaxOutputTokens: 8, Timeout: time.Second}}
}
func TestConformance(t *testing.T) {
	h := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-goog-api-key") != "never-log" {
			t.Error("key header absent")
		}
		w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":3}}`))
	}))
	defer h.Close()
	o, e := NewForTest(h.URL, http.DefaultClient).Complete(context.Background(), binding(), llm.Request{Messages: []llm.Message{{Role: "user", Content: "x"}}})
	if e != nil || o.Message.Content != "ok" || o.FinishReason != "stop" {
		t.Fatal(o, e)
	}
}

func TestCompletionRejectsOversizeAndTrailingResponses(t *testing.T) {
	valid := `{"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":3}}`
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
	valid := `{"models":[]}`
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
