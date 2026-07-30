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
  revocation. `GOCACHE=/private/tmp/tinker-live-go-cache go test -race
  ./internal/live ./internal/gateway` passed with loopback-listener permission
  on 2026-07-27.

## 2026-07-24

- `go test -race ./...` passes when local loopback listener permission is
  granted. The ordinary sandbox blocks `httptest` listener creation.
- M4 HTTP regression tests cover server-derived identity, two-app KV isolation,
  malformed JSON, strict content type, and optimistic-version conflicts.
- `internal/live/revocation_regression_test.go` proves revocation removes the
  connection before a buffered inbound frame can publish. The result is a
  direct hub close signal, not an idle-timeout side effect.

## 2026-07-27 — bounded KV, blob, and recovery evidence

- KV admission uses server-derived viewer-within-app, app, and global windows.
  Focused tests prove each dimension is isolated as intended, a denied global
  request consumes no dynamic scope, and the finite dynamic-scope map fails
  closed before repository access.
- Blob service, SQLite catalog, and private local store tests cover the
  `staging → ready → deleting` state transitions, exact app quota/concurrency,
  close/sync/commit failure cleanup, missing-byte denial, and bounded catalog/
  storage reconciliation. A ready row with missing, corrupt, or foreign bytes
  is unavailable rather than substituted.
- Gateway and built-SDK tests cover capability-disabled no-touch behavior,
  multipart extra-part denial before readiness, same-origin mutation checks,
  opaque app-scoped IDs, cursor/list bounds, cross-app denial, and typed blob
  errors. Successful blob downloads are attachment-only with `nosniff` and
  `private, no-store`; they expose no local path or storage URL.
- The M3 restart charter now has executable persistence coverage for
  database-led, stable per-record recovery: malformed historical records fail
  individually without blocking a healthy app, while a corrupt current record
  atomically clears its pointer and makes that app unavailable. No recovery
  path scans a release or staging directory for authority.
- The checked-in Attachment Shelf example compiles against the SDK and shows
  capability discovery, cancellation, upload/list/download/delete, and the
  local-VPS durability boundary.

## M0–M5 follow-up findings

- `internal/releases/manifest_regression_test.go` rejects duplicate nested
  manifest feature keys (`kv: false` followed by `kv: true`) before domain
  conversion, so deployment policy/capability inputs cannot silently use
  last-write-wins semantics.
- `internal/controlapi` passes idempotency keys through but has no bounded
  format validation or repository-level replay contract. This remains a proof
  gap rather than a demonstrated bypass.

## Outstanding proof gaps (not claims of passing coverage)

- Control-plane idempotency is declared but no idempotency repository contract
  or implementation is present.
- Updated-live-VPS acceptance of the current blob-enabled binary remains
  pending: the existing disposable-host evidence predates the release-key
  approval needed to install this exact build. This is not claimed by local,
  real-listener, or source-level tests.
