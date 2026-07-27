#!/bin/sh
set -eu
test $# -gt 0 || { echo "usage: release-checksums.sh ARTIFACT..." >&2; exit 2; }
LC_ALL=C sha256sum "$@" | LC_ALL=C sort
