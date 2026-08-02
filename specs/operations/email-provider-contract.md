# Email Provider Contract

Tinkercloud sends numeric authentication codes through one environment-selected,
compiled-in outbound adapter. The choice changes only outbound email delivery;
it grants no authority and does not change challenge, identity, session, or
policy behavior.

## Supported providers

- `resend`: direct HTTPS JSON requests to the fixed Resend API.
- `postmark`: direct HTTPS JSON requests to the fixed Postmark API using the
  default transactional `outbound` message stream.
- `sendgrid`: direct HTTPS JSON requests to the fixed SendGrid v3 Mail Send API.
- `smtp`: direct send-only SMTP delivery over authenticated STARTTLS or
  implicit TLS. Tinkercloud never listens for or receives email.

Exactly one provider is active for each process. There is no per-message
choice, provider URL override, plaintext SMTP, inbound mail, or browser/SDK
provider surface.

## Configuration and initialization

Generated non-secret configuration has this shape:

```yaml
email:
  from: Tinkercloud <access@example.com>
```

The root-owned systemd environment file selects the adapter by a deterministic
first-present cascade:

1. non-empty `RESEND_API_KEY` selects Resend;
2. otherwise non-empty `POSTMARK_SERVER_TOKEN` selects Postmark;
3. otherwise non-empty `SENDGRID_API_KEY` selects SendGrid;
4. otherwise any SMTP variable selects SMTP, which requires all of
   `TINKERCLOUD_SMTP_HOST`, `TINKERCLOUD_SMTP_PORT`,
   `TINKERCLOUD_SMTP_USERNAME`, `TINKERCLOUD_SMTP_PASSWORD`, and
   `TINKERCLOUD_SMTP_TLS`.

Later provider variables are ignored after an earlier provider wins. The SMTP
TLS value must be exactly `starttls` or `tls`; plaintext delivery is rejected.
An absent provider, partial winning SMTP configuration, unsafe host or port, or
unknown TLS mode denies startup. Provider selection and credentials never
enter the non-secret config.

The loader accepts the prior `email.provider` plus `email.api_key` form and the
older provider-less `email.resend_api_key` form for compatibility. When none of
the canonical provider environment variables is present, that legacy config
resolves its referenced credential and retains its prior Resend or Postmark
selection. New configuration never writes either form.

Initialization uses `--email-provider` and `--email-api-key-file`. The former
accepts `resend`, `postmark`, `sendgrid`, or `smtp` and defaults to `resend` for
compatibility. SMTP additionally requires `--smtp-host`, `--smtp-port`,
`--smtp-username`, and `--smtp-tls`; the neutral key file supplies its password.
The deprecated `--resend-api-key-file` alias is accepted only when the selected
provider is Resend and the neutral file flag is absent. Secret values are never
accepted in argv.

## Adapter behavior

All adapters accept the provider-neutral OTP message and return only success
or the fixed `email unavailable` error. They use a bounded HTTP client, a fixed
HTTPS production origin or a bounded TLS SMTP connection, and no third-party
package. Provider credentials, recipients, codes, response bodies, message IDs, and upstream
error details never enter HTTP responses, diagnostics, logs, or audit records.

Resend uses bearer authorization and its provider schema. Postmark uses the
server-token header plus `From`, `To`, `Subject`, `TextBody`, and
`MessageStream: outbound` fields. SendGrid uses bearer authorization and the v3
Mail Send `personalizations`, `from`, `subject`, and plain-text `content`
schema. Missing configuration, request construction, transport failure,
timeout, and any provider rejection return the same fixed unavailable error.

SMTP parses the configured sender and recipient as mailbox addresses, validates
the server name and port, negotiates the selected TLS mode with certificate
verification, authenticates with `AUTH PLAIN`, and submits only one text OTP
message. It never accepts message-controlled server, credentials, TLS mode, or
envelope sender values.

## Diagnostics

`tinkercloud status` remains offline and never reads provider credentials.
Root-only `tinkercloud doctor` reads only allowlisted provider variables plus
the required Tinkercloud secrets from the validated credential file, applies
the same cascade, and performs one bounded, non-delivering check. HTTP adapters
use their compiled-in read-only HTTPS API; SMTP connects, negotiates TLS, and
authenticates without issuing `MAIL FROM` or `RCPT TO`.
The result is named `email_provider`; output may say only `credential accepted`
or `unavailable` and must not include the provider, secret reference, path,
credential, account metadata, or response body.
