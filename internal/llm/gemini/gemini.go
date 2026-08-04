// Package gemini maps Tinkercloud's common chat contract to Gemini's fixed
// generateContent endpoint. It does not provide proxy behavior.
package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/ChrisMarxDev/tinkercloud/internal/llm"
)

const officialBase = "https://generativelanguage.googleapis.com/v1beta/models/"
const maxProviderResponseBytes = 1 << 20

var errProvider = errors.New("gemini unavailable")

type Adapter struct {
	client *http.Client
	base   string
}

func New(client *http.Client) *Adapter                     { return newAdapter(officialBase, client) }
func NewForTest(base string, client *http.Client) *Adapter { return newAdapter(base, client) }
func newAdapter(base string, client *http.Client) *Adapter {
	if client == nil {
		client = &http.Client{Timeout: 35 * time.Second}
	}
	c := *client
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Adapter{&c, base}
}
func (a *Adapter) Complete(ctx context.Context, b llm.Binding, in llm.Request) (llm.Response, error) {
	if a == nil || a.client == nil || b.Provider != llm.ProviderGemini || len(b.Credential) == 0 || b.Model == "" || strings.ContainsAny(b.Model, "/?#&") {
		return llm.Response{}, errProvider
	}
	endpoint := strings.TrimRight(a.base, "/") + "/" + b.Model + ":generateContent"
	u, err := url.Parse(endpoint)
	if err != nil || (u.Scheme != "https" && u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") {
		return llm.Response{}, errProvider
	}
	type part struct {
		Text string `json:"text"`
	}
	type content struct {
		Role  string `json:"role"`
		Parts []part `json:"parts"`
	}
	contents := make([]content, len(in.Messages))
	for i, m := range in.Messages {
		role := m.Role
		if role == "assistant" {
			role = "model"
		}
		contents[i] = content{role, []part{{m.Content}}}
	}
	max := in.MaxOutputTokens
	if max == 0 {
		max = b.Limits.MaxOutputTokens
	}
	body := struct {
		Contents         []content `json:"contents"`
		GenerationConfig struct {
			Max int `json:"maxOutputTokens"`
		} `json:"generationConfig"`
	}{Contents: contents}
	body.GenerationConfig.Max = max
	payload, err := json.Marshal(body)
	if err != nil {
		return llm.Response{}, errProvider
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(payload))
	if err != nil {
		return llm.Response{}, errProvider
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-goog-api-key", string(b.Credential))
	resp, err := a.client.Do(req)
	if err != nil {
		return llm.Response{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return llm.Response{}, errProvider
	}
	var raw struct {
		Candidates []struct {
			Content struct {
				Parts []part `json:"parts"`
			} `json:"content"`
			Finish string `json:"finishReason"`
		} `json:"candidates"`
		Usage *struct {
			Input  int `json:"promptTokenCount"`
			Output int `json:"candidatesTokenCount"`
		} `json:"usageMetadata"`
	}
	if readStrictJSON(resp.Body, &raw) != nil || len(raw.Candidates) != 1 || raw.Usage == nil ||
		len(raw.Candidates[0].Content.Parts) == 0 || raw.Usage.Input < 0 || raw.Usage.Output < 0 {
		return llm.Response{}, errProvider
	}
	txt := ""
	for _, p := range raw.Candidates[0].Content.Parts {
		txt += p.Text
	}
	finish := ""
	if raw.Candidates[0].Finish == "STOP" {
		finish = "stop"
	}
	if raw.Candidates[0].Finish == "MAX_TOKENS" {
		finish = "length"
	}
	if txt == "" || finish == "" {
		return llm.Response{}, errProvider
	}
	return llm.Response{Message: llm.MessageResponse{Role: "assistant", Content: txt}, Usage: llm.Usage{InputTokens: raw.Usage.Input, OutputTokens: raw.Usage.Output}, FinishReason: finish}, nil
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
		Name                       string   `json:"name"`
		SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
	}
	if json.Unmarshal(raw, &entries) != nil || len(entries) > 1000 {
		return nil, errProvider
	}
	seen := make(map[string]struct{}, len(entries))
	models := make([]string, 0, len(entries))
	for _, entry := range entries {
		supported := false
		for _, method := range entry.SupportedGenerationMethods {
			if method == "generateContent" {
				supported = true
				break
			}
		}
		if !supported {
			continue
		}
		model := strings.TrimPrefix(entry.Name, "models/")
		if model == entry.Name || !llm.ValidModelIdentifier(model) || strings.ContainsAny(model, "/?#&") {
			return nil, errProvider
		}
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		models = append(models, model)
	}
	sort.Strings(models)
	return models, nil
}

func (a *Adapter) fetchModels(ctx context.Context, key []byte) (json.RawMessage, error) {
	if a == nil || a.client == nil || len(key) == 0 {
		return nil, errProvider
	}
	endpoint := strings.TrimRight(a.base, "/")
	if endpoint == strings.TrimRight(officialBase, "/") {
		endpoint = "https://generativelanguage.googleapis.com/v1beta/models"
	}
	u, e := url.Parse(endpoint)
	if e != nil || (u.Scheme != "https" && u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") {
		return nil, errProvider
	}
	q := u.Query()
	q.Set("pageSize", "1000")
	u.RawQuery = q.Encode()
	r, e := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if e != nil {
		return nil, errProvider
	}
	r.Header.Set("x-goog-api-key", string(key))
	resp, e := a.client.Do(r)
	if e != nil {
		return nil, errProvider
	}
	defer resp.Body.Close()
	var validation struct {
		Models json.RawMessage `json:"models"`
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 ||
		readStrictJSON(resp.Body, &validation) != nil ||
		len(validation.Models) == 0 || bytes.Equal(validation.Models, []byte("null")) {
		return nil, errProvider
	}
	return validation.Models, nil
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
