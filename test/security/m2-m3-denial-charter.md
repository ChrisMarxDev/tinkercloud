# M2–M3 Denial Test Charter

Before happy paths, prove that control services deny cross-deployer app access,
revoked tokens, expired tokens, missing permissions, audit-store failure, and
idempotency reuse with a different request. Prove activation rejects missing
policy, TLS, probe, and non-verified release state, preserving the old active
release.

Control bearer authentication records only a nullable UTC `last_used_at` value
after an authorization succeeds. A wrong app, missing or partial scope,
expired, revoked, suspended, malformed, or unknown token must leave that value
unchanged. The authentication write atomically rechecks the active user,
revocation, expiry, app binding, and exact scope; a persistence failure denies
the request and rolls the metadata write back. A concurrent revoke and token
authentication may produce one successful request before revocation commits,
but no request may succeed after it, and no failed request can advance
`last_used_at`. Token list and dashboard views may expose the nullable timestamp
but never a raw token, token hash, IP address, or user-agent value; cross-owner
token list requests disclose neither metadata nor existence.

For deployment transport, prove a valid archive above the JSON control-body
limit is accepted up to the configured archive upload limit; reject a declared
or chunked limit-plus-one body before it can become a release. Verify sequential
CLI deploy invocations for one app carry distinct random idempotency keys, while
retry attempts for an individual invocation retain their key. Verify local
archive staging is mode 0600 and is removed on success, failure, and
cancellation.

For archives, exercise traversal, absolute/drive paths, symlinks, hardlinks,
devices, duplicate/case-folded paths, corrupt headers, unsafe permissions,
entry/depth/expanded-size/single-file limits, and cleanup after an extraction
failure. Test ZIP and tar.gz inputs separately.

For manifests, reject unknown keys, public mode, invalid/duplicate email or
domain entries, unsafe SPA fallbacks, malformed indentation/scalars, and
capabilities unavailable under the effective server policy.

For deploy orchestration, prove a newly authorized deployer can create and
deploy their manifest-named app with one `tiny deploy` invocation. A conflicting
slug owned by another deployer must remain a denial: a client-side create
conflict is never treated as evidence that the caller owns that app.

For public deployment evidence, prove the deployer makes a separate anonymous
GET to the activated `https://{slug}.{server-derived-domain}/` URL before
printing success. The
GET must retain real TLS transport behavior but send neither deployer bearer
credentials nor cookies. Reject 404, 2xx app bytes, redirects, a wrong host or
scheme, timeout/transport failures, an oversized/malformed body, or any denial
other than the gateway's bounded `401` JSON `not_authorized` envelope with a
valid request ID matching its header plus no-store and nosniff headers.

For candidate-bound access, prove that a verified deployment's canonical
private manifest allowlist—not an older current policy—is the policy activation
validates and installs. A broad active policy followed by an owner-only or
narrow candidate must atomically move both the deployment pointer and policy
revision. Injected policy, audit, or commit failures must preserve both old
values. Rolling back an immutable release must atomically reinstall that
release's canonical manifest policy.

For activation replay, prove that only the exact same deployment and
idempotency key after a durable successful activation returns success. That
replay must consult durable audit evidence before candidate planning and must
not rerun policy, certificate, probe, or capability gates; add a policy
revision/audit row; or emit a live-session side effect. A different key,
deployment, app, or non-active durable state fails closed. Activation gate
failures expose only one stable category—`activation_policy_not_ready`,
`activation_certificate_not_ready`, `activation_candidate_probe_failed`,
`activation_capability_not_ready`, or `activation_commit_failed`—and, when
the gateway supplied it, the matching `X-Request-ID` in the error envelope.

For dashboard policy visibility, prove the bounded server-derived read model
shows only the current private revision's canonical email/domain rules to an
authorized owner. Missing, non-private, malformed, cross-owner, or stale policy
state must fail unavailable rather than be represented as an empty allowlist;
an empty valid current allowlist is explicitly owner-only. Form posts retain
same-origin, CSRF, current actor/ownership, typed policy validation, audit, and
immediate live-session revocation checks.
