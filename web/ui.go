// Package webui owns the trusted, embedded visual system for TinyHost-native
// HTML. It contains no authorization or application state.
package webui

import (
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"html/template"
)

//go:embed assets/tinyhost.css
var stylesheet string

//go:embed assets/tiny-cloud-mark.svg
var cloudMark string

//go:embed assets/tinyhost.js
var interactions string

// FuncMap exposes only trusted, compile-time assets. User-controlled values
// must continue through html/template's normal escaping.
func FuncMap() template.FuncMap {
	return template.FuncMap{
		"tinyCSS":  func() template.CSS { return template.CSS(stylesheet) },
		"tinyJS":   func() template.JS { return template.JS(interactions) },
		"tinyMark": func() template.HTML { return template.HTML(cloudMark) },
	}
}

// TinyStyleCSPSource returns the exact CSP hash source for a template that
// embeds Stylesheet in a style element. It lets the gateway allow the trusted,
// compile-time bytes without allowing arbitrary inline style.
func TinyStyleCSPSource() string {
	return cspHash(stylesheet)
}

// TinyScriptCSPSource returns the exact CSP hash source for a template that
// embeds Interactions in a script element. It lets the gateway allow the
// trusted, compile-time helper without unsafe-inline or a new asset route.
func TinyScriptCSPSource() string {
	return cspHash(interactions)
}

func cspHash(asset string) string {
	digest := sha256.Sum256([]byte(asset))
	return "'sha256-" + base64.StdEncoding.EncodeToString(digest[:]) + "'"
}

// Stylesheet returns the canonical source for tests and non-template renderers.
func Stylesheet() string {
	return stylesheet
}

// CloudMark returns the canonical SVG source for tests and non-template
// renderers. Templates should normally use FuncMap.
func CloudMark() string {
	return cloudMark
}

// Interactions returns the optional progressive-enhancement helper. It contains
// presentation behavior only; callers must not use it as an authorization or
// mutation control.
func Interactions() string {
	return interactions
}
