// Package releases implements immutable deployment state transitions and atomic
// activation planning. Filesystem/persistence adapters provide durable commits.
package releases

import (
	"errors"
	"fmt"
)

type State string

const (
	Uploading  State = "uploading"
	Uploaded   State = "uploaded"
	Validating State = "validating"
	Staged     State = "staged"
	Verified   State = "verified"
	Active     State = "active"
	Superseded State = "superseded"
	Rejected   State = "rejected"
	Failed     State = "failed"
)

var ErrTransition = errors.New("invalid deployment transition")

func (s State) Terminal() bool { return s == Rejected || s == Failed || s == Superseded }
func Transition(from, to State) error {
	ok := map[State]map[State]bool{Uploading: {Uploaded: true, Failed: true}, Uploaded: {Validating: true, Rejected: true}, Validating: {Staged: true, Rejected: true}, Staged: {Verified: true, Failed: true}, Verified: {Active: true, Failed: true}, Active: {Superseded: true}}
	if !ok[from][to] {
		return fmt.Errorf("%w: %s to %s", ErrTransition, from, to)
	}
	return nil
}

type Deployment struct {
	ID, AppID, ReleaseHash string
	State                  State
}
type ActivationRequirements struct{ PolicyReady, CertificateReady, DenialProbePassed bool }

func (d Deployment) CanActivate(r ActivationRequirements) error {
	if d.State != Verified {
		return ErrTransition
	}
	if !r.PolicyReady || !r.CertificateReady || !r.DenialProbePassed {
		return errors.New("activation verification incomplete")
	}
	return nil
}

// PlanActivation produces the only valid in-memory transition set. A repository
// must atomically persist the app's current deployment pointer plus these states.
func PlanActivation(previous *Deployment, next Deployment, r ActivationRequirements) ([]Deployment, error) {
	if err := next.CanActivate(r); err != nil {
		return nil, err
	}
	next.State = Active
	out := []Deployment{next}
	if previous != nil {
		if previous.AppID != next.AppID || previous.State != Active {
			return nil, ErrTransition
		}
		old := *previous
		old.State = Superseded
		out = append(out, old)
	}
	return out, nil
}
