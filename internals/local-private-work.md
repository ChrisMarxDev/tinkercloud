# Local-only working material

Use the ignored root `.private/` directory for non-secret material that is
useful only in one checkout, such as temporary plans, live acceptance briefs,
design comparisons, and local review notes. The directory is a shared naming
convention, not a security boundary and not a durable project record.

Use these locations deliberately:

- `internals/` for contributor knowledge that should be reviewed and published;
- `.private/` for repository-specific material that must remain untracked;
- `.tinker/` for Tinkercloud-owned emulator, VPS-acceptance, and runtime state
  only, using the permissions and paths required by its contracts;
- `.git/info/exclude` for an individual's additional repository-only ignore
  patterns; and
- storage outside the repository for ordinary operational credentials, OTPs,
  private keys, provider responses, real viewer data, and other secrets. The
  narrow `.tinker/vps/` acceptance fixtures remain an explicit exception and
  must stay ignored and mode `0600`.

Before deleting a checkout, promote any durable non-sensitive decision into
`internals/`, `docs/decisions/`, a contract, or the PRD. Never force-add
`.private/`, copy it into a source archive, or treat ignored content as backed
up.
