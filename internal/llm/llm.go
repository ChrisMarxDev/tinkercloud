// Package llm implements TinyHost's narrow, operator-governed chat capability.
// It deliberately contains no HTTP routing or browser-facing identity inputs.
package llm

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/tinyhost/tiny/internal/appauth"
	"github.com/tinyhost/tiny/internal/capabilities"
)

// Stable errors are intentionally detail-free: provider bodies, URLs, models,
// grants, and credentials are never suitable browser error material.
var (
	ErrUnauthorized           = errors.New("llm request is not authorized")
	ErrCapabilityUnavailable  = errors.New("llm capability unavailable")
	ErrInvalidRequest         = errors.New("invalid llm chat request")
	ErrRateLimited            = errors.New("llm rate limited")
	ErrQuotaExhausted         = errors.New("llm quota exhausted")
	ErrTemporarilyUnavailable = errors.New("llm temporarily unavailable")
	ErrCancelled              = errors.New("llm request cancelled")
)

type Provider string

const (
	ProviderAnthropic Provider = "anthropic"
	ProviderGemini    Provider = "gemini"
)

type Message struct{ Role, Content string }
type Request struct {
	Messages        []Message
	MaxOutputTokens int
}
type MessageResponse struct{ Role, Content string }
type Usage struct{ InputTokens, OutputTokens int }
type Response struct {
	Message                 MessageResponse
	Usage                   Usage
	FinishReason, RequestID string
}

// Limits are supplied exclusively by an operator-selected profile. A caller
// may tighten MaxOutputTokens but never increase any value here.
type Limits struct {
	MaxMessages, MaxMessageBytes, MaxInputBytes, MaxOutputTokens int
	Timeout                                                      time.Duration
	ViewerRequests, AppRequests                                  int
	RateWindow                                                   time.Duration
}

func DefaultLimits() Limits {
	return Limits{MaxMessages: 32, MaxMessageBytes: 16 << 10, MaxInputBytes: 64 << 10, MaxOutputTokens: 1024, Timeout: 30 * time.Second, ViewerRequests: 20, AppRequests: 120, RateWindow: time.Minute}
}
func absoluteLimits() Limits {
	return Limits{MaxMessages: 128, MaxMessageBytes: 262144, MaxInputBytes: 1048576, MaxOutputTokens: 16384, Timeout: 120 * time.Second, ViewerRequests: 10000, AppRequests: 100000, RateWindow: time.Hour}
}

type Binding struct {
	AppID, ProfileID, ConnectionID, Model, ReservationID string
	Credential                                           []byte
	Provider                                             Provider
	Limits                                               Limits
	ReservedTokens                                       int
}

// Repository is the persistence and grant boundary. Admit must atomically
// verify the active app grant/connection/profile and reserve usage/concurrency.
// It is never called with client supplied tenancy identifiers.
type Repository interface {
	// AdmissionLimits returns only the effective, non-secret profile limits for
	// an active grant. Complete uses it to reject rate-limit excess before it
	// creates a durable usage reservation. Admit repeats every active-state
	// check inside its transaction, so a concurrent revoke or profile change
	// still fails closed.
	AdmissionLimits(context.Context, string) (Limits, error)
	// Both numeric inputs are derived from the validated request. The repository
	// clamps output to the active operator profile before reserving usage.
	Admit(context.Context, string, string, int, int, time.Time) (Binding, error)
	Reconcile(context.Context, Binding, Usage, Outcome, time.Time) error
}

type Outcome string

const (
	OutcomeSucceeded Outcome = "succeeded"
	OutcomeFailed    Outcome = "failed"
	OutcomeCancelled Outcome = "cancelled"
	OutcomeAmbiguous Outcome = "ambiguous"
)

type Adapter interface {
	Complete(context.Context, Binding, Request) (Response, error)
}

// CredentialValidator is used by operator connection create/rotate flows
// before an encrypted credential becomes active.
type CredentialValidator interface {
	ValidateCredential(context.Context, []byte) error
}

type Service struct {
	Repository Repository
	Adapters   map[Provider]Adapter
	Now        func() time.Time
	mu         sync.Mutex
	rates      map[string][]rateMark
	rateSeq    uint64
}

