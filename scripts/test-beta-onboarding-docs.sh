#!/bin/sh
set -eu

# This gate prepares the next immutable public-beta documentation independently
# of the package source bump owned by the release package change.
version=0.1.6
release_url="https://github.com/ChrisMarxDev/tinkercloud/releases/download/v${version}"
host_install="curl --proto '=https' --proto-redir '=https' --tlsv1.2 -fsSL ${release_url}/install-host.sh | sh"
client_install="curl --proto '=https' --tlsv1.2 -fsSL ${release_url}/install-client.sh | sh"
sdk_install="npm install @tinkercloud/sdk@${version}"
failed=0

require() {
  file=$1
  text=$2
  grep -F -- "$text" "$file" >/dev/null || {
    echo "beta onboarding documentation missing from ${file}: ${text}" >&2
    failed=1
  }
}

require_text() {
  label=$1
  text=$2
  needle=$3
  printf '%s\n' "$text" | grep -F -- "$needle" >/dev/null || {
    echo "beta onboarding documentation missing from ${label}: ${needle}" >&2
    failed=1
  }
}

require_phrase() {
  label=$1
  text=$2
  phrase=$3
  compact=$(printf '%s\n' "$text" | tr '\n' ' ')
  printf '%s\n' "$compact" | grep -F -- "$phrase" >/dev/null || {
    echo "beta onboarding documentation missing from ${label}: ${phrase}" >&2
    failed=1
  }
}

require_block() {
  label=$1
  text=$2
  block=$3
  case "$text" in
    *"$block"*) ;;
    *)
      echo "beta onboarding documentation missing ordered operator-flow block in ${label}: ${block}" >&2
      failed=1
      ;;
  esac
}

require_ordered_operator_flow() {
  file=$1
  block=$(sed -n '/<!-- beta-operator-clean:start -->/,/<!-- beta-operator-clean:end -->/p' "$file")
  remaining=$block
  test -n "$block" || {
    echo "beta onboarding documentation missing bounded operator flow in ${file}" >&2
    failed=1
    return
  }
  while IFS= read -r step; do
    case "$remaining" in
      *"$step"*) remaining=${remaining#*"$step"} ;;
      *)
        echo "beta onboarding documentation missing or reordered operator-flow step in ${file}: ${step}" >&2
        failed=1
        return
        ;;
    esac
  done <<EOF
$operator_flow_steps
EOF
}

require_operator_completion_item() {
  file=$1
  label=$2
  needle=$3
  block=$(sed -n '/^### 8\. Verify operator completion$/,/<!-- beta-operator-clean:end -->/p' "$file")
  compact=$(printf '%s\n' "$block" | tr '\n' ' ')
  case "$compact" in
    *"$needle"*) ;;
    *)
      echo "beta onboarding documentation missing operator completion ${label} in ${file}: ${needle}" >&2
      failed=1
      ;;
  esac
}

require_ordered_readme_operator_flow() {
  block=$(sed -n '/<!-- beta-operator-readme:start -->/,/<!-- beta-operator-readme:end -->/p' "$readme")
  remaining=$block
  test -n "$block" || {
    echo "beta onboarding documentation missing bounded README operator flow" >&2
    failed=1
    return
  }
  while IFS= read -r step; do
    case "$remaining" in
      *"$step"*) remaining=${remaining#*"$step"} ;;
      *)
        echo "beta onboarding documentation missing or reordered README operator-flow step: ${step}" >&2
        failed=1
        return
        ;;
    esac
  done <<EOF
$readme_operator_flow_steps
EOF
}

require_deployer_terminal_auth_rule() {
  file=$1
  block=$(sed -n '/<!-- shared:deployer:start -->/,/<!-- shared:deployer:end -->/p' "$file")
  remaining=$block
  test -n "$block" || {
    echo "beta onboarding documentation missing deployer workflow in ${file}" >&2
    failed=1
    return
  }
  while IFS= read -r step; do
    case "$remaining" in
      *"$step"*) remaining=${remaining#*"$step"} ;;
      *)
        echo "beta onboarding documentation missing or reordered deployer authentication step in ${file}: ${step}" >&2
        failed=1
        return
        ;;
    esac
  done <<EOF
