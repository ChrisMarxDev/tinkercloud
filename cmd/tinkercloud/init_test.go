package main

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ChrisMarxDev/tinkercloud/internal/config"
	"github.com/ChrisMarxDev/tinkercloud/internal/operations"
	"github.com/ChrisMarxDev/tinkercloud/internal/persistence"
)

func fakeInitRuntime(t *testing.T) (initRuntime, *int) {
	t.Helper()
	installs := new(int)
	rt := initRuntime{
		GOOS:     func() string { return "linux" },
		GOARCH:   func() string { return "amd64" },
		ReadFile: func(string) ([]byte, error) { return []byte("ID=ubuntu\nVERSION_ID=\"26.04\"\n"), nil },
		Listen: func(string, string) (net.Listener, error) {
			a, b := net.Pipe()
			_ = b.Close()
			return pipeListener{Conn: a}, nil
		},
		ClockOK: func(context.Context) error { return nil },
		Run: func(_ context.Context, name string, _ ...string) error {
			if name == "id" {
				return nil
			}
			return nil
		},
		Install:                func(string, string) error { *installs++; return nil },
		DNSLookup:              func(context.Context, string) ([]string, error) { return []string{"127.0.0.1"}, nil },
		PrepareSQLiteOwnership: func(string) error { return nil },
		InitializeSQLite: func(ctx context.Context, databasePath, operatorEmail string) error {
			db, err := persistence.OpenSQLite(ctx, databasePath)
			if err != nil {
				return err
			}
			if operatorEmail != "" {
				err = db.EnsureInitialOperator(ctx, operatorEmail, "init")
			}
			closeErr := db.Close()
			if err != nil {
				return err
			}
			return closeErr
		},
		LocalHealth:  func(context.Context, string) error { return nil },
		PublicHealth: func(context.Context, string) error { return nil },
	}
	return rt, installs
}

func TestInitSQLiteDropsToServiceIdentityBeforeOpeningDatabase(t *testing.T) {
	oldDrop, oldOpen := dropToTinkercloudIdentity, openInitSQLite
	t.Cleanup(func() {
		dropToTinkercloudIdentity = oldDrop
		openInitSQLite = oldOpen
	})
	dropped := false
	dropToTinkercloudIdentity = func() error {
		dropped = true
		return nil
	}
	openInitSQLite = func(ctx context.Context, databasePath string) (*persistence.SQLiteStore, error) {
		if !dropped {
			t.Fatal("SQLite opened before dropping to the service identity")
		}
		return persistence.OpenSQLite(ctx, databasePath)
	}

	databasePath := filepath.Join(t.TempDir(), "tinkercloud.db")
	if err := runInitSQLiteInProcess(context.Background(), databasePath, "operator@example.test"); err != nil {
		t.Fatal(err)
	}
	if !dropped {
		t.Fatal("SQLite opened without dropping to the service identity")
	}
	store, err := persistence.OpenSQLite(context.Background(), databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var count int
	if err = store.DB.QueryRow("SELECT COUNT(*) FROM users WHERE normalized_email='operator@example.test' AND role='operator'").Scan(&count); err != nil || count != 1 {
		t.Fatalf("initial operator = %d, %v", count, err)
	}
}

// pipeListener only exists to exercise the injected port check without taking
// a real public port during the test suite.
type pipeListener struct{ net.Conn }

func (p pipeListener) Accept() (net.Conn, error) { return nil, errors.New("not supported") }
func (p pipeListener) Close() error              { return p.Conn.Close() }
func (p pipeListener) Addr() net.Addr            { return p.Conn.LocalAddr() }

func initArgs(root string) []string {
	return []string{"--non-interactive", "--config", filepath.Join(root, "etc", "config.yaml"), "--credentials", filepath.Join(root, "etc", "credentials", "tinkercloud.env"), "--domain", "apps.tinker.example.test", "--operator-email", "operator@example.test", "--email-from", "operator@example.test", "--data-directory", filepath.Join(root, "data"), "--acme-cache-directory", filepath.Join(root, "acme"), "--resend-api-key-file", filepath.Join(root, "resend"), "--hmac-key-file", filepath.Join(root, "hmac")}
}

func TestSupportedUbuntuHostDenyCharter(t *testing.T) {
	t.Parallel()
	for name, osRelease := range map[string]string{
		"missing id":             `VERSION_ID="26.04"`,
		"missing version":        `ID=ubuntu`,
		"other distribution":     "ID=debian\nVERSION_ID=\"26.04\"\n",
		"id-like only":           "ID=debian\nID_LIKE=ubuntu\nVERSION_ID=\"26.04\"\n",
		"old release":            "ID=ubuntu\nVERSION_ID=\"22.04\"\n",
		"interim 24":             "ID=ubuntu\nVERSION_ID=\"24.10\"\n",
		"interim 25.04":          "ID=ubuntu\nVERSION_ID=\"25.04\"\n",
		"interim 25.10":          "ID=ubuntu\nVERSION_ID=\"25.10\"\n",
		"future release":         "ID=ubuntu\nVERSION_ID=\"26.10\"\n",
		"version prefix":         "ID=ubuntu\nVERSION_ID=\"26.040\"\n",
		"version suffix":         "ID=ubuntu\nVERSION_ID=\"26.04-lts\"\n",
		"point-like impostor":    "ID=ubuntu\nVERSION_ID=\"26.04.1\"\n",
		"duplicate id":           "ID=ubuntu\nID=ubuntu\nVERSION_ID=\"26.04\"\n",
		"conflicting id":         "ID=ubuntu\nID=debian\nVERSION_ID=\"26.04\"\n",
		"duplicate version":      "ID=ubuntu\nVERSION_ID=\"26.04\"\nVERSION_ID=\"26.04\"\n",
		"conflicting version":    "ID=ubuntu\nVERSION_ID=\"24.04\"\nVERSION_ID=\"26.04\"\n",
		"unterminated id quote":  "ID=\"ubuntu\nVERSION_ID=\"26.04\"\n",
		"unterminated version":   "ID=ubuntu\nVERSION_ID=\"26.04\n",
		"trailing version input": "ID=ubuntu\nVERSION_ID=\"26.04\" unexpected\n",
	} {
		osRelease := osRelease
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if supportedUbuntuHost([]byte(osRelease)) {
				t.Fatal("unsupported or ambiguous host accepted")
			}
		})
	}
}

