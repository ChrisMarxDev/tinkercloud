# Threat Model

## Security objective

For a private active app, an actor receives app content or uses an app
capability only when the gateway has:

1. resolved the app from the request host;
2. validated an app-scoped session;
3. evaluated the current app policy;
4. produced an authorization context for that request.

This is the primary asset. Confidentiality of operator/deployer credentials,
viewer identity, release content, KV state, blob content and metadata, keys,
and update artifacts follows from it.

For the accepted post-V1 public-static posture, anonymous callers may receive
only active immutable static bytes after the gateway has resolved the app,
loaded the explicit public policy, loaded the current default-off operator
gate, proved capability-free active metadata, classified a non-reserved static
route, and produced the sealed public-static context. That context cannot call
any capability dispatcher.

## Trust boundaries

```text
Untrusted internet
  │
  ├── host, path, headers, cookies, archives, JSON, blob bytes/metadata,
  │   WebSocket frames
  ▼
Public gateway
  │ typed authorization context
  ▼
Protected dispatchers
  │ app-scoped repository calls
  ▼
SQLite and private filesystem

Tinkercloud ── outbound-only HTTPS ──▶ selected Resend or Postmark provider
Operator ── root/service boundary ─▶ host and recovery commands
```

App JavaScript, deployed files, deployers, viewers, archives, and all network
input are untrusted. The VPS operator and Tinkercloud binary/signing process are
trusted in V1.

## Threat inventory

| Threat | Example | Prevent | Detect / recover |
|---|---|---|---|
| Host confusion | `app.apps.example.com.evil.test` | strict canonical suffix parser | host fuzz tests |
| Auth bypass | unwrapped new route | typed protected-handler registry | route enumeration test |
| Cross-app access | supply another app ID, KV key, blob ID, or live channel | derive app ID from auth context; scope every lookup | two-app HTTP/storage/socket isolation suite |
| Session bleed | parent-domain cookie | host-only app cookie | cookie contract tests |
| Policy staleness | removed viewer still accesses | revision invalidation / current read | revocation latency test |
| OTP enumeration | response differs by allowlist | generic response and comparable flow | black-box response tests |
| OTP brute force | repeated codes | attempt and layered rate limits | audit alert |
| CSRF | app JS mutates platform data | separate origin/cookies; CSRF on control plane | browser integration tests |
| Open redirect | crafted return URL | server-side transaction + relative path only | redirect corpus |
| Path traversal | encoded `../`, symlink, or filename used as a blob path | open beneath root, no links; blob keys use only server IDs | archive/path/blob-ID fuzzing |
| Archive bomb | huge expansion | entry/byte/depth budgets | rejected deployment telemetry |
| Zip collision | Unicode/case duplicate | canonical collision rejection | archive corpus |
| Stored XSS in admin | malicious app name | contextual escaping and CSP | browser tests |
| Uploaded active content | HTML/SVG blob executes in the app origin | authenticated attachment download, `nosniff`, no public/inline URL | browser download/header tests |
| Secret leakage | logs contain token/OTP | structured allowlisted logging | log scanning tests |
| Disk exhaustion | deployments/KV/blobs fill disk | quotas and watermarks | health alert; reject growth writes |
| Malicious/broken update | compromised or incompatible binary | signature, compatibility gate, local rollback state | post-restart health gate |
| Direct origin bypass | alternate Tinkercloud process/port | process-level TCP 80/443 bind confinement plus operator-owned firewall | Tinkercloud-owned socket inventory check |
| Supply-chain compromise | unsafe dependency/update | pin, scan, sign, reproduce | release provenance |
| Secret disclosure | provider key reaches app bundle/browser/log | server-only connection store; no read-secret API | secret scanning and audited rotations |
| Confused deputy | app uses shared Jira/LLM credential too broadly | explicit app grant and operation/resource scopes | per-connection/app audit |
| Provider cost abuse | compromised app loops LLM calls | app/viewer/global budgets and rate limits | cost alerts and immediate grant disable |
| Outbound request abuse | adapter becomes an SSRF proxy | fixed adapter destinations and schemas | denied-destination telemetry |
| Socket resource abuse | connection/subscription flood or slow consumer | hard quotas, rate limits, bounded queues | disconnect and realtime load tests |
| Public-context privilege escalation | public page calls KV/blob/live/LLM or login route | sealed public-static context accepted only by static runtime; all `/_tinker/*` denied | route registry and dispatcher-call counters |
| Gate/policy cache staleness | bytes remain public after disable | current gate/policy every request; no-store pilot | next-request transition matrix |
| Analytics tracking profile | store raw cookie/IP/path/referrer/identity | app-scoped HMAC digest only; fixed fields and 30-day cutoff | DB/log schema inspection and retention tests |
| Catalog metadata leak | browser filters a list containing denied apps | policy-filtered bounded server query from global identity | two-owner policy/revocation matrix |

## Special cases

### Pre-authentication endpoints

Login and OTP routes are deliberately public but have no release or app-data
repository dependency. A valid active app is still required. They return
generic messages, enforce limits before expensive work, and never accept
client-provided identity as authenticated identity.

### Post-V1 public static apps

V1 exposes no public-app mode. The accepted post-V1 extension requires an
explicit current public policy plus a revisioned operator gate and still passes
through app resolution, active immutable evidence, route classification,
security headers, and a sealed static authorization decision. It excludes all
anonymous identity, login, SDK, data, blob, realtime, LLM, and future
capability routes. Missing or unavailable gate state means no anonymous access.

### Local insights privacy

Insights are server-side request-outcome counters, not app instrumentation.
They retain only UTC daily totals and app-scoped keyed random-cookie digests for
30 days. The raw cookie, identity/session, email, IP, URL/query/path, referrer,
user agent, content, geography, and device data never enter analytics storage.
A bounded full/unavailable recorder drops evidence rather than delaying or
altering an authorized app response.

### Same-origin platform APIs

Same-origin makes browser use simple but does not relax authorization. The
current app comes from the host. State-changing methods validate content type,
exact origin where meaningful, request size, feature flags, and app quota.
Blob downloads are authorized like every other app-plane request and are sent
as private attachment responses with MIME sniffing disabled.

### Future operator-managed capabilities

Provider credentials never cross into browser code. Tinkercloud resolves an
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
- global email, deployment-upload, and blob-upload budgets.

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
