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
| rollback | `deploy:activate` | owned app + prior release | `deployment.rolled_back` |

Successful security-sensitive mutations commit their metadata and audit event in
the same transaction. Audit failure denies the mutation.

## Archive transport boundary

JSON control payloads are capped independently from archive uploads. A deployment
archive is an authenticated, bounded byte stream whose maximum is the server's
effective `archive_upload_bytes` limit (100 MiB by default). The transport
rejects a declared body above that limit before staging, and consumes at most
one additional byte for an unknown-length stream to prove and return the same
rejection. It never buffers the archive in memory.

The CLI creates one private temporary archive per logical `tiny deploy`
invocation, removes it on every exit path, and sends a fresh cryptographically
random idempotency key. A retry of that invocation reuses its key; a later
invocation, including one for the same app, receives a different key.

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
active deployment pointer in one transaction. Rollback likewise reinstalls the
target deployment's canonical policy with its pointer. The owner remains
server-derived and implicit; the client never supplies an owner or app ID for
this binding.

The optional manifest description is immutable release metadata in
`manifest_json`, not a mutable application field. Finalized deployment metadata
with a missing or malformed manifest is unavailable to the dashboard; it is
never rendered as an empty description. Intermediate uploads that have no
final manifest may display no description.

The authenticated activation result includes both the protected `url` and the
server-derived `app_suffix`. A deployer accepts a URL only when it is exactly
`https://{slug}.{app_suffix}/`; it must not infer the app suffix from the
control-plane host, because a deployment may use `tiny.example.com` for control
and `*.apps.example.com` for app traffic.

## Denial charter

Activation gates receive a server-derived immutable candidate record, never a
client-supplied release path. Policy, certificate, or public-denial gate failure
preserves the previous active pointer.

- An inactive, expired, revoked, wrong-owner, wrong-app, or missing token is
  denied before control mutation.
- App/release ownership is checked by the repository/service, not only the CLI.
- Missing audit capability, valid candidate policy, certificate readiness, or probe result
  denies activation.
- Malformed manifests and hostile archives are rejected before staging becomes
  an immutable release.
- Description input with invalid UTF-8, a Unicode control character, a Unicode
  line/paragraph separator, or more than 280 Unicode code points after edge
  trimming is rejected. A dashboard cannot use release metadata to create an
  immutable/raw release URL; its optional launch control targets only the
  configured stable `https://{slug}.{app_suffix}/` gateway origin, retains
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
