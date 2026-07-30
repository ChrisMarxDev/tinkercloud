package client

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"github.com/ChrisMarxDev/tinkercloud/internal/releases"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func ArchiveProject(project string, manifest []byte, w io.Writer) error {
	m, e := releases.ParseManifest(manifest)
	if e != nil {
		return e
	}
	root, e := filepath.Abs(project)
	if e != nil {
		return e
	}
	out := filepath.Join(root, filepath.FromSlash(m.BuildOutput))
	rel, e := filepath.Rel(root, out)
	if e != nil || strings.HasPrefix(rel, "..") {
		return errors.New("unsafe output")
	}
	g := gzip.NewWriter(w)
	g.Header.ModTime = time.Unix(0, 0)
	tw := tar.NewWriter(g)
	h := &tar.Header{Name: "tinker.yaml", Mode: 0644, Size: int64(len(manifest)), ModTime: time.Unix(0, 0), Format: tar.FormatPAX}
	if e = tw.WriteHeader(h); e == nil {
		_, e = tw.Write(manifest)
	}
	if e != nil {
		return e
	}
	// When build.output is ".", tinker.yaml is inside the output directory. It
	// was already written as the canonical manifest entry above, so exclude it
	// from the bundle rather than creating a duplicate archive path.
	if e = archiveDirectoryTarExcept(out, tw, map[string]bool{"tinker.yaml": true}); e != nil {
		return e
	}
	if e = tw.Close(); e != nil {
		return e
	}
	return g.Close()
}

// ArchiveDirectory creates reproducible tar.gz bytes from exactly one directory.
func ArchiveDirectory(root string, w io.Writer) error {
	root, e := filepath.Abs(root)
	if e != nil {
		return e
	}
	st, e := os.Stat(root)
	if e != nil || !st.IsDir() {
		return errors.New("explicit directory required")
	}
	g := gzip.NewWriter(w)
	g.Header.ModTime = time.Unix(0, 0)
	t := tar.NewWriter(g)
	if e = archiveDirectoryTar(root, t); e != nil {
		return e
	}
	if e = t.Close(); e != nil {
		return e
	}
	return g.Close()
}

// archiveDirectoryTar writes sorted regular files directly to an existing tar
// stream. Keeping this separate lets ArchiveProject add tinker.yaml without ever
// materializing a second compressed archive in memory.
func archiveDirectoryTar(root string, t *tar.Writer) error {
	return archiveDirectoryTarExcept(root, t, nil)
}

func archiveDirectoryTarExcept(root string, t *tar.Writer, excluded map[string]bool) error {
	var names []string
	e := filepath.WalkDir(root, func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if p == root {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 || d.Type()&os.ModeType != 0 {
			return errors.New("special file rejected")
		}
		if d.IsDir() {
			return nil
		}
		rel, e := filepath.Rel(root, p)
		if e != nil || strings.HasPrefix(rel, "..") || strings.Contains(rel, "\\") {
			return errors.New("unsafe path")
		}
		name := filepath.ToSlash(rel)
		if excluded[name] {
			return nil
		}
		names = append(names, name)
		return nil
	})
	if e != nil {
		return e
	}
	sort.Strings(names)
	for _, n := range names {
		p := filepath.Join(root, filepath.FromSlash(n))
		f, e := os.Open(p)
		if e != nil {
			return e
		}
		i, e := f.Stat()
		if e == nil {
			e = t.WriteHeader(&tar.Header{Name: n, Mode: 0644, Size: i.Size(), ModTime: time.Unix(0, 0), Format: tar.FormatPAX})
		}
		if e == nil {
			_, e = io.Copy(t, f)
		}
		f.Close()
		if e != nil {
			return e
		}
	}
	return nil
}
