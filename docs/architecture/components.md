# Component Map

The code should be a modular monolith. Package boundaries below represent
ownership and contracts, not microservices.

## Composition roots

| Future component | Responsibility | Depends on |
|---|---|---|
| `cmd/tinkercloud` | server, setup, root recovery, signed update/rollback | application services |
| `cmd/tinker` | deployer CLI, credential-store adapter, machine-readable output | public control-plane client |

The same repository produces two distributions while keeping the “one server
binary” product promise. Deployers install only `tinker`; the clean VPS installs
only `tinkercloud`. Shared code is limited to contracts and the control-plane
client, not server internals.

## Public gateway

### `gateway`

Owns:

- listener and server limits;
- trusted-proxy policy;
- canonical host parsing;
- platform/app/unknown host classification;
- security headers and generic failures;
- composition of app-plane middleware.

Must not own:

- access-rule semantics;
- file lookup;
- OTP generation;
- release activation.

Pseudo-interface:

```go
type HostResolver interface {
    ResolveApp(ctx context.Context, host CanonicalHost) (ResolvedApp, error)
}

type AuthorizedDispatcher interface {
    Dispatch(ctx AuthorizationContext, w ResponseWriter, r *Request)
}
```

`AuthorizationContext` is constructible only by the authorization pipeline. It
contains validated app, identity, session, policy revision, request time, and a
correlation ID.

### `appauth`

Owns:

- app-scoped session lookup and validation;
- current access-policy evaluation;
- login redirect decisions;
- production of authorization contexts.

Policy evaluation is pure where possible:

```go
func Evaluate(policy Policy, subject Subject, at time.Time) Decision {
    if policy.Invalid() || policy.Disabled() {
        return Deny("invalid_policy")
    }
    if policy.Public && policy.PublicAllowedByOperator {
        return Allow("public")
    }
    if subject.Anonymous {
        return Challenge("authentication_required")
    }
    if policy.OwnerID == subject.DeployerID {
        return Allow("owner")
    }
    if policy.Emails.Contains(subject.Email) {
        return Allow("exact_email")
    }
    if policy.Domains.Contains(subject.EmailDomain) {
        return Allow("email_domain")
    }
    return Deny("no_matching_rule")
}
```

The router handles `Challenge` only for browser navigations and otherwise emits
an appropriate unauthorized response. It never treats errors as `Allow`.

### `staticruntime`

Owns:

- path normalization after authorization;
- opening files relative to an immutable release directory;
- no-follow and root-containment enforcement;
- MIME, range, ETag, cache, and SPA fallback behavior.

It accepts `AuthorizationContext` and `ReleaseHandle`; it has no API that accepts
only an app ID plus path.

Pseudo-flow:

```go
func Serve(ctx AuthorizationContext, rawPath string) Response {
    clean := NormalizeURLPath(rawPath)
    candidate := OpenBeneath(ctx.ReleaseRoot, clean, NoSymlinks)
    if candidate.NotFound && ctx.App.SPAFallbackEnabled {
        candidate = OpenBeneath(ctx.ReleaseRoot, "index.html", NoSymlinks)
    }
    return StreamWithMetadata(candidate)
}
```

## Authentication

### `identity`

Owns canonical email normalization and identity records. V1 normalization should
be conservative: trim, case-fold the domain, validate syntax, and avoid
provider-specific dot/plus rewriting.

### `otp`

Owns challenge generation, keyed hashing, expiry, attempt consumption, resend
cooldown, and verification transactions.

```go
RequestOTP(app, rawEmail, requestSignals):
    email = identity.Normalize(rawEmail)
    rateLimiter.Check(ip, emailHash, app)
    eligible = policy.MightAllow(app, email)       // internal only
    challenge = challenges.CreateHashed(app, email)
    if eligible:
        outbox.EnqueueOTP(challenge)
    audit.RecordGenericOTPRequest(...)
    return GenericAccepted

VerifyOTP(app, rawEmail, code):
    in one transaction:
        challenge = challenges.LockActive(...)
        challenge.CheckExpiryAndAttempts()
        challenge.Verify(code)
        policy = policies.LoadCurrent(app)
        require policy.Allows(email)
        challenge.Consume()
        session = sessions.CreateAppScoped(app, identity)
    return session
```

An ineligible email should have sufficiently similar observable behavior without
creating an email amplification vector.

### `sessions`

Owns opaque random session secrets, hash-at-rest, app scope, expiry, rotation,
revocation, and cookies. It does not decide app access.

### `email`

Defines a provider-neutral outbound message interface plus direct Resend and
Postmark adapters. API
keys remain in the configuration secret source. Delivery retries are bounded;
OTP expiry is never extended because of retries.

## Deployment

### `uploads`

Streams authenticated uploads into a quota-limited staging area while computing
a content hash. It never extracts into a release directory.

### `archive`

Owns untrusted archive inspection and extraction:

