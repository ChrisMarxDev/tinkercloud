package persistence

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"github.com/tinyhost/tiny/internal/identity"
	"github.com/tinyhost/tiny/internal/otp"
	"github.com/tinyhost/tiny/internal/policies"
	"time"
)

type OTPOutbox interface {
	EnqueueOTP(context.Context, otp.Message) error
}
type AppLogin struct {
	Store   *SQLiteStore
	HMACKey []byte
	Outbox  OTPOutbox
	Clock   func() time.Time
	TTL     time.Duration
	// MaxAttempts is the configured fail-closed OTP limit. Zero preserves the
	// V1 default of five for direct callers that have not supplied config.
	MaxAttempts int
}

func (a AppLogin) now() time.Time {
	if a.Clock != nil {
		return a.Clock()
	}
	return time.Now()
}
func (a AppLogin) Request(ctx context.Context, app, email string, eligible bool, fingerprint string) (string, error) {
	m, e := a.Store.CreateChallenge(ctx, app, "viewer", email, fingerprintHash(a.HMACKey, fingerprint), a.HMACKey, eligible, a.now(), a.TTL)
	if e != nil {
		return opaqueTransaction(), nil
	}
	if m != nil && a.Outbox != nil {
		_ = a.Outbox.EnqueueOTP(ctx, otp.Message{AppID: app, Email: m.Email, Code: m.Code, ChallengeID: m.ID})
		return m.ID, nil
	}
	return opaqueTransaction(), nil
}

func fingerprintHash(key []byte, fingerprint string) []byte {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write([]byte("otp-request-fingerprint:" + fingerprint))
	return h.Sum(nil)
}
func opaqueTransaction() string {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		return "otp_unavailable000000000000000000000000"
	}
	return fmt.Sprintf("otp_%x", b)
}
func (a AppLogin) VerifyAndCreateSession(ctx context.Context, app, email, id, code string, expiry time.Time) (string, error) {
	email, e := identity.Normalize(email)
	if e != nil {
		return "", ErrOTP
	}
	var raw string
	e = a.Store.VerifyAndIssue(ctx, app, "viewer", email, id, code, a.HMACKey, a.now(), a.MaxAttempts, func(tx *sql.Tx) error {
		p, e := a.policyTx(ctx, tx, app)
		if e != nil || policies.Evaluate(p, identity.Identity{ID: email, Email: email}) != policies.Allow {
			return ErrOTP
		}
		if _, e = tx.ExecContext(ctx, "INSERT INTO identities(id,normalized_email,created_at) VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET normalized_email=excluded.normalized_email", email, email, a.now().UTC().Format(time.RFC3339Nano)); e != nil {
			return e
		}
		var v identity.Identity
		v.ID = email
		v.Email = email
		raw, _, e = issueAppSessionTx(ctx, tx, app, v, expiry)
		return e
	})
	return raw, e
}
func (a AppLogin) policyTx(ctx context.Context, tx *sql.Tx, app string) (policies.Policy, error) {
	var p policies.Policy
	var mode string
	e := tx.QueryRowContext(ctx, "SELECT a.policy_revision,p.mode,u.normalized_email FROM applications a JOIN access_policies p ON p.app_id=a.id AND p.revision=a.policy_revision JOIN users u ON u.id=a.owner_user_id WHERE a.id=? AND a.status='active'", app).Scan(&p.Revision, &mode, &p.OwnerIdentityID)
	if e != nil || mode != "private" {
		return p, ErrOTP
	}
	p.AppID = app
	p.Valid = true
	p.Emails = map[string]struct{}{}
	p.Domains = map[string]struct{}{}
	rows, e := tx.QueryContext(ctx, "SELECT kind,normalized_value FROM access_rules WHERE app_id=? AND policy_revision=?", app, p.Revision)
	if e != nil {
		return p, e
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		rows.Scan(&k, &v)
		if k == "email" {
			p.Emails[v] = struct{}{}
		}
		if k == "domain" {
			p.Domains[v] = struct{}{}
		}
	}
	return p, rows.Err()
}
