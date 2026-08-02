# Release pipeline

Tinkercloud releases are built locally or in a controlled CI environment.
Preparation commands never publish a package or execute a candidate binary.
The explicitly dispatched beta workflow is the only pre-stable hosted-release
mutation path. The release authority is an Ed25519 private key held outside
this repository. The matching public key is committed in
`packaging/release-public-key.pem` and compiled into the Linux `tinkercloud`
binary.

## Build a candidate

Generate or obtain the signing key through the operator's release-key process,
then make its directory private and its file readable only to the release
account. Do not place it in the checkout, pass its contents on a command line,
copy it to the VPS, or print it.

```sh
umask 077
install -d -m 700 "$HOME/.tinkercloud/release"
export TINKERCLOUD_RELEASE_SIGNING_KEY="$HOME/.tinkercloud/release/tinkercloud-ed25519.pem"
test "$(stat -f '%Lp' "$TINKERCLOUD_RELEASE_SIGNING_KEY")" = 600
SOURCE_DATE_EPOCH=0 ./scripts/release-build.sh 0.1.0 ./dist/0.1.0
./scripts/release-verify.sh ./dist/0.1.0
```

On the operator's macOS release machine, the beta key lives at
`~/.tinkercloud/release/tinkercloud-ed25519.pem` with directory mode `0700` and file
mode `0600`. The public half is committed; the private half remains external.
A build must still set `TINKERCLOUD_RELEASE_SIGNING_KEY` explicitly so ordinary
development and test commands never use the beta authority by accident.

The committed authority is beta-only. Before the first stable release, generate
a new production key outside GitHub and update
`packaging/release-public-key.pem`, `packaging/install-client.sh`,
`packaging/install-host.sh`, and `internal/hostops/bootstrap.sh` together.
`scripts/check-release-key-drift.sh` rejects a partial rotation.

The builder refuses a private key that does not derive to
`packaging/release-public-key.pem`. It uses `CGO_ENABLED=0`, Go `-trimpath`,
disabled VCS stamping, and an empty Go build ID. It emits:

- `tinkercloud-linux-amd64` (the only V1 server target);
- `tinker-linux-amd64`, `tinker-linux-arm64`, `tinker-darwin-amd64`, and
  `tinker-darwin-arm64`;
- the reviewed, signed `tinkercloud.service`, `install-host.sh`, and
  `install-client.sh` used by first installation;
- a packed `@tinkercloud/sdk` tarball;
- a SHA-256 metadata document and Ed25519 signature per artifact;
- `SHA256SUMS`, dependency evidence, and reproducible-build provenance.

The requested release version must match `package.json`, `jsr.json`, and the
SDK's exported `SDK_VERSION`; the builder rejects drift before creating a
candidate. Packing the SDK does not publish it to either registry.

The signed-message and metadata format is normative in the
[artifact contract](../../specs/operations/release-artifact-contract.md).

## Verify and install

Always verify the complete release directory before distributing it:

```sh
./scripts/release-verify.sh ./dist/0.1.0
sudo ./packaging/install.sh \
  ./dist/0.1.0/tinkercloud-linux-amd64 \
  ./dist/0.1.0/tinkercloud-linux-amd64.metadata.json \
  ./dist/0.1.0/tinkercloud-linux-amd64.signature
```

`tinkercloud verify-artifact` uses the public key compiled into a released server
binary. The installer and verifier validate the digest before accepting a
signature; neither runs the candidate. Treat any mismatch as a release
incident, not a reason to bypass verification.

Run the tamper suite before releasing:

```sh
./scripts/release-test.sh
./packaging/install_test.sh
```

The test generates an ephemeral key in a temporary directory and proves that
changed artifact bytes, metadata, and signatures are rejected. It never uses a
beta or production signing key.

## Publish a GitHub beta

The canonical beta release is an exact GitHub prerelease. It uses the locked
Tinkercloud identities without claiming JSR, Homebrew, DNS, or stable/latest
availability. After it is public and verified, its exact SDK tarball may be
published separately to npm's opt-in `beta` dist-tag.

Configure the repository once:

1. Make the repository public; anonymous installer and SDK URLs cannot work
   from a private repository.
