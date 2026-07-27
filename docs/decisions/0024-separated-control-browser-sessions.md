# ADR 0024: Separate control browser sessions from CLI bearer tokens

**Status:** Accepted for V1

## Context

The platform dashboard needs an ambient host-only browser session, while the
deployer CLI needs a bearer token. Storing both values in `api_tokens` makes
the cookie name the only distinction: a browser credential can then be sent as
an API bearer and a CLI credential can become an ambient browser credential.
That violates the control/viewer/CLI separation promised by TinyHost.

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

## Consequences

- A dashboard cookie cannot be replayed to the bearer control API, and a CLI
  token cannot authenticate dashboard HTML.
- Browser logout and root operator recovery revoke control-session rows;
  suspension/revocation still denies both credential types on the next request.
- The schema migration is forward-only. No downgrade promise is made, and
  operators/deployers must complete a fresh CLI login after upgrade; an
  interrupted legacy OTP flow also requires a fresh request.
