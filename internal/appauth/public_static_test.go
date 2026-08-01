package appauth

import (
	"context"
	"testing"
	"time"

	"github.com/ChrisMarxDev/tinkercloud/internal/apps"
	"github.com/ChrisMarxDev/tinkercloud/internal/identity"
	"github.com/ChrisMarxDev/tinkercloud/internal/policies"
	"github.com/ChrisMarxDev/tinkercloud/internal/releases"
	"github.com/ChrisMarxDev/tinkercloud/internal/sessions"
)

func TestAuthorizeStaticPublicIsCurrentAndBound(t *testing.T) {
	store := sessions.NewMemoryStore()
	token, _, err := store.Create("a", identity.Identity{ID: "viewer", Email: "viewer@example.test"}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	policiesStore := &policies.MemoryStore{Policies: map[string]policies.Policy{"a": {AppID: "a", Mode: "public", Revision: 7, Valid: true}}, Gate: policies.PublicGate{Enabled: true, Revision: 9, Valid: true}}
	a := apps.App{ID: "a", Slug: "alpha", DeploymentID: "deployment", ReleaseRoot: "/immutable", ReleaseEvidence: releases.FileManifest{Hash: "hash"}, PublicIndexing: true}
	auth, err := (Authorizer{Sessions: store, Policies: policiesStore}).AuthorizeStatic(context.Background(), a, token, "req")
	if err != nil || !auth.Public() || auth.AppID() != "a" || auth.DeploymentID() != "deployment" || auth.PolicyRevision() != 7 || auth.PublicGateRevision() != 9 || !auth.Indexing() {
		t.Fatalf("unexpected public context: %#v, %v", auth, err)
	}
	policiesStore.Gate.Enabled = false
	auth, err = (Authorizer{Sessions: store, Policies: policiesStore}).AuthorizeStatic(context.Background(), a, "", "req")
	if err == nil || auth != nil {
		t.Fatal("gate disable must deny next anonymous request")
	}
}
