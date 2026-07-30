# Distribution Preparation Contract

This contract covers internal package-manager preparation only. Its commands do
not publish a package, create a hosted release, change a package-manager tap, or
claim that a locked public identity is registered or available. The separately
authorized GitHub prerelease channel is governed by
[`beta-release-contract.md`](beta-release-contract.md).

## One source release

- One version produces the `tinkercloud` Linux server, the `tinker` workstation CLI
  platform matrix, the TypeScript SDK tarball, signed metadata, checksums,
  provenance, and a signed release manifest.
- The version must be strict `MAJOR.MINOR.PATCH` and must equal the SDK package,
  JSR, runtime export, CLI, server, npm-wrapper, and Homebrew candidate version.
- `release-manifest.json` schema 2 binds every evidence file and one explicit
  compatibility matrix. The existing artifact metadata/signature bytes remain
  V1-compatible.
- A preparation check verifies the signed release before generating any
  package-manager input. Generated packages contain only files from that
  verified directory plus reviewed repository templates.

## Workstation CLI channels

- The canonical executable is the native `tinker` binary.
- The npm package contains all supported CLI binaries and a dependency-free
  launcher. npm, pnpm, Yarn, and Bun therefore install the same bytes and do not
  run lifecycle download scripts.
- The Homebrew formula selects one signed macOS artifact from the verified
  release and records its SHA-256. Formula generation does not update a tap.
- The reviewed one-line installer downloads the selected native CLI artifact
  over HTTPS, verifies its release checksum, exact metadata, and Ed25519
  signature, then atomically replaces the local executable.
- JSR remains an SDK channel. It is not used as an executable installer.

## Server channel

- `tinkercloud` is never installed by a JavaScript package manager.
- A root host bootstrap downloads only the Linux/amd64 server artifact and its
  verification inputs from one HTTPS release origin, verifies them before
  replacement, installs the reviewed service unit, and leaves initialization
  as an explicit operator action.
- `tinker host install` transports the reviewed bootstrap bundled into the
  installed CLI over an operator-authenticated SSH connection. It does not
  accept an arbitrary remote command or weaken host-key verification.
- Manual `tinkercloud update` remains the only V1 update action. Preparing
  distribution does not enable scheduled or silent updates.

## Naming and publication boundary

Product and distribution identities are locked by
[`ADR 0052`](../../docs/decisions/0052-tinkercloud-product-identity.md).
Preparation uses those canonical names, while a release base remains a
versioned GitHub URL until a separate official-domain decision is accepted.
Preparation MUST NOT:

- publish to npm, JSR, Homebrew, GitHub Releases, or another registry;
- create or mutate a public tap, release channel, DNS name, or install URL;
- infer that a configured public name is registered or available;
- write production credentials or signing keys into an artifact.
