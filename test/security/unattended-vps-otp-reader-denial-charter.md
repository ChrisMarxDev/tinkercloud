# Unattended VPS OTP reader denial charter

The local Resend reader is test infrastructure outside the Tinkercloud production
trust boundary. Before reporting unattended VPS acceptance ready, prove that:

- the reader is invoked only with `deployer|viewer EMAIL HOSTNAME`, validates
  the hostname against configured platform/app shape, and never accepts an app
  ID or viewer identity from a provider message;
- before reading a reader key or ledger or making a provider request, the
  local automation requires a single-line, normalized, valid DNS domain in
  `TINKERCLOUD_AUTOMATION_RECIPIENT_DOMAIN`; missing, empty, whitespace-padded,
  multiline, or malformed configuration denies;
- at that same early boundary, every recipient whose normalized domain is not
  exactly the configured automation recipient domain denies, including
  subdomains and suffix lookalikes; this guard does not alter normal
  Tinkercloud human login;
- a missing, relative, symlinked, non-regular, non-owner, or non-`0600` reader
  key or consumed-ID ledger fails before any provider request;
- the reader uses only the fixed HTTPS Resend origin; no environment, argument,
  DNS, redirect, or response field can select another endpoint;
- empty, malformed, non-2xx, oversized, or structurally ambiguous provider
  responses fail closed without emitting an OTP;
- a message must match exactly the requested recipient, configured sender,
  exact Tinkercloud subject, bounded recent window, and exact text form
  `Your code: NNNN...`; unrelated, stale, malformed, HTML-only, or multi-code
  messages are rejected;
- more than one eligible unconsumed message denies rather than guessing; a
  consumed message ID is never emitted again, including after restart;
- the reader writes only a private mode-`0600` opaque consumed-ID ledger and
  never writes the OTP, provider body, key, email content, or message ID to
  stdout, stderr, test reports, Git, or the VPS; and
- the VPS acceptance gate, exact target acknowledgement, strict pinned SSH
  trust, and reuse marker remain mandatory. The reader is never an OTP bypass:
  Tinkercloud still sends, verifies, and consumes a real OTP through its normal
  gateway flow.
