package verification

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ChrisMarxDev/tinkercloud/internal/config"
	"github.com/ChrisMarxDev/tinkercloud/internal/deployments"
	"github.com/ChrisMarxDev/tinkercloud/internal/releases"
)

func TestProbeCandidatePrivateRelease(t *testing.T) {
	root := t.TempDir()
	record := realisticPrivateCandidate(t, root)
	// Activation receives the durable JSON reconstruction rather than the
	// in-memory value produced directly by YAML parsing.
	raw, err := json.Marshal(record.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	record.Manifest = releases.Manifest{}
	if err := json.Unmarshal(raw, &record.Manifest); err != nil {
		t.Fatal(err)
	}
	probe, err := ProbeCandidate(context.Background(), config.Config{
		Domain:        "tinker.test",
		SessionCookie: "__Host-tinker_app",
	}, root, record)
	if err != nil {
		t.Fatal(err)
	}
	if !probe.Passed() || !probe.AnonymousDenied || !probe.AuthenticatedHealthy {
		t.Fatalf("private LLM candidate did not produce complete denial and health evidence: %+v", probe)
	}
}

func TestProbeCandidatePrivateReleaseProvesAssetAndReservedDenials(t *testing.T) {
	root := t.TempDir()
	record := realisticPrivateCandidate(t, root)
	probe, err := ProbeCandidate(context.Background(), config.Config{
		Domain:        "tinker.test",
		SessionCookie: "__Host-tinker_app",
	}, root, record)
	if err != nil {
		t.Fatal(err)
	}
	if !probe.Passed() || !probe.AnonymousDenied || !probe.ReservedDenied || probe.AssetPath != "app.js" || probe.AssetBytes == 0 {
		t.Fatalf("private asset/reserved denial evidence incomplete: %+v", probe)
	}
	if probe.Detail != "" {
		t.Fatalf("private probe reflected release bytes: %q", probe.Detail)
	}
}

func TestProbeCandidatePrivateSingleFileReleaseDoesNotRequireAssetEvidence(t *testing.T) {
	root := t.TempDir()
	record := realisticPrivateCandidateFiles(t, root, false)
	probe, err := ProbeCandidate(context.Background(), config.Config{
		Domain:        "tinker.test",
		SessionCookie: "__Host-tinker_app",
	}, root, record)
	if err != nil {
		t.Fatal(err)
	}
	if !probe.Passed() || !probe.AnonymousDenied || !probe.ReservedDenied || !probe.AuthenticatedHealthy || probe.AssetPath != "" || probe.AssetBytes != 0 {
		t.Fatalf("single-file private evidence was fabricated or incomplete: %+v", probe)
	}
}

func TestProbeCandidatePrivateRejectsAlteredReleaseEvidence(t *testing.T) {
	root := t.TempDir()
	record := realisticPrivateCandidate(t, root)
	record.Files[0].Hash = "altered"

	if probe, err := ProbeCandidate(context.Background(), config.Config{
		Domain:        "tinker.test",
		SessionCookie: "__Host-tinker_app",
	}, root, record); err == nil || probe.Passed() || CandidateProbeStage(err) != CandidateProbeReleaseEvidence {
		t.Fatalf("altered immutable evidence reached a passing probe: probe=%+v err=%v", probe, err)
	}
}

func realisticPrivateCandidate(t *testing.T, dataRoot string) deployments.Record {
	return realisticPrivateCandidateFiles(t, dataRoot, true)
}

func realisticPrivateCandidateFiles(t *testing.T, dataRoot string, withAssets bool) deployments.Record {
	t.Helper()
	staging := t.TempDir()
	manifest := "version: 2\nname: llm-chat\nbuild:\n  output: dist\naccess:\n  mode: private\n"
	files := map[string][]byte{
		"index.html":  []byte("<!doctype html><title>LLM chat</title><main>private candidate marker</main>"),
		"tinker.yaml": []byte(manifest),
	}
	if withAssets {
		files["app.js"] = []byte("import { Tinker } from './tinker-sdk.js';\nexport const app = new Tinker();\n")
		files["errors.js"] = []byte("export const safeMessage = () => 'Chat is temporarily unavailable.';\n")
		files["history.js"] = []byte("export const history = [];\n")
		files["styles.css"] = []byte("main { max-width: 42rem; margin: auto; }\n")
		files["tinker-sdk.js"] = []byte("export class Tinker {}\n")
	}
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(staging, name), contents, 0600); err != nil {
			t.Fatal(err)
		}
	}
	evidence, err := releases.Inspect(staging)
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(dataRoot, "releases", "app_llm", evidence.Hash)
	if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(staging, destination); err != nil {
		t.Fatal(err)
	}
	for name := range files {
		if err := os.Chmod(filepath.Join(destination, name), 0444); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(destination, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(destination, 0755)
		for name := range files {
			_ = os.Chmod(filepath.Join(destination, name), 0644)
		}
	})
	parsed, err := releases.ParseManifest([]byte(manifest))
	if err != nil {
		t.Fatal(err)
	}
	return deployments.Record{
		Deployment: releases.Deployment{
			ID:          "dep_llm",
			AppID:       "app_llm",
			ReleaseHash: evidence.Hash,
			State:       releases.Verified,
		},
		AppSlug:  "llm-chat",
		Files:    evidence.Files,
		Manifest: parsed,
	}
}
