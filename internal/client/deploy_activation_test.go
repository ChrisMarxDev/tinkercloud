package client

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
	"time"
)

type rt func(*http.Request) (*http.Response, error)

func (f rt) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestDeployActivatesVerified(t *testing.T) {
	n := 0
	c := New("https://tiny.test", "secret-token")
	c.HTTP = &http.Client{Transport: rt(func(r *http.Request) (*http.Response, error) {
		n++
		if n <= 2 && r.Header.Get("Authorization") != "Bearer secret-token" {
			t.Fatal("bearer")
		}
		if n == 1 {
			return &http.Response{StatusCode: 202, Body: io.NopCloser(strings.NewReader(`{"deployment_id":"d","state":"verified"}`)), Header: make(http.Header), Request: r}, nil
		}
		if n == 2 && (r.URL.Path != "/api/v1/apps/demo/deployments/d/activate" || r.Header.Get("Idempotency-Key") == "") {
			t.Fatal(r.URL, r.Header)
		}
		if n == 2 {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"deployment_id":"d","url":"https://demo.tiny.test/","app_suffix":"tiny.test","policy_ready":true,"tls_ready":true,"anonymous_denied":true,"authenticated_healthy":true}`)), Header: make(http.Header), Request: r}, nil
		}
		if n != 3 || r.URL.String() != "https://demo.tiny.test/" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Fatalf("anonymous probe request=%s headers=%v", r.URL, r.Header)
		}
		return anonymousDenied(r), nil
	})}
	o, e := c.Deploy(context.Background(), "demo", bytes.NewReader([]byte("x")), 1, "upload")
	if e != nil || o.DeploymentID != "d" {
		t.Fatal(o, e)
	}
}

func TestDeployActivatesWhenPollingReachesVerified(t *testing.T) {
	n := 0
	c := New("https://tiny.test", "secret-token")
	c.HTTP = &http.Client{Transport: rt(func(r *http.Request) (*http.Response, error) {
		n++
		switch n {
		case 1:
			if r.Method != http.MethodPost || r.URL.Path != "/api/v1/apps/demo/deployments" || r.Header.Get("Authorization") != "Bearer secret-token" {
				t.Fatalf("unexpected upload request: %s %s headers=%v", r.Method, r.URL, r.Header)
			}
			return &http.Response{StatusCode: http.StatusAccepted, Body: io.NopCloser(strings.NewReader(`{"deployment_id":"d","state":"staged","status_url":"https://tiny.test/api/v1/apps/demo/deployments/d"}`)), Header: make(http.Header), Request: r}, nil
		case 2:
			if r.Method != http.MethodGet || r.URL.String() != "https://tiny.test/api/v1/apps/demo/deployments/d" {
				t.Fatalf("unexpected status request: %s %s", r.Method, r.URL)
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"deployment_id":"d","state":"verified"}`)), Header: make(http.Header), Request: r}, nil
		case 3:
			if r.Method != http.MethodPost || r.URL.Path != "/api/v1/apps/demo/deployments/d/activate" || r.Header.Get("Authorization") != "Bearer secret-token" || r.Header.Get("Idempotency-Key") == "" {
				t.Fatalf("unexpected activation request: %s %s headers=%v", r.Method, r.URL, r.Header)
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"deployment_id":"d","url":"https://demo.tiny.test/","app_suffix":"tiny.test","policy_ready":true,"tls_ready":true,"anonymous_denied":true,"authenticated_healthy":true}`)), Header: make(http.Header), Request: r}, nil
		case 4:
			if r.URL.String() != "https://demo.tiny.test/" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
				t.Fatalf("anonymous probe request=%s headers=%v", r.URL, r.Header)
			}
			return anonymousDenied(r), nil
		default:
			t.Fatalf("unexpected request %d: %s %s", n, r.Method, r.URL)
			return nil, nil
		}
	})}

	result, err := c.Deploy(context.Background(), "demo", bytes.NewReader([]byte("x")), 1, "upload")
	if err != nil || result.DeploymentID != "d" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if n != 4 {
		t.Fatalf("request count = %d, want 4", n)
	}
}

func anonymousDenied(r *http.Request) *http.Response {
	h := make(http.Header)
	h.Set("Content-Type", "application/json; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Request-ID", "req_0123456789abcdef01234567")
	return &http.Response{StatusCode: http.StatusUnauthorized, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"not_authorized","message":"This request is not authorized.","request_id":"req_0123456789abcdef01234567"}}`)), Header: h, Request: r}
}

