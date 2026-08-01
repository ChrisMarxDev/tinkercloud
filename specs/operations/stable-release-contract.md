# Stable GitHub and npm CLI Release Contract

## Scope

This contract authorizes two manually approved stable distribution mutations
after every prerequisite is satisfied:

1. an immutable GitHub release containing the complete signed Tinkercloud
   release; and
2. the `@tinkercloud/cli` npm package derived from that exact release.

The npm artifact is the one executable package consumed by npm, pnpm, Yarn,
Bun, and their package runners. This contract does not publish the TypeScript
SDK, JSR package, Homebrew tap, server through a JavaScript registry, or a
scheduled update channel.

## Prerequisites

- Product and channel identities equal ADR 0052.
- The source is an exact `vMAJOR.MINOR.PATCH` tag reachable from `origin/main`.
- The repository is public and GitHub release immutability is enabled.
- The beta signing authority has been retired. The committed trust anchor and
  every embedded copy have been rotated together to a separately controlled
  production key, and `release-key-policy.json` records `production` plus the
  matching SHA-256 fingerprint of its DER public key.
- The `stable-release` and `npm-cli` GitHub Environments require a reviewer and
  restrict deployment to reviewed release tags.
- The `@tinkercloud` npm scope is operator-controlled. The package has been
  bootstrapped once with interactive 2FA, then configured to trust only
  `npm-cli-publish.yml` in `ChrisMarxDev/tinkercloud`, Environment `npm-cli`,
  for `npm publish`.

## Stable release workflow

The workflow is manually dispatched with the exact numeric version and
`publish-stable-vVERSION`. It validates source, repository visibility, version
parity, release absence, production-key policy, and the full normal test suite
before entering `stable-release` or reading its signing secret.

The protected job builds once, verifies locally, creates a draft stable
release, downloads the complete draft into a fresh directory, compares the
asset set, verifies it again, and only then publishes it as `latest`. It smoke
tests both exact-version and `latest` one-line installers. Published releases,
tags, and assets are never edited or replaced.

The production private key is stored only as
`TINKERCLOUD_RELEASE_SIGNING_KEY_B64` in the protected Environment. Its decoded
temporary file is random, mode `0600`, outside the checkout, removed on every
exit, and unavailable to child builds except through the canonical release
builder's narrow input.

## npm CLI workflow

The npm workflow is a separate manual dispatch with the exact version,
explicit dist-tag, and `publish-npm-cli-vVERSION`. It checks out the tag,
requires the corresponding GitHub release to be stable and published,
downloads all release assets anonymously, and verifies the complete release.
It generates one npm tarball through `distribution-prepare.sh`, inspects its
contents and manifest, and refuses an existing registry version.

Publication runs on a GitHub-hosted runner with Node 24, npm 11.5.1 or newer,
`id-token: write`, the `npm-cli` Environment, no npm token, and
`npm publish --access public --tag DIST_TAG`. npm trusted publishing supplies
short-lived OIDC authentication and provenance. The workflow then installs the
exact registry version into a clean consumer and verifies `tinker version`.

## Consumer commands

The stable one-line installer is:

```sh
curl --proto '=https' --tlsv1.2 -fsSL https://github.com/ChrisMarxDev/tinkercloud/releases/latest/download/install-client.sh | TINKER_RELEASE_BASE=https://github.com/ChrisMarxDev/tinkercloud/releases/latest/download/ sh
```

The exact-version form replaces both `latest` path segments with `vVERSION`.
The npm-registry package is installed with any of:

```sh
npm install -g @tinkercloud/cli
pnpm add -g @tinkercloud/cli
yarn add --dev @tinkercloud/cli
bun add -g @tinkercloud/cli
```

These commands are advertised only after their corresponding public channel
has been anonymously verified.

## Failure recovery

Failure before external mutation creates no public state. Draft upload or
verification failure leaves a draft for diagnosis. Any failure after a release
or npm version becomes public is an incident: preserve evidence, do not replace
or unpublish the version, and reconcile forward under a new version or an
explicitly reviewed dist-tag correction.
