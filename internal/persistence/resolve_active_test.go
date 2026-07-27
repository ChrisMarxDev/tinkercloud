package persistence

import (
	"context"
	"encoding/json"
	"github.com/tinyhost/tiny/internal/apps"
	"github.com/tinyhost/tiny/internal/releases"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveActiveDerivedRootAndEligibility(t *testing.T) {
	s := seeded(t)
	seedActiveRelease(t, s)
	defer s.Close()
	a, e := s.ResolveActive(context.Background(), "alpha")
	if e != nil || filepath.Base(a.ReleaseRoot) == "hash" || filepath.Dir(filepath.Dir(a.ReleaseRoot)) != filepath.Join(s.DataRoot, "releases") {
		t.Fatal(a, e)
	}
	if !s.EligibleAppHost(context.Background(), "alpha") {
		t.Fatal("eligible")
	}
	if s.EligibleAppHost(context.Background(), "beta") {
		t.Fatal("suspended")
	}
}
func TestResolveActiveCorruptManifestDenies(t *testing.T) {
	s := seeded(t)
	seedActiveRelease(t, s)
	defer s.Close()
	if _, e := s.DB.Exec("UPDATE deployments SET manifest_json='bad' WHERE id='d'"); e != nil {
		t.Fatal(e)
	}
	if _, e := s.ResolveActive(context.Background(), "alpha"); e != apps.ErrNotFound {
		t.Fatal(e)
	}
}
func TestResolveActiveDoesNotReadReleaseFiles(t *testing.T) {
	s := seeded(t)
	seedActiveRelease(t, s)
	defer s.Close()
	a, err := s.ResolveActive(context.Background(), "alpha")
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(a.ReleaseRoot, "index.html")
	if err := os.WriteFile(p, []byte("tampered"), 0644); err != nil {
		t.Fatal(err)
	}
	// App resolution is deliberately metadata-only. Login, OTP, and reserved
	// paths can resolve an app without opening untrusted files; staticruntime
	// verifies the immutable release only after authorization.
	if _, err := s.ResolveActive(context.Background(), "alpha"); err != nil {
		t.Fatalf("metadata lookup read tampered release: %v", err)
	}
	if _, err := s.DB.Exec("UPDATE deployment_files SET content_hash='bad' WHERE deployment_id='d'"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ResolveActive(context.Background(), "alpha"); err != nil {
		t.Fatalf("metadata lookup read corrupt evidence: %v", err)
	}
}
func TestEligibleWithoutDeployment(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	if _, e := s.DB.Exec("UPDATE applications SET current_deployment_id=NULL WHERE id='a'"); e != nil {
		t.Fatal(e)
	}
	if !s.EligibleAppHost(context.Background(), "alpha") {
		t.Fatal("not eligible")
	}
	if _, e := s.ResolveActive(context.Background(), "alpha"); e != apps.ErrNotFound {
		t.Fatal(e)
	}
}

var _ = json.Marshal
var _ = releases.Manifest{}
var _ = os.ModeSymlink
