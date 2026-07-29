# Operator and deployer flow necessity audit

**Audited:** 2026-07-29
**Reviewer:** independent TinyHost concept-flow subagent
**Sources:** Principle 17, PRD D9, minimum-necessary input contract, complete
operator and deployer flow files

## Method

Every input or ceremony was classified:

- **KEEP** — necessary, unknown, and unsafe to default;
- **DERIVE** — available from bounded trusted state;
- **DEFAULT** — secure default preserves intended outcome;
- **DELAY** — ask only when the person chooses the optional branch; or
- **REMOVE** — no distinct decision is unlocked.

The reviewed HTML flows already include the accepted edits below.

## Operator result

### Kept

- acquire/control a domain;
- create a supported dedicated VPS and retain root SSH;
- own firewall policy while exposing only required 80/443;
- provide one base domain and initial operator email;
- establish Resend domain ownership and provide the provider secret through the
  root-owned credential boundary;
- enter operator OTP only when no valid control session exists;
- choose the exact active-deployer email set and confirm additions/reactivations;
- confirm app suspension; and
- provide a replacement email only when a chosen root recovery changes it.

### Derived, defaulted, delayed, or removed

- platform host, app suffix, sender, ACME contact, service identity, paths,
  internal secrets, config, database, probe hosts, and current allowlist are
  derived;
- the trusted release source is default; alternate source is advanced;
- the generic setup confirmation is removed because `tinyhost setup` already
  expresses intent;
- Resend records are collected before one combined DNS-provider visit;
- saving dashboard/recovery output is optional operator runbook practice;
- granting the operator a deployer role and sharing invitations are delayed
  until requested; and
- normal update no longer asks for config path, app slug, probe host, or URL.

## Deployer result

### Kept

- platform URL once when no verified default exists;
- deployer email and OTP only when no valid bearer exists;
- output selection only among multiple valid candidates;
- a replacement name only when safe slug derivation cannot work;
- deliberate capability/viewer/fallback/description edits;
- one final deploy action that names viewer-access broadening;
- forced account-switch email and OTP; and
- exact `delete:<slug>` confirmation for permanent app deletion.

### Derived, defaulted, delayed, or removed

- OS/architecture/install path, credential path, server reuse, identity reuse,
  project path, safe slug/output, owner-only access, stable URL, app/owner IDs,
  archive details, TLS, release IDs, and probe targets are derived;
- existing valid output is reused; absent output produces one exact
  project-owned build action, and TinyHost never runs it;
- an existing valid manifest is reused without discovery prompts or rewriting;
- capability detection may warn but never enables authority;
- optional fields live behind one review/edit action;
- separate review confirmation, manifest confirmation, and deploy confirmation
  are collapsed into one final action;
- an unchanged repeat deploy needs no confirmation; and
- logout has no extra confirmation but retains the credential on ambiguous
  server/local failure.

## Contradictions resolved

- Global viewer identity is V1 infrastructure, not a deferred central-SSO
  feature. App policy and app-local sessions remain isolated.
- Setup proves platform health, socket/route confinement, and safe unknown-app
  denial. Full active-app denial starts with the first deployment.
- Operator/deployer/control authority never doubles as viewer authentication.
- Normal server update selects denial evidence from installed state instead of
  asking for an app slug.
- The primary operator dashboard model is one exact active-deployer list, not a
  collection of per-deployer forms.

## Implementation gaps exposed by the audit

1. Implement the resumable `tinyhost setup` human assistant.
2. Complete inference-first project/output behavior and the single final deploy
   review/action.
3. Remove normal update `--app-slug` input and select/prove installed state as
   specified.
4. Add prompt-contract tests proving every question is necessary and JSON mode
   never prompts or mutates missing state.
5. Benchmark and document the smallest supported Hetzner plan.
