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

	"github.com/ChrisMarxDev/tinkercloud/internal/appauth"
	"github.com/ChrisMarxDev/tinkercloud/internal/releases"
)

// Serve is intentionally unreachable without the sealed authorization context.
// Outcome separates the exact static-document decision from side effects such
// as local insights. DocumentCandidate only becomes countable if the gateway's
// response observer confirms a successful 200 response.
type Outcome struct{ DocumentCandidate bool }

func Serve(auth appauth.StaticAccessContext, w http.ResponseWriter, r *http.Request, beforeDocument ...func()) Outcome {
	if r.Method != "GET" && r.Method != "HEAD" {
		http.NotFound(w, r)
		return Outcome{}
	}
	// An authorization context is sealed by appauth. Only after it exists may
	// static serving read the release tree; a path alone is never evidence that
	// the current release is safe to serve.
	if !verifyRelease(auth) {
		http.NotFound(w, r)
		return Outcome{}
	}
	rel, err := safePath(r.URL)
	if err != nil {
		http.NotFound(w, r)
		return Outcome{}
	}
	f, info, err := openBeneath(auth.ReleaseRoot(), rel)
	if errors.Is(err, fs.ErrNotExist) && auth.SPAFallback() && !strings.HasPrefix(r.URL.Path, "/_tinker/") {
		f, info, err = openBeneath(auth.ReleaseRoot(), "index.html")
	}
	if err != nil {
		http.NotFound(w, r)
		return Outcome{}
	}
	defer f.Close()
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if auth.Public() {
		w.Header().Set("Cache-Control", "no-store")
		if !auth.Indexing() {
			w.Header().Set("X-Robots-Tag", "noindex, nofollow")
		}
	} else {
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	}
	etag := fmt.Sprintf("W/\"%x-%x\"", info.Size(), info.ModTime().UnixNano())
	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return Outcome{}
	}
	ct := mime.TypeByExtension(filepath.Ext(info.Name()))
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	document := r.Method == http.MethodGet && r.Header.Get("Range") == "" && strings.HasPrefix(strings.ToLower(ct), "text/html")
	if document && len(beforeDocument) > 0 && beforeDocument[0] != nil {
		beforeDocument[0]()
	}
	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
	return Outcome{DocumentCandidate: document}
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

func verifyRelease(auth appauth.StaticAccessContext) bool {
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
	if !releases.ServableStaticAssetPath(rel) {
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
