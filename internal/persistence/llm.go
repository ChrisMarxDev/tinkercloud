package persistence

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tinyhost/tiny/internal/llm"
)

// LLMRepository owns encrypted connection records, operator-selected profiles,
// app grants, and exact usage reservations. It deliberately has no method that
// returns an operator credential or its ciphertext to a public caller.
type LLMRepository struct {
	Store    *SQLiteStore
	Envelope llm.Envelope
	Now      func() time.Time
}
type LLMConnectionInput struct {
	ID, DisplayName, ActorID string
	Provider                 llm.Provider
	Secret                   []byte
	KeyVersion               int
}
type LLMProfileInput struct {
	ID, ConnectionID, Model, ActorID    string
	Limits                              llm.Limits
	ConcurrencyLimit, MonthlyTokenLimit int
}
type LLMGrantInput struct {
	AppID, ProfileID, OperatorID, Status string
	Revision                             uint64
}

func (r LLMRepository) CreateConnectionValidated(ctx context.Context, in LLMConnectionInput, validator llm.CredentialValidator) error {
	if validator == nil || validator.ValidateCredential(ctx, in.Secret) != nil {
		return llm.ErrTemporarilyUnavailable
	}
	return r.CreateConnection(ctx, in)
}

func (r LLMRepository) now() time.Time {
	if r.Now != nil {
		return r.Now().UTC()
	}
	return time.Now().UTC()
}
func newLLMID() (string, error) {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return hex.EncodeToString(b), nil
}

// CreateConnection seals the supplied key before the database transaction.
// Neither this method nor any list/read method exposes plaintext later.
func (r LLMRepository) CreateConnection(ctx context.Context, in LLMConnectionInput) error {
	if r.Store == nil || r.Envelope == nil || in.ID == "" || in.DisplayName == "" || (in.Provider != llm.ProviderAnthropic && in.Provider != llm.ProviderGemini) || len(in.Secret) == 0 || in.KeyVersion < 1 {
		return llm.ErrCapabilityUnavailable
	}
	box, e := r.Envelope.Seal(ctx, append([]byte(nil), in.Secret...))
	if e != nil {
		return llm.ErrTemporarilyUnavailable
	}
	defer clear(in.Secret)
	now := r.now().Format(time.RFC3339Nano)
	return r.Store.Write(ctx, func(tx *sql.Tx) error {
		_, e := tx.ExecContext(ctx, "INSERT INTO provider_connections(id,display_name,provider_kind,credential_envelope,credential_key_version,status,created_at,updated_at) VALUES(?,?,?,?,?,'active',?,?)", in.ID, in.DisplayName, string(in.Provider), box, in.KeyVersion, now, now)
		if e != nil {
			return e
		}
		if in.ActorID == "" {
			return llm.ErrInvalidRequest
		}
		return r.operatorAudit(ctx, tx, in.ActorID, "llm.connection.created", "provider_connection", in.ID)
	})
}
func (r LLMRepository) CreateProfile(ctx context.Context, in LLMProfileInput) error {
	if r.Store == nil || in.ID == "" || in.ConnectionID == "" || !validModel(in.Model) || in.ConcurrencyLimit < 1 || in.MonthlyTokenLimit < 1 || !validLLMLimits(in.Limits) {
		return llm.ErrInvalidRequest
	}
	n := r.now().Format(time.RFC3339Nano)
	l := in.Limits
	return r.Store.Write(ctx, func(tx *sql.Tx) error {
		res, e := tx.ExecContext(ctx, `INSERT INTO llm_chat_profiles(id,connection_id,model,max_messages,max_message_bytes,max_input_bytes,max_output_tokens,timeout_ms,viewer_requests,app_requests,rate_window_ms,concurrency_limit,monthly_token_limit,status,revision,created_at,updated_at) SELECT ?,?,?,?,?,?,?,?,?,?,?,?,?, 'active',1,?,? WHERE EXISTS(SELECT 1 FROM provider_connections WHERE id=? AND status='active')`, in.ID, in.ConnectionID, in.Model, l.MaxMessages, l.MaxMessageBytes, l.MaxInputBytes, l.MaxOutputTokens, l.Timeout.Milliseconds(), l.ViewerRequests, l.AppRequests, l.RateWindow.Milliseconds(), in.ConcurrencyLimit, in.MonthlyTokenLimit, n, n, in.ConnectionID)
		if e != nil {
			return e
		}
		c, e := res.RowsAffected()
		if e != nil {
			return e
		}
		if c != 1 {
			return llm.ErrCapabilityUnavailable
		}
		if in.ActorID == "" {
			return llm.ErrInvalidRequest
		}
		return r.operatorAudit(ctx, tx, in.ActorID, "llm.profile.created", "llm_profile", in.ID)
	})
}
func (r LLMRepository) SetGrant(ctx context.Context, in LLMGrantInput) error {
	if r.Store == nil || in.AppID == "" || in.ProfileID == "" || (in.Status != "requested" && in.Status != "approved" && in.Status != "disabled" && in.Status != "revoked") || in.Revision == 0 {
		return llm.ErrInvalidRequest
	}
	now := r.now().Format(time.RFC3339Nano)
	return r.Store.Write(ctx, func(tx *sql.Tx) error {
		var current string
		e := tx.QueryRowContext(ctx, `SELECT status FROM app_capability_grants WHERE app_id=? AND capability='llm.chat' AND version=1`, in.AppID).Scan(&current)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		if current == "revoked" {
			return llm.ErrCapabilityUnavailable
		}
		result, e := tx.ExecContext(ctx, `INSERT INTO app_capability_grants(app_id,capability,version,profile_id,status,revision,approved_by,created_at,updated_at) VALUES(?,'llm.chat',1,?,?,?,?,?,?) ON CONFLICT(app_id,capability,version) DO UPDATE SET profile_id=excluded.profile_id,status=excluded.status,revision=excluded.revision,approved_by=excluded.approved_by,updated_at=excluded.updated_at`, in.AppID, in.ProfileID, in.Status, in.Revision, nullIfEmpty(in.OperatorID), now, now)
		if e != nil {
			return e
		}
		changed, e := result.RowsAffected()
		if e != nil || changed != 1 {
			return llm.ErrCapabilityUnavailable
		}
		return nil
	})
}
func (r LLMRepository) DisableConnection(ctx context.Context, id string) error {
	if r.Store == nil || id == "" {
		return llm.ErrInvalidRequest
	}
	return r.Store.Write(ctx, func(tx *sql.Tx) error {
		_, e := tx.ExecContext(ctx, "UPDATE provider_connections SET status='disabled',updated_at=? WHERE id=? AND status='active'", r.now().Format(time.RFC3339Nano), id)
		return e
	})
}
func (r LLMRepository) Available(ctx context.Context, appID string) bool {
	_, err := r.AdmissionLimits(ctx, appID)
	return err == nil
}

