package compose

// This file is the single composition seam for M5 resource limits. Server
// command wiring calls it once; app/static read handlers intentionally receive
// none of its write gate.

import (
	"github.com/ChrisMarxDev/tinkercloud/internal/archive"
	"github.com/ChrisMarxDev/tinkercloud/internal/blob"
	"github.com/ChrisMarxDev/tinkercloud/internal/config"
	"github.com/ChrisMarxDev/tinkercloud/internal/deployments"
	"github.com/ChrisMarxDev/tinkercloud/internal/operations"
	"github.com/ChrisMarxDev/tinkercloud/internal/persistence"
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

func (r ResourceControls) KVRepository(apps *persistence.AppDatabaseManager) persistence.KVRepository {
	return persistence.KVRepository{Apps: apps, WriteGate: r.Gate}
}
func (r ResourceControls) BlobRepository(store *persistence.SQLiteStore) *persistence.BlobRepository {
	return &persistence.BlobRepository{Store: store, Bytes: blob.LocalStore{Root: store.DataRoot}, WriteGate: r.Gate}
}

func (r ResourceControls) ConfigureControl(service *persistence.ControlService) {
	if service == nil {
		return
	}
	service.WriteGate = r.Gate
	service.AppsPerDeployer = r.Limits.AppsPerDeployer
}
