package persistence

import (
	"context"
	"errors"
	"github.com/tinyhost/tiny/internal/deployments"
	"github.com/tinyhost/tiny/internal/releases"
	"os"
	"testing"
)

func manifest(slug string) releases.Manifest { return releases.Manifest{Version: 1, Name: slug} }

func TestDeploymentRepositoryCreateGet(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	r := DeploymentRepository{Store: s}
	d := deployments.Record{Deployment: releases.Deployment{ID: "d1", AppID: "a", State: releases.Uploading}, OwnerID: "u", IdempotencyKey: "k"}
	d.ArchiveHash[0] = 1
	if e := r.Create(context.Background(), d); e != nil {
		t.Fatal(e)
	}
	d.State = releases.Verified
	d.Manifest = manifest("a")
	d.ArchiveHash[0] = 2
	if e := r.Create(context.Background(), d); e != nil {
		t.Fatal(e)
	}
	got, e := r.Get(context.Background(), "d1")
	if e != nil || got.State != releases.Verified || got.ArchiveHash[0] != 2 {
		t.Fatal(e, got)
	}
	other := d
	other.ID = "d2"
	if e := r.Create(context.Background(), other); !errors.Is(e, deployments.ErrIdempotency) {
		t.Fatal(e)
	}
	if _, e := r.Get(context.Background(), "missing"); !errors.Is(e, os.ErrNotExist) {
		t.Fatal(e)
	}
}

