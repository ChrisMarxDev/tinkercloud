---
name: tiny-deployment-agent
description: Run one opt-in local TinyHost test deployment from a deployer's workstation using a saved CLI credential or the normal Resend-backed CLI OTP flow. Use when controlled test automation must deploy a known app directory to a known HTTPS TinyHost server as one exact deployer identity.
---

# TinyHost deployment agent

Use this skill only for explicit, local deployer-workstation test automation.
It deliberately completes the normal human deployer's OTP login and stores the
ordinary local CLI credential. It is not production CI/noninteractive
deployment-agent authentication: that path requires a separately provisioned,
app-scoped deployer token and must not use this wrapper or an interactive OTP.
It grants no authority beyond the exact deployer's saved CLI credential.

## Boundary

The deployment agent is non-human automation acting through deployer authority.
It cannot act as an operator or viewer, create a bearer, choose an app identity,
or broaden access. TinyHost remains private-only; the gateway owns activation,
the anonymous public-denial proof, and app identity.

Never accept or print a bearer, OTP, Resend key, provider secret, app ID, or
viewer identity. Keep the reader key and consumed-message ledger local; never
copy either to a VPS, project, manifest, or source control. Missing, malformed,
redirected, incompatible, ambiguous, or timed-out state is a denial.

## One-deployment workflow

Ask only for these non-secret values when they cannot be discovered safely:

- exact absolute path to the `tiny` CLI;
- exact HTTPS platform server URL;
- exact deployer email; and
- exact absolute app directory containing `tiny.yaml`.

Use the checked-in deterministic wrapper once:

```sh
python3 skills/tiny-deployment-agent/scripts/deploy_once.py \
  --tiny /absolute/path/to/tiny \
  --server https://admin.example.com \
  --deployer-email deployer@example.com \
  --app-dir /absolute/path/to/app
```

Before a forced login, export only the sibling reader's existing local settings:
`TINYHOST_RESEND_READER_API_KEY_FILE`, `TINYHOST_RESEND_OTP_LEDGER_FILE`,
`TINYHOST_VPS_EMAIL_FROM`, and `TINYHOST_VPS_DOMAIN`. The exact server must be
`https://admin.<domain>`; the wrapper derives that admin host from the one root
domain and never accepts a separately supplied platform host. If the domain is
missing, invalid, or does not match `--server`, stop rather than weakening the
sibling reader's hostname validation.

The wrapper runs `tiny whoami --json` against the explicit server. It reuses a
saved credential only when the response is exact valid JSON and its identity
exactly matches the requested deployer email. A missing/invalid saved login or
a different exact identity triggers one `tiny login --force`; the wrapper sends
the email to the normal CLI prompt and obtains the OTP only through
`skills/tiny-full-stack-test/scripts/read-resend-otp.py`. It then proves the
exact identity again with `whoami --json`.

It performs exactly one `tiny deploy --json` and accepts it only when the safe
JSON success result confirms the CLI's built-in fresh anonymous HTTPS denial
proof. Do not retry login or deploy. Any failure blocks deployment or reports
the single failed attempt without leaking command output.

## Verification

Run the deterministic offline script tests; they use fake CLI and reader
processes and never contact a network or mutate a live host:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 skills/tiny-deployment-agent/scripts/test_deploy_once.py
```

Report only whether a saved identity was reused or a forced login occurred, the
safe deployment URL when successful, and the command outcome. Do not claim a
deployment succeeded on malformed output or on an incomplete public proof.
