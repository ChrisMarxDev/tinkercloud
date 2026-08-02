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
`@tinkercloud/sdk`. During beta, npm publication is authorized only for the
exact SDK tarball in an already-public verified GitHub prerelease, through the
fixed `beta` dist-tag. JSR and stable/latest publication remain unauthorized.

## Load the contract

From the repository root, read in order:

1. `PRINCIPLES.md`
2. `PRD.md`
3. `specs/sdk/distribution-contract.md`
4. `specs/operations/distribution-contract.md`
5. `specs/operations/beta-release-contract.md`
6. `specs/sdk/npm-beta-publishing-contract.md`
7. `specs/operations/update-compatibility-contract.md`
8. `specs/api/http-contract.md`
9. `docs/architecture/client-sdk.md`
10. `docs/operations/release-pipeline.md`
11. `docs/decisions/0026-sdk-registry-distribution.md`
12. `docs/decisions/0044-signed-distribution-compatibility-manifest.md`
13. `docs/decisions/0046-github-beta-release-channel.md`
14. `docs/decisions/0054-sdk-npm-beta-trusted-publishing.md`

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

## Use the GitHub beta as the source

The explicitly approved GitHub beta workflow publishes the SDK tarball as part
of the complete signed prerelease. The GitHub workflow itself does not publish
npm or JSR. Consumers may install that exact versioned tarball URL directly, or
a separately approved SDK workflow may publish the same bytes to npm beta.

Verify the GitHub asset against the complete signed release and test an import
from a clean temporary consumer. Do not call a GitHub asset an npm-registry or
JSR publication, and do not create a separate SDK-only hosted release.

## Publish the npm beta only with explicit authority

Before npm mutation, require:

- verified control of the npm `@tinkercloud` scope;
- finalized canonical repository metadata matching the provenance source;
- an immutable source commit and completely verified public GitHub prerelease;
- confirmation that the version does not already exist;
- exact authorization for `@tinkercloud/sdk`, the numeric version, and the
  fixed `beta` dist-tag;
- GitHub Environment `npm-sdk` approval and the exact trusted publisher;
- no npm token, developer session, `.npmrc` credential, or inherited secret.

Run the non-publishing checks first:

```sh
task release:npm-sdk-check
```

If the npm package does not exist, stop and route the operator through the
documented one-time 2FA bootstrap from the exact verified SDK tarball. Then
configure the trusted publisher for repository `ChrisMarxDev/tinkercloud`,
workflow `release.yml`, Environment `npm-sdk`, and only `npm publish`.

For later versions, publish only through:

```sh
gh workflow run release.yml \
  --ref main \
  -f channel=beta \
  -f version="$VERSION" \
  -f confirmation="publish-beta-v$VERSION"
```

Approve the `npm-sdk` Environment only after verifying the version, tag,
GitHub prerelease, workflow commit, and fixed beta channel. The workflow must:

1. Download and verify the complete signed GitHub prerelease.
2. Select and inspect its exact `tinkercloud-sdk-VERSION.tgz` without rebuilding.
3. Reject an existing npm version.
4. Publish with OIDC, provenance, scripts disabled, and `--tag beta`.
5. Verify the registry integrity, attestation, dist-tag, repository, and clean
   consumer import.

Never publish the CLI or server, invoke JSR, rebuild, overwrite a version,
advance `latest`, or hide partial publication. On failure, stop, preserve
evidence, and reconcile forward with a new numeric patch version.

## Report the result

Report:

- mode: inspect, prepare, or publish;
- package identity, version, source commit, and signed release;
- npm tarball digest and fixed `beta` dist-tag;
- trusted-publisher/provenance verification;
- compatibility range and app API version;
- tests and anonymous registry verification actually completed; and
- unresolved bootstrap, namespace ownership, release-origin, or
  partial-publication state. Report JSR as unauthorized, not pending within the
  npm beta workflow.
