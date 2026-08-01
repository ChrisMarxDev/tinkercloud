package appnamespace

import (
	"errors"
	"testing"
)

func TestValidateAcceptsExistingCanonicalAppSlugs(t *testing.T) {
	for _, value := range []string{"a", "demo", "invoice-review", "a1-b2", "tinker", "tinkercloud", "x23456789012345678901234567890123456789012345678901234567890123"} {
		got, err := Validate(value)
		if err != nil || got != value {
			t.Fatalf("Validate(%q) = %q, %v", value, got, err)
		}
		if !Valid(value) {
			t.Fatalf("Valid(%q) = false", value)
		}
	}
}

func TestValidateRejectsReservedLabelsCaseInsensitively(t *testing.T) {
	for _, value := range []string{
		"admin", "ADMIN", "AdMiN",
		"api", "API",
		"auth", "AUTH",
		"status", "STATUS",
		"www", "WWW",
		"docs", "DOCS",
		"install", "INSTALL",
	} {
		if got, err := Validate(value); got != "" || !errors.Is(err, ErrReservedSlug) {
			t.Errorf("Validate(%q) = %q, %v; want ErrReservedSlug", value, got, err)
		}
	}
}

func TestValidateRejectsMalformedAndCanonicalizationTricks(t *testing.T) {
	for _, value := range []string{
		"", "-demo", "demo-",
		"Demo", " demo", "demo ", "demo\n", "demo.", "demo..app", "demo/app", "demo%2Fapp", "demo:443",
		"аdmin", "admın", "ＡＤＭＩＮ", "admin\x00", "a_b", string(make([]byte, 64)),
	} {
		if got, err := Validate(value); got != "" || !errors.Is(err, ErrInvalidSlug) {
			t.Errorf("Validate(%q) = %q, %v; want ErrInvalidSlug", value, got, err)
		}
	}
	if got, err := Validate("demo--app"); err != nil || got != "demo--app" {
		t.Fatalf("double-hyphen normal label rejected: %q, %v", got, err)
	}
}