func TestSupportedUbuntuHostAllowlist(t *testing.T) {
	t.Parallel()
	for _, version := range []string{"24.04", "26.04"} {
		version := version
		t.Run(version, func(t *testing.T) {
			t.Parallel()
			if !supportedUbuntuHost([]byte("NAME=\"Ubuntu\"\nID=ubuntu\nVERSION_ID=\"" + version + "\"\n")) {
				t.Fatal("supported Ubuntu LTS denied")
			}
		})
	}
}

func TestRunPreflightDeniesUnsupportedOSBeforeDependencyChecks(t *testing.T) {
	t.Parallel()
	rt, _ := fakeInitRuntime(t)
	rt.ReadFile = func(string) ([]byte, error) {
		return []byte("ID=ubuntu\nVERSION_ID=\"25.10\"\n"), nil
	}
	clockChecked := false
	listenerChecked := false
	rt.ClockOK = func(context.Context) error {
		clockChecked = true
		return nil
	}
	rt.Listen = func(string, string) (net.Listener, error) {
		listenerChecked = true
		return nil, errors.New("must not be called")
	}
	if err := runPreflight(context.Background(), rt); err == nil || err.Error() != "tinkercloud: unsupported_host" {
		t.Fatalf("unsupported host result: %v", err)
	}
	if clockChecked || listenerChecked {
		t.Fatal("unsupported host reached dependency checks")
	}
}

