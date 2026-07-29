#!/bin/sh
# Generate local package-manager candidates from a verified signed release.
# This script has no publication or registry mutation operation.
set -eu

test $# -eq 7 || {
  echo "usage: $0 VERSION RELEASE_DIR OUTPUT_DIR NPM_PACKAGE FORMULA_CLASS COMMAND RELEASE_BASE" >&2
  exit 2
}
version=$1
release_dir=$2
output_dir=$3
npm_package=$4
formula_class=$5
command_name=$6
release_base=$7
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)

test ! -e "$output_dir" || { echo "distribution output already exists" >&2; exit 1; }
"$root/scripts/release-verify.sh" "$release_dir"
mkdir -p "$output_dir"
node "$root/scripts/generate-npm-cli.mjs" "$release_dir" "$output_dir/npm" "$npm_package" "$version"
node "$root/scripts/generate-homebrew-formula.mjs" "$release_dir" "$output_dir/homebrew/$command_name.rb" "$formula_class" "$command_name" "$version" "$release_base"
(
  cd "$output_dir/npm"
  npm pack --ignore-scripts --silent --pack-destination "$output_dir"
)
echo "distribution candidates written to $output_dir (not published)"
