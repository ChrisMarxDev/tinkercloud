# Control and Deployment Contract (V1)

This contract defines the technology-neutral boundary between the deployer CLI
and the control domain. It deliberately does not define an HTTP transport.

## Commands

Every command carries a server-derived authenticated actor, an explicit target,
an idempotency key for mutations, and a request ID. The actor is never supplied
as an app ID or owner ID in deployer-controlled request data.

| Command | Required permission | Target | Audit action |
| --- | --- | --- | --- |
| create app | `app:create` | requested slug | `app.created` |
| replace policy | `access:write` | owned app | `policy.replaced` |
| create token | `token:create` | owned app or actor | `token.created` |
| revoke token | `token:revoke` | token owned by actor | `token.revoked` |
| create deployment | `deploy:create` | owned app | `deployment.created` |
| activate deployment | `deploy:activate` | owned app + release | `deployment.activated` |
| inspect managed data | `data:read` | owned app + bounded KV/document selector | request metadata only |
| mutate managed data | `data:write` | owned active app + one versioned KV/document selector | `app_data.mutated` |

Successful control-database security-sensitive mutations commit their metadata
and audit event in the same transaction. App-data mutations are different:
their data lives in a separate private SQLite file, so they require a durable
control-database audit intent before the app-data mutation. A successful
app-data commit is followed by an outcome update used for exact idempotent
replay. An interrupted outcome is reconciled only when current versioned state
proves the intended result; otherwise it fails closed. This is not a
cross-database rollback claim. Intent failure denies before the app-data write.

Managed-data commands are capability operations, not deployment commands. The
route slug is an ownership target only; after exact scope/app-binding/ownership
validation Tinkercloud derives the immutable app ID and passes a typed
deployer-data context to the existing app-data service. These commands never
accept a database/file selector, SQL, schema, migration, bulk operation, or
viewer identity. The full API and denial charter live in
[`../api/deployer-data-contract.md`](../api/deployer-data-contract.md).

## Archive transport boundary

JSON control payloads are capped independently from archive uploads. A deployment
archive is an authenticated, bounded byte stream whose maximum is the server's
effective `archive_upload_bytes` limit (100 MiB by default). The transport
rejects a declared body above that limit before staging, and consumes at most
one additional byte for an unknown-length stream to prove and return the same
rejection. It never buffers the archive in memory.

The CLI creates one private temporary archive per logical `tinker deploy`
invocation, removes it on every exit path, and sends a fresh cryptographically
random idempotency key. A retry of that invocation reuses its key; a later
invocation, including one for the same app, receives a different key.

Before archiving, `tinker deploy [DIR]` uses `DIR` or `.` and loads a local strict
`tinker.yaml`. A human deploy may create a missing manifest through the bounded
local wizard; JSON mode never does. The deployer credential is first proven by
the API-version and authenticated `whoami` checks. Only a definite unauthorized
result may enter the existing OTP flow, and a fresh bearer is persisted only
after that proof succeeds. Explicit `--server` applies to only that invocation.

## Deployment state and activation

The only state transitions are:

```text
uploading -> uploaded -> validating -> staged -> verified -> active
      \-> failed       \-> rejected       \-> failed
active -> superseded
```

`rejected` is invalid deployer input; `failed` is an operational failure. Both
are terminal. Only a `verified` immutable release with an activatable candidate
policy,
ready certificate, and passing anonymous-denial probe can become active. A
failed activation restores the previously active release.

During upload the server validates and persists the deployment manifest's
canonical private allowlist. Activation installs that exact allowlist and the
active deployment pointer in one transaction. The owner remains server-derived
and implicit; the client never supplies an owner or app ID for this binding.

Before activation, a newly staged app's exact HTTPS origin receives a bounded
certificate-readiness probe at `/_tinker/api/v1/app`. This is a protected,
non-mutating endpoint: it must never issue a viewer handoff, set a cookie, or
otherwise alter authentication state. The server may retry only transient/readiness
failure within one cancellable 45-second overall budget, with finite attempts
and bounded individual requests. Each attempt disallows redirects, requires the
same HTTPS host and a verified TLS chain, and accepts only the normal non-
redirect readiness status range. Policy, ownership, candidate validation,
anonymous-denial evidence, and all post-activation checks are never retried or
weakened by this certificate window.

The deployer client gives upload, status, and activation control requests a
bounded transport budget longer than the server's 45-second first-host
readiness gate. The caller's cancellation still wins. A shorter generic
control-client timeout must not abandon an activation whose readiness gate is
still legitimately running.

The optional manifest description is immutable release metadata in
`manifest_json`, not a mutable application field. Finalized deployment metadata
with a missing or malformed manifest is unavailable to the dashboard; it is
never rendered as an empty description. Intermediate uploads that have no
final manifest may display no description.

The authenticated activation result includes both the protected `url` and the
server-derived root `domain`. A deployer accepts a URL only when it is exactly
`https://{slug}.{domain}/`. The dashboard is always the reserved exact host
`admin.{domain}`; the deployer must use the returned domain rather than derive
it from a control client URL.

## Denial charter

Activation gates receive a server-derived immutable candidate record, never a
client-supplied release path. Policy, certificate, or public-denial gate failure
preserves the previous active pointer.

