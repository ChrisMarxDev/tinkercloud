# ADR 0020: Root deployer command writes SQLite as the service identity

**Status:** Accepted for V1

## Context

`tinyhost deployers` is a root-only recovery command, but opening SQLite as
root can create or replace the database's WAL and SHM sidecars as root-owned.
The unprivileged gateway then cannot persist OTP challenges or other state.

## Decision

The command validates that its original local caller is root, parses its
non-secret command/configuration, then permanently drops to the installed
`tinyhost` UID and primary GID before it opens SQLite. The service identity,
not root, creates and mutates `tinyhost.db`, `tinyhost.db-wal`, and
`tinyhost.db-shm`. No broad mode change is used to compensate for ownership.

For recovery from the pre-decision regression, root may hand off only the three
known regular root-owned SQLite artifacts after no-symlink and non-permissive
mode validation. It uses a no-follow ownership syscall; it never recursively
changes the data directory or uses permissive `chmod`.

If service-identity lookup, artifact handoff, supplementary-group reset,
privilege drop, database open, or the deployer mutation fails, the command
returns a typed failure and prints no successful authorization result.

## Consequences

- Root remains the authority that may begin a deployer authorization command.
- The process cannot regain root after its database boundary, which is safe
  because no later step needs privileged host access.
- The root command can recover only its three known root-owned SQLite artifacts
  under strict validation; it cannot recursively alter arbitrary data files.
