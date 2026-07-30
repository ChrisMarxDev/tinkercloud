---
name: distribute-tinker-cli
description: Prepare, verify, or publish the native Tinker workstation CLI through the reviewed one-line installer, npm-family package, and Homebrew formula. Use for CLI release candidates, signed artifact preparation, npm/pnpm/Yarn/Bun executable distribution, Homebrew tap work, release-origin setup, post-publication verification, or diagnosing CLI channel drift. Do not use for the TypeScript app SDK package; use distribute-tinkercloud-sdk.
---

# Distribute the Tinker CLI

Treat distribution as one signed supply chain. Never manufacture channel
artifacts independently.

## Establish authority

Classify the request before doing work:

- **Inspect**: read state and report readiness; make no external mutation.
- **Prepare**: build and verify local candidates; do not publish, reserve a
  namespace, create a release, update a tap, or advance a tag.
- **Publish**: proceed only when the user explicitly asks to publish. Stable or
  package-manager publication requires the locked identity, exact version,
  verified destination ownership, and channel. Before stable distribution,
  only the separately contracted GitHub beta channel may publish.

An instruction to prepare, test, distribute later, or get release-ready is not
publication authority. Stop before stable or package-manager publication while
destination ownership or the release origin is unverified. The explicit GitHub
beta workflow below is the only pre-stable publication path.

## Load the contract

From the repository root, read in order:

1. `PRINCIPLES.md`
2. `PRD.md`
3. `specs/operations/distribution-contract.md`
4. `specs/operations/release-artifact-contract.md`
5. `specs/operations/update-compatibility-contract.md`
6. `specs/operations/beta-release-contract.md`
7. `docs/operations/release-pipeline.md`
8. `docs/decisions/0044-signed-distribution-compatibility-manifest.md`
9. `docs/decisions/0046-github-beta-release-channel.md`

Preserve unrelated work in a dirty checkout. Never rewrite an existing release
directory or use a production signing key implicitly.

## Prepare one source release

Require a strict `MAJOR.MINOR.PATCH` version and explicit new output paths.
Confirm that the version equals the SDK package, JSR declaration, exported SDK
version, server build version, CLI build version, and generated channel
versions.

Build and verify through the repository entry points:

```sh
task release:build VERSION="$VERSION" OUTPUT="$RELEASE_DIR"
task release:verify RELEASE_DIR="$RELEASE_DIR"
```

`TINKERCLOUD_RELEASE_SIGNING_KEY` must name an operator-controlled private key
outside the checkout. Never print, copy, inspect, or persist the key material.
For test-only work use `task release:check`, which creates a disposable
authority.

Generate local channel candidates only after complete release verification:

```sh
task distribution:prepare \
  VERSION="$VERSION" \
  RELEASE_DIR="$RELEASE_DIR" \
  OUTPUT="$DISTRIBUTION_DIR"
```

The task enforces `@tinkercloud/cli`, `Tinker`, `tinker`, and the exact versioned
`https://github.com/ChrisMarxDev/tinkercloud/releases/download/vVERSION/`
release base. Inventory the Taskfile, release scripts, `packaging/npm`,
`install-client.sh`, package metadata, native artifact names, launcher, and
documentation. Require those identities to agree across all of them before
preparing a publishable candidate; a parameter that affects only one channel
is not enough.

## Inspect the candidates

Prove all of the following before calling preparation complete:

- The signed release contains exactly the supported CLI matrix:
  Linux/macOS × amd64/arm64.
- Every CLI artifact has exact metadata, checksum, Ed25519 signature, and the
  same release version.
- The npm candidate contains the four native `tinker` binaries, one reviewed
  launcher, package metadata, and the Apache-2.0 license.
- The npm `bin` key, launcher target, native artifact names, Homebrew command,
  installer command, and documented invocation all use the approved final
  identity.
- The npm candidate has no dependency, peer dependency, lifecycle download
  hook, server binary, signing key, credential, release URL selected at install
  time, or unsupported platform claim.
- npm, pnpm, Yarn, and Bun consume the same npm artifact; do not create four
  divergent packages.
- The Homebrew formula selects only the verified macOS artifact for the local
  architecture and records the release checksum. Formula preparation never
  mutates a tap.
- The one-line installer is the signed `install-client.sh` release input. It
  selects locally, verifies checksum plus pinned signature, refuses root, and
  replaces the CLI atomically.
- No JavaScript package manager installs `tinkercloud`.

Run at minimum:

```sh
go test ./...
npm test --prefix sdk/typescript
task release:check
./scripts/check-skill-drift
git diff --check
```

Record exactly which checks ran. Do not claim Homebrew or registry behavior
from local package inspection alone.

## Publish a GitHub beta

Use this exception only when the user explicitly authorizes the exact beta
version and GitHub destination. Require:

- an existing `vVERSION` tag reachable from `main`;
- exact package/SDK/source version parity;
- a configured, reviewer-protected `beta-release` Environment;
- the beta-only signing secret matching the committed public key; and
- confirmation that no release already exists for the tag.

Run `task release:beta-check` before dispatch. It validates workflow syntax,
publication denial gates, and embedded trust-anchor parity without reading a
private key or changing GitHub state.

Dispatch only the reviewed workflow:

```sh
gh workflow run beta-release.yml \
  --ref main \
  -f version="$VERSION" \
  -f confirmation="publish-beta-v$VERSION"
```

Do not obtain, transmit, or inspect the signing secret. Stop for the GitHub
Environment approval. After completion, verify the release remains marked
prerelease, download and verify its exact assets, and report the versioned CLI,
host, and SDK-tarball URLs. Never publish npm/JSR, update Homebrew, promote
latest/stable, or overwrite a beta asset. A failed public beta moves forward
under a new numeric version.

## Publish stable and package-manager channels

Before any external mutation, require:

- final product, binary, package, formula, tap, repository, and release-origin
  names;
- verified ownership of every namespace and destination;
- a clean, intentional source commit and immutable signed release directory;
- the exact version and prerelease/stable channel;
- an approved release authority and public verification key;
- explicit approval for the external destinations being mutated.

Publish in dependency order:

1. Create the immutable canonical hosted release with the exact verified files.
2. Verify remote checksums and signatures by downloading them as an anonymous
   consumer.
3. Publish the generated npm tarball with an explicit dist-tag. Never publish
   from a rebuilt or modified package directory.
4. Commit the generated `Tinker` formula to
   `ChrisMarxDev/homebrew-tinkercloud` without changing its version, URLs, or
   hashes.
5. Expose the reviewed one-line installer only from the approved HTTPS origin.
6. Install independently through npm, pnpm, Yarn, Bun, Homebrew, and the
   one-line path; compare `tinker version` and artifact identity with the signed
   release.

Use provenance where the destination supports it. Never silently default a
prerelease to `latest`.

If a later channel fails, do not overwrite an immutable version or bypass
verification. Stop, preserve evidence, report the partial external state, and
propose a forward-only reconciliation.

## Report the result

Report:

- mode: inspect, prepare, or publish;
- exact version and source commit;
- signed release directory or canonical release URL;
- CLI platform matrix;
- npm package and dist-tag;
- Homebrew tap/formula;
- installer origin;
- verification performed per channel;
- placeholders, unverified destinations, or partial publication state.
