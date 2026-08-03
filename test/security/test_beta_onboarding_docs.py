#!/usr/bin/env python3
"""Regression checks for public-beta onboarding documentation truth."""

from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[2]
PREPARED_RELEASE = "v0.1.6"
REAL_STAGING_DOMAIN = "testing.tinkercloud.fun"
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
                self.assertNotIn(REAL_STAGING_DOMAIN, contents)
                self.assertIn(EXAMPLE_STAGING_DOMAIN, contents)

    def test_prepared_v016_commands_are_explicitly_unpublished(self):
        files = (
            "docs/operations/hetzner-deployment.md",
            "docs/getting-started/first-app.md",
            "specs/agent/beta-onboarding-contract.md",
        )
        for relative_path in files:
            with self.subTest(relative_path=relative_path):
                contents = self.read(relative_path)
                self.assertIn(PREPARED_RELEASE, contents)
                self.assertIn("only after", contents)
                self.assertIn("published", contents)

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
