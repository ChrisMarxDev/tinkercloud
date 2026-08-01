package persistence

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/ChrisMarxDev/tinkercloud/internal/llm"
)

func llmLimits() llm.Limits {
	return llm.Limits{MaxMessages: 4, MaxMessageBytes: 100, MaxInputBytes: 200, MaxOutputTokens: 10, Timeout: time.Second, ViewerRequests: 2, AppRequests: 3, RateWindow: time.Minute}
}
func llmRepo(t *testing.T) (LLMRepository, *SQLiteStore) {
	t.Helper()
	s := seeded(t)
	env, e := llm.NewAESGCMEnvelope(make([]byte, 32))
	if e != nil {
		t.Fatal(e)
	}
	return LLMRepository{Store: s, Envelope: env}, s
}
func setupLLM(t *testing.T, r LLMRepository) {
	t.Helper()
	ctx := context.Background()
	if e := r.CreateConnection(ctx, LLMConnectionInput{ID: "conn", DisplayName: "operator connection", Provider: llm.ProviderAnthropic, Secret: []byte("super-secret"), KeyVersion: 1, ActorID: "u"}); e != nil {
		t.Fatal(e)
	}
	if e := r.CreateProfile(ctx, LLMProfileInput{ID: "profile", ConnectionID: "conn", Model: "claude-test", Limits: llmLimits(), ConcurrencyLimit: 1, MonthlyTokenLimit: 100, ActorID: "u"}); e != nil {
		t.Fatal(e)
	}
	if e := r.SetGrant(ctx, LLMGrantInput{AppID: "a", ProfileID: "profile", OperatorID: "u", Status: "approved", Revision: 1}); e != nil {
		t.Fatal(e)
	}
}
func TestLLMEncryptedConnectionAndAdmission(t *testing.T) {
	r, s := llmRepo(t)
	defer s.Close()
	setupLLM(t, r)
	var box []byte
	if e := s.DB.QueryRow("SELECT credential_envelope FROM provider_connections WHERE id='conn'").Scan(&box); e != nil || string(box) == "super-secret" {
		t.Fatalf("ciphertext=%q err=%v", box, e)
	}
	b, e := r.Admit(context.Background(), "a", "i", 1, 9, time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC))
	if e != nil || string(b.Credential) != "super-secret" {
		t.Fatalf("binding=%+v err=%v", b, e)
	}
	if e = r.Reconcile(context.Background(), b, llm.Usage{InputTokens: 2, OutputTokens: 3}, llm.OutcomeSucceeded, time.Now()); e != nil {
		t.Fatal(e)
	}
	var used, reserved, flight int
	if e = s.DB.QueryRow("SELECT used_tokens,reserved_tokens,in_flight FROM llm_usage").Scan(&used, &reserved, &flight); e != nil || used != 5 || reserved != 0 || flight != 0 {
		t.Fatalf("usage %d %d %d %v", used, reserved, flight, e)
	}
	var meta string
	if e = s.DB.QueryRow("SELECT metadata_json FROM audit_events WHERE action='llm.chat.complete'").Scan(&meta); e != nil || meta == "" {
		t.Fatal(meta, e)
	}
}
func TestLLMGrantAndBudgetDenyBeforeDecrypt(t *testing.T) {
	r, s := llmRepo(t)
	defer s.Close()
	setupLLM(t, r)
	if _, e := s.DB.Exec("UPDATE app_capability_grants SET status='revoked'"); e != nil {
		t.Fatal(e)
	}
	if _, e := r.Admit(context.Background(), "a", "i", 1, 1, time.Now()); !errors.Is(e, llm.ErrCapabilityUnavailable) {
		t.Fatalf("revoked=%v", e)
	}
	if e := r.SetGrant(context.Background(), LLMGrantInput{AppID: "a", ProfileID: "profile", OperatorID: "u", Status: "approved", Revision: 2}); !errors.Is(e, llm.ErrCapabilityUnavailable) {
		t.Fatalf("terminal revoke set=%v", e)
	}
	if e := r.UpdateGrant(context.Background(), LLMGrantInput{AppID: "a", ProfileID: "profile", OperatorID: "u", Status: "approved"}, 1); e == nil {
		t.Fatal("current control update revived a revoked grant")
	}
	var status string
	if e := s.DB.QueryRow("SELECT status FROM app_capability_grants WHERE app_id='a'").Scan(&status); e != nil || status != "revoked" {
		t.Fatalf("grant status=%q err=%v", status, e)
	}
}
func TestLLMConcurrencyAndAmbiguousReservation(t *testing.T) {
	r, s := llmRepo(t)
	defer s.Close()
	setupLLM(t, r)
	b, e := r.Admit(context.Background(), "a", "i", 1, 9, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	if _, e := r.Admit(context.Background(), "a", "i", 1, 9, time.Now()); !errors.Is(e, llm.ErrRateLimited) {
		t.Fatalf("concurrency %v", e)
	}
	if e := r.Reconcile(context.Background(), b, llm.Usage{}, llm.OutcomeAmbiguous, time.Now()); e != nil {
		t.Fatal(e)
	}
	if _, e := r.Admit(context.Background(), "a", "i", 90, 1, time.Now()); !errors.Is(e, llm.ErrQuotaExhausted) {
		t.Fatalf("ambiguous reservation released: %v", e)
	}
}
func TestLLMAdmissionUsesProfileOutputWhenRequestOmitsIt(t *testing.T) {
	r, s := llmRepo(t)
	defer s.Close()
	l := llmLimits()
	l.MaxOutputTokens = 5
	ctx := context.Background()
	if e := r.CreateConnection(ctx, LLMConnectionInput{ID: "conn", DisplayName: "operator connection", Provider: llm.ProviderAnthropic, Secret: []byte("super-secret"), KeyVersion: 1, ActorID: "u"}); e != nil {
		t.Fatal(e)
	}
	if e := r.CreateProfile(ctx, LLMProfileInput{ID: "profile", ConnectionID: "conn", Model: "claude-test", Limits: l, ConcurrencyLimit: 1, MonthlyTokenLimit: 6, ActorID: "u"}); e != nil {
		t.Fatal(e)
	}
	if e := r.SetGrant(ctx, LLMGrantInput{AppID: "a", ProfileID: "profile", OperatorID: "u", Status: "approved", Revision: 1}); e != nil {
		t.Fatal(e)
	}
	b, e := r.Admit(ctx, "a", "i", 1, 0, time.Now())
	if e != nil || b.ReservedTokens != 6 {
		t.Fatalf("admission=%+v err=%v", b, e)
	}
}
func TestLLMAuditFailureKeepsConservativeReservation(t *testing.T) {
	r, s := llmRepo(t)
	defer s.Close()
	setupLLM(t, r)
	b, e := r.Admit(context.Background(), "a", "i", 1, 9, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.DB.Exec("CREATE TRIGGER deny_llm_audit BEFORE INSERT ON audit_events WHEN NEW.action='llm.chat.complete' BEGIN SELECT RAISE(FAIL,'test'); END"); e != nil {
		t.Fatal(e)
	}
	if e = r.Reconcile(context.Background(), b, llm.Usage{InputTokens: 1, OutputTokens: 1}, llm.OutcomeSucceeded, time.Now()); e == nil {
		t.Fatal("audit failure accepted")
	}
	var reserved, flight int
	if e = s.DB.QueryRow("SELECT reserved_tokens,in_flight FROM llm_usage").Scan(&reserved, &flight); e != nil || reserved != 10 || flight != 1 {
		t.Fatalf("audit failure was not atomic %d %d %v", reserved, flight, e)
	}
}
func TestLLMUsageAboveReservationStillReleasesConcurrency(t *testing.T) {
	r, s := llmRepo(t)
	defer s.Close()
	setupLLM(t, r)
	b, e := r.Admit(context.Background(), "a", "i", 1, 1, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	if e = r.Reconcile(context.Background(), b, llm.Usage{InputTokens: b.ReservedTokens + 1, OutputTokens: 0}, llm.OutcomeSucceeded, time.Now()); e != nil {
		t.Fatal(e)
	}
	var reserved, flight int
	if e = s.DB.QueryRow("SELECT reserved_tokens,in_flight FROM llm_usage").Scan(&reserved, &flight); e != nil || reserved != 0 || flight != 0 {
		t.Fatalf("stuck usage reserved=%d flight=%d err=%v", reserved, flight, e)
	}
}

func TestLLMAdmissionFailsClosedWithoutEncryptionRootOrWriteContext(t *testing.T) {
	r, s := llmRepo(t)
	defer s.Close()
	setupLLM(t, r)
	withoutRoot := r
	withoutRoot.Envelope = nil
	if _, err := withoutRoot.Admit(context.Background(), "a", "i", 1, 1, time.Now()); !errors.Is(err, llm.ErrCapabilityUnavailable) {
		t.Fatalf("missing root err=%v", err)
	}
	var reservations int
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM llm_reservations").Scan(&reservations); err != nil || reservations != 0 {
		t.Fatalf("missing root reserved=%d err=%v", reservations, err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.Admit(cancelled, "a", "i", 1, 1, time.Now()); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled write err=%v", err)
	}
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM llm_reservations").Scan(&reservations); err != nil || reservations != 0 {
		t.Fatalf("cancelled admission reserved=%d err=%v", reservations, err)
	}
	started, release := make(chan struct{}), make(chan struct{})
	writeDone := make(chan error, 1)
	go func() {
		writeDone <- s.Write(context.Background(), func(*sql.Tx) error {
			close(started)
			<-release
			return nil
		})
	}()
	<-started
	busy, busyCancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer busyCancel()
	if _, err := r.Admit(busy, "a", "i", 1, 1, time.Now()); !errors.Is(err, context.DeadlineExceeded) {
		close(release)
		t.Fatalf("busy writer err=%v", err)
	}
	close(release)
	if err := <-writeDone; err != nil {
		t.Fatal(err)
	}
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM llm_reservations").Scan(&reservations); err != nil || reservations != 0 {
		t.Fatalf("busy admission reserved=%d err=%v", reservations, err)
	}
}
