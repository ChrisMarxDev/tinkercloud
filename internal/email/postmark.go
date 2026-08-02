package email

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/ChrisMarxDev/tinkercloud/internal/otp"
)

// Postmark is the direct, dependency-free OTP adapter for Postmark's default
// transactional message stream. Endpoint is a test seam; production
// composition leaves it empty and therefore cannot select a runtime origin.
type Postmark struct {
	Credential Credential
	From       string
	Client     *http.Client
	Endpoint   string
}

func (p Postmark) EnqueueOTP(ctx context.Context, m otp.Message) error {
	if p.Credential == nil || p.Credential.APIKey() == "" || p.From == "" {
		return ErrUnavailable
	}
	body, err := json.Marshal(map[string]string{
		"From":          p.From,
		"To":            m.Email,
		"Subject":       "Your sign-in code",
		"TextBody":      "Your code: " + m.Code,
		"MessageStream": "outbound",
	})
	if err != nil {
		return ErrUnavailable
	}
	endpoint := p.Endpoint
	if endpoint == "" {
		endpoint = "https://api.postmarkapp.com/email"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return ErrUnavailable
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Postmark-Server-Token", p.Credential.APIKey())
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ErrUnavailable
	}
	return nil
}
