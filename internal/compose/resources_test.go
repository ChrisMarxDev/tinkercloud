package compose

import (
	"context"
	"errors"
	"testing"

	"github.com/ChrisMarxDev/tinkercloud/internal/config"
	"github.com/ChrisMarxDev/tinkercloud/internal/deployments"
	"github.com/ChrisMarxDev/tinkercloud/internal/operations"
)

type resourceDisk struct{ d operations.Disk }

func (s resourceDisk) Disk(context.Context) (operations.Disk, error) { return s.d, nil }

func TestResourceControlsApplyConfiguredLimitsAndWatermark(t *testing.T) {
	cfg := config.Config{Limits: config.ResourceLimits{AppsPerDeployer: 3, ArchiveUploadBytes: 2 << 20, ExpandedReleaseBytes: 3 << 20, FilesPerRelease: 7, SingleFileBytes: 1 << 20, DeploymentAttemptsPerHour: 4, ReleaseRetention: 2, DiskWarningPercent: 80, DiskStopPercent: 90}}
	r, err := NewResourceControls(cfg, resourceDisk{operations.Disk{Used: 90, Total: 100}})
	if err != nil {
		t.Fatal(err)
	}
	d := &deployments.Service{}
	r.ConfigureDeployments(d)
	if d.UploadLimit != 2<<20 || d.Limits.MaxEntries != 7 || d.Limits.MaxExpanded != 3<<20 || d.AttemptLimit != 4 {
		t.Fatalf("service=%+v", d)
	}
	if err := d.WriteGate.AllowWrite(context.Background(), operations.WriteDeployment); !errors.Is(err, operations.ErrWriteDisabled) {
		t.Fatal(err)
	}
}
