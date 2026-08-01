package main

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"
)

const (
	certificateReadinessBudget  = 45 * time.Second
	certificateReadinessAttempt = 5 * time.Second
	certificateReadinessTries   = 8
)

// certificateReadinessSchedule is injectable so the production retry policy is
// bounded and tests can exercise every retry transition without sleeping.
type certificateReadinessSchedule struct {
	budget  time.Duration
	attempt time.Duration
	tries   int
	delays  []time.Duration
	wait    func(context.Context, time.Duration) error
}

func productionCertificateReadinessSchedule() certificateReadinessSchedule {
	return certificateReadinessSchedule{
		budget:  certificateReadinessBudget,
		attempt: certificateReadinessAttempt,
		tries:   certificateReadinessTries,
		delays:  []time.Duration{time.Second, 2 * time.Second, 3 * time.Second, 5 * time.Second, 7 * time.Second, 9 * time.Second, 12 * time.Second},
		wait:    waitForCertificateRetry,
	}
}

// certificateReady gives a newly staged app one finite opportunity to obtain
// its certificate before activation. It retries only this pre-activation
// readiness proof; policy, ownership, and all post-activation evidence retain
// their independent fail-closed gates.
func certificateReady(ctx context.Context, host string, c *http.Client) bool {
	return certificateReadyWithSchedule(ctx, host, c, productionCertificateReadinessSchedule())
}

func certificateReadyWithSchedule(ctx context.Context, host string, c *http.Client, schedule certificateReadinessSchedule) bool {
	if c == nil || host == "" || schedule.budget <= 0 || schedule.attempt <= 0 || schedule.tries <= 0 || len(schedule.delays) < schedule.tries-1 || schedule.wait == nil {
		return false
	}
	budgetCtx, cancel := context.WithTimeout(ctx, schedule.budget)
	defer cancel()
	for attempt := 0; attempt < schedule.tries; attempt++ {
		if budgetCtx.Err() != nil {
			return false
		}
		attemptCtx, cancelAttempt := context.WithTimeout(budgetCtx, schedule.attempt)
		ready, retryable := certificateReadyOnce(attemptCtx, host, c)
		cancelAttempt()
		if ready {
			return true
		}
		if !retryable || attempt == schedule.tries-1 || budgetCtx.Err() != nil {
			return false
		}
		if err := schedule.wait(budgetCtx, schedule.delays[attempt]); err != nil {
			return false
		}
	}
	return false
}

func certificateReadyOnce(ctx context.Context, host string, c *http.Client) (ready bool, retryable bool) {
	// This must remain a protected endpoint that does not issue an app handoff
	// or otherwise mutate viewer authentication state. The login route redirects
	// after an app becomes active, which makes a same-host redeploy impossible.
	u := (&url.URL{Scheme: "https", Host: host, Path: "/_tinker/api/v1/app"}).String()
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return false, false
	}
	// Clone the value so readiness checks never mutate a shared client while a
	// deployment is being uploaded or another candidate is verified.
	probe := *c
	probe.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, err := probe.Do(r)
	if err != nil {
		// A per-attempt deadline is the expected shape while ACME is still
		// issuing. The caller owns the longer budget context and decides whether
		// another attempt remains possible after this transport failure.
		return false, true
	}
	defer res.Body.Close()
	if res.Request == nil || res.Request.URL.Scheme != "https" || res.Request.URL.Host != host || res.TLS == nil || len(res.TLS.VerifiedChains) == 0 {
		return false, false
	}
	if res.StatusCode >= http.StatusInternalServerError {
		return false, true
	}
	return certificateReadinessStatus(res.StatusCode), false
}

func certificateReadinessStatus(status int) bool {
	// Redirects are intentionally not readiness: a newly activated origin must
	// prove its own exact HTTPS endpoint, not a redirect target.
	return (status >= http.StatusOK && status < http.StatusMultipleChoices) || (status >= http.StatusBadRequest && status < http.StatusInternalServerError)
}

func waitForCertificateRetry(ctx context.Context, delay time.Duration) error {
	if delay < 0 {
		return errors.New("negative retry delay")
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
