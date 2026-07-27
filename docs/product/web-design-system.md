# TinyHost native web design system

TinyHost's native UI should feel like the landing page grew up into an
operational tool: warm, friendly, small, and unusually clear about security.
The system deliberately keeps the landing page's cream, grape, cobalt, rounded
geometry, compact color flourishes, and direct language while reducing display
scale and decoration for forms and data-heavy screens.

The normative behavior and denial rules live in
[`specs/ui/native-web-system.md`](../../specs/ui/native-web-system.md). The
canonical implementation is [`web/assets/tinyhost.css`](../../web/assets/tinyhost.css),
[`web/assets/tinyhost.js`](../../web/assets/tinyhost.js), and
[`web/assets/tiny-cloud-mark.svg`](../../web/assets/tiny-cloud-mark.svg).
The repository workflow for using the system, creating components, and
capturing future rules lives in
[`skills/tiny-native-ui/SKILL.md`](../../skills/tiny-native-ui/SKILL.md).

## Foundations

### Color roles

| Token | Role |
|---|---|
| `--tiny-canvas` | Warm neutral page background |
| `--tiny-surface` | Primary paper/card surface |
| `--tiny-ink` | Deep grape headings and primary text |
| `--tiny-muted` | Supporting copy and metadata |
| `--tiny-primary` | Cobalt action, link, and focus color |
| `--tiny-primary-soft` | Selected and informational surface |
| `--tiny-positive` / `--tiny-positive-soft` | Healthy, active, verified |
| `--tiny-warning` / `--tiny-warning-soft` | Degraded, attention, limits |
| `--tiny-danger` / `--tiny-danger-soft` | Destructive, failed, blocked |
| `--tiny-accent-pink` / `--tiny-accent-yellow` | Tiny decorative accents only |

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

1. TinyHost brand or “Protected by TinyHost” lockup.
2. One plain heading.
3. One sentence describing the current step.
4. A generic status notice when needed.
5. One primary form action.
6. A quiet security/recovery note.

Never reveal whether an entered email is authorized. Code inputs use
`autocomplete="one-time-code"` and numeric input mode.

### Dashboard

The dashboard uses a compact top bar, a plain page heading, visible ownership
and utility-data disclaimers, then progressive disclosure:

- summary cards first;
- app state and protection summary next;
- policy, token, release, and danger operations inside native `details`
  panels;
- operator-only deployers and audit remain distinct sections;
- health is always visible and includes a local diagnostic next step; and
- the operator-only host resource overview uses a compact, labeled recent
  CPU/RAM/data-volume chart with current storage bytes and watermark markers.
  It remains understandable as text without JavaScript and shows unavailable
  data explicitly rather than drawing a zero value.

When a dashboard has multiple authorized app cards, a local search plus status
filter may help scan that already-rendered list. It is deliberately a
progressive presentation aid: controls are hidden until the local helper is
ready, no-JavaScript users see every authorized card, and no client query,
storage, or authorization state is involved. Search uses the stable `tiny.yaml`
slug and the current immutable deployment description case-insensitively;
status uses the displayed durable state exactly. A
polite count announces the narrowed result and a no-results panel says to
adjust the search, description, or status filter, rather than implying that the account owns
no apps.

An active app with a configured stable gateway origin may show one compact
external-launch icon. It is a native link with an accessible label and title,
opens only `https://{slug}.{app_suffix}/` in a new tab with `noopener noreferrer`,
and does not imply that it bypasses the app's ordinary authentication policy.
Release hashes and immutable release paths are never links.

### Forms and mutations

- Primary buttons advance a safe flow.
- Secondary buttons perform bounded reversible operations.
- Quiet buttons navigate or sign out.
- Danger buttons suspend, revoke, or delete.
- A destructive panel states the exact app, deployer, token, or release target.
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

## Usage

Go templates receive `tinyCSS`, `tinyJS`, and `tinyMark` through
`webui.FuncMap()`:

```go
t := template.Must(
    template.New("page").
        Funcs(webui.FuncMap()).
        Parse(pageTemplate),
)
```

```html
<style>{{tinyCSS}}</style>
<script>{{tinyJS}}</script>
<a class="tiny-brand" href="/">
  <span class="tiny-brand__mark" aria-hidden="true">{{tinyMark}}</span>
  <span>tinyhost</span>
</a>
```

Use classes for composition, but keep semantic HTML:

```html
<label class="tiny-field">
  <span class="tiny-label">Email address</span>
  <input class="tiny-input" type="email" autocomplete="email" required>
</label>
<button class="tiny-button tiny-button--primary">Send code</button>
```

The optional embedded interaction helper powers declarative dialog triggers and
toast creation in richer native screens:

```html
<button data-tiny-dialog-open="delete-dialog">Review deletion</button>
<dialog class="tiny-dialog" id="delete-dialog" aria-labelledby="delete-title">
  <!-- ordinary server-rendered confirmation form -->
</dialog>

<div
  class="tiny-toast-region"
  data-tiny-toast-region
  aria-live="polite"
  aria-label="Notifications"
></div>
```

Call `TinyUI.toast("Deployment active.", {tone: "success"})` for client-side
feedback that does not carry server authority. Native pages can also render
`.tiny-toast` markup directly when feedback must survive without JavaScript.

Deployed application content is not a consumer of this system. It is for
TinyHost-owned login, control, diagnostics, and operations surfaces only.
