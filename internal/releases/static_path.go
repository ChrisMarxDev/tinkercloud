package releases

import (
	"path"
	"strings"
)

// ServableStaticAssetPath reports whether a release-relative path can be
// requested from the static runtime. Receipts must never nominate an upload
// artifact that the runtime deliberately hides, such as a dotfile or source
// map.
func ServableStaticAssetPath(rel string) bool {
	if rel == "" || strings.HasPrefix(rel, "/") || strings.Contains(rel, "\\") || path.Clean(rel) != rel || strings.HasPrefix(rel, "../") {
		return false
	}
	for _, part := range strings.Split(rel, "/") {
		if part == "" || part == "." || strings.HasPrefix(part, ".") {
			return false
		}
	}
	return !strings.HasSuffix(strings.ToLower(rel), ".map")
}
