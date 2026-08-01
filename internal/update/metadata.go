package update

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
)

var ErrMetadata = errors.New("invalid artifact metadata")

type Metadata struct {
	Version string `json:"version"`
	API     string `json:"api"`
	Schema  string `json:"schema"`
	SHA256  string `json:"sha256"`
}

func ParseMetadata(b []byte) (Metadata, error) {
	var m Metadata
	if len(b) == 0 || len(b) > 4096 {
		return m, ErrMetadata
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(&m) != nil || d.Decode(&struct{}{}) != io.EOF || m.Version == "" || m.API == "" || m.Schema == "" || len(m.Version) > 128 || len(m.API) > 128 || len(m.Schema) > 128 || len(m.SHA256) != 64 {
		return Metadata{}, ErrMetadata
	}
	if _, e := hex.DecodeString(m.SHA256); e != nil || m.SHA256 != string(bytes.ToLower([]byte(m.SHA256))) {
		return Metadata{}, ErrMetadata
	}
	return m, nil
}
func ArtifactFrom(binary, metadata, signature []byte) (Artifact, error) {
	m, e := ParseMetadata(metadata)
	if e != nil {
		return Artifact{}, e
	}
	sig, e := base64.StdEncoding.DecodeString(string(signature))
	if e != nil || len(sig) != ed25519.SignatureSize {
		return Artifact{}, ErrMetadata
	}
	digest, e := hex.DecodeString(m.SHA256)
	if e != nil {
		return Artifact{}, ErrMetadata
	}
	var d [32]byte
	copy(d[:], digest)
	return Artifact{Version: m.Version, API: m.API, Schema: m.Schema, Bytes: binary, Signature: sig, Digest: d}, nil
}
