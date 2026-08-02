# Publish the Tinker CLI

This is the maintainer runbook for the native `tinker` CLI. GitHub Releases is
the canonical binary origin. `@tinkercloud/cli` is one npm-registry wrapper
used by npm, pnpm, Yarn, and Bun; those tools do not receive separate builds.

Nothing in this guide publishes the privileged `tinkercloud` server through a
JavaScript registry.

## Current activation gates

Do not dispatch the stable unified release until all of these are true:

- `https://github.com/ChrisMarxDev/tinkercloud` is publicly readable and
  release immutability is enabled;
- the beta release key has been replaced with a separately controlled
  production key across `packaging/release-public-key.pem`, both installers,
  the CLI host bootstrap, and `packaging/release-key-policy.json`;
- GitHub Environments `stable-release`, `npm-sdk`, and `npm-cli` exist with required
  reviewers and reviewed-tag deployment restrictions;
- the production private key is base64-encoded only into the
  `stable-release` Environment secret
  `TINKERCLOUD_RELEASE_SIGNING_KEY_B64`;
- the npm `@tinkercloud` scope is controlled by the operator and the local npm
  account uses 2FA; and
- both npm packages have completed the one-time bootstrap described by their
  distribution runbooks; and
- the GitHub CLI and npm CLI are authenticated for the one-time setup actions.

The stable checks intentionally fail while the committed key policy says
`beta`. Never change only the word in the policy file: rotate the actual key
and every embedded trust anchor, update the recorded DER public-key
fingerprint, and run `task release:check`.

## One-time GitHub setup

1. Make the repository public.
2. Enable immutable releases in **Settings → General → Releases**.
3. Protect version tags and `main`.
4. Create Environment `stable-release`, require a reviewer, prevent self-review
   when the plan supports it, and restrict it to `main` and protected version
   tags.
5. Create Environment `npm-cli` with the same review and tag restrictions. It
   has no npm token secret; npm authentication is OIDC.
6. Generate the Ed25519 production private key outside the checkout. Store it
   mode `0600` under an operator-controlled mode `0700` directory. Commit only
   its public key and the synchronized embedded copies.
7. Set the private key secret without placing bytes in argv or shell history:

   ```sh
   base64 <"$HOME/.tinkercloud/release/production-ed25519.pem" |
     tr -d '\n' |
     gh secret set TINKERCLOUD_RELEASE_SIGNING_KEY_B64 \
       --env stable-release
   ```

Run the non-mutating gates after setup:

```sh
task release:stable-check
task release:check
```

## Publish the first complete stable release

The first manual npm bootstrap uses the exact verified tarball with
`--provenance=false`. npm assigns its initial `latest` tag as part of that
immutable first publication; do not unpublish or mutate tags to hide it. Later
reviewed `release.yml` publications use trusted OIDC provenance and their fixed
channel tags.

After both npm packages are bootstrapped, choose one new strict numeric version. Update
`sdk/typescript/package.json`, `sdk/typescript/jsr.json`, and `SDK_VERSION` to
that exact value, merge the reviewed commit to `main`, then tag and dispatch
that exact tag. The workflow requires its caller SHA to equal the source tag
SHA.

```sh
VERSION=0.1.0
git tag -a "v$VERSION" -m "Tinkercloud v$VERSION"
git push origin "v$VERSION"
gh workflow run release.yml \
  --ref "v$VERSION" \
  -f channel=stable \
  -f version="$VERSION" \
  -f confirmation="publish-stable-v$VERSION"
```

Approve `stable-release` only after reviewing the tag and workflow. The job
builds once, verifies locally, uploads a draft, downloads and verifies the
draft, publishes it as stable/latest, and smoke-tests both installer paths.
The same unified run continues into the protected SDK and CLI npm stages after
the stable GitHub release is verified.

After success, the convenient installer is one line:

```sh
curl --proto '=https' --tlsv1.2 -fsSL https://github.com/ChrisMarxDev/tinkercloud/releases/latest/download/install-client.sh | sh
```

For an exact immutable version, use
`releases/download/vVERSION/` in both URLs.

## Bootstrap the npm package once before unified publication

npm trusted publishing cannot be configured until the package exists. The
first `@tinkercloud/cli` version is therefore one deliberate interactive 2FA
publish from an exact verified release that predates the unified flow; it is
not rebuilt locally. The existing `v0.1.2` beta is suitable and uses `next`.

Download every release asset, verify it, and derive the package:

```sh
VERSION=0.1.2
RELEASE_DIR="$(mktemp -d)"
DISTRIBUTION_DIR="$(mktemp -d)/prepared"
gh release download "v$VERSION" \
  --repo ChrisMarxDev/tinkercloud \
  --dir "$RELEASE_DIR"
task release:verify RELEASE_DIR="$RELEASE_DIR"
task distribution:prepare \
  VERSION="$VERSION" \
  RELEASE_DIR="$RELEASE_DIR" \
  OUTPUT="$DISTRIBUTION_DIR"
npm publish "$DISTRIBUTION_DIR/tinkercloud-cli-$VERSION.tgz" \
  --access public \
  --tag next
```

Complete the 2FA prompt. Then use npm 11.15.0 or newer to bind future publishes
to the exact GitHub workflow and protected Environment:

```sh
npm trust github @tinkercloud/cli \
  --repo ChrisMarxDev/tinkercloud \
  --file release.yml \
  --env npm-cli \
  --allow-publish \
  --yes
```

The npm account must have 2FA enabled. On npmjs.com, verify the trusted
publisher fields and that the only allowed action is `npm publish`.

## Publish later complete releases

The one dispatch creates the GitHub release and both npm versions:

```sh
VERSION=0.1.1
gh workflow run release.yml \
  --ref "v$VERSION" \
  -f channel=stable \
  -f version="$VERSION" \
  -f confirmation="publish-stable-v$VERSION"
```

Approve the `npm-cli` Environment. The workflow downloads and verifies the
stable release, regenerates and inspects the wrapper, rejects an existing npm
version, publishes with OIDC provenance, and installs the public exact version
into a clean prefix.

## Consumer commands after verification

All four commands install the same npm artifact:

```sh
npm install -g @tinkercloud/cli
pnpm add -g @tinkercloud/cli
yarn add --dev @tinkercloud/cli
bun add -g @tinkercloud/cli
```

Modern Yarn intentionally has no global-install model; run the project-local
binary with `yarn tinker`, or use `yarn dlx @tinkercloud/cli@latest` for a
one-off command. The package requests Yarn's unplugged layout so its selected
native executable is present on the real filesystem.

Do not advertise a channel until an anonymous clean-machine installation has
returned the expected `tinker version`. If publication partially succeeds,
do not delete, overwrite, move the tag, or re-sign. Preserve the evidence and
release the correction forward under a new version.
