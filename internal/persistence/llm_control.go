package persistence

import (
	"context"
	"database/sql"
	"time"

	"github.com/tinyhost/tiny/internal/controlapi"
	"github.com/tinyhost/tiny/internal/llm"
)

// LLMConnectionView is deliberately credential-free metadata for an operator
// dashboard. The encrypted envelope is never selected by this read path.
type LLMConnectionView struct {
	ID, DisplayName, Provider, Status, CreatedAt, UpdatedAt string
}
type LLMProfileView struct {
	ID, ConnectionID, Model, Status, CreatedAt, UpdatedAt string
	Revision                                              uint64
	Limits                                                llm.Limits
	ConcurrencyLimit, MonthlyTokenLimit                   int
}
type LLMGrantView struct {
	AppID, AppSlug, ProfileID, Status, UpdatedAt string
	Revision                                     uint64
	UsedTokens, ReservedTokens, InFlight         int
}

func (r LLMRepository) OperatorViews(ctx context.Context) ([]LLMConnectionView, []LLMProfileView, []LLMGrantView, error) {
	if r.Store == nil {
		return nil, nil, nil, llm.ErrCapabilityUnavailable
	}
	connections := []LLMConnectionView{}
	rows, err := r.Store.DB.QueryContext(ctx, `SELECT id,display_name,provider_kind,status,created_at,updated_at FROM provider_connections ORDER BY display_name,id LIMIT 100`)
	if err != nil {
		return nil, nil, nil, err
	}
	for rows.Next() {
		var v LLMConnectionView
		if err := rows.Scan(&v.ID, &v.DisplayName, &v.Provider, &v.Status, &v.CreatedAt, &v.UpdatedAt); err != nil {
			rows.Close()
			return nil, nil, nil, err
		}
		connections = append(connections, v)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, nil, nil, err
	}
	rows.Close()
	profiles := []LLMProfileView{}
	rows, err = r.Store.DB.QueryContext(ctx, `SELECT id,connection_id,model,max_messages,max_message_bytes,max_input_bytes,max_output_tokens,timeout_ms,viewer_requests,app_requests,rate_window_ms,concurrency_limit,monthly_token_limit,status,revision,created_at,updated_at FROM llm_chat_profiles ORDER BY created_at DESC,id LIMIT 100`)
	if err != nil {
		return nil, nil, nil, err
	}
	for rows.Next() {
		var v LLMProfileView
		var timeout, window int64
		if err := rows.Scan(&v.ID, &v.ConnectionID, &v.Model, &v.Limits.MaxMessages, &v.Limits.MaxMessageBytes, &v.Limits.MaxInputBytes, &v.Limits.MaxOutputTokens, &timeout, &v.Limits.ViewerRequests, &v.Limits.AppRequests, &window, &v.ConcurrencyLimit, &v.MonthlyTokenLimit, &v.Status, &v.Revision, &v.CreatedAt, &v.UpdatedAt); err != nil {
			rows.Close()
			return nil, nil, nil, err
		}
		v.Limits.Timeout = time.Duration(timeout) * time.Millisecond
		v.Limits.RateWindow = time.Duration(window) * time.Millisecond
		profiles = append(profiles, v)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, nil, nil, err
	}
	rows.Close()
	grants := []LLMGrantView{}
	rows, err = r.Store.DB.QueryContext(ctx, `SELECT g.app_id,a.slug,g.profile_id,g.status,g.revision,g.updated_at,COALESCE(u.used_tokens,0),COALESCE(u.reserved_tokens,0),COALESCE(u.in_flight,0) FROM app_capability_grants g JOIN applications a ON a.id=g.app_id LEFT JOIN llm_usage u ON u.app_id=g.app_id AND u.profile_id=g.profile_id AND u.period_start=? ORDER BY a.slug LIMIT 100`, monthStart(r.now()))
	if err != nil {
		return nil, nil, nil, err
	}
	for rows.Next() {
		var v LLMGrantView
		if err := rows.Scan(&v.AppID, &v.AppSlug, &v.ProfileID, &v.Status, &v.Revision, &v.UpdatedAt, &v.UsedTokens, &v.ReservedTokens, &v.InFlight); err != nil {
			rows.Close()
			return nil, nil, nil, err
		}
		grants = append(grants, v)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, nil, err
	}
	return connections, profiles, grants, nil
}

func (r LLMRepository) RotateConnection(ctx context.Context, id string, secret []byte, keyVersion int, actor string) error {
	if r.Store == nil || r.Envelope == nil || id == "" || len(secret) == 0 || keyVersion < 1 || actor == "" {
		return llm.ErrInvalidRequest
	}
	box, err := r.Envelope.Seal(ctx, append([]byte(nil), secret...))
	clear(secret)
	if err != nil {
		return llm.ErrTemporarilyUnavailable
	}
	return r.Store.Write(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `UPDATE provider_connections SET credential_envelope=?,credential_key_version=?,status='active',updated_at=? WHERE id=? AND status IN ('active','disabled','validation_failed')`, box, keyVersion, r.now().Format(time.RFC3339Nano), id)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil || n != 1 {
			return llm.ErrCapabilityUnavailable
		}
		return r.operatorAudit(ctx, tx, actor, "llm.connection.rotated", "provider_connection", id)
	})
}

