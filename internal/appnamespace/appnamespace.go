// Package appnamespace validates app slugs that become public hostname labels.
//
// A slug is deliberately accepted only in its canonical lowercase ASCII form.
// That prevents validation from silently changing the stable origin that a
// deployer requested while still making reserved-label checks case-insensitive.
package appnamespace

import (
	"errors"
	"strings"
)

var (
	// ErrInvalidSlug means the input is not one canonical DNS hostname label
	// accepted for a TinyHost app origin.
	ErrInvalidSlug = errors.New("invalid app slug")
	// ErrReservedSlug means the canonical hostname label belongs to TinyHost,
	// rather than to a deployer app.
	ErrReservedSlug = errors.New("reserved app slug")
)

var reserved = map[string]struct{}{
	"admin":   {},
	"api":     {},
	"auth":    {},
	"status":  {},
	"www":     {},
	"docs":    {},
	"install": {},
}

// Validate returns the canonical slug when value is a valid, canonical app
// hostname label. Reserved labels are detected after ASCII case folding so
// callers can reliably present the stable ErrReservedSlug result for variants
// such as "ADMIN" without accepting or silently rewriting that input.
func Validate(value string) (string, error) {
	canonical := strings.ToLower(value)
	if _, found := reserved[canonical]; found {
		return "", ErrReservedSlug
	}
	if value != canonical || !validCanonicalLabel(canonical) {
		return "", ErrInvalidSlug
	}
	return canonical, nil
}

// Valid reports whether value is a permitted canonical app slug.
func Valid(value string) bool {
	_, err := Validate(value)
	return err == nil
}

func validCanonicalLabel(value string) bool {
	if len(value) == 0 || len(value) > 63 || value[0] == '-' || value[len(value)-1] == '-' {
		return false
	}
	for _, b := range []byte(value) {
		if (b < 'a' || b > 'z') && (b < '0' || b > '9') && b != '-' {
			return false
		}
	}
	return true
}
