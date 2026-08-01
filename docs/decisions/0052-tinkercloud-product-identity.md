# ADR 0052: Tinkercloud product identity

**Status:** Accepted

## Context

The product is still unreleased, and its public identity needed to be finalized
before package, binary, repository, route, and distribution names became
compatibility commitments.

This decision changes naming only. It does not change the gateway trust
boundary, actor model, authorization context, persistence ownership,
deployment topology, SDK capability model, or V1 scope.

## Decision

The canonical product name is **Tinkercloud**.

- Human-facing product nouns use Tinkercloud: Tinkercloud platform, server,
  app, SDK, and UI.
- The operator/server binary, service, service account, configuration, and
  host data paths use `tinkercloud`.
- The deployer CLI command and distribution use `tinker`.
- Browser apps import `tinker` from `@tinkercloud/sdk`.
- The deployment receipt is `tinker.yaml`.
- App-facing reserved HTTP and WebSocket routes use `/_tinker/*`.
- App-facing UI and browser identifiers use the `tinker` prefix.
- Server and operations environment variables use `TINKERCLOUD_*`; CLI and SDK
  environment variables use `TINKER_*`.
- The Go module and repository identity is
  `github.com/ChrisMarxDev/tinkercloud`.
- The npm CLI wrapper package is `@tinkercloud/cli`.
- The npm and JSR SDK package is `@tinkercloud/sdk`.
- The Homebrew formula command and class are `tinker` and `Tinker`.
- The Homebrew tap repository is
  `github.com/ChrisMarxDev/homebrew-tinkercloud`.
- Public documentation, installation, source, issue, and release links remain
  GitHub-based until a separate official domain decision is accepted.

The existing visual identity, including the cloud character, colors, geometry,
and motion, remains unchanged.

Because no release or public distribution exists, this is a clean break.
There are no compatibility aliases, legacy command shims, dual route
registrations, fallback manifests, or migration promises for earlier labels.

## Consequences

- Every contract, binary, package, route, manifest, cookie, service, filesystem
  path, environment variable, skill, example, test, document, and marketing
  artifact uses the canonical identity.
- Route-registry, anonymous-denial, installer, SDK, skill-drift, and release
  tests must reject accidental reintroduction of a second naming surface.
- The rename adds no listener, dependency, authority, persistence system, or
  public capability.
