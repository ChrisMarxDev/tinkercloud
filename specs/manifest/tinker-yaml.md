# `tinker.yaml` Contract

## V1 shape

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
- `access.mode` defaults to `private`, never `public`.
- The active owner is always an implicit viewer. An empty allowlist therefore
  means owner-only, not public.
- `access.mode: public` is invalid in V1.
- Email/domain values are normalized by the server and echoed in canonical form.
- The canonical allowlist is immutable deployment metadata. Activation makes
  that exact private allowlist current atomically with the release pointer.
- Unknown top-level keys are errors in V1 to catch typos.
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

## Local creation and deploy-first onboarding

`tinker init [DIR]` creates a new strict V1 `tinker.yaml` only in an existing,
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

The generator serializes only this strict V1 shape and validates the generated
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
