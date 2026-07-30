// Package webui owns the trusted, embedded visual system for Tinkercloud-native
// HTML. It contains no authorization or application state.
package webui

import (
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"html/template"
)

//go:embed assets/tinkercloud.css
var stylesheet string

//go:embed assets/tinkercloud-mark.svg
var cloudMark string

//go:embed assets/tinkercloud.js
var interactions string

// FuncMap exposes only trusted, compile-time assets. User-controlled values
// must continue through html/template's normal escaping.
func FuncMap() template.FuncMap {
	return template.FuncMap{
		"tinkerCSS":  func() template.CSS { return template.CSS(stylesheet) },
		"tinkerJS":   func() template.JS { return template.JS(interactions) },
		"tinkerMark": func() template.HTML { return template.HTML(cloudMark) },
		"tinkerQR":   qrCodeData,
	}
}

// TinkerStyleCSPSource returns the exact CSP hash source for a template that
// embeds Stylesheet in a style element. It lets the gateway allow the trusted,
// compile-time bytes without allowing arbitrary inline style.
func TinkerStyleCSPSource() string {
	return cspHash(stylesheet)
}

// TinkerScriptCSPSource returns the exact CSP hash source for a template that
// embeds Interactions in a script element. It lets the gateway allow the
// trusted, compile-time helper without unsafe-inline or a new asset route.
func TinkerScriptCSPSource() string {
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
