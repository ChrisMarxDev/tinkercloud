// Package deployments is the control-domain deployment workflow. It is
// transport-neutral: callers supply a typed authenticated actor and adapters
// supply private staging, policy/TLS/probe, and transactional persistence.
package deployments

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"github.com/tinyhost/tiny/internal/archive"
	"github.com/tinyhost/tiny/internal/operations"
	"github.com/tinyhost/tiny/internal/releases"
	"github.com/tinyhost/tiny/internal/uploads"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	ErrDenied      = errors.New("deployment denied")
	ErrIdempotency = errors.New("idempotency conflict")
	ErrProbe       = errors.New("protected probe failed")
	ErrRateLimited = errors.New("deployment rate limited")
)

type Actor struct {
	ID     string
	Active bool
}
type Record struct {
	releases.Deployment
	OwnerID, AppSlug, IdempotencyKey, Staging string
	ArchiveHash                               [32]byte
	Manifest                                  releases.Manifest
	Files                                     []releases.File
	// RecoveryCorrupt is set only by the database-led recovery reader when a
	// durable row cannot be safely reconstructed. It deliberately remains part
	// of the record passed to recovery rather than turning one bad historical
	// row into a startup-wide repository error.
	RecoveryCorrupt bool
}
type Repository interface {
	Create(context.Context, Record) error
	Get(context.Context, string) (Record, error)
	Active(context.Context, string) (*Record, error)
	CommitActivation(context.Context, Record, *Record, string) error
	Fail(context.Context, string) error
}

// RecoveryRepository is the single database-led startup seam. It returns
// durable records only; callers never discover releases by scanning disk.
type RecoveryRepository interface {
	RecoveryRecords(context.Context) ([]Record, error)
	ApplyRecovery(context.Context, string, releases.State) error
}

// EvidenceVerifier checks only the content-addressed location derived from a
// durable record and returns no filesystem path to callers.
type EvidenceVerifier interface {
	Valid(context.Context, Record) bool
}

// RecoverStartup deterministically classifies every durable deployment state.
// It never resumes incomplete work and is idempotent: applying an already
// classified state is a no-op in the repository.
func (s *Service) RecoverStartup(ctx context.Context, evidence EvidenceVerifier) error {
	r, ok := s.Repo.(RecoveryRepository)
	if !ok || evidence == nil {
		return ErrDenied
	}
	records, err := r.RecoveryRecords(ctx)
	if err != nil {
		return err
	}
	for _, record := range records {
		valid := evidence.Valid(ctx, record)
		next := Recover(record, valid)
		if next != record.State {
			if err := r.ApplyRecovery(ctx, record.ID, next); err != nil {
				return err
			}
		}
	}
	return nil
}

// AttemptCounter is optional so pure in-memory/unit repositories remain small.
// Production persistence supplies it to enforce the per-deployer hourly bound.
type AttemptCounter interface {
	DeploymentAttempts(context.Context, string, time.Time) (int, error)
}

// Recover classifies interrupted durable deployment records. Persistence adapters
// may retry a verified candidate; incomplete filesystem work is failed closed.
func Recover(r Record, releaseExists bool) releases.State {
	if r.RecoveryCorrupt {
		return releases.Failed
	}
	switch r.State {
	case releases.Uploading, releases.Uploaded, releases.Validating, releases.Staged:
		return releases.Failed
	case releases.Verified, releases.Active, releases.Superseded:
		if !releaseExists {
			return releases.Failed
		}
		return r.State
	case releases.Rejected, releases.Failed:
		return r.State
	default:
		return releases.Failed
	}
}

type Gates interface {
	Policy(context.Context, Record) bool
	Certificate(context.Context, Record) bool
	Probe(context.Context, Record) bool
}
type GateFuncs struct {
	PolicyFunc, CertificateFunc, ProbeFunc func(context.Context, Record) bool
}

func (g GateFuncs) Policy(ctx context.Context, r Record) bool {
	return g.PolicyFunc != nil && g.PolicyFunc(ctx, r)
}
func (g GateFuncs) Certificate(ctx context.Context, r Record) bool {
	return g.CertificateFunc != nil && g.CertificateFunc(ctx, r)
}
func (g GateFuncs) Probe(ctx context.Context, r Record) bool {
	return g.ProbeFunc != nil && g.ProbeFunc(ctx, r)
}