func writeInitSecrets(t *testing.T, root string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "resend"), []byte("resend-secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "hmac"), []byte("0123456789abcdef0123456789abcdef"), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestInitCompletesOnlyAfterAllDurableSteps(t *testing.T) {
	root := t.TempDir()
	writeInitSecrets(t, root)
	rt, installs := fakeInitRuntime(t)
	var sqliteCalls []string
	initializeSQLite := rt.InitializeSQLite
	rt.PrepareSQLiteOwnership = func(databasePath string) error {
		sqliteCalls = append(sqliteCalls, "repair:"+databasePath)
		return nil
	}
	rt.InitializeSQLite = func(ctx context.Context, databasePath, operatorEmail string) error {
		sqliteCalls = append(sqliteCalls, "service:"+operatorEmail)
		return initializeSQLite(ctx, databasePath, operatorEmail)
	}
	oldUID := effectiveUID
	effectiveUID = func() int { return 0 }
	t.Cleanup(func() { effectiveUID = oldUID })
	if err := runInit(initArgs(root), os.Stdout, rt); err != nil {
		t.Fatal(err)
	}
	if *installs != 1 {
		t.Fatal(*installs)
	}
	if got, want := strings.Join(sqliteCalls, ","), "repair:"+filepath.Join(root, "data", "tinkercloud.db")+",service:,service:operator@example.test"; got != want {
		t.Fatalf("SQLite ownership/write order = %q, want %q", got, want)
	}
	stateBytes, err := os.ReadFile(filepath.Join(root, "etc", "init-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	state, err := operations.ParseInitState(stateBytes)
	if err != nil || state.Next() != "" {
		t.Fatal(string(stateBytes), state, err)
	}
	credential, err := os.Stat(filepath.Join(root, "etc", "credentials", "tinkercloud.env"))
	if err != nil || credential.Mode().Perm() != 0600 {
		t.Fatal(credential, err)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "etc", "config.yaml")); string(b) == "" || string(b) == "resend-secret" {
		t.Fatal("config wrote secret")
	}
	if b, _ := os.ReadFile(filepath.Join(root, "etc", "config.yaml")); !strings.Contains(string(b), "acme:\n  email: operator@example.test\n") {
		t.Fatalf("ACME contact was not derived from operator email: %s", b)
	}
}

func TestInitRejectsLegacyACMEEmailFlagBeforeHostMutation(t *testing.T) {
	root := t.TempDir()
	writeInitSecrets(t, root)
	rt, _ := fakeInitRuntime(t)
	preflightRan := false
	rt.ReadFile = func(string) ([]byte, error) {
		preflightRan = true
		return nil, errors.New("must not be called")
	}
	oldUID := effectiveUID
	effectiveUID = func() int { return 0 }
	t.Cleanup(func() { effectiveUID = oldUID })
	args := append(initArgs(root), "--acme-email", "different@example.test")
	if err := runInit(args, os.Stdout, rt); err == nil || err.Error() != "tinkercloud: invalid_arguments" {
		t.Fatalf("legacy ACME flag result = %v", err)
	}
	if preflightRan {
		t.Fatal("legacy ACME flag reached host preflight")
	}
}

func TestInitStoresNormalizedOperatorEmailAsACMEContact(t *testing.T) {
	root := t.TempDir()
	writeInitSecrets(t, root)
	rt, _ := fakeInitRuntime(t)
	oldUID := effectiveUID
	effectiveUID = func() int { return 0 }
	t.Cleanup(func() { effectiveUID = oldUID })
	args := initArgs(root)
	for i := range args {
		if args[i] == "operator@example.test" {
			args[i] = " operator@EXAMPLE.TEST "
			break
		}
	}
	if err := runInit(args, os.Stdout, rt); err != nil {
		t.Fatal(err)
	}
	config, err := os.ReadFile(filepath.Join(root, "etc", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(config), "acme:\n  email: operator@example.test\n") {
		t.Fatalf("ACME contact = %s, want normalized operator email", config)
	}
}

func TestInitInterruptedServiceIsRetryable(t *testing.T) {
	root := t.TempDir()
	writeInitSecrets(t, root)
	rt, installs := fakeInitRuntime(t)
	first := true
	rt.Install = func(string, string) error {
		*installs++
		if first {
			first = false
			return errors.New("injected")
		}
		return nil
	}
	oldUID := effectiveUID
	effectiveUID = func() int { return 0 }
	t.Cleanup(func() { effectiveUID = oldUID })
	if err := runInit(initArgs(root), os.Stdout, rt); err == nil {
		t.Fatal("service failure accepted")
	}
	stateBytes, _ := os.ReadFile(filepath.Join(root, "etc", "init-state.json"))
	state, err := operations.ParseInitState(stateBytes)
	if err != nil || state.Next() != operations.InitService {
		t.Fatal(string(stateBytes), state, err)
	}
	if err := runInit(initArgs(root), os.Stdout, rt); err != nil {
		t.Fatal(err)
	}
	if *installs != 2 {
		t.Fatal(*installs)
	}
}

func TestInitDNSPreflightDenyCharter(t *testing.T) {
	for name, lookup := range map[string]func(context.Context, string) ([]string, error){
		"platform missing": func(_ context.Context, host string) ([]string, error) {
			if host == "admin.apps.tinker.example.test" {
				return nil, errors.New("not found")
			}
			return []string{"192.0.2.1"}, nil
		},
		"wildcard empty": func(_ context.Context, host string) ([]string, error) {
			if host == "tinkercloud-init.apps.tinker.example.test" {
				return nil, nil
			}
			return []string{"2001:db8::1"}, nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeInitSecrets(t, root)
			rt, installs := fakeInitRuntime(t)
			rt.DNSLookup = lookup
			oldUID := effectiveUID
			effectiveUID = func() int { return 0 }
			t.Cleanup(func() { effectiveUID = oldUID })

			err := runInit(initArgs(root), os.Stdout, rt)
			if err == nil || !strings.Contains(err.Error(), "dns_") || !strings.Contains(err.Error(), "configure") || strings.Contains(err.Error(), "192.0.2.1") || strings.Contains(err.Error(), "2001:db8") {
				t.Fatalf("DNS failure result = %v", err)
			}
			if *installs != 0 {
				t.Fatalf("service installed before DNS preflight: %d", *installs)
			}
			stateBytes, readErr := os.ReadFile(filepath.Join(root, "etc", "init-state.json"))
			if readErr != nil {
				t.Fatal(readErr)
			}
			state, parseErr := operations.ParseInitState(stateBytes)
			if parseErr != nil || state.Next() != operations.InitDNS {
				t.Fatalf("init state = %q, %v, %v", stateBytes, state, parseErr)
			}
		})
	}
}

func TestRunDNSPreflightAcceptsResolvedNamesWithoutAddressComparison(t *testing.T) {
	var hosts []string
	cfg := config.Config{Domain: "example.test"}
	if err := runDNSPreflight(context.Background(), cfg, func(_ context.Context, host string) ([]string, error) {
		hosts = append(hosts, host)
		return []string{"2001:db8::1"}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(hosts, ","), "admin.example.test,tinkercloud-init.example.test"; got != want {
		t.Fatalf("DNS hosts = %q, want %q", got, want)
	}
}

func TestInitPublicHealthFailureLeavesVerificationRetryable(t *testing.T) {
	root := t.TempDir()
	writeInitSecrets(t, root)
	rt, _ := fakeInitRuntime(t)
	rt.PublicHealth = func(context.Context, string) error { return errors.New("gateway proof rejected") }
	oldUID := effectiveUID
	effectiveUID = func() int { return 0 }
	t.Cleanup(func() { effectiveUID = oldUID })
	if err := runInit(initArgs(root), os.Stdout, rt); err == nil {
		t.Fatal("public health failure accepted")
	}
	stateBytes, err := os.ReadFile(filepath.Join(root, "etc", "init-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	state, err := operations.ParseInitState(stateBytes)
	if err != nil || state.Next() != operations.InitVerified {
		t.Fatal(string(stateBytes), state, err)
	}
}

func TestInitDeniesUnsupportedAndUnsafeSecretFiles(t *testing.T) {
	root := t.TempDir()
	writeInitSecrets(t, root)
	rt, _ := fakeInitRuntime(t)
	rt.GOARCH = func() string { return "arm64" }
	oldUID := effectiveUID
	effectiveUID = func() int { return 0 }
	t.Cleanup(func() { effectiveUID = oldUID })
	if err := runInit(initArgs(root), os.Stdout, rt); err == nil {
		t.Fatal("unsupported host accepted")
	}
	rt, _ = fakeInitRuntime(t)
	if err := os.Chmod(filepath.Join(root, "resend"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := runInit(initArgs(root), os.Stdout, rt); err == nil {
		t.Fatal("world-readable secret accepted")
	}
}

type healthRoundTripper func(*http.Request) (*http.Response, error)

func (f healthRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func gatewayVersionResponse(r *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header: http.Header{
			"Content-Type":           []string{"application/json; charset=utf-8"},
			"Cache-Control":          []string{"no-store"},
			"X-Content-Type-Options": []string{"nosniff"},
		},
		Request: r,
		Body:    io.NopCloser(strings.NewReader(body)),
	}
}

func TestPublicGatewayHealthAcceptsOnlyExactGatewayVersionOverTrustedTLS(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "example.com" || r.Method != http.MethodGet || r.URL.Path != "/api/v1/version" || r.URL.RawQuery != "" {
			t.Fatalf("unexpected request: %s %s host=%s", r.Method, r.URL.String(), r.Host)
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_, _ = w.Write([]byte(`{"api_version":1}`))
	}))
	defer server.Close()
	transport := server.Client().Transport.(*http.Transport).Clone()
	address := server.Listener.Addr().String()
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	if err := publicGatewayHealth(context.Background(), "example.com", client); err != nil {
		t.Fatal(err)
	}
}

func TestPublicGatewayHealthRejectsNonGatewayEvidence(t *testing.T) {
	valid := `{"api_version":1}`
	for name, roundTrip := range map[string]healthRoundTripper{
		"not found": func(r *http.Request) (*http.Response, error) {
			return gatewayVersionResponse(r, http.StatusNotFound, valid), nil
		},
		"unauthorized": func(r *http.Request) (*http.Response, error) {
			return gatewayVersionResponse(r, http.StatusUnauthorized, valid), nil
		},
		"server error": func(r *http.Request) (*http.Response, error) {
			return gatewayVersionResponse(r, http.StatusBadGateway, valid), nil
		},
		"html success": func(r *http.Request) (*http.Response, error) {
			resp := gatewayVersionResponse(r, http.StatusOK, `<html>gateway</html>`)
			resp.Header.Set("Content-Type", "text/html")
			return resp, nil
		},
		"missing no-store": func(r *http.Request) (*http.Response, error) {
			resp := gatewayVersionResponse(r, http.StatusOK, valid)
			resp.Header.Del("Cache-Control")
			return resp, nil
		},
		"wrong api version": func(r *http.Request) (*http.Response, error) {
			return gatewayVersionResponse(r, http.StatusOK, `{"api_version":2}`), nil
		},
		"extra protocol field": func(r *http.Request) (*http.Response, error) {
			return gatewayVersionResponse(r, http.StatusOK, `{"api_version":1,"extra":true}`), nil
		},
		"duplicate protocol field": func(r *http.Request) (*http.Response, error) {
			return gatewayVersionResponse(r, http.StatusOK, `{"api_version":1,"api_version":1}`), nil
		},
		"malformed json": func(r *http.Request) (*http.Response, error) {
			return gatewayVersionResponse(r, http.StatusOK, `{"api_version":`), nil
		},
		"oversized json": func(r *http.Request) (*http.Response, error) {
			return gatewayVersionResponse(r, http.StatusOK, `{"api_version":1}`+strings.Repeat(" ", maxPublicGatewayProof)), nil
		},
		"wrong final host": func(r *http.Request) (*http.Response, error) {
			resp := gatewayVersionResponse(r, http.StatusOK, valid)
			clone := r.Clone(r.Context())
			clone.URL.Host = "other.example.com"
			resp.Request = clone
			return resp, nil
		},
		"redirect": func(r *http.Request) (*http.Response, error) {
			resp := gatewayVersionResponse(r, http.StatusMovedPermanently, valid)
			resp.Header.Set("Location", "https://example.com/api/v1/version")
			return resp, nil
		},
		"transport failure": func(*http.Request) (*http.Response, error) { return nil, errors.New("dial failed") },
	} {
		t.Run(name, func(t *testing.T) {
			client := &http.Client{Transport: roundTrip, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
			if err := publicGatewayHealth(context.Background(), "example.com", client); err == nil {
				t.Fatal("invalid public gateway evidence accepted")
			}
		})
	}
}

func TestPublicGatewayHealthDoesNotFollowSameHostRedirect(t *testing.T) {
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			http.Redirect(w, r, "/api/v1/version?redirected=1", http.StatusFound)
			return
		}
		t.Fatal("health check followed a redirect")
	}))
	defer server.Close()
	transport := server.Client().Transport.(*http.Transport).Clone()
	address := server.Listener.Addr().String()
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	if err := publicGatewayHealth(context.Background(), "example.com", client); err == nil {
		t.Fatal("redirect accepted")
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
}

func TestPublicGatewayHealthRejectsUntrustedTLS(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("untrusted TLS request reached handler")
	}))
	defer server.Close()
	address := server.Listener.Addr().String()
	transport := &http.Transport{DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}}
	defer transport.CloseIdleConnections()
	if err := publicGatewayHealth(context.Background(), "example.com", &http.Client{Transport: transport}); err == nil {
		t.Fatal("untrusted TLS certificate accepted")
	}
}
