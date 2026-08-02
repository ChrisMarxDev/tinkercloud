# Minimum-necessary human input contract

## Rule

A human command may ask a question only when all are true:

1. the value or decision is required before the requested outcome can continue;
2. current trusted local/server state does not already contain it;
3. it cannot be discovered from the relevant bounded environment;
4. no secure default preserves the user’s stated intent; and
5. the answer can be collected without weakening a credential, authorization,
   path, or automation boundary.

Optional values appear behind one review/edit choice. A derived value is shown
before mutation but does not become an individual prompt. Authorization
broadening is named by the same final action that performs it. A single human
intent must not become separate review, generated-file, and mutation
confirmations.

## Human setup

`tinkercloud setup` discovers supported host facts and asks for one controlled base
domain plus the initial operator email. It derives conventional platform/app
hosts, sender, and the internal ACME contact from that normalized operator email,
along with canonical paths, service identity, limits, and internal-secret
locations. It persists only validated non-secret resumable progress. Neither
human setup nor deterministic initialization accepts a separate ACME-contact
flag, question, or environment input.

Missing DNS, selected email-provider verification, firewall, or certificate
state produces one exact external action and a continuation instruction. Provider secrets enter
only the root-owned credential boundary through a protected file or an audited
no-echo prompt; they never enter argv, ordinary config, logs, audit, browser
state, or deployed bytes.
Tinkercloud and provider-supplied DNS records are grouped into one DNS-provider
checkpoint when their prerequisite data can be collected first.

## Human deploy

`tinker deploy [DIR]` defaults to the current directory, reuses a verified default
platform and current bearer, and inspects a bounded non-symlinked project. A
valid directory-derived slug and one unambiguous conventional output directory
need no prompt. Owner-only is the access default. Description, viewer rules,
capabilities, and SPA fallback are optional review/edit choices.

When no manifest exists, the CLI writes the strict `tinker.yaml` atomically
without overwriting after the one final deploy action. Manifest creation has no
separate confirmation. The manifest is the durable receipt and future
automation input.

## Automation

JSON and non-interactive modes never prompt, infer an absent platform, request
OTP, create credentials, generate a manifest, or persist missing setup state.
They require explicit validated inputs and produce deterministic output.

## Denial charter

- Unknown, duplicate, malformed, redirected, non-HTTPS, incompatible, or
  ambiguously failed platform state cannot be cached or used.
- Project traversal, symlink, ambiguous output, invalid slug, missing build
  output, or unproven capability cannot be silently accepted.
- No wizard may infer public access, viewer rules, deployer authority, provider
  permission, app identity, or owner identity.
- A separate ACME-contact input, including a legacy flag or VPS-test environment
  variable, must be rejected or ignored before host mutation; the persisted
  ACME contact must equal the normalized initial operator email. Deployer flows
  never accept certificate, key, or ACME-contact data.
- Secrets in argv/ordinary config, echoing prompts, logs, audit, or generated
  manifest deny setup.
- A resumable assistant cannot treat partial/unverified external state as
  complete or reclassify a prior failure as success.
- A generated config/manifest cannot be reported written until atomic
  persistence and permission checks succeed.
- Every added prompt requires a regression test proving why discovery,
  verified reuse, and a secure default are insufficient.
