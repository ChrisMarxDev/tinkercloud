package persistence

import (
	"context"
	"database/sql"
	"errors"
	"github.com/ChrisMarxDev/tinkercloud/internal/controlapi"
	"github.com/ChrisMarxDev/tinkercloud/internal/deployments"
	"github.com/ChrisMarxDev/tinkercloud/internal/releases"
	"os"
	"path/filepath"
	"testing"
)

func manifest(slug string) releases.Manifest { return releases.Manifest{Version: 1, Name: slug} }

type replayRevoker struct{ calls int }

func (r *replayRevoker) Revoke(string, string) { r.calls++ }

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

func TestDeploymentRecoveryFailsMissingActiveAndMakesAppUnavailable(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	r := DeploymentRepository{Store: s}
	if _, err := s.DB.Exec("INSERT INTO deployments(id,app_id,created_by,idempotency_key,state,created_at) VALUES('broken','a','u','broken','active',datetime('now')); UPDATE applications SET current_deployment_id='broken' WHERE id='a'"); err != nil {
		t.Fatal(err)
	}
	if err := r.ApplyRecovery(context.Background(), "broken", releases.Failed); err != nil {
		t.Fatal(err)
	}
	if err := r.ApplyRecovery(context.Background(), "broken", releases.Failed); err != nil {
		t.Fatal(err)
	}
	var state string
	var current sql.NullString
	if err := s.DB.QueryRow("SELECT d.state,a.current_deployment_id FROM deployments d JOIN applications a ON a.id=d.app_id WHERE d.id='broken'").Scan(&state, &current); err != nil || state != "failed" || current.Valid {
		t.Fatalf("recovery state=%q current=%#v err=%v", state, current, err)
	}
	var appStatus string
	if err := s.DB.QueryRow("SELECT status FROM applications WHERE id='a'").Scan(&appStatus); err != nil || appStatus != "failed" {
		t.Fatalf("app remained available: %q %v", appStatus, err)
	}
}

