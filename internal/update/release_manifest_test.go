package update

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"testing"
)

func signedBytes(t *testing.T, private ed25519.PrivateKey, version string, payload []byte) (Artifact, []byte, []byte) {
	t.Helper()
	digest := sha256.Sum256(payload)
	metadata := []byte(fmt.Sprintf(`{"version":"%s","api":"1","schema":"1","sha256":"%s"}`, version, hex.EncodeToString(digest[:])))
	signature := ed25519.Sign(private, []byte(version+"\n1\n1\n"+hex.EncodeToString(digest[:])))
	artifact, err := ArtifactFrom(payload, metadata, []byte(base64.StdEncoding.EncodeToString(signature)))
	if err != nil {
		t.Fatal(err)
	}
	return artifact, metadata, []byte(base64.StdEncoding.EncodeToString(signature))
}

func TestVerifyReleaseManifestBindsCompatibilityAndServer(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	server, _, _ := signedBytes(t, private, "0.1.0", []byte("server"))
	raw := []byte(fmt.Sprintf(`{"schema":"2","version":"0.1.0","compatibility":{"server_version":"0.1.0","control_api":{"version":"1","client":{"min_inclusive":"0.1.0","max_exclusive":"1.0.0"}},"app_api":{"version":"1","client":{"min_inclusive":"0.1.0","max_exclusive":"1.0.0"}},"schema_version":"1","release_manifest_schema":"2"},"files":{"tinkercloud-linux-amd64":"%s"}}`, hex.EncodeToString(server.Digest[:])))
	_, metadata, signature := signedBytes(t, private, "0.1.0", raw)
	if _, err = VerifyReleaseManifest(public, raw, metadata, signature, server); err != nil {
		t.Fatal(err)
	}
	tampered := append([]byte(nil), raw...)
	tampered[11] = '1'
	if _, err = VerifyReleaseManifest(public, tampered, metadata, signature, server); err == nil {
		t.Fatal("tampered manifest accepted")
	}
}
