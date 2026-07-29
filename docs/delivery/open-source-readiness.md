# Open-source readiness gate

**Status:** Run this checklist immediately before making the repository public.

This is a publication gate, not evidence that TinyHost is production-ready.
Public source and a public installable release are separate decisions. The
repository may become public while clearly marked pre-release; publishing
packages, binaries, or production-readiness claims requires the additional
release gate below.

Do not mark a section complete from memory. Record the exact commit, command,
URL, or GitHub setting that proves each item.

## Accepted publication decisions

- [x] Apache-2.0 is the source license.
- [x] `dev@christopher-marx.de` may appear in public documentation and Git
  commit metadata.
- [x] Real staging domains, VPS identifiers, viewer identities, credentials,
  OTPs, and local absolute paths are not approved for publication.
- [x] Repository examples and tests use reserved `.example`, `.test`, and
  documentation IP ranges.
- [x] TinyHost may be published as explicitly pre-release source before its V1
  production gates pass.
- [x] The final public history will be created only after the last product and
  namespace rename, as one reviewed initial commit.

## 1. Freeze the exact publication candidate

- [ ] Stop concurrent repository edits and automation.
- [ ] Finish or deliberately discard every uncommitted workstream.
- [ ] Confirm `git status --short --branch` is clean.
- [ ] Fetch and inspect every remote branch and tag.
- [ ] Record the exact candidate tree SHA.
- [ ] Confirm generated build output, local VPS state, package installs,
  independent checkouts, editor state, and test credentials are ignored rather
  than staged.
- [ ] Confirm the repository contains no accidental binary archive. Every
  intentional binary or archive has a documented purpose and inspection path.

Evidence:

```text
Candidate tree:
Reviewer:
Date:
```

## 2. Complete the final namespace and product rename

- [ ] Decide the public repository owner and name.
- [ ] Align the Git remote, `go.mod` module path, internal Go imports, README
  clone commands, issue/security URLs, package metadata, release metadata,
  installer URLs, Homebrew formula, npm package, and JSR package.
- [ ] Confirm the CLI, server binary, SDK package, cookie names, service name,
  config paths, and documentation use the final intended names.
- [ ] Claim every required organization, npm, JSR, Homebrew, and package
  namespace before describing it as available.
- [ ] Search for superseded names and classify every remaining match as an
  intentional compatibility alias or remove it.

Evidence:

```text
Repository:
Go module:
npm package:
JSR package:
CLI/server names:
```

## 3. Create the final initial history

Perform this while the repository is private.

- [ ] Preserve the reviewed candidate tree before changing Git history.
- [ ] Replace the exploratory history with one reviewed initial commit after
  the final rename.
- [ ] Use only the approved public author name and email.
- [ ] Ensure no old branch, tag, pull-request ref, release, artifact, cache, or
  fork keeps the exploratory history reachable.
- [ ] Push the new initial branch only after comparing its tree to the preserved
  candidate tree.
- [ ] Fetch again and prove local `HEAD` equals the intended remote default
  branch.

History rewriting never remediates a real leaked secret. If any credential was
ever exposed, rotate or revoke it before continuing.

Evidence:

```text
Initial commit:
Tree comparison:
Reachable refs reviewed:
```

## 4. Privacy and secret audit

Audit both the final tree and every reachable object after the history rewrite.

- [ ] Run `task security:secrets`.
- [ ] Run the scanner deny/allow self-tests.
- [ ] Run an independent full-reference and entropy-aware scanner such as
  Gitleaks or TruffleHog.
- [ ] Search commit metadata, paths, text, archives, images, SVG metadata, and
  editable design files for personal or infrastructure information.
- [ ] Confirm there are no real staging domains, IP addresses, VPS hostnames,
  viewer identities, OTPs, session values, API keys, signing private keys,
  local absolute paths, logs, database files, environment files, or
  `known_hosts` files.
- [ ] Confirm every credential-shaped test value is an unmistakable fixture or
  placeholder.
- [ ] Confirm `packaging/release-public-key.pem` is public verification
  material and that its corresponding private key is absent everywhere.
- [ ] Review Git author/committer names and emails.

Approved personal data:

```text
dev@christopher-marx.de
```

Evidence:

```text
Repository scanner:
Independent scanner:
Manual searches:
```

## 5. Legal and asset provenance

- [ ] Confirm GitHub recognizes the root Apache-2.0 `LICENSE`.
- [ ] Review direct and redistributed dependency licenses.
- [ ] Generate complete third-party notices for every distributed binary,
  installer, npm/JSR package, and vendored asset that requires them.
- [ ] Confirm every logo, icon, screenshot, font, illustration, example,
  editable canvas, and copied design has original authorship or documented
  redistribution rights.
