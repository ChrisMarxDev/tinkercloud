package main

import (
	"errors"
	"os"
	"path/filepath"
)

func writeAtomicPrivate(path string, data []byte) error {
	if st, e := os.Lstat(path); e == nil && st.Mode()&os.ModeSymlink != 0 {
		return errors.New("unsafe state path")
	}
	dir := filepath.Dir(path)
	f, e := os.CreateTemp(dir, ".tinkercloud-")
	if e != nil {
		return e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(data)
	}
	if e == nil {
		e = f.Sync()
	}
	if ce := f.Close(); e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	if e = os.Rename(tmp, path); e != nil {
		return e
	}
	_ = os.Chmod(path, 0600)
	if d, e := os.Open(dir); e == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}
