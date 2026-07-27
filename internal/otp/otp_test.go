package otp

import (
	"context"
	"testing"
	"time"
)

type capture struct{ message Message }

func (c *capture) EnqueueOTP(_ context.Context, m Message) error { c.message = m; return nil }
func TestNewChallengeInvalidatesOldAndConsumesOnce(t *testing.T) {
	store := NewMemoryStore()
	out := &capture{}
	now := time.Now()
	s := Service{Store: store, Outbox: out, Key: []byte("test-key"), Clock: func() time.Time { return now }, TTL: time.Minute, MaxAttempts: 2}
	if err := s.Request(context.Background(), "a", "alice@example.com", true); err != nil {
		t.Fatal(err)
	}
	old := out.message
	if err := s.Request(context.Background(), "a", "alice@example.com", true); err != nil {
		t.Fatal(err)
	}
	if err := s.Verify(context.Background(), "a", "alice@example.com", old.ChallengeID, old.Code, func(Challenge) error { return nil }); err == nil {
		t.Fatal("old challenge accepted")
	}
	current := out.message
	count := 0
	if err := s.Verify(context.Background(), "a", "alice@example.com", current.ChallengeID, current.Code, func(Challenge) error { count++; return nil }); err != nil || count != 1 {
		t.Fatal("current challenge failed")
	}
	if err := s.Verify(context.Background(), "a", "alice@example.com", current.ChallengeID, current.Code, func(Challenge) error { return nil }); err == nil {
		t.Fatal("replayed challenge accepted")
	}
}
func TestWrongAttemptsDeny(t *testing.T) {
	store := NewMemoryStore()
	out := &capture{}
	s := Service{Store: store, Outbox: out, Key: []byte("test-key"), TTL: time.Minute, MaxAttempts: 2}
	_ = s.Request(context.Background(), "a", "a@b.co", true)
	for i := 0; i < 2; i++ {
		if s.Verify(context.Background(), "a", "a@b.co", out.message.ChallengeID, "000000", func(Challenge) error { return nil }) == nil {
			t.Fatal("wrong code accepted")
		}
	}
}
