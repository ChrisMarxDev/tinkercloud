#!/usr/bin/env bash
# Runs only from the protected direct job in release.yml.  Reusable workflows
# validate source but never receive an Environment signing secret.
set -euo pipefail

channel=${1:-}
case "$channel" in beta|stable) ;; *) echo "release channel must be beta or stable" >&2; exit 1;; esac
test -n "${VERSION:-}" && test -n "${SOURCE_COMMIT:-}" && test -n "${SIGNING_KEY_B64:-}" || {
  echo "protected release inputs are unavailable" >&2; exit 1;
}

tag="v$VERSION"
current_commit=$(git rev-parse HEAD)
tag_commit=$(git rev-list -n 1 "$tag")
[[ "$current_commit" == "$SOURCE_COMMIT" && "$tag_commit" == "$SOURCE_COMMIT" ]] || {
  echo "release source changed after validation" >&2; exit 1;
}
test -z "$(git status --porcelain=v1)" || { echo "release source checkout is dirty" >&2; exit 1; }
if gh release view "$tag" --repo "$GITHUB_REPOSITORY" >/dev/null 2>&1; then
  echo "GitHub release already exists for $tag" >&2; exit 1
fi

if [[ "$channel" == beta ]]; then
  required_authority=beta
  key_label=beta
  prerelease=true
  latest=false
else
  required_authority=production
  key_label=stable
  prerelease=false
  latest=true
fi
TINKERCLOUD_REQUIRED_RELEASE_CHANNEL="$required_authority" ./scripts/check-release-key-policy.sh
./scripts/check-release-key-drift.sh

umask 077
key_file=$(mktemp "$RUNNER_TEMP/.tinkercloud-${key_label}-release-key.XXXXXX")
release_dir="$RUNNER_TEMP/tinkercloud-${key_label}-release-v$VERSION"
cleanup() { rm -f "$key_file"; }
trap cleanup EXIT HUP INT TERM
printf '%s' "$SIGNING_KEY_B64" | base64 --decode >"$key_file"
unset SIGNING_KEY_B64
chmod 0600 "$key_file"
source_date_epoch=$(git show -s --format=%ct "$SOURCE_COMMIT")
TINKERCLOUD_RELEASE_SIGNING_KEY="$key_file" SOURCE_DATE_EPOCH="$source_date_epoch" \
  ./scripts/release-build.sh "$VERSION" "$release_dir"
./scripts/release-verify.sh "$release_dir"

release_base="$GITHUB_SERVER_URL/$GITHUB_REPOSITORY/releases/download/$tag/"
notes="$RUNNER_TEMP/${key_label}-release-notes.md"
if [[ "$channel" == beta ]]; then
  title="$tag beta"
  {
    printf '# Beta release %s\n\nThis is a beta prerelease. It is not a stable or latest release.\n\n' "$tag"
    printf 'Install Tinkercloud from a fresh supported VPS root shell:\n\n```sh\n'
    printf "curl --proto '=https' --proto-redir '=https' --tlsv1.2 -fsSL %sinstall-host.sh | sh\n" "$release_base"
    printf '```\n\nInstall the workstation CLI:\n\n```sh\n'
    printf "curl --proto '=https' --tlsv1.2 -fsSL %sinstall-client.sh | sh\n" "$release_base"
    printf '```\n\nOptional workstation SSH installation after installing the CLI:\n\n```sh\ntinker host install root@HOST\n```\n\n'
    printf 'The released root installer embeds that exact immutable versioned release URL. The released CLI also derives it from its embedded build version. Development or advanced operator flows may pass '
    printf "\`--release-base %s\`" "$release_base"
    printf ' explicitly.\n\nInstall the TypeScript SDK directly from this GitHub release:\n\n```sh\n'
    printf 'npm install %stinkercloud-sdk-%s.tgz\n```\n\n' "$release_base" "$VERSION"
    printf 'Every payload is covered by the attached checksums, Ed25519 signatures, and signed release manifest.\n'
  } >"$notes"
