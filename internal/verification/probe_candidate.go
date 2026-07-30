package verification

import (
	"bytes"
	"context"
	"errors"
	"github.com/tinyhost/tiny/internal/appauth"
	"github.com/tinyhost/tiny/internal/apps"
	"github.com/tinyhost/tiny/internal/config"
	"github.com/tinyhost/tiny/internal/deployments"
	"github.com/tinyhost/tiny/internal/gateway"
	"github.com/tinyhost/tiny/internal/identity"
	"github.com/tinyhost/tiny/internal/policies"
	"github.com/tinyhost/tiny/internal/releases"
	"github.com/tinyhost/tiny/internal/sessions"
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
	ps := &policies.MemoryStore{Policies: map[string]policies.Policy{r.AppID: {AppID: r.AppID, OwnerIdentityID: v.ID, Revision: 1, Valid: true, Emails: map[string]struct{}{}, Domains: map[string]struct{}{}}}}
	app := apps.App{ID: r.AppID, Slug: r.AppSlug, Status: apps.Active, ReleaseRoot: root, ReleaseEvidence: releases.FileManifest{Files: r.Files, Hash: r.ReleaseHash}, SPAFallback: r.Manifest.SPAFallback != ""}
	g := gateway.Gateway{Config: cfg, Apps: apps.NewMemoryRepository(app), Authorizer: appauth.Authorizer{Sessions: ss, Policies: ps}}
	host := r.AppSlug + "." + cfg.AppSuffix()
	anon := httptest.NewRequest("GET", "https://"+host+"/", nil)
	anon.Host = host
	aw := httptest.NewRecorder()
	g.ServeHTTP(aw, anon)
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
