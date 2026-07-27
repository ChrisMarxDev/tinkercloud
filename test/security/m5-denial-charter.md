# M5 Denial Charter

Test doctor redaction, non-root recovery denial, critical disk write denial,
cleanup retention of active/recoverable releases, invalid update signatures,
digest mismatch, interruption before replacement, restart/health/probe failure,
and rollback failure reporting. VPS-only evidence additionally proves systemd
permissions, socket inventory, clean installation, and a real failed update.

`tinyhost status` is strictly offline and must never read a provider credential
file or invoke a Resend diagnostic. `tinyhost doctor` is a root-only provider
credential reader: non-root invocation denies before any credential-file read
or provider request. Its default credential file and any explicit override must
be absolute, clean, free of symlinked components, root-owned, a single regular
file at mode `0600`, and contain exactly one assignment for each configured
Resend and HMAC reference. Absent, symlinked, non-root-owned, permissive,
malformed, duplicate, or unexpected-field files make only the Resend check
unavailable and must not invoke the provider. Diagnostics, command output,
errors, and provider failures must not disclose keys, credential paths,
references, or provider response bodies.

Init must not mark a host ready merely because a public URL answers. Its
derived platform probe permits only verified HTTPS at
`https://{platform_host}/api/v1/version`, with no redirect, the final configured
host, `200`, `application/json`, bounded exact `{"api_version":1}` evidence,
and `no-store`/`nosniff` gateway headers. Reject 401, 404, 5xx, arbitrary 2xx
or HTML, malformed/oversized or duplicate-key JSON, wrong version, redirect,
wrong host, TLS, transport, and timeout failures; a rejected probe leaves init
retryable rather than recording `verified`.

The unprivileged client installer is also a supply-chain denial surface. It
must reject root execution, unsupported OS/architecture combinations, insecure
release origins, malformed metadata, digest mismatches, invalid signatures,
and checksum manifests that omit or change any downloaded client sidecar. It
must not install a server binary, execute the candidate, or replace an existing
client before all of those checks pass.

The update anonymous-denial gate probes the root of an explicitly locally
verified active app host. It must receive the gateway's exact protected-route
denial: `401`, JSON content, `Cache-Control: no-store`, and the stable
`not_authorized` error shape. A missing app, 404, redirect, public 2xx body,
malformed denial, timeout, or HTTP dependency failure fails the gate and rolls
the candidate update back. The probe uses no user-provided URL and does not
read release content to construct its evidence.

The port-80 boundary is a denial surface: only autocert HTTP-01 challenge
requests may be delegated there. A valid configured platform or active app host
otherwise redirects only to its canonical HTTPS origin. Unknown, malformed,
IP-literal, userinfo, path-bearing, and non-HTTP-port Host headers fail closed
without a redirect, and no TinyHost app, control, authentication, or static
handler is reachable over plaintext. HTTPS responses carry HSTS; HTTP-01 and
plaintext denial/redirect responses do not.

App-host login UX is also a denial boundary. Only a genuine browser document
navigation to a protected static route may redirect to that same app's login
route with a validated relative return; anonymous API, asset, range, socket,
and ambiguous requests retain the exact JSON 401 denial with zero app bytes.
Wrong-app, expired, or malformed app cookies never become identity. Logout and
account switch are POST-only, exact-same-origin mutations that revoke only the
current app's host-only session, reject cross-origin and open-return input, and
cannot create a redirect loop or expose app content.

App session persistence must preserve independent opaque credentials across a
SQLite close/reopen: two sessions for one viewer and a session for another
viewer remain independently valid until their own expiry or revocation. A
single-session revoke survives restart and never revokes the sibling sessions;
wrong-app and expired credentials deny throughout. Tests may compare only
redacted values and persisted hashes, never raw credential values.

Control authentication uses three server-owned credential types: browser
control sessions, CLI bearer tokens, and app viewer sessions. A control OTP
challenge is durably bound to the server-selected browser or CLI channel before
delivery. A channel mismatch, legacy challenge without a channel, browser
session used as bearer, CLI bearer used as a control cookie, or app session used
on either control surface denies without issuing, authenticating, or recording
credential-use state. Root recovery revokes both operator bearer tokens and
control sessions before a new operator becomes active. Upgrade migration
evidence proves existing databases gain the control-channel column while old
unbound challenges fail closed. Because old `api_tokens` cannot be classified
as browser or CLI credentials, the follow-on migration revokes all of them;
pre-upgrade bearer values deny while a freshly issued post-migration CLI token
remains valid, and rerunning the migration engine never revives a token.

The packaged and generated systemd units are a privilege boundary: tests reject
root execution, missing `NoNewPrivileges`, any ambient or bounding capability
other than `CAP_NET_BIND_SERVICE`, and writable paths beyond the configured
data and ACME directories. The only retained privilege is binding the gateway's
public ports while the process remains the `tinyhost` user.

The root-only deployer recovery command is also a persistence ownership
boundary. It must reject a non-root caller before configuration or SQLite work,
then drop permanently to the installed `tinyhost` identity before opening the
database. Tests prove the database plus live WAL/SHM sidecars are owned by the
service identity after an authorization mutation. Missing identity,
privilege-drop, database-open, or mutation failures must not claim success,
leave a root-owned replacement artifact, or make the service database
unwritable; permissive `chmod` is never a repair path.
