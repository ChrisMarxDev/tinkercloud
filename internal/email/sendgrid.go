package email

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/mail"
	"time"

	"github.com/ChrisMarxDev/tinkercloud/internal/otp"
)

// SendGrid is the direct, dependency-free adapter for the fixed v3 Mail Send
// endpoint. Endpoint is a test seam; production composition leaves it empty.
type SendGrid struct {
	Credential Credential
	From       string
	Client     *http.Client
	Endpoint   string
}

type sendGridAddress struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

func (s SendGrid) EnqueueOTP(ctx context.Context, message otp.Message) error {
	if s.Credential == nil || s.Credential.APIKey() == "" {
		return ErrUnavailable
	}
	from, err := mail.ParseAddress(s.From)
	if err != nil {
		return ErrUnavailable
	}
	to, err := mail.ParseAddress(message.Email)
	if err != nil || to.Name != "" {
		return ErrUnavailable
	}
	body, err := json.Marshal(map[string]any{
		"personalizations": []any{map[string]any{"to": []sendGridAddress{{Email: to.Address}}}},
		"from":             sendGridAddress{Email: from.Address, Name: from.Name},
		"subject":          "Your sign-in code",
		"content":          []any{map[string]string{"type": "text/plain", "value": "Your code: " + message.Code}},
	})
	if err != nil {
		return ErrUnavailable
	}
	endpoint := s.Endpoint
	if endpoint == "" {
		endpoint = "https://api.sendgrid.com/v3/mail/send"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return ErrUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+s.Credential.APIKey())
	req.Header.Set("Content-Type", "application/json")
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		return ErrUnavailable
	}
	return nil
}
