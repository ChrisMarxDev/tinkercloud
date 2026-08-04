# Tinkercloud native web design system

Tinkercloud's native UI should feel like the landing page grew up into an
operational tool: warm, friendly, small, and unusually clear about security.
The system deliberately keeps the landing page's cream, grape, cobalt, rounded
geometry, compact color flourishes, and direct language while reducing display
scale and decoration for forms and data-heavy screens.

The normative behavior and denial rules live in
[`specs/ui/native-web-system.md`](../../specs/ui/native-web-system.md). The
canonical implementation is [`web/assets/tinkercloud.css`](../../web/assets/tinkercloud.css),
[`web/assets/tinkercloud.js`](../../web/assets/tinkercloud.js), and
[`web/assets/tinkercloud-mark.svg`](../../web/assets/tinkercloud-mark.svg).
The repository workflow for using the system, creating components, and
capturing future rules lives in
[`skills/tinkercloud-native-ui/SKILL.md`](../../skills/tinkercloud-native-ui/SKILL.md).

## Foundations

### Color roles

| Token | Role |
|---|---|
| `--tinker-canvas` | Warm neutral page background |
| `--tinker-surface` | Primary paper/card surface |
| `--tinker-ink` | Deep grape headings and primary text |
| `--tinker-muted` | Supporting copy and metadata |
| `--tinker-primary` | Cobalt action, link, and focus color |
| `--tinker-primary-soft` | Selected and informational surface |
| `--tinker-positive` / `--tinker-positive-soft` | Healthy, active, verified |
| `--tinker-warning` / `--tinker-warning-soft` | Degraded, attention, limits |
| `--tinker-danger` / `--tinker-danger-soft` | Destructive, failed, blocked |
| `--tinker-accent-pink` / `--tinker-accent-yellow` | Small decorative accents only |
| `--tinker-qr-ink` / `--tinker-qr-surface` | Scanner-safe QR contrast |

Security meaning always includes text. A green dot alone never means
“authorized,” and a pink surface alone never means “failed.”

### Type

Native UI uses `ui-rounded` for brand and headings, `system-ui` for interface
copy, and `ui-monospace` for tokens, commands, hashes, IDs, and code. This
preserves the landing page's rounded character without adding a font download
or frontend build dependency to the server.

### Shape and spacing

- Controls: 12–14 px radius.
- Cards: 20–28 px radius.
- Pills and status badges: fully rounded.
- Spacing follows a compact 4/8/12/16/24/32/48 px rhythm.
- Shadows are short, opaque “lift” shadows rather than gradients or glass.

### Motion

State changes should feel soft and intentional, never ornamental or slow.
Buttons and color changes use the fast token, overlays use the base token, and
content-dependent spatial changes use the disclosure token. Both enter and exit
states animate.

Expandable panels are measured at runtime so opening, closing, and reversing
mid-animation continue from the currently rendered height. Without JavaScript
they remain ordinary native `details` elements. Reduced-motion users receive
the same state change immediately.

Animation follows server truth. A revoked app is revoked before a panel,
badge, toast, or dialog finishes moving.

## Component rules

### Authentication

Use a single centered auth card with:

1. Tinkercloud brand or “Protected by Tinkercloud” lockup.
2. One plain heading.
3. One sentence describing the current step.
4. A generic status notice when needed.
5. One primary form action.
6. A quiet security/recovery note.

Never reveal whether an entered email is authorized. Code inputs use
`autocomplete="one-time-code"` and numeric input mode.

Global viewer identity and app handoff deliberately introduce no new visual
component. Use the existing centered auth card for “Continue as your verified
email” and the existing text-bearing notice for generic denied/unavailable
states. The platform-auth page may offer “Use another email” as an ordinary
POST form; copy must say that it changes the viewer identity for this browser
and signs its app sessions out. Never render that identity as a deployer or
operator role, place it in deployed app chrome, expose callback state, or use a
client-side account picker. Existing auth-card, notice, native form, and
server-rendered failure primitives already cover authenticated, handoff,
denied, and switching states, so a showcase-only component or JavaScript
interaction would add no durable rule.