func deployedClient(probe func(*http.Request) (*http.Response, error)) Client {
	n := 0
	c := New("https://tiny.test", "secret-token")
	c.HTTP = &http.Client{Transport: rt(func(r *http.Request) (*http.Response, error) {
		n++
		switch n {
		case 1:
			return &http.Response{StatusCode: http.StatusAccepted, Body: io.NopCloser(strings.NewReader(`{"deployment_id":"d","state":"verified"}`)), Header: make(http.Header), Request: r}, nil
		case 2:
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"deployment_id":"d","url":"https://demo.tiny.test/","app_suffix":"tiny.test","policy_ready":true,"tls_ready":true,"anonymous_denied":true,"authenticated_healthy":true}`)), Header: make(http.Header), Request: r}, nil
		default:
			return probe(r)
		}
	})}
	c.publicProbeMaxAttempts = 1
	c.publicProbeRetryDelay = time.Nanosecond
	return c
}

func TestDeployRejectsInvalidPublicGatewayEvidence(t *testing.T) {
	for name, probe := range map[string]func(*http.Request) (*http.Response, error){
		"404 is not app denial": func(r *http.Request) (*http.Response, error) {
			out := anonymousDenied(r)
			out.StatusCode = http.StatusNotFound
			return out, nil
		},
		"public 200 is not app denial": func(r *http.Request) (*http.Response, error) {
			out := anonymousDenied(r)
			out.StatusCode = http.StatusOK
			return out, nil
		},
		"malformed envelope": func(r *http.Request) (*http.Response, error) {
			out := anonymousDenied(r)
			out.Body = io.NopCloser(strings.NewReader(`{"error":{"code":"not_found"}}`))
			return out, nil
		},
		"mismatched request id": func(r *http.Request) (*http.Response, error) {
			out := anonymousDenied(r)
			out.Header.Set("X-Request-ID", "req_abcdefabcdefabcdefabcdef")
			return out, nil
		},
		"oversized body": func(r *http.Request) (*http.Response, error) {
			out := anonymousDenied(r)
			out.Body = io.NopCloser(strings.NewReader(strings.Repeat("x", int(maxAnonymousDenyEvidenceBytes+1))))
			return out, nil
		},
		"transport failure": func(*http.Request) (*http.Response, error) {
			return nil, errors.New("dial failed")
		},
		"redirect is not a denial": func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": []string{"https://evil.example/"}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := deployedClient(probe).Deploy(context.Background(), "demo", bytes.NewReader([]byte("x")), 1, "upload")
			if !errors.Is(err, ErrDeploymentEvidence) {
				t.Fatalf("error = %v, want deployment evidence denial", err)
			}
		})
	}
}

func TestDeployRetriesOnlyTransientPublicReadiness(t *testing.T) {
	probes := 0
	c := deployedClient(func(r *http.Request) (*http.Response, error) {
		probes++
		switch probes {
		case 1:
			return nil, errors.New("temporary DNS failure")
		case 2:
			out := anonymousDenied(r)
			out.StatusCode = http.StatusNotFound
			return out, nil
		default:
			return anonymousDenied(r), nil
		}
	})
	c.publicProbeMaxAttempts = 3
	c.publicProbeRetryDelay = time.Nanosecond
	c.publicProbeBudget = time.Second

	result, err := c.Deploy(context.Background(), "demo", bytes.NewReader([]byte("x")), 1, "upload")
	if err != nil || result.DeploymentID != "d" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if probes != 3 {
		t.Fatalf("public probes = %d, want 3", probes)
	}
}

func TestDeployDoesNotRetryContradictoryPublicEvidence(t *testing.T) {
	probes := 0
	c := deployedClient(func(r *http.Request) (*http.Response, error) {
		probes++
		out := anonymousDenied(r)
		out.StatusCode = http.StatusOK
		return out, nil
	})
	c.publicProbeMaxAttempts = 3
	c.publicProbeRetryDelay = time.Nanosecond

	_, err := c.Deploy(context.Background(), "demo", bytes.NewReader([]byte("x")), 1, "upload")
	var active *ActiveButUnverifiedError
	if !errors.As(err, &active) || active.Reason != EvidencePublicProbeInvalid {
		t.Fatalf("error = %#v, want active invalid-response evidence", err)
	}
	if probes != 1 {
		t.Fatalf("unsafe evidence was retried %d times", probes)
	}
}

func TestDeployReturnsActiveReceiptAfterPublicProbeExhaustion(t *testing.T) {
	probes := 0
	c := deployedClient(func(*http.Request) (*http.Response, error) {
		probes++
		return nil, errors.New("raw transport detail must not escape")
	})
	c.publicProbeMaxAttempts = 2
	c.publicProbeRetryDelay = time.Nanosecond
	c.publicProbeBudget = time.Second

	result, err := c.Deploy(context.Background(), "demo", bytes.NewReader([]byte("x")), 1, "upload")
	var active *ActiveButUnverifiedError
	if !errors.As(err, &active) || !errors.Is(err, ErrDeploymentEvidence) {
		t.Fatalf("error = %#v, want active-but-unverified evidence", err)
	}
	if result.DeploymentID != "d" || active.Deployment.DeploymentID != "d" ||
		active.Deployment.URL != "https://demo.tiny.test/" ||
		active.Reason != EvidencePublicProbeTransport {
		t.Fatalf("result=%+v active=%+v", result, active)
	}
	if strings.Contains(err.Error(), "raw transport") {
		t.Fatalf("raw transport error escaped: %v", err)
	}
	if probes != 2 {
		t.Fatalf("public probes = %d, want 2", probes)
	}
}

func TestDeployControlBudgetOutlivesGenericClientTimeout(t *testing.T) {
	n := 0
	c := New("https://tiny.test", "secret-token")
	c.HTTP = &http.Client{
		Timeout: 5 * time.Millisecond,
		Transport: rt(func(r *http.Request) (*http.Response, error) {
			n++
			if n == 1 {
				return &http.Response{StatusCode: http.StatusAccepted, Body: io.NopCloser(strings.NewReader(`{"deployment_id":"d","state":"verified"}`)), Header: make(http.Header), Request: r}, nil
			}
			if n == 2 {
				select {
				case <-r.Context().Done():
					return nil, r.Context().Err()
				case <-time.After(20 * time.Millisecond):
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"deployment_id":"d","url":"https://demo.tiny.test/","app_suffix":"tiny.test","policy_ready":true,"tls_ready":true,"anonymous_denied":true,"authenticated_healthy":true}`)), Header: make(http.Header), Request: r}, nil
			}
			return anonymousDenied(r), nil
		}),
	}

	result, err := c.Deploy(context.Background(), "demo", bytes.NewReader([]byte("x")), 1, "upload")
	if err != nil || result.DeploymentID != "d" {
		t.Fatalf("deployment request inherited generic 5ms timeout: result=%+v err=%v", result, err)
	}
}

