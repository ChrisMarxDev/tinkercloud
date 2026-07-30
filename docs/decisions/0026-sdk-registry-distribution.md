# ADR 0026: One SDK API across npm and JSR

**Status:** Accepted for preparation; publication pending namespace ownership

## Context

Tinkercloud app creators use several JavaScript package managers. Building a
different SDK for each manager would multiply release evidence and create
opportunities for API or security drift. Deno and TypeScript-first users also
benefit from a source-native registry.

The repository is still private and the canonical organization and package
namespace have not been confirmed. No registry publication is authorized yet.

## Decision

Prepare `@tinkercloud/sdk` for two registry artifacts with one name, version, and
public API:

- npm receives the compiled ESM runtime, declarations, README, and license;
- JSR receives the TypeScript source, README, and license; and
- npm, pnpm, Yarn, Bun, and Deno all consume the same npm-registry tarball
  rather than receiving package-manager-specific builds.

Package manifests and the exported `SDK_VERSION` must remain synchronized.
Local and CI preparation use dry runs only. When publication is authorized, it
must originate from a reviewed public release tag through short-lived trusted
publishing with provenance, not a repository or developer token.

## Consequences

- Consumers choose their package manager without changing the SDK artifact.
- JSR and npm can serve their native module formats without creating two APIs.
- Release checks must install the packed npm artifact and validate both
  manifests.
- The release builder rejects a server/CLI release version that differs from
  the SDK version.
- Namespace ownership and canonical repository metadata remain explicit
  open-source publication blockers.
