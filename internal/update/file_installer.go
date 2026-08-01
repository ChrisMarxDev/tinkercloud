package update

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

type FileInstaller struct{ Target, RollbackDir string }

func (f FileInstaller) Snapshot(context.Context) error {
	return f.copy(f.Target, filepath.Join(f.RollbackDir, "previous"), 0600)
}
func (f FileInstaller) Apply(_ context.Context, a Artifact) error {
	return f.atomic(f.Target, a.Bytes, 0755)
}
func (f FileInstaller) Restore(context.Context) error {
	return f.copy(filepath.Join(f.RollbackDir, "previous"), f.Target, 0755)
}

// Commit removes only the updater-owned bounded snapshot after every health
// gate has passed. It refuses a symlink so cleanup cannot be redirected out of
// the private data directory.
func (f FileInstaller) Commit(context.Context) error {
	p := filepath.Join(f.RollbackDir, "previous")
	st, err := os.Lstat(p)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil || st.Mode()&os.ModeSymlink != 0 || !st.Mode().IsRegular() {
		return errors.New("unsafe rollback snapshot")
	}
	if err := os.Remove(p); err != nil {
		return err
	}
	if err := os.Remove(f.RollbackDir); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
func (f FileInstaller) copy(src, dst string, mode os.FileMode) error {
	st, e := os.Lstat(src)
	if e != nil || st.Mode()&os.ModeSymlink != 0 || !st.Mode().IsRegular() {
		return errors.New("unsafe binary")
	}
	b, e := os.ReadFile(src)
	if e != nil {
		return e
	}
	return f.atomic(dst, b, mode)
}
func (f FileInstaller) atomic(path string, b []byte, mode os.FileMode) error {
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	tmp, e := os.CreateTemp(filepath.Dir(path), ".tinkercloud-")
	if e != nil {
		return e
	}
	n := tmp.Name()
	defer os.Remove(n)
	if e = tmp.Chmod(mode); e == nil {
		_, e = tmp.Write(b)
	}
	if e == nil {
		e = tmp.Sync()
	}
	if ce := tmp.Close(); e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	if e = os.Rename(n, path); e != nil {
		return e
	}
	d, e := os.Open(filepath.Dir(path))
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Sync()
}
