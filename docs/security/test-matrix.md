# Security Test Matrix

This matrix defines proof obligations, not test implementation.

## Protected surface matrix

Every protected surface runs the same actor variants.

| Surface | Anonymous | Wrong app session | Revoked viewer | Allowed viewer | Suspended app |
|---|---|---|---|---|---|
| `/` HTML | deny/challenge, zero bytes | deny | deny next request | serve | generic deny |
| JS/CSS/image asset | deny, zero bytes | deny | deny | serve | deny |
| SPA fallback | deny before fallback | deny | deny | serve fallback | deny |
| source map / dotfile | deny or unavailable | deny | deny | policy + file rules | deny |
| `/_tiny/api/me` | 401 | 403/401 | 403 | scoped identity | deny |
| `/_tiny/api/kv/*` | 401 | deny | deny | scoped operation | deny |
| `/_tiny/api/v1/blobs` upload/list | deny, no mutation | deny | deny | bounded operation | deny |
| `/_tiny/api/v1/blobs/*` get/delete | deny, zero bytes/no mutation | deny | deny | bounded operation | deny |
| `/_tiny/ws/v1` | reject upgrade | reject | disconnect/reject | app-scoped connect | reject/disconnect |

## Cross-tenant matrix

Create App A and App B with different owners, viewers, releases, sessions, KV
entries, blobs, channels, and connections. For every repository, storage, HTTP,
and WebSocket operation, prove credentials for A
cannot observe or mutate B by:

- path or query app ID;
- hostname manipulation;
- session cookie replay;
- guessed record/release IDs;
- duplicate KV key or channel name;
- guessed blob ID, display filename, cursor, or storage key;
- rollback/deployment ID;
- token scope escalation.

## Routing corpus

Test:

- exact platform host;
- exact one-label app host;
- uppercase and trailing-dot input;
- host with port;
- suffix prefix/suffix tricks;
- extra labels;
- punycode and invalid Unicode;
- empty/oversized labels;
- untrusted forwarded host;
- absolute-form proxy requests;
- unknown app slug.

All ambiguous inputs fail closed.

## Host preflight corpus

Accept only Linux/amd64 with exact, single `/etc/os-release` assignments for
`ID=ubuntu` and `VERSION_ID=24.04` or `VERSION_ID=26.04`.

Reject before host mutation:

- non-Linux and non-amd64 runtimes;
- unreadable or malformed OS metadata;
- missing, duplicated, or conflicting `ID` and `VERSION_ID`;
- `ID_LIKE=ubuntu` on another distribution;
- Ubuntu 24.10, 25.04, 25.10, 26.10, and version prefixes/suffixes; and
- every release not explicitly added with contract and acceptance evidence.

## Archive corpus

Include benign archives and attacks:

- `../`, absolute, Windows drive, mixed separator paths;
- symlink/hardlink/device/socket entries;
- nested archives and compression bombs;
- duplicate and case/Unicode-colliding names;
- excessive files, depth, single-file size, and total size;
- sparse files and unsafe modes;
- truncated/corrupt headers;
- cancellation halfway through extraction.

Staging cleanup must not follow attacker-controlled links.

## Failure injection

At each stage, inject:

- SQLite busy/unavailable;
- disk full/critical watermark;

  - app creation, deployment creation, KV mutations, and blob uploads deny at the exact
    stop boundary and when disk measurement fails;
  - existing authorized static reads and security revocations still succeed;
  - cleanup rejects path traversal/symlinks, preserves active plus recovery
    release, and remains retryable after a read-only filesystem failure.
- filesystem permission or rename failure;
- blob short write, close/sync failure, staging/ready/deleting interruption,
  orphan bytes, missing ready bytes, and metadata/storage size/hash disagreement;
- fsync/walk failure while sealing a candidate, and an existing
  content-addressed directory whose bytes or file manifest were corrupted;
- process interruption before and after durable state change;
- Resend timeout/error;
- ACME pending/failure;
- audit append failure;
- probe timeout.

Expected outcome: no unauthorized content or blob bytes, no readable partial
blob or active release, previous release preserved, exact reconciled quota,
durable diagnosable state, cleanup retry safe.

## Revocation timing

Measure from committed mutation to denied request:

- access rule removed;
- session revoked;
- app suspended;
- deployer/token revoked;
- deployer authorization revoked.
- root operator recovery, including recovery to the same normalized email;
  every previously issued operator control credential is denied on the next
  request while a newly authenticated recovered operator succeeds.

The target for same-node V1 is the next request after mutation success and
prompt closure of every affected WebSocket connection.

## OTP and capability denial

- Incorrect viewer and control OTP submissions durably increment the configured
  attempt budget; exhausting it denies a later correct code. Concurrent correct
  submissions issue at most one credential/session.
- A disabled KV capability denies get, set, delete, and prefix list before the
  repository is called, preventing storage existence or prefix-oracle leakage.

## Continuous gates

- Unit: policy tables, parsers, state transitions, normalizers.
- Contract: route registry, JSON errors, manifest, migrations, audit schema.
- Integration: real HTTP server + temporary SQLite + filesystem.
- Browser: cookies, origins, redirects, CSRF, CSP, socket upgrade/reconnect.
- Realtime: default 32 KiB frame/payload denial, 20 publishes/second per
  connection, bounded-queue slow-consumer closure, subscription limits,
  revocation closure, cross-app isolation, and loss-tolerant recovery.
- Fuzz: host/path/archive/manifest/policy parsers.
- Release: socket inventory, anonymous probes, signed update and rollback drill.
