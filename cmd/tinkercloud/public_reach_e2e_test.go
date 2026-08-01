package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ChrisMarxDev/tinkercloud/internal/client"
	"github.com/ChrisMarxDev/tinkercloud/internal/config"
	"github.com/ChrisMarxDev/tinkercloud/internal/controlapi"
	"github.com/ChrisMarxDev/tinkercloud/internal/deployments"
	"github.com/ChrisMarxDev/tinkercloud/internal/identity"
	"github.com/ChrisMarxDev/tinkercloud/internal/persistence"
	"github.com/ChrisMarxDev/tinkercloud/internal/verification"
)

func TestPublicReachProductionCompositionLocalAcceptance(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	root := t.TempDir()
	databasePath := filepath.Join(root, "tinkercloud.db")
	store, err := persistence.OpenSQLite(ctx, databasePath)
	if err != nil {
		t.Fatal(err)
	}
	closed := false
	t.Cleanup(func() {
		cancel()
		if !closed {
			_ = store.Close()
		}
		_ = filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr == nil {
				if info.IsDir() {
					_ = os.Chmod(path, 0755)
				} else {
					_ = os.Chmod(path, 0644)
				}
			}
			return nil
		})
	})
	cfg := config.Config{
		Domain:        "apps.acceptance.test",
		SessionCookie: "__Host-tinker_app",
		DataDirectory: root,
		OTPExpiry:     time.Minute,
		SessionExpiry: time.Hour,
		Limits:        config.ResourceLimits{DiskWarningPercent: 80, DiskStopPercent: 99},
	}
	gates := deployments.GateFuncs{
		PolicyFunc: func(ctx context.Context, record deployments.Record) bool {
			return store.CandidatePolicyReady(ctx, record)
		},
		CertificateFunc: func(context.Context, deployments.Record) bool { return true },
		ProbeFunc: func(ctx context.Context, record deployments.Record) bool {
			probe, probeErr := verification.ProbeCandidate(ctx, cfg, root, record)
			return probeErr == nil && probe.Passed()
		},
	}
	handler, _, insights, err := buildHandler(cfg, config.Secrets{HMACKey: "local-public-acceptance-hmac-key"}, store, gates, nil)
	if err != nil {
		t.Fatal(err)
	}
	insights.Start(ctx)
	server := httptest.NewTLSServer(handler)
	serverClosed := false
	t.Cleanup(func() {
		if !serverClosed {
			server.Close()
		}
	})
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // local httptest certificate only
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
		},
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	httpClient := &http.Client{Transport: transport, Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	gate, err := store.CurrentPublicGate(ctx)
	if err != nil || gate.Enabled || gate.Revision != 1 {
		t.Fatalf("default gate=%#v err=%v", gate, err)
	}
	ownerToken := seedDeployer(t, store, "owner@example.com")
	ownerAPI := client.Client{Base: "https://" + cfg.PlatformHost(), Token: ownerToken, HTTP: httpClient}
	if err := ownerAPI.EnsureApp(ctx, "public-story"); err != nil {
		t.Fatal(err)
	}
	private := localDeploy(t, ctx, ownerAPI, "public-story", "private-initial", false, false, false)
	if private.err != nil || private.result.Posture != client.PosturePrivate {
		t.Fatalf("private deployment=%#v err=%v", private.result, private.err)
	}
	var publicAppID, ownerID string
	if err := store.DB.QueryRow("SELECT a.id,a.owner_user_id FROM applications a WHERE a.slug='public-story'").Scan(&publicAppID, &ownerID); err != nil {
		t.Fatal(err)
	}
	ownerSession, _, err := store.CreateAppSession(ctx, publicAppID, identity.Identity{ID: "owner@example.com", Email: "owner@example.com"}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	var privateDeployment string
	if err := store.DB.QueryRow("SELECT current_deployment_id FROM applications WHERE id=?", publicAppID).Scan(&privateDeployment); err != nil {
		t.Fatal(err)
	}

	ownerAPI.PublicAcknowledged = true
	failedPublic := localDeploy(t, ctx, ownerAPI, "public-story", "public-gate-off", true, true, true)
	if failedPublic.err == nil {
		t.Fatal("gate-off public candidate activated")
	}
	assertCurrentDeployment(t, store, publicAppID, privateDeployment, "private")
	if response := localRequest(t, httpClient, http.MethodGet, "public-story."+cfg.AppSuffix(), "/asset.txt", nil, false); response.status != http.StatusUnauthorized || strings.Contains(response.body, "PUBLIC-ASSET") {
		t.Fatalf("gate-off asset status=%d body=%q", response.status, response.body)
	}

	if err := store.SetPublicGate(ctx, "root", true, 1, "acceptance-enable"); err != nil {
		t.Fatal(err)
	}
	ownerAPI.PublicAcknowledged = false
	unacknowledged := localDeploy(t, ctx, ownerAPI, "public-story", "public-unacknowledged", true, false, false)
	if unacknowledged.err == nil {
		t.Fatal("unacknowledged private-to-public candidate activated")
	}
	assertCurrentDeployment(t, store, publicAppID, privateDeployment, "private")
	ownerAPI.PublicAcknowledged = true
	publicNoIndex := localDeploy(t, ctx, ownerAPI, "public-story", "public-noindex", true, false, false)
	if publicNoIndex.err != nil || publicNoIndex.result.Posture != client.PosturePublicStatic || publicNoIndex.result.PublicStatic == nil || publicNoIndex.result.PublicStatic.Indexing {
		t.Fatalf("non-indexed public deployment=%#v err=%v", publicNoIndex.result, publicNoIndex.err)
	}
	publicHost := "public-story." + cfg.AppSuffix()
	noIndexDocument := localRequest(t, httpClient, http.MethodGet, publicHost, "/", nil, false)
	if noIndexDocument.status != http.StatusOK || noIndexDocument.body != "<h1>PUBLIC-DOCUMENT</h1>" || noIndexDocument.header.Get("X-Robots-Tag") != "noindex, nofollow" {
		t.Fatalf("non-indexed public document status=%d robots=%q body=%q", noIndexDocument.status, noIndexDocument.header.Get("X-Robots-Tag"), noIndexDocument.body)
	}
	ownerAPI.PublicAcknowledged = false
	public := localDeploy(t, ctx, ownerAPI, "public-story", "public-live", true, true, true)
	if public.err != nil || public.result.Posture != client.PosturePublicStatic || public.result.PublicStatic == nil || !public.result.PublicStatic.Indexing {
		t.Fatalf("public deployment=%#v err=%v", public.result, public.err)
	}
	document := localRequest(t, httpClient, http.MethodGet, publicHost, "/", nil, true)
	if document.status != http.StatusOK || document.body != "<h1>PUBLIC-DOCUMENT</h1>" || document.header.Get("X-Robots-Tag") != "" {
		t.Fatalf("public document status=%d robots=%q body=%q", document.status, document.header.Get("X-Robots-Tag"), document.body)
	}
	asset := localRequest(t, httpClient, http.MethodGet, publicHost, "/asset.txt", nil, false)
	if asset.status != http.StatusOK || asset.body != "PUBLIC-ASSET" {
		t.Fatalf("public asset status=%d body=%q", asset.status, asset.body)
	}
	spa := localRequest(t, httpClient, http.MethodGet, publicHost, "/nested/route", nil, true)
	if spa.status != http.StatusOK || spa.body != "<h1>PUBLIC-DOCUMENT</h1>" {
		t.Fatalf("public SPA status=%d body=%q", spa.status, spa.body)
	}
	waitInsightSummary(t, store, publicAppID, 2, 1)

	reserved := []struct{ method, path string }{
		{http.MethodGet, "/_tinker/auth/login"},
		{http.MethodPost, "/_tinker/auth/logout"},
		{http.MethodGet, "/_tinker/auth/callback"},
		{http.MethodGet, "/_tinker/api/v1/me"},
		{http.MethodGet, "/_tinker/api/v1/app"},
		{http.MethodGet, "/_tinker/api/v1/capabilities"},
		{http.MethodGet, "/_tinker/api/v1/kv/key"},
		{http.MethodPost, "/_tinker/api/v1/db/items"},
		{http.MethodGet, "/_tinker/api/v1/blobs"},
		{http.MethodPost, "/_tinker/api/v1/llm/chat"},
		{http.MethodGet, "/_tinker/ws/v1"},
		{http.MethodPost, "/_tinker/auth/otp"},
	}
	for _, route := range reserved {
		response := localRequest(t, httpClient, route.method, publicHost, route.path, nil, false)
		if response.status != http.StatusNotFound || !strings.Contains(response.body, `"not_found"`) || strings.Contains(response.body, "PUBLIC-") || response.header.Get("Location") != "" || response.header.Get("Set-Cookie") != "" || response.header.Get("Cache-Control") != "no-store" || response.header.Get("X-Content-Type-Options") != "nosniff" || !strings.HasPrefix(response.header.Get("Content-Type"), "application/json") {
			t.Fatalf("reserved %s %s status=%d location=%q cookie=%q body=%q", route.method, route.path, response.status, response.header.Get("Location"), response.header.Get("Set-Cookie"), response.body)
		}
	}
	for _, host := range []string{"unknown." + cfg.AppSuffix(), "public-story." + cfg.AppSuffix() + ".invalid"} {
		response := localRequest(t, httpClient, http.MethodGet, host, "/asset.txt", nil, false)
		if response.status != http.StatusNotFound || strings.Contains(response.body, "PUBLIC-ASSET") {
			t.Fatalf("wrong host %q status=%d body=%q", host, response.status, response.body)
		}
	}

	secondToken := seedDeployer(t, store, "second@example.com")
	secondAPI := client.Client{Base: "https://" + cfg.PlatformHost(), Token: secondToken, HTTP: httpClient}
	if err := secondAPI.EnsureApp(ctx, "private-board"); err != nil {
		t.Fatal(err)
	}
	second := localDeployWithManifest(t, ctx, secondAPI, "private-board", "second-private", "version: 2\nname: private-board\ndescription: Private board\ntags: [team]\nbuild:\n  output: .\naccess:\n  mode: private\n  allow:\n    emails: [viewer@example.test]\n", map[string]string{"index.html": "<h1>SECOND-PRIVATE</h1>", "asset.txt": "SECOND-ASSET"})
	if second.err != nil {
		t.Fatal(second.err)
	}
	var secondAppID, secondOwnerID string
	if err := store.DB.QueryRow("SELECT id,owner_user_id FROM applications WHERE slug='private-board'").Scan(&secondAppID, &secondOwnerID); err != nil {
		t.Fatal(err)
	}
	cross := &http.Cookie{Name: cfg.SessionCookie, Value: ownerSession}
	crossApp := localRequest(t, httpClient, http.MethodGet, "private-board."+cfg.AppSuffix(), "/asset.txt", cross, false)
	if crossApp.status != http.StatusUnauthorized || strings.Contains(crossApp.body, "SECOND-ASSET") {
		t.Fatalf("cross-app session status=%d body=%q", crossApp.status, crossApp.body)
	}

	service := persistence.ControlService{Store: store, AppSuffix: cfg.AppSuffix()}
	secondSession, _, err := store.CreateAppSession(ctx, secondAppID, identity.Identity{ID: "second@example.com", Email: "second@example.com"}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	secondCookie := &http.Cookie{Name: cfg.SessionCookie, Value: secondSession}
	secondPrivate := localRequest(t, httpClient, http.MethodGet, "private-board."+cfg.AppSuffix(), "/asset.txt", secondCookie, false)
	if secondPrivate.status != http.StatusOK || secondPrivate.body != "SECOND-ASSET" {
		t.Fatalf("second private session status=%d body=%q", secondPrivate.status, secondPrivate.body)
	}
	secondActor := controlapi.Actor{ID: secondOwnerID, Role: "deployer", Active: true}
	if err := service.SetAppStatus(ctx, secondActor, "private-board", "suspended", "acceptance-suspend"); err != nil {
		t.Fatal(err)
	}
	revokedNextRequest := localRequest(t, httpClient, http.MethodGet, "private-board."+cfg.AppSuffix(), "/asset.txt", secondCookie, false)
	if revokedNextRequest.status == http.StatusOK || strings.Contains(revokedNextRequest.body, "SECOND-ASSET") {
		t.Fatalf("suspended app/session status=%d body=%q", revokedNextRequest.status, revokedNextRequest.body)
	}
	if err := service.SetAppStatus(ctx, secondActor, "private-board", "active", "acceptance-resume"); err != nil {
		t.Fatal(err)
	}
	catalog, err := service.Catalog(ctx, controlapi.ViewerIdentity{IdentityID: "viewer", Email: "viewer@example.test"})
	if err != nil || len(catalog) != 2 || catalog[0].Slug != "private-board" || catalog[0].Posture != "private" || catalog[1].Slug != "public-story" || catalog[1].Posture != "public" {
		t.Fatalf("catalog=%#v err=%v", catalog, err)
	}
	ownerDashboard, err := service.Dashboard(ctx, controlapi.Actor{ID: ownerID, Role: "deployer", Active: true})
	if err != nil || len(ownerDashboard.Apps) != 1 || ownerDashboard.Apps[0].Slug != "public-story" || !ownerDashboard.Apps[0].Insights.Available || ownerDashboard.Apps[0].Insights.Last7Days.PageViews != 2 || ownerDashboard.Apps[0].Insights.Last7Days.ApproximateVisitors != 1 {
		t.Fatalf("owner dashboard=%#v err=%v", ownerDashboard.Apps, err)
	}
	secondDashboard, err := service.Dashboard(ctx, controlapi.Actor{ID: secondOwnerID, Role: "deployer", Active: true})
	if err != nil || len(secondDashboard.Apps) != 1 || secondDashboard.Apps[0].Slug != "private-board" {
		t.Fatalf("second dashboard=%#v err=%v", secondDashboard.Apps, err)
	}
	if secondDashboard.Apps[0].Insights.Last7Days.PageViews != 0 || secondDashboard.Apps[0].Insights.Last7Days.ApproximateVisitors != 0 {
		t.Fatalf("second owner received public app insights: %#v", secondDashboard.Apps[0].Insights)
	}
	if _, err := store.DB.Exec("INSERT INTO users(id,normalized_email,role,status,created_at) VALUES('acceptance-op','operator@example.test','operator','active',datetime('now'))"); err != nil {
		t.Fatal(err)
	}
	operatorDashboard, err := service.Dashboard(ctx, controlapi.Actor{ID: "acceptance-op", Role: "operator", Active: true})
	if err != nil || len(operatorDashboard.Apps) != 2 || !operatorDashboard.PublicGate.Enabled {
		t.Fatalf("operator dashboard apps=%d gate=%#v err=%v", len(operatorDashboard.Apps), operatorDashboard.PublicGate, err)
	}
	operatorSawPublicInsights := false
	for _, app := range operatorDashboard.Apps {
		if app.Slug == "public-story" && app.Insights.Available && app.Insights.Last7Days.PageViews == 2 && app.Insights.Last7Days.ApproximateVisitors == 1 {
			operatorSawPublicInsights = true
		}
	}
	if !operatorSawPublicInsights {
		t.Fatalf("operator dashboard missing public insights: %#v", operatorDashboard.Apps)
	}
	capabilityBearing := localDeployRawManifest(t, ctx, ownerAPI, "public-story", "public-capability", "version: 2\nname: public-story\nbuild:\n  output: .\naccess:\n  mode: public\nfeatures:\n  kv: true\n", map[string]string{"index.html": "<h1>CAPABILITY-PUBLIC</h1>"})
	if capabilityBearing.err == nil {
		t.Fatal("capability-bearing public candidate activated")
	}
	assertCurrentDeployment(t, store, publicAppID, public.result.DeploymentID, "public")

	if _, err := store.DB.Exec("CREATE TRIGGER fail_acceptance_insight BEFORE INSERT ON app_insight_days BEGIN SELECT RAISE(ABORT,'insights unavailable'); END"); err != nil {
		t.Fatal(err)
	}
	failureDocument := localRequest(t, httpClient, http.MethodGet, publicHost, "/", nil, true)
	if failureDocument.status != http.StatusOK || failureDocument.body != "<h1>PUBLIC-DOCUMENT</h1>" {
		t.Fatalf("analytics failure changed response status=%d body=%q", failureDocument.status, failureDocument.body)
	}
	time.Sleep(100 * time.Millisecond)
	failedSummary, err := store.InsightSummary(ctx, publicAppID, 7, time.Now())
	if err != nil || failedSummary.PageViews != 2 || failedSummary.ApproximateVisitors != 1 {
		t.Fatalf("failed analytics write changed summary=%#v err=%v", failedSummary, err)
	}
	if _, err := store.DB.Exec("DROP TRIGGER fail_acceptance_insight"); err != nil {
		t.Fatal(err)
	}

	if err := store.SetPublicGate(ctx, "root", false, 2, "acceptance-disable"); err != nil {
		t.Fatal(err)
	}
	deniedAfterDisable := localRequest(t, httpClient, http.MethodGet, publicHost, "/asset.txt", nil, false)
	if deniedAfterDisable.status != http.StatusUnauthorized || strings.Contains(deniedAfterDisable.body, "PUBLIC-ASSET") {
		t.Fatalf("gate-disable denial status=%d body=%q", deniedAfterDisable.status, deniedAfterDisable.body)
	}
	gateOffCatalog, err := service.Catalog(ctx, controlapi.ViewerIdentity{IdentityID: "viewer", Email: "viewer@example.test"})
	if err != nil || len(gateOffCatalog) != 1 || gateOffCatalog[0].Slug != "private-board" || gateOffCatalog[0].Posture != "private" {
		t.Fatalf("gate-off catalog=%#v err=%v", gateOffCatalog, err)
	}
	privateOwner := localRequest(t, httpClient, http.MethodGet, publicHost, "/", &http.Cookie{Name: cfg.SessionCookie, Value: ownerSession}, true)
	if privateOwner.status != http.StatusOK || privateOwner.body != "<h1>PUBLIC-DOCUMENT</h1>" {
		t.Fatalf("owner fallback status=%d body=%q", privateOwner.status, privateOwner.body)
	}
	if err := store.SetPublicGate(ctx, "root", true, 3, "acceptance-reenable"); err != nil {
		t.Fatal(err)
	}

	if _, err := store.DB.Exec("PRAGMA ignore_check_constraints=ON; UPDATE public_static_settings SET enabled=2 WHERE singleton=1"); err != nil {
		t.Fatal(err)
	}
	malformedGate := localRequest(t, httpClient, http.MethodGet, publicHost, "/asset.txt", nil, false)
	if malformedGate.status != http.StatusUnauthorized || strings.Contains(malformedGate.body, "PUBLIC-ASSET") {
		t.Fatalf("malformed gate status=%d body=%q", malformedGate.status, malformedGate.body)
	}
	if _, err := store.DB.Exec("UPDATE public_static_settings SET enabled=1 WHERE singleton=1"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB.Exec("UPDATE access_policies SET mode='malformed' WHERE app_id=? AND revision=(SELECT policy_revision FROM applications WHERE id=?)", publicAppID, publicAppID); err != nil {
		t.Fatal(err)
	}
	malformedPolicy := localRequest(t, httpClient, http.MethodGet, publicHost, "/asset.txt", nil, false)
	if malformedPolicy.status != http.StatusUnauthorized || strings.Contains(malformedPolicy.body, "PUBLIC-ASSET") {
		t.Fatalf("malformed policy status=%d body=%q", malformedPolicy.status, malformedPolicy.body)
	}
	if _, err := store.DB.Exec("UPDATE access_policies SET mode='public' WHERE app_id=? AND revision=(SELECT policy_revision FROM applications WHERE id=?)", publicAppID, publicAppID); err != nil {
		t.Fatal(err)
	}

	ownerAPI.PublicAcknowledged = false
	privateFinal := localDeploy(t, ctx, ownerAPI, "public-story", "private-final", false, false, false)
	if privateFinal.err != nil || privateFinal.result.Posture != client.PosturePrivate {
		t.Fatalf("public-to-private result=%#v err=%v", privateFinal.result, privateFinal.err)
	}
	anonymousPrivate := localRequest(t, httpClient, http.MethodGet, publicHost, "/asset.txt", nil, false)
	if anonymousPrivate.status != http.StatusUnauthorized || strings.Contains(anonymousPrivate.body, "PRIVATE-FINAL-ASSET") {
		t.Fatalf("public-to-private denial status=%d body=%q", anonymousPrivate.status, anonymousPrivate.body)
	}
	ownerFinal := localRequest(t, httpClient, http.MethodGet, publicHost, "/", &http.Cookie{Name: cfg.SessionCookie, Value: ownerSession}, true)
	if ownerFinal.status != http.StatusOK || ownerFinal.body != "<h1>PRIVATE-FINAL</h1>" {
		t.Fatalf("private final owner status=%d body=%q", ownerFinal.status, ownerFinal.body)
	}

	server.Close()
	serverClosed = true
	cancel()
	for deadline := time.Now().Add(time.Second); insights.Available() && time.Now().Before(deadline); {
		time.Sleep(5 * time.Millisecond)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	closed = true
	reopened, err := persistence.OpenSQLite(context.Background(), databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	reopenedGate, err := reopened.CurrentPublicGate(context.Background())
	if err != nil || !reopenedGate.Enabled || reopenedGate.Revision != 4 {
		t.Fatalf("reopened gate=%#v err=%v", reopenedGate, err)
	}
	assertCurrentDeployment(t, reopened, publicAppID, privateFinal.result.DeploymentID, "private")

	restartCtx, cancelRestart := context.WithCancel(context.Background())
	restartGates := deployments.GateFuncs{
		PolicyFunc: func(ctx context.Context, record deployments.Record) bool {
			return reopened.CandidatePolicyReady(ctx, record)
		},
		CertificateFunc: func(context.Context, deployments.Record) bool { return true },
		ProbeFunc: func(ctx context.Context, record deployments.Record) bool {
			probe, probeErr := verification.ProbeCandidate(ctx, cfg, root, record)
			return probeErr == nil && probe.Passed()
		},
	}
	restartHandler, _, restartInsights, err := buildHandler(cfg, config.Secrets{HMACKey: "local-public-acceptance-hmac-key"}, reopened, restartGates, nil)
	if err != nil {
		cancelRestart()
		t.Fatal(err)
	}
	restartInsights.Start(restartCtx)
	restartServer := httptest.NewTLSServer(restartHandler)
	restartTransport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // local httptest certificate only
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, restartServer.Listener.Addr().String())
		},
	}
	restartClient := &http.Client{Transport: restartTransport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	persistedAnonymous := localRequest(t, restartClient, http.MethodGet, publicHost, "/asset.txt", nil, false)
	if persistedAnonymous.status != http.StatusUnauthorized || strings.Contains(persistedAnonymous.body, "PRIVATE-FINAL-ASSET") {
		t.Fatalf("restarted anonymous private status=%d body=%q", persistedAnonymous.status, persistedAnonymous.body)
	}
	persistedOwner := localRequest(t, restartClient, http.MethodGet, publicHost, "/", &http.Cookie{Name: cfg.SessionCookie, Value: ownerSession}, true)
	if persistedOwner.status != http.StatusOK || persistedOwner.body != "<h1>PRIVATE-FINAL</h1>" {
		t.Fatalf("restarted owner session status=%d body=%q", persistedOwner.status, persistedOwner.body)
	}
	restartServer.Close()
	restartTransport.CloseIdleConnections()
	cancelRestart()
}

type localDeployment struct {
	result client.DeploymentResult
	err    error
}

func localDeploy(t *testing.T, ctx context.Context, api client.Client, slug, key string, public, indexing, spa bool) localDeployment {
	t.Helper()
	mode := "private"
	document, asset := "<h1>PRIVATE-FINAL</h1>", "PRIVATE-FINAL-ASSET"
	if key == "private-initial" {
		document, asset = "<h1>PRIVATE-INITIAL</h1>", "PRIVATE-INITIAL-ASSET"
	}
	if public {
		mode, document, asset = "public", "<h1>PUBLIC-DOCUMENT</h1>", "PUBLIC-ASSET"
	}
	manifest := fmt.Sprintf("version: 2\nname: %s\ndescription: Acceptance fixture\ntags: [acceptance]\nbuild:\n  output: .\naccess:\n  mode: %s\n  indexing: %t\n", slug, mode, indexing)
	if spa {
		manifest += "spa:\n  fallback: index.html\n"
	}
	return localDeployWithManifest(t, ctx, api, slug, key, manifest, map[string]string{"index.html": document, "asset.txt": asset})
}

func localDeployWithManifest(t *testing.T, ctx context.Context, api client.Client, slug, key, manifest string, files map[string]string) localDeployment {
	t.Helper()
	project := t.TempDir()
	manifestBytes := []byte(manifest)
	if err := os.WriteFile(filepath.Join(project, "tinker.yaml"), manifestBytes, 0600); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(project, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var archive bytes.Buffer
	if err := client.ArchiveProject(project, manifestBytes, &archive); err != nil {
		return localDeployment{err: err}
	}
	result, err := api.Deploy(ctx, slug, bytes.NewReader(archive.Bytes()), int64(archive.Len()), key)
	return localDeployment{result: result, err: err}
}

// localDeployRawManifest intentionally bypasses the deployer-side manifest
// parser so the real server upload path proves the same hostile public intent
// is rejected without changing the active release.
func localDeployRawManifest(t *testing.T, ctx context.Context, api client.Client, slug, key, manifest string, files map[string]string) localDeployment {
	t.Helper()
	all := make(map[string]string, len(files)+1)
	all["tinker.yaml"] = manifest
	for name, body := range files {
		all[name] = body
	}
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	for name, body := range all {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(tw, body); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	result, err := api.Deploy(ctx, slug, bytes.NewReader(archive.Bytes()), int64(archive.Len()), key)
	return localDeployment{result: result, err: err}
}

type localResponse struct {
	status int
	header http.Header
	body   string
}

func localRequest(t *testing.T, client *http.Client, method, host, path string, cookie *http.Cookie, document bool) localResponse {
	t.Helper()
	request, err := http.NewRequest(method, "https://"+host+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cookie != nil {
		request.AddCookie(cookie)
	}
	if document {
		request.Header.Set("Accept", "text/html,application/xhtml+xml")
		request.Header.Set("Sec-Fetch-Dest", "document")
		request.Header.Set("Sec-Fetch-Mode", "navigate")
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		t.Fatal(err)
	}
	return localResponse{status: response.StatusCode, header: response.Header.Clone(), body: string(body)}
}

func assertCurrentDeployment(t *testing.T, store *persistence.SQLiteStore, appID, wantDeployment, wantMode string) {
	t.Helper()
	var deployment, mode string
	if err := store.DB.QueryRow("SELECT a.current_deployment_id,p.mode FROM applications a JOIN access_policies p ON p.app_id=a.id AND p.revision=a.policy_revision WHERE a.id=?", appID).Scan(&deployment, &mode); err != nil || deployment != wantDeployment || mode != wantMode {
		t.Fatalf("current deployment=%q mode=%q err=%v want=%q/%q", deployment, mode, err, wantDeployment, wantMode)
	}
}

func waitInsightSummary(t *testing.T, store *persistence.SQLiteStore, appID string, pageViews, visitors int64) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		summary, err := store.InsightSummary(context.Background(), appID, 7, time.Now())
		if err == nil && summary.PageViews == pageViews && summary.ApproximateVisitors == visitors {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	summary, err := store.InsightSummary(context.Background(), appID, 7, time.Now())
	t.Fatalf("insight summary=%#v err=%v want views=%d visitors=%d", summary, err, pageViews, visitors)
}
