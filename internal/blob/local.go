package blob

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"syscall"
)

// LocalStore is a private byte adapter. os.Root keeps every operation beneath
// the configured data root; the explicit Lstat directory checks additionally
// reject a symlink that could otherwise redirect one app's namespace to another
// namespace inside that root.
type LocalStore struct{ Root string }

var localSync = func(f *os.File) error { return f.Sync() }
var localRename = func(r *os.Root, old, new string) error { return r.Rename(old, new) }
var localSyncDir = func(r *os.Root) error {
	d, err := r.Open(".")
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

func regularSingleLink(info os.FileInfo) bool {
	if info == nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return false
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && st.Nlink == 1
}
func appRelative(app, id string) (string, error) {
	if !validAppID(app) || !validID(id) {
		return "", errors.New("unsafe blob key")
	}
	return path.Join("blobs", app, id), nil
}

// validAppID rejects every value that could have path semantics before it is
// ever handed to os.Root. In particular, path.Base alone accepts "." and
// "..", which must never name a tenant namespace.
func validAppID(app string) bool {
	return app != "" && app != "." && app != ".." &&
		!strings.ContainsAny(app, "/\\\x00") && path.Base(app) == app
}
func safeDir(root *os.Root, name string) error {
	info, e := root.Lstat(name)
	if e != nil {
		return e
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("unsafe blob namespace")
	}
	return nil
}
func (s LocalStore) appRoot(app string, create bool) (*os.Root, error) {
	_, e := appRelative(app, "blb_0123456789abcdef0123456789abcdef")
	if e != nil {
		return nil, e
	}
	r, e := os.OpenRoot(s.Root)
	if e != nil {
		return nil, e
	}
	if create {
		e = r.MkdirAll("blobs", 0700)
	}
	if e == nil {
		e = safeDir(r, "blobs")
	}
	if e == nil && create {
		e = r.MkdirAll(path.Join("blobs", app), 0700)
	}
	if e == nil {
		e = safeDir(r, path.Join("blobs", app))
	}
	if e != nil {
		r.Close()
		return nil, e
	}
	// Pin the app directory itself. Later operations use only this descriptor,
	// never `blobs/app/...` again, so an in-root swap cannot cross app tenants.
	child, e := r.OpenRoot(path.Join("blobs", app))
	r.Close()
	if e != nil {
		return nil, e
	}
	return child, nil
}
func randomTemp() (string, error) {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return ".staging-" + hex.EncodeToString(b), nil
}
func (s LocalStore) Put(app, id string, src io.Reader, limit int64) (int64, string, error) {
	_, e := appRelative(app, id)
	if e != nil {
		return 0, "", e
	}
	r, e := s.appRoot(app, true)
	if e != nil {
		return 0, "", e
	}
	defer r.Close()
	tmp, e := randomTemp()
	if e != nil {
		return 0, "", e
	}
	f, e := r.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return 0, "", e
	}
	defer r.Remove(tmp)
	h := sha256.New()
	n, e := io.Copy(io.MultiWriter(h, f), io.LimitReader(src, limit+1))
	if e == nil && n == 0 {
		e = ErrInvalidMetadata
	}
	if e == nil && n > limit {
		e = ErrQuotaExceeded
	}
	if x := localSync(f); e == nil {
		e = x
	}
	if x := f.Close(); e == nil {
		e = x
	}
	if e != nil {
		return 0, "", e
	}
	if e = localRename(r, tmp, id); e != nil {
		return 0, "", e
	}
	// A rename is only durable after its containing directory is synced. This
	// prevents a successful response from claiming a ready byte object that a
	// power loss can silently unlink.
	if e = localSyncDir(r); e != nil {
		_ = r.Remove(id)
		return 0, "", e
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}
func (s LocalStore) Open(app, id string) (io.ReadCloser, int64, string, error) {
	_, e := appRelative(app, id)
	if e != nil {
		return nil, 0, "", e
	}
	r, e := s.appRoot(app, false)
	if e != nil {
		return nil, 0, "", e
	}
	defer r.Close()
	info, e := r.Lstat(id)
	if e != nil || !regularSingleLink(info) {
		return nil, 0, "", errors.New("unavailable blob bytes")
	}
	f, e := r.Open(id)
	if e != nil {
		return nil, 0, "", e
	}
	st, e := f.Stat()
	if e != nil || !regularSingleLink(st) {
		f.Close()
		return nil, 0, "", errors.New("unavailable blob bytes")
	}
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		f.Close()
		return nil, 0, "", e
	}
	if _, e = f.Seek(0, 0); e != nil {
		f.Close()
		return nil, 0, "", e
	}
	return f, st.Size(), hex.EncodeToString(h.Sum(nil)), nil
}
func (s LocalStore) Delete(app, id string) error {
	_, e := appRelative(app, id)
	if e != nil {
		return e
	}
	r, e := s.appRoot(app, false)
	if e != nil {
		if os.IsNotExist(e) {
			return nil
		}
		return e
	}
	defer r.Close()
	info, e := r.Lstat(id)
	if e == nil && !regularSingleLink(info) {
		return errors.New("unsafe blob bytes")
	}
	e = r.Remove(id)
	if errors.Is(e, fs.ErrNotExist) {
		return nil
	}
	return e
}

