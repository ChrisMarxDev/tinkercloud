package update

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"io"
	"regexp"

	"github.com/tinyhost/tiny/internal/compatibility"
)

type ReleaseManifest struct {
	Schema        string               `json:"schema"`
	Version       string               `json:"version"`
	Compatibility compatibility.Matrix `json:"compatibility"`
	Files         map[string]string    `json:"files"`
}

var releaseFileName = regexp.MustCompile(`^[-A-Za-z0-9._+]+$`)
var releaseDigest = regexp.MustCompile(`^[0-9a-f]{64}$`)

// VerifyReleaseManifest authenticates the complete-release compatibility
// statement and binds it to the already authenticated server artifact.
func VerifyReleaseManifest(pub ed25519.PublicKey, raw, metadata, signature []byte, server Artifact) (compatibility.Matrix, error) {
	signed, err := ArtifactFrom(raw, metadata, signature)
	if err != nil || Verify(pub, signed) != nil || !compatibility.CompatibleArtifact(signed.Version, signed.API, signed.Schema) {
		return compatibility.Matrix{}, ErrMetadata
	}
	var manifest ReleaseManifest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&manifest) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return compatibility.Matrix{}, ErrMetadata
	}
	expected := compatibility.Current(manifest.Version)
	if manifest.Schema != compatibility.ReleaseManifestSchema ||
		manifest.Version != server.Version ||
		manifest.Compatibility != expected ||
		len(manifest.Files) == 0 {
		return compatibility.Matrix{}, ErrMetadata
	}
	for name, digest := range manifest.Files {
		if !releaseFileName.MatchString(name) || !releaseDigest.MatchString(digest) {
			return compatibility.Matrix{}, ErrMetadata
		}
	}
	if manifest.Files["tinyhost-linux-amd64"] != hex.EncodeToString(server.Digest[:]) {
		return compatibility.Matrix{}, ErrMetadata
	}
	return manifest.Compatibility, nil
}