2. In **Settings → General → Releases**, enable release immutability.
3. In **Settings → Environments**, create `beta-release`.
4. Add a required reviewer, prevent self-review when available, and restrict
   deployment branches to `main` and protected version tags.
5. Add environment secret `TINKERCLOUD_BETA_RELEASE_SIGNING_KEY_B64`. Its decoded
   private key must match `packaging/release-public-key.pem`.

Set the secret without putting it in argv or shell history:

```sh
base64 <"$HOME/.tinkercloud/release/tinkercloud-ed25519.pem" |
  tr -d '\n' |
  gh secret set TINKERCLOUD_BETA_RELEASE_SIGNING_KEY_B64 \
    --env beta-release
```

For each beta, update `package.json`, `jsr.json`, and exported `SDK_VERSION` to
one new strict numeric version, merge the reviewed commit to `main`, then create
and push its exact tag:

```sh
VERSION=0.1.0
git tag -a "v$VERSION" -m "Beta v$VERSION"
git push origin "v$VERSION"
gh workflow run release.yml \
  --ref main \
  -f channel=beta \
  -f version="$VERSION" \
  -f confirmation="publish-beta-v$VERSION"
```

Approve the `beta-release` Environment only after checking the tag, commit, and
workflow diff. The workflow tests without the private key, signs the complete
release, verifies it locally, uploads it as a draft, downloads and verifies the
draft bytes, publishes it as a prerelease, and smoke-tests the public CLI
installer plus SDK tarball. This workflow itself never publishes npm/JSR or
changes Homebrew.

Every public beta uses a new patch version. Do not rerun against an existing
release, replace an asset, move its tag, or promote it to stable/latest. A
failure after publication is repaired with a new reviewed version.

For repository `ChrisMarxDev/tinkercloud` and version `0.1.0`, install the beta
CLI:

```sh
curl --proto '=https' --tlsv1.2 -fsSL \
  https://github.com/ChrisMarxDev/tinkercloud/releases/download/v0.1.0/install-client.sh | sh
```

Install the SDK directly from the same GitHub release without using the npm
registry:

```sh
npm install \
  https://github.com/ChrisMarxDev/tinkercloud/releases/download/v0.1.0/tinkercloud-sdk-0.1.0.tgz
```

Install a beta host from that VPS's root shell with the exact same release:

```sh
curl --proto '=https' --proto-redir '=https' --tlsv1.2 -fsSL \
  https://github.com/ChrisMarxDev/tinkercloud/releases/download/v0.1.0/install-host.sh | sh
```

The signed host installer embeds `RELEASE_BASE`; HTTPS redirects may carry
release assets but signed checksum and Ed25519 evidence remains the trust
decision. Update an installed beta host with:

```sh
tinker host update root@HOST --release-base https://github.com/ChrisMarxDev/tinkercloud/releases/download/v0.1.0/
```

The GitHub beta workflow is governed by
[`specs/operations/beta-release-contract.md`](../../specs/operations/beta-release-contract.md).

## Publish the SDK npm beta

Only `@tinkercloud/sdk` is eligible for npm during beta. The native CLI remains
on the signed GitHub installer, and the server is never placed in a JavaScript
registry. The npm version stays strict numeric for compatibility; npm's fixed
`beta` dist-tag and the GitHub prerelease flag identify the channel.

Configure GitHub Environment `npm-sdk` once with a required reviewer, access
from `main`, and no npm token secret. The package's trusted publisher must be
exactly repository `ChrisMarxDev/tinkercloud`, workflow
`release.yml`, Environment `npm-sdk`, with only `npm publish` allowed.

The package must first be bootstrapped once with interactive 2FA from the exact
verified prerelease tarball. The complete guarded procedure is in
[SDK distribution](sdk-distribution.md). After that, each later beta is:

```sh
task release:npm-sdk-check
gh workflow run release.yml \
  --ref main \
  -f channel=beta \
  -f version="$VERSION" \
  -f confirmation="publish-beta-v$VERSION"
```