else
  title="$tag"
  latest_base="$GITHUB_SERVER_URL/$GITHUB_REPOSITORY/releases/latest/download/"
  {
    printf '# Tinkercloud %s\n\nInstall Tinkercloud from a fresh supported VPS root shell:\n\n```sh\n' "$tag"
    printf "curl --proto '=https' --proto-redir '=https' --tlsv1.2 -fsSL %sinstall-host.sh | sh\n" "$release_base"
    printf '```\n\nInstall the Tinker CLI with the stable one-line installer:\n\n```sh\n'
    printf "curl --proto '=https' --tlsv1.2 -fsSL %sinstall-client.sh | sh\n" "$latest_base"
    printf '```\n\nReproducible exact-version installation:\n\n```sh\n'
    printf "curl --proto '=https' --tlsv1.2 -fsSL %sinstall-client.sh | sh\n" "$release_base"
    printf '```\n\nThe npm CLI package is published separately from these exact verified bytes.\n\nEvery release payload is covered by checksums, Ed25519 signatures, and the signed release manifest.\n'
  } >"$notes"
fi

mapfile -t assets < <(find "$release_dir" -maxdepth 1 -type f -print | LC_ALL=C sort)
test "${#assets[@]}" -gt 0 || { echo "verified release has no assets" >&2; exit 1; }
create_args=(release create "$tag" "${assets[@]}" --repo "$GITHUB_REPOSITORY" --verify-tag --draft --title "$title" --notes-file "$notes")
[[ "$prerelease" == true ]] && create_args+=(--prerelease)
gh "${create_args[@]}"

state=$(gh release view "$tag" --repo "$GITHUB_REPOSITORY" --json isDraft,isPrerelease,tagName --jq '[.isDraft,.isPrerelease,.tagName] | @tsv')
expected_draft=$'true\tfalse\t'
[[ "$prerelease" == true ]] && expected_draft=$'true\ttrue\t'
[[ "$state" == "$expected_draft$tag" ]] || { echo "unexpected draft release state" >&2; exit 1; }
remote_dir="$RUNNER_TEMP/tinkercloud-${key_label}-remote-v$VERSION"
mkdir -p "$remote_dir"
gh release download "$tag" --repo "$GITHUB_REPOSITORY" --dir "$remote_dir"
(cd "$release_dir" && find . -maxdepth 1 -type f -print | LC_ALL=C sort) >"$RUNNER_TEMP/local-assets.txt"
(cd "$remote_dir" && find . -maxdepth 1 -type f -print | LC_ALL=C sort) >"$RUNNER_TEMP/remote-assets.txt"
cmp "$RUNNER_TEMP/local-assets.txt" "$RUNNER_TEMP/remote-assets.txt" || { echo "draft release asset set drifted" >&2; exit 1; }
./scripts/release-verify.sh "$remote_dir"

if [[ "$latest" == true ]]; then
  gh release edit "$tag" --repo "$GITHUB_REPOSITORY" --draft=false --prerelease=false --latest
  expected_published=$'false\tfalse\t'
else
  gh release edit "$tag" --repo "$GITHUB_REPOSITORY" --draft=false --prerelease --latest=false
  expected_published=$'false\ttrue\t'
fi
state=$(gh release view "$tag" --repo "$GITHUB_REPOSITORY" --json isDraft,isPrerelease,tagName --jq '[.isDraft,.isPrerelease,.tagName] | @tsv')
[[ "$state" == "$expected_published$tag" ]] || { echo "unexpected published release state" >&2; exit 1; }

if [[ "$channel" == beta ]]; then selectors=("download/v$VERSION"); else selectors=("download/v$VERSION" "latest/download"); fi
for selector in "${selectors[@]}"; do
  install_dir="$RUNNER_TEMP/tinkercloud-${selector//\//-}-bin"
  curl --proto '=https' --tlsv1.2 --fail --silent --show-error --location --retry 5 --retry-all-errors \
    "$GITHUB_SERVER_URL/$GITHUB_REPOSITORY/releases/$selector/install-client.sh" | TINKER_INSTALL_DIR="$install_dir" sh
  [[ "$("$install_dir/tinker" version)" == "tinker $VERSION" ]] || { echo "public installer version mismatch" >&2; exit 1; }
done

if [[ "$channel" == beta ]]; then
  sdk_consumer="$RUNNER_TEMP/tinkercloud-beta-sdk-consumer"
  mkdir -p "$sdk_consumer"
  (cd "$sdk_consumer" && npm init --yes >/dev/null && npm install --ignore-scripts "$release_base/tinkercloud-sdk-$VERSION.tgz" >/dev/null && SDK_VERSION="$VERSION" node --input-type=module -e 'import { SDK_VERSION } from "@tinkercloud/sdk"; if (SDK_VERSION !== process.env.SDK_VERSION) process.exit(1);')
fi
