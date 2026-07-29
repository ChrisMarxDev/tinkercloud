# Release pipeline

TinyHost releases are built locally or in a controlled CI environment.
Preparation commands never publish a package or execute a candidate binary.
The explicitly dispatched beta workflow is the only pre-rename hosted-release
mutation path. The release authority is an Ed25519 private key held outside
this repository. The matching public key is committed in
`packaging/release-public-key.pem` and compiled into the Linux `tinyhost`
binary.

## Build a candidate

Generate or obtain the signing key through the operator's release-key process,
then make its directory private and its file readable only to the release
account. Do not place it in the checkout, pass its contents on a command line,
copy it to the VPS, or print it.

```sh
umask 077
install -d -m 700 "$HOME/.tinyhost/release"
export TINYHOST_RELEASE_SIGNING_KEY="$HOME/.tinyhost/release/tinyhost-ed25519.pem"
test "$(stat -f '%Lp' "$TINYHOST_RELEASE_SIGNING_KEY")" = 600
SOURCE_DATE_EPOCH=0 ./scripts/release-build.sh 0.1.0 ./dist/0.1.0
./scripts/release-verify.sh ./dist/0.1.0
```

On the operator's macOS release machine, the pre-rename beta key lives at
`~/.tinyhost/release/tinyhost-ed25519.pem` with directory mode `0700` and file
mode `0600`. The public half is committed; the private half remains external.
A build must still set `TINYHOST_RELEASE_SIGNING_KEY` explicitly so ordinary
development and test commands never use the beta authority by accident.

The committed authority is beta-only. Before the first stable release, generate
a new production key outside GitHub and update
`packaging/release-public-key.pem`, `packaging/install-client.sh`,
`packaging/install-host.sh`, and `internal/hostops/bootstrap.sh` together.
`scripts/check-release-key-drift.sh` rejects a partial rotation.

The builder refuses a private key that does not derive to
`packaging/release-public-key.pem`. It uses `CGO_ENABLED=0`, Go `-trimpath`,
disabled VCS stamping, and an empty Go build ID. It emits:

- `tinyhost-linux-amd64` (the only V1 server target);
- `tiny-linux-amd64`, `tiny-linux-arm64`, `tiny-darwin-amd64`, and
  `tiny-darwin-arm64`;
- the reviewed, signed `tinyhost.service`, `install-host.sh`, and
  `install-client.sh` used by first installation;
- a packed `@tinyhost/sdk` tarball;
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
  ./dist/0.1.0/tinyhost-linux-amd64 \
  ./dist/0.1.0/tinyhost-linux-amd64.metadata.json \
  ./dist/0.1.0/tinyhost-linux-amd64.signature
```

`tinyhost verify-artifact` uses the public key compiled into a released server
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

The beta channel is an exact GitHub prerelease. It uses the working names
without reserving npm, JSR, Homebrew, DNS, or stable/latest identities.

Configure the repository once:

1. Make the repository public; anonymous installer and SDK URLs cannot work
   from a private repository.
2. In **Settings → General → Releases**, enable release immutability.
3. In **Settings → Environments**, create `beta-release`.
4. Add a required reviewer, prevent self-review when available, and restrict
   deployment branches to `main` and protected version tags.
5. Add environment secret `TINYHOST_BETA_RELEASE_SIGNING_KEY_B64`. Its decoded
   private key must match `packaging/release-public-key.pem`.

Set the secret without putting it in argv or shell history:

```sh
base64 <"$HOME/.tinyhost/release/tinyhost-ed25519.pem" |
  tr -d '\n' |
  gh secret set TINYHOST_BETA_RELEASE_SIGNING_KEY_B64 \
    --env beta-release
```

For each beta, update `package.json`, `jsr.json`, and exported `SDK_VERSION` to
one new strict numeric version, merge the reviewed commit to `main`, then create
and push its exact tag:

```sh
VERSION=0.1.0
git tag -a "v$VERSION" -m "Beta v$VERSION"
git push origin "v$VERSION"
gh workflow run beta-release.yml \
  --ref main \
  -f version="$VERSION" \
  -f confirmation="publish-beta-v$VERSION"
```

Approve the `beta-release` Environment only after checking the tag, commit, and
workflow diff. The workflow tests without the private key, signs the complete
release, verifies it locally, uploads it as a draft, downloads and verifies the
draft bytes, publishes it as a prerelease, and smoke-tests the public CLI
installer plus SDK tarball. It never publishes npm/JSR or changes Homebrew.

Every public beta uses a new patch version. Do not rerun against an existing
release, replace an asset, move its tag, or promote it to stable/latest. A
failure after publication is repaired with a new reviewed version.

For repository `OWNER/REPO` and version `0.1.0`, install the beta CLI:

```sh
RELEASE_BASE=https://github.com/OWNER/REPO/releases/download/v0.1.0/
curl --proto '=https' --tlsv1.2 -fsSL \
  "${RELEASE_BASE}install-client.sh" |
  TINYHOST_CLIENT_RELEASE_BASE="$RELEASE_BASE" sh
