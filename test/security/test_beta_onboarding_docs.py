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
