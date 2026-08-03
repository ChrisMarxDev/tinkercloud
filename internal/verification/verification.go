// Package verification models gateway evidence required before deploy success.
package verification

import (
	"context"
	"errors"
	"github.com/ChrisMarxDev/tinkercloud/internal/deployments"
	"github.com/ChrisMarxDev/tinkercloud/internal/releases"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
)

var lowercaseSHA256 = regexp.MustCompile(`^[0-9a-f]{64}$`)

type Probe struct {
	URL                  string
	AnonymousDenied      bool
	AuthenticatedHealthy bool
	PublicReachable      bool
	ReservedDenied       bool
	Detail               string
	Posture              string
	RootSHA256           string
	RootBytes            int64
	AssetPath            string
	AssetSHA256          string
	AssetBytes           int64
	Indexing             bool
}

// Passed is posture-aware. Private probes require root and reserved-route
// denials, optional real-asset denial evidence, and authenticated health. A
// public static candidate instead needs exact anonymously reachable bytes.
func (p Probe) Passed() bool {
	if p.URL == "" {
		return false
	}
	if p.Posture == "public_static" {
		if !p.PublicReachable || !p.ReservedDenied || !lowercaseSHA256.MatchString(p.RootSHA256) || p.RootBytes < 0 || (p.AssetPath == "") != (p.AssetSHA256 == "") {
			return false
		}
		return (p.AssetPath == "" && p.AssetBytes == 0) || (canonicalAssetPath(p.AssetPath) && lowercaseSHA256.MatchString(p.AssetSHA256) && p.AssetBytes >= 0)
	}
	if (p.Posture != "" && p.Posture != "private") || !p.AnonymousDenied || !p.AuthenticatedHealthy || !p.ReservedDenied {
		return false
	}
	if p.AssetPath == "" {
		return p.AssetSHA256 == "" && p.AssetBytes == 0
	}
	return canonicalAssetPath(p.AssetPath) && lowercaseSHA256.MatchString(p.AssetSHA256) && p.AssetBytes >= 0
}

func canonicalAssetPath(v string) bool {
	return v != "" && v != "index.html" && v[0] != '/' && path.Clean(v) == v && !strings.HasPrefix(v, "../") && !strings.Contains(v, "\\")
}

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