Approve `npm-sdk` only after reviewing the existing public GitHub prerelease and
confirming the npm version is unused. The workflow downloads and verifies the
complete release, publishes the exact SDK tarball without lifecycle scripts or
repacking, then verifies its integrity, provenance, repository metadata,
`beta` dist-tag, and clean import. It cannot select `latest` or publish the CLI,
server, JSR, or Homebrew.

This path is governed by
[`specs/sdk/npm-beta-publishing-contract.md`](../../specs/sdk/npm-beta-publishing-contract.md).

## Publish stable CLI channels

Stable distribution uses the same `release.yml` invocation with `channel=stable`.
It builds, signs, remotely verifies, and publishes the canonical GitHub release,
then publishes the exact same-version SDK and CLI npm artifacts through their
separate protected OIDC environments.

Both workflows fail while `packaging/release-key-policy.json` identifies the
committed authority as `beta`. Rotate to a separately controlled production
key and synchronize every embedded trust anchor before changing that policy.
The GitHub repository must be public, releases immutable, and the
`stable-release` and `npm-cli` Environments reviewer-protected.

The first npm version is a one-time exception: npm requires a package to exist
before it can be assigned a trusted publisher. Publish the exact generated
tarball interactively with 2FA, then bind `@tinkercloud/cli` to
`release.yml`, repository `ChrisMarxDev/tinkercloud`, Environment
`npm-cli`, with only `npm publish` allowed. Later versions use Node 24, npm
11.5.1 or newer, `id-token: write`, no registry token, and automatic
provenance.

The complete one-time setup and dispatch commands are in
[CLI distribution](cli-distribution.md). The normative behavior and denials
are in the [stable release contract](../../specs/operations/stable-release-contract.md)
and [denial charter](../../specs/operations/stable-distribution-denial-charter.md).

After the source repository is public, and before either npm publishing or the
planned package-maintenance workflow is activated, run
`npm audit --package-lock-only` from both the repository root and `landing/`.
Review and record both results as release evidence. This is deliberately not a
pre-public gate because the registry request discloses each dependency graph to
npm; publishing remains denied until the post-public audit is complete.

## Install the deployer client

Deployer machines install only `tinker`, never the privileged server binary. The
installer is intentionally non-root and requires a trusted HTTPS release
directory. It chooses the operating system and architecture locally, validates
the selected artifact against `SHA256SUMS`, metadata, and the embedded Ed25519
release key, then atomically writes `tinker` to `~/.local/bin` (or
`TINKER_INSTALL_DIR`).

```sh
curl --proto '=https' --tlsv1.2 -fsSL \
  https://github.com/ChrisMarxDev/tinkercloud/releases/download/vVERSION/install-client.sh | sh
export PATH="$HOME/.local/bin:$PATH"
tinker login --server https://tinker.example.net
```

Do not use a client installer fetched from an unauthenticated URL as its own
trust root. Obtain this script from the signed source release or a reviewed
checkout; the public key embedded in it is the verification authority.

The stable one-line form remains a template until a stable GitHub release is
published and anonymously verified:

```sh
curl --proto '=https' --tlsv1.2 -fsS \
  https://github.com/ChrisMarxDev/tinkercloud/releases/latest/download/install-client.sh | sh
```

Replace both `latest` path segments with `download/vVERSION` for an exact,
reproducible version.

The HTTPS bootstrap authenticates the reviewed installer; the installer then
uses the pinned Ed25519 key for the selected native binary. Do not advertise
this command before the origin is operator-controlled and the installer itself
is part of the signed release evidence.

## Prepare package-manager candidates

After verifying a signed local release, generate but do not publish the
workstation packages:

```sh
task distribution:prepare \
  VERSION=0.1.0 \
  RELEASE_DIR=./dist/0.1.0 \
  OUTPUT=./dist/prepared-0.1.0
```

The npm candidate contains all four native `tinker` binaries and a
dependency-free Node launcher. It has no install/postinstall hook, download
step, runtime dependency, or server binary. One eventual npm publication works
with npm, pnpm, Yarn, and Bun. JSR remains the TypeScript SDK channel. The
Homebrew formula is generated from the verified macOS checksums but no tap is
created or modified.

