---
name: tiny-native-ui
description: Use and evolve TinyHost's dependency-free native web UI for TinyHost-owned login, dashboard, diagnostics, token, and operations screens. Use when Codex creates, edits, or reviews native UI components, CSS tokens, vanilla JavaScript interactions, Go templates, component states, motion, accessibility, showcase examples, or new design-system rules. Do not use for deployed app content or the marketing landing page.
---

# Tiny Native UI

Build TinyHost-owned interfaces as one small server-rendered system. Preserve
the warm Tiny Cloud character without weakening operational clarity, native
semantics, accessibility, or the gateway security boundary.

## Read the sources of truth

Read these files before planning or editing, in this order:

1. `PRINCIPLES.md`
2. `PRD.md`, especially UI requirements and §18.3
3. `specs/ui/native-web-system.md`
4. `docs/product/web-design-system.md`
5. `docs/decisions/0021-embedded-native-web-design-system.md`
6. `web/assets/tinyhost.css`, `web/assets/tinyhost.js`, `web/showcase.html`,
   `web/ui.go`, and `web/ui_test.go`

Treat the UI spec as normative behavior, the product document as rationale and
examples, and the assets as the canonical implementation. Update higher-order
sources before derived guidance when they conflict.

## Choose the smallest path

- To use an existing component, compose its `tiny-*` classes with semantic HTML
  in the relevant Go template. Do not create a near-duplicate.
- To add a visual variant, prefer an existing token or state attribute. Add a
  token only when it represents a reusable role.
- To create or materially change a component, follow the full component
  workflow below.
- To add a design rule, follow the rule-capture protocol in the same change.

This system applies only to TinyHost-owned surfaces. Never give deployed apps
the control-plane mark, stylesheet, or chrome.

## Preserve the architecture

- Keep Go `html/template`, embedded local CSS, and minimal local vanilla
  JavaScript.
- When a template embeds `tinyCSS` or `tinyJS`, keep the gateway CSP strict by
  using `webui.TinyStyleCSPSource()` and `webui.TinyScriptCSPSource()` for the
  exact compile-time bytes. Never solve the embedding problem with
  `unsafe-inline`, a remote source, or a new asset route.
- Add no React, Tailwind, UI runtime, remote font, CDN asset, analytics, second
  web server, or frontend build requirement.
- Start with semantic HTML and native controls. Use JavaScript only for
  progressive presentation behavior that HTML and CSS cannot express well.
- Keep authentication, authorization, confirmation, CSRF, ownership, and
  mutation on typed server paths. Browser state and animation are never
  security controls.
- Global viewer identity/handoff pages use the existing auth card, native form,
  and text-bearing notice primitives. Do not create an account picker,
  browser-held identity state, or app-visible control. “Use another email” is
  a platform-host POST verification flow whose copy says it signs app sessions
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
- Escape user-controlled text through `html/template`. Never introduce unsafe
  HTML injection to make a component convenient.
- Keep the server self-contained and the dependency surface narrow.

Stop and record an ADR before changing deployment shape, trust boundaries,
asset delivery, persistence, or a public interface. Ask the user before adding
a production dependency.

## Apply the visual grammar

- Use the existing warm neutral canvas, paper surfaces, deep grape text, cobalt
  actions, rounded geometry, and tiny supporting color accents.
- Use `--tiny-*` role tokens. Do not scatter raw product colors through
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
- use shared `--tiny-motion-*` and `--tiny-ease-*` tokens;
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
6. Implement the thinnest semantic primitive in `web/assets/tinyhost.css` and,
   only when needed, `web/assets/tinyhost.js`.
7. Add a realistic example covering meaningful states to `web/showcase.html`.
8. Integrate the primitive into TinyHost templates only after the showcase
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
node --check web/assets/tinyhost.js
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
python3 "${SKILL_CREATOR_ROOT:?set to the skill-creator directory}/scripts/quick_validate.py" skills/tiny-native-ui
```

Report the component outcome, rules captured, visual verification, tests run,
and any environment-limited check. Do not claim visual or security behavior
that was not exercised.
