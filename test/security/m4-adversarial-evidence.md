# M4 adversarial verification evidence

## 2026-07-27 — SDK compatibility and tenancy denial charter

- A built SDK is exercised through a real listener backed by the composed
  gateway, a host-derived app, and an authorized opaque app session. It proves
  `me`, `app`, capability discovery, and every KV operation without a mocked
  server or fabricated authorization context.
- SDK method inputs do not contain an app ID. Extra browser-side fields and
  query values cannot select a second app; hostname and session remain
  authoritative and the second app's KV data is not returned or mutated.
- A supplied unsupported or malformed SDK major returns a stable, typed,
  actionable incompatibility error after authorization. Missing headers remain
  compatible for raw HTTP/older clients; compatibility cannot bypass auth.
- Checked-in example apps compile against the supported SDK API and package
  output stays below the declared bundle gate. Realtime tests stay
  complementary: reconnects reread current KV and make no replay, ordering,
  history, or durability claim.

## 2026-07-27 — live liveness denial charter

- The authenticated WebSocket adapter must run bounded idle/ping/pong/write
  state. A peer that never reads or pongs must be closed by the configured
  liveness bound; a peer servicing normal pings remains connected.
- Revocation is a direct hub close signal, not an idle-timeout side effect.
  A revoked transport and any buffered inbound frame must be rejected before
  delivery.
- A full outbound queue or write timeout must fail closed for that socket only
  and must not block a healthy subscriber or leak a writer goroutine.
- Focused socket tests cover anonymous denial before upgrade, silent-peer
  timeout, normal pong liveness, slow-consumer isolation, and prompt
  revocation. `GOCACHE=/private/tmp/tiny-live-go-cache go test -race
  ./internal/live ./internal/gateway` passed with loopback-listener permission
  on 2026-07-27.

## 2026-07-24

- `go test -race ./...` passes when local loopback listener permission is
  granted. The ordinary sandbox blocks `httptest` listener creation.
- M4 HTTP regression tests cover server-derived identity, two-app KV isolation,
  malformed JSON, strict content type, and optimistic-version conflicts.
- `internal/live/revocation_regression_test.go` currently exposes a revocation
  bypass: `Connection.Publish` does not verify that the connection remains in
  the hub after `Hub.Revoke`. A buffered inbound frame can therefore be fanned
  out after revocation. This test is intentionally failing until production
  rejects operations from detached connections.

## M0–M5 follow-up findings

- `internal/releases/manifest_regression_test.go` exposes that duplicate nested
  manifest feature keys are accepted and last-write-wins (`kv: false` followed
  by `kv: true`). This is intentionally failing: deployment policy/capability
  inputs must be unambiguous.
- `internal/controlapi` passes idempotency keys through but has no bounded
  format validation or repository-level replay contract. This remains a proof
  gap rather than a demonstrated bypass.

## Outstanding proof gaps (not claims of passing coverage)

- Control-plane idempotency is declared but no idempotency repository contract
  or implementation is present.
- No executable tests yet cover current-policy revocation wiring into the live
  hub, CSP/CSRF/CORS, or production SQLite interruption recovery.
