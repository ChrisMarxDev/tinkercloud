# ADR 0050: Local deployer-workstation OTP automation is not CI credentialing

**Status:** Accepted for V1 test infrastructure

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

This wrapper is explicitly not production CI/noninteractive deployment-agent
authentication. That authority continues to require a separately provisioned
app-scoped deployer token. The wrapper neither creates nor scopes such a token.

## Consequences

- Controlled local tests can reuse the real login and deployment boundaries
  without an auth bypass.
- A malformed identity, provider/reader failure, compatibility error, unsafe
  path, timeout, or incomplete deploy proof stops the one attempt.
- Production automation does not receive a human's implicit local credential
  write or interactive OTP path.