func TestDeployControlBudgetStillHonorsCallerCancellation(t *testing.T) {
	n := 0
	c := New("https://tiny.test", "secret-token")
	c.HTTP = &http.Client{Transport: rt(func(r *http.Request) (*http.Response, error) {
		n++
		if n == 1 {
			return &http.Response{StatusCode: http.StatusAccepted, Body: io.NopCloser(strings.NewReader(`{"deployment_id":"d","state":"verified"}`)), Header: make(http.Header), Request: r}, nil
		}
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()

	if _, err := c.Deploy(ctx, "demo", bytes.NewReader([]byte("x")), 1, "upload"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want caller deadline", err)
	}
}

func TestDeployPublicProbeUsesNoCookieJarOrBearer(t *testing.T) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	c := deployedClient(func(r *http.Request) (*http.Response, error) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("deployer authorization leaked to app host: %q", got)
		}
		if got := r.Header.Get("Cookie"); got != "" {
			t.Fatalf("cookie leaked to app host: %q", got)
		}
		return anonymousDenied(r), nil
	})
	c.HTTP.Jar = jar
	// The app-origin cookie proves this is stronger than merely avoiding a
	// manually-added Cookie header on the request.
	appURL, _ := http.NewRequest(http.MethodGet, "https://demo.tiny.test/", nil)
	jar.SetCookies(appURL.URL, []*http.Cookie{{Name: "session", Value: "viewer"}})
	if _, err := c.Deploy(context.Background(), "demo", bytes.NewReader([]byte("x")), 1, "upload"); err != nil {
		t.Fatal(err)
	}
}

