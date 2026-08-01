# ADR 0046: GitHub prereleases are the pre-stable beta channel

Status: Accepted for beta distribution

## Context

Maintainers need an installable beta before stable registry, tap, and domain
distribution is authorized. npm and Homebrew would create registry/tap state
that requires separate ownership and publication approval. The existing signed
release already contains the complete CLI, server, SDK, installer,
compatibility, and verification evidence needed for a direct GitHub channel.

Running the eventual production signing authority inside ordinary CI would
unnecessarily expand stable-release trust before that authority and identity
are finalized.

## Decision

Publish manually approved GitHub prereleases directly from exact reviewed
version tags. A protected `beta-release` GitHub Environment holds the private
key matching the currently committed public key. That authority is explicitly
beta-only.

The workflow validates and tests without the private key, signs one complete
release, verifies it locally, creates a draft, downloads and verifies the draft
assets, and only then publishes the release as a prerelease. npm, JSR,
Homebrew, stable/latest promotion, and silent update channels remain disabled.
The pinned `actionlint` version is the narrow additional CI dependency used to
validate the security-critical workflow syntax.

Before stable distribution, generate a new production authority outside GitHub
and update the committed key plus every embedded installer/bootstrap trust
anchor together.

## Consequences

Beta consumers can install the CLI and server directly from immutable,
versioned GitHub assets. Maintainers operate one public prerelease channel
without claiming npm, JSR, or Homebrew availability. Compromise of the beta
authority can affect beta consumers, so the environment requires human
approval and the authority is never promoted to production trust.

Every public beta consumes a new strict numeric patch version. Failures after
publication reconcile forward; published assets and tags are never replaced.
