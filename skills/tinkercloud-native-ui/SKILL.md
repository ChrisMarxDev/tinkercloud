---
name: tinkercloud-native-ui
description: Use and evolve Tinkercloud's dependency-free native web UI for Tinkercloud-owned login, dashboard, diagnostics, token, and operations screens. Use when Codex creates, edits, or reviews native UI components, CSS tokens, vanilla JavaScript interactions, Go templates, component states, motion, accessibility, showcase examples, or new design-system rules. Do not use for deployed app content or the marketing landing page.
---

# Tinkercloud Native UI

Build Tinkercloud-owned interfaces as one small server-rendered system. Preserve
the warm Tinkercloud character without weakening operational clarity, native
semantics, accessibility, or the gateway security boundary.

## Read the sources of truth

Read these files before planning or editing, in this order:

1. `PRINCIPLES.md`
2. `PRD.md`, especially UI requirements and §18.3
3. `specs/ui/native-web-system.md`
4. `docs/product/web-design-system.md`
5. `docs/decisions/0021-embedded-native-web-design-system.md`
6. `web/assets/tinkercloud.css`, `web/assets/tinkercloud.js`, `web/showcase.html`,
   `web/ui.go`, and `web/ui_test.go`

Treat the UI spec as normative behavior, the product document as rationale and
examples, and the assets as the canonical implementation. Update higher-order
sources before derived guidance when they conflict.

## Choose the smallest path

- To use an existing component, compose its `tinker-*` classes with semantic HTML
  in the relevant Go template. Do not create a near-duplicate.
- To add a visual variant, prefer an existing token or state attribute. Add a
  token only when it represents a reusable role.
- To create or materially change a component, follow the full component
  workflow below.
- To add a design rule, follow the rule-capture protocol in the same change.

This system applies only to Tinkercloud-owned surfaces. Never give deployed apps
the control-plane mark, stylesheet, or chrome.

## Preserve the architecture

- Keep Go `html/template`, embedded local CSS, and minimal local vanilla
  JavaScript.
- When a template embeds `tinkerCSS` or `tinkerJS`, keep the gateway CSP strict by
  using `webui.TinkerStyleCSPSource()` and `webui.TinkerScriptCSPSource()` for the
  exact compile-time bytes. Never solve the embedding problem with
  `unsafe-inline`, a remote source, or a new asset route.
- Add no React, Tailwind, UI runtime, remote font, CDN asset, analytics, second
  web server, or frontend build requirement.
- Start with semantic HTML and native controls. Use JavaScript only for
  progressive presentation behavior that HTML and CSS cannot express well.
- Keep authentication, authorization, confirmation, CSRF, ownership, and
  mutation on typed server paths. Browser state and animation are never
  security controls.
- Global browser identity/handoff pages use the existing auth card, native form,
  and text-bearing notice primitives. Do not create an account picker,
  browser-held identity state, or app-visible control. “Use another email” is
  an admin-host POST verification flow whose copy says it signs app sessions
  out. Keep it visibly distinct from app-host “Sign out of this app”; generic
  denied and handoff states reveal no policy membership, callback state, or app
  bytes.
- The platform's browser-profile binding is an HTTP-only non-authorizing OTP
  race-grouping detail. Never render, label, serialize, or expose it in a
  template, form, URL, JavaScript, notice, or app UI; retaining it through
  global sign-out never implies that a viewer remains signed in.
- For a local list filter, operate only on already server-rendered authorized
  items. Hide controls until initialization, leave every item visible without
  JavaScript, use labelled native controls plus a polite result count, and do
  not fetch, persist, or use filtering as authorization.
- For app-card phone navigation, derive the QR code only from the same
  server-rendered stable gateway URL as the launch link. Generate it locally,
  keep the destination visible in the native dialog, state that app sign-in
  still applies, and omit the action on encoding failure; never call a remote
  QR service or encode a release path, credential, session, or app-selected
  identity.
