# ADR 0044: Bind distribution compatibility in the signed release manifest

Status: Accepted for distribution preparation

## Context

Tinkercloud already signs each payload with version, API, schema, and digest
labels. Package-manager candidates and self-update need a richer compatibility
statement, but changing the artifact signature bytes would invalidate existing
installers and verifiers.

## Decision

Keep the V1 per-artifact metadata and signature format unchanged. Advance the
signed complete-release manifest to schema 2 and add one strict compatibility
matrix covering server, CLI, SDK, control API, app API, persistence schema, and
manifest schema. Compile the same policy into runtime negotiation and update
preflight.

Generate npm and Homebrew inputs only from a completely verified signed release.
Generators write local candidates and contain no registry, tap, hosted-release,
or channel mutation.

## Consequences

Existing artifact verification remains stable. Compatibility becomes
tamper-evident and reviewable before install/update. The locked product
identity and a future release-origin decision can update public
package/origin metadata without redesigning the release format. Advancing a
minimum client/SDK version becomes a deliberate contract change with denial
tests and migration evidence.
