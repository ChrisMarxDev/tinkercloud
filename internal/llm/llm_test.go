package llm

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ChrisMarxDev/tinkercloud/internal/appauth"
	"github.com/ChrisMarxDev/tinkercloud/internal/apps"
	"github.com/ChrisMarxDev/tinkercloud/internal/identity"
	"github.com/ChrisMarxDev/tinkercloud/internal/policies"
	"github.com/ChrisMarxDev/tinkercloud/internal/sessions"
)

func authWithLLM(t *testing.T, app, viewer string, requested bool) appauth.AuthorizationContext {
	t.Helper()
	ss := sessions.NewMemoryStore()
	tok, _, e := ss.Create(app, identity.Identity{ID: viewer, Email: viewer + "@example.test"}, time.Now().Add(time.Hour))
	if e != nil {
		t.Fatal(e)
	}
	a, e := (appauth.Authorizer{Sessions: ss, Policies: &policies.MemoryStore{Policies: map[string]policies.Policy{app: {AppID: app, OwnerIdentityID: viewer, Valid: true}}}}).Authorize(context.Background(), apps.App{ID: app, LLMChatRequested: requested}, tok, "request")
	if e != nil {
		t.Fatal(e)
	}
	return a
}

func auth(t *testing.T, app, viewer string) appauth.AuthorizationContext {
	return authWithLLM(t, app, viewer, true)
}

type fakeRepo struct {
	mu          sync.Mutex
	calls, done int
	binding     Binding
	admitErr    error
	outcomes    []Outcome
}

func (r *fakeRepo) AdmissionLimits(_ context.Context, _ string) (Limits, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.admitErr != nil {
		return Limits{}, r.admitErr
	}
	return r.binding.Limits, nil
}

func (r *fakeRepo) Admit(_ context.Context, app, viewer string, input, output int, _ time.Time) (Binding, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if r.admitErr != nil {
		return Binding{}, r.admitErr
	}
	b := r.binding
	b.AppID = app
	b.ReservationID = "r"
	b.ReservedTokens = input + output
	return b, nil
}
func (r *fakeRepo) Reconcile(_ context.Context, _ Binding, _ Usage, o Outcome, _ time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.done++
	r.outcomes = append(r.outcomes, o)
	return nil
}

type fakeAdapter struct {
	response Response
	err      error
	calls    int
}

func (a *fakeAdapter) Complete(ctx context.Context, _ Binding, _ Request) (Response, error) {
	a.calls++
	if a.err != nil {
		return Response{}, a.err
	}
	return a.response, nil
}
func limits() Limits {
	return Limits{MaxMessages: 4, MaxMessageBytes: 100, MaxInputBytes: 200, MaxOutputTokens: 12, Timeout: time.Second, ViewerRequests: 2, AppRequests: 3, RateWindow: time.Minute}
}
func TestInvalidInputNeverReachesRepository(t *testing.T) {
	r := &fakeRepo{}
	s := New(r, nil)
	_, e := s.Complete(context.Background(), auth(t, "a", "v"), Request{Messages: []Message{{Role: "assistant", Content: "no"}}})
	if !errors.Is(e, ErrInvalidRequest) || r.calls != 0 {
		t.Fatalf("err=%v calls=%d", e, r.calls)
	}
}

