package update

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestMetadataStrict(t *testing.T) {
	m := []byte(`{"version":"1","api":"1","schema":"1","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`)
	if _, err := ParseMetadata(m); err != nil {
		t.Fatal(err)
	}
	for _, input := range [][]byte{[]byte(`{"version":"1","api":"1","schema":"1","sha256":"aa","x":1}`), []byte(`{}`), []byte(strings.Repeat("x", 4097))} {
		if _, err := ParseMetadata(input); err == nil {
			t.Fatal(string(input))
		}
	}
	if _, err := ArtifactFrom([]byte("x"), m, []byte(base64.StdEncoding.EncodeToString(make([]byte, 64)))); err != nil {
		t.Fatal(err)
	}
}
