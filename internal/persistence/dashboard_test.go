package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/ChrisMarxDev/tinkercloud/internal/controlapi"
	"github.com/ChrisMarxDev/tinkercloud/internal/releases"
)

func TestDashboardReadModelRoleBoundAndSafe(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	if _, err := s.DB.Exec("INSERT INTO users VALUES('op','operator@example.com','operator','active',datetime('now')),('other','other@example.com','deployer','active',datetime('now')); INSERT INTO applications(id,owner_user_id,slug,status,policy_revision,created_at,updated_at) VALUES('otherapp','other','other-app','active',1,datetime('now'),datetime('now')); INSERT INTO access_policies(app_id,revision,mode,created_at) VALUES('a',1,'private',datetime('now')),('b',1,'private',datetime('now')),('otherapp',1,'private',datetime('now')); INSERT INTO api_tokens(id,user_id,app_id,secret_hash,scopes,expires_at) VALUES('tok','u','a',x'0102','app:read',datetime('now','+1 hour')); INSERT INTO audit_events(id,occurred_at,actor_kind,action,outcome,target_id) VALUES('audit',datetime('now'),'root','secret.checked','success','alpha')"); err != nil {
		t.Fatal(err)
	}
	svc := ControlService{Store: s}
	v, err := svc.Dashboard(context.Background(), controlapi.Actor{ID: "u", Role: "deployer", Active: true})
	if err != nil || len(v.Apps) != 2 || len(v.ActiveDeployerEmails) != 0 || len(v.Audit) != 0 {
		t.Fatalf("deployer dashboard %#v %v", v, err)
	}
	for _, a := range v.Apps {
		if a.Slug == "other-app" {
			t.Fatal("cross-owner app leaked")
		}
		for _, tok := range a.Tokens {
			if tok.ID == "" {
				t.Fatal("token read malformed")
			}
		}
	}
	v, err = svc.Dashboard(context.Background(), controlapi.Actor{ID: "op", Role: "operator", Active: true})
	if err != nil || len(v.Apps) != 3 || len(v.ActiveDeployerEmails) != 2 || len(v.Audit) != 1 {
		t.Fatalf("operator dashboard %#v %v", v, err)
	}
}

