package archive

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractZipRejectsTraversal(t *testing.T) {
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	f, _ := w.Create("../escape")
	f.Write([]byte("bad"))
	w.Close()
	r, e := zip.NewReader(bytes.NewReader(b.Bytes()), int64(b.Len()))
	if e != nil {
		t.Fatal(e)
	}
	root := t.TempDir()
	if err := ExtractZip(context.Background(), r, root, Limits{10, 4, 100, 100}); err == nil {
		t.Fatal("traversal accepted")
	}
	if _, e := os.Stat(filepath.Join(filepath.Dir(root), "escape")); !os.IsNotExist(e) {
		t.Fatal("wrote outside root")
	}
}
func TestExtractZipRejectsCaseCollision(t *testing.T) {
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for _, n := range []string{"A.js", "a.js"} {
		f, _ := w.Create(n)
		f.Write([]byte("x"))
	}
	w.Close()
	z, _ := zip.NewReader(bytes.NewReader(b.Bytes()), int64(b.Len()))
	if ExtractZip(context.Background(), z, t.TempDir(), Limits{10, 4, 100, 100}) == nil {
		t.Fatal("case collision accepted")
	}
}
func TestExtractTarRejectsLink(t *testing.T) {
	var b bytes.Buffer
	g := gzip.NewWriter(&b)
	tw := tar.NewWriter(g)
	tw.WriteHeader(&tar.Header{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"})
	tw.Close()
	g.Close()
	if ExtractTarGz(context.Background(), bytes.NewReader(b.Bytes()), t.TempDir(), Limits{10, 4, 100, 100}) == nil {
		t.Fatal("link accepted")
	}
}
func TestCleanRejectsWindowsAndAbsolute(t *testing.T) {
	for _, p := range []string{"/x", "C:/x", "a\\b", "../x"} {
		if _, e := clean(p, 4); e == nil {
			t.Fatal(p)
		}
	}
}
func TestTarRejectsSpecialHeaders(t *testing.T) {
	for _, typ := range []byte{tar.TypeLink, tar.TypeChar, tar.TypeBlock, tar.TypeFifo, tar.TypeSymlink} {
		t.Run(string([]byte{typ}), func(t *testing.T) {
			var b bytes.Buffer
			g := gzip.NewWriter(&b)
			tw := tar.NewWriter(g)
			if e := tw.WriteHeader(&tar.Header{Name: "bad", Typeflag: typ, Size: 0}); e != nil {
				t.Fatal(e)
			}
			tw.Close()
			g.Close()
			if ExtractTarGz(context.Background(), bytes.NewReader(b.Bytes()), t.TempDir(), Limits{10, 4, 100, 100}) == nil {
				t.Fatal("special header accepted")
			}
		})
	}
}
func TestArchiveBudgetsAndCancellation(t *testing.T) {
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for _, n := range []string{"a", "b"} {
		f, _ := w.Create(n)
		f.Write([]byte("hello"))
	}
	w.Close()
	z, _ := zip.NewReader(bytes.NewReader(b.Bytes()), int64(b.Len()))
	if ExtractZip(context.Background(), z, t.TempDir(), Limits{1, 4, 100, 100}) == nil {
		t.Fatal("entry budget ignored")
	}
	z, _ = zip.NewReader(bytes.NewReader(b.Bytes()), int64(b.Len()))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if ExtractZip(ctx, z, t.TempDir(), Limits{10, 4, 100, 100}) == nil {
		t.Fatal("cancel ignored")
	}
}
func TestCleanRejectsUnicode(t *testing.T) {
	if _, e := clean("é.txt", 4); e == nil {
		t.Fatal("unicode accepted")
	}
}
func FuzzClean(f *testing.F) {
	for _, s := range []string{"a/b", "../x", "C:/x", "a\\b"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		p, e := clean(s, 8)
		if e == nil && (p == "" || p[0] == '/') {
			t.Fatal("unsafe accepted")
		}
	})
}
func TestExtractZipWritesNormalFile(t *testing.T) {
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	f, _ := w.Create("assets/app.js")
	f.Write([]byte("ok"))
	w.Close()
	r, _ := zip.NewReader(bytes.NewReader(b.Bytes()), int64(b.Len()))
	root := t.TempDir()
	if err := ExtractZip(context.Background(), r, root, Limits{10, 4, 100, 100}); err != nil {
		t.Fatal(err)
	}
	v, e := os.ReadFile(filepath.Join(root, "assets", "app.js"))
	if e != nil || string(v) != "ok" {
		t.Fatal("missing content")
	}
}
