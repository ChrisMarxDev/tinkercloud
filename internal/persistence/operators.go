package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/ChrisMarxDev/tinkercloud/internal/identity"
	"time"
)

var ErrOperatorExists = errors.New("operator exists")

func (s *SQLiteStore) EnsureInitialOperator(ctx context.Context, email, request string) error {
	e, err := identity.Normalize(email)
	if err != nil {
		return err
	}
	return s.Write(ctx, func(tx *sql.Tx) error {
		var existing string
		err := tx.QueryRowContext(ctx, "SELECT normalized_email FROM users WHERE role='operator' AND status='active' LIMIT 1").Scan(&existing)
		if err == nil {
			if existing == e {
				return nil
			}
			return ErrOperatorExists
		}
		if err != sql.ErrNoRows {
			return err
		}
		id := "opr_" + fmt.Sprintf("%x", time.Now().UnixNano())
		if _, err = tx.ExecContext(ctx, "INSERT INTO users(id,normalized_email,role,status,created_at) VALUES(?,?, 'operator','active',?)", id, e, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO audit_events(id,occurred_at,actor_kind,action,outcome,target_kind,target_id,request_id) VALUES(?,?,'root','operator.initialized','success','user',?,?)", request+"_audit", time.Now().UTC().Format(time.RFC3339Nano), id, request)
		return err
	})
}
func (s *SQLiteStore) RecoverOperator(ctx context.Context, email, request string) error {
	e, err := identity.Normalize(email)
	if err != nil {
		return err
	}
	return s.Write(ctx, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		// Recovery replaces the control authority. Revoke every current and
		// historical operator credential before an operator is reactivated; this
		// includes same-email recovery, where an UPSERT preserves the user row.
		if _, err = tx.ExecContext(ctx, "UPDATE api_tokens SET revoked_at=? WHERE user_id IN (SELECT id FROM users WHERE role='operator') AND revoked_at IS NULL", now); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, "UPDATE users SET status='revoked' WHERE role='operator' AND status='active'")
		if err != nil {
			return err
		}
		id := "opr_" + fmt.Sprintf("%x", time.Now().UnixNano())
		_, err = tx.ExecContext(ctx, "INSERT INTO users(id,normalized_email,role,status,created_at) VALUES(?,?, 'operator','active',?) ON CONFLICT(normalized_email) DO UPDATE SET role='operator',status='active'", id, e, now)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO audit_events(id,occurred_at,actor_kind,action,outcome,request_id) VALUES(?,?,'root','operator.recovered','success',?)", request+"_audit", now, request)
		return err
	})
}
