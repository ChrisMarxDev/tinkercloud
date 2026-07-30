package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestRunVerifyArtifact(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	old := releasePublicKeyBase64
	defer func() { releasePublicKeyBase64 = old }()
	releasePublicKeyBase64 = base64.StdEncoding.EncodeToString(pub)
	dir := t.TempDir()
	bin := []byte("artifact")
	d := sha256.Sum256(bin)
	meta := []byte(`{"version":"1","api":"1","schema":"1","sha256":"` + hex.EncodeToString(d[:]) + `"}`)
	sig := ed25519.Sign(priv, []byte("1\n1\n1\n"+hex.EncodeToString(d[:])))
	for n, b := range map[string][]byte{"bin": bin, "meta": meta, "sig": []byte(base64.StdEncoding.EncodeToString(sig))} {
		if e := os.WriteFile(filepath.Join(dir, n), b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	var out bytes.Buffer
	if e := runVerifyArtifact([]string{"--binary", filepath.Join(dir, "bin"), "--metadata", filepath.Join(dir, "meta"), "--signature", filepath.Join(dir, "sig")}, &out); e != nil {
		t.Fatal(e)
	}
	releasePublicKeyBase64 = ""
	if e := runVerifyArtifact([]string{"--binary", filepath.Join(dir, "bin"), "--metadata", filepath.Join(dir, "meta"), "--signature", filepath.Join(dir, "sig")}, &out); e == nil {
		t.Fatal("missing key")
	}
}