The platform may maintain an HTTP-only browser-profile binding to serialize
competing OTP completions. It is deliberately invisible: never name it in
copy, render it as a field, transfer it in browser-visible state, or imply that
its persistence through global sign-out means the viewer remains signed in.

Keep scopes explicit in visible labels: the app-host POST action is “Sign out
of this app,” while the admin-host action is “Use another email” or “Sign out
of Tinkercloud.” The latter explains that dashboard identity and app sessions are
signed out.
An allowed handoff may display the verified address as escaped text in the auth
card. A denied app may also show that already-authenticated address beside the
generic notice and “Use another email” action; it must not reveal allowlist
membership, policy detail, or callback state.

### Dashboard

The dashboard uses a compact top bar, a plain page heading, visible ownership
and utility-data disclaimers, then progressive disclosure:

- summary cards first;
- app state and protection summary next;
- policy, release, and danger operations inside native `details` panels;
- operator-only deployers and audit remain distinct sections;
- health is always visible and includes a local diagnostic next step; and
- the operator-only host resource overview uses a compact, labeled recent
  CPU/RAM/data-volume chart with current storage bytes and watermark markers.
  It remains understandable as text without JavaScript and shows unavailable
  data explicitly rather than drawing a zero value.

The same top bar stays present for operators and deployers. It identifies the
server-derived current role and offers one quiet `Sign out of Tinkercloud` form.
Sign-out is not cosmetic: the server revokes the global browser identity family
and all derived app sessions before it clears the identity and CSRF cookies. If
that durable revocation cannot be completed, the dashboard keeps the current
cookies and renders an actionable server error instead of pretending the
browser signed out. It never acts on a CLI or deployment-agent bearer.

A deployer sees a deliberately list-only overview containing only
server-authorized cards for their own apps. The app list is the primary landing
surface; operator deployer, audit, health-management, provider, and per-app
control panels are omitted rather than visually disabled. Deployers continue
to use the scoped Tinker CLI for management actions.
If that owned-app read model is unavailable, show an explicit unavailable state
with the safe retry/operator-diagnostic next step; never make an outage look
like the deployer owns no apps.

A suspended app remains in the full-width app list as a metadata-only row. Its
last active immutable release may still supply the description, access posture,
and release history, but the row omits the stable launch link and QR action.
Suspending one app must not make the dashboard or sibling rows unavailable, and
resuming never resurrects revoked app sessions, app-scoped tokens, or live
connections.

#### Deferred dashboard token management

Do not render token counts, inventory, scope or lifetime inputs, creation, or
revocation in operator or deployer dashboards. The removed panel mixed an
operator-only presentation with owner-only mutations, accepted scopes that were
not all meaningful for an app-bound credential, and omitted essential expiry,
last-use, and revoked-state context. A partial browser control would therefore
make authority harder to understand.

Keep the existing scoped control API, Tinker CLI workflow, and display-once
success page. A later dashboard overhaul must first define the actor and app
ownership model, task-oriented scope presets, safe lifetime defaults, complete
credential status, display-once handling, and exact revocation behavior. Until
then, omission is server-rendered: do not ship hidden markup, disabled forms,
token counts, or a JavaScript feature flag that can reveal the controls.

#### Operator API-key and LLM chat controls

Keep external-provider setup in a dedicated server-rendered **API keys**
section before **LLM chat**. The API-key card asks only for a fixed provider
selection and one write-only password field; it never asks for a display name,
identifier, URL, or generic secret label. Tinkercloud derives the label from the
fixed provider and may show only that safe label, provider kind, opaque
server-generated ID, and durable status afterwards. When key management is not
server-ready, show a quiet unavailable card with root-only `tinkercloud llm enable`
and restart guidance instead of any credential mutation control; never show the
root or configuration reason. Rotation has no provider select: Tinkercloud resolves
the connection’s stored provider before validating the replacement credential.
Keep profiles, grants, limits, and usage in **LLM chat**. A profile is a labeled
form for a fixed model and every limit. Prefer a native select populated by the
server from bounded live model lists fetched with active stored API keys. Group
each option by its safe connection label and provider; omit a connection on
decrypt, timeout, provider, or response failure instead of showing a stale or
unverified choice. Keep an adjacent native disclosure named **Use a custom model
identifier** with an active-connection select and bounded text input. Explain
that it supports newly released models while the fixed provider endpoint and
all limits remain in force. The custom path must remain usable when no catalog
option is available. Grant controls live on the
server-rendered target app, not in a free-form app-ID field. Use revision fields
for profile/grant updates, and native exact-target confirmation fields for key
disable and grant disable/revoke. Never place credentials, envelopes, provider
URLs, or raw provider errors in a notice, table, source, or reveal view.

