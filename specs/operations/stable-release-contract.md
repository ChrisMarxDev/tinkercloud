# Stable unified GitHub and npm Release Contract

## Scope

This contract authorizes one manually approved stable distribution flow after
every prerequisite is satisfied:

1. an immutable GitHub release containing the complete signed Tinkercloud
   release; and
2. the `@tinkercloud/sdk` and `@tinkercloud/cli` npm packages derived from
   that exact release.

The CLI npm artifact is the one executable package consumed by npm, pnpm, Yarn,
Bun, and their package runners. This contract does not publish JSR, a Homebrew
tap, or server through a JavaScript registry. An
operator-enabled stable server channel may discover the immutable GitHub
release, but its signed server artifact and manifest remain the installation
authority.

## Prerequisites

- Product and channel identities equal ADR 0052.
- The source is an exact `vMAJOR.MINOR.PATCH` tag reachable from `origin/main`.
- The repository is public and GitHub release immutability is enabled.
- The beta signing authority has been retired. The committed trust anchor and
  every embedded copy have been rotated together to a separately controlled
  production key, and `release-key-policy.json` records `production` plus the
  matching SHA-256 fingerprint of its DER public key.
- The `stable-release`, `npm-sdk`, and `npm-cli` GitHub Environments require a reviewer and
  restrict deployment to reviewed release tags.
- The `@tinkercloud` npm scope is operator-controlled. The package has been
  bootstrapped once with interactive 2FA, then configured to trust only
  `release.yml` in `ChrisMarxDev/tinkercloud` and its own Environment for
  `npm publish`.

## Stable release workflow

The `release.yml` workflow is manually dispatched with channel `stable`, the
exact numeric version, and `publish-stable-vVERSION`. It validates source,
repository visibility, version parity, release absence, production-key policy,
and absence of both npm versions before entering any protected environment or
reading a signing secret.

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

## Protected npm stages

After the canonical GitHub release succeeds, internal reusable jobs check out
the same tag, require that release to be stable and published, download all
release assets anonymously, and verify the complete release. The SDK job uses
the signed SDK tarball directly. The CLI job generates one npm tarball through
`distribution-prepare.sh` from the signed native artifacts. Both inspect their
contents and manifests and recheck that the registry version remains unused.

Publication runs on GitHub-hosted runners with Node 24, npm 11.5.1 or newer,
`id-token: write`, fixed `npm-sdk` and `npm-cli` Environments, no npm token, and
`npm publish --access public --tag latest`. npm trusted publishing supplies
short-lived OIDC authentication and provenance. The jobs then install the exact
registry versions into clean consumers and verify the SDK API and
`tinker version`.

## Consumer commands

The stable one-line installer is:

```sh
curl --proto '=https' --tlsv1.2 -fsSL https://github.com/ChrisMarxDev/tinkercloud/releases/latest/download/install-client.sh | sh
```

The exact-version form replaces `latest` with `vVERSION`. The signed released
installer embeds that same immutable base and rejects `TINKER_RELEASE_BASE`;
only a reviewed development installer accepts that explicit origin.
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