func TestRecoveryRecordsClassifyCorruptHistoricalRowsWithoutBlockingGoodActiveApp(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	seedActiveRelease(t, s)
	r := DeploymentRepository{Store: s}
	if _, err := s.DB.Exec(`
        INSERT INTO deployments(id,app_id,created_by,idempotency_key,release_hash,manifest_json,state,created_at)
        VALUES
	          ('corrupt-verified','b','u','corrupt-verified','not-a-release',X'ff','verified',datetime('now')),
          ('corrupt-superseded','b','u','corrupt-superseded','not-a-release',X'ff','superseded',datetime('now')),
          ('missing-files','b','u','missing-files','not-a-release','{"version":1,"name":"beta"}','verified',datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	if err := (&deployments.Service{Repo: r}).RecoverStartup(context.Background(), deployments.FilesystemEvidence{Root: s.DataRoot}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"corrupt-verified", "corrupt-superseded", "missing-files"} {
		var state string
		if err := s.DB.QueryRow("SELECT state FROM deployments WHERE id=?", id).Scan(&state); err != nil || state != string(releases.Failed) {
			t.Fatalf("%s recovery state=%q err=%v", id, state, err)
		}
	}
	active, err := r.Active(context.Background(), "a")
	if err != nil || active == nil || active.ID != "d" {
		t.Fatalf("known-good active app was blocked: %#v %v", active, err)
	}
}

func TestRecoveryCorruptCurrentRecordAtomicallyFailsAppAndClearsPointer(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	r := DeploymentRepository{Store: s}
	if _, err := s.DB.Exec(`
        UPDATE applications SET status='active' WHERE id='b';
        INSERT INTO deployments(id,app_id,created_by,idempotency_key,release_hash,manifest_json,state,created_at)
        VALUES('corrupt-current','b','u','corrupt-current','not-a-release','{','active',datetime('now'));
        UPDATE applications SET current_deployment_id='corrupt-current' WHERE id='b'`); err != nil {
		t.Fatal(err)
	}
	if err := (&deployments.Service{Repo: r}).RecoverStartup(context.Background(), deployments.FilesystemEvidence{Root: filepath.Join(s.DataRoot, "unused")}); err != nil {
		t.Fatal(err)
	}
	var state, status string
	var current sql.NullString
	if err := s.DB.QueryRow(`SELECT d.state,a.status,a.current_deployment_id
        FROM deployments d JOIN applications a ON a.id=d.app_id WHERE d.id='corrupt-current'`).Scan(&state, &status, &current); err != nil {
		t.Fatal(err)
	}
	if state != string(releases.Failed) || status != "failed" || current.Valid {
		t.Fatalf("corrupt current recovery state=%q status=%q pointer=%#v", state, status, current)
	}
	// A second pass is a no-op durable outcome, including the already-cleared
	// pointer and failed application state.
	if err := (&deployments.Service{Repo: r}).RecoverStartup(context.Background(), deployments.FilesystemEvidence{Root: filepath.Join(s.DataRoot, "unused")}); err != nil {
		t.Fatal(err)
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
	a := deployments.Record{Deployment: releases.Deployment{ID: "a1", AppID: "a", State: releases.Verified, ReleaseHash: "same-release-hash"}, OwnerID: "u", IdempotencyKey: "a1", Manifest: manifest("a")}
	_ = r.Create(context.Background(), a)
	if e := r.CommitActivation(context.Background(), a, nil, "k1"); e != nil {
		t.Fatal(e)
	}
	old, _ := r.Active(context.Background(), "a")
	b := deployments.Record{Deployment: releases.Deployment{ID: "b1", AppID: "a", State: releases.Verified, ReleaseHash: "same-release-hash"}, OwnerID: "u", IdempotencyKey: "b1", Manifest: manifest("a")}
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

func TestDeploymentActivationReplayIsExactAndDoesNotMutatePolicyOrAudit(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	r := DeploymentRepository{Store: s}
	next := deployments.Record{Deployment: releases.Deployment{ID: "replay", AppID: "a", State: releases.Verified}, OwnerID: "u", AppSlug: "a", IdempotencyKey: "upload", Manifest: manifest("a")}
	if err := r.Create(context.Background(), next); err != nil {
		t.Fatal(err)
	}
	if err := r.CommitActivation(context.Background(), next, nil, "activate-once"); err != nil {
		t.Fatal(err)
	}
	active, err := r.Get(context.Background(), "replay")
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := r.ActivationReplay(context.Background(), active, "activate-once"); err != nil || !ok {
		t.Fatalf("exact replay=%v err=%v", ok, err)
	}
	var revision, audits int
	if err := s.DB.QueryRow("SELECT policy_revision FROM applications WHERE id='a'").Scan(&revision); err != nil {
		t.Fatal(err)
	}
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM audit_events WHERE action='deployment.activated'").Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if revision != 2 || audits != 1 {
		t.Fatalf("replay mutated durable state revision=%d audits=%d", revision, audits)
	}
	other := active
	other.ID = "other"
	if _, err := r.ActivationReplay(context.Background(), other, "activate-once"); err == nil {
		t.Fatal("different target accepted for committed key")
	}
	if _, err := r.ActivationReplay(context.Background(), active, "different-key"); err != nil {
		t.Fatalf("unseen key must proceed to normal validation: %v", err)
	}
}

func TestControlActivationExactReplayDoesNotRevokeLiveSessions(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	repo := DeploymentRepository{Store: s}
	next := deployments.Record{Deployment: releases.Deployment{ID: "live-replay", AppID: "a", State: releases.Verified}, OwnerID: "u", AppSlug: "alpha", IdempotencyKey: "upload", Manifest: manifest("alpha")}
	if err := repo.Create(context.Background(), next); err != nil {
		t.Fatal(err)
	}
	service := &deployments.Service{Repo: repo, Gates: deployments.GateFuncs{
		PolicyFunc:      func(context.Context, deployments.Record) bool { return true },
		CertificateFunc: func(context.Context, deployments.Record) bool { return true },
		ProbeFunc:       func(context.Context, deployments.Record) bool { return true },
	}}
	live := &replayRevoker{}
	control := ControlService{Store: s, Deployments: service, Live: live, AppSuffix: "apps.example.test"}
	actor := controlapi.Actor{ID: "u", Active: true}
	if _, err := control.Activate(context.Background(), actor, "alpha", "live-replay", "same-key"); err != nil {
		t.Fatal(err)
	}
	if _, err := control.Activate(context.Background(), actor, "alpha", "live-replay", "same-key"); err != nil {
		t.Fatal(err)
	}
	if live.calls != 1 {
		t.Fatalf("exact replay revoked live sessions %d times", live.calls)
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

func TestDeploymentActivationBindsManifestPolicyAtomically(t *testing.T) {
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

func TestDeploymentV2PublicRequestCannotActivateBeforePublicAuthorizationExists(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	r := DeploymentRepository{Store: s}
	if _, err := s.DB.Exec("INSERT INTO access_policies(app_id,revision,mode,created_at) VALUES('a',1,'private',datetime('now'))"); err != nil {
		t.Fatal(err)
	}
	old := deployments.Record{Deployment: releases.Deployment{ID: "old-private", AppID: "a", State: releases.Active}, OwnerID: "u", AppSlug: "alpha", IdempotencyKey: "old-private", Manifest: releases.Manifest{Version: 1, Name: "alpha"}}
	public := deployments.Record{Deployment: releases.Deployment{ID: "public-request", AppID: "a", State: releases.Verified}, OwnerID: "u", AppSlug: "alpha", IdempotencyKey: "public-request", Manifest: releases.Manifest{Version: 2, Name: "alpha", AccessMode: "public"}}
	for _, record := range []deployments.Record{old, public} {
		if err := r.Create(context.Background(), record); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.DB.Exec("UPDATE applications SET current_deployment_id='old-private' WHERE id='a'"); err != nil {
		t.Fatal(err)
	}
	if s.CandidatePolicyReady(context.Background(), public) {
		t.Fatal("v2 public request passed private-only activation gate")
	}
	if err := r.CommitActivation(context.Background(), public, &old, "public-request"); err == nil {
		t.Fatal("v2 public request activated without public authorization")
	}
	var current string
	if err := s.DB.QueryRow("SELECT current_deployment_id FROM applications WHERE id='a'").Scan(&current); err != nil || current != "old-private" {
		t.Fatalf("public request changed active deployment: %q %v", current, err)
	}
}

func TestPublicActivationAcknowledgementContinuityAndInterveningPrivateTransition(t *testing.T) {
	publicManifest := releases.Manifest{Version: 2, Name: "alpha", AccessMode: "public"}
	t.Run("public to public needs no fresh acknowledgement", func(t *testing.T) {
		s := seeded(t)
		defer s.Close()
		repo := DeploymentRepository{Store: s}
		if _, err := s.DB.Exec("INSERT INTO access_policies(app_id,revision,mode,created_at) VALUES('a',1,'public',datetime('now')); UPDATE public_static_settings SET enabled=1 WHERE singleton=1"); err != nil {
			t.Fatal(err)
		}
		old := deployments.Record{Deployment: releases.Deployment{ID: "old-public", AppID: "a", State: releases.Active}, OwnerID: "u", AppSlug: "alpha", IdempotencyKey: "old-public", Manifest: publicManifest, PublicAcknowledged: true}
		next := deployments.Record{Deployment: releases.Deployment{ID: "next-public", AppID: "a", State: releases.Verified}, OwnerID: "u", AppSlug: "alpha", IdempotencyKey: "next-public", Manifest: publicManifest}
		for _, record := range []deployments.Record{old, next} {
			if err := repo.Create(context.Background(), record); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := s.DB.Exec("UPDATE applications SET current_deployment_id='old-public' WHERE id='a'"); err != nil {
			t.Fatal(err)
		}
		if !s.CandidatePolicyReady(context.Background(), next) {
			t.Fatal("current public posture did not satisfy acknowledgement continuity")
		}
		if err := repo.CommitActivation(context.Background(), next, &old, "public-continuity"); err != nil {
			t.Fatal(err)
		}
		var current, mode string
		var revision int
		if err := s.DB.QueryRow("SELECT a.current_deployment_id,a.policy_revision,p.mode FROM applications a JOIN access_policies p ON p.app_id=a.id AND p.revision=a.policy_revision WHERE a.id='a'").Scan(&current, &revision, &mode); err != nil || current != next.ID || revision != 2 || mode != "public" {
			t.Fatalf("current=%q revision=%d mode=%q err=%v", current, revision, mode, err)
		}
	})

	t.Run("intervening private transition requires fresh acknowledgement", func(t *testing.T) {
		s := seeded(t)
		defer s.Close()
		repo := DeploymentRepository{Store: s}
		if _, err := s.DB.Exec("INSERT INTO access_policies(app_id,revision,mode,created_at) VALUES('a',1,'public',datetime('now')); UPDATE public_static_settings SET enabled=1 WHERE singleton=1"); err != nil {
			t.Fatal(err)
		}
		old := deployments.Record{Deployment: releases.Deployment{ID: "old-public-race", AppID: "a", State: releases.Active}, OwnerID: "u", AppSlug: "alpha", IdempotencyKey: "old-public-race", Manifest: publicManifest, PublicAcknowledged: true}
		next := deployments.Record{Deployment: releases.Deployment{ID: "next-public-race", AppID: "a", State: releases.Verified}, OwnerID: "u", AppSlug: "alpha", IdempotencyKey: "next-public-race", Manifest: publicManifest}
		for _, record := range []deployments.Record{old, next} {
			if err := repo.Create(context.Background(), record); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := s.DB.Exec("UPDATE applications SET current_deployment_id='old-public-race' WHERE id='a'"); err != nil {
			t.Fatal(err)
		}
		if !s.CandidatePolicyReady(context.Background(), next) {
			t.Fatal("initial public continuity missing")
		}
		if _, err := s.DB.Exec("INSERT INTO access_policies(app_id,revision,mode,created_at) VALUES('a',2,'private',datetime('now')); UPDATE applications SET policy_revision=2 WHERE id='a'"); err != nil {
			t.Fatal(err)
		}
		if err := repo.CommitActivation(context.Background(), next, &old, "intervening-private"); err == nil {
			t.Fatal("unacknowledged public candidate activated after private transition")
		}
		var current string
		var revision int
		if err := s.DB.QueryRow("SELECT current_deployment_id,policy_revision FROM applications WHERE id='a'").Scan(&current, &revision); err != nil || current != old.ID || revision != 2 {
			t.Fatalf("current=%q revision=%d err=%v", current, revision, err)
		}
	})
}

func TestPublicToPrivateActivationTransitionsPolicyAtomically(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	repo := DeploymentRepository{Store: s}
	if _, err := s.DB.Exec("INSERT INTO access_policies(app_id,revision,mode,created_at) VALUES('a',1,'public',datetime('now'))"); err != nil {
		t.Fatal(err)
	}
	old := deployments.Record{Deployment: releases.Deployment{ID: "public-old", AppID: "a", State: releases.Active}, OwnerID: "u", AppSlug: "alpha", IdempotencyKey: "public-old", Manifest: releases.Manifest{Version: 2, Name: "alpha", AccessMode: "public"}, PublicAcknowledged: true}
	next := deployments.Record{Deployment: releases.Deployment{ID: "private-next", AppID: "a", State: releases.Verified}, OwnerID: "u", AppSlug: "alpha", IdempotencyKey: "private-next", Manifest: releases.Manifest{Version: 2, Name: "alpha", AccessMode: "private"}}
	for _, record := range []deployments.Record{old, next} {
		if err := repo.Create(context.Background(), record); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.DB.Exec("UPDATE applications SET current_deployment_id='public-old' WHERE id='a'"); err != nil {
		t.Fatal(err)
	}
	if !s.CandidatePolicyReady(context.Background(), next) {
		t.Fatal("private candidate unexpectedly denied")
	}
	if err := repo.CommitActivation(context.Background(), next, &old, "return-private"); err != nil {
		t.Fatal(err)
	}
	var current, mode string
	if err := s.DB.QueryRow("SELECT a.current_deployment_id,p.mode FROM applications a JOIN access_policies p ON p.app_id=a.id AND p.revision=a.policy_revision WHERE a.id='a'").Scan(&current, &mode); err != nil || current != next.ID || mode != "private" {
		t.Fatalf("current=%q mode=%q err=%v", current, mode, err)
	}
}

func TestPublicActivationDenialMatrixPreservesPreviousReleaseAndPolicy(t *testing.T) {
	for _, tc := range []struct {
		name         string
		acknowledged bool
		capability   bool
		gate         string
	}{
		{name: "gate off", acknowledged: true, gate: "off"},
		{name: "gate missing", acknowledged: true, gate: "missing"},
		{name: "unacknowledged broadening", gate: "on"},
		{name: "capability bearing", acknowledged: true, capability: true, gate: "on"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := seeded(t)
			defer s.Close()
			repo := DeploymentRepository{Store: s}
			if _, err := s.DB.Exec("INSERT INTO access_policies(app_id,revision,mode,created_at) VALUES('a',1,'private',datetime('now'))"); err != nil {
				t.Fatal(err)
			}
			switch tc.gate {
			case "on":
				if _, err := s.DB.Exec("UPDATE public_static_settings SET enabled=1 WHERE singleton=1"); err != nil {
					t.Fatal(err)
				}
			case "missing":
				if _, err := s.DB.Exec("DELETE FROM public_static_settings"); err != nil {
					t.Fatal(err)
				}
			}
			old := deployments.Record{Deployment: releases.Deployment{ID: "matrix-old", AppID: "a", State: releases.Active}, OwnerID: "u", AppSlug: "alpha", IdempotencyKey: "matrix-old", Manifest: releases.Manifest{Version: 2, Name: "alpha", AccessMode: "private"}}
			next := deployments.Record{Deployment: releases.Deployment{ID: "matrix-next", AppID: "a", State: releases.Verified}, OwnerID: "u", AppSlug: "alpha", IdempotencyKey: "matrix-next", Manifest: releases.Manifest{Version: 2, Name: "alpha", AccessMode: "public", KV: tc.capability}, PublicAcknowledged: tc.acknowledged}
			for _, record := range []deployments.Record{old, next} {
				if err := repo.Create(context.Background(), record); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.DB.Exec("UPDATE applications SET current_deployment_id='matrix-old' WHERE id='a'"); err != nil {
				t.Fatal(err)
			}
			if s.CandidatePolicyReady(context.Background(), next) {
				t.Fatal("candidate policy unexpectedly ready")
			}
			if err := repo.CommitActivation(context.Background(), next, &old, "matrix-denied"); err == nil {
				t.Fatal("public denial activated")
			}
			var current string
			var revision int
			if err := s.DB.QueryRow("SELECT current_deployment_id,policy_revision FROM applications WHERE id='a'").Scan(&current, &revision); err != nil || current != old.ID || revision != 1 {
				t.Fatalf("current=%q revision=%d err=%v", current, revision, err)
			}
		})
	}
}

func TestDeploymentActivationAuditFailurePreservesReleaseAndPolicy(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	r := DeploymentRepository{Store: s}
	if _, err := s.DB.Exec("INSERT INTO access_policies(app_id,revision,mode,created_at) VALUES('a',1,'private',datetime('now'))"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec("UPDATE public_static_settings SET enabled=1 WHERE singleton=1"); err != nil {
		t.Fatal(err)
	}
	old := deployments.Record{Deployment: releases.Deployment{ID: "old-audit", AppID: "a", State: releases.Active}, OwnerID: "u", AppSlug: "alpha", IdempotencyKey: "old-audit", Manifest: releases.Manifest{Version: 1, Name: "alpha"}}
	next := deployments.Record{Deployment: releases.Deployment{ID: "next-audit", AppID: "a", State: releases.Verified}, OwnerID: "u", AppSlug: "alpha", IdempotencyKey: "next-audit", Manifest: releases.Manifest{Version: 2, Name: "alpha", AccessMode: "public", Emails: []string{"alice@example.com"}}, PublicAcknowledged: true}
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

func TestPublicActivationCommitFailurePreservesReleaseAndPolicy(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	repo := DeploymentRepository{Store: s}
	if _, err := s.DB.Exec("INSERT INTO access_policies(app_id,revision,mode,created_at) VALUES('a',1,'private',datetime('now')); UPDATE public_static_settings SET enabled=1 WHERE singleton=1"); err != nil {
		t.Fatal(err)
	}
	old := deployments.Record{Deployment: releases.Deployment{ID: "public-commit-old", AppID: "a", State: releases.Active}, OwnerID: "u", AppSlug: "alpha", IdempotencyKey: "public-commit-old", Manifest: releases.Manifest{Version: 2, Name: "alpha", AccessMode: "private"}}
	next := deployments.Record{Deployment: releases.Deployment{ID: "public-commit-next", AppID: "a", State: releases.Verified}, OwnerID: "u", AppSlug: "alpha", IdempotencyKey: "public-commit-next", Manifest: releases.Manifest{Version: 2, Name: "alpha", AccessMode: "public"}, PublicAcknowledged: true}
	for _, record := range []deployments.Record{old, next} {
		if err := repo.Create(context.Background(), record); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.DB.Exec("UPDATE applications SET current_deployment_id='public-commit-old' WHERE id='a'; CREATE TRIGGER deny_public_activation BEFORE UPDATE OF state ON deployments WHEN NEW.id='public-commit-next' BEGIN SELECT RAISE(ABORT,'commit denied'); END"); err != nil {
		t.Fatal(err)
	}
	if err := repo.CommitActivation(context.Background(), next, &old, "public-commit-fails"); err == nil {
		t.Fatal("commit failure accepted")
	}
	var current, mode string
	var revision, audits int
	if err := s.DB.QueryRow("SELECT a.current_deployment_id,a.policy_revision,p.mode FROM applications a JOIN access_policies p ON p.app_id=a.id AND p.revision=a.policy_revision WHERE a.id='a'").Scan(&current, &revision, &mode); err != nil || current != old.ID || revision != 1 || mode != "private" {
		t.Fatalf("current=%q revision=%d mode=%q err=%v", current, revision, mode, err)
	}
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM audit_events WHERE request_id='public-commit-fails'").Scan(&audits); err != nil || audits != 0 {
		t.Fatalf("audits=%d err=%v", audits, err)
	}
}
