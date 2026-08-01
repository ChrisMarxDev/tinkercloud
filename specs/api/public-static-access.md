# Public static access contract

## Outcome and trust boundary

An operator may permit deployers to make reviewed, capability-free immutable
static apps anonymously readable through the existing gateway. No second
listener, proxy, file server, runtime, or anonymous capability API is created.

Public access is effective only when all are current and valid:

- the canonical host resolves one active app and active immutable release;
- the app's current policy mode is exactly `public`;
- the durable revisioned operator public-app gate is enabled;
- the active release declares no browser capability; and
- the requested route is a static file or validated SPA fallback outside
  `/_tinker/*`.

Missing, corrupt, unknown, stale, or unavailable state denies.

## Typed authorization

The gateway is the only producer of a sealed static-access context:

```text
StaticAccessContext = PrivateViewer(AuthorizationContext)
                    | PublicStatic(AppID, DeploymentID, PolicyRevision,
                                   PublicGateRevision)
```

The static runtime accepts this sum type and server-derived immutable release
evidence. Capability, identity, auth, data, blob, realtime, LLM, and other
reserved dispatchers accept only the existing private viewer-bearing context.
No boolean or request-provided app identifier can construct authority.

## Policy, gate, and transition behavior

- The operator gate defaults disabled and is changed only by operator authority
  through a revision-checked, audited mutation.
- Enabling the gate changes no app policy. Disabling it removes anonymous
  access on the next request.
- A deployer may set public mode only for an owned app. Owner/email/domain rules
  remain stored and continue to govern private login access.
- Interactive private-to-public deployment requires an exact access-broadening
  confirmation. JSON deployment requires an explicit public acknowledgement.
- Public mode with KV, collections, blobs, realtime, LLM, or any future browser
  capability enabled cannot become verified or active.
- Public-to-private and gate-disable transitions use no grace cache and deny
  the next anonymous request.

## Indexing and cache behavior

`access.indexing` is immutable active-release metadata, defaults false, and is
valid only with `access.mode: public`. Unless both public access and indexing
are effective, successful document responses include
`X-Robots-Tag: noindex, nofollow`. Public responses must not be cached across a
policy/gate revision in a way that extends anonymous access.

## Posture-aware deployment proof

Private candidates retain the exact anonymous `401 not_authorized` zero-byte
release proof. Public candidates must instead prove, through the exact HTTPS
app origin with redirects disabled:

1. the expected candidate root document bytes/hash are returned anonymously;
2. a representative immutable asset is returned when present;
3. every reserved identity, SDK, data, blob, realtime, and LLM route is denied
   without invoking its dispatcher; and
4. the response's indexing directive matches the immutable manifest.

Activation failure preserves the previous release and policy.

## Denials

Unknown/private/suspended/deleting/failed apps, gate-off/unavailable state,
capability-bearing public candidates, reserved routes, wrong hosts, malformed
policies, stale revisions, and release-evidence mismatch expose no app bytes.
The owner of App A cannot reuse App A's public decision for App B.
