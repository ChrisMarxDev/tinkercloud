package deployments

import (
	"context"
	"errors"
	"github.com/tinyhost/tiny/internal/releases"
	"sync"
)

var ErrGate = errors.New("activation gate failed")

type ActivationStore struct {
	mu      sync.Mutex
	Records map[string]Record
	Current map[string]string
}

func (s *ActivationStore) Activate(_ context.Context, actor, id string, r releases.ActivationRequirements, probe func() bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.Records[id]
	if !ok || d.OwnerID != actor || d.State != releases.Verified {
		return ErrDenied
	}
	if !r.PolicyReady || !r.CertificateReady || !r.DenialProbePassed {
		return ErrGate
	}
	if s.Current == nil {
		s.Current = map[string]string{}
	}
	old := s.Current[d.AppID]
	s.Current[d.AppID] = id
	d.State = releases.Active
	s.Records[id] = d
	if !probe() {
		s.Current[d.AppID] = old
		d.State = releases.Failed
		s.Records[id] = d
		return ErrProbe
	}
	if old != "" {
		o := s.Records[old]
		o.State = releases.Superseded
		s.Records[old] = o
	}
	return nil
}
