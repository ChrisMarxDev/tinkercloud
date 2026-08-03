package update

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

var (
	ErrSignature = errors.New("artifact signature invalid")
	ErrDigest    = errors.New("artifact digest mismatch")
	ErrHealth    = errors.New("update health gate failed")
)

type Artifact struct {
	Version, API, Schema string
	Bytes, Signature     []byte
	Digest               [32]byte
}

func (a Artifact) signed() []byte {
	return []byte(a.Version + "\n" + a.API + "\n" + a.Schema + "\n" + hex.EncodeToString(a.Digest[:]))
}

func Verify(pub ed25519.PublicKey, a Artifact) error {
	if sha256.Sum256(a.Bytes) != a.Digest {
		return ErrDigest
	}
	if !ed25519.Verify(pub, a.signed(), a.Signature) {
		return ErrSignature
	}
	return nil
}

type State string

const (
	RollbackReady  State = "rollback-ready"
	Applying       State = "applying"
	Healthy        State = "healthy"
	RolledBack     State = "rolled-back"
	RollbackFailed State = "rollback-failed"
)

type Installer interface {
	Snapshot(context.Context) error
	Apply(context.Context, Artifact) error
	Restore(context.Context) error
}

// Committer removes the bounded rollback snapshot after a terminal recovery
// state. Installers that retain no durable snapshot need not implement it.
type Committer interface{ Commit(context.Context) error }

type Health interface{ Check(context.Context) error }

// Restarter changes the running service to the just-installed binary. It is
// deliberately an interface: update orchestration must be testable without a
// system service manager and the same operation is used after rollback.
type Restarter interface{ Restart(context.Context) error }

func Apply(ctx context.Context, pub ed25519.PublicKey, a Artifact, i Installer, h Health) (State, error) {
	if err := Verify(pub, a); err != nil {
		return "", err
	}
	if err := i.Snapshot(ctx); err != nil {
		return "", err
	}
	if err := i.Apply(ctx, a); err != nil {
		if restoreErr := i.Restore(ctx); restoreErr != nil {
			return RollbackFailed, restoreErr
		}
		return RolledBack, err
	}
	if err := h.Check(ctx); err != nil {
		if restoreErr := i.Restore(ctx); restoreErr != nil {
			return RollbackFailed, restoreErr
		}
		return RolledBack, ErrHealth
	}
	return Healthy, nil
}

// ApplyAfterRestart keeps rollback state until the replacement has restarted
// and every independent health gate has passed. A failed restart or probe
// restores the old binary and attempts to restart that old service too.
func ApplyAfterRestart(ctx context.Context, pub ed25519.PublicKey, a Artifact, i Installer, r Restarter, checks ...Health) (State, error) {
	if err := Verify(pub, a); err != nil {
		return "", err
	}
	if err := i.Snapshot(ctx); err != nil {
		return "", err
	}
	if err := i.Apply(ctx, a); err != nil {
		return restoreAndRestart(ctx, i, r, err)
	}
	if err := r.Restart(ctx); err != nil {
		return restoreAndRestart(ctx, i, r, ErrHealth)
	}
	for _, check := range checks {
		if check == nil || check.Check(ctx) != nil {
			return restoreAndRestart(ctx, i, r, ErrHealth)
		}
	}
	return Healthy, nil
}

func restoreAndRestart(ctx context.Context, i Installer, r Restarter, cause error) (State, error) {
	if err := i.Restore(ctx); err != nil {
		return RollbackFailed, err
	}
	if err := r.Restart(ctx); err != nil {
		return RollbackFailed, err
	}
	// The previous binary was healthy before replacement. Once it has been
	// restored and restarted, the snapshot no longer represents pending
	// operator recovery. Clear it so ordinary doctor remains an accurate
	// fail-closed signal; retain it if either recovery or cleanup fails.
	if committer, ok := i.(Committer); ok {
		if err := committer.Commit(ctx); err != nil {
			return RollbackFailed, err
		}
	}
	return RolledBack, cause
}