func TestDeployRejectsUnexpectedPublicURL(t *testing.T) {
	c := New("https://tiny.test", "secret-token")
	n := 0
	c.HTTP = &http.Client{Transport: rt(func(r *http.Request) (*http.Response, error) {
		n++
		if n == 1 {
			return &http.Response{StatusCode: http.StatusAccepted, Body: io.NopCloser(strings.NewReader(`{"deployment_id":"d","state":"verified"}`)), Header: make(http.Header), Request: r}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"deployment_id":"d","url":"https://other.tiny.test/","app_suffix":"tiny.test","policy_ready":true,"tls_ready":true,"anonymous_denied":true,"authenticated_healthy":true}`)), Header: make(http.Header), Request: r}, nil
	})}
	if _, err := c.Deploy(context.Background(), "demo", bytes.NewReader([]byte("x")), 1, "upload"); !errors.Is(err, ErrDeploymentEvidence) {
		t.Fatalf("error = %v, want deployment evidence denial", err)
	}
	if n != 2 {
		t.Fatalf("unexpected public probe to untrusted URL: %d requests", n)
	}
}

func TestExpectedAppURLUsesServerDerivedSuffixNotControlHost(t *testing.T) {
	// Production commonly exposes control at tiny.example.com while the
	// wildcard app gateway is *.apps.example.com. The activation response's
	// server-derived suffix, rather than the control base URL, binds that host.
	u, err := expectedAppURL("demo", "apps.example.com", "https://demo.apps.example.com/")
	if err != nil || u.String() != "https://demo.apps.example.com/" {
		t.Fatalf("expected documented app URL, got %v %v", u, err)
	}
	if _, err := expectedAppURL("demo", "apps.example.com", "https://demo.apps.example.com.evil/"); err == nil {
		t.Fatal("accepted a suffix-confusion app URL")
	}
}

func TestDeployActivationEvidenceDenied(t *testing.T) {
	c := New("https://tiny.test", "secret")
	n := 0
	c.HTTP = &http.Client{Transport: rt(func(r *http.Request) (*http.Response, error) {
		n++
		body := `{"deployment_id":"d","state":"verified"}`
		if n == 2 {
			body = `{"deployment_id":"d"}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: r}, nil
	})}
	_, e := c.Deploy(context.Background(), "demo", bytes.NewReader([]byte("x")), 1, "k")
	if e == nil || strings.Contains(e.Error(), "secret") {
		t.Fatal(e)
	}
}
