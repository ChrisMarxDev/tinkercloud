# Actors and Critical Journeys

## Operator

Owns infrastructure and the trust root.

Critical journeys:

1. Initialize a clean Hetzner VPS, DNS, TLS, email, database, and first operator.
2. Authorize or revoke a deployer.
3. Suspend an app without touching its files.
4. Diagnose email, disk, certificate, and database health.
5. Apply a signed update and automatically return to the prior working version
   when its health gate fails.

The operator has a root-only VPS recovery path that does not depend on email.

## Deployer

Owns only their apps, policies, releases, and scoped automation tokens.

Critical journeys:

1. Authenticate the CLI by email OTP.
2. Deploy a directory using flags or `tiny.yaml`.
3. See validation and protection checks before “ready.”
4. Add or revoke viewer access immediately.
5. Roll back to a prior immutable release.

## Viewer

Has no platform account requirement beyond an email identity.

Critical journey:

```text
open app URL
→ existing global viewer identity, or generic email OTP once per browser profile
→ server-created one-time handoff bound to this app
→ policy is re-evaluated before grant and before local session creation
→ receive app-scoped host-only session
→ return to original safe path
```

The global identity proves only the email. Each app independently evaluates
that email against current policy; no deployed app receives the global cookie.

## Deployment agent

Uses a non-interactive scoped token and deterministic output.

Critical journey:

```text
inspect build output
→ validate manifest and allowlist
→ deploy immutable archive
→ wait for activation checks
→ independently probe anonymous denial
→ report protected URL and release ID
```

The agent must fail the job when denial cannot be verified.
