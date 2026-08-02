package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ChrisMarxDev/tinkercloud/internal/compatibility"
)

func TestDiscoverGitHubReleaseUsesOnlyExactAssetsAndChannel(t *testing.T) {
	oldClient, oldValidator := updateHTTPClient, githubDiscoveryValidator
	defer func() { updateHTTPClient, githubDiscoveryValidator = oldClient, oldValidator }()
	githubDiscoveryValidator = func(string) error { return nil }
	assets := func(v string) string {
		var parts []string
		for _, n := range []string{"tinkercloud-linux-amd64", "tinkercloud-linux-amd64.metadata.json", "tinkercloud-linux-amd64.signature", "release-manifest.json", "release-manifest.json.metadata.json", "release-manifest.json.signature"} {
			parts = append(parts, `{"browser_download_url":"https://github.com/ChrisMarxDev/tinkercloud/releases/download/v`+v+`/`+n+`"}`)
		}
		return strings.Join(parts, ",")
	}
	body := `[{"tag_name":"v1.2.0","prerelease":false,"draft":false,"assets":[` + assets("1.2.0") + `]},{"tag_name":"v1.3.0","prerelease":true,"draft":false,"assets":[` + assets("1.3.0") + `]},{"tag_name":"v9.0.0","prerelease":false,"draft":true,"assets":[` + assets("9.0.0") + `]}]`
	updateHTTPClient = commandDoer(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Request: r, Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body))}, nil
	})
	current, _ := compatibility.Parse("1.0.0")
	_, stable, err := discoverGitHubRelease(context.Background(), "stable", current)
	if err != nil || stable != "1.2.0" {
		t.Fatal(stable, err)
	}
	_, beta, err := discoverGitHubRelease(context.Background(), "beta", current)
	if err != nil || beta != "1.3.0" {
		t.Fatal(beta, err)
	}
}

func TestGitHubAssetsExactRejectsDuplicateRequiredAsset(t *testing.T) {
	version := "1.2.3"
	var assets []githubAsset
	for _, name := range []string{"tinkercloud-linux-amd64", "tinkercloud-linux-amd64.metadata.json", "tinkercloud-linux-amd64.signature", "release-manifest.json", "release-manifest.json.metadata.json", "release-manifest.json.signature"} {
		assets = append(assets, githubAsset{BrowserDownloadURL: "https://github.com/ChrisMarxDev/tinkercloud/releases/download/v" + version + "/" + name})
	}
	if !githubAssetsExact(assets, version) {
		t.Fatal("complete exact asset set rejected")
	}
	assets = append(assets, assets[0])
	if githubAssetsExact(assets, version) {
		t.Fatal("duplicate required asset accepted")
	}
}

func TestReadUpdateScheduleStateRequiresPrivateCanonicalJSON(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "schedule.json")
	b, err := json.Marshal(updateScheduleState{Channel: "beta", Enabled: true, InstalledVersion: "1.2.3", Updated: time.Now().UTC().Format(time.RFC3339)})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	if state, err := readUpdateScheduleState(path); err != nil || !state.Enabled || state.Channel != "beta" {
		t.Fatalf("valid state rejected: %#v %v", state, err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := readUpdateScheduleState(path); err == nil {
		t.Fatal("over-permissive state accepted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "missing"), path); err != nil {
		t.Fatal(err)
	}
	if _, err := readUpdateScheduleState(path); err == nil {
		t.Fatal("symlink state accepted")
	}
}

func TestDiscoverGitHubReleaseRejectsMalformedAssets(t *testing.T) {
	oldClient, oldValidator := updateHTTPClient, githubDiscoveryValidator
	defer func() { updateHTTPClient, githubDiscoveryValidator = oldClient, oldValidator }()
	githubDiscoveryValidator = func(string) error { return nil }
	body := `[{"tag_name":"v1.2.0","prerelease":false,"draft":false,"assets":[{"browser_download_url":"https://evil.example/x"}]}]`
	updateHTTPClient = commandDoer(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Request: r, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	current, _ := compatibility.Parse("1.0.0")
	if _, _, err := discoverGitHubRelease(context.Background(), "stable", current); err == nil {
		t.Fatal("bad discovery accepted")
	}
}
