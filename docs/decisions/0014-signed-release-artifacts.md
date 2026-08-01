# 0014: Pin signed release metadata to one Ed25519 public key

**Status:** Accepted

## Context

The update path changes the gateway executable, which is part of the platform
trust boundary. A checksum fetched beside a candidate does not establish who
authorized that candidate. The installer and update command need one small,
offline-verifiable trust root that is practical for a one-VPS operator.

## Decision

Tinkercloud signs every V1 distributable artifact with Ed25519. The committed
`packaging/release-public-key.pem` is the release trust root. Release builders
derive its raw public key and compile it into `tinkercloud` with Go linker flags;
the build refuses a mismatched signing key. The private key is an external
release-environment input (`TINKERCLOUD_RELEASE_SIGNING_KEY` points to a file) and
is neither stored in this repository nor copied to release output.

The signed payload is the four-line canonical metadata payload defined by the
[release artifact contract](../../specs/operations/release-artifact-contract.md).
This binds a version, API/schema compatibility labels, and the exact SHA-256 of
the artifact. `SHA256SUMS`, dependency evidence, and provenance make review and
reproduction practical, but are not signature substitutes.

The V1 production private key is retained only on the operator's local release
machine under `~/.tinkercloud/release/`, with directory mode `0700` and key mode
`0600`. It is never copied to the VPS. The build requires an explicit
`TINKERCLOUD_RELEASE_SIGNING_KEY` file path, preventing routine tests from using
the production authority implicitly.

On 2026-07-27, the initial disposable rollout key was found to have been
correctly deleted without first establishing a durable release authority. The
operator explicitly approved a one-time trust-root rotation. The replacement
public key is installed through local root recovery with a known-good binary
snapshot and the ordinary post-restart health and anonymous-denial gates.
After that bootstrap, every update again follows the normal signed update path.

## Consequences

- Key rotation is an explicit release/recovery event: it cannot be silently
  delivered by an artifact signed with the old key.
- A developer can run all release pipeline tests with a disposable temporary
  key, but cannot cut a production-compatible release without the external
  private key.
- Candidate installation/update fails closed before execution when a digest,
  metadata document, signature, or pinned public key is wrong.
