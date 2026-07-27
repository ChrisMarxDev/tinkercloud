package persistence

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"github.com/tinyhost/tiny/internal/controlapi"
	"strings"
	"time"
)

var ErrToken = errors.New("token denied")

func (s *SQLiteStore) IssueToken(ctx context.Context, user, app string, scopes []string, expiry time.Time) (string, error) {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	raw := "tiny_" + base64.RawURLEncoding.EncodeToString(b)
	h := sha256.Sum256([]byte(raw))
	e := s.Write(ctx, func(tx *sql.Tx) error {
		var active string
		if e := tx.QueryRowContext(ctx, "SELECT status FROM users WHERE id=?", user).Scan(&active); e != nil || active != "active" {
			return ErrToken
		}
		id := base64.RawURLEncoding.EncodeToString(h[:12])
		var appValue any = app
		if app == "" {
			appValue = nil
		}
		_, e := tx.ExecContext(ctx, "INSERT INTO api_tokens(id,user_id,app_id,secret_hash,scopes,expires_at) VALUES(?,?,?,?,?,?)", id, user, appValue, h[:], strings.Join(scopes, ","), expiry.UTC().Format(time.RFC3339Nano))
		return e
	})
	if e != nil {
		return "", e
	}
	return raw, nil
}
func (s *SQLiteStore) AuthenticateToken(ctx context.Context, raw, scope, app string, now time.Time) (controlapi.Actor, error) {
	if s == nil || s.DB == nil || !validTokenScope(scope) {
		return controlapi.Actor{}, ErrToken
	}
	h := sha256.Sum256([]byte(raw))
	var actor controlapi.Actor
	usedAt := now.UTC().Format(time.RFC3339Nano)
	err := s.Write(ctx, func(tx *sql.Tx) error {
		var expiry string
		if err := tx.QueryRowContext(ctx, "SELECT expires_at FROM api_tokens WHERE secret_hash=?", h[:]).Scan(&expiry); err != nil {
			return ErrToken
		}
		expiresAt, err := time.Parse(time.RFC3339Nano, expiry)
		if err != nil || !now.Before(expiresAt) {
			return ErrToken
		}
		// All conditions are repeated by the conditional update. This keeps a
		// successful authorization and its audit-safe metadata inseparable from
		// a concurrent revocation or suspension.
		result, err := tx.ExecContext(ctx, `UPDATE api_tokens
			SET last_used_at=?
			WHERE secret_hash=?
			  AND revoked_at IS NULL
			  AND expires_at=?
			  AND (app_id IS NULL OR app_id=?)
			  AND instr(',' || scopes || ',', ',' || ? || ',')>0
			  AND EXISTS (SELECT 1 FROM users u WHERE u.id=api_tokens.user_id AND u.status='active')`,
			usedAt, h[:], expiry, app, scope)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return ErrToken
		}
		if err := tx.QueryRowContext(ctx, `SELECT u.id,u.normalized_email,u.role
			FROM api_tokens t JOIN users u ON u.id=t.user_id WHERE t.secret_hash=?`, h[:]).Scan(&actor.ID, &actor.Email, &actor.Role); err != nil {
			return err
		}
		actor.Active = true
		return nil
	})
	if err != nil {
		// An unavailable or failed metadata write must not turn into a usable
		// credential. Deliberately return the generic token denial.
		return controlapi.Actor{}, ErrToken
	}
	return actor, nil
}

// validTokenScope prevents a caller-controlled delimiter from changing the
// exact-scope predicate used by AuthenticateToken.
func validTokenScope(scope string) bool {
	return scope != "" && !strings.ContainsAny(scope, ",\r\n")
}
