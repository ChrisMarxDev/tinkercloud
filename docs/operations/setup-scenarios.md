# Tinkercloud setup scenarios

This document separates the network topology Tinkercloud supports in V1 from
topologies that are secure in principle but still need product work.

## Support matrix

| Scenario | V1 status | Security verdict | Main condition |
|---|---|---|---|
| Solo operator or startup on a public Hetzner VPS | Canonical V1 topology | Secure when V1 gates and operator duties are met | Public DNS and inbound TCP 80/443 |
| Company with VPN-only ingress | Accepted direction, not yet implemented | Secure when the company supplies TLS and all VPN acceptance gates pass | One certificate covering the admin and app hosts, private DNS, and trusted-network probes |

The VPN is an additional network boundary. It does not replace Tinkercloud's
per-app policy, viewer authentication, session, or authorization checks.

## Implemented guided setup experience

The operator runs `sudo tinkercloud setup` on a fresh supported machine. The
assistant asks for the root domain, initial operator email, and verified sender,
then derives `admin.<domain>`, `<slug>.<domain>`, and the ACME contact. It asks
only for values it cannot discover or safely default:

1. the controlled root domain;
2. the initial operator email;
3. the verified Resend sender and credential source; and
4. for the future VPN-only mode, the certificate and private-key credential
   sources.

The internal ACME contact is the normalized operator email in public mode; no
separate flag, question, or environment input can override it. Tinkercloud handles
host checks, service identity, directories, configuration, credentials,
database initialization, systemd installation, startup, and final security
verification. It reports one actionable prerequisite when DNS, email,
certificate, VPN reachability, or an operator-owned firewall prevents
completion. It persists validated non-secret progress and resumes without
asking for prior valid answers. Generated config is a durable receipt and
automation interface, not prerequisite paperwork. It does not take ownership
of SSH or firewall policy.

## Scenario 1: public VPS for a solo operator or startup

### Intended use

A technical person or small startup buys a dedicated supported Hetzner VPS,
prepares DNS and email, installs Tinkercloud, and deploys private utility apps.
Viewers reach the app over the Internet and authenticate through Tinkercloud.

```text
Internet
  → operator-owned firewall
  → Tinkercloud TCP 80/443
  → hostname routing
  → app session and current policy
  → private app content/capability
```

### Before the ten-minute setup clock

The PRD's setup target starts only after these external prerequisites exist:

1. A supported Ubuntu 24.04 LTS or 26.04 LTS x86-64 VPS.
2. Root SSH access with a pinned host key.
3. Public platform and wildcard app DNS records pointing at the VPS.
4. A verified Resend or Postmark sender and root-readable API-key file.
5. An operator decision about the host/cloud firewall and SSH source rules.

DNS propagation, buying the server, verifying the sending domain, and choosing
firewall policy are not part of the ten-minute Tinkercloud initialization target.

### Tinkercloud setup

```text
verify and install the signed tinkercloud binary
→ run tinkercloud setup
→ discover host state; ask base domain + operator email
→ show exact DNS/email-provider actions and resume after completion
→ generate the service identity, config, credentials, SQLite, and operator
→ confirm `admin.<domain>` and a synthetic one-label app hostname resolve
  before starting the ACME-capable service
→ obtain the platform certificate through public HTTP-01
→ prove the public HTTPS gateway
→ reconcile the active-deployer allowlist
→ tinker login
→ tinker deploy
→ prove anonymous denial before reporting success
```

The deterministic automation contract remains
`tinkercloud init --non-interactive`. The human product path is the implemented
`tinkercloud setup` assistant in the
[complete operator flow](../../concept/flows/operator.html); it calls the same
validation/domain services and generates the explicit automation state.

### Network exposure

- Tinkercloud adds only TCP 80 and 443.
- Port 80 serves ACME HTTP-01 and safe redirects or denials.
- Port 443 serves the platform and app gateway.
- Login, OTP request, and version routes are intentionally reachable before
  authentication but have no private app-content dependency.
- Apps receive no individual port, process, raw storage URL, or direct backend.
- Tinkercloud does not change SSH or firewall policy.

### Security ownership

