package persistence

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAppLoginWrongAttemptCommitsConfiguredLimit(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	if _, err := s.DB.Exec("INSERT INTO access_policies(app_id,revision,mode,created_by,created_at) VALUES('a',1,'private','u',datetime('now')); INSERT INTO access_rules(id,app_id,policy_revision,kind,normalized_value,created_by,created_at) VALUES('viewer','a',1,'email','viewer@example.com','u',datetime('now'))"); err != nil {
		t.Fatal(err)
	}
	o := &captureOutbox{}
	login := AppLogin{Store: s, HMACKey: []byte("key"), Outbox: o, MaxAttempts: 2}
	challenge, err := login.Request(context.Background(), "a", "viewer@example.com", true)
	if err != nil || challenge == "" || o.m.Code == "" {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err = login.VerifyAndCreateSession(context.Background(), "a", "viewer@example.com", challenge, "000000", time.Now().Add(time.Hour)); !errors.Is(err, ErrOTP) {
			t.Fatalf("wrong attempt %d = %v", i, err)
		}
	}
	var attempts int
	if err = s.DB.QueryRow("SELECT attempts FROM otp_challenges WHERE id=?", challenge).Scan(&attempts); err != nil || attempts != 2 {
		t.Fatalf("attempts=%d err=%v", attempts, err)
	}
	if _, err = login.VerifyAndCreateSession(context.Background(), "a", "viewer@example.com", challenge, o.m.Code, time.Now().Add(time.Hour)); !errors.Is(err, ErrOTP) {
		t.Fatalf("configured limit was bypassed: %v", err)
	}
}
