---
name: check-public-readiness
description: Audit a Git repository before changing it from private to public. Use when Codex needs to find committed or historical secrets, personal or infrastructure data, sensitive filenames, large or unexplained binaries, local-path residue, licensing and asset-provenance gaps, missing community files, unsafe GitHub settings, or other open-source publication blockers; also use for a final go/no-go public-repository review.
---

# Check Public Readiness

Treat public source, installable releases, and production readiness as separate
decisions. Audit without changing repository visibility, deleting history,
rotating credentials, or modifying GitHub settings unless the operator asks for
those actions explicitly.

## Workflow

1. Read repository instructions and product principles before judging content.
2. Preserve the current worktree. Record `git status --short --branch`; do not
   clean, reset, or rewrite history during an audit.
3. Run the redacting local sweep:

   ```sh
   python3 skills/check-public-readiness/scripts/repo_sweep.py . \
     --allow-email approved-public@example.com
   ```

   Omit `--allow-email` when no personal address is approved. The script scans
   tracked and non-ignored candidates, sensitive ignored paths, every blob
   reachable from local refs, commit identities, large blobs, and baseline
   community files. It reports locations and rule names, never matching values.
4. Run every checked-in repository scanner and its self-test. Then run an
   independent full-history and entropy-aware scanner such as Gitleaks or
   TruffleHog with redaction enabled. If no independent scanner is installed,
   record that as incomplete evidence rather than calling the repository clean.
5. Review findings manually. Classify each as:
   - `blocker`: secret, private key, real personal/tenant data, private
     infrastructure, unlicensed material, unsafe automation, or unexplained
     history;
   - `review`: intentional public identity, fixture, binary, generated file, or
     external URL that needs provenance or an allowlist decision;
   - `accepted`: documented, narrowly justified public material.
6. Inventory legal and distribution surfaces: source license, direct and
   redistributed dependency licenses, vendored code, fonts, images, audio,
   video, screenshots, design files, generated artifacts, and required notices.
7. Verify public documentation and community files, relative links, examples,
   support promises, security reporting, contribution terms, and pre-release
   claims from a clean clone of the exact candidate commit.
8. Inspect live forge settings read-only: visibility, default branch, rulesets,
   required checks, token permissions, action pinning, security analysis,
   vulnerability reporting, dependency alerts, secret scanning and push
   protection, environments, collaborators, deploy keys, webhooks, apps,
   releases, artifacts, caches, branches, tags, and open pull requests.
9. Choose a review model proportional to the repository. A sole maintainer may
   sign off a side project alone when that ownership model is explicit and the
   evidence is recorded. Require an independent reviewer only when repository
   policy, multiple-maintainer governance, regulation, or the risk profile calls
   for one; otherwise list it as optional additional assurance.
10. End with `GO`, `NO-GO`, or `GO WITH ACCEPTED RISKS`, naming the exact commit,
    unresolved blockers, accepted risks, evidence commands, and responsible
    maintainer or reviewer. Never infer `GO` from a passing pattern scan alone.

## Secret response

If a real credential appears anywhere, do not print it and do not rely on file
deletion or history rewriting. Identify only the rule, path, and reachable ref;
ask the credential owner to revoke or rotate it first. After revocation, remove
it from every reachable ref, release, artifact, cache, fork, and mirror, then
rescan the rewritten repository independently.

## History and ignored data

Ignored data is not part of a normal commit, but it remains a force-add,
archive, screen-share, and local-compromise risk. Report sensitive ignored
state separately from publication blockers. A repository with exploratory
history is not ready merely because the tip is clean: inspect or deliberately
replace all reachable history while private, and verify no remote ref, pull
request, release, artifact, cache, or fork retains it.

## Output

Lead with the decision and blockers. Separate:

- current-tree and untracked findings;
- history-only findings;
- ignored local-state risks;
- legal/asset provenance;
- documentation and clean-clone evidence;
- live forge settings;
- release-only gates that do not block public source.

Give exact file or settings locations without reproducing secret values.