When a dashboard has multiple authorized app cards, a local search plus status
filter may help scan that already-rendered list. It is deliberately a
progressive presentation aid: controls are hidden until the local helper is
ready, no-JavaScript users see every authorized card, and no client query,
storage, or authorization state is involved. Search uses the stable `tinker.yaml`
slug and the current immutable deployment description case-insensitively;
status uses the displayed durable state exactly. A
polite count announces the narrowed result and a no-results panel says to
adjust the search, description, or status filter, rather than implying that the account owns
no apps.

Successful irreversible deletion redirects to a dashboard whose operational
read model has already excluded that app: it is absent from the list, local
search/filter results, and visible count because confirmed deletion permanently
removes the app and its owned data. It is not represented as a deleted card, a
`deleted` filter option, or a restore action. This is server truth, never
JavaScript-only hiding. If deletion is denied or fails, retain the app in the
dashboard and say plainly that deletion did not complete; do not imply that
revocation, removal, or cleanup succeeded.

An active app with a configured stable gateway origin may show one compact
external-launch icon beside its title. It is a native link with an accessible label and title,
opens only `https://{slug}.{domain}/` in a new tab with `noopener noreferrer`,
and does not imply that it bypasses the app's ordinary authentication policy.
Beside it, a compact QR action may open a small native dialog for moving the
same stable URL to a phone. Generate the code locally from that already
server-derived URL, show the destination as wrapping text, and say that normal
sign-in and access policy still apply. If the local encoder cannot represent
the URL, omit the action; never fall back to a remote QR image service.
Release hashes and immutable release paths are never links.
Release history is read-only in V1: show immutable metadata without a rollback
button, form, or client-side approximation. Failed activation preservation is
reported as server truth, not an operator/deployer action.

Only `verified`, `active`, and `superseded` release records may contribute a
description: those immutable states require a valid stored manifest. The
operational history can also show `uploading`, `uploaded`, `validating`,
`staged`, `rejected`, and `failed` candidates, but renders their descriptions
blank without parsing candidate bytes. An unknown state or malformed immutable
manifest makes the dashboard unavailable rather than inventing a summary.

Keep the resting app row about recognition and current state. Place the app
title, stable launch action, and QR action in one compact identity cluster;
keep the durable status and one operator-only More disclosure opposite it.
More lists Access policy, Releases, the optional LLM chat grant, and App
controls, and opens each surface in its own focused native dialog. These are
the existing server-rendered forms and read models, not client-side copies.
Before enhancement, or whenever pairing a dialog fails, leave their native
fallback details visible and keep More hidden. A deployer overview has no
operator More menu or empty management dialogs.

The operator-only deployer section uses one native multiline active allowlist
form, prefilled from a server-rendered revisioned snapshot. It makes authority
broadening explicit and says that removed addresses are signed out and cannot
deploy; it is neither an account picker nor a viewer-identity control.

### Minimum-necessary input

Do not render a field merely because a config or API schema contains it. Reuse
trusted server state, derive server-owned values, and apply secure defaults
before asking. Optional fields belong behind a review/edit disclosure. A field
or confirmation is justified only when its answer is required, not already
known, and unsafe to default.

Never ask a browser actor for an app ID, owner ID, policy revision, stable URL,
storage path, credential selector, or other value the server can derive.
Authorization broadening and permanent deletion remain explicit even when the
rest of the form is inferred.

### Forms and mutations

- Primary buttons advance a safe flow.
- Secondary buttons perform bounded reversible operations.
- Quiet buttons navigate or sign out.
- Danger buttons suspend, revoke, or delete.
- A destructive panel states the exact app, deployer, token, or release target.
- A successful delete confirmation says the app and its owned data are
  permanently removed; it offers no restore action.
