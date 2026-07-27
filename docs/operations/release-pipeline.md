# Release pipeline

TinyHost releases are built locally or in a controlled CI environment; no
release tool publishes a package or executes a candidate binary. The release
authority is an Ed25519 private key held outside this repository. The matching
public key is committed in `packaging/release-public-key.pem` and compiled into
the Linux `tinyhost` binary.

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

On the operator's macOS release machine, the V1 production key lives at
`~/.tinyhost/release/tinyhost-ed25519.pem` with directory mode `0700` and file
mode `0600`. The public half is committed; the private half remains external.
A build must still set `TINYHOST_RELEASE_SIGNING_KEY` explicitly so ordinary
development and test commands never use the production authority by accident.

The builder refuses a private key that does not derive to
`packaging/release-public-key.pem`. It uses `CGO_ENABLED=0`, Go `-trimpath`,
disabled VCS stamping, and an empty Go build ID. It emits:

- `tinyhost-linux-amd64` (the only V1 server target);
- `tiny-linux-amd64`, `tiny-linux-arm64`, `tiny-darwin-amd64`, and
  `tiny-darwin-arm64`;
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
production signing key.

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

## Update health evidence

Before retaining a replacement server, `tinyhost update --app-slug` verifies a
locally confirmed active app through its normal HTTPS gateway route. An
anonymous request to that app's `/` must receive the normal protected-route
`401` JSON denial, including the stable error envelope and no-store security
headers. A 404 is not acceptable evidence: it can mean the app route is absent.
Likewise, a redirect, public 2xx response, malformed response, timeout, or
transport failure automatically restores the prior binary and restarts it.
