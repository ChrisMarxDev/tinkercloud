# Native web UI system contract

**Status:** V1 contract

**Applies to:** platform login, app login, operator/deployer dashboard, token
reveal, diagnostics, and every other Tinkercloud-owned HTML surface.

## Outcome

Tinkercloud-owned pages share one small visual and interaction language extracted
from the Tinkercloud landing page. The system is embedded in the `tinkercloud`
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
- The Tinkercloud mark identifies a Tinkercloud-owned surface. App content never
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
- The stylesheet exposes stable `--tinker-*` custom properties and `tinker-*`
  component classes. Application/domain packages do not embed color literals.

## Motion contract

- Every component with a visible state change defines a smooth transition for
  both entering and leaving that state. State changes must not jump merely
  because their final size is content-dependent.
- Motion uses shared `--tinker-motion-*` duration and `--tinker-ease-*` easing
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

Only `verified`, `active`, and `superseded` releases are immutable metadata
authorities: their stored manifests must be valid before the dashboard may use
their descriptions. Known non-authoritative `uploading`, `uploaded`,
`validating`, `staged`, `rejected`, and `failed` records render with a blank
description without parsing candidate manifest bytes. An unknown release state,
or an invalid manifest for an immutable state, makes the dashboard read model
unavailable rather than guessing at metadata.

A suspended app remains a visible durable dashboard row. Its current pointer
may still reference an active immutable deployment, from which the dashboard
may derive description, access metadata, and bounded release history. It has
no stable launch URL or launch/QR affordance while suspended. Suspension does
not restore gateway/static/capability authority, app sessions, app-scoped
tokens, or live connections; a later resume does not resurrect those revoked
app-scoped credentials. A missing,
malformed, foreign, or non-active current deployment pointer remains a
fail-closed dashboard read-model error.

## Post-V1 reach and insights surfaces

- The viewer catalog is a distinct global-identity surface, not a weakened
  operator/deployer dashboard. The server renders only current authorized
  private apps plus effective public apps; client filtering never receives a
  denied record.
- Catalog cards reuse the native app-card grammar and show escaped slug, active
  immutable description, bounded tags, stable URL, and explicit `Private` or
  `Public` text. A public badge never implies SDK/capability availability.
- Catalog search and tag controls are hidden until local initialization, keep
  every authorized card visible without JavaScript, use labelled native
  controls, announce a polite visible result count, and perform no fetch or
  persistence. Search matches only the rendered slug, active immutable
  description, and rendered tags case-insensitively; the tag select matches a
  rendered canonical tag exactly.
- Owner/operator app summaries label the metric **Approximate visitors** and
  show 7-day and 30-day page-view/visitor totals, last activity, and a textual
  zero-filled 30-day daily series. Unavailable analytics renders as unavailable,
  never as zero. Viewers and unrelated
  deployers receive no analytics markup or serialized values.
- Dashboard app lists use full-width compact rows, not a two-column card grid.
  Stable launch and QR actions sit beside the app title rather than occupying a
  separate action column. Owner/operator rows keep local insights secondary:
  compact 7-day and 30-day totals and last activity use one half of the desktop
  row, while a 30-day two-series chart with a tiny visible legend uses the other
  half. The halves stack at narrow widths.
  Quantized page-view and approximate-visitor bars use bounded `data-*` levels
  and local CSS only; no inline style, external chart runtime, request,
  storage, or inferred value is permitted. Each day exposes its exact page
  views and **Approximate visitors** in a hover-and-keyboard-focus detail,
  while a semantic textual daily table remains available to assistive
  technology. An unavailable result remains an explicit unavailable state and
  never becomes a zero chart or zero totals.
- Operator-only app management stays out of the resting row. One compact More
  disclosure lists the existing Access policy, Releases, optional LLM chat
  grant, and App controls surfaces; each option opens a focused native dialog
  containing the existing server-rendered read model or form. The enhancement
  changes presentation only: without JavaScript the same content remains
  available through native fallback `details`, and every form retains its
  server-owned target, CSRF value, revision, confirmation, and authorization.
- Public-policy and operator-gate states use visible exact text. Enabling public
  authority states “Anyone on the internet can open this app” and requires the
  normal exact server-side broadening confirmation; color/icon styling is only
  supporting information.
- Public indexing is shown as a separate immutable state and defaults to
  `Not indexed`. No UI may imply that indexing changes access authority.
- Deployer dashboard remains owned-app-only and list-oriented. Public policy
  mutation stays in the scoped CLI; operator gate mutation stays root-local in
  the first pilot. The dashboard may display their current effective states.
