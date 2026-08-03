#!/usr/bin/env python3
"""Regression checks for public-beta onboarding documentation truth."""

from pathlib import Path
import re
import unittest


ROOT = Path(__file__).resolve().parents[2]
PREPARED_RELEASE = "v0.1.6"
EXAMPLE_STAGING_DOMAIN = "testing.tinkercloud.example"


class BetaOnboardingDocumentationTest(unittest.TestCase):
    def read(self, relative_path: str) -> str:
        return (ROOT / relative_path).read_text()

    def test_owned_onboarding_material_uses_only_reserved_staging_domain(self):
        files = (
            "skills/tinkercloud-deployment-agent/SKILL.md",
            "specs/agent/beta-onboarding-contract.md",
            "test/security/deployment-agent-skill-denial-charter.md",
            "test/security/beta-onboarding-denial-charter.md",
        )
        for relative_path in files:
            with self.subTest(relative_path=relative_path):
                contents = self.read(relative_path)
                self.assertIn(EXAMPLE_STAGING_DOMAIN, contents)
                platform_domains = re.findall(r"testing\.tinkercloud\.[a-z]+", contents)
                self.assertTrue(platform_domains)
                self.assertTrue(all(domain == EXAMPLE_STAGING_DOMAIN for domain in platform_domains))

    def test_primary_v016_onboarding_is_timeless_and_requires_the_exact_asset(self):
        files = (
            "README.md",
            "skills/tinkercloud-operator/SKILL.md",
            "skills/tinkercloud-deployer/SKILL.md",
            "specs/agent/beta-onboarding-contract.md",
        )
        for relative_path in files:
            with self.subTest(relative_path=relative_path):
                contents = self.read(relative_path)
                self.assertIn(PREPARED_RELEASE, contents)
                self.assertIn("installer asset", contents)
                self.assertNotIn("currently unavailable", contents)

    def test_primary_role_flows_probe_exact_tag_before_role_asset(self):
        tag_probe = "https://github.com/ChrisMarxDev/tinkercloud/releases/tag/v0.1.6"
        for relative_path, asset in (
            ("README.md", "install-client.sh"),
            ("skills/tinkercloud-operator/SKILL.md", "install-host.sh"),
            ("skills/tinkercloud-deployer/SKILL.md", "install-client.sh"),
        ):
            with self.subTest(relative_path=relative_path):
                contents = self.read(relative_path)
                self.assertLess(contents.index(tag_probe), contents.index(asset))

    def test_primary_deployer_flow_requires_release_build_version_and_one_command(self):
        files = ("README.md", "skills/tinkercloud-deployer/SKILL.md")
        for relative_path in files:
            with self.subTest(relative_path=relative_path):
                contents = self.read(relative_path)
                self.assertIn("tinker version` must print exactly `tinker 0.1.6`", contents)
                self.assertIn("Do not run standalone `tinker whoami` or `tinker login`", " ".join(contents.split()))
                self.assertIn("tinker --server <SERVER> deploy .", contents)

    def test_primary_deployer_flow_orders_manifest_before_authentication(self):
        phrase = (
            "For a fully fresh no-manifest/no-bearer deploy, the combined CLI prompt order is "
            "exactly the six manifest prompts above, then—only after manifest creation succeeds—"
            "`Email: ` and `Code: ` when authentication is needed."
        )
        for relative_path in ("README.md", "skills/tinkercloud-deployer/SKILL.md"):
            with self.subTest(relative_path=relative_path):
                self.assertIn(phrase, " ".join(self.read(relative_path).split()))

    def test_private_success_evidence_is_combined_and_asset_optional(self):
        phrase = (
            "Private combined success evidence always proves anonymous root and a representative "
            "reserved route return exact safe `401 not_authorized` denials with no app bytes. When "
            "the immutable release contains a servable non-index asset, the server candidate "
            "additionally proves denial of that actual asset; a single-file app requires no asset "
            "evidence and must not fabricate it. The independent live client probe covers root plus "
            "the representative reserved route; the private activation receipt does not expose an asset path."
        )
        for relative_path in ("README.md", "skills/tinkercloud-deployer/SKILL.md"):
            with self.subTest(relative_path=relative_path):
                self.assertIn(phrase, " ".join(self.read(relative_path).split()))

    def test_fresh_server_origin_and_inline_manifest_creation_are_unambiguous(self):
        server_phrase = (
            "On a fully fresh workstation, `<SERVER>` comes only from the operator-provided "
            "exact normalized HTTPS admin URL."
        )
        for relative_path in ("README.md", "skills/tinkercloud-deployer/SKILL.md"):
            with self.subTest(relative_path=relative_path):
                contents = " ".join(self.read(relative_path).split())
                self.assertIn(server_phrase, contents)
                self.assertIn("current CLI state cannot invent or derive a server", contents)
                self.assertIn("Never run `tinker init` before the bounded fresh single-deploy path", contents)
        self.assertNotIn("Use `tinker init .` to create the manifest ahead of time.", self.read("README.md"))

    def test_private_v2_skill_sample_omits_public_only_indexing(self):
        contents = self.read("skills/tinkercloud-deployer/SKILL.md")
        sample = contents.split("Use this V1 shape and omit unused optional sections:", 1)[1]
        sample = sample.split("```yaml", 1)[1].split("```", 1)[0]
        self.assertIn("mode: private", sample)
        self.assertNotIn("indexing:", sample)

    def test_operator_transfer_finishes_with_exact_destination_proof(self):
        contents = self.read("skills/tinkercloud-operator/SKILL.md")
        section = contents.split("### 5. Reuse or transfer the Resend credential", 1)[1]
        section = section.split("### 6. Run setup", 1)[0]
        command = next(line for line in section.splitlines() if line.startswith("ssh root@<HOST> 'chown"))
        for step in (
            "chown root:root /root/.config/tinkercloud/resend-api-key",
            "chmod 0600 /root/.config/tinkercloud/resend-api-key",
            "test -f /root/.config/tinkercloud/resend-api-key",
            "test ! -L /root/.config/tinkercloud/resend-api-key",
            "stat -c",
            "root:root 600",
        ):
            self.assertIn(step, command)

    def test_operator_status_proves_exact_build_without_masking_failure(self):
        for relative_path in ("README.md", "skills/tinkercloud-operator/SKILL.md"):
            with self.subTest(relative_path=relative_path):
                contents = self.read(relative_path)
                self.assertIn("status_output=$(sudo tinkercloud status) &&", contents)
                self.assertIn(
                    "test \"$(printf '%s\\n' \"$status_output\" | grep -Fxc 'version: 0.1.6')\" -eq 1 &&",
                    contents,
                )
                self.assertIn("printf '%s\\n' \"$status_output\"", contents)
                self.assertNotRegex(contents, r"(?m)^(?:sudo )?tinkercloud version$")

    def test_fully_fresh_human_otp_budget_is_exact_and_terminal(self):
        phrase = (
            "A completely fresh successful end-to-end onboarding with no reusable identity "
            "requests exactly two human codes total: exactly one operator dashboard OTP and exactly "
            "one deployer CLI OTP."
        )
        for relative_path in (
            "README.md",
            "skills/tinkercloud-operator/SKILL.md",
            "skills/tinkercloud-deployer/SKILL.md",
            "specs/agent/beta-onboarding-contract.md",
        ):
            with self.subTest(relative_path=relative_path):
                contents = " ".join(self.read(relative_path).split())
                self.assertIn(phrase, contents)
                self.assertIn("Any additional code, retry, account switch, or viewer login is a deployment-flow failure", contents)
                self.assertIn("A failed OTP is terminal; there is no automatic human OTP retry", contents)
                self.assertIn(
                    "The clean two-code human flow MUST NOT invoke the extended VPS security matrix. "
                    "That matrix is separate and unattended; it never authorizes asking the human for more codes.",
                    contents,
                )

    def test_first_app_clones_the_exact_prepared_tag_after_publication(self):
        contents = self.read("docs/getting-started/first-app.md")
        self.assertIn(
            "git clone --depth 1 --branch v0.1.6 https://github.com/ChrisMarxDev/tinkercloud.git tinkercloud-source",
            contents,
        )
        self.assertNotIn("git clone --depth 1 https://github.com/ChrisMarxDev/tinkercloud.git", contents)

    def test_first_app_defers_viewer_opening_without_a_reusable_browser_identity(self):
        contents = " ".join(self.read("docs/getting-started/first-app.md").split())
        self.assertIn("proven reusable exact browser identity", contents)
        self.assertIn("separate later viewer work", contents)
        self.assertIn("anonymous denial evidence", contents)
        self.assertIn("must not request a viewer/browser OTP", contents)
        self.assertNotIn("normal email identity flow grants the owner access", contents)

    def test_security_policy_supports_only_the_newest_published_prerelease(self):
        contents = self.read("SECURITY.md")
        self.assertIn("`0.1.5` prerelease", contents)
        self.assertIn("Prepared but unpublished versions, including `0.1.6`, are unsupported", contents)
        self.assertNotIn("`0.1.6` beta | Best effort", contents)


if __name__ == "__main__":
    unittest.main()
