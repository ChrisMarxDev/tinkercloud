package staticruntime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ChrisMarxDev/tinkercloud/internal/appauth"
	"github.com/ChrisMarxDev/tinkercloud/internal/apps"
	"github.com/ChrisMarxDev/tinkercloud/internal/identity"
	"github.com/ChrisMarxDev/tinkercloud/internal/policies"
	"github.com/ChrisMarxDev/tinkercloud/internal/releases"
	"github.com/ChrisMarxDev/tinkercloud/internal/sessions"
)

func TestOpenBeneathRejectsEscapes(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret")
	if e := os.WriteFile(outside, []byte("outside"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(outside, filepath.Join(root, "file")); e != nil {
		t.Fatal(e)
	}
	if _, _, e := openBeneath(root, "file"); e == nil {
		t.Fatal("file symlink")
	}
	if e := os.Symlink(filepath.Dir(outside), filepath.Join(root, "dir")); e != nil {
		t.Fatal(e)
	}
	if _, _, e := openBeneath(root, "dir/secret"); e == nil {
		t.Fatal("directory symlink")
	}
	for _, p := range []string{"../secret", "/etc/passwd"} {
		if _, _, e := openBeneath(root, p); e == nil {
			t.Fatal(p)
		}
	}
}

func TestServeVerifiesImmutableEvidenceAfterAuthorization(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	evidence, err := releases.Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	viewer := identity.Identity{ID: "viewer", Email: "viewer@example.com"}
	store := sessions.NewMemoryStore()
	token, _, err := store.Create("app", viewer, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	auth, err := (appauth.Authorizer{Sessions: store, Policies: &policies.MemoryStore{Policies: map[string]policies.Policy{"app": {AppID: "app", OwnerIdentityID: "owner", Revision: 1, Emails: map[string]struct{}{viewer.Email: {}}, Valid: true}}}}).Authorize(context.Background(), apps.App{ID: "app", Slug: "alpha", ReleaseRoot: root, ReleaseEvidence: evidence}, token, "req")
	if err != nil {
		t.Fatal(err)
	}
	reads := 0
	restore := SetInspectorForTest(func(path string) (releases.FileManifest, error) {
		reads++
		return releases.Inspect(path)
	})
	defer restore()
	w := httptest.NewRecorder()
	Serve(auth, w, httptest.NewRequest(http.MethodGet, "https://alpha.test/", nil))
	if w.Code != http.StatusOK || w.Body.String() != "private" || reads != 1 {
		t.Fatalf("verified serve: code=%d body=%q reads=%d", w.Code, w.Body.String(), reads)
	}
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	Serve(auth, w, httptest.NewRequest(http.MethodGet, "https://alpha.test/", nil))
	if w.Code != http.StatusNotFound || w.Body.String() == "tampered" || reads != 2 {
		t.Fatalf("corrupt release served: code=%d body=%q reads=%d", w.Code, w.Body.String(), reads)
	}
}
func TestOpenBeneathSymlinkSwapNeverLeaks(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside")
	_ = os.WriteFile(outside, []byte("OUTSIDE"), 0600)
	target := filepath.Join(root, "x")
	_ = os.WriteFile(target, []byte("inside"), 0600)
	var wg sync.WaitGroup
	done := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			_ = os.Remove(target)
			_ = os.Symlink(outside, target)
			_ = os.Remove(target)
			_ = os.WriteFile(target, []byte("inside"), 0600)
		}
		close(done)
	}()
	for i := 0; i < 500; i++ {
		f, _, e := openBeneath(root, "x")
		if e == nil {
			b := make([]byte, 16)
			n, _ := f.Read(b)
			f.Close()
			if string(b[:n]) == "OUTSIDE" {
				t.Fatal("outside leak")
			}
		}
	}
	<-done
	wg.Wait()
}
