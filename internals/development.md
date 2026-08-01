# Development and verification

This page is for contributors and coding agents. End users should start with
the root [README](../README.md); operators should use the deployment and
operations guides in [docs](../docs/).

## Prerequisites

The repository expects Task 3.38+, Go 1.25.12, Node.js 22, npm, a POSIX shell,
OpenSSL, and Python 3. On macOS, release scripts also require GNU `sha256sum`.

## Local verification

```sh
task setup
task -l
task build
task check
```

Focused checks include `task test`, `task lint`, `task security`, and
`task release:check`. These are local evidence commands; they do not deploy,
publish, or install production software.

The explicitly gated `task vps:e2e` mutates a disposable VPS only after its
acknowledgement and pinned host-key inputs are present. Read
[docs/operations/vps-e2e.md](../docs/operations/vps-e2e.md) first.

## Contribution boundary

- Keep public behavior and operator/deployer guidance in `README.md` and `docs/`.
- Keep agent procedures in `skills/` and contributor mechanics here.
- Treat `PRINCIPLES.md` as a review gate and `PRD.md` as canonical scope.
- Run the narrowest relevant tests, then the repository checks required by the
  changed area before opening a PR.
