# ADR 0050: Local deployer-workstation OTP automation is not CI credentialing

**Status:** Accepted for V1 test infrastructure; recipient-domain detail superseded by ADR 0056

## Context

Some controlled local test workflows need exactly one deployment while proving
the ordinary deployer CLI identity. The existing local Resend reader can obtain
one exact OTP without moving the reader key or consumed-message ledger to a
VPS. Treating that convenience as general noninteractive deployment-agent
authentication would silently turn a human credential flow into CI authority.

## Decision

Provide an opt-in, local-workstation-only wrapper that first validates the
saved CLI identity with `tinker whoami --json`, and otherwise drives exactly one
normal `tinker login --force` through the local Resend reader. It accepts only an
exact requested deployer identity, then performs exactly one normal `tinker
deploy --json`; the CLI's public anonymous-denial verification remains the
release proof. It receives only the CLI path, HTTPS server, deployer email, and
app directory as arguments. Keys, OTPs, and bearers never enter arguments,
output, project files, or a VPS.

Before any CLI/app path check, saved-credential inspection, reader/key access,
or network action, the wrapper requires the mandatory local configuration
defined by [ADR 0056](0056-configurable-local-automation-recipient-domain.md),
normalizes the requested deployer email, and requires its domain to equal that
configuration exactly. It rejects subdomains, suffix lookalikes, the hyphenless
domain, and unrelated domains. This restriction is only on the opt-in local
wrapper, not on direct CLI use, saved CLI credentials, or normal Tinkercloud
human login.

This wrapper is explicitly not production CI/noninteractive deployment-agent
authentication. That authority continues to require a separately provisioned
app-scoped deployer token. The wrapper neither creates nor scopes such a token.

## Consequences

- Controlled local tests can reuse the real login and deployment boundaries
  without an auth bypass.
- A malformed identity, provider/reader failure, compatibility error, unsafe
  path, timeout, incomplete deploy proof, or disallowed requested deployer
  domain stops the one attempt before any credential reuse or network action.
- Production automation does not receive a human's implicit local credential
  write or interactive OTP path.
