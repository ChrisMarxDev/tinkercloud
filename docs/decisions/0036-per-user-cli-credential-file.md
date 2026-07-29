# ADR 0036: Protected per-user CLI credential file

## Context

The interactive `tiny` deployer client must retain a scoped bearer after OTP
login so a deployer can run later commands without repeating authentication.
The original design delegated persistence to macOS Keychain or Linux
`secret-tool`. Those command dependencies are brittle across headless machines,
minimal Linux installations, and development environments, and can fail after
the server has already issued a valid bearer. That turns the intended
one-time-login experience into an unreliable platform-specific flow.

This is client-local persistence only. It must not blur the server-persisted
credential types: a CLI bearer remains distinct from a browser control session,
the global viewer identity credential, and app-bound viewer sessions.

## Decision

The interactive CLI stores its bearer in one Tiny-owned per-user credential file
per normalized HTTPS platform URL, at
`os.UserConfigDir()/tiny/<sha256(normalized-server)>.json`. It uses the
platform's user configuration location, not a project directory or a
release/install location.

The same directory also contains a separate non-secret default-platform file,
`default-server.json`. It contains only the exact version and normalized HTTPS
server URL from the most recent successful interactive login or verified
first-run platform setup. It has the same
strict directory/file mode, non-symlink, bounded JSON, atomic replacement, and
directory-fsync checks as a bearer file. Commands may use it when `--server` is
omitted; an explicit `--server` is invocation-only and cannot alter the
default. For recognized human commands, exactly missing state opens one bounded
setup prompt, then caches only a normalized HTTPS URL that passes direct
no-redirect API-v1 compatibility proof. This setup persists the platform before
authentication, so the continued command may correctly report `Login required`.
JSON never prompts or caches. Unsafe, malformed, incompatible, redirected,
transport-failed, or storage-failed state fails closed.

The storage boundary is deliberately strict:

- the configuration directory is mode `0700`;
- the credential file is a regular, non-symlinked file at mode `0600`;
- directory/file validation and writes reject symlink traversal;
- the file is bounded, versioned JSON with exactly the `version`, `server`, and
  `token` members; malformed, oversized, unknown, duplicate JSON members, or a
  stored-server mismatch fail closed;
- updates write a new file and atomically replace the old file in the same
  directory, never treating a partial write as valid state;
- a raw bearer never appears in argv, environment variables, logs, human or
  JSON CLI output, or a project file.

`tiny login` completes OTP once and saves a still-valid token. Subsequent CLI
commands silently reuse the file selected by their normalized platform URL. The
server continues to enforce exact scope, deployer status, expiry, app binding
where app-scoped, and revocation on every use.

`tiny logout` is a real self-revocation operation, not local cache clearing.
It calls the authenticated `POST /api/v1/auth/logout` route, whose server-side
authorization derives the exact global bearer row from the request and revokes
only that row transactionally. It cannot select a token, app, browser session,
or viewer session. Only after successful revocation—or an unauthorized response
proving the bearer is already unusable—does the CLI remove the matching local
credential. A network, server, persistence, or local removal failure retains
the local credential. The default platform URL remains, allowing the deployer
to run `tiny login` without repeating `--server`. The route uses the existing
global `app:read` bearer boundary and requires an unbound token row, so an
app-scoped deployment-agent bearer cannot perform this CLI action.

## Consequences

The CLI becomes predictable across macOS, desktop Linux, headless Linux, and
minimal developer environments without relying on a separately configured
credential-store command. It preserves the normal expectation that login is a
one-time action until the token expires or is revoked.

The tradeoff is explicit: unlike a native OS credential store, the bearer is
readable by the same OS account that runs `tiny`. Mode checks, no-symlink
handling, bounded exact JSON, and atomic replacement reduce accidental
exposure and corruption, but do not protect against a compromised same-user
process. Therefore tokens remain narrowly scoped, time-bounded, and promptly
revocable by server policy; local-file protection never substitutes for those
server-side controls.

No browser cookie, app-viewer credential, global viewer identity credential,
or operator/provider secret is stored in this file. Non-interactive deployment
agent tokens retain their separate explicit creation and handling workflow.

## Alternatives considered

- **Native OS credential stores:** stronger same-user-at-rest isolation where
  available, but command availability and interaction failures make the basic
  cross-platform deployer login unreliable.
- **Plain project-local token files:** rejected because they are easy to commit,
  copy into deployment bundles, or share across unrelated projects.
- **Browser-only/device login:** may improve the future OTP experience but does
  not solve predictable local CLI bearer persistence on its own.
