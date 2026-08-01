# SDK Distribution

**Status:** Prepared, not published

`@tinkercloud/sdk` is ready to be reviewed as an npm-registry package and a JSR
package. No command or workflow in this repository publishes it.

## Consumer formats

One npm artifact supports npm, pnpm, Yarn, Bun, and Deno:

```sh
npm install @tinkercloud/sdk
pnpm add @tinkercloud/sdk
yarn add @tinkercloud/sdk
bun add @tinkercloud/sdk
deno add npm:@tinkercloud/sdk
```

JSR provides the same API directly from the reviewed TypeScript source:

```sh
deno add jsr:@tinkercloud/sdk
```

These commands will work only after the corresponding first publication.

## Prepare a candidate

From `sdk/typescript`:

```sh
npm ci
npm test
npm publish --dry-run
deno publish --dry-run
```

`npm test` verifies version consistency, exact tarball contents, examples,
bundle size, and an offline install/import from the generated tarball. The two
publish commands are dry runs and do not mutate either registry.

The following values must be identical for a release:

- `package.json` version;
- `jsr.json` version;
- `SDK_VERSION` in `src/index.ts`; and
- the version passed to `scripts/release-build.sh`.

## First-publication checklist

Do not publish until all of these are complete:

1. Confirm the canonical public GitHub repository and update exact-case package
   metadata.
2. Confirm maintainers control the `@tinkercloud` scope and `sdk` package on npm
   and JSR.
3. Review the dry-run file lists and the generated npm tarball.
4. Run the complete release and secret-scanning gates from a clean checkout.
5. Create a reviewed release tag whose version matches the SDK.
6. Configure npm and JSR trusted publishers for the exact repository and
   release workflow.
7. Publish from the protected release environment with provenance.
8. Install the public version with each documented package manager and rerun
   the browser SDK examples.

Prefer registry trusted publishing with short-lived OIDC credentials. Do not
add an npm token, JSR token, `.npmrc` credential, or developer session to this
repository. A successful publication is distribution evidence only; it does
not prove Tinkercloud authorization or isolation.