- The operator-only dashboard may provide a ready-to-copy coding-agent prompt.
  Its repository URL is fixed and its HTTPS admin endpoint comes only from
  validated server configuration; no query, form, or browser-controlled value
  selects it. A malformed host omits the prompt. The prompt contains no
  credential, token, one-time-code value, or secret; it names the deployer
  skill, initially asks only for the deployer email (with the bounded OTP
  exception below), and directs the normal Tinker CLI
  and deployer-skill flow. It first checks for and reuses the exact
  server-scoped saved identity for that deployer. Only when that identity is
  missing, unauthorized, or expired may it request at most one CLI OTP. The
  agent uses only the bounded supervised same-CLI OTP handoff. On CLI OTP
  failure it stops and reports the failure; it never retries
  the OTP, switches deployer identity, clears, logs out, or deletes saved CLI
  authentication, or creates another OTP path. It asks only the minimal
  remaining generate/build/deploy questions and never asks for bearer tokens,
  tokens, secrets, or operator access.
  The prompt stays selectable without JavaScript; a labelled native copy
  button and polite feedback are optional
  clipboard-only enhancement.
- Dashboard token management is deliberately absent until a dedicated
  ownership-and-scope overhaul is accepted. Operator and deployer dashboards
  render no token count, inventory, scope/lifetime field, create action, revoke
  action, or token-management form. The scoped control API, Tinker CLI, and
  successful display-once token response remain available; hiding the browser
  surface must not weaken their authorization, expiry, revocation, or audit
  checks.

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
    stored manifest for `verified`, `active`, or `superseded` makes the
    dashboard read model unavailable. Known `uploading`, `uploaded`,
    `validating`, `staged`, `rejected`, and `failed` candidate records are
    description-less without parsing their manifest bytes; unknown states fail
    closed.
15. Global viewer identity is presented only on Tinkercloud-owned platform/app
   authentication pages. It must never be rendered as a control role, sent to
   deployed app content, selected by a query parameter, or stored in browser
   JavaScript. Handoff progress and a denied-app outcome use the existing auth
   card and text notice primitives; they retain generic copy, provide a safe
   next step, and expose no policy membership, account existence, callback
   state, or app bytes.
16. “Use another email” is an ordinary admin-host POST verification flow.
   Its visible confirmation makes clear that it changes the browser identity
   and signs app sessions out; it is not an app-local sign-out,
   app-local GET mutation, or a client-side account selector.
17. App-host UI labels its existing POST action “Sign out of this app” (or an
   equivalently local phrase), never “Sign out everywhere.” Admin identity UI
   labels the separate POST action “Use another email” or “Sign out of
   Tinkercloud,” and explains its global consequence. An allowed handoff may
   show the verified email only as ordinary escaped text. An unauthorized app
   shows the generic denial notice; when a valid global identity is already
   established, it may show that verified email and “Use another email,” but
   never allowlist, app-policy, or callback details.
18. The admin-host browser-binding cookie is a non-authorizing HTTP-only
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
21. Operator controls split provider credentials into a top-level **API keys**
    section and place profiles, grants, limits, and aggregate usage in **LLM
    chat**. Provider credentials and their encrypted envelopes are write-only:
    no dashboard, form value, notice, audit row, error, source, or reveal path
    may render them. The fixed-provider API-key create form asks only for
    provider and one password field; it has no display-name, identifier,
    arbitrary secret, URL, or generic-key input. The server generates the
    identifier and derives `Anthropic API key` or `Gemini API key`; existing
    connection labels still render. Credential rotation never accepts a
    provider selector: the server resolves the existing connection's provider
    before validating the replacement. A server-derived unavailable key state
    gives only the root-only `tinkercloud llm enable` plus restart next step and
    renders no credential mutation form or encryption-root detail. A model
    catalog option renders only after the server decrypts an active connection's
    key and that fixed provider returns the model in a bounded valid list; failed
    connections contribute no option and no error detail. Catalog selection is
    a convenience, not authorization. Beside it, a native disclosure provides
    an explicit custom model-identifier form so a new provider model does not
    require a Tinkercloud upgrade; it still selects an active connection and
    cannot select a provider URL or provider options. Profile/grant writes carry
    the current revision; disabling a connection or
    disabling/revoking a grant requires a visible exact-target confirmation
    plus the normal operator, same-origin, and CSRF checks. Deployer dashboards
    omit both sections.
22. An app quick-navigation QR code is derived only from the same
    server-rendered stable gateway URL admitted for the adjacent launch link.
    It is generated locally with no remote image, request, analytics, release
    path, token, session, or browser-held authorization state. If the URL
    cannot be encoded, the dashboard omits the QR trigger instead of rendering
    a broken or substituted code. Scanning the code never bypasses the app's
    ordinary viewer authentication and current access policy.
