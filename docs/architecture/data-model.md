# Logical Data Model

This model captures invariants and relationships. Column types, indexes, and
migration syntax belong in implementation.

## Identity and control plane

```text
users
  id, normalized_email, role(operator|deployer), status, created_at

api_tokens
  id, user_id, app_id?, secret_hash, scopes, expires_at, revoked_at, last_used_at

identities
  id, normalized_email, created_at, last_authenticated_at
```

`users` represent platform authority. `identities` represent authenticated
browser email identities. The same normalized email may correspond to both
records. The dashboard performs a current role lookup; each app performs a
current policy lookup. Neither authorization follows merely from matching
email text.

## Apps and policies

```text
applications
  id, owner_user_id, slug, status, current_deployment_id?, policy_revision,
  created_at, updated_at

access_policies
  app_id, mode(private|public), revision, created_by, created_at

access_rules
  id, app_id, policy_revision, kind(owner|email|domain), normalized_value,
  created_by, created_at

host_settings
  key(public_apps_enabled|analytics_enabled), revision, boolean_value,
  updated_by, updated_at
```

Policy updates create or transactionally replace a revision. `applications`
points to the current revision. There is never an “absence means public”
interpretation.

Uniqueness:

- app slug is unique under the configured root domain and cannot equal a
  reserved platform label;
- normalized email/domain rules are unique within one policy revision;
- an active app must reference an activatable policy revision.

Host settings are durable current operator decisions, not activation snapshots.
Missing, corrupt, or unavailable public-gate state is disabled. Enabling the
host gate changes no application policy; disabling it removes effective
anonymous access on the next request.

## Authentication

```text
otp_challenges
  id, app_id?, purpose, control_channel?(cli), normalized_email,
  code_hash, expires_at, attempts, consumed_at, invalidated_at,
  request_fingerprint_hash, created_at

sessions
  id, scope(app), app_id, identity_id, identity_session_id,
  secret_hash, expires_at, revoked_at, last_seen_at, created_at

platform_identity_challenges
  id, normalized_email, code_hash, browser_binding_hash,
  expires_at, attempts, consumed_at, invalidated_at, created_at

identity_sessions
  id, identity_id, family_id, secret_hash, browser_binding_hash,
  previous_secret_hash?, previous_valid_until?, expires_at, last_seen_at,
  rotated_at, revoked_at, created_at

identity_handoffs
  id, app_id, identity_session_id?, state_hash, browser_binding_hash?, return_path, expires_at,
  authorized_at?, consumed_at, created_at
```

An app session requires `app_id`, `identity_id`, and its global identity-family
parent. A CLI bearer exists only in `api_tokens` and cannot be presented as a
browser credential. A platform identity challenge has no app or role and can
issue only a global browser identity. An `identity_session` proves only an
email on the admin host and belongs to one family; the dashboard still resolves
the current `users` role. An app session records that family as its parent so global
switch, revocation, or rotation-replay detection revokes every derived local
session. A handoff is state-bound to one server-derived app, stores only secret
hashes, expires in five minutes or less, and is atomically single-use. The
platform browser-binding hash is attached when OTP is requested and copied to
the issued global identity session; it is grouping/anti-race state, never an
identity credential or authorization input. SQLite permits at most one
unrevoked identity-session family for a binding hash, while expiration cleanup
revokes its children and pending grants before a replacement can be created.
The binding lookup has one narrow additional use: it may revoke that bound
family and its descendants during browser cleanup; it never returns an identity
or grants access.

Migration 5 adds the child linkage without rewriting any existing app session.
At app-session validation, a parentless session is quarantined when its RFC3339
creation time predates migration 5's persisted `applied_at` cutoff. Every newly
issued app session is handoff-linked; a parentless post-cutoff session denies.
Missing or malformed cutoff/session timestamps also deny without changing
persisted revocation state.

## Deployments

```text
deployments
  id, app_id, created_by, idempotency_key, archive_hash, release_hash,
  state, failure_code?, manifest_json, created_at, verified_at?, activated_at?

deployment_files
  deployment_id, relative_path, size, content_hash, media_type
```

Version-2 immutable `manifest_json` carries bounded presentation tags plus
public indexing intent. It is the source for current catalog metadata through
`applications.current_deployment_id`; there is no mutable application tag or
description column.

## Local app insights

```text
app_analytics_daily
  app_id, day_utc, page_views, last_activity_at

app_analytics_visitors
  app_id, day_utc, visitor_digest, expires_at
```

`visitor_digest` is an app-scoped keyed digest of a random host-only cookie.
Neither the raw cookie nor identity/session, email, IP, URL/query/path,
referrer, user agent, content, geography, or device data is stored. Period
visitors are counted distinctly across the requested window, not summed from
daily unique counts. Reads exclude data older than 30 days; startup and
scheduled bounded cleanup physically remove it. Hard app deletion removes both
tables with other app-owned control state.

Release paths are derived internally from IDs and never accepted from requests.
The file manifest enables integrity checks, update recovery, and a later backup
feature without changing release identity.

## App capabilities

```text
app_blobs
  id, app_id, state, display_name, content_type, size_bytes, content_hash,
  created_by_identity_id, created_at, updated_at

app_quota_usage
  app_id, metric, used, limit, measured_at
```

Each app-local `apps/{immutable-app-id}/data.db` contains an `app_kv` table
whose primary key is `key`. Repository APIs derive its database from
`AuthorizationContext`; the control database deliberately has no app KV table
after the pre-release destructive transition.

The blob catalog is also app-scoped. Only `ready` rows are listable/readable;
`staging` and `deleting` rows are recovery state. Byte-store keys are derived
from `(app_id,id)` and never accepted from a request. SQLite listing avoids
depending on local-directory or future object-provider listing semantics.

Realtime subscriptions and events are intentionally absent from the durable
model. The V1 hub is in memory; a committed KV write emits a best-effort change
event only after the transaction succeeds.

LLM capability records are deliberately separate from V1 KV state:

```text
provider_connections
  id, adapter_kind, operator_id, encrypted_credential, status, key_version,
  created_at, rotated_at

llm_chat_profiles
  id, connection_id, model, technical_limits, is_default, status, revision

llm_host_policy
  monthly_token_limit?, revision

app_llm_policies
  app_id, status, quota_mode, monthly_token_limit?, revision, updated_by

llm_usage
  app_id, utc_month, used_tokens, reserved_tokens, in_flight
```

Connection reads never return `encrypted_credential`. Only the registered
server-side adapter may request a short-lived decrypted credential after the
gateway-derived app, current default, app policy, and optional quota pass.

## Audit and jobs

```text
audit_events
  id, occurred_at, actor_kind, actor_id?, app_id?, action, outcome,
  target_kind?, target_id?, request_id?, metadata_json

outbox
  id, kind, payload_encrypted_or_redacted, available_at, attempts, state

jobs
  id, kind, scope_id?, state, lease_until?, attempts, last_error_code?
```

Audit metadata has a schema per action and excludes secrets, OTPs, cookies,
authorization headers, and raw request bodies.

## Transaction boundaries

- OTP consume + global identity/session-family creation, or an app-local
  session creation where no handoff is involved.
- Global identity switch/revocation + child app-session revocation.
- Handoff grant consumption + current policy recheck + app-session creation.
- Policy revision creation + current revision swap + audit.
- Deployment activation + current deployment swap + audit.
- Token creation/revocation + audit.
- KV write + quota accounting; publish its best-effort live event after commit.

Filesystem operations cannot share a SQLite transaction. Deployment and update
flows therefore use durable intermediate states, content hashes, idempotent
recovery, and cleanup that treats database state as authoritative.
