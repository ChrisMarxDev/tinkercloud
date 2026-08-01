package verification

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ChrisMarxDev/tinkercloud/internal/appauth"
	"github.com/ChrisMarxDev/tinkercloud/internal/apps"
	"github.com/ChrisMarxDev/tinkercloud/internal/config"
	"github.com/ChrisMarxDev/tinkercloud/internal/deployments"
	"github.com/ChrisMarxDev/tinkercloud/internal/gateway"
	"github.com/ChrisMarxDev/tinkercloud/internal/identity"
	"github.com/ChrisMarxDev/tinkercloud/internal/policies"
	"github.com/ChrisMarxDev/tinkercloud/internal/releases"
	"github.com/ChrisMarxDev/tinkercloud/internal/sessions"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

var probeRequestID = regexp.MustCompile(`^req_[0-9a-f]{24}$`)

type CandidateProbeFailureStage string

const (
	CandidateProbeReleaseEvidence     CandidateProbeFailureStage = "release_evidence"
	CandidateProbeIndex               CandidateProbeFailureStage = "index"
	CandidateProbeSession             CandidateProbeFailureStage = "session"
	CandidateProbeAnonymousDenial     CandidateProbeFailureStage = "anonymous_denial"
	CandidateProbeAuthenticatedHealth CandidateProbeFailureStage = "authenticated_health"
	CandidateProbePublicStaticHealth  CandidateProbeFailureStage = "public_static_health"
	CandidateProbeUnknown             CandidateProbeFailureStage = "unknown"
)

type candidateProbeError struct{ stage CandidateProbeFailureStage }

func (e candidateProbeError) Error() string { return "candidate probe failed" }

// CandidateProbeStage returns only a fixed non-secret diagnostic category.
// Callers must not log the underlying probe error or candidate metadata.
func CandidateProbeStage(err error) CandidateProbeFailureStage {
	var probeErr candidateProbeError
	if errors.As(err, &probeErr) {
		return probeErr.stage
	}
	return CandidateProbeUnknown
}

func candidateProbeFailed(stage CandidateProbeFailureStage) error {
	return candidateProbeError{stage: stage}
}

type protectedProbeSpy struct{ calls int }

func (s *protectedProbeSpy) Dispatch(appauth.AuthorizationContext, gateway.Endpoint, http.ResponseWriter, *http.Request) {
	s.calls++
}

type preAuthProbeSpy struct{ calls int }

func (s *preAuthProbeSpy) DispatchPreAuth(apps.App, gateway.Endpoint, http.ResponseWriter, *http.Request) {
	s.calls++
}

type reservedProbeRoute struct{ method, path string }

// representativeReservedProbeRoutes is deliberately one route for every
// gateway registry classification. Keep it synchronized with ClassifyRoute.
var representativeReservedProbeRoutes = []reservedProbeRoute{
	{http.MethodGet, "/_tinker/auth/login"},
	{http.MethodGet, "/_tinker/auth/callback"},
	{http.MethodPost, "/_tinker/auth/logout"},
	{http.MethodGet, "/_tinker/api/v1/me"},
	{http.MethodGet, "/_tinker/api/v1/app"},
	{http.MethodGet, "/_tinker/api/v1/capabilities"},
	{http.MethodGet, "/_tinker/api/v1/kv"},
	{http.MethodGet, "/_tinker/api/v1/db"},
	{http.MethodGet, "/_tinker/api/v1/blobs"},
	{http.MethodGet, "/_tinker/ws/v1"},
	{http.MethodPost, "/_tinker/api/v1/llm/chat"},
}

func safeReservedDenial(w *httptest.ResponseRecorder) bool {
	if w.Code != http.StatusNotFound || w.Header().Get("Content-Type") != "application/json; charset=utf-8" ||
		w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Set-Cookie") != "" || w.Header().Get("Location") != "" ||
		w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("Referrer-Policy") != "same-origin" || !probeRequestID.MatchString(w.Header().Get("X-Request-ID")) || w.Body.Len() > 32<<10 {
		return false
	}
	var envelope map[string]json.RawMessage
	if json.Unmarshal(w.Body.Bytes(), &envelope) != nil || len(envelope) != 1 || envelope["error"] == nil {
		return false
	}
	var denial map[string]string
	if json.Unmarshal(envelope["error"], &denial) != nil || len(denial) != 3 {
		return false
	}
	return denial["code"] == "not_found" && denial["message"] == "This request is not authorized." && probeRequestID.MatchString(denial["request_id"]) && denial["request_id"] == w.Header().Get("X-Request-ID")
}