- A viewer team catalog is a separate global-identity surface, never a
  role-weakened dashboard. Render only the server-authorized private apps plus
  effective public apps. Local search/tag controls may inspect only those
  rendered cards, remain hidden until initialization, leave every card visible
  without JavaScript, announce a polite result count, and never fetch or store.
- Catalog cards show escaped active immutable description/tags, stable URL, and
  explicit Private/Public text. Public never implies capability access. Catalog
  search matches only rendered slug, active description, and tags
  case-insensitively; an optional tag select matches rendered canonical tags
  exactly.
- Owner/operator insights use existing stat/table primitives and the exact
  label “Approximate visitors.” Show page views, last activity, and textual
  daily values; unavailable is never rendered as zero. No viewer, app SDK,
  deployment-agent, or unrelated-deployer markup may contain analytics values.
- Dashboard app lists are full-width compact rows, never a two-column app-card
  grid. Keep owner/operator insights secondary: compact 7/30 totals and last
  activity plus a local 30-day quantized CSS bar chart with a tiny visible
  two-series page-view/Approximate-visitors legend. Bars reveal exact daily values on
  hover and keyboard focus, while an assistive semantic daily table preserves
  the raw series. No inline geometry, chart runtime, fetch, storage, marker,
  identity, or authorization behavior is allowed. Unavailable remains explicit
  and never renders as zero.
- Dashboard release descriptions come only from a valid stored manifest on a
  `verified`, `active`, or `superseded` immutable release. Known
  `uploading`, `uploaded`, `validating`, `staged`, `rejected`, and `failed`
  candidate records stay description-less without parsing their manifest
  bytes. An unknown state or malformed immutable manifest makes the dashboard
  unavailable; do not guess at metadata.
- A suspended app remains a visible metadata-only dashboard row when its
  current pointer is an active immutable deployment: it may retain description,
  access metadata, and bounded release history, but has no stable launch URL,
  launch/QR affordance, gateway/static/capability authority, or revived app
  session/app-scoped-token/live credential. A missing, malformed, foreign, or non-active
  current pointer fails the dashboard read model closed; resuming never
  resurrects revoked credentials.
- Public publishing copy must state that anyone on the internet can open the
  app, keep indexing as a separate default-off fact, and retain exact
  server-side broadening confirmation. In the first pilot, UI displays the
  operator public gate but root-local CLI owns the mutation.
- After a successful irreversible app deletion, the redirected operational
  dashboard's server read model excludes the app from its list, local
  search/filter inputs and results, and visible count because confirmed
  deletion permanently removes the app and its owned data. The dashboard
  renders no deleted card, `deleted` filter option, or restore affordance;
  JavaScript-only hiding is forbidden. A denied or failed delete keeps the app
  visible and explicitly says deletion did not complete without claiming
  revocation, removal, or cleanup.
- For the operator-only active-deployer allowlist, use one labelled native
  textarea prefilled from the server-rendered active snapshot plus a visible
  broadening checkbox. State that removed emails are signed out and cannot
  deploy; keep revision, authorization, collision, idempotency, audit, CSRF,
  and origin checks on the typed server path.
- A deployer dashboard is a compact, server-derived owned-app overview, not a
  visually disabled operator dashboard. Render only owned app summaries and
  stable launch links; omit management forms and every operator surface.
  Deployer management remains in the scoped Tinker CLI.
- Dashboard token management is deferred pending a dedicated ownership and
  scope overhaul. Render no token count, inventory, scope/lifetime field,
  create/revoke action, or hidden/disabled token form for any dashboard role.
  Keep the scoped API, Tinker CLI, and display-once result intact; browser
  omission is not authorization. A future reintroduction must first align
  actor ownership, task-oriented scopes, expiry, last-use/revoked status,
  display-once handling, and exact revocation behavior across contract,
  guidance, showcase, tests, and this skill.
