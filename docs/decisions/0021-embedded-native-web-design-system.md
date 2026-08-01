# ADR 0021: Embedded native web design system

**Status:** Accepted for V1

## Context

Tinkercloud has server-rendered platform and app-login pages, plus operational
forms that must remain understandable without JavaScript. Their original styles
were isolated inline declarations and did not express the Tinkercloud product
character or a shared state language.

The PRD requires local embedded assets, minimal JavaScript, no frontend build
dependency, and no second web server. Security-sensitive forms must remain
ordinary server-authorized commands. The gateway's strict CSP deliberately does
not permit arbitrary inline code, so an embedded asset needs an exact CSP
source rather than a broad exception.

## Decision

Tinkercloud uses one dependency-free native web design system in the root `web`
package. Its canonical CSS and cloud mark are embedded into the server binary
and injected as trusted static template content.

The package also derives stable SHA-256 CSP source expressions from the exact
embedded stylesheet and optional interaction helper. Gateway-owned response
headers include those exact hashes in `style-src` and `script-src`. The helpers
are exported so every native renderer and the gateway use the same bytes and
the same hashes. Neither directive permits `unsafe-inline`, remote hosts, or a
new static-asset route.

The system provides stable `--tinker-*` tokens and `tinker-*` HTML classes for
native pages. It uses local system font stacks and semantic HTML. It adds no
asset route, public listener, remote request, client-side router, or browser
authorization state.

Platform templates and app-origin login templates consume the same functions.
Security-relevant behavior remains in existing typed Go handlers and services;
CSS, template structure, disclosure widgets, and optional JavaScript are never
authorization controls.

## Consequences

- Login, dashboard, token, diagnostics, and future native pages share a small
  visual and accessibility vocabulary.
- The server remains one self-contained binary with a strict CSP whose only
  inline allowances are hashes of compile-time native UI bytes.
- Changing the stylesheet or interaction helper changes its stable CSP source;
  gateway tests must prove those sources are present and must reject
  `unsafe-inline` and remote sources.
- Native UI can evolve without introducing Node, a CDN, or a client framework.
- System fonts approximate the landing page's rounded type rather than
  downloading Fredoka or Nunito.
- Deployed apps do not receive this chrome or stylesheet; origin and ownership
  boundaries remain visually distinct.
