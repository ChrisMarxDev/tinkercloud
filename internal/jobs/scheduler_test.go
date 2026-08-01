package jobs

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestSchedulerLeaseAndCancellation(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var runs atomic.Int32
	s := &Scheduler{Timeout: time.Second, Run: func(ctx context.Context) error {
		runs.Add(1)
		close(started)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}}
	go s.RunOnce(context.Background())
	<-started
	if s.RunOnce(context.Background()) {
		t.Fatal("overlapping cleanup acquired lease")
	}
	close(release)
	deadline := time.Now().Add(time.Second)
	for runs.Load() != 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if runs.Load() != 1 {
		t.Fatal("cleanup did not run")
	}
}

func TestSchedulerReportsFailureAndStartStops(t *testing.T) {
	var reports atomic.Int32
	s := &Scheduler{Interval: time.Millisecond, Timeout: time.Second, Run: func(context.Context) error { return context.Canceled }, Report: func(outcome string) {
		if outcome != "failed" {
			t.Errorf("outcome=%q", outcome)
		}
		reports.Add(1)
	}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Start(ctx); close(done) }()
	time.Sleep(15 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("scheduler ignored cancellation")
	}
	if reports.Load() == 0 {
		t.Fatal("failure was not reported")
	}
}