- [ ] Confirm contributions are accepted under the documented Apache-2.0 terms.
- [ ] Add a `NOTICE` file if the final dependency or asset audit requires one.

Evidence:

```text
Dependency inventory:
Asset review:
NOTICE decision:
```

## 6. Documentation and community surface

- [ ] README accurately describes the current pre-release state, supported
  platforms, prerequisites, build commands, limitations, and absence of
  production guarantees.
- [ ] `SECURITY.md` points to a working private reporting path and lists the
  actually supported versions.
- [ ] `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `SUPPORT.md`, `GOVERNANCE.md`,
  `CHANGELOG.md`, issue forms, pull-request template, and `CODEOWNERS` use the
  final repository namespace.
- [ ] The documentation index and every relative link resolve.
- [ ] Public examples compile and use only reserved identities and domains.
- [ ] Concept, PRD, contracts, ADR index, skills, examples, and implementation
  agree on current behavior and explicitly label target-only behavior.
- [ ] Remove internal conversation residue, obsolete audits, misleading
  implementation claims, and local-machine instructions.
- [ ] Confirm contact and response-time promises are realistic.

Evidence:

```text
Link check:
Documentation review:
Example verification:
```

## 7. Clean-clone verification

Use a new temporary directory and only documented public prerequisites.

- [ ] Clone the exact final initial commit without local caches or untracked
  files.
- [ ] Run `task setup`.
- [ ] Run `task build`.
- [ ] Run `task check`.
- [ ] Verify the release and installer denial tests.
- [ ] Verify the SDK package-content and clean-consumer install/import tests.
- [ ] Confirm no test depends on private DNS, a private VPS, local credentials,
  an unpublished package, or an absolute workstation path.
- [ ] Record the supported Go, Node.js, npm, Task, OS, and architecture
  versions actually used.

Evidence:

```text
Clean-clone commit:
Environment:
task build:
task check:
```

## 8. GitHub security and governance settings

- [ ] Repository description, homepage, topics, social preview, license, and
  default branch are correct.
- [ ] Enable private vulnerability reporting and verify its link while the
  repository is still private.
- [ ] Enable the dependency graph, Dependabot alerts, Dependabot security
  updates, secret scanning, and push protection.
- [ ] Keep Actions default token permissions read-only.
- [ ] Add an active `main` ruleset requiring pull requests, required CI,
  resolved review conversations, and blocking force pushes and branch deletion.
- [ ] Require code-owner review when another maintainer exists.
- [ ] Protect release environments and signing/publishing secrets.
- [ ] Review collaborators, teams, bypass actors, deploy keys, webhooks,
  installed GitHub Apps, Actions secrets, environments, and organization
  policies.
- [ ] Disable the wiki unless it has an explicit owner and purpose.
- [ ] Decide whether Discussions is enabled and who moderates it.
- [ ] Configure triage labels and milestones, then test every issue form.
- [ ] Confirm forks cannot expose secrets through untrusted pull-request
  workflows.

Evidence:

```text
Ruleset:
Required checks:
Security features:
Access review:
```

## 9. Public-source launch

- [ ] Obtain a second-person review of the exact tree, history, settings, and
  privacy audit.
- [ ] Change visibility only after sections 1–8 are complete.
- [ ] From a signed-out browser, verify the repository, README, license,
  clone/build path, issue forms, and private security-report route.
- [ ] Verify CI on a public pull request from a fork without exposing secrets or
  granting write permissions.
- [ ] Re-run the secret scan after visibility changes.
- [ ] Record the public commit and launch date.

Evidence:

```text
Public commit:
Launch date:
Signed-out verification:
```

## 10. Additional gate for public packages or installable releases

Do not infer completion from the repository becoming public.

- [ ] Complete the applicable M0–M5 release evidence.
- [ ] Publish only from a reviewed tag pointing at the public source commit.
- [ ] Verify checksums, signatures, provenance, compatibility metadata,
  third-party notices, and release notes.
- [ ] Bind npm/JSR publishing to reviewed trusted-publisher workflows.
- [ ] Protect release environments and require explicit maintainer approval.
- [ ] Run a clean install/update/uninstall test for every supported platform.
- [ ] Run the destructive VPS acceptance suite on an acknowledged disposable
  host.
- [ ] Prove anonymous, wrong-app, revoked, suspended, malformed, and
  dependency-failure denial paths for the release candidate.
- [ ] Keep production-readiness claims narrower than the evidence.

Evidence:

```text
Release tag:
Artifacts:
VPS acceptance:
Denial evidence:
```

## Final sign-off

```text
Candidate commit:
Candidate tree:
Public repository URL:
Source-publication reviewer:
Security/privacy reviewer:
Release reviewer (if applicable):
Outstanding accepted risks:
Decision: GO / NO-GO
Date:
```