```

Install the SDK directly from the same GitHub release without using the npm
registry:

```sh
npm install \
  https://github.com/OWNER/REPO/releases/download/v0.1.0/tinyhost-sdk-0.1.0.tgz
```

Install or update a beta host from the exact same release:

```sh
tiny host install root@HOST --release-base "$RELEASE_BASE"
tiny host update root@HOST --release-base "$RELEASE_BASE"
```

The beta workflow is governed by
[`specs/operations/beta-release-contract.md`](../../specs/operations/beta-release-contract.md).

## Install the deployer client

Deployer machines install only `tiny`, never the privileged server binary. The
installer is intentionally non-root and requires a trusted HTTPS release
directory. It chooses the operating system and architecture locally, validates
the selected artifact against `SHA256SUMS`, metadata, and the embedded Ed25519
release key, then atomically writes `tiny` to `~/.local/bin` (or
`TINYHOST_CLIENT_INSTALL_DIR`).

```sh
TINYHOST_CLIENT_RELEASE_BASE=https://releases.example.net/tinyhost/v1.0.0/ \
  ./packaging/install-client.sh
export PATH="$HOME/.local/bin:$PATH"
tiny login --server https://tiny.example.net
```

Do not use a client installer fetched from an unauthenticated URL as its own
trust root. Obtain this script from the signed source release or a reviewed
checkout; the public key embedded in it is the verification authority.

The stable one-line form remains a template until the final rename and release
origin are accepted:

```sh
curl --proto '=https' --tlsv1.2 -fsS \
  https://RELEASE_ORIGIN/VERSION/install-client.sh |
  TINYHOST_CLIENT_RELEASE_BASE=https://RELEASE_ORIGIN/VERSION/ sh
```

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
  OUTPUT=./dist/prepared-0.1.0 \
  NPM_PACKAGE=@PENDING_RENAME/cli \
  FORMULA_CLASS=PendingRename \
  COMMAND=pending-rename \
  RELEASE_BASE=https://RELEASE_ORIGIN/0.1.0/
```

The npm candidate contains all four native `tiny` binaries and a
dependency-free Node launcher. It has no install/postinstall hook, download
step, runtime dependency, or server binary. One eventual npm publication works
with npm, pnpm, Yarn, and Bun. JSR remains the TypeScript SDK channel. The
Homebrew formula is generated from the verified macOS checksums but no tap is
created or modified.

Public commands such as `npm install -g PACKAGE`, `pnpm add -g PACKAGE`,
`yarn global add PACKAGE`, `bun add -g PACKAGE`, and
`brew install TAP/FORMULA` remain documentation templates until the rename,
namespace ownership, origins, and release channels are approved.

## Install or inspect a host through tiny

The workstation CLI may transport only the reviewed fixed host grammar over
normal host-key-verified SSH:

```sh
tiny host install root@HOST --release-base https://RELEASE_ORIGIN/VERSION/
tiny host status root@HOST
tiny host doctor root@HOST
tiny host update root@HOST --release-base https://RELEASE_ORIGIN/VERSION/
```

Install sends the embedded bootstrap, which verifies the signed full installer
before executing it. Existing servers refuse the install path and use the local
rollback-capable updater. `tiny host` does not store root credentials, weaken
known-host verification, accept arbitrary SSH options, or expose remote exec.

After interactive OTP verification, `tiny login` saves its server-bound bearer
in `os.UserConfigDir()/tiny/<sha256(normalized-server)>.json`. The platform
configuration directory must be mode `0700`, the regular non-symlinked
credential file mode `0600`, and writes atomic replacements in the same
directory. The file's bounded exact JSON must reject malformed, unknown,
duplicate, server-mismatched, or oversized input. Client tests must exercise
these denial paths and prove that a raw bearer never reaches argv, environment
variables, output, or logs. The file is local to the deployer's OS account, not
a release artifact, project file, browser credential, or app credential.

On a later `tiny login`, the client first checks the stored bearer through
authenticated `whoami`. A valid response is reused without issuing another OTP
or rewriting the file. Only an unauthorized or expired bearer may fall back to
the interactive CLI OTP flow; dependency, transport, malformed-response,
unexpected-status, and ambiguous authorization failures stop without changing
the saved credential. Use `tiny login --force` to deliberately switch
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
state fails closed. `tiny logout` calls `POST /api/v1/auth/logout` with the current global
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

Before retaining a replacement server, `tinyhost update` deterministically
selects the lexicographically first locally verified active app and requests its
`/_tiny/api/v1/capabilities` route through the normal HTTPS gateway. The
operator supplies no app slug or probe URL. An anonymous request must receive
the exact protected-route `401` JSON denial, including the stable error envelope
and no-store security headers. A 404 is not acceptable evidence: it can mean
the app route is absent. Likewise, a redirect, public 2xx response, malformed
response, timeout, or transport failure automatically restores the prior binary
and restarts it. With no active app, the updater proves that exact installed
state plus platform health, socket confinement, and safe unknown-app-host
denial; ambiguous state is failure.