- An operator-only coding-agent handoff is server-rendered guidance, never a
  deployment control: its fixed repository URL and exact HTTPS admin endpoint
  come from validated canonical server configuration, never a query/form/browser
  value; omit it when that host is malformed. The prompt contains no token,
  cookie, one-time-code value, secret, or browser identity data; it names
  `skills/tinkercloud-deployer/SKILL.md`, initially asks only for the deployer
  email (with the bounded OTP exception below), and
  directs the normal Tinker CLI and deployer-skill flow. It first checks for
  and reuses the exact server-scoped saved identity; only when it is missing,
  unauthorized, or expired may it request at most one CLI OTP. The agent asks
  the human initially only for the deployer email. Human-supervised coding-agent OTP
  handoff: only when no reusable server-bound bearer exists and after endpoint,
  email, and manifest validation, when the same CLI process reaches its normal
  `Code: ` prompt, the agent may ask the human
  exactly once for the short-lived emailed OTP, accept it in the agent
  interaction, and immediately submit it only to that same CLI process. Use this
  only with a trusted human-supervised agent; its provider may retain the
  interaction. Do not restate it or copy it into files, source, argv, logs, summaries, or final
  output; never ask for a bearer. A failed, malformed, timed-out, or denied OTP is
  terminal: there is no second code, retry, forced login, identity/account/server
  switch, or alternate collection path. The CLI stores the resulting scoped
  bearer for later exact-server reuse. Fully unattended
  `tinkercloud-deployment-agent` and VPS Resend-reader paths must not fall back
  to an agent interaction OTP. Then ask only minimal generate/build/deploy
  questions; never ask for bearer tokens, tokens, secrets, or operator access.
  “No OTP” forbids a code value, not this conditional normal CLI flow. Keep the prompt focusable and selectable
  without JavaScript. A labelled native copy button and polite feedback may
  enhance it only through local clipboard behavior; deployer dashboards omit it.
- Render the dashboard `Sign out of Tinkercloud` action as an ordinary labelled
  `POST /logout` form in the persistent top bar for every authenticated role.
  It carries the server-rendered CSRF value and must not clear browser cookies
  or redirect until the server has durably revoked the global identity family
  and every derived app session. A failed revocation remains a visible server
  error; it is never a local-only sign-out and never revokes CLI/agent bearers.
  App hosts separately label their local action `Sign out of this app`.
- Operator-managed external-capability credentials live in a distinct
  operator-only **API keys** section, before capability-specific profiles and
  grants. Initial fixed-provider creation asks only for provider and one native
  write-only password field; do not add a display-name, ID, URL, arbitrary-key,
  reveal, value echo, recovery, or provider-URL display. The service derives
  labels and connection/profile IDs server-side. If the server has no ready
  credential boundary, render the root-only enable-and-restart next step and no
  credential mutation forms or root detail. For credential rotation, omit a
  provider select and derive the provider server-side from the existing
  connection. App capability grant forms are nested under the server-rendered
  app target; revision fields protect profile/grant changes, and disable/revoke
  actions show an exact-target confirmation field.
- Escape user-controlled text through `html/template`. Never introduce unsafe
  HTML injection to make a component convenient.
- Keep the server self-contained and the dependency surface narrow.

Stop and record an ADR before changing deployment shape, trust boundaries,
asset delivery, persistence, or a public interface. Ask the user before adding
a production dependency.

## Apply the visual grammar

- Use the existing warm neutral canvas, paper surfaces, deep grape text, cobalt
  actions, rounded geometry, and tinker supporting color accents.
- Use `--tinker-*` role tokens. Do not scatter raw product colors through
  templates or domain packages.
- Keep operational copy direct. Playfulness belongs in display copy, not
  identifiers, errors, confirmation targets, or durable state.
- Keep security meaning in visible text. Color and icons may reinforce it but
  never replace it.
- Prefer one clear hierarchy, compact spacing, and restrained elevation.

Avoid generic generated-dashboard styling:

- no colored left or right accent rails on toasts, notices, or cards;
- no tone-colored container outline when a small icon or label is enough;
- no gratuitous gradients, glass, glow, floating blobs, or oversized shadows;
- no excessive pills, giant display type, or decorative mascots in operational
  flows;
- no copied third-party mascots or brand-specific visual signatures;
- no new visual treatment when an existing primitive already communicates the
  state.

