# System Architecture

## Context

```text
                     ┌────────────────────┐
                     │ Operator / Deployer│
                     │ browser and CLI    │
                     └─────────┬──────────┘
                               │ HTTPS
Viewer browser ─── HTTPS ──────┤
                               ▼
                  ┌─────────────────────────┐
Internet ────────▶│ Tinkercloud public gateway │
                  │ :80 / :443 only         │
                  └────────────┬────────────┘
                               │
            ┌──────────────────┼───────────────────┐
            ▼                  ▼                   ▼
      SQLite database    private data dir     selected email HTTPS API
      metadata/state     immutable releases   outbound OTP only
```

There are two logical planes in one process:

- The **control plane** handles setup, operator/deployer roles, apps,
  policies, uploads, releases, tokens, audit, and health.
- The **app plane** handles app-host resolution, viewer authentication,
  authorization, static content, and app-scoped platform APIs.

They share one verified browser identity but not authorization rules or
host-only cookies. CLI and deployment-agent bearers remain separate.

## Public listener rule

Only the gateway owns Tinkercloud's public sockets. Internal component APIs are Go
interfaces and function calls in V1, not local HTTP services. SSH, firewall
policy, and pre-existing listeners remain operator-owned. This reduces
alternate routes, distributed failure modes, and identity-header confusion.

## Request classification

The gateway classifies traffic using validated host configuration:

```text
host exactly matches admin.<configured-domain>
  → control-plane router

host is exactly one non-reserved label beneath configured domain
  → app-plane router

anything else
  → generic not found
```

Host parsing occurs on a canonical host without a port, trailing dot, Unicode
ambiguity, empty label, or suffix trick. Forwarded host headers are trusted only
when the request came from an explicitly configured trusted proxy.

## App-plane architecture

```text
Request
  → canonicalize host
  → resolve active app
  → classify reserved or content route
  → validate app-scoped session
  → load current policy
  → authorize
  → mint AuthorizationContext
  → dispatch to exactly one protected capability
       ├── static files
       ├── current user
       ├── JSON key-value store
       ├── lightweight app-scoped blobs
       ├── authenticated WebSocket hub
       ├── app handoff/session endpoints (special pre-auth routes)
       └── later: backend proxy
```

The exact admin host is the sole browser OTP broker. App handoff endpoints are
intentionally reachable before an app session exists, but they expose no app
content. They still require a valid resolved app, generic responses, rate
limits, current-policy checks, and strict redirect validation.

## Control-plane architecture

```text
CLI / dashboard request
  → exact admin-host router
  → global browser identity + current role, or CLI/agent bearer
  → role + resource authorization
  → command service
  → transaction + filesystem staging
  → audit event in same logical operation
```

The control plane may create app state, but it cannot write a release into the
active position directly. Activation is owned by the release manager.

## Storage layout

```text
data/
├── tinkercloud.db
├── releases/
│   └── <app-id>/
│       └── <deployment-id>/       # immutable after validation
├── staging/
│   └── <upload-id>/               # never served
├── blobs/
│   └── <app-id>/
│       └── <blob-id>              # server-derived, ready metadata required
├── blob-staging/
│   └── <blob-id>                  # bounded, private, never served
├── keys/
├── update-rollback/                # bounded, internal upgrade recovery only
└── tmp/                            # bounded, cleanup-safe work
```

The active deployment ID lives in SQLite. Static serving opens files from a
validated release root identified by that record. A symlink is therefore not
required for correctness; an optional pointer can remain an operational
optimization.

Blob listing, readiness, and quota state come from SQLite. The local byte store
is addressed only by server-derived app/blob IDs; filenames remain display
metadata. V1 does not mount remote storage or run a second object-store service.

## Concurrency model

- HTTP handlers are short-lived and context-cancellable.
- WebSocket connections are bounded, app-scoped, revocation-aware, and
  explicitly backpressured; slow consumers are disconnected.
- Database writes are bounded transactions with explicit busy timeouts.
- Upload validation, cleanup, certificate provisioning, and update jobs use
  separate bounded worker pools.
- Activation for one app is serialized by an app-scoped lease.
- Shutdown stops accepting traffic, drains handlers, checkpoints safely where
  appropriate, and never promotes staged work.

## Failure boundaries

| Failure | App request result | Control-plane result |
|---|---|---|
| SQLite read unavailable | deny / generic unavailable | fail operation |
| Policy missing or invalid | deny | refuse activation |
| Release missing | authenticated unavailable page | mark unhealthy |
| Email provider unavailable | existing valid sessions continue | new OTP fails generically |
| Disk critical | existing reads continue if safe | reject upload/app-data writes |
| Certificate not ready | app not reported ready | deployment remains non-ready |
| Audit persistence fails | read may continue | sensitive mutation fails |

## Extension boundary

Future backend processes attach behind a new protected dispatcher. They do not
change host resolution, authentication, policy evaluation, or public listeners.
That work requires a separate threat model and is not implied by the V1
architecture.