$deployer_terminal_auth_flow_steps
EOF
  terminal_block=$(printf '%s\n' "$block" | sed -n '/^Terminal CLI authentication rule:/,/^$/p')
  test -n "$terminal_block" || {
    echo "beta onboarding documentation missing terminal CLI authentication rule in ${file}" >&2
    failed=1
    return
  }
  for boundary in 'stop that deploy attempt' 'Do not retry `tinker login`' 'use `--force`' 'switch account or identity' 'log out' 'delete or clear saved credentials' 'alternate OTP path' 'valid exact server-scoped saved identity uses zero OTP' 'directly in the CLI, never chat'; do
    require_phrase "$file" "$terminal_block" "$boundary"
  done
}

require_deployer_single_invocation_rule() {
  file=$1
  block=$(sed -n '/<!-- shared:deployer:start -->/,/<!-- shared:deployer:end -->/p' "$file")
  remaining=$block
  while IFS= read -r step; do
    case "$remaining" in
      *"$step"*) remaining=${remaining#*"$step"} ;;
      *)
        echo "beta onboarding documentation missing or reordered deployer invocation step in ${file}: ${step}" >&2
        failed=1
        return
        ;;
    esac
  done <<EOF
$deployer_single_invocation_flow_steps
EOF
  invocation_block=$(printf '%s\n' "$block" | sed -n '/^One-invocation deployment rule:/,/^$/p')
  test -n "$invocation_block" || {
    echo "beta onboarding documentation missing one-invocation deployment rule in ${file}" >&2
    failed=1
    return
  }
  for boundary in 'invoke `tinker deploy .`' 'exactly once for that deploy attempt' 'Any CLI deploy outcome' 'do not rerun deploy' 'upload another release' 'retry from chat' 'already-bounded transient readiness retries' 'active_but_unverified' 'independent exact-URL recheck' 'never a second deployment' 'explicit new human request'; do
    require_phrase "$file" "$invocation_block" "$boundary"
  done
}

