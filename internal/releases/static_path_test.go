package releases

import "testing"

func TestServableStaticAssetPath(t *testing.T) {
	for path, want := range map[string]bool{"assets/app.js": true, "index.html": true, ".hidden": false, "assets/.hidden/app.js": false, "assets/app.js.map": false, "../app.js": false} {
		if got := ServableStaticAssetPath(path); got != want {
			t.Fatalf("%q = %v", path, got)
		}
	}
}
