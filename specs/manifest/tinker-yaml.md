# `tinker.yaml` Contract

## Version 1 private shape

```yaml
version: 1
name: invoice-review
description: Review invoices before the weekly finance close.

build:
  output: dist

access:
  mode: private
  allow:
    emails:
      - alice@example.com
    domains:
      - company.com

features:
  kv: true
  blobs: true
  realtime: true

capabilities:
  llm:
    chat: true

spa:
  fallback: index.html
```

## Rules

- `version` is required once a manifest exists.
- Version 1 retains its exact private-only meaning. It rejects `tags`,
  `access.indexing`, and `access.mode: public` so an old receipt is never
  reinterpreted as broader authority.
- Version 2 is the accepted post-V1 reach-and-insights shape. The post-V1 CLI
  generator writes version 2; the server continues to accept valid version 1
  private manifests.
- `name` becomes a slug after validation; silent lossy rewriting is avoided.
- `description` is optional immutable deployment presentation metadata. It is
  trimmed at both edges before persistence; an empty value is allowed and is
  omitted from dashboard presentation. A non-empty value must be one line and
  at most 280 Unicode code points after trimming. Invalid UTF-8, every Unicode
  control character (including tabs and newlines), and U+2028/U+2029 are
  rejected rather than normalized away. No other internal whitespace is
  rewritten.
- The description lives only in that deployment's immutable `manifest_json`.
  It does not create application-level mutable metadata. A release can show
  its own description; the app summary is the active deployment's description,
  while failed activation leaves that summary unchanged.
- `build.output` is resolved beneath the project directory by the CLI.
- `access.mode` defaults to `private` in both versions.
- The active owner is always an implicit viewer. An empty allowlist therefore
  means owner-only, not public.
- `access.mode: public` is invalid in version 1. In version 2 it requests only
  operator-gated anonymous immutable static access and requires every browser
  feature/capability to be false.
- Before the separate public-static authorization and gateway slice exists, a
  version 2 `public` request is structurally valid but activation rejects it;
  Tinkercloud must not silently install it as a private policy or display it as
  an effective public posture.
- Email/domain values are normalized by the server and echoed in canonical form.
- The canonical allowlist is immutable deployment metadata. Activation makes
  that exact private allowlist current atomically with the release pointer.
- Unknown keys are errors in every supported manifest version to catch typos.
- `spa.fallback` must name a normal file in the uploaded release.
- Enabling a capability that the server/operator disabled is an error.
- `features.blobs` opts the app into the V1 lightweight app-shared blob
  capability. It does not name a bucket, path, provider, endpoint, or
  credential. Missing or `false` means blob routes deny before storage access.
- `capabilities.llm.chat: true` requests only the logical provider-neutral chat
  capability. It cannot name a connection, provider, model, endpoint, header,
  key, or grant. Deployment and invocation remain unavailable until the
  operator has approved a current app/profile binding; disabling either side
  denies the next request.

## Version 2 reach metadata

```yaml
version: 2
name: product-story
description: Interactive product walkthrough.
tags:
  - demo
  - product

build:
  output: dist

access:
  mode: public
  indexing: false
  allow:
    emails: []
    domains: []

features:
  kv: false
  blobs: false
  realtime: false

capabilities:
  llm:
    chat: false

spa:
  fallback: index.html
```

- `tags` is optional immutable active-deployment presentation metadata. It has
  zero to eight unique values. Each value is 1–24 lowercase ASCII characters,
  begins and ends with a letter or number, and may contain only internal
  hyphens. Canonical serialization sorts tags; tags never select policy,
  capability, storage, or identity.
- `access.indexing` defaults false and is valid only when mode is `public`.
  Indexing changes no authorization and is effective only while both the
  public policy and operator gate are current.
- A version 2 public manifest is invalid when `features.kv`,
  `features.blobs`, `features.realtime`, `capabilities.llm.chat`, or any future
  browser capability is enabled. One central capability-free validation must
  cover future fields rather than relying on scattered checks.
- Owner/email/domain rules remain canonical in a public manifest. They govern
  the normal private login path when anonymous access is ineffective.
- Unknown keys and unknown versions reject before upload/activation state can
  broaden access.

## Local creation and deploy-first onboarding

The post-V1 `tinker init [DIR]` creates a new strict version 2 `tinker.yaml` only in an existing,
non-symlinked project directory. It never overwrites a manifest. The interactive
wizard uses the normalized project-directory basename without asking when it is
a valid slug. It asks for a replacement only when derivation is invalid or the
server generically rejects availability. It selects one unambiguous
conventional built output without asking. Multiple valid outputs require a
choice; no valid output stops with an exact build action instead of suggesting
the project root.

Description, allowlist, capabilities, and SPA fallback are optional behind one
review/edit step. Capability detection may produce a warning but never enables
authority; only an existing manifest or deliberate edit does. A combined
allowlist accepts comma-separated email addresses and domains; values are
validated through this same contract before the file is created. The one final
action names any access broadening.

`tinker deploy [DIR]` defaults to `.`. For a human invocation with no manifest it
runs that wizard and, after the same final deploy action, atomically creates
`tinker.yaml`, prints its path, and continues. Manifest creation has no separate
confirmation. `--json` is non-interactive: it neither prompts, creates a
missing manifest, requests OTP, nor stores credentials.

The generator serializes only the strict current version 2 shape and validates the generated
bytes with `ParseManifest` before returning them. Build output and fallback must
be relative, normalized paths beneath the project; every existing component is
checked with `Lstat` and symlinks, traversal, non-directories, and non-regular
fallback files are rejected.

### Local denial charter

- A missing/invalid project directory, manifest symlink, pre-existing target,
  malformed wizard value, or failed atomic write creates no replacement file.
- JSON/non-interactive invocations do not read stdin or mutate local missing
  prerequisites.
- A path that escapes the project, or has a symlink in any output/fallback
  component, is rejected before archive creation.
- Empty optional fields are valid; every interactive input is bounded and each
  prompt exists only for required ambiguity or explicit edit intent.

`features` remains the spelling for local built-in primitives. `capabilities`
requests operator-governed external operations without selecting their
credential or implementation:

```yaml
capabilities:
  llm:
    chat: true
```

## Precedence

```text
server safety policy
  > operator limits
  > explicit CLI flags
  > tinker.yaml
  > secure defaults
```

Flags may narrow access without warning. Any flag that broadens access must be
explicit in command output and machine-readable results.

## Validation result sketch

```json
{
  "valid": false,
  "errors": [
    {
      "path": "access.allow.emails[0]",
      "code": "invalid_email",
      "message": "Enter a syntactically valid email address."
    }
  ]
}
```