// Discovery is the browser-safe subset of the active LLM capability. It never
// includes the selected model, provider, connection, grant, or credential.
type Discovery struct {
	Limits     map[string]int
	Disclosure string
}

const externalContentDisclosure = "Chat message content is sent to an operator-selected external AI provider."

// rateStateMaxKeys bounds the in-memory pre-admission limiter. When full, it
// denies new keys rather than evicting active ones and accidentally allowing a
// caller to bypass a configured rate window.
const rateStateMaxKeys = 2048

func New(repository Repository, adapters map[Provider]Adapter) *Service {
	return &Service{Repository: repository, Adapters: adapters, Now: time.Now, rates: map[string][]rateMark{}}
}

func (s *Service) Available(ctx context.Context, auth appauth.AuthorizationContext) bool {
	_, ok := s.Discovery(ctx, auth)
	return ok
}

// Discovery returns only safe effective limits after checking both manifest
// intent and the current active grant. It is intentionally absent for an app
// that did not request llm.chat or no longer has an active grant.
func (s *Service) Discovery(ctx context.Context, auth appauth.AuthorizationContext) (Discovery, bool) {
	if s == nil || s.Repository == nil || auth == nil || !auth.LLMChatRequested() {
		return Discovery{}, false
	}
	app, _, _, err := capabilities.Scope(auth)
	if err != nil {
		return Discovery{}, false
	}
	limits, err := s.Repository.AdmissionLimits(ctx, app)
	if err != nil || !validLimits(limits) {
		return Discovery{}, false
	}
	return Discovery{Limits: safeLimits(limits), Disclosure: externalContentDisclosure}, true
}

// Complete has no app, viewer, provider, model, connection, URL, header or
// credential parameter. Those values are all established after authorization.
func (s *Service) Complete(ctx context.Context, auth appauth.AuthorizationContext, request Request) (Response, error) {
	if ctx == nil || auth == nil {
		return Response{}, ErrUnauthorized
	}
	// A sealed gateway authorization context is valid even when this app did
	// not request chat. Keep that capability boundary distinct from anonymous
	// access: callers learn only that chat is unavailable, never a provider
	// detail, and no repository/provider work can occur.
	if !auth.LLMChatRequested() || s == nil || s.Repository == nil {
		return Response{}, ErrCapabilityUnavailable
	}
	appID, viewerID, _, err := capabilities.Scope(auth)
	if err != nil {
		return Response{}, ErrUnauthorized
	}
	if err := validateRequest(request, absoluteLimits()); err != nil {
		return Response{}, err
	}
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}
	// The non-secret active profile limits are read before a durable reservation
	// so rate rejection never consumes quota. Admit rechecks the same state in
	// its transaction; a concurrent change is denied rather than using stale
	// policy.
	limits, err := s.Repository.AdmissionLimits(ctx, appID)
	if err != nil || !validLimits(limits) {
		return Response{}, ErrCapabilityUnavailable
	}
	if err := validateRequest(request, limits); err != nil {
		return Response{}, err
	}
	permit, err := s.reserveRate(appID, viewerID, limits, now)
	if err != nil {
		return Response{}, err
	}
	// Profile-specific validation follows admission. The conservative global
	// validation above ensures malformed/bomb inputs never touch persistence.
	binding, err := s.Repository.Admit(ctx, appID, viewerID, estimateInputTokens(request), request.MaxOutputTokens, now)
	if err != nil {
		permit.Release()
		return Response{}, stable(err)
	}
	defer clear(binding.Credential)
	if binding.AppID != appID || binding.ProfileID == "" || len(binding.Credential) == 0 {
		permit.Release()
		_ = s.Repository.Reconcile(context.Background(), binding, Usage{}, OutcomeFailed, now)
		return Response{}, ErrCapabilityUnavailable
	}
	if !sameLimits(limits, binding.Limits) || validateRequest(request, binding.Limits) != nil {
		permit.Release()
		_ = s.Repository.Reconcile(context.Background(), binding, Usage{}, OutcomeFailed, now)
		return Response{}, ErrTemporarilyUnavailable
	}
	adapter := s.Adapters[binding.Provider]
	if adapter == nil {
		_ = s.Repository.Reconcile(context.Background(), binding, Usage{}, OutcomeFailed, now)
		return Response{}, ErrCapabilityUnavailable
	}
	callCtx, cancel := context.WithTimeout(ctx, binding.Limits.Timeout)
	defer cancel()
	response, callErr := adapter.Complete(callCtx, binding, request)
	if callErr != nil {
		outcome := OutcomeFailed
		if errors.Is(ctx.Err(), context.Canceled) || errors.Is(callCtx.Err(), context.Canceled) {
			outcome = OutcomeAmbiguous
		}
		if err := s.Repository.Reconcile(context.Background(), binding, Usage{}, outcome, now); err != nil {
			return Response{}, ErrTemporarilyUnavailable
		}
		if outcome == OutcomeAmbiguous {
			return Response{}, ErrCancelled
		}
		return Response{}, stable(callErr)
	}
	response.RequestID = auth.RequestID()
	if response.Message.Role != "assistant" || response.Message.Content == "" || !utf8.ValidString(response.Message.Content) || response.Usage.InputTokens < 0 || response.Usage.OutputTokens < 0 || response.Usage.OutputTokens > binding.Limits.MaxOutputTokens || (response.FinishReason != "stop" && response.FinishReason != "length") {
		if err := s.Repository.Reconcile(context.Background(), binding, Usage{}, OutcomeAmbiguous, now); err != nil {
			return Response{}, ErrTemporarilyUnavailable
		}
		return Response{}, ErrTemporarilyUnavailable
	}
	if err := s.Repository.Reconcile(context.Background(), binding, response.Usage, OutcomeSucceeded, now); err != nil {
		return Response{}, ErrTemporarilyUnavailable
	}
	return response, nil
}

