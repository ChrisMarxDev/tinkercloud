package persistence

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"time"

	"github.com/tinyhost/tiny/internal/controlapi"
)

// AuthenticateControlSession accepts only a persisted browser control session.
// It deliberately does not inspect api_tokens: bearer authority is never an
// ambient browser credential.
func (s *SQLiteStore) AuthenticateControlSession(ctx context.Context, raw string, now time.Time) (controlapi.Actor, error) {
	if s == nil || s.DB == nil || raw == "" {
		return controlapi.Actor{}, ErrToken
	}
	hash := sha256.Sum256([]byte(raw))
	usedAt := now.UTC().Format(time.RFC3339Nano)
	var actor controlapi.Actor
	err := s.Write(ctx, func(tx *sql.Tx) error {
		var expiry string
		if err := tx.QueryRowContext(ctx, "SELECT expires_at FROM sessions WHERE scope='control' AND secret_hash=?", hash[:]).Scan(&expiry); err != nil {
			return ErrToken
		}
		expiresAt, err := time.Parse(time.RFC3339Nano, expiry)
		if err != nil || !now.Before(expiresAt) {
			return ErrToken
		}
		result, err := tx.ExecContext(ctx, `UPDATE sessions
			SET last_seen_at=?
			WHERE scope='control'
			  AND secret_hash=?
			  AND revoked_at IS NULL
			  AND expires_at=?
			  AND app_id IS NULL
			  AND identity_id IS NULL
			  AND EXISTS (SELECT 1 FROM users u WHERE u.id=sessions.user_id AND u.status='active')`,
			usedAt, hash[:], expiry)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil || count != 1 {
			return ErrToken
		}
		if err := tx.QueryRowContext(ctx, `SELECT u.id,u.normalized_email,u.role
			FROM sessions s JOIN users u ON u.id=s.user_id
			WHERE s.scope='control' AND s.secret_hash=?`, hash[:]).Scan(&actor.ID, &actor.Email, &actor.Role); err != nil {
			return err
		}
		actor.Active = true
		return nil
	})
	if err != nil {
		return controlapi.Actor{}, ErrToken
	}
	return actor, nil
}
