# ADR 0045: Separate maintainer skills for CLI and SDK distribution

Status: Accepted for distribution preparation

## Context

CLI distribution and TypeScript SDK publication share a signed release and
compatibility policy, but their artifacts, registries, verification, and
failure recovery differ. Adding publication instructions to the deployer or
operator skills would mix product-use authority with maintainer release
authority.

## Decision

Create two standalone maintainer skills:

- `distribute-tinker-cli` for the native CLI, npm-family executable package,
  Homebrew formula, and one-line installer;
- `distribute-tinkercloud-sdk` for npm/JSR browser client packaging.

Each classifies work as inspect, prepare, or publish. Inspect and prepare are
non-publishing defaults. Publication requires explicit authorization for the
exact finalized identities, version, destinations, and channel.

The skills reuse repository release tooling and contracts rather than bundling
an alternate publisher or signing implementation.

## Consequences

Role-facing skills remain focused on deploying apps and operating servers.
Maintainers receive low-freedom, channel-specific release workflows. The final
rename can update distribution identities without changing the security or
compatibility model.
