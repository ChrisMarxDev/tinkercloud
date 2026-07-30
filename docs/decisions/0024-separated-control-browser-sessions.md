# ADR 0024: Separate control browser sessions from CLI bearer tokens

**Status:** Dashboard-session portion superseded by ADR 0051; CLI bearer
separation retained

## Context

The platform dashboard needs an ambient host-only browser session, while the
deployer CLI needs a bearer token. Storing both values in `api_tokens` makes
the cookie name the only distinction: a browser credential can then be sent as
an API bearer and a CLI credential can become an ambient browser credential.
That violates the control/viewer/CLI separation promised by Tinkercloud.

## Decision

Browser control logins create a `sessions` row with `scope='control'`; CLI
logins create an `api_tokens` row. Authentication queries only its matching
table and credential type, regardless of token text or HTTP header/cookie
placement. Control OTP challenges record a server-selected `control_channel`
of `browser` or `cli`; verification requires the same channel and creates only
the matching credential in the same transaction as challenge consumption.

The migration adds the nullable channel to existing challenge rows. Existing
unbound control challenges are deliberately invalid after upgrade rather than
being guessed into either authority. Since pre-separation `api_tokens` cannot
be distinguished as former browser cookies or CLI bearers, a follow-on
migration revokes every existing token. Operators and deployers complete one
fresh CLI login; no legacy credential is silently converted or retained.

ADR 0033 adds a fourth, still separate browser credential: a platform-host
global *viewer identity* session. It can participate only in a one-time,
app-bound viewer handoff and is neither a control cookie nor a CLI bearer.

ADR 0051 removes the dashboard control-session credential and makes that global
browser identity the dashboard authentication input. Dashboard authority still
requires a fresh role lookup. CLI bearer separation and header/cookie
cross-presentation denial from this decision remain in force.

## Consequences

The following two bullets are historical consequences of the superseded
dashboard-session portion. They do not describe a supported current browser
credential path; ADR 0051 replaces it with the admin-host global identity.

- The retired dashboard cookie could not be replayed to the bearer control API,
  and a CLI token could not authenticate dashboard HTML.
- The retired browser logout and root recovery revoked control-session rows.
- The schema migration is forward-only. No downgrade promise is made, and
  operators/deployers must complete a fresh CLI login after upgrade; an
  interrupted legacy OTP flow also requires a fresh request.
- The current global browser identity does not merge dashboard-role authority
  with app-policy authorization, even where normalized email values match.
