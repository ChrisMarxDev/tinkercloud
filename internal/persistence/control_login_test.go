package persistence

import (
	"context"
	"errors"
	"github.com/tinyhost/tiny/internal/controlapi"
	"github.com/tinyhost/tiny/internal/otp"
	"testing"
	"time"
)

type captureOutbox struct{ m otp.Message }

func (c *captureOutbox) EnqueueOTP(_ context.Context, m otp.Message) error { c.m = m; return nil }
func TestControlLoginOneTimeToken(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	o := &captureOutbox{}
	l := ControlLogin{Store: s, HMACKey: []byte("key"), Outbox: o}
	tx, e := l.RequestOTP(context.Background(), "owner@example.com", controlapi.BrowserLoginChannel)
	if e != nil || tx == "" || o.m.Code == "" {
		t.Fatal(e)
	}
	var raw []byte
	if e = s.DB.QueryRow("SELECT code_hash FROM otp_challenges WHERE id=?", tx).Scan(&raw); e != nil || string(raw) == o.m.Code {
		t.Fatal("raw OTP")
	}
	token, e := l.VerifyOTP(context.Background(), tx, o.m.Code, controlapi.BrowserLoginChannel)
	if e != nil || token == "" {
		t.Fatal(e)
	}
	if _, e = s.AuthenticateControlSession(context.Background(), token, time.Now()); e != nil {
		t.Fatal(e)
	}
	if _, e = s.AuthenticateToken(context.Background(), token, "app:read", "", time.Now()); e == nil {
		t.Fatal("browser control credential authenticated as bearer")
	}
	if _, e = l.VerifyOTP(context.Background(), tx, o.m.Code, controlapi.BrowserLoginChannel); e == nil {
		t.Fatal("replay")
	}
}

func TestControlLoginLogoutRevokesCredential(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	o := &captureOutbox{}
	l := ControlLogin{Store: s, HMACKey: []byte("key"), Outbox: o}
	tx, err := l.RequestOTP(context.Background(), "owner@example.com", controlapi.BrowserLoginChannel)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := l.VerifyOTP(context.Background(), tx, o.m.Code, controlapi.BrowserLoginChannel)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.RevokeControlCredential(context.Background(), raw); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AuthenticateControlSession(context.Background(), raw, time.Now()); err == nil {
		t.Fatal("logged out credential remained valid")
	}
}

func TestControlBrowserSessionsRevokeIndependently(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	o := &captureOutbox{}
	l := ControlLogin{Store: s, HMACKey: []byte("key"), Outbox: o}
	issue := func() string {
		tx, err := l.RequestOTP(context.Background(), "owner@example.com", controlapi.BrowserLoginChannel)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := l.VerifyOTP(context.Background(), tx, o.m.Code, controlapi.BrowserLoginChannel)
		if err != nil || raw == "" {
			t.Fatal(err)
		}
		return raw
	}
	first, second := issue(), issue()
	if first == second {
		t.Fatal("control login reused an opaque browser credential")
	}
	if _, err := s.AuthenticateControlSession(context.Background(), first, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AuthenticateControlSession(context.Background(), second, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := l.RevokeControlCredential(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AuthenticateControlSession(context.Background(), first, time.Now()); err == nil {
		t.Fatal("revoked browser session remained valid")
	}
	if _, err := s.AuthenticateControlSession(context.Background(), second, time.Now()); err != nil {
		t.Fatal("revoking one browser session revoked its sibling")
	}
}

func TestControlLoginWrongAttemptCommitsConfiguredLimit(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	o := &captureOutbox{}
	l := ControlLogin{Store: s, HMACKey: []byte("key"), Outbox: o, MaxAttempts: 2}
	tx, err := l.RequestOTP(context.Background(), "owner@example.com", controlapi.BrowserLoginChannel)
	if err != nil || tx == "" || o.m.Code == "" {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err = l.VerifyOTP(context.Background(), tx, "000000", controlapi.BrowserLoginChannel); !errors.Is(err, ErrOTP) {
			t.Fatalf("wrong attempt %d = %v", i, err)
		}
	}
	var attempts int
	if err = s.DB.QueryRow("SELECT attempts FROM otp_challenges WHERE id=?", tx).Scan(&attempts); err != nil || attempts != 2 {
		t.Fatalf("attempts=%d err=%v", attempts, err)
	}
	if _, err = l.VerifyOTP(context.Background(), tx, o.m.Code, controlapi.BrowserLoginChannel); !errors.Is(err, ErrOTP) {
		t.Fatalf("configured limit was bypassed: %v", err)
	}
}

func TestControlLoginChannelCannotMintOtherCredential(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	outbox := &captureOutbox{}
	login := ControlLogin{Store: s, HMACKey: []byte("key"), Outbox: outbox}
	tx, err := login.RequestOTP(context.Background(), "owner@example.com", controlapi.BrowserLoginChannel)
	if err != nil || tx == "" || outbox.m.Code == "" {
		t.Fatal(err)
	}
	if _, err = login.VerifyOTP(context.Background(), tx, outbox.m.Code, controlapi.CLILoginChannel); !errors.Is(err, ErrOTP) {
		t.Fatalf("browser challenge issued CLI credential: %v", err)
	}
	browser, err := login.VerifyOTP(context.Background(), tx, outbox.m.Code, controlapi.BrowserLoginChannel)
	if err != nil || browser == "" {
		t.Fatal(err)
	}
	if _, err = s.AuthenticateToken(context.Background(), browser, "app:read", "", time.Now()); err == nil {
		t.Fatal("browser credential accepted as bearer")
	}
	cliTx, err := login.RequestOTP(context.Background(), "owner@example.com", controlapi.CLILoginChannel)
	if err != nil || cliTx == "" || outbox.m.Code == "" {
		t.Fatal(err)
	}
	cli, err := login.VerifyOTP(context.Background(), cliTx, outbox.m.Code, controlapi.CLILoginChannel)
	if err != nil || cli == "" {
		t.Fatal(err)
	}
	if _, err = s.AuthenticateControlSession(context.Background(), cli, time.Now()); err == nil {
		t.Fatal("CLI bearer authenticated as browser control session")
	}
}
