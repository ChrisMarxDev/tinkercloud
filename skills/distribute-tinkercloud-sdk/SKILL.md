---
name: distribute-tinkercloud-sdk
description: Prepare, verify, or publish the Tinkercloud TypeScript browser client/SDK package to npm and JSR from the same versioned source release. Use for SDK package candidates, npm or JSR publishing, package metadata and export checks, registry parity, provenance, compatibility-range validation, consumer installation tests, or diagnosing client package drift. Do not use for the native Tinker CLI executable; use distribute-tinker-cli.
---

# Distribute the Tinkercloud TypeScript SDK

Keep npm, JSR, runtime exports, examples, and the signed Tinkercloud release on one
version and one source commit.

## Establish authority

Classify the request before doing work:

- **Inspect**: report package and registry readiness without mutation.
- **Prepare**: build, pack, and verify local candidates without publishing.
- **Publish**: proceed only when the user explicitly authorizes the exact
  package identity, version, registries, and tags/channels.

Preparation is not publication authority. The package identity is locked as
`@tinkercloud/sdk`, but registry publication stops with verified local
artifacts until destination ownership and the exact release are approved. The
complete signed GitHub beta below may expose the SDK tarball only when that
separate beta publication is explicitly authorized.

## Load the contract

From the repository root, read in order:

1. `PRINCIPLES.md`
2. `PRD.md`
3. `specs/sdk/distribution-contract.md`
4. `specs/operations/distribution-contract.md`
5. `specs/operations/beta-release-contract.md`
6. `specs/operations/update-compatibility-contract.md`
7. `specs/api/http-contract.md`
8. `docs/architecture/client-sdk.md`
9. `docs/operations/release-pipeline.md`
10. `docs/decisions/0026-sdk-registry-distribution.md`
11. `docs/decisions/0044-signed-distribution-compatibility-manifest.md`
12. `docs/decisions/0046-github-beta-release-channel.md`

Preserve unrelated local changes. Do not change compatibility ranges, public
exports, package identity, or registry metadata as a packaging convenience.

## Prepare the client package

Require strict `MAJOR.MINOR.PATCH`. Confirm exact equality among:

- `sdk/typescript/package.json`;
- `sdk/typescript/jsr.json`;
- `SDK_VERSION` in `sdk/typescript/src/index.ts`;
- the requested release version;
- the signed release manifest and SDK tarball filename.

Confirm `APP_API_VERSION` and the SDK range match
`internal/compatibility` and the signed compatibility manifest. Raising the
minimum supported SDK requires installed-deployment compatibility evidence and
a separate migration decision.

Build and test from the package directory:

```sh
npm ci --prefix sdk/typescript
npm test --prefix sdk/typescript
```

Then build and verify the complete signed release:

```sh
task release:build VERSION="$VERSION" OUTPUT="$RELEASE_DIR"
task release:verify RELEASE_DIR="$RELEASE_DIR"
```

Use `task release:check` for a disposable-key rehearsal. Packing must not
publish to either registry.

## Inspect the package

Prove all of the following:

- The package is ESM and exports the expected JavaScript plus type declarations.
- npm and JSR expose the same public source API and version.
- The npm tarball contains exactly `LICENSE`, `README.md`, `dist/index.d.ts`,
  `dist/index.js`, and `package.json`.
- JSR includes only the declared TypeScript source, README, and license.
- There are no runtime dependencies, peer dependencies, consumer
  preinstall/install/postinstall hooks, CDN imports, server internals, provider
  credentials, deployer tokens, release keys, or generated host configuration.
- `SDK_VERSION` and `APP_API_VERSION` are sent on HTTP calls and the fixed live
  subprotocol; typed incompatibility remains actionable.
- Examples compile against the packed public surface rather than repository
  internals.
- Bundle-size, package-content, version-sync, distribution, and live tests
  pass.

Run at minimum:

```sh
npm test --prefix sdk/typescript
task release:check
./scripts/check-skill-drift
git diff --check
```

If a JSR dry-run tool is not pinned or already approved by the repository, stop
and report that missing supply-chain input instead of executing an unpinned
`npx` package.

## Use the GitHub beta

The explicitly approved GitHub beta workflow publishes the SDK tarball as part
of the complete signed prerelease. It does not publish npm or JSR. Consumers
may install that exact versioned tarball URL through npm-compatible tooling
while npm and JSR publication remain unauthorized.

Verify the GitHub asset against the complete signed release and test an import
from a clean temporary consumer. Do not call a GitHub asset an npm-registry or
JSR publication, and do not create a separate SDK-only hosted release.

## Publish registries only with explicit authority

Before any registry mutation, require:

- the final package name and verified npm/JSR namespace ownership;
- finalized canonical repository metadata matching the provenance source;
- an immutable source commit and completely verified signed release;
- confirmation that the version does not already exist;
- the exact stable/prerelease npm dist-tag and JSR channel behavior;
- registry authentication supplied through approved secret storage, never argv,
  chat, files in the checkout, logs, or generated artifacts;
- an approved, pinned publishing CLI and provenance configuration.

Publish in this order:

1. Publish the canonical signed release and verify it anonymously.
2. Publish npm from the exact SDK tarball contained in that release, with the
   explicit tag and provenance settings.
3. Publish JSR from the same source commit only after its dry-run file list and
   version match the verified npm artifact.
4. Fetch both registry versions anonymously and compare version, exports,
   declarations, license, README, and package contents.
5. Compile small consumers through npm, pnpm, Yarn, Bun, and JSR/Deno as
   applicable. Exercise at least one HTTP call type and the live constructor.

Never rebuild between registry publications. Never overwrite a published
version, silently move `latest`, or hide partial publication. On failure, stop,
preserve evidence, and reconcile forward with a new version or an explicitly
approved tag correction.

## Report the result

Report:

- mode: inspect, prepare, or publish;
- package identity, version, source commit, and signed release;
- npm tarball digest and selected dist-tag;
- JSR source/file-list parity;
- compatibility range and app API version;
- tests and anonymous registry verification actually completed;
- unresolved namespace ownership, tooling, credential, release-origin, or
  partial-publication state.
