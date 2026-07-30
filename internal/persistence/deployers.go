package persistence

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"github.com/ChrisMarxDev/tinkercloud/internal/identity"
	"time"
)

var ErrDeployerStatus = errors.New("invalid deployer status")

func (s *SQLiteStore) SetDeployerStatus(ctx context.Context, email, status, request string) error {
	if status != "active" && status != "suspended" && status != "revoked" {
		return ErrDeployerStatus
	}
	e, err := identity.Normalize(email)
	if err != nil {
		return err
	}
	return s.Write(ctx, func(tx *sql.Tx) error {
		var id, role, old string
		err := tx.QueryRowContext(ctx, "SELECT id,role,status FROM users WHERE normalized_email=?", e).Scan(&id, &role, &old)
		if err == sql.ErrNoRows {
			id = "usr_" + fmt.Sprintf("%x", sha256.Sum256([]byte(e)))[:16]
			_, err = tx.ExecContext(ctx, "INSERT INTO users(id,normalized_email,role,status,created_at) VALUES(?,?, 'deployer',?,?)", id, e, status, time.Now().UTC().Format(time.RFC3339Nano))
		} else if err == nil && role != "operator" {
			_, err = tx.ExecContext(ctx, "UPDATE users SET status=? WHERE id=?", status, id)
		}
		if err != nil {
			return err
		}
		if status != "active" {
			now := time.Now().UTC().Format(time.RFC3339Nano)
			if _, err = tx.ExecContext(ctx, "UPDATE api_tokens SET revoked_at=? WHERE user_id=? AND revoked_at IS NULL", now, id); err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO audit_events(id,occurred_at,actor_kind,actor_id,action,outcome,target_kind,target_id,request_id) VALUES(?,?,'root',NULL,?,'success','user',?,?)", request+"_audit", time.Now().UTC().Format(time.RFC3339Nano), "deployer."+status, id, request)
		return err
	})
}