// ConnectionProvider resolves the provider already bound to a connection. It
// intentionally returns no credential material and is used before rotation so
// a browser cannot mislabel a replacement key as another provider's key.
func (r LLMRepository) ConnectionProvider(ctx context.Context, id string) (llm.Provider, error) {
	if r.Store == nil || id == "" {
		return "", llm.ErrCapabilityUnavailable
	}
	var provider string
	if err := r.Store.DB.QueryRowContext(ctx, `SELECT provider_kind FROM provider_connections WHERE id=? AND status IN ('active','disabled','validation_failed')`, id).Scan(&provider); err != nil {
		return "", llm.ErrCapabilityUnavailable
	}
	p := llm.Provider(provider)
	if p != llm.ProviderAnthropic && p != llm.ProviderGemini {
		return "", llm.ErrCapabilityUnavailable
	}
	return p, nil
}

func (r LLMRepository) DisableConnectionAs(ctx context.Context, id, actor string) error {
	if r.Store == nil || id == "" || actor == "" {
		return llm.ErrInvalidRequest
	}
	return r.Store.Write(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `UPDATE provider_connections SET status='disabled',updated_at=? WHERE id=? AND status='active'`, r.now().Format(time.RFC3339Nano), id)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil || n != 1 {
			return llm.ErrCapabilityUnavailable
		}
		return r.operatorAudit(ctx, tx, actor, "llm.connection.disabled", "provider_connection", id)
	})
}

func (r LLMRepository) UpdateProfile(ctx context.Context, in LLMProfileInput, expected uint64) error {
	if r.Store == nil || in.ID == "" || in.ActorID == "" || expected < 1 || !validModel(in.Model) || in.ConcurrencyLimit < 1 || in.MonthlyTokenLimit < 1 || !validLLMLimits(in.Limits) {
		return llm.ErrInvalidRequest
	}
	l := in.Limits
	return r.Store.Write(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `UPDATE llm_chat_profiles SET connection_id=?,model=?,max_messages=?,max_message_bytes=?,max_input_bytes=?,max_output_tokens=?,timeout_ms=?,viewer_requests=?,app_requests=?,rate_window_ms=?,concurrency_limit=?,monthly_token_limit=?,revision=revision+1,updated_at=? WHERE id=? AND revision=? AND status='active' AND EXISTS(SELECT 1 FROM provider_connections WHERE id=? AND status='active')`, in.ConnectionID, in.Model, l.MaxMessages, l.MaxMessageBytes, l.MaxInputBytes, l.MaxOutputTokens, l.Timeout.Milliseconds(), l.ViewerRequests, l.AppRequests, l.RateWindow.Milliseconds(), in.ConcurrencyLimit, in.MonthlyTokenLimit, r.now().Format(time.RFC3339Nano), in.ID, expected, in.ConnectionID)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return controlapi.ErrLLMRevision
		}
		return r.operatorAudit(ctx, tx, in.ActorID, "llm.profile.updated", "llm_profile", in.ID)
	})
}

// UpdateGrant creates at revision one only when expected is zero; otherwise it
// changes the exact current revision. This prevents a stale operator page from
// silently reviving a revoked app grant.
func (r LLMRepository) UpdateGrant(ctx context.Context, in LLMGrantInput, expected uint64) error {
	if r.Store == nil || in.AppID == "" || in.ProfileID == "" || in.OperatorID == "" || (in.Status != "approved" && in.Status != "disabled" && in.Status != "revoked") {
		return llm.ErrInvalidRequest
	}
	now := r.now().Format(time.RFC3339Nano)
	return r.Store.Write(ctx, func(tx *sql.Tx) error {
		if expected == 0 {
			result, err := tx.ExecContext(ctx, `INSERT INTO app_capability_grants(app_id,capability,version,profile_id,status,revision,approved_by,created_at,updated_at) SELECT ?,'llm.chat',1,?,?,1,?,?,? WHERE EXISTS(SELECT 1 FROM applications WHERE id=?) AND EXISTS(SELECT 1 FROM llm_chat_profiles p JOIN provider_connections c ON c.id=p.connection_id WHERE p.id=? AND p.status='active' AND c.status='active')`, in.AppID, in.ProfileID, in.Status, in.OperatorID, now, now, in.AppID, in.ProfileID)
			if err != nil {
				return err
			}
			changed, err := result.RowsAffected()
			if err != nil || changed != 1 {
				return controlapi.ErrLLMRevision
			}
		} else {
			result, err := tx.ExecContext(ctx, `UPDATE app_capability_grants SET profile_id=?,status=?,revision=revision+1,approved_by=?,updated_at=? WHERE app_id=? AND capability='llm.chat' AND version=1 AND revision=? AND status != 'revoked' AND EXISTS(SELECT 1 FROM llm_chat_profiles p JOIN provider_connections c ON c.id=p.connection_id WHERE p.id=? AND p.status='active' AND c.status='active')`, in.ProfileID, in.Status, in.OperatorID, now, in.AppID, expected, in.ProfileID)
			if err != nil {
				return err
			}
			n, err := result.RowsAffected()
			if err != nil {
				return err
			}
			if n != 1 {
				return controlapi.ErrLLMRevision
			}
		}
		return r.operatorAudit(ctx, tx, in.OperatorID, "llm.grant."+in.Status, "app", in.AppID)
	})
}

func (r LLMRepository) operatorAudit(ctx context.Context, tx *sql.Tx, actor, action, targetKind, target string) error {
	id, err := newLLMID()
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO audit_events(id,occurred_at,actor_kind,actor_id,action,outcome,target_kind,target_id) VALUES(?,?, 'operator',? ,?,'success',?,?)`, id, r.now().Format(time.RFC3339Nano), actor, action, targetKind, target)
	return err
}
