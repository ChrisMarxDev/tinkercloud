package compose

// This file is the single composition seam for M5 resource limits. Server
// command wiring calls it once; app/static read handlers intentionally receive
// none of its write gate.

import (
	"github.com/tinyhost/tiny/internal/archive"
	"github.com/tinyhost/tiny/internal/config"
	"github.com/tinyhost/tiny/internal/deployments"
	"github.com/tinyhost/tiny/internal/operations"
	"github.com/tinyhost/tiny/internal/persistence"
)

type ResourceControls struct {
	Limits config.ResourceLimits
	Gate   operations.WriteGate
}

func NewResourceControls(cfg config.Config, source operations.DiskSource) (ResourceControls, error) {
	limits := cfg.EffectiveLimits()
	if err := limits.Validate(); err != nil {
		return ResourceControls{}, err
	}
	return ResourceControls{Limits: limits, Gate: operations.DiskWriteGate{Source: source, Watermarks: operations.Watermarks{Warning: limits.DiskWarningPercent, Stop: limits.DiskStopPercent}}}, nil
}

func (r ResourceControls) ArchiveLimits() archive.Limits {
	return archive.Limits{MaxEntries: r.Limits.FilesPerRelease, MaxDepth: 32, MaxExpanded: r.Limits.ExpandedReleaseBytes, MaxFile: r.Limits.SingleFileBytes}
}

func (r ResourceControls) ConfigureDeployments(service *deployments.Service) {
	if service == nil {
		return
	}
	service.WriteGate = r.Gate
	service.UploadLimit = r.Limits.ArchiveUploadBytes
	service.Limits = r.ArchiveLimits()
	service.AttemptLimit = r.Limits.DeploymentAttemptsPerHour
}

func (r ResourceControls) KVRepository(store *persistence.SQLiteStore) persistence.KVRepository {
	return persistence.KVRepository{Store: store, WriteGate: r.Gate}
}

func (r ResourceControls) ConfigureControl(service *persistence.ControlService) {
	if service == nil {
		return
	}
	service.WriteGate = r.Gate
	service.AppsPerDeployer = r.Limits.AppsPerDeployer
}