func ProbeCandidate(ctx context.Context, cfg config.Config, dataRoot string, r deployments.Record) (Probe, error) {
	root, e := CandidateFilesystem(dataRoot, r)
	if e != nil {
		return Probe{}, candidateProbeFailed(CandidateProbeReleaseEvidence)
	}
	index, e := os.ReadFile(filepath.Join(root, "index.html"))
	if e != nil {
		return Probe{}, candidateProbeFailed(CandidateProbeIndex)
	}
	v := identity.Identity{ID: "probe@invalid", Email: "probe@invalid"}
	ss := sessions.NewMemoryStore()
	token, _, e := ss.Create(r.AppID, v, time.Now().Add(time.Minute))
	if e != nil {
		return Probe{}, candidateProbeFailed(CandidateProbeSession)
	}
	mode := r.Manifest.AccessMode
	if mode == "" {
		mode = "private"
	}
	// Candidate verification proves the release posture and bytes. The durable
	// policy transition already owns acknowledgement and gate authorization.
	ps := &policies.MemoryStore{Policies: map[string]policies.Policy{r.AppID: {AppID: r.AppID, OwnerIdentityID: v.ID, Revision: 1, Mode: mode, Valid: true, Emails: map[string]struct{}{}, Domains: map[string]struct{}{}}}, Gate: policies.PublicGate{Enabled: mode == "public", Revision: 1, Valid: true}}
	app := apps.App{ID: r.AppID, Slug: r.AppSlug, DeploymentID: r.ID, Status: apps.Active, ReleaseRoot: root, ReleaseEvidence: releases.FileManifest{Files: r.Files, Hash: r.ReleaseHash}, SPAFallback: r.Manifest.SPAFallback != "", KVEnabled: r.Manifest.KV, BlobsEnabled: r.Manifest.Blobs, RealtimeEnabled: r.Manifest.Realtime, LLMChatRequested: r.Manifest.LLMChat, PublicIndexing: r.Manifest.Indexing}
	protected, preauth := &protectedProbeSpy{}, &preAuthProbeSpy{}
	g := gateway.Gateway{Config: cfg, Apps: apps.NewMemoryRepository(app), Authorizer: appauth.Authorizer{Sessions: ss, Policies: ps}, Protected: protected, PreAuth: preauth}
	host := r.AppSlug + "." + cfg.AppSuffix()
	anon := httptest.NewRequest("GET", "https://"+host+"/", nil)
	anon.Host = host
	aw := httptest.NewRecorder()
	g.ServeHTTP(aw, anon)
	if mode == "public" {
		if aw.Code != http.StatusOK || !bytes.Equal(aw.Body.Bytes(), index) || aw.Header().Get("Cache-Control") != "no-store" {
			return Probe{}, candidateProbeFailed(CandidateProbePublicStaticHealth)
		}
		wantRobots := "noindex, nofollow"
		if r.Manifest.Indexing {
			wantRobots = ""
		}
		if aw.Header().Get("X-Robots-Tag") != wantRobots {
			return Probe{}, candidateProbeFailed(CandidateProbePublicStaticHealth)
		}
		var assetPath, assetHash string
		var rootBytes, assetBytes int64
		for _, file := range r.Files {
			if file.Path == "index.html" {
				rootBytes = file.Size
			} else if assetPath == "" && releases.ServableStaticAssetPath(file.Path) {
				assetPath, assetHash, assetBytes = file.Path, file.Hash, file.Size
			}
		}
		if int64(len(index)) != rootBytes {
			return Probe{}, candidateProbeFailed(CandidateProbePublicStaticHealth)
		}
		if assetPath != "" {
			asset, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(assetPath)))
			if err != nil || int64(len(asset)) != assetBytes || fmt.Sprintf("%x", sha256.Sum256(asset)) != assetHash {
				return Probe{}, candidateProbeFailed(CandidateProbePublicStaticHealth)
			}
			q := httptest.NewRequest(http.MethodGet, "https://"+host+"/"+assetPath, nil)
			q.Host = host
			w := httptest.NewRecorder()
			g.ServeHTTP(w, q)
			if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), asset) || w.Header().Get("Set-Cookie") != "" || w.Header().Get("Location") != "" {
				return Probe{}, candidateProbeFailed(CandidateProbePublicStaticHealth)
			}
		}
		for _, route := range representativeReservedProbeRoutes {
			q := httptest.NewRequest(route.method, "https://"+host+route.path, nil)
			q.Host = host
			w := httptest.NewRecorder()
			g.ServeHTTP(w, q)
			if !safeReservedDenial(w) {
				return Probe{}, candidateProbeFailed(CandidateProbePublicStaticHealth)
			}
		}
		if protected.calls != 0 || preauth.calls != 0 {
			return Probe{}, candidateProbeFailed(CandidateProbePublicStaticHealth)
		}
		return Probe{URL: "https://" + host, PublicReachable: true, ReservedDenied: true, Posture: "public_static", RootSHA256: fmt.Sprintf("%x", sha256.Sum256(index)), RootBytes: rootBytes, AssetPath: assetPath, AssetSHA256: assetHash, AssetBytes: assetBytes, Indexing: r.Manifest.Indexing}, nil
	}
	if aw.Code != http.StatusUnauthorized || bytes.Contains(aw.Body.Bytes(), index) {
		return Probe{}, candidateProbeFailed(CandidateProbeAnonymousDenial)
	}
	auth := httptest.NewRequestWithContext(ctx, "GET", "https://"+host+"/", nil)
	auth.Host = host
	auth.AddCookie(&http.Cookie{Name: cfg.SessionCookie, Value: token})
	rw := httptest.NewRecorder()
	g.ServeHTTP(rw, auth)
	if rw.Code != http.StatusOK || !bytes.Equal(rw.Body.Bytes(), index) {
		return Probe{}, candidateProbeFailed(CandidateProbeAuthenticatedHealth)
	}
	return Probe{URL: "https://" + host, AnonymousDenied: true, AuthenticatedHealthy: true}, nil
}
