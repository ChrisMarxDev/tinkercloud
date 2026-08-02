# ADR 0057: Support Resend, Postmark, SendGrid, and send-only SMTP

**Status:** Accepted for V1

## Context

Tinkercloud sends authentication codes through a provider-neutral outbox, but
the shipped composition, configuration, initialization, diagnostics, and
operator guidance select only Resend. An operator therefore has no supported
alternative when Resend is unavailable, unsuitable for their organization, or
already outside their approved vendor set.

The candidates below were compared using current first-party product and API
documentation. Distribution is directional because vendors publish different
metrics; it is not a market-share measurement.

| Provider | Distribution | Implementation effort | Notes |
|---|---|---|---|
| Resend | Very broad and fast-growing; [one million users and tens of millions of daily messages](https://resend.com/blog/1-million-users) | Already shipped | Developer-focused baseline with a small JSON API. |
| SendGrid | Very broad; [more than 200 billion messages per month](https://www.twilio.com/en-us/sendgrid) | Low | Mature JSON API, but a more nested message schema and a broader marketing platform. |
| Brevo | Very broad; [600,000 customers globally](https://www.brevo.com/company/about/) | Low | Simple API and strong European presence, but broader CRM and marketing scope than OTP delivery needs. |
| Mailgun | Very broad; [more than 150,000 businesses and 650 billion messages per year](https://www.mailgun.com/enterprise/) | Medium | Mature developer platform; multipart requests and selectable US/EU API origins add configuration. |
| Amazon SES | Very broad within AWS; [used by global, high-volume customers](https://aws.amazon.com/ses/customers/) | Medium to high | Cost-effective and proven, but AWS regions, IAM, sandbox rules, and Signature Version 4 expand setup and code. |
| Postmark | Broad and mature; [transactional email since 2010 with billions of messages sent](https://www.activecampaign.com/platform/transactional-email) | Low | Purpose-built for transactional mail with one fixed JSON endpoint and a server-scoped token. |

Postmark is the smallest useful second HTTP provider. Its documented send operation
uses one fixed HTTPS endpoint, a JSON message, and an
`X-Postmark-Server-Token` header. It also provides a test token that validates
requests without delivery. Adding it needs no production dependency, new
listener, persistence system, or browser-visible capability.

The operator also needs a provider-neutral escape hatch for established mail
services and self-managed relays. SMTP submission is the common denominator,
but it is safe here only as outbound authenticated delivery with mandatory TLS;
Tinkercloud has no reason to receive mail or run a mail server.

## Decision

Support `resend`, `postmark`, `sendgrid`, and `smtp` as compiled-in email providers.

- Non-secret config names the sender only. The root-owned systemd environment
  selects the adapter by first-present priority: `RESEND_API_KEY`, then
  `POSTMARK_SERVER_TOKEN`, then `SENDGRID_API_KEY`, then the complete
  `TINKERCLOUD_SMTP_*` set. Once a provider wins, later variables are ignored.
  Existing `email.provider` plus
  `email.api_key` and older `email.resend_api_key` configs retain a fallback
  compatibility path only when no canonical provider variable exists.
- `tinkercloud init` accepts `--email-provider` and
  `--email-api-key-file`. The previous `--resend-api-key-file` remains a
  Resend-only compatibility alias; ambiguous or non-Resend use denies before
  host mutation.
- The composition root selects one direct standard-library adapter.
  Resend retains `POST https://api.resend.com/emails`. Postmark uses
  `POST https://api.postmarkapp.com/email`, the server-token header, and the
  transactional `outbound` message stream described by the
  [Postmark send API](https://postmarkapp.com/developer/user-guide/send-email-with-api).
- SendGrid uses `POST https://api.sendgrid.com/v3/mail/send`, bearer
  authorization, one personalization, and one plain-text content part as
  documented by the [Mail Send API](https://www.twilio.com/docs/sendgrid/api-reference/mail-send/mail-send).
- SMTP uses the Go standard library to connect to the operator-owned host and
  port, requires either certificate-verified STARTTLS or implicit TLS,
  authenticates with the configured username and password, and submits one
  outbound text message. Plaintext SMTP and inbound mail are unsupported.
- Doctor performs one bounded, non-delivering credential check for the selected
  provider. HTTP providers use a compiled-in read-only HTTPS endpoint; SMTP
  negotiates TLS and authenticates without creating an envelope. Postmark uses
  its aggregate sent-count statistics endpoint rather than a server-details
  response that includes credential metadata. Doctor emits the generic
  `email_provider` check name and never reads or reports a provider response
  body.
- There is no failure-based failover or per-message selection. Cascade priority
  is evaluated once from process environment; a selected provider failure
  leaves new authentication unavailable while existing valid sessions continue.

## Consequences

- Operators can choose between three established transactional-email APIs or an
  existing authenticated SMTP submission service without changing
  Tinkercloud's authentication or trust boundary.
- Provider selection and credentials remain operator-owned, local, and
  explicit. App code, viewers, deployers, and the SDK cannot observe or choose
  them.
- Existing Resend installations and the Resend-based unattended acceptance
  reader remain supported.
- Adding another provider still requires an explicit contract, fixed endpoint,
  bounded diagnostics, negative evidence, and operator documentation; this is
  not a generic HTTP-provider plugin system.
