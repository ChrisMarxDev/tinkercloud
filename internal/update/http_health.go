package update

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"regexp"
	"time"
)

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// HTTPHealth verifies a public endpoint returns a successful response without
// following a redirect to an unexpected endpoint.
type HTTPHealth struct {
	URL     string
	Client  HTTPDoer
	Timeout time.Duration
}

func (h HTTPHealth) Check(ctx context.Context) error {
	if h.URL == "" || h.Client == nil {
		return ErrHealth
	}
	if h.Timeout <= 0 {
		h.Timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, h.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.URL, nil)
	if err != nil {
		return ErrHealth
	}
	resp, err := h.Client.Do(req)
	if err != nil {
		return ErrHealth
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ErrHealth
	}
	return nil
}

const maxAnonymousDenyEvidenceBytes int64 = 32 << 10

var gatewayRequestID = regexp.MustCompile(`^req_[0-9a-f]{24}$`)

// AnonymousDenyHealth proves that an unauthenticated request reached the
// composed gateway's protected-route denial. It does not treat a generic 404
// (or any arbitrary 401) as evidence: the update probe must fail rather than
// preserve a candidate binary whose app routing is missing or public.
type AnonymousDenyHealth struct{ HTTPHealth }

func (h AnonymousDenyHealth) Check(ctx context.Context) error {
	if h.URL == "" || h.Client == nil {
		return ErrHealth
	}
	if h.Timeout <= 0 {
		h.Timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, h.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.URL, nil)
	if err != nil {
		return ErrHealth
	}
	resp, err := h.Client.Do(req)
	if err != nil || resp == nil || resp.Body == nil || resp.Request == nil || resp.Request.URL == nil || resp.Request.URL.String() != req.URL.String() {
		return ErrHealth
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized || resp.Header.Get("Cache-Control") != "no-store" || resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		return ErrHealth
	}
	contentType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" {
		return ErrHealth
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxAnonymousDenyEvidenceBytes+1))
	if err != nil || int64(len(body)) > maxAnonymousDenyEvidenceBytes {
		return ErrHealth
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil || len(envelope) != 1 {
		return ErrHealth
	}
	var denial struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
	}
	raw, ok := envelope["error"]
	if !ok || json.Unmarshal(raw, &denial) != nil || denial.Code != "not_authorized" || denial.Message != "This request is not authorized." || !gatewayRequestID.MatchString(denial.RequestID) || denial.RequestID != resp.Header.Get("X-Request-ID") {
		return ErrHealth
	}
	return nil
}

// UnknownHostDenyHealth is used only when the control database proves there
// are no active apps.  It makes the empty-install health gate explicit without
// ever accepting a caller-selected app host.
type UnknownHostDenyHealth struct{ HTTPHealth }

func (h UnknownHostDenyHealth) Check(ctx context.Context) error {
	if h.URL == "" || h.Client == nil {
		return ErrHealth
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.URL, nil)
	if err != nil {
		return ErrHealth
	}
	resp, err := h.Client.Do(req)
	if err != nil || resp == nil || resp.Body == nil || resp.Request == nil || resp.Request.URL == nil || resp.Request.URL.String() != req.URL.String() {
		return ErrHealth
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound || resp.Header.Get("Location") != "" || resp.Header.Get("Cache-Control") != "no-store" || resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		return ErrHealth
	}
	contentType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" {
		return ErrHealth
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxAnonymousDenyEvidenceBytes+1))
	if err != nil || int64(len(body)) > maxAnonymousDenyEvidenceBytes {
		return ErrHealth
	}
	var envelope map[string]json.RawMessage
	if json.Unmarshal(body, &envelope) != nil || len(envelope) != 1 {
		return ErrHealth
	}
	var denial struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
	}
	raw, ok := envelope["error"]
	if !ok || json.Unmarshal(raw, &denial) != nil || denial.Code != "not_found" || denial.Message != "This request is not authorized." || !gatewayRequestID.MatchString(denial.RequestID) || denial.RequestID != resp.Header.Get("X-Request-ID") {
		return ErrHealth
	}
	return nil
}
