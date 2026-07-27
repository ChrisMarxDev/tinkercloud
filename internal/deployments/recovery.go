package deployments

import (
	"context"
	"path/filepath"

	"github.com/tinyhost/tiny/internal/releases"
)

// FilesystemEvidence validates only the content-addressed release derived from
// a persisted app ID and release hash. It performs no discovery, deletion, or
// staging-directory recovery.
type FilesystemEvidence struct{ Root string }

func (f FilesystemEvidence) Valid(_ context.Context, r Record) bool {
	if f.Root == "" || !releases.ValidReleaseID(r.AppID) || r.ReleaseHash == "" || len(r.Files) == 0 {
		return false
	}
	m, err := releases.Inspect(filepath.Join(f.Root, "releases", r.AppID, r.ReleaseHash))
	if err != nil {
		return false
	}
	return m.Equal(releases.FileManifest{Hash: r.ReleaseHash, Files: r.Files})
}
