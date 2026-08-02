# Email Provider Contract

Tinkercloud sends numeric authentication codes through one operator-selected,
compiled-in outbound adapter. The choice changes only outbound email delivery;
it grants no authority and does not change challenge, identity, session, or
policy behavior.

## Supported providers

- `resend`: direct HTTPS JSON requests to the fixed Resend API.
- `postmark`: direct HTTPS JSON requests to the fixed Postmark API using the
  default transactional `outbound` message stream.

Exactly one provider is active. There is no failover, per-message choice,
SMTP, provider URL override, or browser/SDK provider surface.

## Configuration and initialization

Generated non-secret configuration has this shape:

```yaml
email:
  provider: postmark
  from: Tinkercloud <access@example.com>
  api_key: env:POSTMARK_SERVER_TOKEN
```

`email.provider` must be exactly `resend` or `postmark`. `email.api_key` must be
an `env:` reference whose value exists only in the root-owned systemd
credential file. The selected provider determines the generated reference:
`RESEND_API_KEY` or `POSTMARK_SERVER_TOKEN`.

The loader accepts the prior provider-less `email.resend_api_key` field only as
a compatibility form for Resend. A config containing both key fields, a legacy
key with Postmark, an unknown provider, a raw secret, or a missing selected key
denies.

New initialization uses `--email-provider` and
`--email-api-key-file`. The former defaults to `resend` for compatibility. The
deprecated `--resend-api-key-file` alias is accepted only when the selected
provider is Resend and the neutral file flag is absent. Secret values are never
accepted in argv.

## Adapter behavior

Both adapters accept the provider-neutral OTP message and return only success
or the fixed `email unavailable` error. They use a bounded HTTP client, a fixed
HTTPS production origin, JSON encoding, and no third-party package. Provider
credentials, recipients, codes, response bodies, message IDs, and upstream
error details never enter HTTP responses, diagnostics, logs, or audit records.

Resend uses bearer authorization and its provider schema. Postmark uses the
server-token header plus `From`, `To`, `Subject`, `TextBody`, and
`MessageStream: outbound` fields. Missing configuration, request construction,
transport failure, timeout, and any provider rejection return the same fixed
unavailable error.

## Diagnostics

`tinkercloud status` remains offline and never reads provider credentials.
Root-only `tinkercloud doctor` reads exactly the selected provider key plus the
required Tinkercloud secrets from the validated credential file and performs
one bounded, read-only check against that provider's compiled-in HTTPS API.
The result is named `email_provider`; output may say only `credential accepted`
or `unavailable` and must not include the provider, secret reference, path,
credential, account metadata, or response body.
