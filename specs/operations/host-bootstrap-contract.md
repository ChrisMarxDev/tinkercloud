# Workstation Host Operations Contract

The `tinker` binary may coordinate operator-owned host work without becoming a
second server or a remote control plane.

## Boundary

- The canonical clean-host installation begins in the fresh supported VPS root
  shell. The operator runs the exact-version GitHub command
  `curl --proto '=https' --proto-redir '=https' --tlsv1.2 -fsSL https://github.com/ChrisMarxDev/tinkercloud/releases/download/vVERSION/install-host.sh | sh`.
  The released `install-host.sh` contains that exact immutable release directory
  and accepts no release-base argument. Repository/development copies alone may
  accept one validated explicit release directory for offline tests.
- The root installer refuses a non-root caller, non-Linux host, unsupported
  Ubuntu release, non-x86-64 architecture, existing installation, malformed or
  credentialed origin, and every missing/invalid checksum, metadata, or Ed25519
  sidecar before replacing either installed file. It never accepts signing or
  provider secrets.
- Release asset transport may follow redirects only with curl's
  `--location --proto '=https' --proto-redir '=https'` policy and bounded
  downloads. A redirect is transport only, never a trust decision: checksum
  and pinned Ed25519 verification of each downloaded installer input complete
  before host mutation. HTTP downgrade, redirect loop, transport failure,
  unsigned/tampered bytes, and origin validation failure are non-zero.

- `tinker host ...` is a workstation-side SSH transport for a fixed command
  grammar. Root authority comes from the operator's SSH authentication and the
  target host, never from a deployer bearer.
- Host operations accept only an explicit `root@HOST` target. SSH options,
  proxy commands, shell fragments, environment assignments, and leading-dash
  targets are invalid.
- OpenSSH performs normal known-host verification. Tinker does not add
  `StrictHostKeyChecking=no`, accept a new key automatically, or edit
  `known_hosts`.
- The remote command allowlist is `install`, `status`, `doctor`, `update`, and
  `uninstall`. There is no arbitrary `exec` escape hatch.
- `status` and `doctor` call the installed local `tinkercloud` operations surface.
  `update` passes only a validated HTTPS release directory to the local,
  signed, rollback-capable updater.
- `install` sends the reviewed embedded bootstrap on standard input and invokes
  it with a validated HTTPS release directory. This optional workstation flow
  retains explicit release-base support for development and compatibility. The
  bootstrap never contains a signing private key and follows only HTTPS
  redirects before it verifies the signed root installer.
- A released `tinker` binary derives the exact immutable GitHub release
  directory from its strict semantic build version for `host install`; a
  development build must receive an explicit validated `--release-base`.
  This installs the signed binaries and service unit only. Host initialization
  remains separate: `init --non-interactive` is implemented today, while the
  minimum-question `tinkercloud setup` assistant remains planned.
- `uninstall` requires a deliberate workstation confirmation of the exact
  target, or the deterministic explicit `--yes` acknowledgement for a
  non-interactive operator workflow. It invokes only
  `tinkercloud uninstall --confirm-uninstall` remotely.

## Uninstall boundary and deny charter

- Root-local uninstall accepts only the exact `--confirm-uninstall` argument
  and only operates on the canonical fixed installation paths:
  `/etc/tinkercloud`, `/var/lib/tinkercloud`,
  `/etc/systemd/system/tinkercloud.service`, and
  `/usr/local/bin/tinkercloud`. A missing or malformed canonical config is
  tolerated only when no config is present; a present config must name the
  canonical data directory and ACME cache directory. Custom, symlinked,
  non-regular, or otherwise ambiguous state denies before service mutation.
- On success it stops and disables the fixed systemd unit, removes the fixed
  config/credentials directory, application state, unit, binary, and service
  identity, then reloads systemd. It never removes or rewrites
  `/var/lib/tinkercloud-acme`; preserving that configured cache avoids
  unnecessary certificate issuance on a later reinstall. The service identity
  is intentionally removed too: the next canonical setup recreates it before
  recursively re-owning both the new data directory and preserved ACME cache.
- Before stopping the service, uninstall parses Linux `/proc/self/mountinfo`
  and denies if `/etc/tinkercloud`, `/var/lib/tinkercloud`, or any path below
  either is a mount point. It therefore never recursively deletes across a
  filesystem boundary.
- JSON output, arbitrary SSH options, shell fragments, arbitrary paths,
  arbitrary remote commands, omitted acknowledgement, malformed target,
  failed host-key authentication, service-stop failure, removal failure, and
  service-identity failure are all non-zero. No uninstall path prints provider
  credentials, certificate keys, deployer bearers, or arbitrary remote output.

## Failure behavior

- Target, URL, SSH startup, host-key, authentication, remote verification,
  install, update, restart, doctor, public-health, or denial-probe failure is a
  non-zero result.
- Output is bounded and may contain the remote command's safe diagnostics. It
  must not contain a deployer bearer, signing key, provider credential, or
  arbitrary remote response body.
- An interrupted install never reports success. An interrupted update retains
  the server updater's rollback state and recovery behavior.

## Root installer deny-path test charter

- Prove root-only, supported Ubuntu/x86-64, clean-host, and exact-argument
  checks deny before any `install`, `mv`, or `systemctl` call.
- Prove malformed, insecure, credentialed, private-address, and development
  origin inputs deny; prove a released installer rejects a caller-supplied base.
- Prove HTTPS redirect transport succeeds only when the resulting server,
  service unit, metadata, signatures, and checksum evidence verify; prove an
  HTTP downgrade, loop/transport failure, metadata/signature whitespace, or
  tampered payload denies before mutation.
- Prove the release build embeds `vVERSION`'s exact GitHub directory in
  `install-host.sh` and rejects any staged installer still containing the source
  placeholder.
