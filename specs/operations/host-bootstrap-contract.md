# Workstation Host Operations Contract

The `tinker` binary may coordinate operator-owned host work without becoming a
second server or a remote control plane.

## Boundary

- `tinker host ...` is a workstation-side SSH transport for a fixed command
  grammar. Root authority comes from the operator's SSH authentication and the
  target host, never from a deployer bearer.
- Host operations accept only an explicit `root@HOST` target. SSH options,
  proxy commands, shell fragments, environment assignments, and leading-dash
  targets are invalid.
- OpenSSH performs normal known-host verification. Tinker does not add
  `StrictHostKeyChecking=no`, accept a new key automatically, or edit
  `known_hosts`.
- The remote command allowlist is `install`, `status`, `doctor`, and `update`.
  There is no arbitrary `exec` escape hatch.
- `status` and `doctor` call the installed local `tinkercloud` operations surface.
  `update` passes only a validated HTTPS release directory to the local,
  signed, rollback-capable updater.
- `install` sends the reviewed embedded bootstrap on standard input and invokes
  it with a validated HTTPS release directory. The bootstrap never contains a
  signing private key.

## Failure behavior

- Target, URL, SSH startup, host-key, authentication, remote verification,
  install, update, restart, doctor, public-health, or denial-probe failure is a
  non-zero result.
- Output is bounded and may contain the remote command's safe diagnostics. It
  must not contain a deployer bearer, signing key, provider credential, or
  arbitrary remote response body.
- An interrupted install never reports success. An interrupted update retains
  the server updater's rollback state and recovery behavior.