- A denied or failed delete says deletion did not complete and preserves the
  still-visible app as the safe next step.
- Access-policy forms summarize the resulting private policy before submission.
- Busy state disables the submitted control but never changes authorization
  semantics.
- Invalid controls retain their submitted values, set `aria-invalid="true"`,
  and point to visible help/error copy using `aria-describedby`. Do not hide
  server validation behind a toast.

### Operational states and navigation

- Loading names the operation in progress. It never invents a client-side
  completion state.
- Stale/refreshing data says that it is cached and shows the last safe update
  time. It does not mask unavailable, revoked, or failed server truth.
- Unavailable and error states state the safe next step without exposing a
  provider response, policy membership, path, SQL, stack, or secret.
- A stage list is an ordered semantic list with a visible current state; a
  progress bar supplements the text rather than replacing it.
- Pagination uses ordinary server-derived links. The current page is marked
  with `aria-current="page"`; unavailable directions are non-interactive text
  with `aria-disabled="true"`, not JavaScript-only controls.

### Status and feedback

Status badges render the durable state label. Notices provide a title, a safe
message, and—when useful—one next action. Prefer:

- “Deployment failed. Your previous release is still active.”
- “Email is unavailable. Existing sessions continue; new sign-ins are paused.”
- “Writes are disabled at the critical disk watermark. Static reads continue.”

Avoid vague copy such as “Something went wrong” when a safe stable reason and
next step exist.

Toasts are for short, non-blocking feedback after the server has already
accepted or rejected an operation. They never hold the only copy of an error or
security consequence. Danger toasts remain explicit and use an urgent live
announcement. The container stays neutral across all tones: one quiet border,
one paper surface, and a restrained shadow. A compact leading icon carries the
supporting status color. Do not use colored side rails, tone-colored outlines,
or large tinted backgrounds.

Dialogs use the browser's native `dialog` behavior for focus containment,
Escape handling, and focus restoration. A dialog may summarize an operation,
but the underlying server-rendered route and form remain usable without
JavaScript. Dialog dismissal is never equivalent to authorization or mutation.

Expandable cards use native `details` and `summary`. Use them to defer secondary
configuration or explanation while keeping the summary, durable state, and
important next action visible. The local interaction helper smoothly animates
their content-dependent height in both directions and supports interruption.

### Public reach, catalog, and local insights

The team catalog uses the existing card/list rhythm because it is a shelf of
authorized apps, not a second operational dashboard. Public/private posture and
tags are compact metadata with visible text. Search matches only the rendered
slug, active immutable description, and tags case-insensitively; tag selection
matches one rendered canonical tag exactly. Both controls are a local
convenience over server-authorized cards and disappear cleanly when JavaScript
is unavailable.

Local insights answer only three questions: approximately how many browser
cookies returned, how many document views succeeded, and when the last view
occurred. Reuse stat/definition/table primitives, label “Approximate visitors”
in full, show bounded 7-day and 30-day totals, and keep the zero-filled 30-day
daily values available as text at every width. A future chart may supplement
but never replace those values. Empty means a verified zero; unavailable has
its own state and next step. The global tracking switch remains a root-local
`tinkercloud insights enable|disable` operation; the browser only displays
server-derived read state.

On the compact full-width app row, analytics stays secondary. Pair 7-day and
30-day totals and last activity in the left half with a miniature 30-day
two-series bar chart and visible page-view and **Approximate visitors**
swatches in the right half. Stack the halves only when the row becomes too
narrow. Use bounded `data-*` CSS bars rather
than inline chart geometry or a chart package. Each paired bar is
keyboard-focusable as well as hoverable and reveals its exact UTC day, page
views, and approximate visitors; retain the complete daily series as a
semantic visually-hidden table so the chart never replaces raw values. An
unavailable read shows its existing explicit unavailable state, never a blank
or zero chart. This is display-only local data: it adds no browser request,
storage, identity, or authorization role.

