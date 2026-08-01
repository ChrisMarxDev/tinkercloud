package persistence

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ChrisMarxDev/tinkercloud/internal/controlapi"
	"github.com/ChrisMarxDev/tinkercloud/internal/releases"
)

func seedCatalogApp(t *testing.T, s *SQLiteStore, id, owner, slug string, manifest releases.Manifest, email, domain string) {
	t.Helper()
	b, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`INSERT INTO applications(id,owner_user_id,slug,status,policy_revision,created_at,updated_at)
		VALUES(?,?,?,'active',1,datetime('now'),datetime('now'))`, id, owner, slug); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`INSERT INTO access_policies(app_id,revision,mode,created_at) VALUES(?,1,'private',datetime('now'))`, id); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`INSERT INTO deployments(id,app_id,manifest_json,state,created_at) VALUES(?,?,?,'active',datetime('now'))`, id+"-deployment", id, b); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`UPDATE applications SET current_deployment_id=? WHERE id=?`, id+"-deployment", id); err != nil {
		t.Fatal(err)
	}
	if email != "" {
		if _, err = s.DB.Exec(`INSERT INTO access_rules(id,app_id,policy_revision,kind,normalized_value,created_at) VALUES(?, ?, 1, 'email', ?, datetime('now'))`, id+"-email", id, email); err != nil {
			t.Fatal(err)
		}
	}
	if domain != "" {
		if _, err = s.DB.Exec(`INSERT INTO access_rules(id,app_id,policy_revision,kind,normalized_value,created_at) VALUES(?, ?, 1, 'domain', ?, datetime('now'))`, id+"-domain", id, domain); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCatalogFiltersCurrentPolicyBeforeReadingActiveMetadata(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	if _, err := s.DB.Exec("INSERT INTO users(id,normalized_email,role,status,created_at) VALUES('other','other@example.test','deployer','active',datetime('now'))"); err != nil {
		t.Fatal(err)
	}
	seedCatalogApp(t, s, "catalog-a", "u", "catalog-alpha", releases.Manifest{Version: 2, Name: "catalog-alpha", Description: "Alpha board", Tags: []string{"demo", "team"}, AccessMode: "private"}, "viewer@example.test", "")
	seedCatalogApp(t, s, "other", "other", "catalog-app", releases.Manifest{Version: 2, Name: "catalog-app", Description: "Team board", Tags: []string{"team"}, AccessMode: "private"}, "", "example.test")
	// A suspended app and an unrelated active app must not become catalog
	// candidates even though their immutable metadata is valid.
	seedCatalogApp(t, s, "catalog-b", "u", "catalog-beta", releases.Manifest{Version: 2, Name: "catalog-beta", AccessMode: "private"}, "viewer@example.test", "")
	if _, err := s.DB.Exec("UPDATE applications SET status='suspended' WHERE id='catalog-b'"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec("INSERT INTO applications(id,owner_user_id,slug,status,policy_revision,created_at,updated_at) VALUES('hidden','other','hidden','active',1,datetime('now'),datetime('now')); INSERT INTO access_policies(app_id,revision,mode,created_at) VALUES('hidden',1,'private',datetime('now'))"); err != nil {
		t.Fatal(err)
	}

	svc := ControlService{Store: s, AppSuffix: "apps.example.test"}
	viewer := controlapi.ViewerIdentity{IdentityID: "viewer@example.test", Email: "viewer@example.test", IdentitySessionID: "gis"}
	apps, err := svc.Catalog(context.Background(), viewer)
	if err != nil || len(apps) != 2 {
		t.Fatalf("catalog=%#v err=%v", apps, err)
	}
	if apps[0].Slug != "catalog-alpha" || apps[0].Description != "Alpha board" || apps[0].StableURL != "https://catalog-alpha.apps.example.test/" || len(apps[0].Tags) != 2 || apps[1].Slug != "catalog-app" {
		t.Fatalf("catalog records=%#v", apps)
	}

	// Removing the current email rule changes the next read; the stale browser
	// card is never a server-side authorization grant.
	if _, err := s.DB.Exec("DELETE FROM access_rules WHERE id='catalog-a-email'"); err != nil {
		t.Fatal(err)
	}
	apps, err = svc.Catalog(context.Background(), viewer)
	if err != nil || len(apps) != 1 || apps[0].Slug != "catalog-app" {
		t.Fatalf("revoked catalog=%#v err=%v", apps, err)
	}
}

func TestCatalogFailsClosedForMalformedAuthorizedActiveManifest(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	seedCatalogApp(t, s, "catalog-a", "u", "catalog-alpha", releases.Manifest{Version: 2, Name: "catalog-alpha", AccessMode: "private"}, "viewer@example.test", "")
	if _, err := s.DB.Exec("UPDATE deployments SET manifest_json='not-json' WHERE id='catalog-a-deployment'"); err != nil {
		t.Fatal(err)
	}
	if _, err := (ControlService{Store: s, AppSuffix: "apps.example.test"}).Catalog(context.Background(), controlapi.ViewerIdentity{IdentityID: "viewer", Email: "viewer@example.test", IdentitySessionID: "gis"}); err != ErrUnavailable {
		t.Fatalf("malformed active manifest catalog error=%v", err)
	}
}

func TestCatalogIncludesOnlyEffectivePublicStaticApps(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	seedCatalogApp(t, s, "public-catalog", "u", "public-catalog", releases.Manifest{Version: 2, Name: "public-catalog", Description: "Public proof", Tags: []string{"demo"}, AccessMode: "public"}, "", "")
	if _, err := s.DB.Exec("UPDATE access_policies SET mode='public' WHERE app_id='public-catalog'"); err != nil {
		t.Fatal(err)
	}
	svc := ControlService{Store: s, AppSuffix: "apps.example.test"}
	viewer := controlapi.ViewerIdentity{IdentityID: "viewer", Email: "viewer@example.test"}
	apps, err := svc.Catalog(context.Background(), viewer)
	if err != nil || len(apps) != 0 {
		t.Fatalf("gate-off catalog=%#v err=%v", apps, err)
	}
	if _, err := s.DB.Exec("UPDATE public_static_settings SET enabled=1,revision=2 WHERE singleton=1"); err != nil {
		t.Fatal(err)
	}
	apps, err = svc.Catalog(context.Background(), viewer)
	if err != nil || len(apps) != 1 || apps[0].Slug != "public-catalog" || apps[0].Posture != "public" {
		t.Fatalf("public catalog=%#v err=%v", apps, err)
	}
	// An unavailable gate may not accidentally turn a public row into a
	// discoverable one, even though public metadata remains in SQLite.
	if _, err := s.DB.Exec("DELETE FROM public_static_settings"); err != nil {
		t.Fatal(err)
	}
	apps, err = svc.Catalog(context.Background(), viewer)
	if err != nil || len(apps) != 0 {
		t.Fatalf("missing-gate catalog=%#v err=%v", apps, err)
	}
}