func TestDashboardRefusesTruncatedActiveDeployerAllowlist(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	if _, err := s.DB.Exec("INSERT INTO users(id,normalized_email,role,status,created_at) VALUES('op','operator@example.com','operator','active',datetime('now'))"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 101; i++ {
		if _, err := s.DB.Exec("INSERT INTO users(id,normalized_email,role,status,created_at) VALUES(?,?, 'deployer','active',datetime('now'))", fmt.Sprintf("d%d", i), fmt.Sprintf("d%d@example.com", i)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := (ControlService{Store: s}).Dashboard(context.Background(), controlapi.Actor{ID: "op", Role: "operator", Active: true}); err != ErrUnavailable {
		t.Fatalf("dashboard=%v", err)
	}
}

func TestDashboardExcludesDeletedAppsButKeepsOperationalStates(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	if _, err := s.DB.Exec(`
		INSERT INTO users VALUES
			('op','operator@example.com','operator','active',datetime('now')),
			('other','other@example.com','deployer','active',datetime('now'));
		INSERT INTO applications(id,owner_user_id,slug,status,policy_revision,created_at,updated_at) VALUES
			('deleted-own','u','deleted-own','deleted',1,datetime('now'),datetime('now')),
			('active-other','other','active-other','active',1,datetime('now'),datetime('now')),
			('deleted-other','other','deleted-other','deleted',1,datetime('now'),datetime('now'));
		INSERT INTO access_policies(app_id,revision,mode,created_at) VALUES
			('a',1,'private',datetime('now')),
			('b',1,'private',datetime('now')),
			('deleted-own',1,'private',datetime('now')),
			('active-other',1,'private',datetime('now')),
			('deleted-other',1,'private',datetime('now'))`); err != nil {
		t.Fatal(err)
	}

	svc := ControlService{Store: s}
	for _, tc := range []struct {
		name  string
		actor controlapi.Actor
		want  []string
	}{
		{"deployer", controlapi.Actor{ID: "u", Role: "deployer", Active: true}, []string{"alpha", "beta"}},
		{"operator", controlapi.Actor{ID: "op", Role: "operator", Active: true}, []string{"active-other", "alpha", "beta"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v, err := svc.Dashboard(context.Background(), tc.actor)
			if err != nil {
				t.Fatal(err)
			}
			if len(v.Apps) != len(tc.want) {
				t.Fatalf("dashboard app count = %d, want %d: %#v", len(v.Apps), len(tc.want), v.Apps)
			}
			for i, app := range v.Apps {
				if app.Slug != tc.want[i] || app.Status == "deleted" {
					t.Fatalf("dashboard apps = %#v, want operational apps %v", v.Apps, tc.want)
				}
			}
		})
	}

	var retained int
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM applications WHERE status='deleted'").Scan(&retained); err != nil || retained != 2 {
		t.Fatalf("deleted records retained = %d, %v", retained, err)
	}
}

func TestDashboardUsesCurrentCanonicalAccessPolicy(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	if _, err := s.DB.Exec("INSERT INTO access_policies(app_id,revision,mode,created_at) VALUES('a',1,'private',datetime('now')),('a',2,'private',datetime('now')),('b',1,'private',datetime('now')); INSERT INTO access_rules(id,app_id,policy_revision,kind,normalized_value,created_at) VALUES('old','a',1,'email','old@example.com',datetime('now')),('email','a',2,'email','viewer@example.com',datetime('now')),('domain','a',2,'domain','example.com',datetime('now')); UPDATE applications SET policy_revision=2 WHERE id='a'"); err != nil {
		t.Fatal(err)
	}
	v, err := (ControlService{Store: s}).Dashboard(context.Background(), controlapi.Actor{ID: "u", Role: "deployer", Active: true})
	if err != nil || len(v.Apps) != 2 {
		t.Fatalf("dashboard = %#v, %v", v, err)
	}
	for _, app := range v.Apps {
		if app.Slug != "alpha" {
			continue
		}
		if app.Access.Mode != "private" || app.Access.Revision != 2 || len(app.Access.Emails) != 1 || app.Access.Emails[0] != "viewer@example.com" || len(app.Access.Domains) != 1 || app.Access.Domains[0] != "example.com" {
			t.Fatalf("current access = %#v", app.Access)
		}
		return
	}
	t.Fatal("alpha missing")
}

func TestDashboardDescriptionsAreImmutableCurrentMetadataAndFailClosed(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	if _, err := s.DB.Exec("INSERT INTO access_policies(app_id,revision,mode,created_at) VALUES('a',1,'private',datetime('now')),('b',1,'private',datetime('now'))"); err != nil {
		t.Fatal(err)
	}
	current, _ := json.Marshal(releases.Manifest{Version: 1, Name: "alpha", Description: "Current release"})
	older, _ := json.Marshal(releases.Manifest{Version: 1, Name: "alpha", Description: "Prior release"})
	if _, err := s.DB.Exec("INSERT INTO deployments(id,app_id,manifest_json,state,created_at) VALUES('old','a',?,'superseded',datetime('now','-1 minute')),('current','a',?,'active',datetime('now')); UPDATE applications SET current_deployment_id='current' WHERE id='a'", older, current); err != nil {
		t.Fatal(err)
	}
	v, err := (ControlService{Store: s, AppSuffix: "apps.example.test"}).Dashboard(context.Background(), controlapi.Actor{ID: "u", Role: "deployer", Active: true})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, app := range v.Apps {
		if app.Slug == "alpha" {
			found = true
			if app.Description != "Current release" || app.StableURL != "https://alpha.apps.example.test/" || len(app.Releases) != 2 || app.Releases[0].Description != "Current release" || app.Releases[1].Description != "Prior release" {
				t.Fatalf("dashboard descriptions = %#v", app)
			}
		}
	}
	if !found {
		t.Fatal("alpha missing from dashboard")
	}
	if _, err := s.DB.Exec("UPDATE deployments SET manifest_json='not-json' WHERE id='current'"); err != nil {
		t.Fatal(err)
	}
	if _, err := (ControlService{Store: s, AppSuffix: "apps.example.test"}).Dashboard(context.Background(), controlapi.Actor{ID: "u", Role: "deployer", Active: true}); err != ErrUnavailable {
		t.Fatalf("corrupt final manifest = %v", err)
	}
}

func TestDashboardCurrentDescriptionSurvivesBoundedReleaseHistory(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	if _, err := s.DB.Exec("INSERT INTO access_policies(app_id,revision,mode,created_at) VALUES('a',1,'private',datetime('now')),('b',1,'private',datetime('now'))"); err != nil {
		t.Fatal(err)
	}
	manifest, _ := json.Marshal(releases.Manifest{Version: 1, Name: "alpha", Description: "Rolled-back current release"})
	if _, err := s.DB.Exec("INSERT INTO deployments(id,app_id,manifest_json,state,created_at) VALUES('current','a',?,'active',datetime('now','-200 minutes')); UPDATE applications SET current_deployment_id='current' WHERE id='a'", manifest); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		if _, err := s.DB.Exec("INSERT INTO deployments(id,app_id,state,created_at) VALUES(?,?,?,datetime('now',?))", fmt.Sprintf("newer-%03d", i), "a", "uploading", fmt.Sprintf("-%d minutes", i)); err != nil {
			t.Fatal(err)
		}
	}
	v, err := (ControlService{Store: s, AppSuffix: "apps.example.test"}).Dashboard(context.Background(), controlapi.Actor{ID: "u", Role: "deployer", Active: true})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, app := range v.Apps {
		if app.Slug == "alpha" {
			found = true
			if app.Description != "Rolled-back current release" || app.StableURL == "" || len(app.Releases) != 100 {
				t.Fatalf("bounded history lost current summary: %#v", app)
			}
		}
	}
	if !found {
		t.Fatal("alpha missing from dashboard")
	}
}

func TestDashboardPolicyStateFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, s *SQLiteStore)
	}{
		{"missing current revision", func(_ *testing.T, s *SQLiteStore) {
			_, _ = s.DB.Exec("INSERT INTO access_policies(app_id,revision,mode,created_at) VALUES('a',1,'private',datetime('now')),('b',1,'private',datetime('now'))")
			_, _ = s.DB.Exec("UPDATE applications SET policy_revision=2 WHERE id='a'")
		}},
		{"non-private current policy", func(_ *testing.T, s *SQLiteStore) {
			_, _ = s.DB.Exec("INSERT INTO access_policies(app_id,revision,mode,created_at) VALUES('a',1,'public',datetime('now')),('b',1,'private',datetime('now'))")
		}},
		{"unknown current rule", func(t *testing.T, s *SQLiteStore) {
			if _, err := s.DB.Exec("INSERT INTO access_policies(app_id,revision,mode,created_at) VALUES('a',1,'private',datetime('now')),('b',1,'private',datetime('now')); PRAGMA ignore_check_constraints=ON; INSERT INTO access_rules(id,app_id,policy_revision,kind,normalized_value,created_at) VALUES('unknown','a',1,'unknown','bad',datetime('now'))"); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := seeded(t)
			defer s.Close()
			tc.setup(t, s)
			if _, err := (ControlService{Store: s}).Dashboard(context.Background(), controlapi.Actor{ID: "u", Role: "deployer", Active: true}); err != ErrUnavailable {
				t.Fatalf("dashboard error = %v", err)
			}
		})
	}
}

