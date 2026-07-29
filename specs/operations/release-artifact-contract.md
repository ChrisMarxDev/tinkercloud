# Signed Release Artifact Contract (V1)

Each published release artifact is immutable and consists of exactly these
files, with `NAME` being the artifact filename:

```text
NAME
NAME.metadata.json
NAME.signature
SHA256SUMS
dependency-evidence.txt
provenance.txt
```

`NAME.metadata.json` is UTF-8 JSON with exactly four lowercase string fields,
in this canonical order and without semantic aliases:

```json
{"version":"VERSION","api":"1","schema":"1","sha256":"64 lowercase hexadecimal characters"}
```

The signed bytes are exactly the following UTF-8 bytes (each `\n` denotes one
LF byte, `0x0a`; there is no final LF byte):

```text
VERSION\nAPI\nSCHEMA\nSHA256
```

`NAME.signature` is standard base64 for a 64-byte Ed25519 signature of those
bytes. The SHA-256 value is over `NAME` bytes. Verification fails closed for a
missing/unknown field, noncanonical digest, extra bytes, a digest mismatch, an
invalid signature, or a key other than the embedded pinned public key.

The only V1 update target is `tinyhost-linux-amd64`. Every V1 release contains
that server plus `tiny-linux-amd64`, `tiny-linux-arm64`,
`tiny-darwin-amd64`, `tiny-darwin-arm64`, and one versioned SDK tarball. Each
of those distributable payloads has its own metadata and signature; a release
also contains the reviewed `tinyhost.service` unit, `install-host.sh`, and
`install-client.sh` with their own metadata and signatures and is invalid if an
installation input or supported client platform is absent. The release public
key is public and committed at
`packaging/release-public-key.pem`. A signing private key is supplied only to
the release environment through `TINYHOST_RELEASE_SIGNING_KEY`; it is never
written to the repository, artifacts, logs, metadata, SDK, or browser bundle.

The release version must exactly match the semantic version inside the SDK
tarball and the SDK's npm, JSR, and exported runtime version declarations.
Version drift is a release-build failure rather than a filename rewrite.

`SHA256SUMS` covers every generated release file other than itself, including
all payloads, sidecars, dependency evidence, and provenance. The signed
schema-2 `release-manifest.json` binds the release version, the exact
compatibility matrix from the update-compatibility contract, and the SHA-256 of
every release file except itself, its metadata/signature, and `SHA256SUMS`;
this makes the evidence and compatibility policy tamper-evident even if a
checksum manifest is replaced. `provenance.txt` lists
the build target and SHA-256 for every distributable payload, allowing a
reviewer to map each signature to its reproducible build target. Dependency and
provenance evidence are review aids, not substitutes for signature
verification. A successful verification does not execute an artifact.

Auxiliary installer inputs are not release artifacts. In particular, a
disposable VPS-acceptance public key may be staged for the installer only after
the complete release directory has passed verification, and it must remain
outside that directory. Adding it to the release evidence without re-signing
the complete evidence set is a verification failure.

The unprivileged client installer accepts only an HTTPS release directory,
selects exactly one of the four `tiny-OS-ARCH` payloads, validates its three
checksum-manifest entries, then validates its metadata and signature with the
embedded public key. It refuses root, never installs `tinyhost`, and replaces
`tiny` only after verification succeeds.
