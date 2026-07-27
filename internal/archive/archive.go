// Package archive validates and extracts deployer archives into a caller-created
// private staging directory. It never accepts destination paths from archives.
package archive

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

var ErrUnsafe = errors.New("unsafe archive")

type Limits struct {
	MaxEntries, MaxDepth int
	MaxExpanded, MaxFile int64
}

func (l Limits) valid() bool {
	return l.MaxEntries > 0 && l.MaxDepth > 0 && l.MaxExpanded > 0 && l.MaxFile > 0
}

type entry struct {
	name string
	size int64
	mode os.FileMode
	dir  bool
	r    io.Reader
}
type budget struct {
	l     Limits
	n     int
	total int64
	seen  map[string]struct{}
}

func clean(name string, maxDepth int) (string, error) {
	if name == "" || strings.Contains(name, "\\") || strings.HasPrefix(name, "/") || strings.Contains(name, ":") || !utf8.ValidString(name) {
		return "", ErrUnsafe
	}
	for _, r := range name {
		if r > 0x7f {
			return "", ErrUnsafe
		}
	}
	c := path.Clean(name)
	if c == "." || strings.HasPrefix(c, "../") || strings.Contains(c, "//") {
		return "", ErrUnsafe
	}
	if len(strings.Split(c, "/")) > maxDepth {
		return "", ErrUnsafe
	}
	return c, nil
}
func (b *budget) put(ctx context.Context, root string, e entry) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	n, err := clean(e.name, b.l.MaxDepth)
	if err != nil {
		return err
	}
	if e.size < 0 || e.size > b.l.MaxFile || b.total > b.l.MaxExpanded-e.size {
		return ErrUnsafe
	}
	if b.n >= b.l.MaxEntries {
		return ErrUnsafe
	}
	key := strings.ToLower(n)
	if _, ok := b.seen[key]; ok {
		return ErrUnsafe
	}
	b.seen[key] = struct{}{}
	b.n++
	b.total += e.size
	// Directory entries are permitted only with conservative permissions.
	// Archive-supplied write/execute permissions are never applied. Privileged
	// permission bits are rejected; extracted files are always created 0644.
	if e.mode&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return ErrUnsafe
	}
	target := filepath.Join(root, filepath.FromSlash(n))
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ErrUnsafe
	}
	if e.dir {
		return os.MkdirAll(target, 0755)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	written, err := io.Copy(f, io.LimitReader(e.r, e.size+1))
	if err != nil || written != e.size {
		return fmt.Errorf("%w: extraction", ErrUnsafe)
	}
	return f.Sync()
}
func ExtractTarGz(ctx context.Context, r io.Reader, root string, l Limits) error {
	if !l.valid() {
		return ErrUnsafe
	}
	g, err := gzip.NewReader(r)
	if err != nil {
		return ErrUnsafe
	}
	defer g.Close()
	tr := tar.NewReader(g)
	b := budget{l: l, seen: map[string]struct{}{}}
	for {
		h, er := tr.Next()
		if er == io.EOF {
			return nil
		}
		if er != nil {
			return ErrUnsafe
		}
		if h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeDir {
			return ErrUnsafe
		}
		if err := b.put(ctx, root, entry{name: h.Name, size: h.Size, mode: os.FileMode(h.Mode), dir: h.Typeflag == tar.TypeDir, r: tr}); err != nil {
			return err
		}
	}
}
func ExtractZip(ctx context.Context, z *zip.Reader, root string, l Limits) error {
	if !l.valid() {
		return ErrUnsafe
	}
	b := budget{l: l, seen: map[string]struct{}{}}
	for _, f := range z.File {
		if f.FileInfo().Mode()&os.ModeType != 0 && !f.FileInfo().IsDir() {
			return ErrUnsafe
		}
		rc, err := f.Open()
		if err != nil {
			return ErrUnsafe
		}
		err = b.put(ctx, root, entry{name: f.Name, size: int64(f.UncompressedSize64), mode: f.Mode(), dir: f.FileInfo().IsDir(), r: rc})
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