func TestTokenReadModelsExposeOnlyNullableLastUseAndRespectOwnership(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	if _, err := s.DB.Exec("UPDATE applications SET status='active' WHERE id='a'"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec("INSERT INTO access_policies(app_id,revision,mode,created_at) VALUES('a',1,'private',datetime('now')),('b',1,'private',datetime('now'))"); err != nil {
		t.Fatal(err)
	}
	used := "2026-07-27T12:00:00Z"
	if _, err := s.DB.Exec("INSERT INTO api_tokens(id,user_id,app_id,secret_hash,scopes,expires_at,last_used_at) VALUES('used','u','a',x'0304','app:read',datetime('now','+1 hour'),?)", used); err != nil {
		t.Fatal(err)
	}
	svc := ControlService{Store: s}
	value, err := svc.Tokens(context.Background(), controlapi.Actor{ID: "u", Active: true}, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var items []struct {
		ID         string   `json:"id"`
		Scopes     []string `json:"scopes"`
		ExpiresAt  string   `json:"expires_at"`
		LastUsedAt *string  `json:"last_used_at"`
		Revoked    bool     `json:"revoked"`
	}
	if err := json.Unmarshal(encoded, &items); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range items {
		if item.ID == "used" {
			found = true
			if item.LastUsedAt == nil || *item.LastUsedAt != used {
				t.Fatalf("last use = %#v", item.LastUsedAt)
			}
		}
	}
	if !found {
		t.Fatal("used token absent")
	}
	if _, err := svc.Tokens(context.Background(), controlapi.Actor{ID: "other", Active: true}, "alpha"); err != ErrUnavailable {
		t.Fatalf("cross-owner list = %v", err)
	}
	v, err := svc.Dashboard(context.Background(), controlapi.Actor{ID: "u", Role: "deployer", Active: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, app := range v.Apps {
		for _, token := range app.Tokens {
			if token.ID == "used" && (token.LastUsedAt == nil || *token.LastUsedAt != used) {
				t.Fatalf("dashboard last use = %#v", token.LastUsedAt)
			}
		}
	}
}
