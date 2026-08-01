# Local app insights contract

## Outcome

Tinkercloud answers only whether an app received successful document visits,
approximately how many browser cookies appeared, and when the last counted
visit occurred. It uses the existing gateway and control SQLite database and
never loads third-party analytics code.

## Counted request

A page view is one successful non-range `GET` that returns an HTML document for
an exact static file or validated SPA fallback after authorization. Assets,
source maps, reserved routes, API requests, WebSockets, health checks,
certificate/deployment probes, HEAD/POST/other methods, ranges, redirects, and
errors do not count.

Analytics is best-effort after authorization. The request path offers one
bounded record to a fixed-capacity process-local queue and never starts a
goroutine per request or waits for SQLite. A full, stopped, unavailable, or
failed recorder drops the analytics record. Failure or timeout must not change
status, headers unrelated to analytics, body bytes, authorization, static
integrity verification, or app availability.

## Approximate browser marker

The gateway uses a cryptographically random app-origin host-only cookie:

- `Secure`, `HttpOnly`, `SameSite=Lax`, `Path=/`;
- maximum lifetime 30 days;
- no `Domain` attribute;
- never readable by app JavaScript.

Storage receives only `HMAC(analytics-key, app-id || cookie-value)` and the UTC
day/expiry. A missing or malformed cookie still increments the page view and
may receive a fresh cookie, but it creates no visitor marker until a later
request returns one valid cookie; this avoids treating a blocked cookie as a
reliable returning-browser identity. Raw cookie values, app/global sessions, identity, email, IP,
request fingerprint, URL/path/query, referrer, user agent, content, geography,
and device data are forbidden. “Approximate visitors” means distinct valid
app-scoped markers observed in the selected period, not people or accounts.

## Storage and retention

The control database stores bounded per-app UTC daily page-view totals and
last-activity time plus expiring app-scoped visitor markers. Period visitor
totals count distinct digests across the complete selected window; summing
daily distinct totals is forbidden. Reads exclude data before their UTC cutoff.
A startup-before-ready and scheduled cleanup removes records older than 30 days
in bounded pages. Hard app
deletion removes both aggregates and markers transactionally with app-owned
control state. Analytics has one operator-controlled global disable switch;
disabled or unavailable means no new tracking and no response failure.
The switch defaults enabled and is mutated only on the VPS through
`tinkercloud insights enable|disable`. The root command delegates the SQLite
write to the `tinkercloud` service identity, then refreshes only an already
active service after the durable write succeeds; it never exposes a browser or
remote mutation route.

## Read authorization

An exact active app owner or operator may read a bounded 7-day or 30-day
summary: page views, approximate visitors, last activity, and a zero-filled UTC
daily series. Viewers, public visitors, app JavaScript/SDK sessions,
deployment-agent tokens, revoked users, and unrelated deployers are denied
before aggregate data is queried.

The native dashboard presents the 7-day and 30-day totals with the exact label
`Approximate visitors`, plus one zero-filled 30-day textual daily table. A
verified empty window displays zero; a disabled or unavailable read displays an
unavailable state and never substitutes zero.

Malformed app targets, ranges, dates, cookies, and digests fail within fixed
bounds. Responses and logs never expose raw tracking material or forbidden
request metadata.
