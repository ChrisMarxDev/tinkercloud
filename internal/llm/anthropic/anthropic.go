// Package anthropic maps Tinkercloud's common chat contract to Anthropic's fixed
// Messages endpoint. It intentionally has no caller-provided URL or headers.
package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"sort"
	"time"

	"github.com/ChrisMarxDev/tinkercloud/internal/llm"
)

const officialEndpoint = "https://api.anthropic.com/v1/messages"
const maxProviderResponseBytes = 1 << 20

var errProvider = errors.New("anthropic unavailable")

type Adapter struct {
	client   *http.Client
	endpoint string
}

func (a *Adapter) ValidateCredential(ctx context.Context, key []byte) error {
	_, err := a.fetchModels(ctx, key)
	return err
}

func (a *Adapter) ListModels(ctx context.Context, key []byte) ([]string, error) {
	raw, err := a.fetchModels(ctx, key)
	if err != nil {
		return nil, err
	}
	var entries []struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(raw, &entries) != nil || len(entries) > 1000 {
		return nil, errProvider
	}
	seen := make(map[string]struct{}, len(entries))
	models := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !llm.ValidModelIdentifier(entry.ID) {
			return nil, errProvider
		}
		if _, ok := seen[entry.ID]; ok {
			continue
		}
		seen[entry.ID] = struct{}{}
		models = append(models, entry.ID)
	}
	sort.Strings(models)
	return models, nil
}

func (a *Adapter) fetchModels(ctx context.Context, key []byte) (json.RawMessage, error) {
	if a == nil || a.client == nil || len(key) == 0 {
		return nil, errProvider
	}
	endpoint := a.endpoint
	if endpoint == officialEndpoint {
		endpoint = "https://api.anthropic.com/v1/models"
	}
	u, e := url.Parse(endpoint)
	if e != nil || (u.Scheme != "https" && u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") {
		return nil, errProvider
	}
	q := u.Query()
	q.Set("limit", "1000")
	u.RawQuery = q.Encode()
	r, e := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if e != nil {
		return nil, errProvider
	}
	r.Header.Set("x-api-key", string(key))
	r.Header.Set("anthropic-version", "2023-06-01")
	resp, e := a.client.Do(r)
	if e != nil {
		return nil, errProvider
	}
	defer resp.Body.Close()
	var validation struct {
		Data json.RawMessage `json:"data"`
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 ||
		readStrictJSON(resp.Body, &validation) != nil ||
		len(validation.Data) == 0 || bytes.Equal(validation.Data, []byte("null")) {
		return nil, errProvider
	}
	return validation.Data, nil
}

func New(client *http.Client) *Adapter { return newAdapter(officialEndpoint, client) }

// NewForTest exists solely to allow a local conformance server; production
// composition must use New, which fixes the official HTTPS destination.
func NewForTest(endpoint string, client *http.Client) *Adapter { return newAdapter(endpoint, client) }
func newAdapter(endpoint string, client *http.Client) *Adapter {
	if client == nil {
		client = &http.Client{Timeout: 35 * time.Second}
	}
	c := *client
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Adapter{&c, endpoint}
}

func (a *Adapter) Complete(ctx context.Context, b llm.Binding, in llm.Request) (llm.Response, error) {
	if a == nil || a.client == nil || b.Provider != llm.ProviderAnthropic || len(b.Credential) == 0 || b.Model == "" {
		return llm.Response{}, errProvider
	}
	u, err := url.Parse(a.endpoint)
	if err != nil || u.Scheme != "https" && u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost" {
		return llm.Response{}, errProvider
	}
	type message struct{ Role, Content string }
	body := struct {
		Model    string    `json:"model"`
		Max      int       `json:"max_tokens"`
		Messages []message `json:"messages"`
	}{b.Model, in.MaxOutputTokens, make([]message, len(in.Messages))}
	if body.Max == 0 {
		body.Max = b.Limits.MaxOutputTokens
	}
	for i, m := range in.Messages {
		body.Messages[i] = message{m.Role, m.Content}
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return llm.Response{}, errProvider
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(payload))
	if err != nil {
		return llm.Response{}, errProvider
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("x-api-key", string(b.Credential))
	resp, err := a.client.Do(req)
	if err != nil {
		return llm.Response{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return llm.Response{}, errProvider
	}
	var raw struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		StopReason string `json:"stop_reason"`
		Usage      *struct {
			Input  int `json:"input_tokens"`
			Output int `json:"output_tokens"`
		} `json:"usage"`
	}
	if readStrictJSON(resp.Body, &raw) != nil || raw.Usage == nil || len(raw.Content) == 0 ||
		raw.Usage.Input < 0 || raw.Usage.Output < 0 {
		return llm.Response{}, errProvider
	}
	text := ""
	for _, p := range raw.Content {
		if p.Type == "text" {
			text += p.Text
		}
	}
	finish := ""
	if raw.StopReason == "end_turn" {
		finish = "stop"
	}
	if raw.StopReason == "max_tokens" {
		finish = "length"
	}
	if text == "" || finish == "" {
		return llm.Response{}, errProvider
	}
	return llm.Response{Message: llm.MessageResponse{Role: "assistant", Content: text}, Usage: llm.Usage{InputTokens: raw.Usage.Input, OutputTokens: raw.Usage.Output}, FinishReason: finish}, nil
}

func readStrictJSON(body io.Reader, dst any) error {
	data, err := io.ReadAll(io.LimitReader(body, maxProviderResponseBytes+1))
	if err != nil || len(data) == 0 || len(data) > maxProviderResponseBytes {
		return errProvider
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(dst); err != nil {
		return errProvider
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return errProvider
	}
	return nil
}
