package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"flag"
	"github.com/ChrisMarxDev/tinkercloud/internal/update"
	"io"
	"os"
)

var releasePublicKeyBase64 string

func loadPinnedKey() (ed25519.PublicKey, error) {
	raw, e := base64.StdEncoding.DecodeString(releasePublicKeyBase64)
	if e != nil || len(raw) != ed25519.PublicKeySize {
		return nil, errors.New("pinned key unavailable")
	}
	return ed25519.PublicKey(raw), nil
}
func loadVerifiedArtifact(binary, metadata, signature string) (update.Artifact, ed25519.PublicKey, error) {
	key, e := loadPinnedKey()
	if e != nil {
		return update.Artifact{}, nil, e
	}
	b, e := os.ReadFile(binary)
	if e != nil || len(b) > 100<<20 {
		return update.Artifact{}, nil, errors.New("artifact unavailable")
	}
	m, e := os.ReadFile(metadata)
	if e != nil || int64(len(m)) > update.MaxMetadataBytes {
		return update.Artifact{}, nil, errors.New("metadata unavailable")
	}
	s, e := os.ReadFile(signature)
	if e != nil || int64(len(s)) > update.MaxSignatureBytes {
		return update.Artifact{}, nil, errors.New("signature unavailable")
	}
	return verifiedArtifactBytes(b, m, s, key)
}

func verifiedArtifactBytes(binary, metadata, signature []byte, key ed25519.PublicKey) (update.Artifact, ed25519.PublicKey, error) {
	a, e := update.ArtifactFrom(binary, metadata, signature)
	if e != nil || update.Verify(key, a) != nil {
		return update.Artifact{}, nil, errors.New("artifact invalid")
	}
	return a, key, nil
}

func runVerifyArtifact(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("verify-artifact", flag.ContinueOnError)
	bin := fs.String("binary", "", "")
	meta := fs.String("metadata", "", "")
	sig := fs.String("signature", "", "")
	if fs.Parse(args) != nil || *bin == "" || *meta == "" || *sig == "" {
		return errors.New("tinkercloud: invalid_arguments")
	}
	_, _, e := loadVerifiedArtifact(*bin, *meta, *sig)
	if e != nil {
		return errors.New("tinkercloud: verification_failed")
	}
	_, e = io.WriteString(out, "artifact verified\n")
	return e
}
