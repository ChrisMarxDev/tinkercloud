package update

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type fake struct {
	snap, apply, restore bool
	failApply            bool
}

func (f *fake) Snapshot(context.Context) error { f.snap = true; return nil }
func (f *fake) Apply(context.Context, Artifact) error {
	f.apply = true
	if f.failApply {
		return errors.New("apply")
	}
	return nil
}
func (f *fake) Restore(context.Context) error { f.restore = true; return nil }

type health bool

func (h health) Check(context.Context) error {
	if !h {
		return errors.New("bad")
	}
	return nil
}

type restarter struct {
	calls  int
	failOn int
}

func (r *restarter) Restart(context.Context) error {
	r.calls++
	if r.calls == r.failOn {
		return errors.New("restart")
	}
	return nil
}

type commitFailInstaller struct {
	fake
	commitCalled     bool
	commitErr        error
	rollbackSnapshot bool
}

func (f *commitFailInstaller) Snapshot(ctx context.Context) error {
	f.rollbackSnapshot = true
	return f.fake.Snapshot(ctx)
}

func (f *commitFailInstaller) Commit(context.Context) error {
	f.commitCalled = true
	return f.commitErr
}
func TestHealthFailureRollsBack(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	a := Artifact{Bytes: []byte("next")}
	a.Digest = sha256.Sum256(a.Bytes)
	a.Signature = ed25519.Sign(priv, a.signed())
	f := &fake{}
	s, e := Apply(context.Background(), pub, a, f, health(false))
	if e != ErrHealth || s != RolledBack || !f.restore {
		t.Fatal(s, e, f)
	}
}
func TestBadSignatureNeverSnapshots(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	f := &fake{}
	_, e := Apply(context.Background(), pub, Artifact{Bytes: []byte("x")}, f, health(true))
	if e != ErrDigest || f.snap {
		t.Fatal(e)
	}
}

func TestProbeFailureRestoresAndRestartsPriorService(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	a := Artifact{Bytes: []byte("next")}
	a.Digest = sha256.Sum256(a.Bytes)
	a.Signature = ed25519.Sign(priv, a.signed())
	f := &fake{}
	r := &restarter{}
	s, err := ApplyAfterRestart(context.Background(), pub, a, f, r, health(true), health(false))
	if err != ErrHealth || s != RolledBack || !f.restore || r.calls != 2 {
		t.Fatal(s, err, f, r.calls)
	}
}

func TestRestartFailureRestoresAndRetriesPriorService(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	a := Artifact{Bytes: []byte("next")}
	a.Digest = sha256.Sum256(a.Bytes)
	a.Signature = ed25519.Sign(priv, a.signed())
	f := &fake{}
	r := &restarter{failOn: 1}
	s, err := ApplyAfterRestart(context.Background(), pub, a, f, r, health(true))
	if err != ErrHealth || s != RolledBack || !f.restore || r.calls != 2 {
		t.Fatal(s, err, f, r.calls)
	}
}

func TestFailedCandidateRollbackClearsRecoveredSnapshot(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "tinkercloud")
	if err := os.WriteFile(target, []byte("healthy binary"), 0755); err != nil {
		t.Fatal(err)
	}
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	a := Artifact{Bytes: []byte("failed candidate")}
	a.Digest = sha256.Sum256(a.Bytes)
	a.Signature = ed25519.Sign(priv, a.signed())
	installer := FileInstaller{Target: target, RollbackDir: filepath.Join(dir, "update-rollback")}

	state, err := ApplyAfterRestart(context.Background(), pub, a, installer, &restarter{}, health(false))
	if err != ErrHealth || state != RolledBack {
		t.Fatalf("state=%q err=%v", state, err)
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "healthy binary" {
		t.Fatalf("restored binary=%q err=%v", got, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "update-rollback")); !os.IsNotExist(err) {
		t.Fatalf("recovered rollback snapshot remains: %v", err)
	}
}

func TestFailedCandidateRollbackRetainsRecoveryStateWhenCleanupFails(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	a := Artifact{Bytes: []byte("failed candidate")}
	a.Digest = sha256.Sum256(a.Bytes)
	a.Signature = ed25519.Sign(priv, a.signed())
	cleanupErr := errors.New("cleanup failed")
	installer := &commitFailInstaller{commitErr: cleanupErr}

	state, err := ApplyAfterRestart(context.Background(), pub, a, installer, &restarter{}, health(false))
	if state != RollbackFailed || !errors.Is(err, cleanupErr) {
		t.Fatalf("state=%q err=%v", state, err)
	}
	if !installer.restore || !installer.commitCalled || !installer.rollbackSnapshot {
		t.Fatalf("restore=%t commit=%t snapshot=%t", installer.restore, installer.commitCalled, installer.rollbackSnapshot)
	}
}
