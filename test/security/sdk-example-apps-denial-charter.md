# SDK example applications deny-path charter

The public examples are documentation that users will copy. They must make the
TinyHost boundary easier to preserve, not teach browser-side substitutes for
authorization or durable delivery.

## Boundary and ownership

- App and viewer identity come only from `tiny.app.info()` and
  `tiny.user.current()` over the current same-origin session.
- KV keys and live channels remain in the gateway-derived app namespace.
- Browser code contains no app selector, bearer credential, provider secret,
  database credential, alternate API origin, or raw reserved-endpoint call.
- KV is utility-grade current state. Live messages are lossy refresh hints.

## Deny and failure cases

- If identity, app, or capability discovery is denied, malformed, cancelled, or
  unavailable, no capability mutation is attempted and the UI presents a safe
  retry path.
- If KV is not granted, the app does not probe get, set, delete, or list.
- If live is not granted or cannot connect, KV-backed examples retain current
  rendered state and expose manual refresh; they do not claim to be synced.
- A version conflict never overwrites newer state. The app reports the conflict
  and rereads current KV before another user action.
- Validation, quota, rate, compatibility, authorization, and temporary
  failures render stable user-facing guidance. Only the safe request ID may be
  shown for diagnosis.
- A missing or malformed KV value is treated as absent/unavailable state, never
  as authority to select another app or identity.
- A reconnect, tab visibility change, or received live event triggers a current
  KV reread. No example attempts event replay, history, ordering, or durable
  delivery.
- Local input is bounded before mutation. Remote text is inserted with DOM text
  APIs rather than HTML injection.
- Page teardown aborts outstanding HTTP work and closes owned live channels or
  KV subscriptions.
- Generated releases use a local copy of the built SDK and contain no remote
  executable asset.

## Executable evidence

`npm test` in `sdk/typescript` must:

1. type-check every example source file against the supported SDK;
2. build all three deployable release directories in a temporary location;
3. prove each release contains `index.html`, `app.js`, `errors.js`,
   `styles.css`, and `tiny-sdk.js`;
4. prove package imports were resolved to the local SDK file; and
5. reject remote URLs, explicit app-selection fields, and credential-like
   browser configuration in example-owned source and built artifacts.

The real-listener SDK contract remains the authorization, error-envelope,
cross-app, and transport evidence. Static example checks do not replace it.
