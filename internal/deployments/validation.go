package deployments

import (
	"errors"
	"github.com/ChrisMarxDev/tinkercloud/internal/releases"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var ErrReleaseValidation = errors.New("release validation failed")

func ValidateRelease(root, slug string) (releases.Manifest, error) {
	f, e := os.Open(filepath.Join(root, "tinker.yaml"))
	if e != nil {
		return releases.Manifest{}, ErrReleaseValidation
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, 64*1024+1))
	if e != nil || len(b) > 64*1024 {
		return releases.Manifest{}, ErrReleaseValidation
	}
	m, e := releases.ParseManifest(b)
	if e != nil || m.Name != slug {
		return releases.Manifest{}, ErrReleaseValidation
	}
	check := func(p string) bool {
		if p == "" || filepath.IsAbs(p) || strings.Contains(p, "\\") {
			return false
		}
		full := filepath.Join(root, p)
		rel, e := filepath.Rel(root, full)
		if e != nil || strings.HasPrefix(rel, "..") {
			return false
		}
		i, e := os.Lstat(full)
		return e == nil && i.Mode().IsRegular()
	}
	if !check("index.html") || m.SPAFallback != "" && !check(m.SPAFallback) {
		return releases.Manifest{}, ErrReleaseValidation
	}
	return m, nil
}
