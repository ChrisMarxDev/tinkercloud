package update

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"errors"
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