func TestUnrequestedChatIsUnavailableForAuthorizedViewerButNilAuthIsUnauthorized(t *testing.T) {
	r := &fakeRepo{}
	s := New(r, nil)
	request := Request{Messages: []Message{{Role: "user", Content: "hi"}}}
	if _, err := s.Complete(context.Background(), authWithLLM(t, "a", "v", false), request); !errors.Is(err, ErrCapabilityUnavailable) {
		t.Fatalf("authorized unrequested chat error=%v", err)
	}
	if r.calls != 0 {
		t.Fatalf("unrequested chat reached repository %d times", r.calls)
	}
	if _, err := s.Complete(context.Background(), nil, request); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("nil auth error=%v", err)
	}
}
func TestCompletionUsesDerivedScopeAndReconciles(t *testing.T) {
	r := &fakeRepo{binding: Binding{ProfileID: "p", ConnectionID: "c", Credential: []byte("secret"), Provider: ProviderAnthropic, Model: "m", Limits: limits()}}
	a := &fakeAdapter{response: Response{Message: MessageResponse{Role: "assistant", Content: "ok"}, Usage: Usage{InputTokens: 1, OutputTokens: 2}, FinishReason: "stop"}}
	s := New(r, map[Provider]Adapter{ProviderAnthropic: a})
	out, e := s.Complete(context.Background(), auth(t, "app-a", "viewer-a"), Request{Messages: []Message{{Role: "user", Content: "hi"}}})
	if e != nil || out.RequestID != "request" || a.calls != 1 || r.done != 1 || r.outcomes[0] != OutcomeSucceeded {
		t.Fatalf("out=%+v err=%v calls=%d outcomes=%v", out, e, a.calls, r.outcomes)
	}
}
func TestProviderFailureNeverLeaksAndReleasesReservation(t *testing.T) {
	r := &fakeRepo{binding: Binding{ProfileID: "p", ConnectionID: "c", Credential: []byte("secret"), Provider: ProviderGemini, Model: "m", Limits: limits()}}
	s := New(r, map[Provider]Adapter{ProviderGemini: &fakeAdapter{err: errors.New("key=secret upstream body")}})
	_, e := s.Complete(context.Background(), auth(t, "a", "v"), Request{Messages: []Message{{Role: "user", Content: "hi"}}})
	if !errors.Is(e, ErrTemporarilyUnavailable) || r.done != 1 || r.outcomes[0] != OutcomeFailed {
		t.Fatalf("err=%v outcomes=%v", e, r.outcomes)
	}
}
func TestCancellationRetainsConservativeReservation(t *testing.T) {
	r := &fakeRepo{binding: Binding{ProfileID: "p", ConnectionID: "c", Credential: []byte("secret"), Provider: ProviderGemini, Model: "m", Limits: limits()}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := New(r, map[Provider]Adapter{ProviderGemini: &fakeAdapter{err: context.Canceled}})
	_, e := s.Complete(ctx, auth(t, "a", "v"), Request{Messages: []Message{{Role: "user", Content: "hi"}}})
	if !errors.Is(e, ErrCancelled) || r.outcomes[0] != OutcomeAmbiguous {
		t.Fatalf("err=%v outcomes=%v", e, r.outcomes)
	}
}
func TestProfileMayUseBoundsAboveDefaults(t *testing.T) {
	l := limits()
	l.MaxMessages = 64
	l.MaxOutputTokens = 5000
	l.MaxMessageBytes = 1024
	l.MaxInputBytes = 64000
	r := &fakeRepo{binding: Binding{ProfileID: "p", ConnectionID: "c", Credential: []byte("secret"), Provider: ProviderAnthropic, Model: "m", Limits: l}}
	a := &fakeAdapter{response: Response{Message: MessageResponse{Role: "assistant", Content: "ok"}, Usage: Usage{InputTokens: 1, OutputTokens: 2}, FinishReason: "stop"}}
	s := New(r, map[Provider]Adapter{ProviderAnthropic: a})
	messages := make([]Message, 33)
	for i := range messages {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		messages[i] = Message{Role: role, Content: "x"}
	}
	if _, e := s.Complete(context.Background(), auth(t, "a", "v"), Request{Messages: messages, MaxOutputTokens: 5000}); e != nil {
		t.Fatal(e)
	}
}

func TestRateRejectionPrecedesDurableAdmission(t *testing.T) {
	l := limits()
	l.ViewerRequests = 1
	r := &fakeRepo{binding: Binding{ProfileID: "p", ConnectionID: "c", Credential: []byte("secret"), Provider: ProviderAnthropic, Model: "m", Limits: l}}
	a := &fakeAdapter{response: Response{Message: MessageResponse{Role: "assistant", Content: "ok"}, Usage: Usage{InputTokens: 1, OutputTokens: 1}, FinishReason: "stop"}}
	s := New(r, map[Provider]Adapter{ProviderAnthropic: a})
	request := Request{Messages: []Message{{Role: "user", Content: "hi"}}}
	if _, err := s.Complete(context.Background(), auth(t, "a", "v"), request); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Complete(context.Background(), auth(t, "a", "v"), request); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("second request = %v", err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.calls != 1 {
		t.Fatalf("rate denial created %d durable reservations", r.calls)
	}
}

func TestRateStateIsGloballyBoundedAndPruned(t *testing.T) {
	s := New(&fakeRepo{}, nil)
	l := limits()
	l.ViewerRequests, l.AppRequests = 2, rateStateMaxKeys+1
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	for i := 0; i < rateStateMaxKeys; i++ {
		if _, err := s.reserveRate("a", fmt.Sprintf("viewer-%d", i), l, now); err != nil {
			break
		}
	}
	s.mu.Lock()
	entries := len(s.rates)
	s.mu.Unlock()
	if entries > rateStateMaxKeys {
		t.Fatalf("rate state grew to %d keys", entries)
	}
	if _, err := s.reserveRate("new-app", "fresh-viewer", l, now.Add(2*time.Hour)); err != nil {
		t.Fatalf("expired global state was not pruned: %v", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.rates) != 2 {
		t.Fatalf("expected only new keys after pruning, got %d", len(s.rates))
	}
}

func TestRatePermitReleaseDoesNotRemoveAnotherConcurrentAdmission(t *testing.T) {
	s := New(&fakeRepo{}, nil)
	l := limits()
	l.ViewerRequests, l.AppRequests = 1, 10
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	first, err := s.reserveRate("a", "one", l, now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.reserveRate("a", "two", l, now)
	if err != nil {
		t.Fatal(err)
	}
	first.Release()
	if _, err := s.reserveRate("a", "two", l, now); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("second admission was lost: %v", err)
	}
	second.Release()
	if _, err := s.reserveRate("a", "two", l, now); err != nil {
		t.Fatalf("released admission remained: %v", err)
	}
}
