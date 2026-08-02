package email

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ChrisMarxDev/tinkercloud/internal/otp"
)

func TestPostmarkSchemaAndScopedCredential(t *testing.T) {
	var got map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("X-Postmark-Server-Token") != "server-token" {
			t.Fatalf("unexpected request: %s %#v", r.Method, r.Header)
		}
		if r.Header.Get("Authorization") != "" || r.Header.Get("Content-Type") != "application/json" || r.Header.Get("Accept") != "application/json" {
			t.Fatalf("unsafe or missing headers: %#v", r.Header)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	provider := Postmark{Credential: cred("server-token"), From: "Tinkercloud <access@example.test>", Endpoint: server.URL}
	if err := provider.EnqueueOTP(context.Background(), otp.Message{Email: "viewer@example.test", Code: "123456"}); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"From":          "Tinkercloud <access@example.test>",
		"To":            "viewer@example.test",
		"Subject":       "Your sign-in code",
		"TextBody":      "Your code: 123456",
		"MessageStream": "outbound",
	}
	if len(got) != len(want) {
		t.Fatalf("payload = %#v", got)
	}
	for key, value := range want {
		if got[key] != value {
			t.Fatalf("%s = %q, want %q", key, got[key], value)
		}
	}
}

func TestPostmarkFailuresAreRedacted(t *testing.T) {
	message := otp.Message{Email: "viewer@example.test", Code: "sensitive-code"}
	for name, provider := range map[string]Postmark{
		"missing credential": {From: "access@example.test"},
		"missing sender":     {Credential: cred("sensitive-token")},
		"transport":          {Credential: cred("sensitive-token"), From: "access@example.test", Endpoint: "http://127.0.0.1:1"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := provider.EnqueueOTP(context.Background(), message); err != ErrUnavailable {
				t.Fatalf("error = %v", err)
			}
		})
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"ErrorCode":10,"Message":"sensitive-token"}`, http.StatusUnprocessableEntity)
	}))
	defer server.Close()
	provider := Postmark{Credential: cred("sensitive-token"), From: "access@example.test", Endpoint: server.URL}
	if err := provider.EnqueueOTP(context.Background(), message); err != ErrUnavailable {
		t.Fatalf("provider rejection = %v", err)
	}
}
