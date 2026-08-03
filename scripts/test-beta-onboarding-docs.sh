#!/bin/sh
set -eu

# This gate prepares the next immutable public-beta documentation independently
# of the package source bump owned by the release package change.
version=0.1.6
release_url="https://github.com/ChrisMarxDev/tinkercloud/releases/download/v${version}"
host_install="curl --proto '=https' --proto-redir '=https' --tlsv1.2 -fsSL ${release_url}/install-host.sh | sh"
tag_probe="curl --proto '=https' --proto-redir '=https' --tlsv1.2 --head --location --fail --silent --show-error --max-time 15 -o /dev/null https://github.com/ChrisMarxDev/tinkercloud/releases/tag/v${version}"
client_install="curl --proto '=https' --proto-redir '=https' --tlsv1.2 -fsSL ${release_url}/install-client.sh | sh"
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

require_release_probe_order() {
  file=$1
  asset_probe=$2
  install=$3
  contents=$(cat "$file")
  case "$contents" in
    *"$tag_probe"*"$asset_probe"*"$install"*) ;;
    *)
      echo "beta onboarding documentation must probe the exact tag, then asset, before installation in ${file}" >&2
      failed=1
      ;;
  esac
  ! printf '%s\n' "$contents" | grep -F -- "--max-filesize 32768 -o /dev/null https://github.com/ChrisMarxDev/tinkercloud/releases/tag/v${version}" >/dev/null || {
    echo "beta onboarding documentation must use the bounded header-only tag probe in ${file}" >&2
    failed=1
  }
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
  for boundary in 'Do not run standalone `tinker whoami` or `tinker login` before this fresh first deployment' 'stop that deploy attempt' 'Do not retry `tinker login`' 'use `--force`' 'switch account or identity' 'log out' 'delete or clear saved credentials' 'alternate OTP path' 'valid exact server-scoped saved identity uses zero OTP'; do
    require_phrase "$file" "$terminal_block" "$boundary"
  done
}

require_supervised_agent_otp_handoff() {
  file=$1
  text=$(cat "$file")
  for boundary in \
    'Human-supervised coding-agent OTP handoff:' \
    'only when no reusable server-bound bearer exists' \
    'after endpoint, email, and manifest validation' \
    'the same CLI process reaches its normal `Code: ` prompt' \
    'ask the human exactly once for the short-lived emailed OTP' \
    'accept it in the agent interaction' \
    'immediately submit it only to that same CLI process' \
    'Use this only with a trusted human-supervised agent; its provider may retain the interaction' \
    'Do not restate it or copy it into files, source, argv, logs, summaries, or final output' \
    'never ask for a bearer' \
    'there is no second code, retry, forced login, identity/account/server switch, or alternate collection path' \
    'stores the resulting scoped bearer for later exact-server reuse' \
    'must not fall back to an agent interaction OTP'; do
    compact=$(printf '%s\n' "$text" | tr -s '[:space:]' ' ')
    printf '%s\n' "$compact" | grep -F -- "$boundary" >/dev/null || {
      echo "beta onboarding documentation missing from ${file}: ${boundary}" >&2
      failed=1
    }
  done
}

require_operator_terminal_auth_rule() {
  file=$1
  text=$(cat "$file")
  terminal_phrase='Terminal operator browser authentication rule: immediately after an operator browser OTP attempt fails, is malformed, times out, or is denied, stop operator onboarding.'
  require_phrase "$file" "$text" "$terminal_phrase"
  for boundary in 'Do not retry or request another code' 'switch operator identity or mailbox' 'clear browser cookies' 'create another OTP path' 'A valid exact browser identity uses zero OTP' 'does not alter unattended machine OTP acceptance'; do
    require_phrase "$file" "$text" "$boundary"
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
  for boundary in 'invoke `tinker --server <SERVER> deploy .`' 'exactly once for that deploy attempt' 'Any CLI deploy outcome' 'do not rerun deploy' 'upload another release' 'retry from chat' 'already-bounded transient readiness retries' 'active_but_unverified' 'independent exact-URL recheck' 'never a second deployment' 'explicit new human request'; do
    require_phrase "$file" "$invocation_block" "$boundary"
  done
}

