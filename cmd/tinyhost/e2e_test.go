package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"github.com/tinyhost/tiny/internal/client"
	"github.com/tinyhost/tiny/internal/config"
	"github.com/tinyhost/tiny/internal/deployments"
	"github.com/tinyhost/tiny/internal/persistence"
	"github.com/tinyhost/tiny/internal/update"
	"github.com/tinyhost/tiny/internal/verification"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestBuildHandlerComposesCLILogin proves the actual production composition,
// rather than a standalone Dispatcher fixture, retains the CLI OTP issuer.
// A generic login_* transaction is deliberately the dispatcher fail-closed
// fallback for an unavailable issuer, so accepting it here would mask a
// deployer-facing login outage after a seemingly healthy server startup.
func TestBuildHandlerComposesCLILogin(t *testing.T) {
	root := t.TempDir()
	store, err := persistence.OpenSQLite(context.Background(), filepath.Join(root, "tiny.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	cfg := config.Config{
		Domain:        "tiny.test",
		SessionCookie: "__Host-tiny_app",
		DataDirectory: root,
		OTPExpiry:     time.Minute,
		SessionExpiry: time.Hour,
		Limits:        config.ResourceLimits{DiskWarningPercent: 80, DiskStopPercent: 99},
	}
	h, _, err := buildHandler(cfg, config.Secrets{HMACKey: "test-composed-cli-login-hmac-key"}, store, denyDeploymentGates{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	// The production service keeps its SQLite handle open before a root-only
	// recovery command may enable a deployer through a distinct process. Keep
	// the handler's original store open and use an independently opened store
	// for the mutation to catch stale WAL snapshots or write-lock mistakes in
	// that real ordering.
	recoveryStore, err := persistence.OpenSQLite(context.Background(), filepath.Join(root, "tiny.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := recoveryStore.SetDeployerStatus(context.Background(), "deployer@example.com", "active", "seed"); err != nil {
		recoveryStore.Close()
		t.Fatal(err)
	}
	if err := recoveryStore.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "https://"+cfg.PlatformHost()+"/api/v1/auth/otp", strings.NewReader(`{"email":"deployer@example.com"}`))
	req.Host = cfg.PlatformHost()
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	h.ServeHTTP(response, req)
	if response.Code != http.StatusAccepted {
		t.Fatalf("OTP request status=%d body=%s", response.Code, response.Body.String())
	}
	var result struct {
		Transaction string `json:"transaction"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(result.Transaction, "otp_") {
		t.Fatalf("composed handler issued fallback transaction %q", result.Transaction)
	}
	var count int
	if err := store.DB.QueryRow(`SELECT COUNT(*) FROM otp_challenges WHERE id=? AND purpose='control' AND control_channel='cli'`, result.Transaction).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("CLI challenge rows=%d", count)
	}
}

func seedDeployer(t *testing.T, s *persistence.SQLiteStore, email string) string {
	t.Helper()
	if e := s.SetDeployerStatus(context.Background(), email, "active", "seed"); e != nil {
		t.Fatal(e)
	}
	var user string
	if e := s.DB.QueryRow("SELECT id FROM users WHERE normalized_email=?", email).Scan(&user); e != nil {
		t.Fatal(e)
	}
	token, e := s.IssueToken(context.Background(), user, "", []string{"app:read", "app:create", "deploy:create", "deploy:activate", "access:read", "access:write"}, time.Now().Add(time.Hour))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.AuthenticateToken(context.Background(), token, "deploy:create", "", time.Now()); e != nil {
		t.Fatal(e)
	}
	return token
}

func TestPlatformVersionTLSHostHarness(t *testing.T) {
	root := t.TempDir()
	defer filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err == nil {
			if info.IsDir() {
				_ = os.Chmod(p, 0755)
			} else {
				_ = os.Chmod(p, 0644)
			}
		}
		return nil
	})
	s, e := persistence.OpenSQLite(context.Background(), root+"/tiny.db")
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	// The host running this integration test may itself be beyond the production
	// 90% stop watermark. Exercise the app/control path independently of host
	// disk pressure; watermark behavior is covered through the injected gate.
	cfg := config.Config{Domain: "apps.tiny.test", SessionCookie: "__Host-tiny_app", DataDirectory: root, OTPExpiry: time.Minute, SessionExpiry: time.Hour, Limits: config.ResourceLimits{DiskWarningPercent: 80, DiskStopPercent: 99}}
	gates := deployments.GateFuncs{PolicyFunc: func(ctx context.Context, r deployments.Record) bool {
		return s.CandidatePolicyReady(ctx, r)
	}, CertificateFunc: func(context.Context, deployments.Record) bool { return true }, ProbeFunc: func(ctx context.Context, r deployments.Record) bool {
		p, e := verification.ProbeCandidate(ctx, cfg, root, r)
		return e == nil && p.Passed()
	}}
	h, _, err := buildHandler(cfg, config.Secrets{}, s, gates, nil)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewTLSServer(h)
	defer ts.Close()
	addr := ts.Listener.Addr().String()
	cl := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}}}
	r, e := cl.Get("https://" + cfg.PlatformHost() + "/api/v1/version")
	if e != nil {
		t.Fatal(e)
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		t.Fatal(r.Status)
	}
	token := seedDeployer(t, s, "deployer@example.com")
	api := client.Client{Base: "https://" + cfg.PlatformHost(), Token: token, HTTP: cl}
	// A fresh deployer has no app-management prerequisite: the deploy client
	// creates only a missing app in its own server-scoped list before upload.
	if e := api.EnsureApp(context.Background(), "demo"); e != nil {
		t.Fatal(e)
	}
	var owner, status string
	var rev int
	if e := s.DB.QueryRow("SELECT owner_user_id,status,policy_revision FROM applications WHERE slug='demo'").Scan(&owner, &status, &rev); e != nil || status != "active" || rev != 1 {
		t.Fatal(owner, status, rev, e)
	}
	var n int
	if e := s.DB.QueryRow("SELECT COUNT(*) FROM audit_events WHERE action='app.created'").Scan(&n); e != nil || n != 1 {
		t.Fatal(n, e)
	}
	project := t.TempDir()
	manifest := []byte("version: 1\nname: demo\nbuild:\n  output: .\n")
	os.WriteFile(filepath.Join(project, "tiny.yaml"), manifest, 0600)
	os.WriteFile(filepath.Join(project, "index.html"), []byte("hello"), 0600)
	var archive bytes.Buffer
	if e := client.ArchiveProject(project, manifest, &archive); e != nil {
		t.Fatal(e)
	}
	result, e := api.Deploy(context.Background(), "demo", bytes.NewReader(archive.Bytes()), int64(archive.Len()), "upload-demo")
	if e != nil || result.Verified() != nil || result.URL != "https://demo.apps.tiny.test/" {
		t.Fatal(result, e)
	}
	// The updater calls this through the same composed TLS gateway after a
	// restart. A missing route or public static response must not count as
	// update evidence.
	if e := (update.AnonymousDenyHealth{HTTPHealth: update.HTTPHealth{URL: result.URL + "_tiny/api/v1/capabilities", Client: cl}}).Check(context.Background()); e != nil {
		t.Fatalf("composed anonymous denial evidence: %v", e)
	}
}
