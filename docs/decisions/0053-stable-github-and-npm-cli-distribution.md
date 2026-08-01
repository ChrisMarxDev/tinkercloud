# ADR 0053: Stable CLI distribution uses GitHub Releases and one npm artifact

**Status:** Accepted; activation gated by production key and destination setup

## Context

The final Tinkercloud identity is locked. Deployer machines need a direct
native installer and JavaScript package-manager users expect npm, pnpm, Yarn,
and Bun support. Producing a different executable package for each manager
would multiply bytes and supply-chain evidence. Promoting the beta signing key
would violate the beta authority decision.

## Decision

Use immutable GitHub Releases as the canonical signed binary origin. A stable
release exposes both an exact-version installer and a convenient `latest`
one-line form; the installer still verifies the selected native binary with
the pinned production Ed25519 authority before replacement.

Publish one dependency-free `@tinkercloud/cli` tarball containing the four
supported native CLI binaries and a reviewed Node launcher. npm, pnpm, Yarn,
and Bun all consume that same npm-registry artifact. The package never installs
the privileged `tinkercloud` server and runs no lifecycle downloader.

Separate stable GitHub publication from npm publication. The GitHub workflow
creates and remotely verifies the canonical release first. The npm workflow
then downloads that release, verifies it, generates the wrapper, and publishes
with npm trusted publishing and provenance. The first npm version is a one-time
interactive 2FA bootstrap because trusted-publisher configuration requires an
existing package. npm 11.5.1 is the narrow pinned publishing dependency because
that is the minimum CLI version supporting this trusted-publisher flow.

Stable activation requires a new production signing key, public repository,
immutable releases, protected GitHub Environments, npm scope ownership, and
the configured trusted publisher. Homebrew and SDK registry publication remain
separate later mutations.

## Consequences

- One GitHub release remains the source of truth for native CLI bytes.
- One npm package supports the four npm-family tools without divergent builds.
- Stable publication cannot accidentally reuse the beta trust anchor.
- A package-manager failure cannot mutate an already verified GitHub release;
  repair remains forward-only.
- The first npm package needs one carefully reviewed interactive publication;
  later versions use short-lived OIDC rather than a repository token.
