# ADR 0020: Root deployer command writes SQLite as the service identity

**Status:** Accepted for V1

## Context

`tinkercloud deployers` is a root-only recovery command, but opening SQLite as
root can create or replace the database's WAL and SHM sidecars as root-owned.
The unprivileged gateway then cannot persist OTP challenges or other state.

## Decision

The root parent validates its local caller and non-secret input, then starts
one fixed internal child operation. That child permanently drops to the
installed `tinkercloud` UID and primary GID before it opens SQLite. The service
identity, not root, creates and mutates `tinkercloud.db`, `tinkercloud.db-wal`, and
`tinkercloud.db-shm`. No broad mode change is used to compensate for ownership.

For recovery from the pre-decision regression, root may hand off only the three
known regular root-owned SQLite artifacts after no-symlink and non-permissive
mode validation. It uses a no-follow ownership syscall; it never recursively
changes the data directory or uses permissive `chmod`.

The parent restarts only an already-active `tinkercloud.service` after the child
has durably committed and closed SQLite, then confirms it is active. This
refresh clears a demonstrated cross-process SQLite connection failure without
starting a deliberately stopped service. If refresh fails, the command returns
the explicit `deployer_applied_service_refresh_failed` result and prints no
success: it does not falsely claim to have rolled back the durable mutation.

If service-identity lookup, artifact handoff, supplementary-group reset,
privilege drop, database open, or the deployer mutation fails, the command
returns a typed failure and prints no successful authorization result.

## Consequences

- Root remains the authority that may begin a deployer authorization command.
- The database-writing child cannot regain root after its database boundary.
  The root parent retains only the fixed post-close service refresh authority;
  it never opens SQLite itself.
- The root command can recover only its three known root-owned SQLite artifacts
  under strict validation; it cannot recursively alter arbitrary data files.
