#!/bin/sh
# Keep the post-publication installer smoke compatible with the installer's
# deliberate refusal of a nonexistent caller-selected destination directory.
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
publisher=${TINKERCLOUD_RELEASE_PUBLISHER_FILE:-"$root/scripts/publish-github-release.sh"}
test -f "$publisher" || { echo 'release publisher unavailable' >&2; exit 1; }

awk '
  /^for selector in "\$\{selectors\[@\]\}"; do$/ { in_smoke = 1; next }
  in_smoke && /^done$/ { exit }
  !in_smoke { next }
  /^  install_dir="\$RUNNER_TEMP\/tinkercloud-\$\{selector\/\/\\\/\/-\}-bin"$/ {
    saw_install_dir = 1
    created = 0
    next
  }
  /^  mkdir -p "\$install_dir"$/ {
    if (!saw_install_dir) exit 2
    created = 1
    next
  }
  /TINKER_INSTALL_DIR="\$install_dir" sh$/ {
    if (!saw_install_dir || !created) exit 3
    invoked = 1
  }
  END {
    if (!in_smoke || !saw_install_dir || !created || !invoked) exit 1
  }
' "$publisher" || {
  echo 'every public installer smoke selector must create TINKER_INSTALL_DIR before invoking the installer' >&2
  exit 1
}

echo 'release installer smoke directory contract passed'
