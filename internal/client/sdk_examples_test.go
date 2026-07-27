package client

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinyhost/tiny/internal/releases"
)

func TestSDKExampleAppsRemainPrivateCapabilityProjects(t *testing.T) {
	examples := map[string]string{
		"shared-checklist": "shared-checklist",
		"team-pulse":       "team-pulse",
		"quick-poll":       "quick-poll",
	}
	for directory, slug := range examples {
		t.Run(directory, func(t *testing.T) {
			project := filepath.Join("..", "..", "examples", "sdk-apps", directory)
			raw, err := os.ReadFile(filepath.Join(project, "tiny.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			manifest, err := releases.ParseManifest(raw)
			if err != nil {
				t.Fatal(err)
			}
			if manifest.Name != slug || manifest.BuildOutput != "dist" ||
				!manifest.KV || !manifest.Realtime ||
				len(manifest.Emails) != 0 || len(manifest.Domains) != 0 {
				t.Fatalf("example expanded its manifest boundary: %#v", manifest)
			}

			for _, name := range []string{"index.html", "app.js"} {
				content, err := os.ReadFile(filepath.Join(project, "src", name))
				if err != nil {
					t.Fatal(err)
				}
				source := string(content)
				for _, forbidden := range []string{
					"http://",
					"https://",
					"appId",
					"app_id",
					"viewerToken",
					"deployerToken",
					"apiSecret",
				} {
					if strings.Contains(source, forbidden) {
						t.Fatalf("%s contains forbidden browser configuration %q", name, forbidden)
					}
				}
			}
		})
	}
}
