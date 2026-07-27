package persistence

import (
	"context"
	"database/sql"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestPersistentOTPDoesNotStoreRawAndConsumesOnce(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	now := time.Now()
	m, e := s.CreateChallenge(context.Background(), "a", "viewer", "a@example.com", "fp", []byte("key"), true, now, time.Minute)
	if e != nil || m == nil {
		t.Fatal(e)
	}
	var stored []byte
	if e = s.DB.QueryRow("SELECT code_hash FROM otp_challenges WHERE id=?", m.ID).Scan(&stored); e != nil || string(stored) == m.Code {
		t.Fatal("raw OTP persisted")
	}
	issued := 0
	issue := func(*sql.Tx) error { issued++; return nil }
	if e = s.VerifyAndIssue(context.Background(), "a", "viewer", m.Email, m.ID, m.Code, []byte("key"), now, 5, issue); e != nil || issued != 1 {
		t.Fatal(e, issued)
	}
	if e = s.VerifyAndIssue(context.Background(), "a", "viewer", m.Email, m.ID, m.Code, []byte("key"), now, 5, issue); e == nil || issued != 1 {
		t.Fatal("OTP replay issued")
	}
}

func TestPersistentOTPWrongAttemptCommitsConfiguredLimit(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	now := time.Now()
	m, err := s.CreateChallenge(context.Background(), "a", "viewer", "a@example.com", "fp", []byte("key"), true, now, time.Minute)
	if err != nil || m == nil {
		t.Fatal(err)
	}
	issue := func(*sql.Tx) error { t.Fatal("wrong OTP issued a session"); return nil }
	for i := 0; i < 2; i++ {
		if err = s.VerifyAndIssue(context.Background(), "a", "viewer", m.Email, m.ID, "000000", []byte("key"), now, 2, issue); !errors.Is(err, ErrOTP) {
			t.Fatalf("wrong attempt %d = %v", i, err)
		}
	}
	var attempts int
	if err = s.DB.QueryRow("SELECT attempts FROM otp_challenges WHERE id=?", m.ID).Scan(&attempts); err != nil || attempts != 2 {
		t.Fatalf("attempts=%d err=%v", attempts, err)
	}
	if err = s.VerifyAndIssue(context.Background(), "a", "viewer", m.Email, m.ID, m.Code, []byte("key"), now, 2, issue); !errors.Is(err, ErrOTP) {
		t.Fatalf("configured limit was bypassed: %v", err)
	}
}

func TestPersistentOTPSuccessConsumesOnceConcurrently(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	now := time.Now()
	m, err := s.CreateChallenge(context.Background(), "a", "viewer", "a@example.com", "fp", []byte("key"), true, now, time.Minute)
	if err != nil || m == nil {
		t.Fatal(err)
	}
	var issued atomic.Int32
	results := make(chan error, 2)
	for range 2 {
		go func() {
			results <- s.VerifyAndIssue(context.Background(), "a", "viewer", m.Email, m.ID, m.Code, []byte("key"), now, 5, func(*sql.Tx) error {
				issued.Add(1)
				return nil
			})
		}()
	}
	success := 0
	for range 2 {
		if <-results == nil {
			success++
		}
	}
	if success != 1 || issued.Load() != 1 {
		t.Fatalf("success=%d issued=%d", success, issued.Load())
	}
}
