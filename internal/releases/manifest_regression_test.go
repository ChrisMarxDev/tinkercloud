package releases

import (
	"strings"
	"testing"
)

// A manifest is a signed/reviewed deployment input. Duplicate nested keys must
// fail rather than silently allowing the last occurrence to change a grant.
func TestManifestRejectsDuplicateNestedFeature(t *testing.T) {
	_, err := ParseManifest([]byte("version: 1\nname: demo\nfeatures:\n  kv: false\n  kv: true\n"))
	if err == nil {
		t.Fatal("duplicate capability key was accepted")
	}
}

func TestManifestDescriptionIsTrimmedBoundedAndSingleLine(t *testing.T) {
	m, err := ParseManifest([]byte("version: 1\nname: demo\ndescription: '  A useful dashboard  '\n"))
	if err != nil || m.Description != "A useful dashboard" {
		t.Fatalf("description = %#v, %v", m.Description, err)
	}
	for _, value := range []string{"one\ntwo", "one\ttwo", "one two", "one two", string([]byte{0xff}), strings.Repeat("界", 281)} {
		if _, err := NormalizeDescription(value); err == nil {
			t.Fatalf("accepted invalid description %q", value)
		}
	}
}