// RemoveAppNamespace removes exactly one empty, validated app blob directory.
// Reconciliation must remove every object first; refusing a non-empty
// directory makes a partial purge fail closed and retryable.
func (s LocalStore) RemoveAppNamespace(app string) error {
	if !validAppID(app) {
		return errors.New("unsafe blob namespace")
	}
	r, err := os.OpenRoot(s.Root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer r.Close()
	if err = safeDir(r, "blobs"); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	blobsRoot, err := r.OpenRoot("blobs")
	if err != nil {
		return err
	}
	defer blobsRoot.Close()
	if err = safeDir(blobsRoot, app); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	child, err := blobsRoot.OpenRoot(app)
	if err != nil {
		return err
	}
	entries, err := fs.ReadDir(child.FS(), ".")
	child.Close()
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return errors.New("blob namespace not empty")
	}
	if err = blobsRoot.Remove(app); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return localSyncDir(blobsRoot)
}

// Keys returns one deterministic page strictly after after. Recovery owns this
// private enumeration; it never informs public list results.
func (s LocalStore) Keys(after Key, limit int) ([]Key, bool, error) {
	if limit < 1 {
		return nil, false, errors.New("invalid recovery page")
	}
	r, e := os.OpenRoot(s.Root)
	if os.IsNotExist(e) {
		return nil, false, nil
	}
	if e != nil {
		return nil, false, e
	}
	defer r.Close()
	if e = safeDir(r, "blobs"); os.IsNotExist(e) {
		return nil, false, nil
	} else if e != nil {
		return nil, false, e
	}
	apps, e := fs.ReadDir(r.FS(), "blobs")
	if e != nil {
		return nil, false, e
	}
	out := []Key{}
	for _, a := range apps {
		// Entries are private adapter evidence, not trusted catalog input. Do
		// not let a corrupt directory name acquire os.Root path semantics.
		if !validAppID(a.Name()) {
			return nil, false, errors.New("unsafe blob namespace")
		}
		if e = safeDir(r, path.Join("blobs", a.Name())); e != nil {
			return nil, false, e
		}
		appRoot, e := r.OpenRoot(path.Join("blobs", a.Name()))
		if e != nil {
			return nil, false, e
		}
		items, e := fs.ReadDir(appRoot.FS(), ".")
		if e != nil {
			appRoot.Close()
			return nil, false, e
		}
		for _, x := range items {
			if len(out) >= limit {
				appRoot.Close()
				return out, true, nil
			}
			// Only our exact random staging form may be removed. An unfamiliar
			// entry is evidence of tampering and stops reconciliation closed.
			if validStaging(x.Name()) {
				if e = appRoot.Remove(x.Name()); e != nil {
					appRoot.Close()
					return nil, false, e
				}
				continue
			}
			if !validID(x.Name()) {
				appRoot.Close()
				return nil, false, errors.New("unsafe blob object")
			}
			info, e := appRoot.Lstat(x.Name())
			if e != nil || !regularSingleLink(info) {
				return nil, false, errors.New("unsafe blob object")
			}
			key := Key{a.Name(), x.Name()}
			if key.App < after.App || (key.App == after.App && key.ID <= after.ID) {
				continue
			}
			out = append(out, key)
		}
		appRoot.Close()
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].App < out[j].App || (out[i].App == out[j].App && out[i].ID < out[j].ID)
	})
	return out, false, nil
}

func validStaging(v string) bool {
	if len(v) != len(".staging-")+32 || !strings.HasPrefix(v, ".staging-") {
		return false
	}
	for _, r := range v[len(".staging-"):] {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}