func TestDeploymentRepositoryPersistsFileEvidenceWithVerifiedRecord(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	r := DeploymentRepository{Store: s}
	d := deployments.Record{
		Deployment:     releases.Deployment{ID: "evidence", AppID: "a", State: releases.Verified, ReleaseHash: "release-hash"},
		OwnerID:        "u",
		IdempotencyKey: "evidence",
		Manifest:       manifest("a"),
		Files:          []releases.File{{Path: "index.html", Size: 2, Hash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},
	}
	if err := r.Create(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	got, err := r.Get(context.Background(), "evidence")
	if err != nil || len(got.Files) != 1 || got.Files[0] != d.Files[0] {
		t.Fatalf("file evidence missing after verified write: %#v %v", got.Files, err)
	}
}
func TestDeploymentRepositoryActivePointers(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	r := DeploymentRepository{Store: s}
	if x, e := r.Active(context.Background(), "a"); e != nil || x != nil {
		t.Fatal(e, x)
	}
	d := deployments.Record{Deployment: releases.Deployment{ID: "active", AppID: "a", State: releases.Active}, OwnerID: "u", IdempotencyKey: "x", Manifest: manifest("a")}
	if e := r.Create(context.Background(), d); e != nil {
		t.Fatal(e)
	}
	if _, e := s.DB.Exec("UPDATE applications SET current_deployment_id='active' WHERE id='a'"); e != nil {
		t.Fatal(e)
	}
	if x, e := r.Active(context.Background(), "a"); e != nil || x.ID != "active" {
		t.Fatal(e, x)
	}
	if _, e := s.DB.Exec("UPDATE applications SET current_deployment_id='active' WHERE id='b'"); e != nil {
		t.Fatal(e)
	}
	if _, e := r.Active(context.Background(), "b"); e == nil {
		t.Fatal("cross app pointer")
	}
	if _, e := s.DB.Exec("UPDATE deployments SET state='verified' WHERE id='active'"); e != nil {
		t.Fatal(e)
	}
	if _, e := r.Active(context.Background(), "a"); e == nil {
		t.Fatal("non active pointer")
	}
}
func TestDeploymentCommitActivation(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	r := DeploymentRepository{Store: s}
	n := deployments.Record{Deployment: releases.Deployment{ID: "n", AppID: "a", State: releases.Verified}, OwnerID: "u", IdempotencyKey: "n", Manifest: manifest("a")}
	if e := r.Create(context.Background(), n); e != nil {
		t.Fatal(e)
	}
	if e := r.CommitActivation(context.Background(), n, nil, "k1"); e != nil {
		t.Fatal(e)
	}
	x, e := r.Active(context.Background(), "a")
	if e != nil || x.ID != "n" {
		t.Fatal(e, x)
	}
	bad := n
	bad.ID = "bad"
	bad.State = releases.Uploaded
	if e := r.CommitActivation(context.Background(), bad, x, "k2"); e == nil {
		t.Fatal("invalid next")
	}
	x2, _ := r.Active(context.Background(), "a")
	if x2.ID != "n" {
		t.Fatal("pointer changed")
	}
}
func TestDeploymentCommitActivationReplacement(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	r := DeploymentRepository{Store: s}
	a := deployments.Record{Deployment: releases.Deployment{ID: "a1", AppID: "a", State: releases.Verified}, OwnerID: "u", IdempotencyKey: "a1", Manifest: manifest("a")}
	_ = r.Create(context.Background(), a)
	if e := r.CommitActivation(context.Background(), a, nil, "k1"); e != nil {
		t.Fatal(e)
	}
	old, _ := r.Active(context.Background(), "a")
	b := deployments.Record{Deployment: releases.Deployment{ID: "b1", AppID: "a", State: releases.Verified}, OwnerID: "u", IdempotencyKey: "b1", Manifest: manifest("a")}
	_ = r.Create(context.Background(), b)
	if e := r.CommitActivation(context.Background(), b, old, "k2"); e != nil {
		t.Fatal(e)
	}
	active, e := r.Active(context.Background(), "a")
	if e != nil || active.ID != "b1" {
		t.Fatal(e, active)
	}
	gone, e := r.Get(context.Background(), "a1")
	if e != nil || gone.State != releases.Superseded {
		t.Fatal(e, gone.State)
	}
}
func TestDeploymentFail(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	r := DeploymentRepository{Store: s}
	d := deployments.Record{Deployment: releases.Deployment{ID: "f", AppID: "a", State: releases.Uploaded}, OwnerID: "u", IdempotencyKey: "f"}
	if e := r.Create(context.Background(), d); e != nil {
		t.Fatal(e)
	}
	if e := r.Fail(context.Background(), "f"); e != nil {
		t.Fatal(e)
	}
	if e := r.Fail(context.Background(), "f"); e != nil {
		t.Fatal(e)
	}
	x, _ := r.Get(context.Background(), "f")
	if x.State != releases.Failed {
		t.Fatal(x.State)
	}
	if e := r.Fail(context.Background(), "missing"); e == nil {
		t.Fatal("unknown")
	}
}
func TestDeploymentFailTerminalStatesDenied(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	r := DeploymentRepository{Store: s}
	for _, state := range []releases.State{releases.Active, releases.Superseded, releases.Rejected} {
		d := deployments.Record{Deployment: releases.Deployment{ID: string(state), AppID: "a", State: state}, OwnerID: "u", IdempotencyKey: string(state), Manifest: manifest("a")}
		if e := r.Create(context.Background(), d); e != nil {
			t.Fatal(e)
		}
		if e := r.Fail(context.Background(), d.ID); e == nil {
			t.Fatal(state)
		}
		got, _ := r.Get(context.Background(), d.ID)
		if got.State != state {
			t.Fatal(got.State)
		}
	}
}
func TestDeploymentCommitActivationNonCurrentOldRollback(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	r := DeploymentRepository{Store: s}
	a := deployments.Record{Deployment: releases.Deployment{ID: "current", AppID: "a", State: releases.Active}, OwnerID: "u", IdempotencyKey: "current", Manifest: manifest("a")}
	b := deployments.Record{Deployment: releases.Deployment{ID: "other", AppID: "a", State: releases.Active}, OwnerID: "u", IdempotencyKey: "other", Manifest: manifest("a")}
	c := deployments.Record{Deployment: releases.Deployment{ID: "next", AppID: "a", State: releases.Verified}, OwnerID: "u", IdempotencyKey: "next", Manifest: manifest("a")}
	for _, d := range []deployments.Record{a, b, c} {
		if e := r.Create(context.Background(), d); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := s.DB.Exec("UPDATE applications SET current_deployment_id='current' WHERE id='a'"); e != nil {
		t.Fatal(e)
	}
	if e := r.CommitActivation(context.Background(), c, &b, "k3"); e == nil {
		t.Fatal("non-current old accepted")
	}
	for id, want := range map[string]releases.State{"current": releases.Active, "other": releases.Active, "next": releases.Verified} {
		got, _ := r.Get(context.Background(), id)
		if got.State != want {
			t.Fatalf("%s=%s", id, got.State)
		}
	}
	active, _ := r.Active(context.Background(), "a")
	if active.ID != "current" {
		t.Fatal("pointer changed")
	}
}

func TestDeploymentActivationAndRollbackBindManifestPolicyAtomically(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	r := DeploymentRepository{Store: s}
	// Model an old broad active policy and release.
	if _, err := s.DB.Exec("INSERT INTO access_policies(app_id,revision,mode,created_by,created_at) VALUES('a',1,'private','u',datetime('now'))"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec("INSERT INTO access_rules(id,app_id,policy_revision,kind,normalized_value,created_by,created_at) VALUES('broad','a',1,'email','broad@example.com','u',datetime('now'))"); err != nil {
		t.Fatal(err)
	}
	old := deployments.Record{Deployment: releases.Deployment{ID: "old-policy", AppID: "a", State: releases.Active}, OwnerID: "u", AppSlug: "alpha", IdempotencyKey: "old-policy", Manifest: releases.Manifest{Version: 1, Name: "alpha", Emails: []string{"broad@example.com"}}}
	next := deployments.Record{Deployment: releases.Deployment{ID: "narrow-policy", AppID: "a", State: releases.Verified}, OwnerID: "u", AppSlug: "alpha", IdempotencyKey: "narrow-policy", Manifest: releases.Manifest{Version: 1, Name: "alpha", Emails: []string{"alice@example.com"}, Domains: []string{"internal.example", "train.example"}}}
	if err := r.Create(context.Background(), old); err != nil {
		t.Fatal(err)
	}
	if err := r.Create(context.Background(), next); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec("UPDATE applications SET current_deployment_id='old-policy' WHERE id='a'"); err != nil {
		t.Fatal(err)
	}
	if err := r.CommitActivation(context.Background(), next, &old, "activate-policy"); err != nil {
		t.Fatal(err)
	}
	var revision int
	var current string
	if err := s.DB.QueryRow("SELECT policy_revision,current_deployment_id FROM applications WHERE id='a'").Scan(&revision, &current); err != nil || revision != 2 || current != "narrow-policy" {
		t.Fatalf("revision=%d current=%q err=%v", revision, current, err)
	}
	var value string
	if err := s.DB.QueryRow("SELECT normalized_value FROM access_rules WHERE app_id='a' AND policy_revision=2 AND kind='email'").Scan(&value); err != nil || value != "alice@example.com" {
		t.Fatalf("candidate policy not installed: %q %v", value, err)
	}
	if err := s.DB.QueryRow("SELECT normalized_value FROM access_rules WHERE app_id='a' AND policy_revision=2 AND kind='domain' ORDER BY normalized_value LIMIT 1").Scan(&value); err != nil || value != "internal.example" {
		t.Fatalf("candidate domain policy not installed: %q %v", value, err)
	}
	currentRecord, err := r.Active(context.Background(), "a")
	if err != nil || currentRecord == nil {
		t.Fatal(err)
	}
	rollbackTarget, err := r.Get(context.Background(), "old-policy")
	if err != nil {
		t.Fatal(err)
	}
	if err = r.CommitRollback(context.Background(), rollbackTarget, *currentRecord, "rollback-policy"); err != nil {
		t.Fatal(err)
	}
	if err := s.DB.QueryRow("SELECT policy_revision,current_deployment_id FROM applications WHERE id='a'").Scan(&revision, &current); err != nil || revision != 3 || current != "old-policy" {
		t.Fatalf("rollback revision=%d current=%q err=%v", revision, current, err)
	}
	if err := s.DB.QueryRow("SELECT normalized_value FROM access_rules WHERE app_id='a' AND policy_revision=3 AND kind='email'").Scan(&value); err != nil || value != "broad@example.com" {
		t.Fatalf("rollback policy not restored: %q %v", value, err)
	}
}

func TestDeploymentCandidatePolicyFailurePreservesActivePointerAndPolicy(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	r := DeploymentRepository{Store: s}
	if _, err := s.DB.Exec("INSERT INTO access_policies(app_id,revision,mode,created_at) VALUES('a',1,'private',datetime('now'))"); err != nil {
		t.Fatal(err)
	}
	old := deployments.Record{Deployment: releases.Deployment{ID: "old", AppID: "a", State: releases.Active}, OwnerID: "u", AppSlug: "alpha", IdempotencyKey: "old", Manifest: releases.Manifest{Version: 1, Name: "alpha"}}
	bad := deployments.Record{Deployment: releases.Deployment{ID: "bad", AppID: "a", State: releases.Verified}, OwnerID: "u", AppSlug: "alpha", IdempotencyKey: "bad", Manifest: releases.Manifest{Version: 1, Name: "alpha", Emails: []string{"not-an-email"}}}
	for _, d := range []deployments.Record{old, bad} {
		if err := r.Create(context.Background(), d); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.DB.Exec("UPDATE applications SET current_deployment_id='old' WHERE id='a'"); err != nil {
		t.Fatal(err)
	}
	if s.CandidatePolicyReady(context.Background(), bad) {
		t.Fatal("invalid candidate policy passed gate")
	}
	if err := r.CommitActivation(context.Background(), bad, &old, "bad-policy"); err == nil {
		t.Fatal("invalid candidate policy activated")
	}
	var revision int
	var current string
	if err := s.DB.QueryRow("SELECT policy_revision,current_deployment_id FROM applications WHERE id='a'").Scan(&revision, &current); err != nil || revision != 1 || current != "old" {
		t.Fatalf("candidate failure changed state: revision=%d current=%q err=%v", revision, current, err)
	}
}

func TestDeploymentActivationAuditFailurePreservesReleaseAndPolicy(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	r := DeploymentRepository{Store: s}
	if _, err := s.DB.Exec("INSERT INTO access_policies(app_id,revision,mode,created_at) VALUES('a',1,'private',datetime('now'))"); err != nil {
		t.Fatal(err)
	}
	old := deployments.Record{Deployment: releases.Deployment{ID: "old-audit", AppID: "a", State: releases.Active}, OwnerID: "u", AppSlug: "alpha", IdempotencyKey: "old-audit", Manifest: releases.Manifest{Version: 1, Name: "alpha"}}
	next := deployments.Record{Deployment: releases.Deployment{ID: "next-audit", AppID: "a", State: releases.Verified}, OwnerID: "u", AppSlug: "alpha", IdempotencyKey: "next-audit", Manifest: releases.Manifest{Version: 1, Name: "alpha", Emails: []string{"alice@example.com"}}}
	for _, d := range []deployments.Record{old, next} {
		if err := r.Create(context.Background(), d); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.DB.Exec("UPDATE applications SET current_deployment_id='old-audit' WHERE id='a'"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec("CREATE TRIGGER deny_deployment_audit BEFORE INSERT ON audit_events WHEN NEW.action='deployment.activated' BEGIN SELECT RAISE(ABORT,'deny'); END"); err != nil {
		t.Fatal(err)
	}
	if err := r.CommitActivation(context.Background(), next, &old, "audit-failure"); err == nil {
		t.Fatal("activation succeeded despite audit failure")
	}
	var revision int
	var current string
	if err := s.DB.QueryRow("SELECT policy_revision,current_deployment_id FROM applications WHERE id='a'").Scan(&revision, &current); err != nil || revision != 1 || current != "old-audit" {
		t.Fatalf("audit failure changed state: revision=%d current=%q err=%v", revision, current, err)
	}
	var policies int
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM access_policies WHERE app_id='a'").Scan(&policies); err != nil || policies != 1 {
		t.Fatalf("audit failure left policy revision: %d %v", policies, err)
	}
}