Tinkercloud owns:

- TLS termination and validated hostname routing;
- operator, deployer, and viewer authentication boundaries;
- current per-app authorization;
- private static files, KV, and realtime dispatch;
- process-level confinement to TCP 80/443; and
- signed updates, rollback, diagnostics, and denial probes.

The operator owns:

- SSH keys, SSH source restrictions, and root access;
- host and cloud firewall policy;
- Ubuntu security updates and VPS lifecycle;
- DNS and email-provider account security; and
- recovery from total VPS loss, because V1 has no backup guarantee.

### Security verdict

This topology can be secure for V1's replaceable toy, prototype, and utility
apps. It is not suitable for business-critical durability. Internet users can
reach the gateway, but private app bytes and capabilities remain behind
Tinkercloud authentication and current policy.

## Scenario 2: company with VPN-only ingress

### Intended use

The company permits access to the Tinkercloud server only from its VPN. Tinkercloud
still provides per-app viewer authorization inside that network.

```text
Company device
  → company VPN
  → operator-owned firewall
  → Tinkercloud TCP 80/443
  → Tinkercloud app authentication and policy
```

This is a sound defense-in-depth topology. VPN membership must not become a
browser-controlled identity or bypass Tinkercloud authorization. Users still log
in to Tinkercloud with an email one-time code and receive access only to apps whose
current policy allows them.

### What already works

- Tinkercloud can listen on the same TCP 80/443 gateway behind a firewall.
- Hostname routing, cookies, app isolation, KV, and WebSockets are compatible
  with a VPN.
- the selected Resend or Postmark provider can work when the server retains
  outbound HTTPS access.
- Tinkercloud does not need to understand VPN users, routes, or credentials.

### What blocks V1 support

1. V1 certificate issuance uses public ACME HTTP-01. A strict VPN-only port 80
   cannot be reached by the certificate authority.
2. Initialization requires a verified HTTPS proof for the configured public
   derived admin hostname.
3. Deployment success requires gateway denial probes from a network path that
   can reach the app hostname.
4. V1 has no implementation for installing and serving an operator-supplied
   VPN certificate.
5. The external VPS acceptance suite does not exercise a VPN-restricted route.

Opening port 80 temporarily for renewals is not an accepted solution: renewal
would depend on an undocumented operator schedule and fail unpredictably.
Skipping TLS validation or denial probes is also forbidden.

### Accepted simplest certificate model

The first VPN-only setup will ask for one company-provided certificate and its
matching private key. That certificate must cover the admin and app hostnames,
such as `admin.example.com` and `*.example.com`.

The company obtains and renews the certificate. Tinkercloud validates it, protects
the private key, serves HTTPS, warns about expiry, and refuses setup or
deployment success when VPN-reachable TLS and anonymous-denial checks fail.
Tinkercloud will not add DNS-provider automation or operate a private certificate
authority for this first mode.

This is an accepted direction, not a claim that VPN-only setup works in the
current binary.

### Work required for secure support

Before claiming VPN-only support:

1. Implement ADR
   [0028](../decisions/0028-operator-supplied-tls-for-vpn-only.md): securely
   install and atomically replace one operator-supplied certificate/key pair.
2. Define split/private DNS behavior for the single root-domain wildcard.
3. Replace the public proof with a trusted-network proof that keeps exact TLS,
   hostname, response, and anonymous-denial validation.
4. Add VPN-topology install, renewal, deploy, revocation, and failure tests.
5. Keep Tinkercloud app authentication enabled; VPN membership alone grants no
   app identity or access.

### Security verdict

VPN-only can be secure, but it is not a supported V1 installation today. The
secure behavior is to stop setup when certificates or verification cannot
complete, not to weaken the gateway.

## Current open setup issues

1. Maintain regression coverage for the guided `tinkercloud setup` assistant.
2. Benchmark and document the smallest recommended Hetzner plan.
3. Implement the accepted operator-supplied VPN certificate and
   trusted-network verification model.
4. Re-run clean-host install and failed-update rollback acceptance after the
   port-confinement change.
5. Complete the planned operator CPU/RAM/storage overview.