23. Every authenticated dashboard, including the deployer-only owned-app
    overview, renders a labelled ordinary `POST /logout` form named `Sign out
    of Tinkercloud` in the persistent top bar. It carries the fresh
    server-rendered CSRF value and revokes the global browser identity family
    plus every derived app session before cookies are cleared or the browser is
    redirected to sign-in. Missing revocation capability, persistence failure,
    anonymous state, or a cross-origin/CSRF failure leaves cookies intact and
    renders a safe server error; global sign-out never revokes a CLI/agent
    bearer. App-host local logout is distinctly labelled `Sign out of this app`
    and revokes only that app session.
24. A non-operator deployer's dashboard is a compact, server-derived list of
    only that deployer's owned app cards. It may show stable launch links and
    current durable summary metadata, but it omits policy, token, release,
    suspension, deletion, provider, audit, health-management, and deployer
    management controls rather than rendering disabled or unauthorized forms.
    Those management operations remain available through the scoped Tinker CLI.
    A failed app read model renders a visible unavailable state and never
    substitutes an empty owned-app list.
25. The coding-agent prompt is operator-only dashboard guidance, never a
    deployment credential or control path. It uses a fixed repository URL and
    a composition-root-derived admin endpoint, rejects endpoint query overrides
    by having no endpoint input at all, and omits all tokens, cookies,
    one-time-code values, secrets, and browser identity material. It asks only
    for the deployer email, follows the normal Tinker CLI and deployer-skill
    flow, first reuses the exact server-scoped saved identity, and requests at
    most one CLI OTP only when that identity is missing, unauthorized, or
    expired. Human-supervised coding-agent OTP handoff: only when no reusable
    server-bound bearer exists and after endpoint, email, and manifest
    validation, when the same CLI process reaches its normal
    `Code: ` prompt, the agent may ask the human exactly once for the
    short-lived emailed OTP, accept it in the agent interaction, and
    immediately submit it only to that same CLI process. Use this only with a
    trusted human-supervised agent; its provider may retain the interaction. Do not restate it or
    copy it into files, source, argv, logs, summaries, or final output; never
    ask for a bearer. A failed, malformed, timed-out, or denied OTP is
    terminal: there is no second code, retry, forced login,
    identity/account/server switch, or alternate collection path. The CLI
    stores the resulting scoped bearer for later exact-server reuse. Fully
    unattended `tinkercloud-deployment-agent` and VPS Resend-reader paths must
    not fall back to an agent interaction OTP. “No OTP” means no one-time-code
    value, not that this conditional
    normal CLI flow may be omitted from the prompt. Its
    no-JavaScript state remains selectable text; clipboard success or failure
    is presentation-only.
26. A compact local-insights chart is display-only. It may render only the
    already authorized aggregate day series, carries no browser identity or
    analytics marker, and performs no fetch, persistence, mutation, or
    authorization. Every bar must reveal its exact daily values on both hover
    and keyboard focus; the same values remain in an assistive semantic table.
    The chart is absent for unavailable insights, which render explicit text
    instead of zero-valued bars or totals.
27. Dashboard rendering omits token management completely, even when its safe
    read model contains token records. It emits no token count, ID, scope,
    lifetime, last-use state, create/revoke control, or `/apps/{slug}/tokens`
    form. Direct scoped API and CLI token behavior remains independently
    authorized and tested; the omission is not a client-side hiding rule or an
    authorization substitute.
28. Compact app-row enhancement must not make management dependent on
    JavaScript. The More disclosure remains hidden until every listed dialog
    can be paired with its server-rendered fallback detail body; otherwise the
    fallback details remain visible and usable. Moving that existing body into
    a native dialog may change only presentation. It must not clone or rewrite
    a form, request data, persist state, select an app, change a target, or
    suppress server validation. Deployer rows render neither operator options
    nor empty operator dialogs.

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
- Quick-navigation QR dialogs include the destination as visible,
  overflow-safe text, explain that normal sign-in still applies, and keep at
  least the standard four-module quiet zone around the code.
- Expandable cards use native `details` and `summary`, so disclosure remains
  keyboard-operable without JavaScript.
- The compact app-row More control uses native disclosure semantics, a visible
  accessible name, ordinary focusable option buttons, and Escape/outside-click
  dismissal when enhanced. Every focused option dialog has a programmatic title
  naming the app and operation, a visible close action, and browser-native focus
  containment and return. Its fallback detail remains keyboard-operable when
  JavaScript is unavailable.
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
- Write-only credential fields use native password controls with an explicit
  statement that the value cannot be displayed or recovered. Every LLM limit
  has a visible label and numeric input mode; malformed or stale submissions
  produce a server-rendered safe error, not optimistic browser state.
- Coding-agent prompt copy uses a native button with a polite status message;
  when clipboard support or JavaScript is unavailable, the complete prompt
  remains focusable and selectable as ordinary text.

## Non-goals

- A client-side component framework.
- A general-purpose application UI kit for deployed apps.
- Theme switching or dark mode in V1.
- Animated mascots, remote icon libraries, or decorative illustration systems.
- Client-side route guards, permission checks, or mutation state as a security
  control.
