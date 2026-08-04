package persistence

import (
	"context"
	"database/sql"
	"sort"
	"sync"
	"time"

	"github.com/ChrisMarxDev/tinkercloud/internal/controlapi"
	"github.com/ChrisMarxDev/tinkercloud/internal/llm"
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
	ConcurrencyLimit                                      int
	IsDefault                                             bool
}
type LLMHostPolicyView struct {
	Status            string
	MonthlyTokenLimit int
	Revision          uint64
}
type LLMAppPolicyView struct {
	AppID, AppSlug, Status, QuotaMode, UpdatedAt string
	MonthlyTokenLimit                            int
	Revision                                     uint64
	UsedTokens, ReservedTokens, InFlight         int
}
type LLMModelCatalogView struct {
	ConnectionID, ConnectionName, Provider, Model string
}

type llmModelCataloger interface {
	ListModels(context.Context, llm.Provider, []byte) ([]string, error)
}

func (r LLMRepository) OperatorViews(ctx context.Context) ([]LLMConnectionView, []LLMProfileView, LLMHostPolicyView, []LLMAppPolicyView, error) {
	if r.Store == nil {
		return nil, nil, LLMHostPolicyView{}, nil, llm.ErrCapabilityUnavailable
	}
	connections := []LLMConnectionView{}
	rows, err := r.Store.DB.QueryContext(ctx, `SELECT id,display_name,provider_kind,status,created_at,updated_at FROM provider_connections ORDER BY display_name,id LIMIT 100`)
	if err != nil {
		return nil, nil, LLMHostPolicyView{}, nil, err
	}
	for rows.Next() {
		var v LLMConnectionView
		if err := rows.Scan(&v.ID, &v.DisplayName, &v.Provider, &v.Status, &v.CreatedAt, &v.UpdatedAt); err != nil {
			rows.Close()
			return nil, nil, LLMHostPolicyView{}, nil, err
		}
		connections = append(connections, v)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, nil, LLMHostPolicyView{}, nil, err
	}
	rows.Close()
	profiles := []LLMProfileView{}
	rows, err = r.Store.DB.QueryContext(ctx, `SELECT id,connection_id,model,max_messages,max_message_bytes,max_input_bytes,max_output_tokens,timeout_ms,viewer_requests,app_requests,rate_window_ms,concurrency_limit,is_default,status,revision,created_at,updated_at FROM llm_chat_profiles ORDER BY is_default DESC,created_at DESC,id LIMIT 100`)
	if err != nil {
		return nil, nil, LLMHostPolicyView{}, nil, err
	}
	for rows.Next() {
		var v LLMProfileView
		var timeout, window int64
		if err := rows.Scan(&v.ID, &v.ConnectionID, &v.Model, &v.Limits.MaxMessages, &v.Limits.MaxMessageBytes, &v.Limits.MaxInputBytes, &v.Limits.MaxOutputTokens, &timeout, &v.Limits.ViewerRequests, &v.Limits.AppRequests, &window, &v.ConcurrencyLimit, &v.IsDefault, &v.Status, &v.Revision, &v.CreatedAt, &v.UpdatedAt); err != nil {
			rows.Close()
			return nil, nil, LLMHostPolicyView{}, nil, err
		}
		v.Limits.Timeout = time.Duration(timeout) * time.Millisecond
		v.Limits.RateWindow = time.Duration(window) * time.Millisecond
		profiles = append(profiles, v)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, nil, LLMHostPolicyView{}, nil, err
	}
	rows.Close()
	var host LLMHostPolicyView
	var hostLimit sql.NullInt64
	if err := r.Store.DB.QueryRowContext(ctx, `SELECT status,monthly_token_limit,revision FROM llm_host_policy WHERE id=1`).Scan(&host.Status, &hostLimit, &host.Revision); err != nil {
		return nil, nil, LLMHostPolicyView{}, nil, err
	}
	if hostLimit.Valid {
		host.MonthlyTokenLimit = int(hostLimit.Int64)
	}
	policies := []LLMAppPolicyView{}
	rows, err = r.Store.DB.QueryContext(ctx, `SELECT a.id,a.slug,COALESCE(p.status,'enabled'),COALESCE(p.quota_mode,'inherit'),p.monthly_token_limit,COALESCE(p.revision,0),COALESCE(p.updated_at,''),COALESCE(u.used_tokens,0),COALESCE(u.reserved_tokens,0),COALESCE(u.in_flight,0) FROM applications a LEFT JOIN app_llm_policies p ON p.app_id=a.id LEFT JOIN llm_usage u ON u.app_id=a.id AND u.period_start=? ORDER BY a.slug LIMIT 100`, monthStart(r.now()))
	if err != nil {
		return nil, nil, LLMHostPolicyView{}, nil, err
	}
	for rows.Next() {
		var v LLMAppPolicyView
		var limit sql.NullInt64
		if err := rows.Scan(&v.AppID, &v.AppSlug, &v.Status, &v.QuotaMode, &limit, &v.Revision, &v.UpdatedAt, &v.UsedTokens, &v.ReservedTokens, &v.InFlight); err != nil {
			rows.Close()
			return nil, nil, LLMHostPolicyView{}, nil, err
		}
		if limit.Valid {
			v.MonthlyTokenLimit = int(limit.Int64)
		}
		policies = append(policies, v)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, LLMHostPolicyView{}, nil, err
	}
	return connections, profiles, host, policies, nil
}

