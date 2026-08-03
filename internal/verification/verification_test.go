package verification

import (
	"strings"
	"testing"
)

func TestProbePassedIsPostureAware(t *testing.T) {
	hash := strings.Repeat("a", 64)
	if !(Probe{URL: "https://app.test", AnonymousDenied: true, AuthenticatedHealthy: true, ReservedDenied: true}).Passed() {
		t.Fatal("private compatibility probe rejected")
	}
	if (Probe{URL: "https://app.test", AnonymousDenied: true, AuthenticatedHealthy: true}).Passed() {
		t.Fatal("private probe accepted without reserved-route denial")
	}
	if (Probe{URL: "https://app.test", Posture: "public_static", AnonymousDenied: true, AuthenticatedHealthy: true}).Passed() {
		t.Fatal("public probe accepted anonymous denial without immutable evidence")
	}
	if !(Probe{URL: "https://app.test", Posture: "public_static", PublicReachable: true, ReservedDenied: true, RootSHA256: hash, RootBytes: 32769}).Passed() {
		t.Fatal("public immutable evidence rejected")
	}
	if (Probe{URL: "https://app.test", Posture: "public_static", RootSHA256: strings.ToUpper(hash), RootBytes: 1}).Passed() {
		t.Fatal("uppercase public hash accepted")
	}
	if (Probe{URL: "https://app.test", Posture: "public_static", PublicReachable: true, ReservedDenied: true, RootSHA256: hash, RootBytes: 1, AssetPath: "../asset.js", AssetSHA256: hash, AssetBytes: 1}).Passed() {
		t.Fatal("noncanonical asset path accepted")
	}
}
