# Native web UI system contract

**Status:** V1 contract

**Applies to:** platform login, app login, operator/deployer dashboard, token
reveal, diagnostics, and every other TinyHost-owned HTML surface.

## Outcome

TinyHost-owned pages share one small visual and interaction language extracted
from the Tiny Cloud landing page. The system is embedded in the `tinyhost`
binary, works with server-rendered HTML, and requires no remote asset, frontend
runtime, or build step.

## Product character

- Warm neutral canvas, white paper surfaces, deep grape text, and cobalt primary
  actions.
- Rounded, friendly shapes without mascot artwork or decorative clutter inside
  dense operational flows.
- Pink, yellow, mint, and lilac are small supporting accents. They never carry
  security meaning by themselves.
- Display copy may be playful; labels, errors, confirmations, identifiers, and
  operational state remain plain and exact.
- The Tiny cloud mark identifies a TinyHost-owned surface. App content never
  inherits or impersonates the native control-plane chrome.

## Technology contract

- One canonical local stylesheet and one local SVG mark live in `web/assets/`.
- Go embeds both assets and injects them only as trusted static template
  content. It derives a stable SHA-256 CSP source expression from each exact
  embedded byte sequence; the gateway uses those expressions for `style-src`
  and `script-src` without `unsafe-inline` or a new asset route.
- Native pages use semantic HTML and ordinary forms. The optional local
  interaction helper may enhance toast dismissal and native dialogs, but
  JavaScript must never be required for authentication, authorization,
  confirmation, or mutation.
- No remote fonts, images, styles, scripts, analytics, or CDN resources.
- The base font stack uses local system rounded/sans faces. Monospace content
  uses the local system monospace stack.
- The stylesheet exposes stable `--tiny-*` custom properties and `tiny-*`
  component classes. Application/domain packages do not embed color literals.

## Motion contract

- Every component with a visible state change defines a smooth transition for
  both entering and leaving that state. State changes must not jump merely
  because their final size is content-dependent.
- Motion uses shared `--tiny-motion-*` duration and `--tiny-ease-*` easing
  tokens. Small feedback is quick; spatial changes such as disclosure height
  use the slower disclosure token.
- Transitions are interruptible. Reversing an expandable while it is moving
  continues from its current rendered height rather than snapping to an
  endpoint.
- Motion reinforces a state that has already changed; it never delays durable
  server truth, authorization, revocation, form submission, or error copy.
- Native semantics and usable fallback behavior remain when JavaScript is
  missing. `prefers-reduced-motion: reduce` makes every transition effectively
  immediate and bypasses measured JavaScript animation.

## Required primitives

- Brand lockup and page shell.
- Auth card and one-time-code input.
- Buttons: primary, secondary, quiet, and danger.
- Field, help text, validation text, textarea, select, checkbox, and radio.
  Invalid controls use `aria-invalid="true"` and reference visible help/error
  copy with `aria-describedby`.
- Card, inset panel, section header, divider, and responsive grid.
- Status badge with a visible text label.
- Inline notice: information, success, warning, and danger.
- Empty, loading, stale/refreshing, unavailable, and error states. Every
  non-empty state has visible text; stale content identifies its last known
  safe time and is never presented as current server truth.
- Table with a narrow-screen overflow container.
- Definition list, code/token reveal, progress/stage list, and pagination.
  Progress and pagination remain server-rendered semantic HTML: the current
  stage and page are visible in text, and pagination links carry server-derived
  `href` values.
- Confirmation/danger zone that states the exact target and consequence.
- Toast region and dismissible toast: information, success, warning, and
  danger.
- Native dialog with labelled title, explicit close action, and action area.
- Expandable card built from native `details` and `summary`.
- Skip link, focus ring, visually hidden text, and reduced-motion behavior.

## State vocabulary

Use durable server states, not optimistic labels:

| Family | Labels |
|---|---|
| App | creating, active, suspended, deleting, failed |
| Deployment | uploading, validating, staged, verified, active, superseded, rejected, failed |
| Health | healthy, degraded, warning, write-disabled, unavailable, local |
| Live | connected, reconnecting, offline |
| Mutation | ready, submitting, succeeded, failed |

Unknown states render with neutral styling and their escaped server-provided
text. A dot, icon, or color may reinforce the state but never replace the text.

After a successful irreversible app deletion, the server permanently removes
the app and its owned data, so the redirected dashboard's app list, local
search/filter inputs and results, and visible app count exclude it. The browser
must receive that already-excluded server read model; hiding a rendered deleted
card with JavaScript is forbidden. The operational dashboard offers no restore
affordance. A denied or failed deletion leaves the existing operational app
visible and states that deletion did not complete, without claiming that
removal, revocation, or cleanup occurred.

The V1 dashboard may display immutable release metadata but offers no
deployer-selected rollback form, route, button, or simulated client-side
control. Failed activation preservation is server-side deployment behavior, not
a dashboard mutation.

## Security and denial charter

Before a styled happy path is accepted, tests must prove:

1. Anonymous platform users still redirect to login and receive no dashboard
   read model.
2. OTP request and verification pages retain generic responses that do not
   reveal allowlist membership, rate-limit buckets, or provider state.
3. Viewer, deployer, and operator credentials are never visually or
   mechanically interchangeable.
4. Raw tokens appear only in the successful create response and never in the
   dashboard or page source afterward.
5. Access-broadening and destructive forms retain server-side authorization,
   same-origin, CSRF, ownership, and exact-target confirmation checks.
6. Styling adds no public route, listener, remote request, inline script, unsafe
   URL, or browser-held authorization state.
7. Error and unavailable pages contain a safe next step and no secret, host
   path, SQL, stack, policy-membership, or provider detail.