func stable(err error) error {
	switch {
	case errors.Is(err, ErrUnauthorized):
		return ErrUnauthorized
	case errors.Is(err, ErrCapabilityUnavailable):
		return ErrCapabilityUnavailable
	case errors.Is(err, ErrInvalidRequest):
		return ErrInvalidRequest
	case errors.Is(err, ErrRateLimited):
		return ErrRateLimited
	case errors.Is(err, ErrQuotaExhausted):
		return ErrQuotaExhausted
	case errors.Is(err, context.Canceled):
		return ErrCancelled
	default:
		return ErrTemporarilyUnavailable
	}
}

func validateRequest(r Request, l Limits) error {
	if l.MaxMessages <= 0 {
		l = DefaultLimits()
	}
	if len(r.Messages) == 0 || len(r.Messages) > l.MaxMessages || (r.MaxOutputTokens != 0 && (r.MaxOutputTokens < 1 || r.MaxOutputTokens > l.MaxOutputTokens)) {
		return ErrInvalidRequest
	}
	total := 0
	for i, m := range r.Messages {
		if (m.Role != "user" && m.Role != "assistant") || m.Content == "" || !utf8.ValidString(m.Content) || len([]byte(m.Content)) > l.MaxMessageBytes || (i > 0 && r.Messages[i-1].Role == m.Role) {
			return ErrInvalidRequest
		}
		total += len([]byte(m.Content))
		if total > l.MaxInputBytes {
			return ErrInvalidRequest
		}
	}
	if r.Messages[len(r.Messages)-1].Role != "user" {
		return ErrInvalidRequest
	}
	return nil
}

func estimateInputTokens(r Request) int {
	bytes := 0
	for _, m := range r.Messages {
		bytes += len([]byte(m.Content))
	}
	return bytes + len(r.Messages)*16
}

type ratePermit struct {
	service *Service
	keys    []string
	mark    rateMark
}

type rateMark struct {
	at time.Time
	id uint64
}

// Release rolls back an admission that did not reach the external provider.
// When admission has already reserved quota, callers reconcile it separately.
func (p *ratePermit) Release() {
	if p == nil || p.service == nil {
		return
	}
	p.service.mu.Lock()
	defer p.service.mu.Unlock()
	for _, key := range p.keys {
		entries := p.service.rates[key]
		for i := len(entries) - 1; i >= 0; i-- {
			if entries[i].id == p.mark.id {
				entries = append(entries[:i], entries[i+1:]...)
				break
			}
		}
		if len(entries) == 0 {
			delete(p.service.rates, key)
		} else {
			p.service.rates[key] = entries
		}
	}
	p.service = nil
}

