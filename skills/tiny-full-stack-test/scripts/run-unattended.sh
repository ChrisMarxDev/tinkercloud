#!/bin/sh
# Run the destructive VPS acceptance suite once, after local-only gates.
# This script reads already-exported environment values only: it never sources
# an env file, evaluates input as shell, or prints credentials.
set -eu
umask 077

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../../.." && pwd -P)
reader="$repo_root/skills/tiny-full-stack-test/scripts/read-resend-otp.py"
report_dir=${TINYHOST_VPS_UNATTENDED_REPORT_DIR:-"$repo_root/.tiny/vps/unattended-reports"}
report_path=
result=failed

usage() {
  echo "usage: $0 (uses already-exported TINYHOST_VPS_* environment)" >&2
  exit 64
}

mode() {
  stat -f %Lp "$1" 2>/dev/null || stat -c %a "$1"
}

safe_regular_file() {
  case $1 in /*) ;; *) return 1 ;; esac
  [ ! -L "$1" ] && [ -f "$1" ] && [ -O "$1" ]
}

safe_owned_directory() {
  case $1 in /*) ;; *) return 1 ;; esac
  [ ! -L "$1" ] && [ -d "$1" ] && [ -O "$1" ]
}

private_0600_file() {
  safe_regular_file "$1" && [ "$(mode "$1")" = 600 ]
}

single_line_value() {
  [ -n "$1" ] && case $1 in *'
'*|*''*|*'\000'*) return 1;; esac
}

record() {
  # The report intentionally contains only step names and outcomes.
  printf '%s=%s\n' "$1" "$2" >> "$report_path"
}

finish() {
  status=$?
  if [ -n "$report_path" ]; then
    if [ "$status" -eq 0 ] && [ "$result" = passed ]; then
      record result passed
    else
      record result failed
    fi
    printf '%s\n' "$report_path" >&2
  fi
}
trap finish EXIT HUP INT TERM

prepare_report() {
  case $report_dir in /*) ;; *) return 1 ;; esac
  mkdir -p -m 700 "$report_dir"
  [ ! -L "$report_dir" ] && [ -d "$report_dir" ] && [ -O "$report_dir" ] && [ "$(mode "$report_dir")" = 700 ]
  report_path="$report_dir/vps-e2e-$(date -u +%Y%m%dT%H%M%SZ)-$$.status"
  : > "$report_path"
  chmod 600 "$report_path"
  record status started
}

preflight() {
  [ "${TINYHOST_VPS_E2E:-}" = 1 ] || return 1
  [ -n "${TINYHOST_VPS_SSH_TARGET:-}" ] || return 1
  case $TINYHOST_VPS_SSH_TARGET in root@?*) ;; *) return 1;; esac
  case $TINYHOST_VPS_SSH_TARGET in *' '*|*'\t'*|*'\r'*|*'\n'*) return 1;; esac
  [ "${TINYHOST_VPS_ACKNOWLEDGE:-}" = "$TINYHOST_VPS_SSH_TARGET" ] || return 1
  [ "${TINYHOST_VPS_OTP_COMMAND:-}" = "$reader" ] || return 1
  [ -x "$reader" ] && [ ! -L "$reader" ] || return 1
  single_line_value "${TINYHOST_VPS_KNOWN_HOSTS_FILE:-}" || return 1
  single_line_value "${TINYHOST_VPS_DOMAIN:-}" || return 1
  single_line_value "${TINYHOST_VPS_OPERATOR_EMAIL:-}" || return 1
  single_line_value "${TINYHOST_VPS_DEPLOYER_EMAIL:-}" || return 1
  single_line_value "${TINYHOST_VPS_VIEWER_EMAIL:-}" || return 1
  single_line_value "${TINYHOST_VPS_EMAIL_FROM:-}" || return 1
  single_line_value "${TINYHOST_VPS_ACME_EMAIL:-}" || return 1
  single_line_value "${TINYHOST_VPS_RESEND_API_KEY_FILE:-}" || return 1
  single_line_value "${TINYHOST_RESEND_READER_API_KEY_FILE:-}" || return 1
  single_line_value "${TINYHOST_RESEND_OTP_LEDGER_FILE:-}" || return 1
  safe_regular_file "$TINYHOST_VPS_KNOWN_HOSTS_FILE" || return 1
  private_0600_file "$TINYHOST_VPS_RESEND_API_KEY_FILE" || return 1
  private_0600_file "$TINYHOST_RESEND_READER_API_KEY_FILE" || return 1
  private_0600_file "$TINYHOST_RESEND_OTP_LEDGER_FILE" || return 1
  if [ -n "${TINYHOST_VPS_SSH_IDENTITY_FILE:-}" ]; then
    safe_regular_file "$TINYHOST_VPS_SSH_IDENTITY_FILE" || return 1
  fi
  if [ -n "${TINYHOST_VPS_REUSE:-}" ] && [ "${TINYHOST_VPS_REUSE}" != 1 ]; then
    return 1
  fi
  if [ "${TINYHOST_VPS_REUSE:-}" = 1 ]; then
    safe_owned_directory "${TINYHOST_VPS_RELEASE_DIR:-}" || return 1
  fi
  return 0
}

run_step() {
  step=$1
  shift
  if "$@"; then
    record "$step" passed
  else
    record "$step" failed
    return 1
  fi
}

[ "$#" -eq 0 ] || usage
prepare_report || { echo "unattended report directory is unsafe" >&2; exit 1; }
if preflight; then
  record preflight passed
else
  record preflight failed
  echo "unattended VPS preflight failed" >&2
  exit 1
fi

run_step offline_reader env PYTHONDONTWRITEBYTECODE=1 python3 "$repo_root/skills/tiny-full-stack-test/scripts/test_read_resend_otp.py"
run_step offline_skill_drift "$repo_root/scripts/check-skill-drift"
run_step offline_vps_package go test ./test/vps -skip '^TestVPSAcceptance$' -count=1
run_step offline_diff git diff --check
run_step live_vps_acceptance go test ./test/vps -run TestVPSAcceptance -count=1 -v
result=passed