require_deployer_executable_command_placement() {
  file=$1
  result=$(awk '
    /^### 5\. Deploy and verify$/ { final_review = NR }
    /^```/ { fenced = !fenced; next }
    fenced && ($0 == "tinker --server <SERVER> deploy .") {
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

require_operator_transfer_proof() {
  file=$1
  block=$(sed -n '/^### 5\. Reuse or transfer the Resend credential$/,/^### 6\. Run setup$/p' "$file")
  remaining=$block
  while IFS= read -r step; do
    case "$remaining" in
      *"$step"*) remaining=${remaining#*"$step"} ;;
      *)
        echo "beta onboarding documentation missing or reordered final credential-transfer proof in ${file}: ${step}" >&2
        failed=1
        return
        ;;
    esac
  done <<'EOF'
chown root:root /root/.config/tinkercloud/resend-api-key
chmod 0600 /root/.config/tinkercloud/resend-api-key
test -f /root/.config/tinkercloud/resend-api-key
test ! -L /root/.config/tinkercloud/resend-api-key
stat -c
root:root 600
EOF
}

require_private_manifest_sample() {
  file=$1
  block=$(sed -n '/Use this V1 shape and omit unused optional sections:/,/^Rules:$/p' "$file")
  require_text "${file} private manifest sample" "$block" "mode: private"
  if printf '%s\n' "$block" | grep -F -- "indexing:" >/dev/null; then
    echo "beta onboarding documentation private manifest sample contains public-only access.indexing in ${file}" >&2
    failed=1
  fi
}

reject() {
  file=$1
  text=$2
  ! grep -F -- "$text" "$file" >/dev/null || {
    echo "beta onboarding documentation contains forbidden text in ${file}: ${text}" >&2
    failed=1
  }
}

reject_exact_line() {
  file=$1
  line=$2
  ! grep -Fx -- "$line" "$file" >/dev/null || {
    echo "beta onboarding documentation contains forbidden command line in ${file}: ${line}" >&2
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

reject_phrase() {
  label=$1
  text=$2
  phrase=$3
  compact=$(printf '%s\n' "$text" | tr '\n' ' ')
  ! printf '%s\n' "$compact" | grep -F -- "$phrase" >/dev/null || {
    echo "beta onboarding documentation contains forbidden text in ${label}: ${phrase}" >&2
    failed=1
  }
}

reject_bare_beta_deploy() {
  file=$1
  ! grep -F -- 'tinker deploy .' "$file" >/dev/null || {
    echo "beta onboarding documentation contains a bare deploy command in ${file}" >&2
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
contract=specs/agent/beta-onboarding-contract.md
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
operator_version_evidence='Completion requires HTTP `200`, an `application/json` media type, and exact bounded body `{"api_version":1}` with no extra or error fields'
operator_status_version_capture='status_output=$(sudo tinkercloud status) &&'
operator_status_version_exact='test "$(printf '\''%s\n'\'' "$status_output" | grep -Fxc '\''version: 0.1.6'\'')" -eq 1 &&'
operator_status_version_print='printf '\''%s\n'\'' "$status_output"'
email_normalization_phrase='Normalize every email by trimming outer whitespace, preserving local-part case, lowercasing only the domain, requiring exactly one `@`, nonempty local/domain, a dotted domain, and no whitespace/control characters; never plus/dot rewrite.'
active_unverified_recheck="curl --include --silent --show-error --no-location --cookie '' --max-time 15 --max-filesize 32768 -H 'Accept: application/json' <returned-url>"
empty_operator_boundary='Protected-app anonymous denial belongs to deployer deployment completion once an app exists; do not fabricate it during empty operator setup.'
operator_completion_commands=$(cat <<'EOF'
### 8. Verify operator completion

After setup and deployer authorization, run the local checks and record the
direct public version proof:

```sh
status_output=$(sudo tinkercloud status) &&
test "$(printf '%s\n' "$status_output" | grep -Fxc 'version: 0.1.6')" -eq 1 &&
printf '%s\n' "$status_output"
sudo tinkercloud doctor
curl --no-location --fail-with-body --include --max-time 15 --max-filesize 32768 https://admin.<domain>/api/v1/version
```

The `&&` chain preserves a failed or unhealthy `status` exit and accepts the
installed build only when its successful output contains exactly one full
`version: 0.1.6` line. There is no `tinkercloud version` command; do not invent
one.
The HTTPS request must not follow a redirect. Completion requires HTTP `200`, an `application/json` media type, and exact bounded body `{"api_version":1}` with no extra or error fields; record included gateway headers. Protected-app anonymous denial belongs to deployer deployment completion once an app exists; do not fabricate it during empty
operator setup.
EOF
)
deployer_terminal_auth_flow_steps='Do not run standalone `tinker whoami` or `tinker login` before this fresh first deployment.
Terminal CLI authentication rule:'
deployer_single_invocation_flow_steps='Terminal CLI authentication rule:
One-invocation deployment rule:'
otp_budget_phrase='One browser OTP maximum for the operator and one CLI OTP maximum for the deployer; the two-role total is at most two and never permits two OTPs for either role'
fresh_human_otp_phrase='A fully fresh successful human onboarding with neither a reusable browser identity nor a saved CLI bearer requests exactly two codes total: exactly one operator browser code and exactly one deployer CLI code. A reusable identity reduces the relevant lane to zero.'
host_release_terminal_phrase='If the exact `v0.1.6` release or the host installer asset is unavailable, stop: do not install, deploy, or substitute another version.'
host_release_readiness_phrase='Minimal HTTPS release readiness is the exact tag page plus the host installer asset URL returning HTTPS success; the installer remains the checksum/signature authority.'
host_release_readiness_command="curl --proto '=https' --proto-redir '=https' --tlsv1.2 --location --fail --silent --show-error --max-time 15 --max-filesize 32768 -o /dev/null ${release_url}/install-host.sh"
client_release_terminal_phrase='If the exact `v0.1.6` release or the required installer asset for this role is unavailable, stop: do not install, deploy, or substitute another version.'
client_release_readiness_phrase="Minimal HTTPS release readiness is the exact tag page plus this role's exact installer asset URL returning HTTPS success; the installer remains the checksum/signature authority."
credential_reuse_phrase='credential is already on the VPS, reuse it after the exact root-only check'
deployer_command='tinker --server <SERVER> deploy .'
readme_deployer_command='tinker --server <SERVER> deploy .'
review_phrase='Review endpoint, slug, description, output, owner-only access, no features, and SPA fallback; even owner-only requires affirmative go-ahead.'
deployer_receipt_phrase='Human success prints `Deployment: <id>`, `State: active`, and `URL: <exact-origin>`'
deployer_approval_phrase='Deploy this owner-only app to <server> now? [y/N]'
combined_prompt_order_phrase='For a fully fresh no-manifest/no-bearer deploy, the combined CLI prompt order is exactly the six manifest prompts above, then—only after manifest creation succeeds—`Email: ` and `Code: ` when authentication is needed.'
fresh_server_origin_phrase='On a fully fresh workstation, `<SERVER>` comes only from the operator-provided exact normalized HTTPS admin URL.'
fresh_inline_manifest_phrase='Never run `tinker init` before the bounded fresh single-deploy path; that path generates the receipt inside its one deploy invocation.'
exact_fresh_otp_phrase='A completely fresh successful end-to-end onboarding with no reusable identity requests exactly two human codes total: exactly one operator dashboard OTP and exactly one deployer CLI OTP.'
terminal_fresh_otp_phrase='Any additional code, retry, account switch, or viewer login is a deployment-flow failure. A failed OTP is terminal; there is no automatic human OTP retry.'
separate_vps_matrix_phrase='The clean two-code human flow MUST NOT invoke the extended VPS security matrix. That matrix is separate and unattended; it never authorizes asking the human for more codes.'
private_combined_evidence_phrase='Private combined success evidence always proves anonymous root and a representative reserved route return exact safe `401 not_authorized` denials with no app bytes. When the immutable release contains a servable non-index asset, the server candidate additionally proves denial of that actual asset; a single-file app requires no asset evidence and must not fabricate it. The independent live client probe covers root plus the representative reserved route; the private activation receipt does not expose an asset path.'
operator_flow_steps='### 1. Verify the beta host
uname -m && . /etc/os-release && printf
### 2. Verify the SSH host key
ssh-keygen -l -f /etc/ssh/ssh_host_ed25519_key.pub
### 3. Configure one wildcard DNS record
Create one `*.<DOMAIN>` wildcard record
getent ahosts admin.<DOMAIN> >/dev/null
getent ahosts onboarding-check.<DOMAIN> >/dev/null
### 4. Install v0.1.6
install-host.sh | sh
### 5. Reuse or transfer the Resend credential
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
Completion requires HTTP `200`,
an `application/json` media type, and exact bounded body `{"api_version":1}`
with no extra or error fields
Protected-app'
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
  require_only_exact_command_lines "$file" "$(cat "$file")" "install-host.sh | sh" "$host_install"
done
require_only_exact_command_lines "README Start the beta" "$readme_beta" "install-host.sh | sh" "$host_install"

for file in "$platform" "$deployer"; do
  require_only_exact_command_lines "$file" "$(cat "$file")" "install-client.sh | sh" "$client_install"
  require_only_exact_command_lines "$file" "$(cat "$file")" "@tinkercloud/sdk@" "$sdk_install"
done
require_only_exact_command_lines "README Start the beta" "$readme_beta" "install-client.sh | sh" "$client_install"
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
  require "$file" "chown root:root /root/.config/tinkercloud/resend-api-key && chmod 0600 /root/.config/tinkercloud/resend-api-key"
  require_block "$file" "$(cat "$file")" "$setup_prompt_block"
  require_block "$file" "$(cat "$file")" "$dashboard_allowlist_block"
  require_phrase "$file" "$(cat "$file")" "external mail guide is optional troubleshooting, not required for the basic Resend beta path"
  require_phrase "$file" "$(cat "$file")" "Substitute <HOST> and <LOCAL_CREDENTIAL_FILE> with the supplied values; do not ask for them again"
  require_operator_transfer_proof "$file"
  require_phrase "$file" "$(cat "$file")" "clean dedicated x86-64 Ubuntu 24.04 or 26.04 VPS"
  require "$file" "$host_preflight"
  require_phrase "$file" "$(cat "$file")" "$host_key_phrase"
  require "$file" "ssh-keygen -l -f /etc/ssh/ssh_host_ed25519_key.pub"
  require_phrase "$file" "$(cat "$file")" "Never trust or accept an ssh-keyscan result by itself"
  require "$file" "$wildcard_record"
  require "$file" "getent ahosts admin.<DOMAIN> >/dev/null"
  require "$file" "getent ahosts onboarding-check.<DOMAIN> >/dev/null"
  require_phrase "$file" "$(cat "$file")" "no extra certificate input"
  require_phrase "$file" "$(cat "$file")" "$allowlist_phrase"
  require_phrase "$file" "$(cat "$file")" "$operator_otp_phrase"
  require_operator_terminal_auth_rule "$file"
  require_phrase "$file" "$(cat "$file")" "$otp_budget_phrase"
  require_phrase "$file" "$(cat "$file")" "$fresh_human_otp_phrase"
  require_phrase "$file" "$(cat "$file")" "$host_release_terminal_phrase"
  require_phrase "$file" "$(cat "$file")" "$host_release_readiness_phrase"
  require "$file" "$host_release_readiness_command"
  require_release_probe_order "$file" "$host_release_readiness_command" "$host_install"
  require_phrase "$file" "$(cat "$file")" "$credential_reuse_phrase"
  require_phrase "$file" "$(cat "$file")" "$installer_claim"
  require "$file" "$installer_link"
  require "$file" "### 8. Verify operator completion"
  require_operator_completion_item "$file" "status evidence" "sudo tinkercloud status"
  require_operator_completion_item "$file" "status version capture" "$operator_status_version_capture"
  require_operator_completion_item "$file" "exact status version line" "$operator_status_version_exact"
  require_operator_completion_item "$file" "status output evidence" "$operator_status_version_print"
  reject_exact_line "$file" "tinkercloud version"
  reject_exact_line "$file" "sudo tinkercloud version"
  require_operator_completion_item "$file" "doctor evidence" "sudo tinkercloud doctor"
  require_operator_completion_item "$file" "version command" "$operator_version_proof"
  require_operator_completion_item "$file" "header/body evidence" "$operator_version_evidence"
  require_phrase "$file" "$(cat "$file")" "$email_normalization_phrase"
  require_operator_completion_item "$file" "anonymous-denial boundary" "$empty_operator_boundary"
  reject "$file" '`admin.<DOMAIN>`:'
  reject "$file" "resolve to that supplied public IP"
  require_ordered_operator_flow "$file"
done

require_text "README Start the beta" "$readme_beta" "clean dedicated Hetzner x86-64 Ubuntu 24.04 or 26.04 VPS"
require_text "README Start the beta" "$readme_beta" "$host_preflight"
require_phrase "README Start the beta" "$readme_beta" "$host_key_phrase"
require_phrase "README Start the beta" "$readme_beta" "$wildcard_record"
require_text "README Start the beta" "$readme_beta" "getent ahosts admin.<DOMAIN> >/dev/null"
require_text "README Start the beta" "$readme_beta" "getent ahosts onboarding-check.<DOMAIN> >/dev/null"
require_phrase "README Start the beta" "$readme_beta" "$allowlist_phrase"
require_phrase "README Start the beta" "$readme_beta" "$operator_otp_phrase"
require_operator_terminal_auth_rule "$readme"
require_phrase "README Start the beta" "$readme_beta" "$otp_budget_phrase"
require_phrase "README Start the beta" "$readme_beta" "$fresh_human_otp_phrase"
require_phrase "README Start the beta" "$readme_beta" "$host_release_terminal_phrase"
require_phrase "README Start the beta" "$readme_beta" "$host_release_readiness_phrase"
require_text "README Start the beta" "$readme_beta" "$host_release_readiness_command"
require_release_probe_order "$readme" "$host_release_readiness_command" "$host_install"
require_phrase "README Start the beta" "$readme_beta" "$credential_reuse_phrase"
require_phrase "README Start the beta" "$readme_beta" "$installer_claim"
require_text "README Start the beta" "$readme_beta" "$installer_link"
require_text "README Start the beta" "$readme_beta" "sudo tinkercloud status"
require_text "README Start the beta" "$readme_beta" "$operator_status_version_capture"
require_text "README Start the beta" "$readme_beta" "$operator_status_version_exact"
require_text "README Start the beta" "$readme_beta" "$operator_status_version_print"
reject_exact_line README.md "tinkercloud version"
reject_exact_line README.md "sudo tinkercloud version"
require_text "README Start the beta" "$readme_beta" "sudo tinkercloud doctor"
require_text "README Start the beta" "$readme_beta" "$operator_version_proof"
require_phrase "README Start the beta" "$readme_beta" "$operator_version_evidence"
require_phrase "README Start the beta" "$readme_beta" "$email_normalization_phrase"
require_phrase "README Start the beta" "$readme_beta" "$empty_operator_boundary"
reject_text "README Start the beta" "$readme_beta" '`admin.<DOMAIN>`:'
reject_text "README Start the beta" "$readme_beta" "resolve to that supplied public IP"
require_ordered_readme_operator_flow

for file in "$platform" "$deployer"; do
  require "$file" "$client_install"
  require_release_probe_order "$file" "curl --proto '=https' --proto-redir '=https' --tlsv1.2 --location --fail --silent --show-error --max-time 15 --max-filesize 32768 -o /dev/null ${release_url}/install-client.sh" "$client_install"
  require "$file" "$sdk_install"
  require "$file" "tinker version"
  require_phrase "$file" "$(cat "$file")" 'tinker version` must print exactly `tinker 0.1.6`'
  require_phrase "$file" "$(cat "$file")" 'Do not run standalone `tinker whoami` or `tinker login` before this fresh first deployment'
  require "$file" "owner-only"
  require "$file" "$deployer_command"
  require "$file" "one human CLI OTP"
  require_phrase "$file" "$(cat "$file")" "immutable deployment ID"
  require_phrase "$file" "$(cat "$file")" "protected exact app origin"
  require_phrase "$file" "$(cat "$file")" "authenticated platform-health success"
  require_phrase "$file" "$(cat "$file")" "$private_combined_evidence_phrase"
  require_phrase "$file" "$(cat "$file")" "$otp_budget_phrase"
  require_phrase "$file" "$(cat "$file")" "$fresh_human_otp_phrase"
  require_phrase "$file" "$(cat "$file")" "$client_release_terminal_phrase"
  require_phrase "$file" "$(cat "$file")" "$client_release_readiness_phrase"
  require_phrase "$file" "$(cat "$file")" "$review_phrase"
  require_phrase "$file" "$(cat "$file")" "$deployer_receipt_phrase"
  require_phrase "$file" "$(cat "$file")" "$deployer_approval_phrase"
  require_phrase "$file" "$(cat "$file")" "$combined_prompt_order_phrase"
  require_phrase "$file" "$(cat "$file")" "$fresh_server_origin_phrase"
  require_phrase "$file" "$(cat "$file")" 'current CLI state cannot invent or derive a server'
  require_phrase "$file" "$(cat "$file")" "$fresh_inline_manifest_phrase"
  require_private_manifest_sample "$file"
  reject_phrase "$file" "$(cat "$file")" 'only human prompts are exactly `Email: ` and `Code: `'
  require_deployer_terminal_auth_rule "$file"
  require_supervised_agent_otp_handoff "$file"
  require_phrase "$file" "$(cat "$file")" 'after endpoint, email, and manifest validation'
  require_phrase "$file" "$(cat "$file")" 'the same CLI process reaches its normal `Code: ` prompt'
  require_deployer_single_invocation_rule "$file"
  require_deployer_executable_command_placement "$file"
  require_phrase "$file" "$(cat "$file")" "$active_unverified_recheck"
  require_phrase "$file" "$(cat "$file")" 'no `Set-Cookie` or `Location`, and no app bytes'
done

require_text "README Start the beta" "$readme_beta" "tinker version"
require_release_probe_order "$readme" "curl --proto '=https' --proto-redir '=https' --tlsv1.2 --location --fail --silent --show-error --max-time 15 --max-filesize 32768 -o /dev/null ${release_url}/install-client.sh" "$client_install"
require_text "README Start the beta" "$readme_beta" "$readme_deployer_command"
require_phrase "README Start the beta" "$readme_beta" 'tinker version` must print exactly `tinker 0.1.6`'
require_phrase "README Start the beta" "$readme_beta" 'Do not run standalone `tinker whoami` or `tinker login` before this fresh deployment'
require_phrase "README Start the beta" "$readme_beta" "$combined_prompt_order_phrase"
require_phrase "README Start the beta" "$readme_beta" "$fresh_server_origin_phrase"
require_phrase "README Start the beta" "$readme_beta" 'current CLI state cannot invent or derive a server'
require_phrase "README Start the beta" "$readme_beta" "$fresh_inline_manifest_phrase"
require_supervised_agent_otp_handoff "$readme"
reject README.md 'Use `tinker init .` to create the manifest ahead of time.'
require_text "README Start the beta" "$readme_beta" "Optional: add SDK capabilities"
require_text "README Start the beta" "$readme_beta" "https://raw.githubusercontent.com/ChrisMarxDev/tinkercloud/main/skills/tinkercloud-operator/SKILL.md"
require_text "README Start the beta" "$readme_beta" "https://raw.githubusercontent.com/ChrisMarxDev/tinkercloud/main/skills/tinkercloud-deployer/SKILL.md"
require_phrase "README Start the beta" "$readme_beta" "immutable deployment ID"
require_phrase "README Start the beta" "$readme_beta" "protected exact app origin"
require_phrase "README Start the beta" "$readme_beta" "authenticated platform-health success"
require_phrase "README Start the beta" "$readme_beta" "$private_combined_evidence_phrase"
require_phrase "README Start the beta" "$readme_beta" "$active_unverified_recheck"
require_phrase "README Start the beta" "$readme_beta" 'no `Set-Cookie` or `Location`, and no app bytes'
require "docs/getting-started/first-app.md" "$active_unverified_recheck"
require_phrase "docs/getting-started/first-app.md" "$(cat docs/getting-started/first-app.md)" 'no `Set-Cookie` or `Location`, and no app bytes'
require_phrase "specs/control/deployment-contract.md" "$(cat specs/control/deployment-contract.md)" "$active_unverified_recheck"
require_phrase "specs/control/deployment-contract.md" "$(cat specs/control/deployment-contract.md)" 'no `Set-Cookie` or `Location`, and zero app bytes'
require_phrase "specs/control/deployment-contract.md" "$(cat specs/control/deployment-contract.md)" 'a single-file app with no such asset remains valid and must not fabricate asset evidence'
require_phrase "specs/agent/role-skill-contract.md" "$(cat specs/agent/role-skill-contract.md)" 'single-file app with no such asset remains valid'
require_phrase "specs/agent/role-skill-contract.md" "$(cat specs/agent/role-skill-contract.md)" 'must not fabricate asset evidence.'

for file in "$platform" "$operator" "$deployer" README.md specs/agent/beta-onboarding-contract.md; do
  require_phrase "$file" "$(cat "$file")" "$exact_fresh_otp_phrase"
  require_phrase "$file" "$(cat "$file")" "$terminal_fresh_otp_phrase"
  require_phrase "$file" "$(cat "$file")" "$separate_vps_matrix_phrase"
done

for file in "$platform" "$operator" "$deployer" "$readme"; do
  require_phrase "$file" "$(cat "$file")" "$otp_budget_phrase"
  require "$file" "suggesting a third human OTP"
  reject_other_exact_beta_versions "$file"
done

for file in "$platform" "$operator" "$deployer" "$readme" "$contract"; do
  reject_bare_beta_deploy "$file"
done

for file in specs/agent/role-skill-contract.md specs/agent/beta-onboarding-contract.md specs/ui/native-web-system.md docs/product/web-design-system.md skills/tinkercloud-native-ui/SKILL.md test/security/beta-onboarding-denial-charter.md test/security/role-skill-denial-charter.md; do
  require_supervised_agent_otp_handoff "$file"
done

for file in "$platform" "$deployer" "$readme" specs/agent/role-skill-contract.md specs/agent/beta-onboarding-contract.md specs/ui/native-web-system.md docs/product/web-design-system.md skills/tinkercloud-native-ui/SKILL.md test/security/beta-onboarding-denial-charter.md test/security/role-skill-denial-charter.md; do
  reject "$file" 'Never ask me to paste or share a one-time code in chat.'
  reject "$file" 'Do not request, read, copy, paste, relay, or handle the code in chat.'
  reject "$file" 'Any mailed code is entered directly into the CLI rather than shared with the agent.'
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