// AdmissionLimits exposes only the effective active profile limits needed for
// safe browser discovery and pre-admission rate limiting. Provider identity,
// model, grant metadata, and credentials never leave this repository method.
func (r LLMRepository) AdmissionLimits(ctx context.Context, appID string) (llm.Limits, error) {
	if r.Store == nil || appID == "" {
		return llm.Limits{}, llm.ErrCapabilityUnavailable
	}
	var limits llm.Limits
	var timeoutMS, windowMS int64
	err := r.Store.DB.QueryRowContext(ctx, `SELECT p.max_messages,p.max_message_bytes,p.max_input_bytes,p.max_output_tokens,p.timeout_ms,p.viewer_requests,p.app_requests,p.rate_window_ms FROM app_capability_grants g JOIN llm_chat_profiles p ON p.id=g.profile_id JOIN provider_connections c ON c.id=p.connection_id JOIN applications a ON a.id=g.app_id WHERE g.app_id=? AND g.capability='llm.chat' AND g.version=1 AND g.status='approved' AND p.status='active' AND c.status='active' AND a.status='active'`, appID).Scan(&limits.MaxMessages, &limits.MaxMessageBytes, &limits.MaxInputBytes, &limits.MaxOutputTokens, &timeoutMS, &limits.ViewerRequests, &limits.AppRequests, &windowMS)
	if err != nil {
		return llm.Limits{}, llm.ErrCapabilityUnavailable
	}
	limits.Timeout = time.Duration(timeoutMS) * time.Millisecond
	limits.RateWindow = time.Duration(windowMS) * time.Millisecond
	if !validLLMLimits(limits) {
		return llm.Limits{}, llm.ErrCapabilityUnavailable
	}
	return limits, nil
}

