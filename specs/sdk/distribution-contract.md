# SDK Distribution Contract

## Scope

This contract defines the V1 browser SDK artifact independently of the tool a
consumer uses to install it. It does not authorize publication or define
registry account ownership.

## Artifact invariants

- There is one public SDK name and semantic version across every registry.
- Every distribution exposes the same single ESM import surface and public
  TypeScript types.
- The consumer runtime has no runtime or peer dependencies and no install
  lifecycle scripts.
- The SDK contains no app ID, viewer or deployer token, provider credential,
  registry credential, or long-lived secret.
- Registry choice does not alter same-origin requests, server-derived identity,
  capability authorization, error types, or compatibility negotiation.
- A release version must equal the exported SDK version and every registry
  manifest version.

## npm-registry artifact

The npm-registry tarball contains exactly:

```text
LICENSE
README.md
dist/index.d.ts
dist/index.js
package.json
```

The scoped package is explicitly public, ESM-only, tree-shakeable, and published
with provenance from a reviewed public repository. npm, pnpm, Yarn, Bun, and
Deno consume this same tarball; separate package-manager builds are forbidden.

## JSR artifact

The JSR package exposes `src/index.ts` and includes exactly the TypeScript
source, README, and Apache-2.0 license selected by `jsr.json`. It has the same
name, version, and API as the npm-registry artifact.

## Verification

Preparation must:

1. build the runtime and declarations;
2. compile public examples;
3. enforce the bundle-size limit;
4. compare package names and versions;
5. inspect the exact npm tarball file list;
6. install and import that tarball in an offline temporary consumer; and
7. pass npm and JSR publication dry runs.

Dry runs may read local files and public package metadata but must not publish,
tag, reserve a name, create a scope, or mutate registry state.

## Publication gate

npm beta publication is governed by
`specs/sdk/npm-beta-publishing-contract.md`: it comes from the exact SDK tarball
in a reviewed signed GitHub prerelease, uses the fixed `beta` dist-tag and
short-lived trusted publishing, and stores no registry token in this
repository. JSR publication remains blocked until its namespace, channel, and
trusted-publisher flow receive separate authorization.