type Service struct {
	Repo         Repository
	Gates        Gates
	Limits       archive.Limits
	UploadLimit  int64
	Root         string
	WriteGate    operations.WriteGate
	AttemptLimit int
	// CapabilityReady is an optional activation gate for manifest-requested
	// platform capabilities. It runs before the immutable release pointer is
	// committed, so an unavailable operator grant preserves the old release.
	CapabilityReady func(context.Context, Record) bool
	Now             func() time.Time
	locks           sync.Map
}

func (s *Service) lock(app string) func() {
	v, _ := s.locks.LoadOrStore(app, &sync.Mutex{})
	m := v.(*sync.Mutex)
	m.Lock()
	return m.Unlock
}
func (s *Service) reject(ctx context.Context, r *Record) error {
	if e := releases.Transition(r.State, releases.Rejected); e != nil {
		return e
	}
	r.State = releases.Rejected
	return s.Repo.Create(ctx, *r)
}
func (s *Service) Create(ctx context.Context, a Actor, app, slug, id, key string) (Record, error) {
	if !a.Active || a.ID == "" || key == "" || !releases.ValidSlug(slug) {
		return Record{}, ErrDenied
	}
	if s.WriteGate != nil {
		if err := s.WriteGate.AllowWrite(ctx, operations.WriteDeployment); err != nil {
			return Record{}, err
		}
	}
	if s.AttemptLimit > 0 {
		if counter, ok := s.Repo.(AttemptCounter); ok {
			now := time.Now()
			if s.Now != nil {
				now = s.Now()
			}
			count, err := counter.DeploymentAttempts(ctx, a.ID, now.Add(-time.Hour))
			if err != nil {
				return Record{}, ErrDenied
			}
			if count >= s.AttemptLimit {
				return Record{}, ErrRateLimited
			}
		}
	}
	r := Record{Deployment: releases.Deployment{ID: id, AppID: app, State: releases.Uploading}, OwnerID: a.ID, AppSlug: slug, IdempotencyKey: key}
	if err := s.Repo.Create(ctx, r); err != nil {
		return Record{}, err
	}
	return r, nil
}
func (s *Service) Upload(ctx context.Context, a Actor, id, contentType string, src io.Reader) error {
	r, e := s.Repo.Get(ctx, id)
	if e != nil || r.OwnerID != a.ID || !a.Active {
		return ErrDenied
	}
	if r.State != releases.Uploading {
		return releases.ErrTransition
	}
	dir, e := os.MkdirTemp(s.Root, "staging-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(dir)
	name := "upload.zip"
	if contentType == "application/gzip" {
		name = "upload.tar.gz"
	} else if contentType != "application/zip" {
		os.RemoveAll(dir)
		return ErrDenied
	}
	f, e := os.Create(filepath.Join(dir, name))
	if e != nil {
		return e
	}
	res, e := uploads.Copy(ctx, f, src, s.UploadLimit)
	ce := f.Close()
	if e != nil {
		os.RemoveAll(dir)
		if errors.Is(e, archive.ErrUnsafe) {
			_ = s.reject(ctx, &r)
		} else {
			_ = s.Repo.Fail(ctx, id)
		}
		return e
	}
	if ce != nil {
		return ce
	}
	if e = releases.Transition(r.State, releases.Uploaded); e != nil {
		return e
	}
	r.State = releases.Uploaded
	r.ArchiveHash = res.SHA256
	if e = s.Repo.Create(ctx, r); e != nil {
		return e
	}
	if e = releases.Transition(r.State, releases.Validating); e != nil {
		return e
	}
	r.State = releases.Validating
	if e = s.Repo.Create(ctx, r); e != nil {
		return e
	}
	z, e := os.Open(filepath.Join(dir, name))
	if e != nil {
		return e
	}
	st, e := z.Stat()
	if e == nil {
		if contentType == "application/zip" {
			zr, x := zip.NewReader(z, st.Size())
			if x == nil {
				e = archive.ExtractZip(ctx, zr, filepath.Join(dir, "release"), s.Limits)
			} else {
				e = x
			}
		} else {
			e = archive.ExtractTarGz(ctx, z, filepath.Join(dir, "release"), s.Limits)
		}
	}
	z.Close()
	if e != nil {
		os.RemoveAll(dir)
		_ = s.reject(ctx, &r)
		return e
	}
	validated, e := ValidateRelease(filepath.Join(dir, "release"), r.AppSlug)
	if e != nil {
		_ = s.reject(ctx, &r)
		return e
	}
	if e = releases.Transition(r.State, releases.Staged); e != nil {
		return e
	}
	r.State = releases.Staged
	r.Manifest = validated
	if e = s.Repo.Create(ctx, r); e != nil {
		return e
	}
	_, manifest, e := releases.Finalize(s.Root, r.AppID, filepath.Join(dir, "release"))
	if e != nil {
		_ = s.Repo.Fail(ctx, id)
		return e
	}
	if e = releases.Transition(r.State, releases.Verified); e != nil {
		return e
	}
	r.State = releases.Verified
	r.Staging = dir
	r.ReleaseHash = manifest.Hash
	r.Files = manifest.Files
	r.Manifest = validated
	r.ArchiveHash = res.SHA256
	return s.Repo.Create(ctx, r)
}
func (s *Service) Activate(ctx context.Context, a Actor, id, requestID string) error {
	if requestID == "" {
		return ErrDenied
	}
	r, e := s.Repo.Get(ctx, id)
	if e != nil || r.OwnerID != a.ID || !a.Active {
		return ErrDenied
	}
	unlock := s.lock(r.AppID)
	defer unlock()
	old, e := s.Repo.Active(ctx, r.AppID)
	if e != nil {
		return e
	}
	// A release which declares a protected external capability must not become
	// active merely because composition forgot to supply its verifier. This is
	// intentionally stricter than optional non-LLM capability behavior.
	if !s.Gates.Policy(ctx, r) || !s.Gates.Certificate(ctx, r) || (r.Manifest.LLMChat && (s.CapabilityReady == nil || !s.CapabilityReady(ctx, r))) {
		return releases.ErrTransition
	}
	if !s.Gates.Probe(ctx, r) {
		return ErrProbe
	}
	if e := r.CanActivate(releases.ActivationRequirements{PolicyReady: true, CertificateReady: true, DenialProbePassed: true}); e != nil {
		return e
	}
	return s.Repo.CommitActivation(ctx, r, old, requestID)
}

// MemoryRepository is deliberately test-only style infrastructure that models
// transactional ownership/current-pointer behavior and injectable failures.
type MemoryRepository struct {
	mu         sync.Mutex
	Records    map[string]Record
	Current    map[string]string
	FailCommit error
}

func (m *MemoryRepository) Create(_ context.Context, r Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Records == nil {
		m.Records = map[string]Record{}
		m.Current = map[string]string{}
	}
	if old, ok := m.Records[r.ID]; ok && old.IdempotencyKey != r.IdempotencyKey {
		return ErrIdempotency
	}
	m.Records[r.ID] = r
	return nil
}
func (m *MemoryRepository) Get(_ context.Context, id string) (Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.Records[id]
	if !ok {
		return Record{}, os.ErrNotExist
	}
	return r, nil
}
func (m *MemoryRepository) Active(_ context.Context, app string) (*Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := m.Current[app]
	if id == "" {
		return nil, nil
	}
	r := m.Records[id]
	return &r, nil
}
func (m *MemoryRepository) CommitActivation(_ context.Context, next Record, old *Record, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.FailCommit != nil {
		return m.FailCommit
	}
	next.State = releases.Active
	m.Records[next.ID] = next
	if old != nil {
		o := *old
		o.State = releases.Superseded
		m.Records[o.ID] = o
	}
	m.Current[next.AppID] = next.ID
	return nil
}
func (m *MemoryRepository) Fail(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.Records[id]
	r.State = releases.Failed
	m.Records[id] = r
	return nil
}
func (m *MemoryRepository) RecoveryRecords(_ context.Context) ([]Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Record, 0, len(m.Records))
	for _, r := range m.Records {
		out = append(out, r)
	}
	return out, nil
}
func (m *MemoryRepository) ApplyRecovery(_ context.Context, id string, next releases.State) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.Records[id]
	if !ok {
		return os.ErrNotExist
	}
	r.State = next
	m.Records[id] = r
	if next == releases.Failed && m.Current[r.AppID] == id {
		delete(m.Current, r.AppID)
	}
	return nil
}
func HashFile(p string) ([32]byte, error) {
	f, e := os.Open(p)
	if e != nil {
		return [32]byte{}, e
	}
	defer f.Close()
	b, e := io.ReadAll(f)
	return sha256.Sum256(b), e
}

var _ = bytes.MinRead
