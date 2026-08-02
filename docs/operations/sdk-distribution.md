# SDK Distribution

**Status:** unified release workflow prepared; first package bootstrap still manual

`@tinkercloud/sdk` is published under `beta` by the same release invocation that
publishes the signed GitHub prerelease and `@tinkercloud/cli` under `next`.
JSR, Homebrew, stable npm, and npm's `latest` dist-tag remain disabled in beta.

## Consumer formats

One npm artifact supports npm, pnpm, Yarn, Bun, and Deno:

```sh
npm install @tinkercloud/sdk@beta
pnpm add @tinkercloud/sdk@beta
yarn add @tinkercloud/sdk@beta
bun add @tinkercloud/sdk@beta
deno add npm:@tinkercloud/sdk@beta
```

These commands work after the one-time npm bootstrap below. An exact beta can
also be pinned as `@tinkercloud/sdk@VERSION`. Do not document the untagged
package until a stable release is separately authorized and verified.

## Prepare a candidate

From `sdk/typescript`:

```sh
npm ci
npm test
npm publish --dry-run
task release:npm-sdk-check
```

`npm test` verifies version consistency, exact tarball contents, examples,
bundle size, and an offline install/import from the generated tarball. The dry
run and Task target do not mutate npm, GitHub, or another registry.

The following values must be identical for a release:

- `package.json` version;
- `jsr.json` version;
- `SDK_VERSION` in `src/index.ts`; and
- the version passed to `scripts/release-build.sh`.

## One-time npm bootstrap

The normal workflow uses npm trusted publishing, but npm cannot attach a trusted
publisher before `@tinkercloud/sdk` exists. Bootstrap exactly one beta version
interactively after its GitHub prerelease is public.

Prerequisites:

1. The exact `vVERSION` GitHub release is published as a prerelease, not a
   stable release or draft.
2. The npm account has 2FA and controls the `@tinkercloud` scope.
3. GitHub Environment `npm-sdk` exists with a required reviewer and permits
   `main`.
4. `task release:npm-sdk-check` and the complete release checks pass from the
   reviewed tag.

Download and verify every canonical release asset before selecting the SDK:

```sh
VERSION=0.1.2
RELEASE_DIR=$(mktemp -d)
gh release download "v$VERSION" \
  --repo ChrisMarxDev/tinkercloud \
  --dir "$RELEASE_DIR"
TINKERCLOUD_REQUIRED_RELEASE_CHANNEL=beta \
  ./scripts/check-release-key-policy.sh
./scripts/release-verify.sh "$RELEASE_DIR"
node ./scripts/verify-sdk-publish-candidate.mjs \
  "$VERSION" "$RELEASE_DIR/tinkercloud-sdk-$VERSION.tgz"
```

After reviewing the exact version and tarball, publish it once with 2FA:

```sh
npm publish "$RELEASE_DIR/tinkercloud-sdk-$VERSION.tgz" \
  --access public \
  --tag beta \
  --ignore-scripts
```

Do not use `latest` or rebuild the tarball. Bootstrap the CLI separately from
the same verified release using the CLI distribution procedure. The
first interactive publish may not carry CI provenance; every later OIDC
publication must.

Immediately configure the package's npm trusted publisher:

```sh
npm trust github @tinkercloud/sdk \
  --repo ChrisMarxDev/tinkercloud \
  --file release.yml \
  --env npm-sdk \
  --allow-publish \
  --yes
```

This command requires npm 11.15.0 or newer. Verify the resulting fields:

```text
Provider: GitHub Actions
Organization/user: ChrisMarxDev
Repository: tinkercloud
Workflow: release.yml
Environment: npm-sdk
Allowed action: npm publish
```

There is no npm token or GitHub npm secret.

## Publish subsequent betas

Dispatch the one complete release after creating the reviewed numeric-patch tag:

```sh
gh workflow run release.yml \
  --ref "v$VERSION" \
  -f channel=beta \
  -f version="$VERSION" \
  -f confirmation="publish-beta-v$VERSION"
```

Approve Environment `npm-sdk` only within that reviewed run after checking the
tag, workflow commit, package absence, and fixed beta channel. The internal job
downloads and verifies the complete signed release, publishes its exact SDK tarball with
OIDC and provenance under `beta`, then verifies registry integrity,
attestation, repository metadata, dist-tag, and a clean consumer import.

Every public beta uses a new numeric patch version. If npm accepts a version and
a later check fails, do not unpublish or reuse it; preserve the evidence and
release the correction forward under another version.

The normative rules and deny paths are in
[`specs/sdk/npm-beta-publishing-contract.md`](../../specs/sdk/npm-beta-publishing-contract.md)
and
[`test/security/sdk-npm-beta-publishing-denial-charter.md`](../../test/security/sdk-npm-beta-publishing-denial-charter.md).
A successful publication is distribution evidence only; it does not prove
Tinkercloud authorization or isolation.
