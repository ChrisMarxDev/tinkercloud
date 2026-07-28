package operations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"syscall"
)

var (
	ErrNotRoot       = errors.New("local root authority required")
	ErrWriteDisabled = errors.New("writes disabled by disk watermark")
)

type Check struct {
	Name    string
	Healthy bool
	Detail  string
	Secret  bool
}

func (c Check) SafeDetail() string {
	if c.Secret {
		return "redacted"
	}
	return strings.TrimSpace(c.Detail)
}

type Doctor interface{ Check(context.Context) []Check }
type Disk struct{ Used, Total uint64 }

func (d Disk) Percent() uint64 {
	if d.Total == 0 {
		return 100
	}
	return d.Used * 100 / d.Total
}

type Watermarks struct{ Warning, Stop uint64 }

// Validate rejects ambiguous or unsafe watermark configurations.  A stopped
// writer must always have a finite warning before it, otherwise an operator
// cannot take corrective action before requests begin failing.
func (w Watermarks) Validate() error {
	if w.Warning == 0 || w.Stop == 0 || w.Warning >= w.Stop || w.Stop > 100 {
		return errors.New("invalid disk watermarks")
	}
	return nil
}

func (w Watermarks) AllowsWrite(d Disk) bool { return w.Validate() == nil && d.Percent() < w.Stop }

// WriteKind is deliberately narrow: disk pressure may stop growth, but it
// must never stop a security revocation or an already-authorized static read.
type WriteKind string

const (
	WriteAppCreate  WriteKind = "app_create"
	WriteDeployment WriteKind = "deployment"
	WriteKV         WriteKind = "kv"
	WriteBlob       WriteKind = "blob"
)

// DiskSource reports the data-volume usage.  It is an injected seam so errors
// and watermark boundaries are testable without relying on the host filesystem.
type DiskSource interface {
	Disk(context.Context) (Disk, error)
}

// WriteGate is used only by growth paths. Implementations fail closed when
// usage cannot be established; callers intentionally do not invoke it for
// static reads, authentication, policy changes, or revocations.
type WriteGate interface {
	AllowWrite(context.Context, WriteKind) error
}

type DiskWriteGate struct {
	Source     DiskSource
	Watermarks Watermarks
}

func (g DiskWriteGate) AllowWrite(ctx context.Context, _ WriteKind) error {
	if g.Source == nil || g.Watermarks.Validate() != nil {
		return ErrWriteDisabled
	}
	disk, err := g.Source.Disk(ctx)
	if err != nil || !g.Watermarks.AllowsWrite(disk) {
		return ErrWriteDisabled
	}
	return nil
}

// StaticDiskSource is the production data-directory adapter. It returns an
// error for an unavailable path, which the gate turns into a safe denial.
type StaticDiskSource struct{ Path string }

func (s StaticDiskSource) Disk(_ context.Context) (Disk, error) {
	if strings.TrimSpace(s.Path) == "" {
		return Disk{}, errors.New("empty disk path")
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs(s.Path, &stat); err != nil {
		return Disk{}, err
	}
	// Blocks and Bavail are filesystem units. Keep the calculation in units to
	// avoid a byte multiplication overflow on very large volumes.
	if stat.Blocks == 0 || stat.Bavail > stat.Blocks {
		return Disk{}, errors.New("invalid filesystem statistics")
	}
	return Disk{Used: uint64(stat.Blocks - stat.Bavail), Total: uint64(stat.Blocks)}, nil
}

type RootGuard interface{ IsLocalRoot(context.Context) bool }

func RequireRoot(ctx context.Context, g RootGuard) error {
	if g == nil || !g.IsLocalRoot(ctx) {
		return ErrNotRoot
	}
	return nil
}

type InitStep string

const (
	InitPreflight InitStep = "preflight"
	InitPaths     InitStep = "paths"
	InitDatabase  InitStep = "database"
	InitOperator  InitStep = "operator"
	InitService   InitStep = "service"
	InitVerified  InitStep = "verified"
)

var ordered = []InitStep{InitPreflight, InitPaths, InitDatabase, InitOperator, InitService, InitVerified}

type InitState struct {
	Completed map[InitStep]bool `json:"completed"`
}

func ParseInitState(data []byte) (InitState, error) {
	var state InitState
	if len(data) == 0 {
		return state, nil
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return InitState{}, fmt.Errorf("invalid init state: %w", err)
	}
	for step := range state.Completed {
		known := false
		for _, allowed := range ordered {
			if step == allowed {
				known = true
				break
			}
		}
		if !known || !state.Completed[step] {
			return InitState{}, errors.New("invalid init state")
		}
	}
	// A persisted state cannot have gaps. This prevents a partial or hand-edited
	// state file from silently skipping a security-relevant step.
	seenIncomplete := false
	for _, step := range ordered {
		if !state.Completed[step] {
			seenIncomplete = true
			continue
		}
		if seenIncomplete {
			return InitState{}, errors.New("invalid init state ordering")
		}
	}
	return state, nil
}

func (s InitState) MarshalJSONState() ([]byte, error) {
	b, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	if _, err := ParseInitState(b); err != nil {
		return nil, err
	}
	return b, nil
}

func (s InitState) Next() InitStep {
	for _, v := range ordered {
		if !s.Completed[v] {
			return v
		}
	}
	return ""
}
func (s *InitState) Complete(v InitStep) error {
	if s.Next() != v {
		return errors.New("init step out of order")
	}
	if s.Completed == nil {
		s.Completed = map[InitStep]bool{}
	}
	s.Completed[v] = true
	return nil
}