Public publishing is an access broadening, not a decorative status toggle. The
confirmation copy names the app and says that anyone on the internet can open
its static files. Capability exclusion and no-index/indexed state remain
visible adjacent facts. The first pilot does not put the root operator gate in
the browser; native UI displays server truth while the root-local command owns
the mutation.

### Operator coding-agent handoff

The operator dashboard may offer one compact **Get started with a coding
agent** card. It is guidance, not a deployment control: it gives a ready-to-copy
prompt with the fixed Tinkercloud repository URL and the current HTTPS admin
endpoint derived by the server from validated configuration. The prompt asks an
agent to follow `skills/tinkercloud-deployer/SKILL.md`, ask only for the
deployer email, and run the normal Tinker CLI and deployer-skill flow. It first
checks for and reuses the exact server-scoped saved identity for that deployer.
Only if that identity is missing, unauthorized, or expired may it request at
most one CLI OTP. The agent initially asks the human only for the deployer
email; the bounded OTP handoff below is the sole later credential exception.
Human-supervised coding-agent OTP handoff: only when no reusable server-bound
bearer exists and after endpoint, email, and manifest validation, when that
the same CLI process reaches its normal `Code: ` prompt, the agent may ask the
human exactly once for the short-lived emailed OTP, accept it
in the agent interaction, and immediately submit it only to that same CLI
process. Use this only with a trusted human-supervised agent; its provider may
retain the interaction. Do not restate it or copy it into files, source, argv, logs, summaries,
or final output; never ask for a bearer. A failed, malformed, timed-out, or
denied OTP is terminal: there is no second code, retry, forced login,
identity/account/server switch, or alternate collection path. The CLI stores
the resulting scoped bearer for later exact-server reuse. Fully unattended
`tinkercloud-deployment-agent` and VPS Resend-reader paths must not fall back to
an agent interaction OTP. It then generates, builds, and deploys with minimal
questions, and never asks for bearer tokens, tokens, secrets, or operator
access.

Render the prompt as ordinary focusable, selectable code text. A native Copy
prompt button can appear only after local clipboard initialization and updates
a polite status message; no-JavaScript and clipboard-failure states tell the
operator to select and copy the text manually. Do not render endpoint fields,
query overrides, browser-held endpoint state, deployer tokens, cookies,
one-time-code values, or secrets. “No OTP” forbids exposing a code value, not
the conditional normal CLI flow: the exact server-scoped saved identity may be
reused without an OTP, and an OTP may be requested once only when it is
missing, unauthorized, or expired. A trusted supervised agent may use only the
bounded same-CLI handoff. A malformed configured host omits the
card. Deployer dashboards omit it entirely.

## Usage

Go templates receive `tinkerCSS`, `tinkerJS`, and `tinkerMark` through
`webui.FuncMap()`:

```go
t := template.Must(
    template.New("page").
        Funcs(webui.FuncMap()).
        Parse(pageTemplate),
)
```

```html
<style>{{tinkerCSS}}</style>
<script>{{tinkerJS}}</script>
<a class="tinker-brand" href="/">
  <span class="tinker-brand__mark" aria-hidden="true">{{tinkerMark}}</span>
  <span>tinkercloud</span>
</a>
```

Use classes for composition, but keep semantic HTML:

```html
<label class="tinker-field">
  <span class="tinker-label">Email address</span>
  <input class="tinker-input" type="email" autocomplete="email" required>
</label>
<button class="tinker-button tinker-button--primary">Send code</button>
```

The optional embedded interaction helper powers declarative dialog triggers and
toast creation in richer native screens:

```html
<button data-tinker-dialog-open="delete-dialog">Review deletion</button>
<dialog class="tinker-dialog" id="delete-dialog" aria-labelledby="delete-title">
  <!-- ordinary server-rendered confirmation form -->
</dialog>

<div
  class="tinker-toast-region"
  data-tinker-toast-region
  aria-live="polite"
  aria-label="Notifications"
></div>
```

Call `TinkerUI.toast("Deployment active.", {tone: "success"})` for client-side
feedback that does not carry server authority. Native pages can also render
`.tinker-toast` markup directly when feedback must survive without JavaScript.

Deployed application content is not a consumer of this system. It is for
Tinkercloud-owned login, control, diagnostics, and operations surfaces only.
