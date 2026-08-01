package releases

import (
	"errors"
	"github.com/ChrisMarxDev/tinkercloud/internal/appnamespace"
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

func TestManifestRejectsReservedAppNamespaceLabels(t *testing.T) {
	for _, name := range []string{"admin", "api", "auth", "status", "www", "docs", "install", "ADMIN"} {
		if _, err := ParseManifest([]byte("version: 1\nname: " + name + "\n")); !errors.Is(err, ErrManifest) {
			t.Errorf("ParseManifest(%q) = %v; want ErrManifest", name, err)
		}
	}
	if _, err := GenerateManifest(Manifest{Version: 1, Name: "admin"}); !errors.Is(err, ErrManifest) {
		t.Fatalf("GenerateManifest reserved name = %v; want ErrManifest", err)
	}
	if !appnamespace.Valid("normal-app") {
		t.Fatal("normal app slug unexpectedly invalid")
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
