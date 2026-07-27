package jobs

import (
	"context"
	"sync"
	"time"
)

// Scheduler is the single-process maintenance loop. Its lease prevents a
// slow cleanup from overlapping the next tick; cancellation always ends both
// the ticker and any active run through the derived context.
type Scheduler struct {
	Interval time.Duration
	Timeout  time.Duration
	Run      func(context.Context) error
	Report   func(outcome string)

	mu      sync.Mutex
	running bool
}

func (s *Scheduler) Start(ctx context.Context) {
	if s == nil || s.Run == nil {
		return
	}
	interval := s.Interval
	if interval <= 0 {
		interval = 6 * time.Hour
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.run(ctx)
		}
	}
}

// RunOnce is intentionally exported for startup and deterministic tests. A
// skipped run is not an error: a prior held lease remains authoritative.
func (s *Scheduler) RunOnce(ctx context.Context) bool { return s.run(ctx) }

func (s *Scheduler) run(parent context.Context) bool {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return false
	}
	s.running = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()
	timeout := s.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	outcome := "succeeded"
	if s.Run(ctx) != nil {
		outcome = "failed"
	}
	if s.Report != nil {
		s.Report(outcome)
	}
	return true
}