Toast containers are neutral across every tone: one uniform border, one paper
surface, restrained shadow, compact leading status icon, title, description,
and visible close control.

## Design every state

List all states before styling: default, hover, focus, active, disabled, busy,
empty, loading, stale, unavailable, success, warning, danger, open, closed,
entering, and leaving as applicable.

Every visible state change must:

- animate smoothly in both directions;
- use shared `--tinker-motion-*` and `--tinker-ease-*` tokens;
- remain interruptible without snapping;
- preserve focus and semantic state;
- become effectively immediate under `prefers-reduced-motion`;
- never delay server truth, revocation, denial, or error text.

For irreversible deletion, distinguish the terminal server outcome in durable
copy: success redirects to an operational list that omits the permanently
removed app and clearly says its owned data is gone; denial or failure leaves
the app in that list and says deletion did not complete. Do not add a restore
affordance or use client-side list filtering to simulate either outcome.

Use native `details` and `summary` for disclosure. Use native `dialog` for modal
focus and Escape behavior. Keep no-JavaScript behavior usable.

## Create or change a component

1. Frame one user-observable outcome and name the affected trust boundary.
2. Inventory content, hierarchy, every state, keyboard behavior, responsive
   behavior, motion, and reduced-motion behavior.
3. For a complex or uncertain interaction, inspect current official guidance
   from mature systems such as Shadcn/Sonner, Base UI, or WAI-ARIA APG. Borrow
   anatomy and behavior, not framework dependencies or brand styling.
4. Write the denial and failure-path requirements in
   `specs/ui/native-web-system.md` before implementing the happy path.
5. Add rationale and usage guidance to
   `docs/product/web-design-system.md`.
6. Implement the thinnest semantic primitive in `web/assets/tinkercloud.css` and,
   only when needed, `web/assets/tinkercloud.js`.
7. Add a realistic example covering meaningful states to `web/showcase.html`.
8. Integrate the primitive into Tinkercloud templates only after the showcase
   contract is clear.
9. Add focused regressions to `web/ui_test.go` and affected handler/template
   tests. Prefer a test that makes a rejected visual or security pattern
   difficult to reintroduce.
10. Review the diff against Principles 8, 10, and 14 and the native UI spec.

Do not report a component complete because its resting screenshot looks good.

## Capture every new rule

When the user adds a durable rule, update all affected layers in the same
change:

1. Normative rule and denial/accessibility requirement in
   `specs/ui/native-web-system.md`.
2. Rationale and examples in `docs/product/web-design-system.md`.
3. Canonical CSS, JavaScript, templates, or embedded assets.
4. A visible state in `web/showcase.html`.
5. An executable regression in `web/ui_test.go` or the closest focused test.
6. This skill when the rule should guide future components.

Do not leave a new rule only in a prompt, screenshot, or prose document.

## Verify proportionally

Run the focused checks:

```text
node --check web/assets/tinkercloud.js
go test ./web ./internal/controlapi ./internal/compose ./internal/gateway
git diff --check
```

Run `go test ./...` when shared assets, templates, handlers, or interaction
behavior changed.

Use the local showcase for visual verification. Check:

- normal desktop and 320 CSS pixels;
- every meaningful state and tone;
- opening and closing motion, including rapid reversal;
- keyboard operation, visible focus, Escape, dismissal, and focus return;
- long text, overflow, and reflow;
- reduced-motion behavior when the browser can emulate it;
- browser console warnings and errors;
- uniform borders, spacing, radii, and shadows.

Compare the rendered result with any supplied screenshot. Inspect computed
styles when the complaint concerns an exact property such as border width.

## Maintain the skill

After changing this skill, keep `agents/openai.yaml` aligned and run:

```text
python3 "${SKILL_CREATOR_ROOT:?set to the skill-creator directory}/scripts/quick_validate.py" skills/tinkercloud-native-ui
```

Report the component outcome, rules captured, visual verification, tests run,
and any environment-limited check. Do not claim visual or security behavior
that was not exercised.
