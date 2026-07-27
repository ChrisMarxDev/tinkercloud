# Open-Source Readiness

**Status:** Not ready to change repository visibility

**Last audited:** 2026-07-27

This checklist covers publication hygiene. It is not evidence that TinyHost is
production-ready or that its M0–M5 release gates have passed.

## Audit results

- The root repository has no commits yet, so there is no Git history to scan or
  rewrite. Its GitHub remote is private, has no default branch, and cannot yet
  have its license or community profile recognized. Issues, Projects, and the
  wiki are enabled; Discussions are disabled.
- The version-controlled-candidate secret scan passes for the root repository,
  including non-ignored files intended for the first commit.
- The independently versioned `landing/` checkout also passes the same source
  scan. It is excluded from the root repository to prevent an accidental
  gitlink or publication of hosting metadata.
- No private-key file or high-confidence live-token pattern was found in either
  source candidate set.
- `packaging/release-public-key.pem` is the only source-candidate PEM/key file.
  It is intentionally public release-verification material.
- Local VPS acceptance configuration under `.tiny/` is ignored and its
  environment file is owner-readable only. Its values were not copied into
  this audit.
- Root runtime databases, environment files, signing keys, provider keys,
  package installs, generated SDK output, and OS/editor files are ignored.
- Direct Go dependencies and the TypeScript SDK toolchain declare permissive
  licenses compatible with Apache-2.0. The accepted dependency decision is
  recorded in ADR 0011.
- `govulncheck v1.6.0` reports zero reachable vulnerabilities. It also reports
  module advisory `GO-2026-5932` because TinyHost imports
  `golang.org/x/crypto/acme/autocert`; the advisory concerns the unimported
  `openpgp` package and has no module-wide fixed version. Re-evaluate this
  finding whenever imports or the ACME dependency change.

The in-repository scanner recognizes high-confidence credential shapes; it is
not an entropy scanner and cannot prove that arbitrary secret formats are
absent.

## Completed repository files

- [x] Apache-2.0 `LICENSE`
- [x] project status, development setup, architecture links, and safety warning
  in `README.md`
- [x] `CONTRIBUTING.md`
- [x] `CODE_OF_CONDUCT.md`
- [x] `SECURITY.md`
- [x] `SUPPORT.md`
- [x] `GOVERNANCE.md`
- [x] `CHANGELOG.md`
- [x] issue forms and pull request template
- [x] `CODEOWNERS`
- [x] CI with read-only default permissions and pinned action revisions
- [x] Dependabot configuration for Go, the SDK, and GitHub Actions
- [x] source secret scanning, deny/allow self-tests, and ignore assertions
- [x] SDK package metadata and an executable package-content check that keeps
  its README, Apache-2.0 license, declarations, and runtime module
- [x] synchronized npm and JSR manifests, public-package metadata, offline
  tarball install/import verification, and registry dry-run instructions
- [x] a Go 1.25-compatible pinned `govulncheck` release in CI

## Publication blockers

- [ ] Decide the canonical namespace. `go.mod` declares
  `github.com/tinyhost/tiny`, while the configured remote is
  `github.com/ChrisMarxDev/tiny`. Align the module path, repository location,
  SDK package ownership, README URLs, and release metadata before the first
  public version.
- [ ] Claim and configure the `@tinyhost` scope and `sdk` package on npm and JSR,
  then bind each registry's trusted publisher to the reviewed public repository
  and release workflow. No package has been published by this preparation.
- [ ] Decide whether the landing site remains a separate private checkout,
  becomes its own public repository, or is flattened into this repository.
  Preserve its current uncommitted work and hosting project metadata during
  that decision.
- [ ] Generate and ship complete third-party license notices with binary and SDK
  release artifacts. The source dependency inventory alone is not a
  redistribution notice.
- [ ] Create the initial commit, then rerun the source scan and an independent
  full-reference/history scanner before changing visibility.
- [ ] Confirm that examples, screenshots, logos, fonts, and other non-code
  assets are original or have documented redistribution rights.
- [ ] Complete the M0–M5 release evidence. Public source availability must not
  be described as a production or security certification.

## GitHub settings before publication

- [ ] Enable private vulnerability reporting and subscribe maintainers to
  security-alert notifications.
- [ ] Enable the dependency graph, Dependabot alerts, Dependabot security
  updates, secret scanning, and push protection.
- [ ] Add an active `main` ruleset that requires pull requests, the
  `release-verify` status check, resolved review conversations, and blocks
  force pushes and branch deletion. Require code-owner review when a second
  maintainer exists.
- [ ] Keep Actions' default token permissions read-only and approve elevated
  permissions per workflow. Protect release environments and signing secrets.
- [ ] Set the repository description, homepage, topics, social preview, and
  public contact path. Confirm the lead maintainer profile exposes a usable
  private conduct-reporting channel, or replace the Code of Conduct contact
  with a dedicated address/form. Confirm GitHub recognizes the Apache-2.0
  license and community profile after the first commit.
- [ ] Decide whether to enable Discussions. Disable the wiki unless it has a
  clear owner; canonical documentation belongs in the repository.
- [ ] Define triage labels and milestones matching M0–M5, then test every issue
  form and the private security-report link.
- [ ] Review collaborator access, deploy keys, webhooks, installed GitHub Apps,
  Actions secrets, environments, and branch/ruleset bypass lists.
- [ ] Publish only from a reviewed tag and attach checksums, signatures,
  provenance, licenses, and release notes.

## Final publication gate

Immediately before changing visibility:

1. Freeze changes and fetch every remote reference.
2. Scan the working tree, index, all branches, tags, reflog-reachable objects,
   large files, archives, generated artifacts, and commit metadata for secrets
   and personal information.
3. Rotate any credential that was ever exposed; history rewriting alone is not
   sufficient.
4. Run CI from a clean clone using only documented prerequisites.
5. Review the exact public tree and GitHub settings with a second person.
6. Change visibility, then verify the README, license, issue forms, security
   reporting, ruleset, CI, and clone/build path as an unauthenticated visitor.
