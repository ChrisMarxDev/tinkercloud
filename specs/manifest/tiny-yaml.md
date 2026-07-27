# `tiny.yaml` Contract

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
  realtime: true

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
  its own description; the app summary is the selected current deployment's
  description, so rollback restores it naturally.
- `build.output` is resolved beneath the project directory by the CLI.
- `access.mode` defaults to `private`, never `public`.
- The active owner is always an implicit viewer. An empty allowlist therefore
  means owner-only, not public.
- `access.mode: public` is invalid in V1.
- Email/domain values are normalized by the server and echoed in canonical form.
- The canonical allowlist is immutable deployment metadata. Activation makes
  that exact private allowlist current atomically with the release pointer;
  rollback restores the selected deployment's allowlist.
- Unknown top-level keys are errors in V1 to catch typos.
- `spa.fallback` must name a normal file in the uploaded release.
- Enabling a capability that the server/operator disabled is an error.

`features` is the V1 spelling for built-in capabilities. A later manifest
version may add operator-approved external capability bindings. Those bindings
name grants or aliases, never raw credentials:

```yaml
capabilities:
  llm:
    grant: staff-llm
```

## Precedence

```text
server safety policy
  > operator limits
  > explicit CLI flags
  > tiny.yaml
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
