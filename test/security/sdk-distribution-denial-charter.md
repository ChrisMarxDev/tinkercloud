# SDK Distribution Denial Charter

## Outcome

Prepare the browser SDK for reviewable publication without publishing it or
changing its same-origin security boundary.

## Trust boundary and ownership

The published SDK is untrusted browser code. TinyHost remains the sole owner of
app identity, viewer identity, authorization, and capability grants. Registry
accounts and release credentials belong to maintainers and must never enter the
package, repository, generated tarball, or browser runtime.

## Deny paths

Distribution verification fails when:

- `package.json`, `jsr.json`, and the exported `SDK_VERSION` disagree;
- either registry manifest names a different package or exposes an unexpected
  entry point;
- the npm tarball contains source, tests, credentials, local configuration, or
  anything outside the reviewed runtime, declarations, README, license, and
  manifest;
- an npm runtime dependency, consumer install lifecycle script, local
  `file:`/`link:` reference, or registry-specific runtime shim enters the
  package;
- a packed tarball cannot be installed and imported in a clean temporary
  consumer;
- distribution metadata makes a scoped npm release private by default;
- JSR publication includes anything outside the reviewed TypeScript source,
  README, and license;
- a release version differs from the SDK package version; or
- a preparation or verification command attempts to publish, tag, or otherwise
  mutate a registry.

## Positive evidence

- `npm test` builds declarations/runtime output, compiles examples, enforces the
  bundle limit, inspects exact tarball contents, checks version/manifest
  consistency, and installs/imports the tarball in a clean temporary project.
- `npm publish --dry-run` and `deno publish --dry-run` complete without a
  registry mutation.
- The root secret scan covers the source candidate and generated package
  metadata.

## Non-goals

- Claiming the `@tinyhost` namespace on npm or JSR.
- Publishing a package or configuring registry credentials.
- Adding CommonJS, Node-only, framework-specific, or CDN builds.
- Treating a successful package build as application authorization evidence.
