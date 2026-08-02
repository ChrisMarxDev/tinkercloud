package email

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ChrisMarxDev/tinkercloud/internal/otp"
)

func TestSendGridSchemaAndScopedCredential(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer sendgrid-token" || r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("unexpected request: %s %#v", r.Method, r.Header)
		}
		if r.Header.Get("X-Postmark-Server-Token") != "" {
			t.Fatal("SendGrid credential copied into Postmark header")
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	provider := SendGrid{Credential: cred("sendgrid-token"), From: "Tinkercloud <access@example.test>", Endpoint: server.URL}
	if err := provider.EnqueueOTP(context.Background(), otp.Message{Email: "viewer@example.test", Code: "123456"}); err != nil {
		t.Fatal(err)
	}
	personalizations, ok := body["personalizations"].([]any)
	if !ok || len(personalizations) != 1 {
		t.Fatalf("personalizations = %#v", body["personalizations"])
	}
	content, ok := body["content"].([]any)
	if !ok || len(content) != 1 || content[0].(map[string]any)["type"] != "text/plain" || content[0].(map[string]any)["value"] != "Your code: 123456" {
		t.Fatalf("content = %#v", body["content"])
	}
}

func TestSendGridFailuresAreRedacted(t *testing.T) {
	message := otp.Message{Email: "viewer@example.test", Code: "sensitive-code"}
	for name, provider := range map[string]SendGrid{
		"missing credential": {From: "access@example.test"},
		"invalid sender":     {Credential: cred("sensitive-token"), From: "bad\nfrom"},
		"transport":          {Credential: cred("sensitive-token"), From: "access@example.test", Endpoint: "http://127.0.0.1:1"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := provider.EnqueueOTP(context.Background(), message); err != ErrUnavailable {
				t.Fatalf("error = %v", err)
			}
		})
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"errors":[{"message":"sensitive-token"}]}`, http.StatusUnauthorized)
	}))
	defer server.Close()
	provider := SendGrid{Credential: cred("sensitive-token"), From: "access@example.test", Endpoint: server.URL}
	if err := provider.EnqueueOTP(context.Background(), message); err != ErrUnavailable {
		t.Fatalf("provider rejection = %v", err)
	}
}
