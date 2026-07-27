package releases

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

var ErrReleaseFS = errors.New("unsafe release filesystem")

// Narrow seams keep destructive filesystem failure paths executable in tests.
// They are package-private and production always uses the os implementation.
var (
	fsMkdirAll = os.MkdirAll
	fsRename   = os.Rename
	fsChmod    = os.Chmod
	fsSyncDir  = syncDir
	fsSyncTree = syncTree
)

type File struct {
	Path, Hash string
	Size       int64
}
type FileManifest struct {
	Files []File
	Hash  string
}

// Equal compares the full canonical file evidence, not only the aggregate
// digest. It is used when a content-addressed release directory already
// exists: a matching path is never by itself evidence that it is safe to use.
func (m FileManifest) Equal(other FileManifest) bool {
	if m.Hash != other.Hash || len(m.Files) != len(other.Files) {
		return false
	}
	for i := range m.Files {
		if m.Files[i] != other.Files[i] {
			return false
		}
	}
	return true
}

// Finalize derives the immutable path only from appID and canonical content.
// It never changes the active deployment pointer; callers may activate only
// after this function and the durable metadata transition both succeed.
func Finalize(dataRoot, appID, staging string) (string, FileManifest, error) {
	m, err := Inspect(staging)
	if err != nil {
		return "", FileManifest{}, err
	}
	if !ValidReleaseID(appID) {
		return "", m, ErrReleaseFS
	}
	dest := filepath.Join(dataRoot, "releases", appID, m.Hash)
	parent := filepath.Dir(dest)
	if existing, err := Inspect(dest); err == nil {
		if !m.Equal(existing) {
			return "", m, ErrReleaseFS
		}
		return dest, m, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", m, err
	}

	// Make the candidate complete and read-only before it is visible under the
	// immutable namespace. Every metadata/data sync error is returned.
	if err := sealAndSyncTree(staging); err != nil {
		return "", m, err
	}
	if err := fsMkdirAll(parent, 0755); err != nil {
		return "", m, err
	}
	if err := fsSyncDir(parent); err != nil {
		return "", m, err
	}
	if err := fsRename(staging, dest); err != nil {
		// A concurrent finalizer may have installed this digest after our first
		// inspection. Re-inspect it rather than treating a path collision as
		// success.
		if existing, inspectErr := Inspect(dest); inspectErr == nil && m.Equal(existing) {
			return dest, m, nil
		}
		return "", m, err
	}
	// The staging root remains writable until after rename because macOS (and
	// some Linux filesystems with ACLs) requires it for a directory rename.
	// It is never reachable by the gateway before this call returns.
	if err := fsChmod(dest, 0555); err != nil {
		return "", m, err
	}
	if err := fsSyncDir(dest); err != nil {
		return "", m, err
	}
	if err := fsSyncDir(parent); err != nil {
		return "", m, err
	}
	return dest, m, nil
}

// ValidReleaseID is intentionally stricter than a filesystem component. App
// IDs are server-issued, but this still prevents Finalize callers from turning
// an accidental malformed ID into a path escape.
func ValidReleaseID(id string) bool {
	return id != "" && filepath.Base(id) == id && !strings.ContainsAny(id, `/\\`) && id != "." && id != ".."
}

func sealAndSyncTree(root string) error {
	if err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() && !info.IsDir() {
			return ErrReleaseFS
		}
		if info.IsDir() {
			if path == root {
				return nil
			}
			return fsChmod(path, 0555)
		}
		return fsChmod(path, 0444)
	}); err != nil {
		return err
	}
	return fsSyncTree(root)
}

func syncTree(root string) error {
	var dirs []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() && !info.IsDir() {
			return ErrReleaseFS
		}
		if info.IsDir() {
			dirs = append(dirs, path)
			return nil
		}
		return syncFile(path)
	})
	if err != nil {
		return err
	}
	// Leaf directories before ancestors makes directory entry persistence
	// explicit even on filesystems with weaker ordering guarantees.
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	for _, dir := range dirs {
		if err := syncDir(dir); err != nil {
			return err
		}
	}
	return nil
}

func syncFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	err = f.Sync()
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func syncDir(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	err = f.Sync()
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}

// Inspect hashes a release tree without mutating it. The aggregate hash covers
// path, byte hash, and byte count so the stored release hash is a canonical
// description of the exact release tree.
func Inspect(staging string) (FileManifest, error) {
	rootInfo, err := os.Lstat(staging)
	if err != nil {
		return FileManifest{}, err
	}
	if !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return FileManifest{}, ErrReleaseFS
	}
	var files []File
	err = filepath.WalkDir(staging, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == staging {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() && !info.IsDir() {
			return ErrReleaseFS
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(staging, path)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") || strings.Contains(rel, "\\") {
			return ErrReleaseFS
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		h := sha256.New()
		n, copyErr := io.Copy(h, f)
		closeErr := f.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		files = append(files, File{Path: filepath.ToSlash(rel), Hash: hex.EncodeToString(h.Sum(nil)), Size: n})
		return nil
	})
	if err != nil {
		return FileManifest{}, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	h := sha256.New()
	for _, file := range files {
		if _, err := io.WriteString(h, fmt.Sprintf("%s\x00%s\x00%s\x00", file.Path, file.Hash, strconv.FormatInt(file.Size, 10))); err != nil {
			return FileManifest{}, err
		}
	}
	return FileManifest{Files: files, Hash: hex.EncodeToString(h.Sum(nil))}, nil
}
