package persistence

import (
	"strings"
	"testing"

	"github.com/ChrisMarxDev/tinkercloud/internal/deployments"
	"github.com/ChrisMarxDev/tinkercloud/internal/releases"
)

func TestActivationReceiptSkipsUnservablePublicAssetEvidence(t *testing.T) {
	hash := strings.Repeat("a", 64)
	r := deployments.Record{Deployment: releases.Deployment{ID: "dep"}, AppSlug: "demo", Manifest: releases.Manifest{AccessMode: "public"}, Files: []releases.File{{Path: ".hidden", Hash: hash, Size: 1}, {Path: "assets/app.js.map", Hash: hash, Size: 2}, {Path: "assets/app.js", Hash: hash, Size: 3}, {Path: "index.html", Hash: hash, Size: 4}}}
	result, err := activationReceipt(r, "apps.test")
	if err != nil || result.PublicStatic == nil || result.PublicStatic.AssetPath != "assets/app.js" || result.PublicStatic.AssetBytes != 3 {
		t.Fatalf("receipt=%+v err=%v", result, err)
	}
}