- An inactive, expired, revoked, wrong-owner, wrong-app, or missing token is
  denied before control mutation.
- A JSON deploy with a missing manifest or credential is denied without stdin,
  OTP, manifest creation, or credential writes. A local path with traversal or
  a symlinked output/fallback component is denied before archive creation.
- App/release ownership is checked by the repository/service, not only the CLI.
- Missing audit capability, valid candidate policy, certificate readiness, or probe result
  denies activation.
- Certificate readiness is pre-activation evidence only: redirect, wrong-host,
  unverified-TLS, exhausted-budget, cancelled, or non-ready outcomes deny and
  preserve the prior active pointer. The finite retry window applies only to
  certificate/readiness transport failures; it never retries policy, ownership,
  probe, or post-activation evidence.
- Activation failure responses use only these safe stable codes:
  `activation_policy_not_ready`, `activation_certificate_not_ready`,
  `activation_candidate_probe_failed`, `activation_capability_not_ready`, and
  `activation_commit_failed`. They are conflict responses with the existing
  gateway request ID echoed in the JSON error envelope. They never disclose a
  raw TLS, policy, probe, capability, or persistence detail.
- After a committed activation, an exact replay with the same deployment and
  activation idempotency key returns the same successful activation receipt.
  The repository checks durable activation audit evidence before verified-state
  planning; it must not rerun gates, create a policy revision or audit event,
  or trigger a live-session side effect. A different key, deployment, app, or
  durable state is never treated as that replay.
- Deployer-initiated selection of a previous release is not a V1 control
  command. The CLI, bearer API, and dashboard must deny or omit rollback paths;
  automatic failed-activation preservation remains required.
- Malformed manifests and hostile archives are rejected before staging becomes
  an immutable release.
- Description input with invalid UTF-8, a Unicode control character, a Unicode
  line/paragraph separator, or more than 280 Unicode code points after edge
  trimming is rejected. A dashboard cannot use release metadata to create an
  immutable/raw release URL; its optional launch control targets only the
  configured stable `https://{slug}.{domain}/` gateway origin, retains
  normal gateway authentication, and uses a new-tab `noopener noreferrer`
  navigation.
- An archive larger than the effective upload limit is rejected and its staging
  directory is removed. A valid archive larger than the JSON request limit is
  accepted when it remains within the effective archive limit.
- A verified release has canonical file evidence: sorted relative path, byte
  size, and SHA-256 for every regular file, plus an aggregate hash over those
  fields. `deployment_files` persists that evidence atomically with verified
  deployment metadata. A pre-existing content-addressed directory is reused
  only after re-inspection exactly matches the candidate evidence. Read-time
  integrity verification compares the active filesystem tree with both the
  aggregate hash and persisted file evidence; a mismatch denies before app
  bytes are served.
- Finalization seals every file read-only and directory non-writable, syncs
  file data and directory metadata, renames only after sealing, and syncs the
  destination parent after rename. Any chmod, walk, sync, rename, or metadata
  failure leaves the candidate non-active and preserves the previous pointer.
- Archives with traversal, links, devices, collisions, unsafe modes, or any
  configured entry/size/depth budget violation are rejected and leave no served
  files.
- V1 extraction runs only into a newly-created private staging directory. Linux
  open-beneath hardening remains an exit gate and must use Go >=1.25.12 or
  >=1.26.5; the vulnerable Go 1.24 `os.Root` implementation is not used.
- A CLI result is successful only after both server candidate evidence and a
  fresh deployer-side anonymous GET to the exact server-derived protected URL
  pass. That GET reuses its real TLS transport but has no bearer token or
  cookie jar, follows no redirect, and requires the bounded gateway `401` JSON
  `not_authorized` envelope with a valid request ID matching `X-Request-ID`,
  `Cache-Control: no-store`, and `X-Content-Type-Options: nosniff`. A 404,
  2xx, redirect, wrong URL/host/scheme, malformed or oversized denial,
  timeout, or transport failure denies CLI success.
- After the server has returned a successful activation result, the CLI may
  retry only transient DNS, TLS, connection, timeout, `404`, `502`, `503`, or
  `504` public-probe outcomes within one finite 45-second budget. Every attempt
  is anonymous, bounded, exact-origin, and redirect-denying. A wrong
  URL/host/scheme, redirect, `2xx`, unexpected `401`, malformed or oversized
  denial, mismatched request ID, or unsafe header is terminal and is never
  retried into success.
- Exhausting or cancelling the post-activation public-probe budget does not
  rewrite committed server state. The CLI returns non-success code
  `active_but_unverified` with only the server-returned deployment ID,
  protected URL, literal state `active`, and a stable safe reason category.
  It never includes a bearer, cookie, raw transport error, response body,
  response headers, internal path, or policy membership. A failure before a
  successful activation response remains `deploy_failed` and carries no active
  receipt.
- A later `tinker deploy` invocation is a new immutable deployment attempt even
  when its release hash matches the active release. Hash equality alone never
  proves activation or policy installation. Activating a same-hash candidate
  remains valid and atomically supersedes the old deployment, because the
  candidate may carry a different reviewed access policy.
