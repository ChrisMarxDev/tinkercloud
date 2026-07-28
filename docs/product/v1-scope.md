# V1 Scope

## Product outcome

An operator can install TinyHost on one Linux VPS, authorize a deployer, and
give that deployer a workflow that ends with a protected app URL:

```text
tiny login
tiny deploy ./dist --allow alice@example.com
```

An allowlisted viewer can authenticate by email and use the app. An anonymous,
revoked, or unrelated viewer cannot retrieve any part of the app.

## V1 capability slices

| Slice | Included | Exit signal |
|---|---|---|
| Secure gateway | Host resolution, global viewer identity, app-bound handoff, app session, policy evaluation, protected static files | Negative requests cannot retrieve any asset |
| Deployment | Deployer login, archive upload, validation, immutable release, atomic activation, rollback | One command returns a verified protected URL |
| App primitives | First-class TypeScript SDK, capability discovery, current-user, JSON KV, lightweight local blobs, and ephemeral realtime | A static app persists current state and bounded attachments and receives live notifications without auth, database, bucket, or mount ceremony |
| Operations | Hetzner-first setup, status, administration, signed update/rollback | Operator initializes a clean VPS with one command |
| Hardening | Rate limits, fuzzing, failure injection, resource limits, reproducible release | Security and recovery gates are repeatable |

## Deliberate reductions from the broad concept

The first production-capable cut should not implement every initially proposed
primitive at once.

- **Ship tiny KV, lightweight local blobs, and ephemeral realtime together.**
  KV remains current JSON state, blobs hold bounded app-shared attachments, and
  sockets provide the immediate collaborative feeling without promising
  history, replay, multi-node fan-out, public files, or remote durability.
- **Ship one-time codes before polishing magic-link exchange.** Codes work across
  devices and avoid redirect-state complexity.
- **Use server-rendered operator/deployer pages.** The gateway, deployment
  safety, and recovery story matter more than dashboard framework sophistication.
- **Use per-host HTTP challenge certificates first.** Wildcard DNS automation
  becomes an adapter once core routing is stable.

## Included

- One operator and multiple deployers.
- Exact-email and email-domain viewer rules.
- Private-only app policy; an owner is always an implicit viewer.
- App-scoped opaque viewer sessions.
- Scoped deployer/API tokens with hashed-at-rest secrets.
- Static archive deployment and SPA fallback.
- Current-user, bounded JSON KV, app-shared local blob, and app-scoped
  WebSocket APIs.
- Browser-first `@tinyhost/sdk` and app capability discovery.
- Local audit events, structured operational logs, and bounded retention.
- Signed self-update with health verification and local update rollback state.

## Non-goals

- Arbitrary containers, backend processes, schedulers, or user code execution.
- Custom domains, organizations, complex roles, billing, or a marketplace.
- Multi-node operation, high availability, external PostgreSQL, or distributed
  queues.
- Public object URLs, direct bucket access, or browser-visible platform secrets.
- Remote blob backends, FUSE mounts, standalone object-store services,
  resumable uploads, transformations, and per-viewer blob ACLs.
- General-purpose database features for apps.
- Durable event history, replay guarantees, ordered delivery, or multi-node
  realtime.
- Operator-supplied secrets, LLM providers, Jira, internal API adapters, and
  arbitrary outbound proxying.
- Operator-managed backup, remote backup storage, and full-host restore.
- Perfect zero-downtime platform upgrades; release activation must be atomic.

## V1 product risks

| Risk | Likelihood | Impact | Response |
|---|---:|---:|---|
| A forgotten request path bypasses authorization | 3 | 5 | Typed authorization context and matrix tests |
| Email outage locks out new sessions | 3 | 4 | Existing sessions continue; operator diagnostics; fail closed |
| Archive extraction damages or escapes storage | 2 | 5 | Staged custom extractor with strict budgets |
| ACME issuance delays first access | 3 | 3 | Provision during deploy; do not announce ready early |
| Scope turns into a PaaS | 4 | 4 | No backend processes in V1 |
| Self-update leaves an unusable server | 2 | 5 | Signed artifact, pre-update rollback snapshot, health-gated replacement |

## Do / do not

**Build this product**, because a structural access gateway removes a repeated,
high-impact failure mode from every small app. **Do not build the broad runtime
yet**: the secure static deployment loop yields the core value at substantially
lower attack surface and operational complexity.

Impact: **5/5**. Complexity: **5/5**. V1 secure-static slice yield:
**4/5** after scope reduction.
