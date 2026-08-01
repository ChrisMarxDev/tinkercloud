#!/bin/sh
# Deterministic, checked-in source scan. This recognizes only high-confidence
# private-key and common live-token shapes; it complements CI credential controls.
set -eu

root=${TINKERCLOUD_SECRET_SCAN_ROOT:-$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)}
cd "$root"

scan_file() {
  file=$1
  test -f "$file" || return 0
  grep -Iq . "$file" || return 0
  awk -v file="$file" '
    BEGIN { begin_marker = "-----" "BEGIN "; private_suffix = "PRIVATE KEY-----" }
    {
      lower = tolower($0)
      placeholder = lower ~ /(<[^>]+>|replace[-_ ]?me|changeme|your[-_ ]|example[-_ ]?(token|key|secret)|fixture[-_ ]?(token|key|secret)|test[-_ ]?token|sk-test-)/
      private_key = index($0, begin_marker) && index($0, private_suffix)
      live_token = $0 ~ /(ghp_[A-Za-z0-9]{36}|github_pat_[A-Za-z0-9_]{30,}|glpat-[A-Za-z0-9_-]{20,}|npm_[A-Za-z0-9]{20,}|sk-(proj-)?[A-Za-z0-9_-]{20,}|sk_live_[A-Za-z0-9]{16,}|rk_live_[A-Za-z0-9]{16,}|xox[baprs]-[A-Za-z0-9-]{20,}|AKIA[A-Z0-9]{16}|AIza[A-Za-z0-9_-]{35}|SG\.[A-Za-z0-9_-]{16,}\.[A-Za-z0-9_-]{16,}|re_[A-Za-z0-9]{20,})/
      # A generated TypeScript property declaration can otherwise resemble an
      # unquoted credential assignment. Keep this exception narrow: an
      # uppercase type identifier followed by a semicolon is not a value.
      type_declaration = $0 ~ /^[[:space:]]*(api[_-]?key|client[_-]?secret|access[_-]?token|auth[_-]?token|password|passwd)\??[[:space:]]*:[[:space:]]*[A-Z][A-Za-z0-9_$]*(\[\])?[[:space:]]*;[[:space:]]*$/
      credential_assignment = lower ~ /(api[_-]?key|client[_-]?secret|access[_-]?token|auth[_-]?token|password|passwd)[[:space:]]*[:=][[:space:]]*"?[a-z0-9_+\/=-]{24,}/ && !type_declaration
      if ((private_key || live_token || credential_assignment) && !placeholder) {
        printf "%s:%d: possible committed secret\\n", file, NR > "/dev/stderr"
        failed = 1
      }
    }
    END { exit failed ? 1 : 0 }
  ' "$file"
}

status=0
if test "$#" -gt 0; then
  for file in "$@"; do scan_file "$file" || status=1; done
else
  # Scan both tracked files and non-ignored candidates for the first commit.
  # Ignored local state and generated artifacts must not make CI flaky.
  git ls-files --cached --others --exclude-standard |
    while IFS= read -r file; do scan_file "$file" || exit 1; done || status=1
fi
test "$status" -eq 0 || exit "$status"
echo "version-controlled candidate secret-pattern scan passed"