require_deployer_executable_command_placement() {
  file=$1
  result=$(awk '
    /^### 5\. Deploy and verify$/ { final_review = NR }
    /^```/ { fenced = !fenced; next }
    fenced && ($0 == "tinker deploy ." || $0 == "tinker deploy --confirm-public .") {
      command_count++
      if (!final_review || NR < final_review) early_command = $0
    }
    END {
      if (early_command != "") {
        printf "early:%s\\n", early_command
      } else if (command_count != 1) {
        printf "count:%d\\n", command_count
      }
    }
  ' "$file")
  case "$result" in
    early:*)
      echo "beta onboarding documentation has executable deploy command before final review in ${file}: ${result#early:}" >&2
      failed=1
      ;;
    count:*)
      echo "beta onboarding documentation must contain exactly one executable deploy command in ${file}; found ${result#count:}" >&2
      failed=1
      ;;
  esac
}

reject() {
  file=$1
  text=$2
  ! grep -F -- "$text" "$file" >/dev/null || {
    echo "beta onboarding documentation contains forbidden text in ${file}: ${text}" >&2
    failed=1
  }
}

reject_text() {
  label=$1
  text=$2
  needle=$3
  ! printf '%s\n' "$text" | grep -F -- "$needle" >/dev/null || {
    echo "beta onboarding documentation contains forbidden text in ${label}: ${needle}" >&2
    failed=1
  }
}

require_only_exact_command_lines() {
  label=$1
  text=$2
  marker=$3
  expected=$4
  lines=$(printf '%s\n' "$text" | grep -F -- "$marker" || true)
  test -n "$lines" || {
    echo "beta onboarding documentation missing ${marker} command in ${label}" >&2
    failed=1
    return
  }
  while IFS= read -r line; do
    test "$line" = "$expected" || {
      echo "beta onboarding documentation has mutable or wrong ${marker} command in ${label}: ${line}" >&2
      failed=1
    }
  done <<EOF
$lines
EOF
}

reject_other_exact_beta_versions() {
  file=$1
  lines=$(grep -E 'releases/(download|tag)/v0\.1\.[0-9]+|@tinkercloud/(sdk|cli)@0\.1\.[0-9]+|tinkercloud-sdk-0\.1\.[0-9]+' "$file" || true)
  if printf '%s\n' "$lines" | grep -Ev '0\.1\.6' >/dev/null; then
    echo "beta onboarding documentation has a non-0.1.6 exact release/package reference in ${file}" >&2
    failed=1
  fi
}

platform=skills/tinkercloud-platform/SKILL.md
operator=skills/tinkercloud-operator/SKILL.md
deployer=skills/tinkercloud-deployer/SKILL.md
full_stack=skills/tinkercloud-full-stack-test/SKILL.md
readme=README.md
changelog=CHANGELOG.md
readme_beta=$(sed -n '/^## Start the beta$/, /^## Install the Tinker CLI$/p' "$readme")
setup_prompt_block='1. `Base domain:`
2. `Operator email:`
3. `Verified Resend sender email:`
4. `Root-readable Resend API key file:`'
dashboard_allowlist_block='human browser OTP. In the dashboard select **Deployers**, then **Active deployer allowlist**; enter the reviewed normalized set in **Allowed deployer emails**.
When adding an email, check **I confirm that adding any email grants deployment authority.**, then select **Save active deployers**.'
host_preflight='uname -m && . /etc/os-release && printf '\''%s %s\n'\'' "$ID" "$VERSION_ID"'
host_key_phrase='VPS provider console is the trust source for the SSH host-key fingerprint'
wildcard_record='Create one `*.<DOMAIN>` wildcard record using `A`/`AAAA` or `CNAME` as the DNS provider supports.'
allowlist_phrase='Replace the initial active deployer allowlist with exactly the intended normalized deployer email set and no other addresses; the operator email alone is sufficient only when that is the exact intended set.'
operator_otp_phrase='Operator setup permits at most one human browser OTP, uses zero when a reusable browser identity is valid, and uses no CLI deployer OTP.'
installer_claim='The copy/paste installer verifies checksums and a pinned Ed25519 signature before installation.'
installer_link='https://github.com/ChrisMarxDev/tinkercloud/releases/tag/v0.1.6'
operator_version_proof='curl --no-location --fail-with-body --include --max-time 15 --max-filesize 32768 https://admin.<domain>/api/v1/version'
operator_version_evidence='included gateway headers and a response body bounded to 32768 bytes containing bounded API-version JSON'
empty_operator_boundary='Protected-app anonymous denial belongs to deployer deployment completion once an app exists; do not fabricate it during empty operator setup.'
operator_completion_commands='### 8. Verify operator completion

After setup and deployer authorization, run the local checks and record the
direct public version proof:

```sh
sudo tinkercloud status
sudo tinkercloud doctor
curl --no-location --fail-with-body --include --max-time 15 --max-filesize 32768 https://admin.<domain>/api/v1/version
```

The HTTPS request must not follow a redirect and must return included gateway headers and a response body bounded to 32768 bytes containing bounded API-version JSON. Protected-app anonymous denial belongs to deployer deployment completion once an app exists; do not fabricate it during empty
operator setup.'
deployer_terminal_auth_flow_steps='tinker whoami --server <remembered-server>
tinker login --server <remembered-server>
`tinker login` reuses a valid server-bound credential.
Terminal CLI authentication rule:'
deployer_single_invocation_flow_steps='Terminal CLI authentication rule:
One-invocation deployment rule:'
operator_flow_steps='### 1. Verify the beta host
uname -m && . /etc/os-release && printf
### 2. Verify the SSH host key
ssh-keygen -l -f /etc/ssh/ssh_host_ed25519_key.pub
### 3. Configure one wildcard DNS record
Create one `*.<DOMAIN>` wildcard record
(dig +short A admin.<DOMAIN>; dig +short AAAA admin.<DOMAIN>) | grep -q .
(dig +short A onboarding-check.<DOMAIN>; dig +short AAAA onboarding-check.<DOMAIN>) | grep -q .
### 4. Install v0.1.6
install-host.sh | sh
### 5. Transfer the Resend credential
scp -p -- "<LOCAL_CREDENTIAL_FILE>" "root@<HOST>:/root/.config/tinkercloud/resend-api-key"
### 6. Run setup
sudo tinkercloud setup
1. `Base domain:`
### 7. Authorize the exact deployer set
human browser OTP
**Deployers**
**Save active deployers**
### 8. Verify operator completion
sudo tinkercloud status
sudo tinkercloud doctor
curl --no-location --fail-with-body --include --max-time 15 --max-filesize 32768 https://admin.<domain>/api/v1/version
included gateway headers and a response body bounded to 32768 bytes containing bounded API-version JSON
Protected-app anonymous denial belongs to deployer deployment completion once an app exists'
readme_operator_flow_steps='uname -m && . /etc/os-release && printf
ssh-keygen -l -f /etc/ssh/ssh_host_ed25519_key.pub
Create one `*.<DOMAIN>` wildcard record
install-host.sh | sh
/root/.config/tinkercloud/resend-api-key
sudo tinkercloud setup
**Deployers**
**Save active deployers**
sudo tinkercloud status
sudo tinkercloud doctor
curl --no-location --fail-with-body --include --max-time 15 --max-filesize 32768 https://admin.<domain>/api/v1/version'

for file in "$platform" "$operator"; do
  require_only_exact_command_lines "$file" "$(cat "$file")" "install-host.sh" "$host_install"
done
require_only_exact_command_lines "README Start the beta" "$readme_beta" "install-host.sh" "$host_install"

for file in "$platform" "$deployer"; do
  require_only_exact_command_lines "$file" "$(cat "$file")" "install-client.sh" "$client_install"
  require_only_exact_command_lines "$file" "$(cat "$file")" "@tinkercloud/sdk@" "$sdk_install"
done
require_only_exact_command_lines "README Start the beta" "$readme_beta" "install-client.sh" "$client_install"
require_only_exact_command_lines "README Start the beta" "$readme_beta" "@tinkercloud/sdk@" "$sdk_install"

require_text "README Start the beta" "$readme_beta" "sudo tinkercloud setup"
require_text "README Start the beta" "$readme_beta" "tinkercloud status"
require_text "README Start the beta" "$readme_beta" "sudo tinkercloud doctor"
require_phrase "README Start the beta" "$readme_beta" 'Setup derives `https://admin.<domain>/login`'
require_phrase "README Start the beta" "$readme_beta" "Sign in there with one human browser OTP only when there is no reusable browser identity"
require_phrase "README Start the beta" "$readme_beta" 'record the no-redirect `https://admin.<domain>/api/v1/version` gateway proof'

for file in "$platform" "$operator"; do
  require "$file" "$host_install"
  require "$file" "sudo tinkercloud setup"
  require "$file" "https://admin.<domain>/login"
  require "$file" "tinkercloud status"
  require "$file" "sudo tinkercloud doctor"
  require "$file" "https://admin.<domain>/api/v1/version"
  require "$file" "one human browser OTP"
  require "$file" "scp -p -- \"<LOCAL_CREDENTIAL_FILE>\" \"root@<HOST>:/root/.config/tinkercloud/resend-api-key\""
  require "$file" "test -f /root/.config/tinkercloud/resend-api-key && test ! -L /root/.config/tinkercloud/resend-api-key && chown root:root /root/.config/tinkercloud/resend-api-key && chmod 0600 /root/.config/tinkercloud/resend-api-key"
  require_block "$file" "$(cat "$file")" "$setup_prompt_block"
  require_block "$file" "$(cat "$file")" "$dashboard_allowlist_block"
  require_phrase "$file" "$(cat "$file")" "external mail guide is optional troubleshooting, not required for the basic Resend beta path"
  require_phrase "$file" "$(cat "$file")" "Substitute <HOST> and <LOCAL_CREDENTIAL_FILE> with the supplied values; do not ask for them again"
  require_phrase "$file" "$(cat "$file")" "clean dedicated x86-64 Ubuntu 24.04 or 26.04 VPS"
  require "$file" "$host_preflight"
  require_phrase "$file" "$(cat "$file")" "$host_key_phrase"
  require "$file" "ssh-keygen -l -f /etc/ssh/ssh_host_ed25519_key.pub"
  require_phrase "$file" "$(cat "$file")" "Never trust or accept an ssh-keyscan result by itself"
  require "$file" "$wildcard_record"
  require "$file" "(dig +short A admin.<DOMAIN>; dig +short AAAA admin.<DOMAIN>) | grep -q ."
  require "$file" "(dig +short A onboarding-check.<DOMAIN>; dig +short AAAA onboarding-check.<DOMAIN>) | grep -q ."
  require_phrase "$file" "$(cat "$file")" "no extra certificate input"
  require_phrase "$file" "$(cat "$file")" "$allowlist_phrase"
  require_phrase "$file" "$(cat "$file")" "$operator_otp_phrase"
  require_phrase "$file" "$(cat "$file")" "$installer_claim"
  require "$file" "$installer_link"
  require "$file" "### 8. Verify operator completion"
  require_operator_completion_item "$file" "status evidence" "sudo tinkercloud status"
  require_operator_completion_item "$file" "doctor evidence" "sudo tinkercloud doctor"
  require_operator_completion_item "$file" "version command" "$operator_version_proof"
  require_operator_completion_item "$file" "header/body evidence" "$operator_version_evidence"
  require_operator_completion_item "$file" "anonymous-denial boundary" "$empty_operator_boundary"
  reject "$file" '`admin.<DOMAIN>`:'
  reject "$file" "resolve to that supplied public IP"
  require_ordered_operator_flow "$file"
done

require_text "README Start the beta" "$readme_beta" "clean dedicated x86-64 Ubuntu 24.04 or 26.04 VPS"
require_text "README Start the beta" "$readme_beta" "$host_preflight"
require_phrase "README Start the beta" "$readme_beta" "$host_key_phrase"
require_text "README Start the beta" "$readme_beta" "$wildcard_record"
require_text "README Start the beta" "$readme_beta" "(dig +short A admin.<DOMAIN>; dig +short AAAA admin.<DOMAIN>) | grep -q ."
require_text "README Start the beta" "$readme_beta" "(dig +short A onboarding-check.<DOMAIN>; dig +short AAAA onboarding-check.<DOMAIN>) | grep -q ."
require_phrase "README Start the beta" "$readme_beta" "$allowlist_phrase"
require_phrase "README Start the beta" "$readme_beta" "$operator_otp_phrase"
require_phrase "README Start the beta" "$readme_beta" "$installer_claim"
require_text "README Start the beta" "$readme_beta" "$installer_link"
require_text "README Start the beta" "$readme_beta" "sudo tinkercloud status"
require_text "README Start the beta" "$readme_beta" "sudo tinkercloud doctor"
require_text "README Start the beta" "$readme_beta" "$operator_version_proof"
require_phrase "README Start the beta" "$readme_beta" "$operator_version_evidence"
require_phrase "README Start the beta" "$readme_beta" "$empty_operator_boundary"
reject_text "README Start the beta" "$readme_beta" '`admin.<DOMAIN>`:'
reject_text "README Start the beta" "$readme_beta" "resolve to that supplied public IP"
require_ordered_readme_operator_flow

for file in "$platform" "$deployer"; do
  require "$file" "$client_install"
  require "$file" "$sdk_install"
  require "$file" "tinker version"
  require "$file" "tinker whoami --server <remembered-server>"
  require "$file" "tinker login --server <remembered-server>"
  require "$file" "owner-only"
  require "$file" "tinker deploy ."
  require "$file" "one human CLI OTP"
  require "$file" "anonymous HTML, asset, and reserved API"
  require_phrase "$file" "$(cat "$file")" "immutable deployment ID"
  require_phrase "$file" "$(cat "$file")" "protected exact app origin"
  require_phrase "$file" "$(cat "$file")" "authenticated platform-health success"
  require_phrase "$file" "$(cat "$file")" "anonymous HTML, asset, and reserved API denial with no app bytes"
  require_deployer_terminal_auth_rule "$file"
  require_deployer_single_invocation_rule "$file"
  require_deployer_executable_command_placement "$file"
done

require_text "README Start the beta" "$readme_beta" "tinker version"
require_text "README Start the beta" "$readme_beta" "tinker deploy ."
require_text "README Start the beta" "$readme_beta" "Optional: add SDK capabilities"
require_text "README Start the beta" "$readme_beta" "https://raw.githubusercontent.com/ChrisMarxDev/tinkercloud/main/skills/tinkercloud-operator/SKILL.md"
require_text "README Start the beta" "$readme_beta" "https://raw.githubusercontent.com/ChrisMarxDev/tinkercloud/main/skills/tinkercloud-deployer/SKILL.md"
require_phrase "README Start the beta" "$readme_beta" "immutable deployment ID"
require_phrase "README Start the beta" "$readme_beta" "protected exact app origin"
require_phrase "README Start the beta" "$readme_beta" "authenticated platform-health success"
require_phrase "README Start the beta" "$readme_beta" "anonymous HTML, asset, and reserved API denial with no app bytes"
reject_text "README Start the beta" "$readme_beta" "<remembered-server>"
reject_text "README Start the beta" "$readme_beta" "tinker login"

for file in "$platform" "$operator" "$deployer" "$readme"; do
  require_phrase "$file" "$(cat "$file")" "most two human OTP requests in total"
  require "$file" "suggesting a third human OTP"
  reject_other_exact_beta_versions "$file"
done

require "$full_stack" "unattended multi-identity security matrix"
require "$full_stack" "two-OTP human"
require "$full_stack" "beta smoke path"
require "$full_stack" "Reader failure stops without human fallback"

require "$changelog" "### Planned for 0.1.6"
reject "$changelog" "## [0.1.6] -"
reject "$changelog" "[0.1.6]:"

for file in "$platform" "$operator" "$deployer"; do
  reject "$file" "vVERSION"
  reject "$file" "/releases/download/latest/"
done
for forbidden in "vVERSION" "/releases/download/latest/" "/releases/download/main/" "/releases/download/master/"; do
  reject_text "README Start the beta" "$readme_beta" "$forbidden"
done

exit "$failed"
