# Event and Audit Catalog

Events provide a stable vocabulary for audit, diagnostics, and later
observability. They are not a promise of a public event bus.

## Envelope

```json
{
  "id": "evt_...",
  "type": "deployment.activated",
  "version": 1,
  "occurred_at": "2026-07-24T12:00:00Z",
  "request_id": "req_...",
  "actor": { "kind": "deployer", "id": "usr_..." },
  "app_id": "app_...",
  "target": { "kind": "deployment", "id": "dep_..." },
  "outcome": "success",
  "metadata": {}
}
```

## V1 event types

```text
operator.authenticated
deployer.authorized
deployer.suspended
token.created
token.revoked
app.created
app.suspended
app.resumed
app.deletion_requested
policy.revised
otp.requested
otp.verification_failed
viewer.authenticated
session.revoked
deployment.uploaded
deployment.rejected
deployment.verified
deployment.activated
deployment.activation_failed
deployment.rolled_back
kv.quota_exceeded
server.update_started
server.update_completed
server.update_failed
server.update_rolled_back
certificate.failed
security.probe_failed
```

## Data rules

- Store stable IDs and reason codes, not secrets or raw credentials.
- Viewer email appears only where operator/deployer authorization requires it;
  unrelated deployers cannot query it.
- OTP values, session secrets, API keys, cookies, archive contents, request
  bodies, and authorization headers are forbidden.
- Schema changes increment the event version.