Public commands such as `npm install -g @tinkercloud/cli`,
`pnpm add -g @tinkercloud/cli`, `yarn add --dev @tinkercloud/cli`,
`bun add -g @tinkercloud/cli`, and
`brew install ChrisMarxDev/tinkercloud/tinker` remain documentation templates
until namespace ownership and the corresponding release channels are approved.

## Install or inspect a host through tinker

The workstation CLI may transport only the reviewed fixed host grammar over
normal host-key-verified SSH:

```sh
tinker host install root@HOST
tinker host status root@HOST
tinker host doctor root@HOST
tinker host update root@HOST --release-base https://github.com/ChrisMarxDev/tinkercloud/releases/download/vVERSION/
tinker host uninstall root@HOST
```

Install sends the embedded bootstrap, which verifies the signed full installer
before executing it. Existing servers refuse the install path and use the local
rollback-capable updater. `tinker host` does not store root credentials, weaken
known-host verification, accept arbitrary SSH options, or expose remote exec.
A released CLI derives the exact immutable install release from its embedded
version; development builds require an explicit `--release-base`. A custom
release base is valid only when its signed `install-host.sh` was built with that
same base; it is not a runtime override of the released root installer.
Uninstall
requires confirmation of the exact host and preserves the ACME cache by
default.

After interactive OTP verification, `tinker login` saves its server-bound bearer
in `os.UserConfigDir()/tinker/<sha256(normalized-server)>.json`. The platform
configuration directory must be mode `0700`, the regular non-symlinked
credential file mode `0600`, and writes atomic replacements in the same
directory. The file's bounded exact JSON must reject malformed, unknown,
duplicate, server-mismatched, or oversized input. Client tests must exercise
these denial paths and prove that a raw bearer never reaches argv, environment
variables, output, or logs. The file is local to the deployer's OS account, not
a release artifact, project file, browser credential, or app credential.

On a later `tinker login`, the client first checks the stored bearer through
authenticated `whoami`. A valid response is reused without issuing another OTP
or rewriting the file. Only an unauthorized or expired bearer may fall back to
the interactive CLI OTP flow; dependency, transport, malformed-response,
unexpected-status, and ambiguous authorization failures stop without changing
the saved credential. Use `tinker login --force` to deliberately switch
accounts. It obtains a fresh OTP and replaces the file only after the new
bearer completes `whoami`. A rate-limited response tells the deployer to wait
and retry without disclosing email eligibility or challenge state.

The same protected directory keeps a separate bounded exact
`default-server.json` containing only `version` and the normalized HTTPS
platform URL. Successful login updates it after credential verification and
storage; later commands may omit `--server`. Explicit `--server` is an
invocation-only override. A human command with exactly missing default state
offers one bounded server prompt, verifies the normalized HTTPS host through a
direct no-redirect API-v1 response, then saves it before continuing; the next
step may still be `Login required`. JSON never prompts or saves. Malformed,
unsafe, incompatible, redirected, transport-failed, or storage-failed setup
state fails closed. `tinker logout` calls `POST /api/v1/auth/logout` with the current global
CLI bearer and removes only that matching local credential after confirmed
revocation (or a `401` proving it is already unusable). It retains the default
URL and retains the local credential on transport, 5xx, persistence, or local
delete failure.

## Update health evidence

Before creating rollback state, a remote update verifies both the server
artifact triplet and the signed schema-2 release-manifest triplet from the same
origin. The manifest version must equal the artifact version, bind the server
digest, and contain the exact current CLI/SDK/API/schema compatibility matrix.
An air-gapped update supplies both triplets; local/remote mixing is rejected.

Before retaining a replacement server, `tinkercloud update` deterministically
selects the lexicographically first locally verified active app and requests its
`/_tinker/api/v1/capabilities` route through the normal HTTPS gateway. The
operator supplies no app slug or probe URL. An anonymous request must receive
the exact protected-route `401` JSON denial, including the stable error envelope
and no-store security headers. A 404 is not acceptable evidence: it can mean
the app route is absent. Likewise, a redirect, public 2xx response, malformed
response, timeout, or transport failure automatically restores the prior binary
and restarts it. With no active app, the updater proves that exact installed
state plus platform health, socket confinement, and safe unknown-app-host
denial; ambiguous state is failure.
