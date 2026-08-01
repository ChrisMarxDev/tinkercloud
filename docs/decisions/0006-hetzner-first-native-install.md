# ADR 0006: Hetzner-first native install and signed self-update

**Status:** Accepted

**Amended:** 2026-07-27

## Context

The initial operator provisions a clean VPS whose only purpose is Tinkercloud.
Optimizing for every Linux environment, container platform, and reverse-proxy
topology would make setup and diagnosis less deterministic.

Canonical recommends LTS releases for long-term stability and gives interim
releases only nine months of updates. Ubuntu 24.04 LTS remains under standard
security maintenance through 2029, while Ubuntu 26.04 LTS remains under
standard security maintenance through 2031. See the
[Ubuntu release cycle](https://ubuntu.com/about/release-cycle).

## Decision

Target clean Hetzner Cloud Ubuntu 24.04 LTS and Ubuntu 26.04 LTS x86-64 VPS
instances with systemd first. These are an exact allowlist, not the endpoints
of a numeric range. Interim, end-of-life, malformed, and future unverified
Ubuntu releases fail preflight before mutation.

Installation requires `sudo`, creates a dedicated unprivileged service account,
installs one `tinkercloud` binary and one systemd unit, and owns ports 80/443
directly.

Provide:

```text
tinkercloud init
tinkercloud status
tinkercloud doctor
tinkercloud update
tinkercloud recover operator
```

`tinkercloud update` downloads a signed compatible release, records bounded local
rollback state, restarts the service, runs a health gate, and restores the prior
version when the gate fails. Automatic unattended update is deferred; the
operator invokes or schedules the command.

Root access to the VPS is the ultimate recovery authority. Email is not required
for local operator recovery.

## Consequences

- Installation and support can assume a known service manager and clean network
  topology.
- Operators can use either maintained Ubuntu LTS available in the V1 support
  window without accepting short-lived interim releases.
- Adding another release requires explicit contract, negative preflight, unit,
  systemd, and external VPS acceptance evidence; a numeric range is not support
  evidence.
- The service runs unprivileged after privileged installation/binding setup.
- Other Linux distributions and Docker remain feasible because the runtime is a
  self-contained binary with explicit paths and signals.
- Docker packaging later wraps `tinkercloud`; it does not become an internal
  dependency or the primary installation.
