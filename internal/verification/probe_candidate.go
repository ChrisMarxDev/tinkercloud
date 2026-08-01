package verification

import (
	"bytes"
	"context"
	"crypto/sha256"
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
	"time"
)

func ProbeCandidate(ctx context.Context, cfg config.Config, dataRoot string, r deployments.Record) (Probe, error) {
	root, e := CandidateFilesystem(dataRoot, r)
	if e != nil {
		return Probe{}, e
	}
	index, e := os.ReadFile(filepath.Join(root, "index.html"))
	if e != nil || len(index) > 1<<20 {
		return Probe{}, errors.New("candidate index unavailable")
	}
	v := identity.Identity{ID: "probe@invalid", Email: "probe@invalid"}
	ss := sessions.NewMemoryStore()
	token, _, e := ss.Create(r.AppID, v, time.Now().Add(time.Minute))
	if e != nil {
		return Probe{}, e
	}
	mode := r.Manifest.AccessMode
	if mode == "" {
		mode = "private"
	}
	ps := &policies.MemoryStore{Policies: map[string]policies.Policy{r.AppID: {AppID: r.AppID, OwnerIdentityID: v.ID, Revision: 1, Mode: mode, Valid: true, Emails: map[string]struct{}{}, Domains: map[string]struct{}{}}}, Gate: policies.PublicGate{Enabled: mode == "public" && r.PublicAcknowledged, Revision: 1, Valid: true}}
	app := apps.App{ID: r.AppID, Slug: r.AppSlug, DeploymentID: r.ID, Status: apps.Active, ReleaseRoot: root, ReleaseEvidence: releases.FileManifest{Files: r.Files, Hash: r.ReleaseHash}, SPAFallback: r.Manifest.SPAFallback != "", KVEnabled: r.Manifest.KV, BlobsEnabled: r.Manifest.Blobs, RealtimeEnabled: r.Manifest.Realtime, LLMChatRequested: r.Manifest.LLMChat, PublicIndexing: r.Manifest.Indexing}
	g := gateway.Gateway{Config: cfg, Apps: apps.NewMemoryRepository(app), Authorizer: appauth.Authorizer{Sessions: ss, Policies: ps}}
	host := r.AppSlug + "." + cfg.AppSuffix()
	anon := httptest.NewRequest("GET", "https://"+host+"/", nil)
	anon.Host = host
	aw := httptest.NewRecorder()
	g.ServeHTTP(aw, anon)
	if mode == "public" {
		if aw.Code != http.StatusOK || !bytes.Equal(aw.Body.Bytes(), index) || aw.Header().Get("Cache-Control") != "no-store" {
			return Probe{}, errors.New("public candidate probe failed")
		}
		wantRobots := "noindex, nofollow"
		if r.Manifest.Indexing {
			wantRobots = ""
		}
		if aw.Header().Get("X-Robots-Tag") != wantRobots {
			return Probe{}, errors.New("public indexing probe failed")
		}
		for _, p := range []string{"/_tinker/auth/login", "/_tinker/api/v1/app", "/_tinker/api/v1/kv", "/_tinker/ws/v1"} {
			q := httptest.NewRequest("GET", "https://"+host+p, nil)
			q.Host = host
			w := httptest.NewRecorder()
			g.ServeHTTP(w, q)
			if w.Code < 400 || w.Code >= 500 || w.Body.Len() > 32<<10 {
				return Probe{}, errors.New("public reserved probe failed")
			}
		}
		return Probe{URL: "https://" + host, AnonymousDenied: true, AuthenticatedHealthy: true, Posture: "public_static", RootSHA256: fmt.Sprintf("%x", sha256.Sum256(index)), Indexing: r.Manifest.Indexing}, nil
	}
	if aw.Code != http.StatusUnauthorized || bytes.Contains(aw.Body.Bytes(), index) {
		return Probe{}, errors.New("anonymous probe failed")
	}
	auth := httptest.NewRequestWithContext(ctx, "GET", "https://"+host+"/", nil)
	auth.Host = host
	auth.AddCookie(&http.Cookie{Name: cfg.SessionCookie, Value: token})
	rw := httptest.NewRecorder()
	g.ServeHTTP(rw, auth)
	if rw.Code != http.StatusOK || !bytes.Equal(rw.Body.Bytes(), index) {
		return Probe{}, errors.New("authenticated probe failed")
	}
	return Probe{URL: "https://" + host, AnonymousDenied: true, AuthenticatedHealthy: true}, nil
}
