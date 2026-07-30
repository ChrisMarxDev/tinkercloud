# ADR 0008: First-class SDK and server-side capability boundary

**Status:** Accepted direction; external provider adapters remain post-V1

## Context

Tinkercloud apps should be extremely easy for staff and coding agents to create.
They need one natural interface to identity, data, and future LLM/internal
services. Operators may hold credentials deployers must be allowed to use but
must not be able to read.

A static browser app cannot safely receive a secret. Anything delivered to its
JavaScript can be inspected or exfiltrated by the viewer.

## Decision

Make `@tinkercloud/sdk` a first-class V1 product surface for current-user, KV,
ephemeral realtime, and capability discovery. It calls same-origin Tinkercloud
endpoints and contains no app IDs, database credentials, provider tokens, or
long-lived secrets.

Future external integrations use server-side capability adapters:

```text
SDK call
→ app/viewer authorization
→ effective capability grant
→ quota and operation policy
→ server-held operator connection
→ provider
```

Do not provide generic raw secret delivery or an unrestricted HTTP proxy.

## Consequences

- App creation is low ceremony and consistent across capabilities.
- Tinkercloud becomes the policy, quota, audit, and credential boundary for future
  external services.
- SDK compatibility, documentation, and coding-agent skills become release
  gates.
- Every adapter must disclose external data flow and define narrow operations.
- Operator-written adapter extensibility requires a later execution/trust ADR.
