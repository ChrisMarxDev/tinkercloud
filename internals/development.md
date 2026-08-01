# Development and verification

This page is for contributors and coding agents. End users should start with
the root [README](../README.md); operators should use the deployment and
operations guides under [`docs/`](../docs/).

## Prerequisites

The repository expects Task 3.38+, Go 1.25.12, Node.js 22, npm, a POSIX shell,
OpenSSL, and Python 3. On macOS, release scripts also require GNU
`sha256sum`.

## Local workflow

```sh
task setup
task -l
task build
task check
```

Focused checks include `task test`, `task lint`, `task security`, and
`task release:check`. These commands produce local evidence; they do not deploy,
publish, or install production software.

Signed artifact creation requires explicit release inputs:

```sh
task release:build VERSION=0.1.0 OUTPUT=./dist/0.1.0
```

The explicitly gated `task vps:e2e` command mutates a disposable VPS only after
its acknowledgement and pinned host-key inputs are present. Read the operator
[VPS E2E guide](../docs/operations/vps-e2e.md) before using it.

## Change discipline

- Read `AGENTS.md`, `PRINCIPLES.md`, and `PRD.md` first.
- Identify the contract and deny-path test charter affected by the change.
- Keep protected dispatch behind a typed authorization result.
- Update the relevant skill when an SDK, manifest, capability, deployment, or
  verification workflow changes.
- Run unit, contract, integration, and negative security checks for the changed
  surface.
- Do not report a deployment or security feature complete from a happy-path
  check alone.
