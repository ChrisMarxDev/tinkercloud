# ADR 0028: Operator-supplied TLS for the first VPN-only topology

**Status:** Accepted direction after V1; not yet implemented or supported

## Context

Strict VPN-only ingress prevents the public HTTP-01 validation used by TinyHost
V1. TinyHost still requires HTTPS: the VPN is an additional network boundary,
not a replacement for hostname authentication, secure browser sessions, or the
gateway authorization boundary.

DNS-01 automation would require DNS-provider integrations and credentials.
Operating a private certificate authority would add another security system.
Both conflict with the goal of providing one deliberately narrow first
VPN-only setup. Operators needing either model can use a separately designed
deployment.

## Decision

The first supported VPN-only topology will use one operator-supplied TLS
certificate and matching private key. The certificate must cover both the
configured platform hostname and the wildcard application hostname. For
example, it may contain names for `tiny.example.com` and
`*.apps.example.com`.

The guided setup flow will accept only absolute root-readable file paths, never
private-key bytes in command arguments or echoed prompts. Before installation,
TinyHost will reject a malformed, expired, not-yet-valid, hostname-mismatched,
untrusted, over-permissive, symlinked, or certificate/key-mismatched input.

The operator owns obtaining the certificate, keeping its trust chain valid for
company devices, and renewing it before expiry. TinyHost owns protected
installation, TLS termination, expiry diagnostics, and refusing to report
setup or deployment success when trusted-network HTTPS or anonymous-denial
verification fails.

VPN membership grants no TinyHost identity and bypasses no app authentication
or current policy. The first VPN-only mode retains TinyHost email OTP login and
per-app authorization exactly as the public topology does. Central SSO is a
separate later decision, not a prerequisite for VPN support.

## Consequences

- TinyHost needs no DNS-provider credential or private-CA implementation for
  its first VPN-only mode.
- Private or split DNS must resolve the platform hostname and wildcard app
  suffix to the VPN-reachable server.
- Setup needs a VPN-reachable trusted-network probe; the existing public V1
  proof must not be disabled or silently reused.
- Renewal is an explicit operator action. TinyHost must provide a narrow,
  atomic certificate replacement path plus expiry diagnostics before claiming
  support.
- One certificate/key pair is the intentionally narrow initial contract.
  Operators requiring multiple certificates, automated DNS-01, an internal CA
  workflow, or external TLS termination need a separately designed setup.
- This decision does not add VPN-only support to V1. Support begins only after
  its contract, deny charter, implementation, and VPN acceptance suite pass.

## Reconsider when

Repeated use shows that manual renewal causes operational failures, or a small
DNS-provider integration would serve most VPN-only operators without weakening
secret ownership.
