# ADR 0054: Publish only the SDK to npm during beta

Status: Superseded by ADR 0058

ADR 0058 retains the exact-artifact, OIDC, fixed-environment, and immutable
version guarantees while moving SDK publication into the one complete release
invocation and adding the matching CLI npm stage.

## Context

Tinkercloud is public and maintainers control the `@tinkercloud` npm scope.
App creators need the browser SDK through normal TypeScript package managers,
while the native CLI already has one signed GitHub installer and does not need
a second distribution mechanism yet.

The V1 compatibility model and signed release format use strict numeric
versions. Introducing SemVer prerelease suffixes only for npm would split
versions across the server, SDK, manifest, and compatibility headers. Publishing
under `latest` would incorrectly claim stability. Rebuilding the package in a
registry workflow would create a second artifact authority.

ADR 0046 originally deferred every registry during beta. Namespace ownership
is now confirmed, so this decision supersedes only that npm-SDK deferral. Its
GitHub prerelease and beta-signing-authority rules remain in force.

## Decision

Publish only `@tinkercloud/sdk` to npm during the pre-stable phase. npm receives
the exact SDK tarball already contained in a verified signed GitHub prerelease.
The numeric version remains identical everywhere; GitHub's prerelease flag and
npm's fixed `beta` dist-tag identify the channel.

Use a separate, manually dispatched `npm-sdk-publish.yml` workflow. It is gated
by the `npm-sdk` GitHub Environment, npm trusted-publisher OIDC, an exact
confirmation phrase, beta release verification, immutable-version denial, and
post-publication provenance and consumer checks. It has no npm token and cannot
select or advance `latest`.

The first npm version is bootstrapped interactively with 2FA from the exact
verified prerelease tarball because npm requires an existing package before a
trusted publisher can be attached. Subsequent versions use only OIDC.

The `tinker` CLI remains distributed through its singular signed GitHub
installer. `@tinkercloud/cli`, the `tinkercloud` server, JSR, Homebrew, stable
GitHub releases, and stable npm tags remain outside this decision.

## Consequences

- npm, pnpm, Yarn, Bun, and Deno npm consumers share one SDK artifact.
- Beta consumers must opt in with `@tinkercloud/sdk@beta` or an exact version.
- Strict numeric compatibility remains unchanged.
- npm publication depends on the canonical signed GitHub prerelease instead of
  creating a parallel build path.
- A compromised beta authority can affect beta SDK consumers, so production
  still requires key rotation and a separate stable approval.
- Published npm mistakes reconcile forward under a new patch version; versions
  are never replaced or reused.
