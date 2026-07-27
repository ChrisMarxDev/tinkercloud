package staticruntime

import (
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/tinyhost/tiny/internal/appauth"
	"github.com/tinyhost/tiny/internal/releases"
)

// Serve is intentionally unreachable without the sealed authorization context.
func Serve(auth appauth.AuthorizationContext, w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "HEAD" {
		http.NotFound(w, r)
		return
	}
	// An authorization context is sealed by appauth. Only after it exists may
	// static serving read the release tree; a path alone is never evidence that
	// the current release is safe to serve.
	if !verifyRelease(auth) {
		http.NotFound(w, r)
		return
	}
	rel, err := safePath(r.URL)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	f, info, err := openBeneath(auth.ReleaseRoot(), rel)
	if errors.Is(err, fs.ErrNotExist) && auth.SPAFallback() && !strings.HasPrefix(r.URL.Path, "/_tiny/") {
		f, info, err = openBeneath(auth.ReleaseRoot(), "index.html")
	}
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	etag := fmt.Sprintf("W/\"%x-%x\"", info.Size(), info.ModTime().UnixNano())
	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	ct := mime.TypeByExtension(filepath.Ext(info.Name()))
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
}

var inspectRelease = releases.Inspect

// SetInspectorForTest replaces the bounded immutable-release inspection used
// by Serve and returns a restore function. It exists so gateway deny-path
// tests can prove that no release inspection occurs before authorization.
// Production code must not call it.
func SetInspectorForTest(fn func(string) (releases.FileManifest, error)) func() {
	previous := inspectRelease
	inspectRelease = fn
	return func() { inspectRelease = previous }
}

func verifyRelease(auth appauth.AuthorizationContext) bool {
	evidence := auth.ReleaseEvidence()
	if evidence.Hash == "" || auth.ReleaseRoot() == "" {
		return false
	}
	actual, err := inspectRelease(auth.ReleaseRoot())
	return err == nil && actual.Equal(evidence)
}
func safePath(u *url.URL) (string, error) {
	raw := u.EscapedPath()
	if raw == "" {
		raw = "/"
	}
	decoded, err := url.PathUnescape(raw)
	if err != nil || strings.ContainsAny(decoded, "\\\x00") {
		return "", errors.New("bad path")
	}
	clean := path.Clean("/" + decoded)
	if clean == "/" {
		clean = "/index.html"
	}
	rel := strings.TrimPrefix(clean, "/")
	for _, part := range strings.Split(rel, "/") {
		if part == "" || part == "." || strings.HasPrefix(part, ".") {
			return "", errors.New("unsafe path")
		}
	}
	if strings.HasSuffix(strings.ToLower(rel), ".map") {
		return "", errors.New("source maps unavailable")
	}
	return rel, nil
}
func openBeneath(root, rel string) (*os.File, os.FileInfo, error) {
	// os.Root is used with the fixed Go 1.25.12 toolchain: resolution cannot
	// escape the immutable release root, including during symlink races.
	rootInfo, err := os.Lstat(root)
	if err != nil || rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return nil, nil, errors.New("unsafe release root")
	}
	r, err := os.OpenRoot(root)
	if err != nil {
		return nil, nil, err
	}
	defer r.Close()
	f, err := r.Open(rel)
	if err != nil {
		return nil, nil, err
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		f.Close()
		return nil, nil, errors.New("not regular")
	}
	return f, info, nil
}
