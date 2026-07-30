# ADR 0032: Local Resend reader for unattended VPS acceptance

**Status:** Accepted for V1 acceptance evidence

## Context

The external VPS acceptance suite intentionally requires real deployer and
viewer OTP delivery, but manually relaying codes prevents unattended testing.
Reading challenge state from Tinkercloud, the VPS, logs, or a test-only endpoint
would weaken the gateway boundary and turn an acceptance test into a bypass.

## Decision

Use a local-only, dependency-free Resend sent-email reader as the optional
`TINKERCLOUD_VPS_OTP_COMMAND`. It receives the purpose, exact identity, and
hostname from the existing black-box harness, reads a local mode-`0600`
full-access reader key, calls a fixed HTTPS Resend API origin, and emits only
the one exact Tinkercloud OTP that passes recipient, sender, subject, recent-time,
hostname-shape, body, and consumed-message checks.

The key and consumed-ID ledger remain in the ignored local `.tinker/` directory
and are never copied to the VPS, server configuration, browser, SDK, database,
or logs. Prefer a least-privilege VPS sending key separate from the local
reader credential. A disposable operator may deliberately reuse a current
full-access file for both reader and sending configuration only if it has
sent-email read permission, then rotate or narrow it afterwards.

## Consequences

- Unattended tests exercise actual email composition, OTP verification, and
  session issuance without an auth bypass or extra mailbox account.
- The reader verifies provider acceptance/content, not downstream mailbox
  delivery; retain occasional manual delivery smoke evidence.
- A compromised reader key can inspect sent mail permitted by its Resend role,
  so it is local-only, restrictive-file-mode, revocable, and excluded from Git.
- Reusing that full-access key for the disposable VPS test temporarily broadens
  the VPS credential; it is a test convenience, not the recommended posture.
- Ambiguity and provider failure stop the test rather than selecting a code.