// OperatorModelCatalog decrypts only active connection credentials and clears
// each plaintext immediately after its fixed provider lookup. Provider,
// decrypt, timeout, and response failures omit that connection; only database
// failures prevent the operator dashboard itself from rendering.
func (r LLMRepository) OperatorModelCatalog(ctx context.Context, cataloger llmModelCataloger) ([]LLMModelCatalogView, error) {
	if r.Store == nil || r.Envelope == nil || cataloger == nil {
		return nil, nil
	}
	rows, err := r.Store.DB.QueryContext(ctx, `SELECT id,display_name,provider_kind,credential_envelope FROM provider_connections WHERE status='active' ORDER BY display_name,id LIMIT 100`)
	if err != nil {
		return nil, err
	}
	type connection struct {
		id, name, provider string
		box                []byte
	}
	connections := make([]connection, 0)
	for rows.Next() {
		var c connection
		if err := rows.Scan(&c.id, &c.name, &c.provider, &c.box); err != nil {
			rows.Close()
			return nil, err
		}
		connections = append(connections, c)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	type catalogResult struct {
		connection int
		models     []LLMModelCatalogView
	}
	results := make(chan catalogResult, len(connections))
	var lookups sync.WaitGroup
	for index, c := range connections {
		lookups.Add(1)
		go func() {
			defer lookups.Done()
			provider := llm.Provider(c.provider)
			if provider != llm.ProviderAnthropic && provider != llm.ProviderGemini {
				return
			}
			secret, err := r.Envelope.Open(lookupCtx, c.box)
			if err != nil {
				return
			}
			models, listErr := cataloger.ListModels(lookupCtx, provider, secret)
			clear(secret)
			if listErr != nil || len(models) > 1000 {
				return
			}
			listed := make([]LLMModelCatalogView, 0, len(models))
			for _, model := range models {
				if !llm.ValidModelIdentifier(model) {
					return
				}
				listed = append(listed, LLMModelCatalogView{ConnectionID: c.id, ConnectionName: c.name, Provider: c.provider, Model: model})
			}
			results <- catalogResult{connection: index, models: listed}
		}()
	}
	lookups.Wait()
	close(results)
	listed := make([]catalogResult, 0, len(connections))
	for item := range results {
		listed = append(listed, item)
	}
	sort.Slice(listed, func(i, j int) bool { return listed[i].connection < listed[j].connection })
	result := make([]LLMModelCatalogView, 0)
	for _, item := range listed {
		result = append(result, item.models...)
	}
	return result, nil
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
	if r.Store == nil || in.ID == "" || in.ActorID == "" || expected < 1 || !validModel(in.Model) || in.ConcurrencyLimit < 1 || !validLLMLimits(in.Limits) {
		return llm.ErrInvalidRequest
	}
	l := in.Limits
	return r.Store.Write(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `UPDATE llm_chat_profiles SET connection_id=?,model=?,max_messages=?,max_message_bytes=?,max_input_bytes=?,max_output_tokens=?,timeout_ms=?,viewer_requests=?,app_requests=?,rate_window_ms=?,concurrency_limit=?,revision=revision+1,updated_at=? WHERE id=? AND revision=? AND status='active' AND EXISTS(SELECT 1 FROM provider_connections WHERE id=? AND status='active')`, in.ConnectionID, in.Model, l.MaxMessages, l.MaxMessageBytes, l.MaxInputBytes, l.MaxOutputTokens, l.Timeout.Milliseconds(), l.ViewerRequests, l.AppRequests, l.RateWindow.Milliseconds(), in.ConcurrencyLimit, r.now().Format(time.RFC3339Nano), in.ID, expected, in.ConnectionID)
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

func (r LLMRepository) SetDefaultProfile(ctx context.Context, id, actor string, expected uint64) error {
	if r.Store == nil || id == "" || actor == "" || expected < 1 {
		return llm.ErrInvalidRequest
	}
	now := r.now().Format(time.RFC3339Nano)
	return r.Store.Write(ctx, func(tx *sql.Tx) error {
		var active, current bool
		if err := tx.QueryRowContext(ctx, `SELECT status='active' AND EXISTS(SELECT 1 FROM provider_connections c WHERE c.id=p.connection_id AND c.status='active'),is_default=1 FROM llm_chat_profiles p WHERE id=? AND revision=?`, id, expected).Scan(&active, &current); err != nil || !active {
			return controlapi.ErrLLMRevision
		}
		if current {
			return nil
		}
		if _, err := tx.ExecContext(ctx, `UPDATE llm_chat_profiles SET is_default=0,revision=revision+1,updated_at=? WHERE is_default=1`, now); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE llm_chat_profiles SET is_default=1,revision=revision+1,updated_at=? WHERE id=? AND revision=? AND status='active'`, now, id, expected)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil || n != 1 {
			return controlapi.ErrLLMRevision
		}
		return r.operatorAudit(ctx, tx, actor, "llm.profile.defaulted", "llm_profile", id)
	})
}

func (r LLMRepository) UpdateHostPolicy(ctx context.Context, status string, monthlyTokenLimit *int, actor string, expected uint64) error {
	if r.Store == nil || actor == "" || expected < 1 || status != "enabled" && status != "disabled" || monthlyTokenLimit != nil && *monthlyTokenLimit < 1 {
		return llm.ErrInvalidRequest
	}
	return r.Store.Write(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `UPDATE llm_host_policy SET status=?,monthly_token_limit=?,revision=revision+1,updated_at=? WHERE id=1 AND revision=?`, status, monthlyTokenLimit, r.now().Format(time.RFC3339Nano), expected)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil || n != 1 {
			return controlapi.ErrLLMRevision
		}
		return r.operatorAudit(ctx, tx, actor, "llm.host_policy.updated", "llm_host_policy", "1")
	})
}

func (r LLMRepository) UpdateAppPolicy(ctx context.Context, appID, status, quotaMode string, monthlyTokenLimit *int, actor string, expected uint64) error {
	validLimit := quotaMode == "specific" && monthlyTokenLimit != nil && *monthlyTokenLimit >= 1 || quotaMode != "specific" && monthlyTokenLimit == nil
	if r.Store == nil || appID == "" || actor == "" || (status != "enabled" && status != "disabled") || (quotaMode != "inherit" && quotaMode != "unlimited" && quotaMode != "specific") || !validLimit {
		return llm.ErrInvalidRequest
	}
	now := r.now().Format(time.RFC3339Nano)
	return r.Store.Write(ctx, func(tx *sql.Tx) error {
		if expected == 0 {
			result, err := tx.ExecContext(ctx, `INSERT INTO app_llm_policies(app_id,status,quota_mode,monthly_token_limit,revision,updated_by,created_at,updated_at) SELECT ?,?,?,?,1,?,?,? WHERE EXISTS(SELECT 1 FROM applications WHERE id=?) ON CONFLICT(app_id) DO NOTHING`, appID, status, quotaMode, monthlyTokenLimit, actor, now, now, appID)
			if err != nil {
				return err
			}
			n, err := result.RowsAffected()
			if err != nil || n != 1 {
				return controlapi.ErrLLMRevision
			}
		} else {
			result, err := tx.ExecContext(ctx, `UPDATE app_llm_policies SET status=?,quota_mode=?,monthly_token_limit=?,revision=revision+1,updated_by=?,updated_at=? WHERE app_id=? AND revision=?`, status, quotaMode, monthlyTokenLimit, actor, now, appID, expected)
			if err != nil {
				return err
			}
			n, err := result.RowsAffected()
			if err != nil || n != 1 {
				return controlapi.ErrLLMRevision
			}
		}
		return r.operatorAudit(ctx, tx, actor, "llm.app_policy.updated", "app", appID)
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
