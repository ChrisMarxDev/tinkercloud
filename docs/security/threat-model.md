# Threat Model

## Security objective

For a private active app, an actor receives app content or uses an app
capability only when the gateway has:

1. resolved the app from the request host;
2. validated an app-scoped session;
3. evaluated the current app policy;
4. produced an authorization context for that request.

This is the primary asset. Confidentiality of operator/deployer credentials,
viewer identity, release content, KV state, keys, and update artifacts follows
from it.

## Trust boundaries

```text
Untrusted internet
  │
  ├── host, path, headers, cookies, archives, JSON, WebSocket frames
  ▼
Public gateway
  │ typed authorization context
  ▼
Protected dispatchers
  │ app-scoped repository calls
  ▼
SQLite and private filesystem

TinyHost ── outbound-only HTTPS ──▶ Resend
Operator ── root/service boundary ─▶ host and recovery commands
```

App JavaScript, deployed files, deployers, viewers, archives, and all network
input are untrusted. The VPS operator and TinyHost binary/signing process are
trusted in V1.

## Threat inventory

| Threat | Example | Prevent | Detect / recover |
|---|---|---|---|
| Host confusion | `app.apps.example.com.evil.test` | strict canonical suffix parser | host fuzz tests |
| Auth bypass | unwrapped new route | typed protected-handler registry | route enumeration test |
| Cross-app access | supply another app ID to KV or live channel | derive app ID from auth context | two-app HTTP/socket isolation suite |
| Session bleed | parent-domain cookie | host-only app cookie | cookie contract tests |
| Policy staleness | removed viewer still accesses | revision invalidation / current read | revocation latency test |
| OTP enumeration | response differs by allowlist | generic response and comparable flow | black-box response tests |
| OTP brute force | repeated codes | attempt and layered rate limits | audit alert |
| CSRF | app JS mutates platform data | separate origin/cookies; CSRF on control plane | browser integration tests |
| Open redirect | crafted return URL | server-side transaction + relative path only | redirect corpus |
| Path traversal | encoded `../` or symlink | open beneath root, no links | archive/path fuzzing |
| Archive bomb | huge expansion | entry/byte/depth budgets | rejected deployment telemetry |
| Zip collision | Unicode/case duplicate | canonical collision rejection | archive corpus |
| Stored XSS in admin | malicious app name | contextual escaping and CSP | browser tests |
| Secret leakage | logs contain token/OTP | structured allowlisted logging | log scanning tests |
| Disk exhaustion | deployments/KV fill disk | quotas and watermarks | health alert; reject writes |
| Malicious/broken update | compromised or incompatible binary | signature, compatibility gate, local rollback state | post-restart health gate |
| Direct origin bypass | alternate TinyHost process/port | process-level TCP 80/443 bind confinement plus operator-owned firewall | TinyHost-owned socket inventory check |
| Supply-chain compromise | unsafe dependency/update | pin, scan, sign, reproduce | release provenance |
| Secret disclosure | provider key reaches app bundle/browser/log | server-only connection store; no read-secret API | secret scanning and audited rotations |
| Confused deputy | app uses shared Jira/LLM credential too broadly | explicit app grant and operation/resource scopes | per-connection/app audit |
| Provider cost abuse | compromised app loops LLM calls | app/viewer/global budgets and rate limits | cost alerts and immediate grant disable |
| Outbound request abuse | adapter becomes an SSRF proxy | fixed adapter destinations and schemas | denied-destination telemetry |
| Socket resource abuse | connection/subscription flood or slow consumer | hard quotas, rate limits, bounded queues | disconnect and realtime load tests |

## Special cases

### Pre-authentication endpoints

Login and OTP routes are deliberately public but have no release or app-data
repository dependency. A valid active app is still required. They return
generic messages, enforce limits before expensive work, and never accept
client-provided identity as authenticated identity.

### Future public apps

V1 exposes no public-app mode. A future version would require an explicit
current policy plus an operator configuration gate and would still pass through
app resolution, status, quotas, security headers, and protected dispatch.

### Same-origin platform APIs

Same-origin makes browser use simple but does not relax authorization. The
current app comes from the host. State-changing methods validate content type,
origin where meaningful, request size, feature flags, and app quota.

### Future operator-managed capabilities

Provider credentials never cross into browser code. TinyHost resolves an
operator connection only after app/viewer authorization, effective grant,
operation/resource scope, quota, and destination checks. Early capability
adapters are narrow and typed; a generic secret-injecting HTTP proxy is outside
the security model.

## Abuse controls

Use layered token buckets with bounded state:

- request IP or privacy-preserving prefix;
- normalized-email keyed hash;
- app;
- deployer/token;
- global email and upload budgets.

Limits are configuration with safe minimum/maximum bounds. Rejected requests
should be cheap and should not disclose which dimension fired.

## Security review questions

For every new endpoint:

1. Is it pre-auth, control-plane, or protected app-plane?
2. Who derives app identity and actor identity?
3. What happens when database, policy, audit, or quota lookup fails?
4. Can an app origin call it with ambient credentials?
5. What are the maximum bytes, rows, work, and time?
6. What anonymous, revoked, cross-app, malformed, and concurrency tests exist?
7. What secret or personal data can enter logs or audit events?

## Deferred threat model

Backend processes introduce sandbox escape, network egress, secret injection,
resource scheduling, dependency download, and identity assertion threats. They
require a new model and architecture decision before any backend runtime code.
