# ADR 0059: Narrow GitHub release-asset redirect boundary for updater retrieval

**Status:** Accepted for V1

## Context

The signed updater originally rejected every HTTP redirect. Immutable GitHub
release download URLs return a redirect to GitHub's release asset host, so that
policy makes an otherwise valid official release fail before signature and
compatibility verification.

Allowing general redirect following would turn the root-local updater into an
SSRF and origin-confusion primitive, especially for explicitly configured
development release bases.

## Decision

Keep redirect denial as the default. Permit exactly one redirect only from an
exact canonical immutable URL of the form:

```text
https://github.com/ChrisMarxDev/tinkercloud/releases/download/vMAJOR.MINOR.PATCH/NAME
```

`NAME` is limited to the server updater artifact, its metadata/signature
sidecars, or the signed release manifest and its sidecars. The initial request
has no credentials, query, fragment, or explicit port. Its destination must be
HTTPS with exact host `release-assets.githubusercontent.com`, no credentials,
fragment, or explicit port. Both hops undergo public-DNS validation and the
second response must be the exact requested final URL with status 200; a second
redirect denies.

All custom origins, mutable/latest paths, other GitHub repositories, other
hosts, private targets, downgrade, final-URL mismatches, and malformed redirect
state continue to deny. The redirect remains transport only: byte limits,
pinned Ed25519 signatures, artifact digests, and signed release-manifest
compatibility verification remain required before any snapshot or replacement.

## Consequences

Official immutable GitHub release updates work through GitHub's asset delivery
host. Operator-selected release bases retain their no-redirect behavior, so
they cannot direct the updater to another origin. The exception is exercised by
both updater retrieval and command-level verification tests.
