package persistence

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"github.com/ChrisMarxDev/tinkercloud/internal/identity"
	"math/big"
	"time"
)

var ErrOTP = errors.New("otp denied")

type ChallengeMessage struct{ ID, Code, Email, AppID, Purpose string }

func (s *SQLiteStore) CreateChallenge(ctx context.Context, appID, purpose, rawEmail string, fingerprintHash, key []byte, eligible bool, now time.Time, ttl time.Duration) (*ChallengeMessage, error) {
	email, e := identity.Normalize(rawEmail)
	if e != nil {
		return nil, nil
	}
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	b := make([]byte, 16)
	if _, e = rand.Read(b); e != nil {
		return nil, e
	}
	id := fmt.Sprintf("otp_%x", b)
	n, e := rand.Int(rand.Reader, big.NewInt(1000000))
	if e != nil {
		return nil, e
	}
	code := fmt.Sprintf("%06d", n.Int64())
	h := hmac.New(sha256.New, key)
	h.Write([]byte(id + ":" + code))
	e = s.Write(ctx, func(tx *sql.Tx) error {
		if _, e := tx.ExecContext(ctx, "UPDATE otp_challenges SET invalidated_at=? WHERE app_id=? AND purpose=? AND normalized_email=? AND consumed_at IS NULL AND invalidated_at IS NULL", now.UTC().Format(time.RFC3339Nano), appID, purpose, email); e != nil {
			return e
		}
		_, e := tx.ExecContext(ctx, "INSERT INTO otp_challenges(id,app_id,purpose,normalized_email,code_hash,expires_at,attempts,request_fingerprint_hash,created_at) VALUES(?,?,?,?,?,?,?,?,?)", id, appID, purpose, email, h.Sum(nil), now.Add(ttl).UTC().Format(time.RFC3339Nano), 0, fingerprintHash, now.UTC().Format(time.RFC3339Nano))
		return e
	})
	if e != nil {
		return nil, e
	}
	if !eligible {
		return nil, nil
	}
	return &ChallengeMessage{ID: id, Code: code, Email: email, AppID: appID, Purpose: purpose}, nil
}

// VerifyAndIssue atomically consumes a current challenge and runs credential
// issuance in the same SQLite transaction. Callers must recheck policy inside
// issue before inserting a session or token.
func (s *SQLiteStore) VerifyAndIssue(ctx context.Context, appID, purpose, email, id, code string, key []byte, now time.Time, maxAttempts int, issue func(*sql.Tx) error) error {
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	denied := false
	err := s.Write(ctx, func(tx *sql.Tx) error {
		var hash []byte
		var expires string
		var attempts int
		var consumed, invalid sql.NullString
		e := tx.QueryRowContext(ctx, "SELECT code_hash,expires_at,attempts,consumed_at,invalidated_at FROM otp_challenges WHERE id=? AND app_id=? AND purpose=? AND normalized_email=?", id, appID, purpose, email).Scan(&hash, &expires, &attempts, &consumed, &invalid)
		if e != nil || consumed.Valid || invalid.Valid {
			denied = true
			return nil
		}
		at, e := time.Parse(time.RFC3339Nano, expires)
		if e != nil || !now.Before(at) || attempts >= maxAttempts {
			denied = true
			return nil
		}
		h := hmac.New(sha256.New, key)
		h.Write([]byte(id + ":" + code))
		if !hmac.Equal(hash, h.Sum(nil)) {
			if _, e = tx.ExecContext(ctx, "UPDATE otp_challenges SET attempts=attempts+1 WHERE id=? AND attempts<?", id, maxAttempts); e != nil {
				return e
			}
			denied = true
			return nil
		}
		if e = issue(tx); e != nil {
			return e
		}
		_, e = tx.ExecContext(ctx, "UPDATE otp_challenges SET consumed_at=? WHERE id=? AND consumed_at IS NULL", now.UTC().Format(time.RFC3339Nano), id)
		return e
	})
	if err != nil {
		return err
	}
	if denied {
		return ErrOTP
	}
	return nil
}
