package main

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/ChrisMarxDev/tinkercloud/internal/config"
	"github.com/ChrisMarxDev/tinkercloud/internal/deployments"
)

func TestCandidateProbeGateLogsOnlyFixedFailureStage(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	record := deployments.Record{}

	if candidateProbePassed(context.Background(), logger, config.Config{Domain: "tinker.test", SessionCookie: "__Host-tinker_app"}, t.TempDir(), record) {
		t.Fatal("candidate without immutable identity passed")
	}
	got := output.String()
	if !strings.Contains(got, "service.candidate_probe_release_evidence") || !strings.Contains(got, `"outcome":"failed"`) {
		t.Fatalf("missing fixed failure stage: %s", got)
	}
	for _, forbidden := range []string{"candidate missing identity", "tinker.yaml", "sha256", "manifest_json", "provider"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("candidate diagnostic leaked %q: %s", forbidden, got)
		}
	}
}