func (s *Service) reserveRate(app, viewer string, l Limits, now time.Time) (*ratePermit, error) {
	if l.RateWindow <= 0 || l.ViewerRequests <= 0 || l.AppRequests <= 0 {
		return nil, ErrCapabilityUnavailable
	}
	cutoff := now.Add(-l.RateWindow)
	viewerKey, appKey := "viewer\x00"+app+"\x00"+viewer, "app\x00"+app
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneRatesLocked(now)
	for _, key := range []string{viewerKey, appKey} {
		s.rates[key] = recent(s.rates[key], cutoff)
	}
	if len(s.rates[viewerKey]) >= l.ViewerRequests || len(s.rates[appKey]) >= l.AppRequests {
		return nil, ErrRateLimited
	}
	newKeys := 0
	for _, key := range []string{viewerKey, appKey} {
		if len(s.rates[key]) == 0 {
			newKeys++
		}
	}
	if len(s.rates)+newKeys > rateStateMaxKeys {
		return nil, ErrRateLimited
	}
	s.rateSeq++
	mark := rateMark{at: now, id: s.rateSeq}
	s.rates[viewerKey] = append(s.rates[viewerKey], mark)
	s.rates[appKey] = append(s.rates[appKey], mark)
	return &ratePermit{service: s, keys: []string{viewerKey, appKey}, mark: mark}, nil
}

func (s *Service) pruneRatesLocked(now time.Time) {
	// Profiles cannot configure a rate window above absoluteLimits(). Using the
	// largest possible window makes global pruning safe across app profiles.
	cutoff := now.Add(-absoluteLimits().RateWindow)
	for key, entries := range s.rates {
		entries = recent(entries, cutoff)
		if len(entries) == 0 {
			delete(s.rates, key)
			continue
		}
		s.rates[key] = entries
	}
}
func recent(v []rateMark, cutoff time.Time) []rateMark {
	i := 0
	for i < len(v) && !v[i].at.After(cutoff) {
		i++
	}
	return append([]rateMark(nil), v[i:]...)
}

func validLimits(l Limits) bool {
	a := absoluteLimits()
	return l.MaxMessages >= 1 && l.MaxMessages <= a.MaxMessages &&
		l.MaxMessageBytes >= 1 && l.MaxMessageBytes <= a.MaxMessageBytes &&
		l.MaxInputBytes >= 1 && l.MaxInputBytes <= a.MaxInputBytes &&
		l.MaxOutputTokens >= 1 && l.MaxOutputTokens <= a.MaxOutputTokens &&
		l.Timeout >= time.Second && l.Timeout <= a.Timeout &&
		l.ViewerRequests >= 1 && l.ViewerRequests <= a.ViewerRequests &&
		l.AppRequests >= 1 && l.AppRequests <= a.AppRequests &&
		l.RateWindow >= time.Second && l.RateWindow <= a.RateWindow
}

func sameLimits(a, b Limits) bool { return a == b }

func safeLimits(l Limits) map[string]int {
	return map[string]int{
		"max_messages":      l.MaxMessages,
		"max_message_bytes": l.MaxMessageBytes,
		"max_input_bytes":   l.MaxInputBytes,
		"max_output_tokens": l.MaxOutputTokens,
		"timeout_ms":        int(l.Timeout.Milliseconds()),
		"viewer_requests":   l.ViewerRequests,
		"app_requests":      l.AppRequests,
		"rate_window_ms":    int(l.RateWindow.Milliseconds()),
	}
}

// ValidateRequest is exposed for strict route decoders and conformance tests.
func ValidateRequest(r Request, l Limits) error { return validateRequest(r, l) }
func EstimateTokens(r Request) int              { return estimateInputTokens(r) }
func (b Binding) String() string                { return fmt.Sprintf("llm binding %s", b.ProfileID) } // never includes credential
