---
name: tinkercloud-deployment-agent
description: Run one opt-in local Tinkercloud test deployment from a deployer's workstation using a saved CLI credential or the normal Resend-backed CLI OTP flow. Use when controlled test automation must deploy a known app directory to a known HTTPS Tinkercloud server as one exact deployer identity.
---

# Tinkercloud deployment agent

Use this skill only for explicit, local deployer-workstation test automation.
It deliberately completes the normal human deployer's OTP login and stores the
ordinary local CLI credential. It is not production CI/noninteractive
deployment-agent authentication: that path requires a separately provisioned,
app-scoped deployer token and must not use this wrapper or an interactive OTP.
It accepts requested deployer identities only in the local automation domain
`christopher-marx.de`; normal CLI and human login policy are not restricted.

## Boundary

The deployment agent is non-human automation acting through deployer authority.
It cannot act as an operator or viewer, create a bearer, choose an app identity,
or infer an access broadening. V1 is historically private-only; the accepted
post-V1 public-static extension still requires a capability-free manifest,
current operator gate, and explicit deployer acknowledgement. The gateway owns
activation, public/private proof, and app identity.

Never accept or print a bearer, OTP, Resend key, provider secret, app ID, or
viewer identity. Keep the reader key and consumed-message ledger local; never
copy either to a VPS, project, manifest, or source control. Missing, malformed,
redirected, incompatible, ambiguous, or timed-out state is a denial.

## One-deployment workflow

Ask only for these non-secret values when they cannot be discovered safely:

- exact absolute path to the `tinker` CLI;
- exact HTTPS platform server URL;
- exact deployer email; and
- exact absolute app directory containing `tinker.yaml`.

Private is the default: the wrapper sends a bare `tinker deploy` and never
derives public reach from `tinker.yaml`. If, and only if, the caller has
deliberately chosen anonymous public static access for a capability-free public
manifest, add the one explicit acknowledgement:

```sh
python3 skills/tinkercloud-deployment-agent/scripts/deploy_once.py \
  --tinker /absolute/path/to/tinker \
  --server https://admin.example.com \
  --deployer-email deployer@example.com \
  --app-dir /absolute/path/to/public-app \
  --confirm-public
```

`--confirm-public` is forwarded exactly once as `tinker deploy
--confirm-public …`; it is never inferred from the manifest, a saved
credential, or a prior deployment. It does not make an invalid or private
manifest public, bypass the current operator public-static gate, or authorize
capabilities. The CLI and gateway perform those independent checks. Omit it
for every private deployment. A duplicated or value-bearing form such as
`--confirm-public=true` is invalid and the wrapper does not invoke the CLI.

Use the checked-in deterministic wrapper once:

```sh
python3 skills/tinkercloud-deployment-agent/scripts/deploy_once.py \
  --tinker /absolute/path/to/tinker \
  --server https://admin.example.com \
  --deployer-email deployer@example.com \
  --app-dir /absolute/path/to/app
```

Before a forced login, export only the sibling reader's existing local settings:
`TINKERCLOUD_RESEND_READER_API_KEY_FILE`, `TINKERCLOUD_RESEND_OTP_LEDGER_FILE`,
`TINKERCLOUD_VPS_EMAIL_FROM`, and `TINKERCLOUD_VPS_DOMAIN`. The exact server must be
`https://admin.<domain>`; the wrapper derives that admin host from the one root
domain and never accepts a separately supplied platform host. If the domain is
missing, invalid, or does not match `--server`, stop rather than weakening the
sibling reader's hostname validation.

Before the one permitted wrapper invocation, check whether the execution
environment restricts outbound network access (for example, a Codex sandbox).
If it does, obtain scoped permission for that exact wrapper command first. The
permission must allow HTTPS to the explicit Tinkercloud server and, only when a
forced login may be needed, the fixed Resend API used by the local reader. Do
not run the command unprivileged as a connectivity probe: a sandbox-denied
wrapper invocation still consumes this workflow's one allowed attempt. If that
permission cannot be obtained, stop and report the environment limitation
without invoking the wrapper.

Before checking any CLI/app path, reading reader configuration, or calling the
CLI, the wrapper lowercases the requested email and requires its domain to be
exactly `christopher-marx.de`. It rejects subdomains, suffix lookalikes, the
hyphenless domain, and all unrelated domains. The wrapper then runs `tinker
whoami --json` against the explicit server. It reuses a saved credential only
when the response is exact valid JSON and its identity exactly matches the
requested deployer email. A missing/invalid saved login or a different exact
identity triggers one `tinker login --force`; the wrapper sends the email to the
normal CLI prompt and obtains the OTP only through
`skills/tinkercloud-full-stack-test/scripts/read-resend-otp.py`. It then proves the
exact identity again with `whoami --json`. This local-wrapper restriction does
not affect a saved CLI credential used directly or Tinkercloud's normal
human-login policy.

It performs exactly one `tinker deploy --json` and accepts it only when the safe
JSON success result confirms the CLI's built-in fresh anonymous HTTPS denial
proof. Do not retry login or deploy. Any failure blocks deployment or reports
the single failed attempt without leaking command output.

On failure, the wrapper writes exactly one fixed identifier to stderr. It never
prints CLI output, an exception, email, path, app ID, OTP, token, or provider
detail. Report that identifier verbatim to the supervising operator and stop;
do not retry the wrapper, login, or deployment.

| Identifier | Meaning |
| --- | --- |
| `input_validation` | A required local input, path, server URL, manifest, or forbidden credential environment was invalid or unsafe. |
| `saved_identity_check` | The saved CLI credential could not be safely classified as the requested deployer or a normal missing-login response. |
| `forced_login` | The one permitted forced login, local OTP reader configuration, or OTP exchange failed. |
| `post_login_identity_check` | The login completed but the follow-up identity proof did not exactly match the requested deployer. |
| `deployment_result_validation` | The one permitted deployment failed or did not return a safe verified deployment result. |
| `internal_failure` | An unexpected wrapper failure occurred; report it without adding diagnostics or retrying. |

## Verification

Run the deterministic offline script tests; they use fake CLI and reader
processes and never contact a network or mutate a live host:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 skills/tinkercloud-deployment-agent/scripts/test_deploy_once.py
```

Report only whether a saved identity was reused or a forced login occurred, the
safe deployment URL when successful, and the command outcome. Do not claim a
deployment succeeded on malformed output or on an incomplete public proof.