8. Closing a toast or dialog changes presentation only. It never submits,
   authorizes, confirms, or mutates an operation.
9. A confirmation dialog is an enhancement around an ordinary server-rendered
   confirmation route or form. Missing or blocked JavaScript cannot bypass the
   exact-target, CSRF, ownership, or server-authorization checks.
10. Animation state contains presentation metadata only. It cannot postpone
    revocation, hide a denial, suppress an error, or become an input to a
    server command.
11. Embedded style and interaction bytes are admitted only by their exact
    SHA-256 CSP source expressions. Tests reject `unsafe-inline`, remote CSP
    sources, and a second asset route.
12. Loading, stale, unavailable, error, progress, pagination, validation, and
    busy states do not grant authority or hide a denial. A blocked script still
    submits ordinary forms and receives the server-rendered result.
13. A client-side dashboard list filter is presentation-only: it may narrow
    already server-authorized app cards in the current document, but it must
    not request data, change server queries, persist browser state, or take
    part in authorization. Its controls remain hidden until the local helper
    initializes; without JavaScript every authorized card stays visible. A
    zero-match state is visibly distinct from the server-rendered no-apps
    state.
14. Dashboard app filtering matches only server-rendered slug and current
    immutable deployment description text. The active-app launch icon appears
    only with a server-derived stable gateway URL, opens a new tab with
    `rel="noopener noreferrer"`, has an accessible name and title, and never
    exposes a release hash, release path, or raw immutable URL. A malformed
    final deployment manifest makes the dashboard read model unavailable;
   absent intermediate metadata is simply description-less.
15. Global viewer identity is presented only on TinyHost-owned platform/app
   authentication pages. It must never be rendered as a control role, sent to
   deployed app content, selected by a query parameter, or stored in browser
   JavaScript. Handoff progress and a denied-app outcome use the existing auth
   card and text notice primitives; they retain generic copy, provide a safe
   next step, and expose no policy membership, account existence, callback
   state, or app bytes.
16. “Use another email” is an ordinary platform-host POST verification flow.
   Its visible confirmation makes clear that it changes the browser’s viewer
   identity and signs app sessions out; it is not a dashboard control sign-out,
   app-local GET mutation, or a client-side account selector.
17. App-host UI labels its existing POST action “Sign out of this app” (or an
   equivalently local phrase), never “Sign out everywhere.” Platform identity
   UI labels the separate POST action “Use another email” or “Sign out of
   TinyHost apps,” and explains its global consequence. An allowed handoff may
   show the verified email only as ordinary escaped text. An unauthorized app
   shows the generic denial notice; when a valid global identity is already
   established, it may show that verified email and “Use another email,” but
   never allowlist, app-policy, or callback details.
18. The platform-host browser-binding cookie is a non-authorizing HTTP-only
   implementation detail for OTP race grouping. Authentication pages never
   render, serialize, label, or expose it in URLs, forms, templates,
   JavaScript, notices, or deployed-app content. It remains stable through
   global sign-out; that persistence must not be represented as a signed-in
   state or account picker.
19. A successful irreversible app deletion permanently removes the app and its
   owned data from the server. The redirected operational dashboard's
   server-derived list, local filter/search result set, and visible count omit
   it because there is no retained app record to render. The dashboard must not
   render a deleted card then hide it with JavaScript, expose a deleted-state
   filter value, or offer restoration. A rejected or failed delete keeps the
   app in the operational read model and gives a safe, explicit failure or
   denial message without asserting that deletion occurred.
20. The operator-only active-deployer allowlist is one labelled native textarea
   prefilled from the server-rendered canonical snapshot and revision. It has
   no JavaScript dependency, requires visible broadening confirmation, and
   states that removed addresses are signed out and cannot deploy. The form
   never renders credential/provider detail; server-side role, CSRF, origin,
   revision, collision, audit, and idempotency denials prevent mutation.

## Accessibility contract

- Text and interactive controls meet WCAG AA contrast against their intended
  background.
- Keyboard focus is always visible.
- Touch targets are at least 44 CSS pixels tall where practical.
- Every field has a programmatic label; help and error text can be associated
  with `aria-describedby`.
- Busy controls include visible busy copy, `aria-busy="true"` where applicable,
  and a disabled submitted control. Server-rendered validation and the final
  mutation outcome remain visible without JavaScript.
- Status updates use `role="status"`; urgent blocking failures use
  `role="alert"`.
- Toasts live in a labelled polite live region; urgent danger toasts use
  `role="alert"`, remain dismissible, and do not communicate through color
  alone.
- Toast containers use one neutral surface, one uniform border, and restrained
  elevation across every tone. Semantic color is confined to a compact leading
  status icon; colored side rails and tone-colored container outlines are
  forbidden.
- Dialogs use the native `dialog` element, have a programmatic title, expose a
  visible close action, close with Escape, and return focus through the
  browser's native modal behavior.
- Expandable cards use native `details` and `summary`, so disclosure remains
  keyboard-operable without JavaScript.
- Page landmarks, heading order, table headers, and native button/link
  semantics are preserved.
- Layout reflows without horizontal page scrolling at 320 CSS pixels. Wide
  tables and code blocks scroll inside their own bounded containers.
- Client-side list filtering uses labelled native search and select controls,
  is keyboard operable, announces its result count through a polite status
  region, searches case-insensitively by the visible stable slug, and matches
  the visible status exactly.
- Motion is smooth in both directions, preserves keyboard focus and native
  semantics, and is effectively disabled under `prefers-reduced-motion`.

## Non-goals

- A client-side component framework.
- A general-purpose application UI kit for deployed apps.
- Theme switching or dark mode in V1.
- Animated mascots, remote icon libraries, or decorative illustration systems.
- Client-side route guards, permission checks, or mutation state as a security
  control.
