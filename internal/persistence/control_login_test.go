package persistence

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ChrisMarxDev/tinkercloud/internal/otp"
)

type captureOutbox struct{ m otp.Message }

func (c *captureOutbox) EnqueueOTP(_ context.Context, m otp.Message) error { c.m = m; return nil }

func TestControlLoginIssuesOnlyCLIBearer(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	o := &captureOutbox{}
	l := ControlLogin{Store: s, HMACKey: []byte("key"), Outbox: o}

	tx, err := l.RequestOTP(context.Background(), "owner@example.com", "test")
	if err != nil || tx == "" || o.m.Code == "" {
		t.Fatal(err)
	}
	var fingerprint []byte
	if err = s.DB.QueryRow("SELECT request_fingerprint_hash FROM otp_challenges WHERE id=?", tx).Scan(&fingerprint); err != nil || string(fingerprint) == "test" || len(fingerprint) == 0 {
		t.Fatalf("fingerprint must be a non-raw keyed digest: %q %v", fingerprint, err)
	}
	bearer, err := l.VerifyOTP(context.Background(), tx, o.m.Code)
	if err != nil || bearer == "" {
		t.Fatal(err)
	}
	if _, err = s.AuthenticateToken(context.Background(), bearer, "app:read", "", time.Now()); err != nil {
		t.Fatalf("CLI bearer denied: %v", err)
	}
}

func TestControlLoginWrongAttemptCommitsConfiguredLimit(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	o := &captureOutbox{}
	l := ControlLogin{Store: s, HMACKey: []byte("key"), Outbox: o, MaxAttempts: 2}
	tx, err := l.RequestOTP(context.Background(), "owner@example.com", "test")
	if err != nil || tx == "" || o.m.Code == "" {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err = l.VerifyOTP(context.Background(), tx, "000000"); !errors.Is(err, ErrOTP) {
			t.Fatalf("wrong attempt %d = %v", i, err)
		}
	}
	var attempts int
	if err = s.DB.QueryRow("SELECT attempts FROM otp_challenges WHERE id=?", tx).Scan(&attempts); err != nil || attempts != 2 {
		t.Fatalf("attempts=%d err=%v", attempts, err)
	}
	if _, err = l.VerifyOTP(context.Background(), tx, o.m.Code); !errors.Is(err, ErrOTP) {
		t.Fatalf("configured limit was bypassed: %v", err)
	}
}
