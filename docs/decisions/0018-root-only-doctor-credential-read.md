# ADR 0018: Root-only credential file for doctor

**Status:** Accepted for V1

## Context

The unprivileged TinyHost service receives provider credentials through a
root-owned systemd `EnvironmentFile`. A separately invoked `tinyhost doctor`
process does not inherit that service environment, so using `os.Getenv` makes
the documented root diagnostic report Resend unavailable despite a healthy
service credential.

## Decision

`tinyhost status` remains offline and never reads provider credentials.
`tinyhost doctor` requires a local root caller before it reads credentials. It
uses `/etc/tinyhost/credentials/tinyhost.env` by default and permits an
explicit absolute override only after strict no-symlink, root-owner, regular
file, mode-`0600`, and exact configured-assignment validation. Parsed values
are retained only in typed local diagnostic composition; doctor does not modify
the process environment.

Doctor uses the Resend key only for its bounded, read-only provider request.
Its output and errors remain typed and redacted: no key, credential path,
environment reference, or provider body is emitted.

## Consequences

- `sudo tinyhost doctor` works independently of systemd's process environment.
- A non-root shell can still run `tinyhost status` for offline local facts, but
  cannot cause provider credential reads.
- Invalid credential storage fails the Resend diagnostic closed without making
  the credential or its location a support-ticket leak.
