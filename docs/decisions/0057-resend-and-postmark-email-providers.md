# ADR 0057: Support Resend and Postmark email providers

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

Postmark is the smallest useful second provider. Its documented send operation
uses one fixed HTTPS endpoint, a JSON message, and an
`X-Postmark-Server-Token` header. It also provides a test token that validates
requests without delivery. Adding it needs no production dependency, new
listener, persistence system, or browser-visible capability.

## Decision

Support exactly `resend` and `postmark` as compiled-in email providers.

- Non-secret config names one active `email.provider`, one sender, and one
  provider credential reference. Generated config always uses the neutral
  `email.api_key` field. Existing config containing only
  `email.resend_api_key` is read as Resend and is rewritten in the neutral form
  when Tinkercloud next generates config.
- `tinkercloud init` accepts `--email-provider` and
  `--email-api-key-file`. The previous `--resend-api-key-file` remains a
  Resend-only compatibility alias; ambiguous or Postmark use denies before
  host mutation.
- The composition root selects one direct standard-library HTTP adapter.
  Resend retains `POST https://api.resend.com/emails`. Postmark uses
  `POST https://api.postmarkapp.com/email`, the server-token header, and the
  transactional `outbound` message stream described by the
  [Postmark send API](https://postmarkapp.com/developer/user-guide/send-email-with-api).
- Doctor performs one bounded, read-only credential check for the selected
  provider against a compiled-in HTTPS endpoint. Postmark uses its aggregate
  sent-count statistics endpoint rather than a server-details response that
  includes credential metadata. Doctor emits the generic
  `email_provider` check name and never reads or reports a provider response
  body.
- There is no provider failover, fallback, runtime endpoint, per-message
  selection, SMTP path, or multi-provider credential inventory. A selected
  provider failure leaves new authentication unavailable while existing valid
  sessions continue.

## Consequences

- Operators can choose between two established transactional-email services
  without changing Tinkercloud's authentication or trust boundary.
- Provider selection and credentials remain operator-owned, local, and
  explicit. App code, viewers, deployers, and the SDK cannot observe or choose
  them.
- Existing Resend installations and the Resend-based unattended acceptance
  reader remain supported.
- Adding another provider still requires an explicit contract, fixed endpoint,
  bounded diagnostics, negative evidence, and operator documentation; this is
  not a generic HTTP-provider plugin system.
