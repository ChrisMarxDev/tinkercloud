# Team app catalog contract

## Outcome

A person with a valid global browser identity can open a bounded catalog on the
admin host containing only apps that identity may currently discover.

## Authorization and read model

The server derives the normalized email from the global identity session. It
queries only current active apps that are either:

- private and currently matched by implicit active owner, exact-email, or
  email-domain policy; or
- currently effective public static apps.

The server never loads all app metadata and filters it in JavaScript. Anonymous,
expired, revoked, or unavailable identity/policy state returns no catalog
metadata. Opening a private app still uses the normal app-bound handoff and
rechecks current policy before release bytes are read.

Each result contains only:

- escaped slug;
- stable gateway URL;
- active immutable description when available;
- zero to eight active immutable tags; and
- current `private` or `public` display posture.

Denied app IDs, owners, rules, deployments, release hashes, capability grants,
analytics, and operational state are not returned.

## Tags and filtering

Tags are active immutable manifest metadata, never policy or capability input.
Each tag is 1–24 lowercase ASCII characters, begins and ends with a letter or
number, may contain internal hyphens, and is unique. At most eight tags exist.

The native UI renders the already-authorized complete bounded list. Optional
local search and tag controls hide only rendered authorized cards, require no
network request or persistence, keep all cards visible without JavaScript, and
announce a polite result count.

Policy revocation, app lifecycle change, or public-gate disable affects the next
catalog read. A stale browser-rendered card grants no authority to open an app.