- allowed formats and path encoding;
- total compressed/uncompressed byte budgets;
- entry and depth limits;
- rejection of links, devices, sockets, ownership, and unsafe permissions;
- duplicate/case-collision policy;
- cancellation and cleanup.

### `releases`

Owns the deployment state machine, immutable release metadata, activation lease,
and failed-activation preservation. It coordinates database and filesystem
operations through an explicit recovery protocol.

```go
Activate(deploymentID):
    deployment = LockDeployment(deploymentID)
    require deployment.State == Verified
    require CurrentPolicy(deployment.AppID).IsActivatable()
    require ReleaseDirectoryMatchesManifest(deployment)
    previous = CurrentDeployment(deployment.AppID)
    transaction:
        SetCurrentDeployment(deployment.AppID, deployment.ID)
        MarkActive(deployment)
        Supersede(previous)
        AppendAuditEvent(...)
    return Active
```

### `verification`

The server runs candidate policy/static-denial evidence before its atomic
activation. The deployer CLI then independently probes the exact real public
HTTPS gateway without credentials. Transient first-host DNS, TLS, transport,
and readiness outcomes receive a bounded retry; unsafe or contradictory
responses fail immediately. Because this independent proof follows the
committed activation, an exhausted probe returns a non-success
`active_but_unverified` receipt instead of claiming that the server restored
the previous release.

### `certificates`

Owns ACME account/certificate lifecycle, per-host readiness, renewal, and
operator diagnostics. A deployment URL is not “ready” until TLS works.

## App capabilities

### `sdk`

A separately published browser-first TypeScript package. It provides typed
modules, transport/error handling, capability discovery, cancellation, and
ergonomic examples over same-origin HTTP. It never accepts secrets or a
caller-selected app ID.

### `currentuser`

Returns only app-appropriate identity fields from `AuthorizationContext`.

### `kv`

Owns app-scoped string keys, JSON values, size limits, versions, bounded prefix
listing, conditional writes, transactions, and quota accounting. The app ID is
always taken from the authorization context.

```go
Set(ctx AuthorizationContext, key Key, value JSONValue, expectedVersion?):
    require ctx.App.Features.KV
    validate key, value, expectedVersion, quota
    -- executed against ctx's server-derived app-local SQLite database
    UPDATE app_kv
       SET value=?, version=version+1
     WHERE key=?
       AND version matches expectedVersion
```

### `blobs`

Owns the V1 app-shared upload, download, bounded list, metadata, delete, quota,
and reconciliation workflow. It receives only the authorization context plus
opaque blob/display inputs. SQLite is the canonical catalog; a narrow internal
store holds bytes by server-derived app/blob IDs. Only `ready` metadata can
open bytes, and every download stays behind the gateway as an attachment.

The V1 adapter is the private local data directory. The domain does not depend
on a mounted filesystem, provider listing, bucket URL, signed URL, or storage
credential, leaving a later direct S3-compatible adapter possible without
changing the SDK.

### `live`

Owns the single-node in-memory WebSocket hub. It authenticates before upgrade,
binds each connection to the server-derived app, identity, and session, and
supports bounded custom channels plus KV change notifications. Policy, session,
or app revocation closes affected connections. It promises no persistence,
history, replay, or cross-node delivery.

### `capabilities`

Returns the app's server-derived enabled capability descriptors and versions.
External adapters appear only after current operator policy and server safety
bounds are applied.

### `connections` (future)

Owns operator-managed provider connections and encrypted credentials. A
connection can be invoked only through a narrow registered adapter with a
gateway-derived app, current policy, operation scope, quotas, destination
restrictions, redaction, and audit.
It has no read-secret API.

## Control plane and operations

### `control`

Application services for operator/deployer commands. Each command declares:
actor, permission, target resource, transaction boundary, audit event, and
idempotency behavior.

### `tokens`

Owns scoped CLI/agent credentials, secret display-once behavior, hash-at-rest,
expiry, revocation, and last-used metadata.

### `audit`

Owns a stable event vocabulary, redaction, append semantics, retention/export,
and correlation IDs. Security-sensitive mutations fail if their audit event
cannot be persisted.

### `config`

Loads typed non-secret configuration and secret references, validates domains
and paths, and produces a redacted diagnostic view.

### `update`

Verifies signed releases, checks compatibility, creates a bounded local
pre-update rollback snapshot, replaces the server binary, restarts, verifies
health, and restores the prior binary/state when the update gate fails. This is
not a general backup or disaster-recovery interface.

### `jobs`

Runs bounded cleanup, renewal, checkpoint, and update work with leases and
observable outcomes. Jobs may never silently delete the active or only
recoverable release.

## Dependency direction

```text
entry points / adapters
        ↓
application services
        ↓
domain policies and state machines
        ↓
repository / filesystem / provider interfaces
```

Domain policy code must not import HTTP, SQLite, provider SDK, or CLI packages.
