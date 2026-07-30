package persistence

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"fmt"
	"github.com/tinyhost/tiny/internal/identity"
	"github.com/tinyhost/tiny/internal/otp"
	"math/big"
	"time"
)

// ControlLogin issues CLI bearer credentials only. Browser authentication is
// deliberately owned by the global identity flow, whose role lookup happens
// at dashboard authorization time rather than by minting a second credential.
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

// OTPOutbox is the narrow delivery boundary shared by the CLI and browser
// identity OTP flows. It carries no authorization result.
type OTPOutbox interface {
	EnqueueOTP(context.Context, otp.Message) error
}

func fingerprintHash(key []byte, fingerprint string) []byte {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write([]byte("otp-request-fingerprint:" + fingerprint))
	return h.Sum(nil)
}

func (c ControlLogin) now() time.Time {
	if c.Clock != nil {
		return c.Clock()
	}
	return time.Now()
}
func (c ControlLogin) RequestOTP(ctx context.Context, raw string, fingerprint string) (string, error) {
	email, e := identity.Normalize(raw)
	if e != nil {
		return "", nil
	}
	b := make([]byte, 16)
	if _, e = rand.Read(b); e != nil {
		return "", otp.Failure(otp.IssuanceEntropy)
	}
	id := fmt.Sprintf("otp_%x", b)
	n, e := rand.Int(rand.Reader, big.NewInt(1000000))
	if e != nil {
		return "", otp.Failure(otp.IssuanceEntropy)
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
	e = c.writeCLIChallenge(ctx, func(tx *sql.Tx) error {
		_, e := tx.ExecContext(ctx, "UPDATE otp_challenges SET invalidated_at=? WHERE purpose='control' AND normalized_email=? AND consumed_at IS NULL AND invalidated_at IS NULL", now.UTC().Format(time.RFC3339Nano), email)
		if e != nil {
			return otp.Failure(otp.IssuancePersistenceInvalidate)
		}
		if e = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE normalized_email=? AND status='active' AND role IN ('operator','deployer'))", email).Scan(&eligible); e != nil {
			return otp.Failure(otp.IssuancePersistenceEligibility)
		}
		_, e = tx.ExecContext(ctx, "INSERT INTO otp_challenges(id,app_id,purpose,control_channel,normalized_email,code_hash,expires_at,attempts,request_fingerprint_hash,created_at) VALUES(?,NULL,'control','cli',?,?,?,?,?,?)", id, email, h.Sum(nil), now.Add(ttl).UTC().Format(time.RFC3339Nano), 0, fingerprintHash(c.HMACKey, fingerprint), now.UTC().Format(time.RFC3339Nano))
		if e != nil {
			return otp.Failure(otp.IssuancePersistenceInsert)
		}
		return nil
	})
	if e != nil {
		// The HTTP boundary deliberately maps this to a generic availability
		// response. Returning the failure here is essential: issuing a fake
		// transaction makes a durable-control failure look like a usable OTP
		// login and cannot ever succeed at verification.
		return "", e
	}
	if eligible && c.Outbox != nil {
		_ = c.Outbox.EnqueueOTP(ctx, otp.Message{Email: email, Code: code, ChallengeID: id})
	}
	return id, nil
}

// writeCLIChallenge mirrors SQLiteStore.Write so that the narrow CLI issuer
// can classify its own fixed operational stages. It intentionally suppresses
// every underlying database value before it leaves persistence.
func (c ControlLogin) writeCLIChallenge(ctx context.Context, fn func(*sql.Tx) error) error {
	if c.Store == nil || c.Store.DB == nil || c.Store.write == nil {
		return otp.Failure(otp.IssuancePersistenceBeginWriteLock)
	}
	select {
	case <-ctx.Done():
		return otp.Failure(otp.IssuancePersistenceBeginWriteLock)
	case <-c.Store.write:
	}
	defer func() { c.Store.write <- struct{}{} }()
	tx, err := c.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return otp.Failure(otp.IssuancePersistenceBeginWriteLock)
	}
	defer tx.Rollback()
	if err = fn(tx); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return otp.Failure(otp.IssuancePersistenceCommit)
	}
	return nil
}
func (c ControlLogin) VerifyOTP(ctx context.Context, id, code string) (string, error) {
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
		e := tx.QueryRowContext(ctx, "SELECT normalized_email,code_hash,expires_at,attempts,consumed_at,invalidated_at FROM otp_challenges WHERE id=? AND purpose='control' AND app_id IS NULL AND control_channel='cli'", id).Scan(&email, &hash, &expires, &attempts, &consumed, &invalid)
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
		_, e = tx.ExecContext(ctx, "INSERT INTO api_tokens(id,user_id,secret_hash,scopes,expires_at) VALUES(?,?,?,?,?)", base64.RawURLEncoding.EncodeToString(th[:12]), user, th[:], "app:read,app:create,app:delete,deploy:create,deploy:activate,access:read,access:write,token:create,token:revoke,data:read,data:write", now.Add(30*24*time.Hour).UTC().Format(time.RFC3339Nano))
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
