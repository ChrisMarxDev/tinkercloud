package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/ChrisMarxDev/tinkercloud/internal/config"
	"github.com/ChrisMarxDev/tinkercloud/internal/update"
)

type commandDoer func(*http.Request) (*http.Response, error)

func (f commandDoer) Do(r *http.Request) (*http.Response, error) { return f(r) }

func signedUpdate(t *testing.T) (ed25519.PublicKey, []byte, []byte, []byte) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	b := []byte("new binary")
	d := sha256.Sum256(b)
	m := []byte(fmt.Sprintf(`{"version":"1.0.0","api":"1","schema":"1","sha256":"%x"}`, d))
	s := ed25519.Sign(priv, []byte("1.0.0\n1\n1\n"+fmt.Sprintf("%x", d)))
	return pub, b, m, []byte(base64.StdEncoding.EncodeToString(s))
}

func TestUpdateArtifactDownloadsAndVerifies(t *testing.T) {
	pub, b, m, s := signedUpdate(t)
	oldKey, oldClient, oldValidate := releasePublicKeyBase64, updateHTTPClient, updateFetchValidator
	defer func() {
		releasePublicKeyBase64, updateHTTPClient, updateFetchValidator = oldKey, oldClient, oldValidate
	}()
	releasePublicKeyBase64 = base64.StdEncoding.EncodeToString(pub)
	updateFetchValidator = func(string) error { return nil }
	updateHTTPClient = commandDoer(func(r *http.Request) (*http.Response, error) {
		var body []byte
		switch {
		case strings.HasSuffix(r.URL.Path, ".metadata.json"):
			body = m
		case strings.HasSuffix(r.URL.Path, ".signature"):
			body = s
		default:
			body = b
		}
		return &http.Response{StatusCode: http.StatusOK, Request: r, ContentLength: int64(len(body)), Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	})
	a, key, err := updateArtifact(context.Background(), "", "", "", "https://releases.example/v1/", "", "", "tinkercloud-linux-amd64")
	if err != nil || string(a.Bytes) != string(b) || string(key) != string(pub) {
		t.Fatal(err, a)
	}
}

func TestUpdateArtifactTamperNeverVerifies(t *testing.T) {
	pub, b, m, s := signedUpdate(t)
	oldKey, oldClient, oldValidate := releasePublicKeyBase64, updateHTTPClient, updateFetchValidator
	defer func() {
		releasePublicKeyBase64, updateHTTPClient, updateFetchValidator = oldKey, oldClient, oldValidate
	}()
	releasePublicKeyBase64 = base64.StdEncoding.EncodeToString(pub)
	updateFetchValidator = func(string) error { return nil }
	updateHTTPClient = commandDoer(func(r *http.Request) (*http.Response, error) {
		body := b
		if strings.HasSuffix(r.URL.Path, ".metadata.json") {
			body = m
		}
		if strings.HasSuffix(r.URL.Path, ".signature") {
			body = append([]byte(nil), s...)
			body[0] ^= 1
		}
		return &http.Response{StatusCode: http.StatusOK, Request: r, ContentLength: int64(len(body)), Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	})
	if _, _, err := updateArtifact(context.Background(), "", "", "", "https://releases.example/v1/", "", "", "tinkercloud-linux-amd64"); err == nil {
		t.Fatal("tampered signature accepted")
	}
}

var _ = update.ErrFetch

type rollbackInstaller struct{ restored bool }

func (i *rollbackInstaller) Snapshot(context.Context) error               { return nil }
func (i *rollbackInstaller) Apply(context.Context, update.Artifact) error { return nil }
func (i *rollbackInstaller) Restore(context.Context) error                { i.restored = true; return nil }

type rollbackRestarter struct{ restarted bool }

func (r *rollbackRestarter) Restart(context.Context) error { r.restarted = true; return nil }

func TestRollbackRestoresBeforeAnyProbe(t *testing.T) {
	i, r := &rollbackInstaller{}, &rollbackRestarter{}
	if err := restoreBeforeProbe(context.Background(), i, r); err != nil || !i.restored || !r.restarted {
		t.Fatal(err, i, r)
	}
}

func TestUpdateChecksUseProtectedCapabilityDenialEndpoint(t *testing.T) {
	checks := updateChecks(config.Config{Domain: "apps.tinker.example.test", ListenHTTP: ":80", ListenHTTPS: ":443"}, "payroll", "/usr/local/bin/tinkercloud", "/etc/tinkercloud/config.yaml")
	if len(checks) != 4 {
		t.Fatalf("health checks = %d", len(checks))
	}
	readiness, ok := checks[0].(update.ListenerHealth)
	if !ok {
		t.Fatalf("listener readiness check = %T", checks[0])
	}
	if strings.Join(readiness.Addresses, ",") != ":80,:443" {
		t.Fatalf("listener readiness addresses = %v", readiness.Addresses)
	}
	if _, ok := checks[1].(update.ExecHealth); !ok {
		t.Fatalf("doctor check = %T", checks[1])
	}
	if _, ok := checks[2].(update.HTTPHealth); !ok {
		t.Fatalf("public health check = %T", checks[2])
	}
	denial, ok := checks[3].(update.AnonymousDenyHealth)
	if !ok {
		t.Fatalf("anonymous denial predicate = %T", checks[3])
	}
	want := "https://payroll.apps.tinker.example.test/_tinker/api/v1/capabilities"
	if denial.URL != want {
		t.Fatalf("denial URL = %q, want %q", denial.URL, want)
	}
	parsed, err := url.Parse(denial.URL)
	if err != nil || parsed.Host != "payroll.apps.tinker.example.test" || parsed.Path != updateAnonymousDenyPath {
		t.Fatalf("denial URL lost app-host binding: %q", denial.URL)
	}
}
