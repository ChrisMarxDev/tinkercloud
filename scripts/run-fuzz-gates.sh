#!/bin/sh
# Individual bounded targets avoid cross-package fuzz worker contention in CI.
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"
fuzz_time=${TINYHOST_FUZZ_TIME:-5s}
# Keep the gate usable in restricted shells as well as CI. Callers may still
# supply their own cache directory (for example, a CI cache action).
if test -z "${GOCACHE:-}"; then
  GOCACHE="${TMPDIR:-/tmp}/tinyhost-go-fuzz-cache"
  export GOCACHE
fi
mkdir -p "$GOCACHE"
run_fuzz() { go test -run='^$' -fuzz="$2" -fuzztime="$fuzz_time" "$1"; }
run_fuzz ./internal/archive FuzzClean
run_fuzz ./internal/gateway FuzzClassifyHostNeverPanics
run_fuzz ./internal/releases FuzzParseManifest
echo "bounded fuzz gates passed"
