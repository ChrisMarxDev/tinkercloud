# Actors and critical journeys

`operator`, `deployer`, and `viewer` are roles, not interchangeable account
types. One person may hold more than one role, but authority and credentials do
not transfer between them. A deployment agent is non-human automation using a
scoped deployer token.

The exhaustive human flows are the browsable
[operator flow](../../concept/flows/operator.html) and
[deployer flow](../../concept/flows/deployer.html). Their
`information requested` and `derived/defaulted` columns are the product gate
for Principle 17: ask only for necessary information.

## Operator

Owns the VPS, domain, DNS, SSH, firewall, email-provider relationship, installed
server, and platform trust root.

Critical journey:

```text
acquire domain + supported dedicated VPS + root SSH
→ install one verified signed server binary
→ tinyhost setup discovers the host
→ ask root domain + operator email
→ derive admin.<domain>, <slug>.<domain>, and sender
→ pause with one wildcard DNS + Resend actions
→ resume without repeating valid state
→ ingest provider secret into root-owned credentials
→ generate config, internal secrets, SQLite, service, TLS, and probes
→ sign in to dashboard
→ reconcile one exact active-deployer allowlist
→ diagnose, suspend/revoke, update, and recover through narrow controls
```

The operator does not author normal config before setup. Root SSH is the
email-independent recovery authority. Total VPS loss has no V1 recovery
guarantee.

## Deployer

Owns only their apps, current access policies, immutable deployments, and scoped
automation tokens.

Critical journey:

```text
operator activates deployer email
→ install signed tiny client as normal OS user
→ tiny deploy .
→ reuse existing output, or run one exact project-owned build action when absent
→ ask/verify/cache platform only when missing
→ reuse verified bearer or complete OTP only after definite unauthorized state
→ derive project/manifest defaults
→ ask only about ambiguity or deliberate customization
→ write tiny.yaml as a deterministic receipt
→ upload, validate, seal, activate, and prove anonymous denial
→ receive stable protected URL
→ manage viewer policy/tokens and deploy later builds
```

`tiny login --force` is deliberate account switching. `tiny logout` revokes the
exact server-side CLI bearer before local removal. The dashboard searches owned
apps by slug/current description, filters status, and links only to stable
protected app origins. Deployment history is read-only in V1. Confirmed app
deletion is permanent and leaves no tombstone or restore action.

## Viewer

Has no platform role or password. Email identity and app access are separate:

```text
open app URL
→ existing rotating admin-host global browser identity, or generic OTP once
  per browser profile when absent
→ server-created one-time handoff bound to this app and safe return path
→ current app policy evaluated before grant and again at callback
→ host-only app session created
→ return to the requested app path
```

The global identity proves only the email. A current role check separately
controls dashboard access, and each app independently decides whether that
identity is allowed. No deployed app receives the admin cookie, and an app
logout revokes only its app session. Account switching and `Sign out of
TinyHost` are explicit admin-host flows that revoke the prior identity family
and child app sessions without revoking CLI bearers.

## Deployment agent

Uses a non-interactive scoped token, explicit server and manifest state, and
deterministic output. It never receives prompts or implicit credential/config
writes.

```text
inspect build output
→ validate explicit manifest and policy
→ deploy immutable archive
→ wait for activation checks
→ independently probe anonymous denial
→ report protected URL and deployment ID
```

The agent fails the job when denial cannot be verified.