func (r LLMRepository) Admit(ctx context.Context, appID, identityID string, inputTokens, requestedOutput int, now time.Time) (llm.Binding, error) {
	if r.Store == nil || r.Envelope == nil {
		return llm.Binding{}, llm.ErrCapabilityUnavailable
	}
	if appID == "" || identityID == "" || inputTokens < 1 || requestedOutput < 0 {
		return llm.Binding{}, llm.ErrUnauthorized
	}
	var b llm.Binding
	var box []byte
	var concurrency, budget int
	period := monthStart(now)
	rid, e := newLLMID()
	if e != nil {
		return b, llm.ErrTemporarilyUnavailable
	}
	e = r.Store.Write(ctx, func(tx *sql.Tx) error {
		var timeoutMS, windowMS int64
		err := tx.QueryRowContext(ctx, `SELECT g.app_id,g.profile_id,p.connection_id,p.model,c.provider_kind,c.credential_envelope,p.max_messages,p.max_message_bytes,p.max_input_bytes,p.max_output_tokens,p.timeout_ms,p.viewer_requests,p.app_requests,p.rate_window_ms,p.concurrency_limit,p.monthly_token_limit FROM app_capability_grants g JOIN llm_chat_profiles p ON p.id=g.profile_id JOIN provider_connections c ON c.id=p.connection_id JOIN applications a ON a.id=g.app_id WHERE g.app_id=? AND g.capability='llm.chat' AND g.version=1 AND g.status='approved' AND p.status='active' AND c.status='active' AND a.status='active'`, appID).Scan(&b.AppID, &b.ProfileID, &b.ConnectionID, &b.Model, &b.Provider, &box, &b.Limits.MaxMessages, &b.Limits.MaxMessageBytes, &b.Limits.MaxInputBytes, &b.Limits.MaxOutputTokens, &timeoutMS, &b.Limits.ViewerRequests, &b.Limits.AppRequests, &windowMS, &concurrency, &budget)
		if err != nil {
			return llm.ErrCapabilityUnavailable
		}
		b.Limits.Timeout = time.Duration(timeoutMS) * time.Millisecond
		b.Limits.RateWindow = time.Duration(windowMS) * time.Millisecond
		if !validLLMLimits(b.Limits) || concurrency < 1 || budget < 1 || !validModel(b.Model) {
			return llm.ErrCapabilityUnavailable
		}
		output := b.Limits.MaxOutputTokens
		if requestedOutput > 0 && requestedOutput < output {
			output = requestedOutput
		}
		reserve := inputTokens + output
		n := now.UTC().Format(time.RFC3339Nano)
		if _, err = tx.ExecContext(ctx, "INSERT INTO llm_usage(app_id,profile_id,period_start,used_tokens,reserved_tokens,in_flight,updated_at) VALUES(?,?,?,0,0,0,?) ON CONFLICT(app_id,profile_id,period_start) DO NOTHING", appID, b.ProfileID, period, n); err != nil {
			return err
		}
		var used, reserved, inflight int
		if err = tx.QueryRowContext(ctx, "SELECT used_tokens,reserved_tokens,in_flight FROM llm_usage WHERE app_id=? AND profile_id=? AND period_start=?", appID, b.ProfileID, period).Scan(&used, &reserved, &inflight); err != nil {
			return err
		}
		if used+reserved+reserve > budget {
			return llm.ErrQuotaExhausted
		}
		if inflight >= concurrency {
			return llm.ErrRateLimited
		}
		if _, err = tx.ExecContext(ctx, "UPDATE llm_usage SET reserved_tokens=reserved_tokens+?,in_flight=in_flight+1,updated_at=? WHERE app_id=? AND profile_id=? AND period_start=?", reserve, n, appID, b.ProfileID, period); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO llm_reservations(id,app_id,profile_id,identity_id,period_start,reserved_tokens,status,created_at) VALUES(?,?,?,?,?,?, 'calling',?)", rid, appID, b.ProfileID, identityID, period, reserve, n); err != nil {
			return err
		}
		b.ReservationID = rid
		b.ReservedTokens = reserve
		return nil
	})
	if e != nil {
		return llm.Binding{}, e
	}
	secret, e := r.Envelope.Open(ctx, box)
	if e != nil {
		_ = r.Reconcile(context.Background(), b, llm.Usage{}, llm.OutcomeFailed, now)
		return llm.Binding{}, llm.ErrTemporarilyUnavailable
	}
	b.Credential = secret
	return b, nil
}
func (r LLMRepository) Reconcile(ctx context.Context, b llm.Binding, usage llm.Usage, outcome llm.Outcome, now time.Time) error {
	if r.Store == nil || b.AppID == "" || b.ProfileID == "" || b.ReservationID == "" || b.ReservedTokens < 1 {
		return llm.ErrTemporarilyUnavailable
	}
	return r.Store.Write(ctx, func(tx *sql.Tx) error {
		var period, status, identity string
		var reserve int
		if e := tx.QueryRowContext(ctx, "SELECT period_start,status,identity_id,reserved_tokens FROM llm_reservations WHERE id=? AND app_id=? AND profile_id=?", b.ReservationID, b.AppID, b.ProfileID).Scan(&period, &status, &identity, &reserve); e != nil || status != "calling" {
			return llm.ErrTemporarilyUnavailable
		}
		if usage.InputTokens < 0 || usage.OutputTokens < 0 {
			return llm.ErrTemporarilyUnavailable
		}
		n := now.UTC().Format(time.RFC3339Nano)
		used := usage.InputTokens + usage.OutputTokens
		var q string
		var result sql.Result
		var e error
		switch outcome {
		case llm.OutcomeSucceeded:
			q = "UPDATE llm_usage SET used_tokens=used_tokens+?,reserved_tokens=reserved_tokens-?,in_flight=in_flight-1,updated_at=? WHERE app_id=? AND profile_id=? AND period_start=?"
			result, e = tx.ExecContext(ctx, q, used, reserve, n, b.AppID, b.ProfileID, period)
		case llm.OutcomeFailed, llm.OutcomeCancelled:
			q = "UPDATE llm_usage SET reserved_tokens=reserved_tokens-?,in_flight=in_flight-1,updated_at=? WHERE app_id=? AND profile_id=? AND period_start=?"
			result, e = tx.ExecContext(ctx, q, reserve, n, b.AppID, b.ProfileID, period)
		case llm.OutcomeAmbiguous:
			q = "UPDATE llm_usage SET in_flight=in_flight-1,updated_at=? WHERE app_id=? AND profile_id=? AND period_start=?"
			result, e = tx.ExecContext(ctx, q, n, b.AppID, b.ProfileID, period)
		default:
			return llm.ErrTemporarilyUnavailable
		}
		if e != nil {
			return e
		}
		changed, e := result.RowsAffected()
		if e != nil || changed != 1 {
			return llm.ErrTemporarilyUnavailable
		}
		if _, e := tx.ExecContext(ctx, "UPDATE llm_reservations SET status=?,finished_at=? WHERE id=? AND status='calling'", string(outcome), n, b.ReservationID); e != nil {
			return e
		}
		meta := fmt.Sprintf(`{"input_tokens":%d,"output_tokens":%d}`, usage.InputTokens, usage.OutputTokens)
		_, e = tx.ExecContext(ctx, "INSERT INTO audit_events(id,occurred_at,actor_kind,actor_id,app_id,action,outcome,target_kind,target_id,metadata_json) VALUES(?,?,?,?,?,'llm.chat.complete',?,'llm_profile',?,?)", b.ReservationID+"-audit", n, "viewer", identity, b.AppID, string(outcome), b.ProfileID, meta)
		return e
	})
}
func monthStart(t time.Time) string {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
}
func validModel(s string) bool {
	return s != "" && len(s) <= 128 && !strings.ContainsAny(s, "\x00\r\n")
}
func validLLMLimits(l llm.Limits) bool {
	return l.MaxMessages >= 1 && l.MaxMessages <= 128 && l.MaxMessageBytes >= 1 && l.MaxMessageBytes <= 262144 && l.MaxInputBytes >= 1 && l.MaxInputBytes <= 1048576 && l.MaxOutputTokens >= 1 && l.MaxOutputTokens <= 16384 && l.Timeout >= time.Second && l.Timeout <= 120*time.Second && l.ViewerRequests >= 1 && l.ViewerRequests <= 10000 && l.AppRequests >= 1 && l.AppRequests <= 100000 && l.RateWindow >= time.Second && l.RateWindow <= time.Hour
}
