// Package jobs contains bounded local maintenance work. Cleanup takes database
// metadata as authority and never tries to infer active releases from paths.
package jobs

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Release struct {
	ID      string
	Created int64
	Active  bool
}

// Cleanup selects surplus inactive releases. keep is the number of inactive
// releases retained in addition to the active release (so V1's 10 means 10 +
// active). It always keeps an inactive recovery candidate when one exists.
func Cleanup(rs []Release, keep int) []Release {
	if keep < 1 {
		keep = 1
	}
	sort.Slice(rs, func(i, j int) bool {
		if rs[i].Created == rs[j].Created {
			return rs[i].ID > rs[j].ID
		}
		return rs[i].Created > rs[j].Created
	})
	out := []Release{}
	inactiveKept := 0
	for _, r := range rs {
		if r.Active {
			continue
		}
		if inactiveKept < keep {
			inactiveKept++
			continue
		}
		out = append(out, r)
	}
	return out
}

var ErrUnsafeReleasePath = errors.New("unsafe release cleanup path")

// ReleasePath contains server-derived components only. Hash is intentionally
// required to be a single component; cleanup never accepts a caller path.
func ReleasePath(dataRoot, appID, releaseHash string) (string, error) {
	if strings.TrimSpace(dataRoot) == "" || !safeComponent(appID) || !safeComponent(releaseHash) {
		return "", ErrUnsafeReleasePath
	}
	root := filepath.Clean(dataRoot)
	path := filepath.Join(root, "releases", appID, releaseHash)
	prefix := filepath.Join(root, "releases") + string(filepath.Separator)
	if !strings.HasPrefix(path, prefix) {
		return "", ErrUnsafeReleasePath
	}
	return path, nil
}

func safeComponent(v string) bool {
	return v != "" && filepath.Base(v) == v && v != "." && v != ".." && !strings.ContainsAny(v, `/\\`)
}

// RemoveRelease removes only a selected immutable release directory. A
// read-only directory or a filesystem error is returned unchanged so cleanup
// remains retryable. os.RemoveAll does not follow a final symlink; refuse one
// explicitly for clarity and defense in depth.
func RemoveRelease(dataRoot, appID, releaseHash string) error {
	p, err := ReleasePath(dataRoot, appID, releaseHash)
	if err != nil {
		return err
	}
	info, err := os.Lstat(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return ErrUnsafeReleasePath
	}
	// Releases are deliberately sealed read-only. Make only the selected tree
	// writable for the duration of removal: this is required on filesystems
	// where RemoveAll needs write access to every nested directory. If removal
	// fails, restore every observed mode so a retry cannot accidentally leave a
	// mutable immutable release behind.
	modes, err := makeWritableTree(p)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(p); err != nil {
		restoreModes(modes)
		return err
	}
	return nil
}

// RemoveAppReleases removes the complete, server-derived release namespace for
// one app. It is used only after the app has been made inaccessible. Unlike
// retention cleanup, app deletion must not leave an old immutable release
// behind merely because it is still within the normal retention window.
func RemoveAppReleases(dataRoot, appID string) error {
	if strings.TrimSpace(dataRoot) == "" || !safeComponent(appID) {
		return ErrUnsafeReleasePath
	}
	root := filepath.Clean(dataRoot)
	p := filepath.Join(root, "releases", appID)
	prefix := filepath.Join(root, "releases") + string(filepath.Separator)
	if !strings.HasPrefix(p, prefix) {
		return ErrUnsafeReleasePath
	}
	info, err := os.Lstat(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return ErrUnsafeReleasePath
	}
	modes, err := makeWritableTree(p)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(p); err != nil {
		restoreModes(modes)
		return err
	}
	return nil
}

type savedMode struct {
	path string
	mode os.FileMode
}

func makeWritableTree(root string) ([]savedMode, error) {
	var dirs []savedMode
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			return ErrUnsafeReleasePath
		}
		if !info.IsDir() {
			return nil
		}
		dirs = append(dirs, savedMode{path: path, mode: info.Mode().Perm()})
		return os.Chmod(path, info.Mode().Perm()|0700)
	})
	if err != nil {
		restoreModes(dirs)
		return nil, err
	}
	return dirs, nil
}

func restoreModes(modes []savedMode) {
	// Children first means a restrictive parent cannot prevent restoration.
	for i := len(modes) - 1; i >= 0; i-- {
		_ = os.Chmod(modes[i].path, modes[i].mode)
	}
}

// Executor keeps the destructive action injectable for failure-injection
// tests. A candidate is marked complete by its persistence owner only after
// this call succeeds.
type Executor struct {
	Remove func(string, string, string) error
}

func (e Executor) Execute(_ context.Context, dataRoot, appID, releaseHash string) error {
	remove := e.Remove
	if remove == nil {
		remove = func(a, b, c string) error { return RemoveRelease(a, b, c) }
	}
	return remove(dataRoot, appID, releaseHash)
}
