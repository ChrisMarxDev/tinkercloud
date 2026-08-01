# ADR 0004: Per-host ACME before wildcard automation

**Status:** Accepted for V1; validate implementation details during architecture
spike

## Context

Wildcard certificates commonly require DNS-provider APIs. Requiring those
credentials undermines the simple install promise. HTTP challenge certificates
can be issued for the platform and individual app hostnames through the public
gateway, but introduce issuance latency and CA rate-limit concerns.

## Decision

Use per-host automatic ACME certificates for the first release. Provision the
app hostname during deployment and do not mark it ready until HTTPS succeeds.
Reuse stable app hostnames across releases.

## Spike questions

- Does the chosen ACME implementation safely coordinate concurrent first hits?
- How are issuance and renewal bounded so public requests cannot cause abuse?
- What operator diagnostics and retry state are required?
- What practical app-creation rate is safe under CA limits?
- Is a manually supplied wildcard certificate needed as an escape hatch?

## Reconsider when

Operators need large app counts or fast bulk creation, or a small set of DNS
provider adapters covers the target audience.

## Deployment-topology consequence

Public HTTP-01 makes the public dedicated VPS the canonical V1 topology. A
strict VPN-only server cannot complete certificate issuance and renewal under
this decision. Supporting it requires a separate accepted certificate and
trusted-network verification model; setup must not work around the gap by
disabling TLS or public-denial evidence. ADR
[0028](0028-operator-supplied-tls-for-vpn-only.md) selects one
operator-supplied certificate/key pair as the deliberately narrow first
post-V1 VPN-only model.
