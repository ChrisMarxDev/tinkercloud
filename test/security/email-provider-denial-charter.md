# Email Provider Denial Charter

- Use the exact first-present environment cascade Resend, then Postmark, then
  SendGrid, then SMTP. Later configured providers never change the selected
  adapter.
- Reject an absent provider. Reject partial SMTP configuration when SMTP is the
  first available candidate, plus invalid host, port, or TLS mode, before
  composing an adapter.
- Reject a missing/raw provider credential reference, both neutral and legacy
  key fields, or a legacy Resend field paired with Postmark.
- Reject simultaneous neutral and legacy init key-file flags. Reject the legacy
  Resend flag when Postmark or SMTP is selected. Reject SMTP flags for an HTTP
  provider and missing SMTP flags for SMTP. These denials happen before host,
  config, credential, database, service, or listener mutation.
- A missing credential or sender, transport error, timeout, non-success status,
  or provider rejection returns only the fixed unavailable error. It must not
  expose the key, recipient, code, endpoint response, message ID, or upstream
  detail.
- The Postmark adapter sends the credential only through
  `X-Postmark-Server-Token`, sends no bearer authorization, selects the fixed
  transactional stream, and never accepts an endpoint from config or message
  input.
- The SendGrid adapter sends the credential only through bearer authorization,
  uses the fixed v3 Mail Send endpoint, emits exactly one personalization and
  one plain-text content part, and never accepts an endpoint from config or
  message input.
- The SMTP adapter never permits plaintext transport, never skips certificate
  verification, never accepts a server or envelope sender from the OTP message,
  and never logs an SMTP reply. Header newlines, invalid mailboxes, connection,
  TLS, authentication, envelope, and DATA failures return only the fixed
  unavailable error.
- Status never reads provider variables or invokes a provider. Doctor applies
  the same cascade and checks only the selected compiled-in adapter; an invalid credential
  file reaches no provider.
- Doctor output remains the generic `email_provider` check with fixed detail.
  Provider response bodies, account metadata, keys, references, paths, and
  provider-specific errors are discarded and never rendered.
- Provider failure cannot create an OTP bypass, alternate recovery surface, or
  session. Existing valid sessions may continue; new authentication remains
  fail-closed.
