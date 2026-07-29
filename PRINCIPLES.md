# TinyHost Core Principles

These principles are decision gates. A feature or implementation that violates
one should be rejected or redesigned before code review.

## 1. The gateway is the security boundary

TinyHost is the only process that accepts public traffic. Application content
and capabilities—including WebSocket upgrades and live messages—are dispatched
only after the gateway has resolved an app, validated a session, and evaluated
the current policy.

## 2. Private is a state, not a convention

An app without a valid access policy cannot become active. V1 is private-only;
any future public mode must be an explicit policy choice with an operator gate.

## 3. Fail closed

Missing state, stale state, malformed input, dependency failure, and ambiguous
routing all deny access. An error path must never skip authorization.

## 4. Identity and tenancy are server-derived

App identity comes from the validated hostname. Viewer identity comes from an
opaque server-side session. Neither can be selected with browser-controlled
parameters or headers.

## 5. Untrusted content stays isolated

Every app has its own origin and storage namespace. Uploaded archives, static
files, app JavaScript, and future backend processes are all untrusted.

## 6. Policy changes take effect now

Revoking a rule, session, deployer, or app must affect the next relevant
request. Convenience caches must never extend authorization.

## 7. Releases are immutable and activation is atomic

Upload, validation, staging, policy creation, and verification occur before
activation. Failure preserves the last known-good release.

## 8. One operator should understand the server

The production shape is intentionally small: one self-contained `tinyhost`
server binary, one database, one data directory, and one service on a dedicated
VPS. The deployer-facing `tiny` binary is a separate, smaller distribution.
Operational simplicity is part of the security model.

## 9. Non-technical deployers get narrow power

Deployers manage their own apps without learning infrastructure. Tokens and UI
actions are least-privilege, app-scoped where practical, time-bounded where
useful, and auditable.

## 10. Security claims require executable evidence

Every new request surface must have a negative test proving anonymous,
cross-app, revoked, and malformed access is denied before its positive path is
considered complete.

## 11. The VPS is the recovery authority

Remote convenience must not create a second security authority. Operator
recovery, credential reset, and failed-update rollback may assume root access to
the dedicated VPS and should work even when email is unavailable.

## 12. The SDK is the app platform

App creators should use one small, typed client SDK for identity, KV, realtime,
and future capabilities. The SDK hides transport ceremony but not security
boundaries: it contains no app ID, database credential, provider token, or
long-lived secret. Capabilities remain server-derived, app-scoped, and
authorized on every request.

## 13. Secrets stay behind TinyHost

Static app code and browser users are untrusted. Operator-supplied credentials
must never be delivered to browser JavaScript. Future integrations use
server-side capability adapters that hold credentials, expose narrow operations,
enforce app grants and quotas, and audit use.

## 14. Keep it simple and embrace constraints

[Shopify Quick](https://shopify.engineering/quick) is TinyHost's usability north
star: deploy a folder, receive a secure URL, and reach a small fixed set of
backend capabilities through an agent-friendly client API. TinyHost should
prefer a few composable primitives over becoming a general-purpose PaaS. It
borrows Quick's low ceremony, not its trust model: TinyHost keeps per-app
allowlists, app ownership, self-hosting, and fail-closed authorization.

## 15. Realtime is ephemeral and app-scoped

V1 realtime is a single-node in-memory convenience capability, not a durable
message system. Every connection and channel belongs to one server-derived app.
Session, policy, or app revocation closes affected connections. Events may be
lost across disconnects or restarts; clients recover by reading current state.

## 16. Agent skills are self-contained

Create one generic Tiny platform skill first as the canonical agent workflow.
Specialized app-development, deployment, and operator skills copy the relevant
shared rules into themselves so they work independently. CI must detect drift
between the generic source and copied sections.

## 17. Ask only for necessary information

Every human flow starts from the outcome the person requested. TinyHost derives
safe values from current state, reuses already verified information, and
chooses secure defaults before asking a question. It asks only for a value or
decision that is both necessary to continue and impossible to discover or
safely default.

A generated config or manifest is a durable receipt and automation interface,
not prerequisite paperwork. Human commands may guide, validate, and persist the
minimum required state in context; deterministic and JSON automation remains
explicit and non-interactive. Security-sensitive secrets still use narrow
credential boundaries, but that boundary must not force unrelated non-secret
ceremony.

## Non-negotiable invariant

There must be one typed, testable authorization result between public request
ingress and every protected dispatcher. Bypassing it must be difficult in the
code structure and visible in review.
