# Email Provider Denial Charter

- Reject an unknown, empty explicit, whitespace-padded, or case-variant
  provider before resolving credentials or composing an adapter.
- Reject a missing/raw provider credential reference, both neutral and legacy
  key fields, or a legacy Resend field paired with Postmark.
- Reject simultaneous neutral and legacy init key-file flags. Reject the legacy
  Resend flag when Postmark is selected. These denials happen before host,
  config, credential, database, service, or listener mutation.
- A missing credential or sender, transport error, timeout, non-success status,
  or provider rejection returns only the fixed unavailable error. It must not
  expose the key, recipient, code, endpoint response, message ID, or upstream
  detail.
- The Postmark adapter sends the credential only through
  `X-Postmark-Server-Token`, sends no bearer authorization, selects the fixed
  transactional stream, and never accepts an endpoint from config or message
  input.
- Status never reads a provider credential or invokes either provider. Doctor
  selects only the configured compiled-in provider check; an invalid credential
  file reaches no provider.
- Doctor output remains the generic `email_provider` check with fixed detail.
  Provider response bodies, account metadata, keys, references, paths, and
  provider-specific errors are discarded and never rendered.
- Provider failure cannot create an OTP bypass, alternate recovery surface, or
  session. Existing valid sessions may continue; new authentication remains
  fail-closed.
