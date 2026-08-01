// Package verification models gateway evidence required before deploy success.
package verification

import (
	"context"
	"errors"
	"github.com/ChrisMarxDev/tinkercloud/internal/deployments"
	"github.com/ChrisMarxDev/tinkercloud/internal/releases"
	"path/filepath"
	"reflect"
)

type Probe struct {
	URL                  string
	AnonymousDenied      bool
	AuthenticatedHealthy bool
	Detail               string
	Posture              string
	RootSHA256           string
	Indexing             bool
}

func (p Probe) Passed() bool { return p.URL != "" && p.AnonymousDenied && p.AuthenticatedHealthy }

type Runner interface {
	Probe(context.Context, string) (Probe, error)
}

func CandidateFilesystem(dataRoot string, r deployments.Record) (string, error) {
	if r.AppID == "" || r.ReleaseHash == "" {
		return "", errors.New("candidate missing identity")
	}
	p := filepath.Join(dataRoot, "releases", r.AppID, r.ReleaseHash)
	m, e := releases.Inspect(p)
	if e != nil || !m.Equal(releases.FileManifest{Files: r.Files, Hash: r.ReleaseHash}) {
		return "", errors.New("candidate hash mismatch")
	}
	parsed, e := deployments.ValidateRelease(p, r.AppSlug)
	if e != nil || !reflect.DeepEqual(parsed, r.Manifest) {
		return "", errors.New("candidate manifest mismatch")
	}
	return p, nil
}
