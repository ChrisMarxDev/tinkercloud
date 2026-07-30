package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type certificateRoundTrip func(*http.Request) (*http.Response, error)

func (f certificateRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func verifiedCertificateResponse(r *http.Request, status int) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader("")),
		Header:     make(http.Header),
		Request:    r,
		TLS:        &tls.ConnectionState{VerifiedChains: [][]*x509.Certificate{{{}}}},
	}
}

func noWait(context.Context, time.Duration) error { return nil }

func testCertificateSchedule(tries int) certificateReadinessSchedule {
	return certificateReadinessSchedule{
		budget:  time.Second,
		attempt: 100 * time.Millisecond,
		tries:   tries,
		delays:  make([]time.Duration, max(tries-1, 0)),
		wait:    noWait,
	}
}

func TestCertificateReadyRetriesOnlyTransientReadinessFailures(t *testing.T) {
	attempts := 0
	client := &http.Client{Transport: certificateRoundTrip(func(r *http.Request) (*http.Response, error) {
		attempts++
		switch attempts {
		case 1:
			return nil, errors.New("temporary TLS failure")
		case 2:
			return verifiedCertificateResponse(r, http.StatusServiceUnavailable), nil
		default:
			return verifiedCertificateResponse(r, http.StatusOK), nil
		}
	})}
	if !certificateReadyWithSchedule(context.Background(), "demo.example.test", client, testCertificateSchedule(4)) {
		t.Fatal("certificate readiness did not recover")
	}
	if attempts != 3 {
		t.Fatalf("attempts=%d want=3", attempts)
	}
}

func TestCertificateReadyRetriesAfterAttemptDeadlineWithinOverallBudget(t *testing.T) {
	attempts := 0
	client := &http.Client{Transport: certificateRoundTrip(func(r *http.Request) (*http.Response, error) {
		attempts++
		if attempts == 1 {
			<-r.Context().Done()
			return nil, r.Context().Err()
		}
		return verifiedCertificateResponse(r, http.StatusOK), nil
	})}
	schedule := testCertificateSchedule(3)
	schedule.attempt = 10 * time.Millisecond
	schedule.budget = time.Second
	if !certificateReadyWithSchedule(context.Background(), "demo.example.test", client, schedule) {
		t.Fatal("attempt deadline did not retry within the overall budget")
	}
	if attempts != 2 {
		t.Fatalf("attempts=%d want=2", attempts)
	}
}

func TestCertificateReadyImmediateSuccessDoesNotRetry(t *testing.T) {
	attempts := 0
	client := &http.Client{Transport: certificateRoundTrip(func(r *http.Request) (*http.Response, error) {
		attempts++
		if r.URL.Path != "/_tinker/api/v1/app" {
			t.Fatalf("readiness path=%q", r.URL.Path)
		}
		return verifiedCertificateResponse(r, http.StatusUnauthorized), nil
	})}
	if !certificateReadyWithSchedule(context.Background(), "demo.example.test", client, testCertificateSchedule(4)) {
		t.Fatal("accepted 4xx readiness status was rejected")
	}
	if attempts != 1 {
		t.Fatalf("attempts=%d want=1", attempts)
	}
}

func TestCertificateReadyActiveAppDoesNotStartLoginHandoff(t *testing.T) {
	client := &http.Client{Transport: certificateRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/_tinker/auth/login" {
			t.Fatal("certificate readiness must not invoke the mutating login handoff")
		}
		if r.URL.Path != "/_tinker/api/v1/app" {
			t.Fatalf("path=%q", r.URL.Path)
		}
		// An active private app returns the normal anonymous app API denial.
		return verifiedCertificateResponse(r, http.StatusUnauthorized), nil
	})}
	if !certificateReadyWithSchedule(context.Background(), "demo.example.test", client, testCertificateSchedule(2)) {
		t.Fatal("active app API denial did not prove certificate readiness")
	}
}

func TestCertificateReadyNeverAcceptsRedirectWrongHostOrUnverifiedTLS(t *testing.T) {
	for _, test := range []struct {
		name     string
		response func(*http.Request) *http.Response
	}{
		{name: "redirect", response: func(r *http.Request) *http.Response { return verifiedCertificateResponse(r, http.StatusFound) }},
		{name: "wrong host", response: func(r *http.Request) *http.Response {
			response := verifiedCertificateResponse(r, http.StatusOK)
			response.Request = r.Clone(r.Context())
			response.Request.URL.Host = "other.example.test"
			return response
		}},
		{name: "unverified TLS", response: func(r *http.Request) *http.Response {
			response := verifiedCertificateResponse(r, http.StatusOK)
			response.TLS = &tls.ConnectionState{}
			return response
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			attempts := 0
			client := &http.Client{Transport: certificateRoundTrip(func(r *http.Request) (*http.Response, error) {
				attempts++
				return test.response(r), nil
			})}
			if certificateReadyWithSchedule(context.Background(), "demo.example.test", client, testCertificateSchedule(3)) {
				t.Fatal("unsafe readiness response was accepted")
			}
			if attempts != 1 {
				t.Fatalf("attempts=%d want terminal evidence denial", attempts)
			}
		})
	}
}

func TestCertificateReadyCancellationAndAttemptBounds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	client := &http.Client{Transport: certificateRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		deadline, ok := r.Context().Deadline()
		if !ok || time.Until(deadline) > 100*time.Millisecond {
			t.Fatal("attempt context exceeds its configured bound")
		}
		cancel()
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}
	schedule := testCertificateSchedule(3)
	schedule.attempt = 50 * time.Millisecond
	schedule.budget = 100 * time.Millisecond
	schedule.wait = func(context.Context, time.Duration) error {
		t.Fatal("cancelled attempt must not wait or retry")
		return nil
	}
	started := time.Now()
	if certificateReadyWithSchedule(ctx, "demo.example.test", client, schedule) {
		t.Fatal("cancelled readiness was accepted")
	}
	if calls != 1 || time.Since(started) > time.Second {
		t.Fatalf("calls=%d elapsed=%s", calls, time.Since(started))
	}
}

func TestCertificateReadyOverallBudgetPreventsAdditionalAttempt(t *testing.T) {
	attempts := 0
	client := &http.Client{Transport: certificateRoundTrip(func(r *http.Request) (*http.Response, error) {
		attempts++
		return nil, errors.New("not ready")
	})}
	schedule := testCertificateSchedule(4)
	schedule.budget = 10 * time.Millisecond
	schedule.wait = func(ctx context.Context, _ time.Duration) error {
		<-ctx.Done()
		return ctx.Err()
	}
	if certificateReadyWithSchedule(context.Background(), "demo.example.test", client, schedule) {
		t.Fatal("expired budget was accepted")
	}
	if attempts != 1 {
		t.Fatalf("attempts=%d want=1 after overall budget exhaustion", attempts)
	}
}
