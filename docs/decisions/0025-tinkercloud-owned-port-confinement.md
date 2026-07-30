# ADR 0025: Confine Tinkercloud-owned network exposure to TCP 80 and 443

**Status:** Accepted for V1

## Context

Tinkercloud intentionally adds one Internet-facing gateway service to a dedicated
VPS. The operator still owns the host and cloud firewalls, SSH, and any
pre-existing service. Treating the whole machine's socket inventory as
Tinkercloud-owned would broaden the product into host firewall and SSH management.

The gateway nevertheless receives `CAP_NET_BIND_SERVICE`, and an unprivileged
process can bind high ports without that capability. A faulty or compromised
gateway therefore needs a process-level bind boundary in addition to its
application authorization boundary.

## Decision

Tinkercloud's production network-exposure delta is exactly TCP ports 80 and 443.
Production configuration rejects different HTTP or HTTPS ports.

The packaged and generated systemd service units retain exactly
`CAP_NET_BIND_SERVICE`, restrict socket address families to `AF_UNIX`,
`AF_INET`, and `AF_INET6`, deny all socket binds by default, and allow only TCP
80 and TCP 443. Unit validation treats a missing or broadened directive as
unsafe before starting the service.

VPS acceptance verifies both required listeners and rejects any additional
non-loopback listener owned by Tinkercloud. It does not reject operator-owned
listeners. Installation does not mutate firewall or SSH configuration.

## Consequences

- Application content, APIs, and realtime continue to share the HTTPS gateway;
  apps never receive individual ports.
- A gateway defect cannot open a UDP listener, another privileged port, or an
  unprivileged high port.
- Operators remain responsible for deciding which source networks may reach
  SSH and the gateway.
- Development configurations use the same port contract; real-listener tests
  continue to use injected listeners rather than production configuration
  ports.
