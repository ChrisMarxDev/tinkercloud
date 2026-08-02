package email

import (
	"context"
	"github.com/ChrisMarxDev/tinkercloud/internal/otp"
	"net/http"
	"net/http/httptest"
	"testing"
)

type cred string

func (c cred) APIKey() string { return string(c) }
func TestResendSchemaAndRedactedFailure(t *testing.T) {
	var auth, body string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		b := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(b)
		body = string(b)
		w.WriteHeader(202)
	}))
	defer s.Close()
	p := Resend{Credential: cred("secret"), From: "a@test", Endpoint: s.URL}
	if e := p.EnqueueOTP(context.Background(), otp.Message{Email: "v@test", Code: "123456"}); e != nil {
		t.Fatal(e)
	}
	if auth != "Bearer secret" || body == "" {
		t.Fatal(auth, body)
	}
	p.Endpoint = "http://127.0.0.1:1"
	if e := p.EnqueueOTP(context.Background(), otp.Message{Email: "v@test", Code: "secret"}); e != ErrUnavailable {
		t.Fatal(e)
	}
}
