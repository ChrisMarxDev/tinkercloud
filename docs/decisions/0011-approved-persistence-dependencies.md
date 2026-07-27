# ADR 0011: Approved persistence and configuration dependencies

## Status

Accepted with explicit operator approval.

## Decision

Use `modernc.org/sqlite v1.54.0` for the CGO-free embedded SQLite adapter and
`go.yaml.in/yaml/v3 v3.0.4` only for strict configuration/manifest decoding.
Use `github.com/coder/websocket v1.8.15` for the bounded live transport and
`golang.org/x/crypto v0.54.0` for the ACME client.

## Justification and licenses

- modernc.org/sqlite — embedded SQLite persistence; BSD-3-Clause.
- go.yaml.in/yaml/v3 — strict YAML parsing; MIT and Apache-2.0.
- github.com/coder/websocket — bounded WebSocket implementation; ISC.
- golang.org/x/crypto — ACME certificate automation; BSD-3-Clause.

All versions are pinned in `go.mod`; upgrades require review and regenerated
verification evidence. No dependency gives browser code database access.

The 2026-07-27 `govulncheck` audit reports no reachable vulnerabilities.
Module advisory `GO-2026-5932` concerns the unimported `x/crypto/openpgp`
package and has no module-wide fixed version; it must be re-evaluated if the
import graph or ACME dependency changes.
