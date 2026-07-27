package persistence

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"fmt"
	"github.com/tinyhost/tiny/internal/controlapi"
	"github.com/tinyhost/tiny/internal/identity"
	"github.com/tinyhost/tiny/internal/otp"
	"math/big"
	"time"
)

// ControlLogin implements controlapi.Login without ever using app viewer state.
type ControlLogin struct {
	Store   *SQLiteStore
	HMACKey []byte
	Outbox  OTPOutbox
	Clock   func() time.Time
	TTL     time.Duration
	// MaxAttempts is the configured fail-closed OTP limit. Zero preserves the
	// V1 default of five for direct callers that have not supplied config.
	MaxAttempts int
}

func (c ControlLogin) now() time.Time {
	if c.Clock != nil {
		return c.Clock()
	}
	return time.Now()
}
func validControlLoginChannel(channel controlapi.LoginChannel) bool {
	return channel == controlapi.BrowserLoginChannel || channel == controlapi.CLILoginChannel
}

func (c ControlLogin) RequestOTP(ctx context.Context, raw string, channel controlapi.LoginChannel) (string, error) {
	if !validControlLoginChannel(channel) {
		return "", nil
	}
	email, e := identity.Normalize(raw)
	if e != nil {
		return "", nil
	}
	b := make([]byte, 16)
	if _, e = rand.Read(b); e != nil {
		return "", nil
	}
	id := fmt.Sprintf("otp_%x", b)
	n, e := rand.Int(rand.Reader, big.NewInt(1000000))
	if e != nil {
		return "", nil
	}
	code := fmt.Sprintf("%06d", n.Int64())
	h := hmac.New(sha256.New, c.HMACKey)
	h.Write([]byte(id + ":" + code))
	now := c.now()
	ttl := c.TTL
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	eligible := false
	e = c.Store.Write(ctx, func(tx *sql.Tx) error {
		_, e := tx.ExecContext(ctx, "UPDATE otp_challenges SET invalidated_at=? WHERE purpose='control' AND normalized_email=? AND consumed_at IS NULL AND invalidated_at IS NULL", now.UTC().Format(time.RFC3339Nano), email)
		if e != nil {
			return e
		}
		if e = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE normalized_email=? AND status='active' AND role IN ('operator','deployer'))", email).Scan(&eligible); e != nil {
			return e
		}
		_, e = tx.ExecContext(ctx, "INSERT INTO otp_challenges(id,app_id,purpose,control_channel,normalized_email,code_hash,expires_at,attempts,created_at) VALUES(?,NULL,'control',?,?,?,?,?,?)", id, string(channel), email, h.Sum(nil), now.Add(ttl).UTC().Format(time.RFC3339Nano), 0, now.UTC().Format(time.RFC3339Nano))
		return e
	})
	if e != nil {
		return "", nil
	}
	if eligible && c.Outbox != nil {
		_ = c.Outbox.EnqueueOTP(ctx, otp.Message{Email: email, Code: code, ChallengeID: id})
	}
	return id, nil
}
func (c ControlLogin) VerifyOTP(ctx context.Context, id, code string, channel controlapi.LoginChannel) (string, error) {
	if !validControlLoginChannel(channel) {
		return "", ErrOTP
	}
	var token string
	now := c.now()
	maxAttempts := c.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	denied := false
	e := c.Store.Write(ctx, func(tx *sql.Tx) error {
		var email, expires string
		var hash []byte
		var attempts int
		var consumed, invalid sql.NullString
		e := tx.QueryRowContext(ctx, "SELECT normalized_email,code_hash,expires_at,attempts,consumed_at,invalidated_at FROM otp_challenges WHERE id=? AND purpose='control' AND app_id IS NULL AND control_channel=?", id, string(channel)).Scan(&email, &hash, &expires, &attempts, &consumed, &invalid)
		if e != nil || consumed.Valid || invalid.Valid {
			denied = true
			return nil
		}
		at, e := time.Parse(time.RFC3339Nano, expires)
		if e != nil || !now.Before(at) || attempts >= maxAttempts {
			denied = true
			return nil
		}
		h := hmac.New(sha256.New, c.HMACKey)
		h.Write([]byte(id + ":" + code))
		if !hmac.Equal(hash, h.Sum(nil)) {
			if _, e = tx.ExecContext(ctx, "UPDATE otp_challenges SET attempts=attempts+1 WHERE id=? AND attempts<?", id, maxAttempts); e != nil {
				return e
			}
			denied = true
			return nil
		}
		var user string
		if e = tx.QueryRowContext(ctx, "SELECT id FROM users WHERE normalized_email=? AND status='active' AND role IN ('operator','deployer')", email).Scan(&user); e != nil {
			denied = true
			return nil
		}
		b := make([]byte, 32)
		if _, e = rand.Read(b); e != nil {
			return e
		}
		token = "tiny_" + base64.RawURLEncoding.EncodeToString(b)
		th := sha256.Sum256([]byte(token))
		if channel == controlapi.BrowserLoginChannel {
			_, e = tx.ExecContext(ctx, "INSERT INTO sessions(id,scope,user_id,secret_hash,expires_at,created_at) VALUES(?, 'control', ?, ?, ?, ?)", "ctl_"+base64.RawURLEncoding.EncodeToString(th[:12]), user, th[:], now.Add(30*24*time.Hour).UTC().Format(time.RFC3339Nano), now.UTC().Format(time.RFC3339Nano))
		} else {
			_, e = tx.ExecContext(ctx, "INSERT INTO api_tokens(id,user_id,secret_hash,scopes,expires_at) VALUES(?,?,?,?,?)", base64.RawURLEncoding.EncodeToString(th[:12]), user, th[:], "app:read,app:create,app:delete,deploy:create,deploy:activate,access:read,access:write,token:create,token:revoke", now.Add(30*24*time.Hour).UTC().Format(time.RFC3339Nano))
		}
		if e != nil {
			return e
		}
		_, e = tx.ExecContext(ctx, "UPDATE otp_challenges SET consumed_at=? WHERE id=? AND consumed_at IS NULL", now.UTC().Format(time.RFC3339Nano), id)
		return e
	})
	if e != nil {
		return "", e
	}
	if denied {
		return "", ErrOTP
	}
	return token, nil
}

// RevokeControlCredential makes browser logout effective on the next request.
// The raw value is accepted only from the host-only cookie in the platform UI;
// persistence retains and compares only its hash in a control session row.
func (c ControlLogin) RevokeControlCredential(ctx context.Context, raw string) error {
	if c.Store == nil || raw == "" {
		return ErrOTP
	}
	h := sha256.Sum256([]byte(raw))
	return c.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE sessions SET revoked_at=? WHERE scope='control' AND secret_hash=? AND revoked_at IS NULL", c.now().UTC().Format(time.RFC3339Nano), h[:])
		return err
	})
}
