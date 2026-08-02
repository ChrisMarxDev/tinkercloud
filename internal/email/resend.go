// Package email contains narrow, redacted provider adapters.
package email

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/ChrisMarxDev/tinkercloud/internal/otp"
	"net/http"
	"time"
)

var ErrUnavailable = errors.New("email unavailable")

type Credential interface{ APIKey() string }
type Resend struct {
	Credential Credential
	From       string
	Client     *http.Client
	Endpoint   string
}

func (r Resend) EnqueueOTP(ctx context.Context, m otp.Message) error {
	if r.Credential == nil || r.Credential.APIKey() == "" || r.From == "" {
		return ErrUnavailable
	}
	body, _ := json.Marshal(map[string]any{"from": r.From, "to": []string{m.Email}, "subject": "Your sign-in code", "text": "Your code: " + m.Code})
	endpoint := r.Endpoint
	if endpoint == "" {
		endpoint = "https://api.resend.com/emails"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return ErrUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+r.Credential.APIKey())
	req.Header.Set("Content-Type", "application/json")
	client := r.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ErrUnavailable
	}
	return nil
}
