package client

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"github.com/tinyhost/tiny/internal/deployments"
	"github.com/tinyhost/tiny/internal/releases"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func names(b []byte) []string {
	g, _ := gzip.NewReader(bytes.NewReader(b))
	t := tar.NewReader(g)
	var out []string
	for {
		h, e := t.Next()
		if e != nil {
			return out
		}
		out = append(out, h.Name)
	}
}
func TestArchiveProjectIsolatedAndReproducible(t *testing.T) {
	r := t.TempDir()
	os.Mkdir(filepath.Join(r, "dist"), 0755)
	os.WriteFile(filepath.Join(r, "dist", "index.html"), []byte("ok"), 0644)
	os.WriteFile(filepath.Join(r, "secret.txt"), []byte("no"), 0644)
	m := []byte("version: 1\nname: demo\nbuild:\n  output: dist\n")
	var a, b bytes.Buffer
	if e := ArchiveProject(r, m, &a); e != nil {
		t.Fatal(e)
	}
	if e := ArchiveProject(r, m, &b); e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(a.Bytes(), b.Bytes()) {
		t.Fatal("not reproducible")
	}
	got := names(a.Bytes())
	if len(got) != 2 || got[0] != "tiny.yaml" || got[1] != "index.html" {
		t.Fatal(got)
	}
	dir := t.TempDir()
	g, _ := gzip.NewReader(bytes.NewReader(a.Bytes()))
	tr := tar.NewReader(g)
	for {
		h, e := tr.Next()
		if e != nil {
			break
		}
		p := filepath.Join(dir, h.Name)
		os.WriteFile(p, func() []byte { x := make([]byte, h.Size); tr.Read(x); return x }(), 0644)
	}
	if _, e := deployments.ValidateRelease(dir, "demo"); e != nil {
		t.Fatal(e)
	}
}

func TestArchiveProjectExcludesManifestWhenOutputIsProjectRoot(t *testing.T) {
	r := t.TempDir()
	m := []byte("version: 1\nname: demo\nbuild:\n  output: .\n")
	if err := os.WriteFile(filepath.Join(r, "tiny.yaml"), m, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r, "index.html"), []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := ArchiveProject(r, m, &out); err != nil {
		t.Fatal(err)
	}
	got := names(out.Bytes())
	if len(got) != 2 || got[0] != "tiny.yaml" || got[1] != "index.html" {
		t.Fatalf("archive contains duplicate/missing manifest: %#v", got)
	}
}

func TestHumanStarterAppIsOwnerOnlyAndDeployable(t *testing.T) {
	project := filepath.Join("..", "..", "examples", "starter-app")
	manifest, err := os.ReadFile(filepath.Join(project, "tiny.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := releases.ParseManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Name != "tiny-ritual" || parsed.BuildOutput != "." ||
		len(parsed.Emails) != 0 || len(parsed.Domains) != 0 ||
		parsed.KV || parsed.Realtime {
		t.Fatalf("starter manifest expanded its owner-only boundary: %#v", parsed)
	}

	var archive bytes.Buffer
	if err := ArchiveProject(project, manifest, &archive); err != nil {
		t.Fatal(err)
	}
	want := []string{"tiny.yaml", "app.js", "index.html", "styles.css"}
	if got := names(archive.Bytes()); !reflect.DeepEqual(got, want) {
		t.Fatalf("starter archive = %#v, want %#v", got, want)
	}

	for _, name := range []string{"index.html", "styles.css", "app.js"} {
		content, err := os.ReadFile(filepath.Join(project, name))
		if err != nil {
			t.Fatal(err)
		}
		source := string(content)
		if strings.Contains(source, "http://") ||
			strings.Contains(source, "https://") ||
			strings.Contains(source, "@tinyhost/sdk") {
			t.Fatalf("%s introduced a remote or capability dependency", name)
		}
	}
}
