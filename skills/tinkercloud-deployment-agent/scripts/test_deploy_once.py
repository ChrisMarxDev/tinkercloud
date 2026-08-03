#!/usr/bin/env python3
"""No-network deterministic tests for the Tinkercloud deployment-agent wrapper."""
from __future__ import annotations

import importlib.util
import json
import os
import pathlib
import tempfile
import unittest
import contextlib
import io
from unittest.mock import Mock, patch

HERE = pathlib.Path(__file__).parent
SPEC = importlib.util.spec_from_file_location("deploy_once", HERE / "deploy_once.py")
module = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(module)

ALLOWED_DEPLOYER_EMAIL = "dev@automation.example"
ALLOWED_DEPLOYER_DOMAIN = ALLOWED_DEPLOYER_EMAIL.split("@", 1)[1]
OTHER_ALLOWED_DOMAIN_EMAIL = "other@" + ALLOWED_DEPLOYER_DOMAIN
WRONG_ALLOWED_DOMAIN_EMAIL = "wrong@" + ALLOWED_DEPLOYER_DOMAIN


class DeploymentAgentTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.root = pathlib.Path(self.temp.name)
        self.tinker = self.root / "tinker"
        self.tinker.write_text("#!/bin/sh\nexit 0\n"); self.tinker.chmod(0o700)
        self.app = self.root / "app"; self.app.mkdir()
        (self.app / "tinker.yaml").write_text("name: demo\n"); (self.app / "tinker.yaml").chmod(0o600)
        self.old_env = dict(os.environ)
        os.environ.update({"TINKERCLOUD_RESEND_READER_API_KEY_FILE": str(self.root / "key"),
                           "TINKERCLOUD_RESEND_OTP_LEDGER_FILE": str(self.root / "ledger"),
                           "TINKERCLOUD_VPS_EMAIL_FROM": "tinker@example.test",
                           "TINKERCLOUD_VPS_DOMAIN": "example.test",
                           "TINKERCLOUD_AUTOMATION_RECIPIENT_DOMAIN": ALLOWED_DEPLOYER_DOMAIN})
        for name in ("key", "ledger"):
            path = self.root / name; path.write_text("x"); path.chmod(0o600)

    def tearDown(self):
        os.environ.clear(); os.environ.update(self.old_env); self.temp.cleanup()

    def run_with(self, responses, login=None, *, confirm_public=False):
        calls = []
        def fake(command, timeout=module.TIMEOUT_SECONDS):
            calls.append(command)
            return responses.pop(0)
        with patch.object(module, "run_json", fake):
            result = module.deploy_once(
                str(self.tinker), "https://admin.example.test", "dev@automation.example", str(self.app),
                login or (lambda *_: None), confirm_public=confirm_public,
            )
        return result, calls

    def test_exact_saved_identity_reuses_without_reader_or_otp(self):
        result, calls = self.run_with([(0, {"valid": True, "name": "dev@automation.example"}), (0, {"valid": True, "name": "https://demo.example.test/"})], lambda *_: self.fail("login invoked"))
        self.assertTrue(result["reused_saved_identity"])
        self.assertEqual([call[-1] for call in calls], ["whoami", str(self.app)])
        self.assertNotIn("--confirm-public", calls[-1])

    def test_skill_keeps_recipient_and_platform_domains_independent(self):
        skill = (HERE.parent / "SKILL.md").read_text()
        prose = " ".join(skill.split())
        self.assertIn("TINKERCLOUD_AUTOMATION_RECIPIENT_DOMAIN='christopher-marx.de'", skill)
        self.assertIn("TINKERCLOUD_VPS_DOMAIN='testing.tinkercloud.example'", skill)
        self.assertIn("--server https://admin.testing.tinkercloud.example", skill)
        self.assertNotIn("testing.tinkercloud.fun", skill)
        self.assertIn("recipient email domain may differ from the platform root domain", prose)
        self.assertIn("The wrapper derives the platform server only from `TINKERCLOUD_VPS_DOMAIN`", prose)
        self.assertIn("saved exact identity and reuses it before reader configuration is required", prose)
        self.assertNotIn("same-domain", prose)
        self.assertNotIn("same domain", prose)

    def test_public_confirmation_is_explicit_and_forwarded_once(self):
        (self.app / "tinker.yaml").write_text(
            "version: 2\nname: demo\naccess:\n  mode: public\n"
        )
        _result, calls = self.run_with([
            (0, {"valid": True, "name": "dev@automation.example"}),
            (0, {"valid": True, "name": "https://demo.example.test/"}),
        ], confirm_public=True)
        self.assertEqual(calls[-1], [
            str(self.tinker), "--json", "--server", "https://admin.example.test",
            "deploy", "--confirm-public", str(self.app),
        ])
        self.assertEqual(calls[-1].count("--confirm-public"), 1)

    def test_public_manifest_never_infers_confirmation(self):
        (self.app / "tinker.yaml").write_text(
            "version: 2\nname: demo\naccess:\n  mode: public\n"
        )
        _result, calls = self.run_with([
            (0, {"valid": True, "name": "dev@automation.example"}),
            (0, {"valid": True, "name": "https://demo.example.test/"}),
        ])
        self.assertEqual(calls[-1], [
            str(self.tinker), "--json", "--server", "https://admin.example.test",
            "deploy", str(self.app),
        ])
        self.assertNotIn("--confirm-public", calls[-1])

    def test_malformed_public_confirmation_denies_before_cli(self):
        for value in (None, 1, "true"):
            with self.subTest(value=value):
                run_json = Mock()
                with patch.object(module, "run_json", run_json):
                    with self.assertRaisesRegex(module.DeploymentError, module.INPUT_VALIDATION):
                        module.deploy_once(
                            str(self.tinker), "https://admin.example.test", "dev@automation.example",
                            str(self.app), confirm_public=value,
                        )
                run_json.assert_not_called()

    def test_malformed_or_duplicate_public_flag_denies_without_cli(self):
        for extra in ("--confirm-public=true", "--confirm-public", "--confirm-public"):
            with self.subTest(extra=extra):
                argv = [
                    "--tinker", str(self.tinker), "--server", "https://admin.example.test",
                    "--deployer-email", "dev@automation.example", "--app-dir", str(self.app),
                    "--confirm-public", extra,
                ]
                stderr = io.StringIO()
                with patch.object(module, "deploy_once", side_effect=AssertionError("must not deploy")), \
                     contextlib.redirect_stderr(stderr):
                    self.assertEqual(module.main(argv), 1)
                self.assertEqual(stderr.getvalue(), module.INPUT_VALIDATION + "\n")

    def test_allowed_deployer_domain_is_case_insensitive(self):
        responses = [(0, {"valid": True, "name": "dev@automation.example"}),
                     (0, {"valid": True, "name": "https://demo.example.test/"})]
        calls = []
        with patch.object(module, "run_json", lambda command, timeout=module.TIMEOUT_SECONDS: (calls.append(command), responses.pop(0))[1]):
            result = module.deploy_once(str(self.tinker), "https://admin.example.test", ALLOWED_DEPLOYER_EMAIL.upper(), str(self.app))
        self.assertTrue(result["reused_saved_identity"])
        self.assertEqual([call[-1] for call in calls], ["whoami", str(self.app)])

    def test_wrong_deployer_domain_denies_before_path_checks_or_cli_actions(self):
        for email in ("dev@automationexample", "dev@sub." + ALLOWED_DEPLOYER_DOMAIN,
                      "dev@" + ALLOWED_DEPLOYER_DOMAIN + ".example", "dev@example.test"):
            with self.subTest(email=email):
                saved_wrong_domain_identity = (0, {"valid": True, "name": email})
                run_json = Mock(return_value=saved_wrong_domain_identity)
                login = Mock(side_effect=AssertionError("login must not run"))
                with patch.object(module, "private_regular", side_effect=AssertionError("path check must not run")), \
                     patch.object(module, "run_json", run_json):
                    with self.assertRaisesRegex(module.DeploymentError, module.INPUT_VALIDATION):
                        module.deploy_once(str(self.tinker), "https://admin.example.test", email, str(self.app), login)
                run_json.assert_not_called()
                login.assert_not_called()

    def test_missing_or_malformed_automation_domain_denies_before_paths_or_cli(self):
        try:
            for value in (None, "", "automation.example ", "AUTOMATION.EXAMPLE",
                          "automation.example\nother.example", "localhost", "-bad.example"):
                with self.subTest(value=value):
                    if value is None:
                        os.environ.pop("TINKERCLOUD_AUTOMATION_RECIPIENT_DOMAIN", None)
                    else:
                        os.environ["TINKERCLOUD_AUTOMATION_RECIPIENT_DOMAIN"] = value
                    run_json = Mock()
                    with patch.object(module, "private_regular", side_effect=AssertionError("path check must not run")), \
                         patch.object(module, "run_json", run_json):
                        with self.assertRaisesRegex(module.DeploymentError, module.INPUT_VALIDATION):
                            module.deploy_once(str(self.tinker), "https://admin.example.test",
                                               ALLOWED_DEPLOYER_EMAIL, str(self.app))
                    run_json.assert_not_called()
        finally:
            os.environ["TINKERCLOUD_AUTOMATION_RECIPIENT_DOMAIN"] = ALLOWED_DEPLOYER_DOMAIN

    def test_wrong_identity_forces_one_login_then_deploys(self):
        logins = []
        result, calls = self.run_with([(0, {"valid": True, "name": OTHER_ALLOWED_DOMAIN_EMAIL}), (0, {"valid": True, "name": ALLOWED_DEPLOYER_EMAIL}), (0, {"valid": True, "name": "https://demo.example.test/"})], lambda *args: logins.append(args))
        self.assertFalse(result["reused_saved_identity"])
        self.assertEqual(len(logins), 1)
        self.assertEqual([call[-1] for call in calls], ["whoami", "whoami", str(self.app)])

    def test_main_emits_only_fixed_stage_and_never_exception_content(self):
        secret = "token=abc123 otp=123456 dev@automation.example /private/secret-app"
        for stage in (module.INPUT_VALIDATION, module.SAVED_IDENTITY_CHECK,
                      module.FORCED_LOGIN, module.POST_LOGIN_IDENTITY_CHECK,
                      module.DEPLOYMENT_RESULT_VALIDATION):
            stderr = io.StringIO()
            with patch.object(module, "deploy_once", side_effect=module.DeploymentError(stage)), contextlib.redirect_stderr(stderr):
                self.assertEqual(module.main(["--tinker", str(self.tinker), "--server", "https://admin.example.test", "--deployer-email", "dev@automation.example", "--app-dir", str(self.app)]), 1)
            self.assertEqual(stderr.getvalue(), stage + "\n")
            self.assertNotIn(secret, stderr.getvalue())

        stderr = io.StringIO()
        with patch.object(module, "deploy_once", side_effect=module.DeploymentError(module.FORCED_LOGIN + " " + secret)), contextlib.redirect_stderr(stderr):
            self.assertEqual(module.main(["--tinker", str(self.tinker), "--server", "https://admin.example.test", "--deployer-email", "dev@automation.example", "--app-dir", str(self.app)]), 1)
        self.assertEqual(stderr.getvalue(), module.INTERNAL_FAILURE + "\n")
        self.assertNotIn(secret, stderr.getvalue())

    def test_main_maps_arbitrary_exception_to_fixed_internal_stage_without_leaking(self):
        secret = "token=abc123 otp=123456 dev@automation.example /private/secret-app"
        stderr = io.StringIO()
        with patch.object(module, "deploy_once", side_effect=RuntimeError(secret)), contextlib.redirect_stderr(stderr):
            self.assertEqual(module.main(["--tinker", str(self.tinker), "--server", "https://admin.example.test", "--deployer-email", "dev@automation.example", "--app-dir", str(self.app)]), 1)
        self.assertEqual(stderr.getvalue(), module.INTERNAL_FAILURE + "\n")
        self.assertNotIn(secret, stderr.getvalue())

    def test_fake_cli_and_reader_force_login_without_logging_otp(self):
        state, reader = self.root / "state", self.root / "reader"
        self.tinker.write_text("""#!/usr/bin/env python3
import json, os, sys
state = os.environ['FAKE_TINKER_STATE']
command = 'login' if 'login' in sys.argv else ('whoami' if 'whoami' in sys.argv else 'deploy')
if command == 'whoami':
    if os.path.exists(state): print(json.dumps({'valid': True, 'name': 'dev@automation.example'})); raise SystemExit(0)
    print(json.dumps({'valid': False, 'error': {'code': 'not_authenticated', 'message': 'Login required.'}})); raise SystemExit(1)
if command == 'login':
    sys.stderr.write('Email: '); sys.stderr.flush()
    if sys.stdin.readline().strip() != 'dev@automation.example': raise SystemExit(1)
    sys.stderr.write('Code: '); sys.stderr.flush()
    if sys.stdin.readline().strip() != '123456': raise SystemExit(1)
    open(state, 'w').write('logged-in')
    print(json.dumps({'valid': True, 'name': 'dev@automation.example'})); raise SystemExit(0)
if command == 'deploy':
    print(json.dumps({'valid': True, 'name': 'https://demo.example.test/'})); raise SystemExit(0)
raise SystemExit(1)
"""); self.tinker.chmod(0o700)
        reader.write_text("#!/bin/sh\nprintf '123456\\n'\n"); reader.chmod(0o700)
        old = os.environ.get("FAKE_TINKER_STATE")
        os.environ["FAKE_TINKER_STATE"] = str(state)
        try:
            stdout, stderr = io.StringIO(), io.StringIO()
            with patch.object(module, "reader_path", lambda: reader), contextlib.redirect_stdout(stdout), contextlib.redirect_stderr(stderr):
                self.assertEqual(module.main(["--tinker", str(self.tinker), "--server", "https://admin.example.test", "--deployer-email", "dev@automation.example", "--app-dir", str(self.app)]), 0)
            self.assertEqual(stderr.getvalue(), "")
            self.assertNotIn("123456", stdout.getvalue())
            self.assertEqual(json.loads(stdout.getvalue()), {"valid": True, "reused_saved_identity": False, "url": "https://demo.example.test"})
        finally:
            if old is None: os.environ.pop("FAKE_TINKER_STATE", None)
            else: os.environ["FAKE_TINKER_STATE"] = old

    def test_fake_cli_reuses_exact_identity_without_invoking_reader(self):
        state, reader = self.root / "state", self.root / "reader"
        state.write_text("logged-in")
        self.tinker.write_text("""#!/usr/bin/env python3
import json, sys
if 'whoami' in sys.argv: print(json.dumps({'valid': True, 'name': 'dev@automation.example'})); raise SystemExit(0)
if 'deploy' in sys.argv: print(json.dumps({'valid': True, 'name': 'https://demo.example.test/'})); raise SystemExit(0)
raise SystemExit(99)
"""); self.tinker.chmod(0o700)
        reader.write_text("#!/bin/sh\necho reader-should-not-run >&2\nexit 99\n"); reader.chmod(0o700)
        stdout = io.StringIO()
        with patch.object(module, "reader_path", lambda: reader), contextlib.redirect_stdout(stdout):
            self.assertEqual(module.main(["--tinker", str(self.tinker), "--server", "https://admin.example.test", "--deployer-email", "dev@automation.example", "--app-dir", str(self.app)]), 0)
        self.assertEqual(json.loads(stdout.getvalue())["reused_saved_identity"], True)

    def test_deploy_failure_is_nonzero_result(self):
        with self.assertRaisesRegex(module.DeploymentError, module.DEPLOYMENT_RESULT_VALIDATION):
            self.run_with([(0, {"valid": True, "name": "dev@automation.example"}), (1, {"valid": False, "error": {"code": "deploy_failed"}})])

    def test_fixed_stage_contract_maps_each_expected_failure_boundary(self):
        with self.assertRaisesRegex(module.DeploymentError, module.INPUT_VALIDATION):
            module.deploy_once("relative-tinker", "https://admin.example.test", "dev@automation.example", str(self.app))

        with patch.object(module, "run_json", side_effect=module.DeploymentError("token=abc123")):
            with self.assertRaisesRegex(module.DeploymentError, module.SAVED_IDENTITY_CHECK):
                module.deploy_once(str(self.tinker), "https://admin.example.test", "dev@automation.example", str(self.app))

        not_authenticated = (1, {"valid": False, "error": {"code": "not_authenticated", "message": "Login required."}})
        with patch.object(module, "run_json", return_value=not_authenticated):
            with self.assertRaisesRegex(module.DeploymentError, module.FORCED_LOGIN):
                module.deploy_once(str(self.tinker), "https://admin.example.test", "dev@automation.example", str(self.app),
                                   lambda *_: (_ for _ in ()).throw(module.DeploymentError("otp=123456")))

        with patch.object(module, "run_json", side_effect=[not_authenticated, (0, {"valid": True, "name": WRONG_ALLOWED_DOMAIN_EMAIL})]):
            with self.assertRaisesRegex(module.DeploymentError, module.POST_LOGIN_IDENTITY_CHECK):
                module.deploy_once(str(self.tinker), "https://admin.example.test", "dev@automation.example", str(self.app), lambda *_: None)

        with patch.object(module, "run_json", side_effect=[(0, {"valid": True, "name": "dev@automation.example"}), module.DeploymentError("app=private-app")]):
            with self.assertRaisesRegex(module.DeploymentError, module.DEPLOYMENT_RESULT_VALIDATION):
                module.deploy_once(str(self.tinker), "https://admin.example.test", "dev@automation.example", str(self.app))

    def test_unsafe_non_https_and_missing_inputs_are_denied(self):
        for server in ("http://admin.example.test", "https://admin.example.test/path", "https://admin.example.test:444"):
            with self.assertRaises(module.DeploymentError): module.deploy_once(str(self.tinker), server, "dev@automation.example", str(self.app))
        with self.assertRaises(module.DeploymentError): module.deploy_once("tinker", "https://admin.example.test", "dev@automation.example", str(self.app))
        with self.assertRaises(module.DeploymentError): module.deploy_once(str(self.tinker), "https://admin.example.test", "dev@automation.example", str(self.root / "missing"))

    def test_symlinked_cli_app_and_manifest_are_denied(self):
        linked_tinker = self.root / "linked-tinker"; linked_tinker.symlink_to(self.tinker)
        with self.assertRaises(module.DeploymentError): module.deploy_once(str(linked_tinker), "https://admin.example.test", "dev@automation.example", str(self.app))
        linked_app = self.root / "linked-app"; linked_app.symlink_to(self.app, target_is_directory=True)
        with self.assertRaises(module.DeploymentError): module.deploy_once(str(self.tinker), "https://admin.example.test", "dev@automation.example", str(linked_app))
        manifest = self.app / "tinker.yaml"; real_manifest = self.app / "tinker-real.yaml"; manifest.rename(real_manifest); manifest.symlink_to(real_manifest)
        with self.assertRaises(module.DeploymentError): module.deploy_once(str(self.tinker), "https://admin.example.test", "dev@automation.example", str(self.app))

    def test_malformed_identity_json_is_denied_before_login(self):
        with patch.object(module, "run_json", lambda *_: (0, {"valid": True, "name": "dev@automation.example", "extra": True})):
            with self.assertRaises(module.DeploymentError): module.deploy_once(str(self.tinker), "https://admin.example.test", "dev@automation.example", str(self.app))

    def test_compatibility_error_and_credential_environment_are_denied(self):
        with patch.object(module, "run_json", lambda *_: (1, {"valid": False, "error": {"code": "incompatible", "message": "upgrade"}})):
            with self.assertRaises(module.DeploymentError): module.deploy_once(str(self.tinker), "https://admin.example.test", "dev@automation.example", str(self.app))
        os.environ["TINKER_TOKEN"] = "must-not-be-read"
        try:
            with self.assertRaises(module.DeploymentError): module.deploy_once(str(self.tinker), "https://admin.example.test", "dev@automation.example", str(self.app))
        finally:
            os.environ.pop("TINKER_TOKEN", None)

    def test_missing_reader_configuration_denies_only_when_forced_login_is_needed(self):
        os.environ.pop("TINKERCLOUD_VPS_DOMAIN")
        try:
            with patch.object(module, "run_json", lambda *_: (1, {"valid": False, "error": {"code": "not_authenticated", "message": "Login required."}})):
                with self.assertRaises(module.DeploymentError): module.deploy_once(str(self.tinker), "https://admin.example.test", "dev@automation.example", str(self.app))
        finally:
            os.environ["TINKERCLOUD_VPS_DOMAIN"] = "example.test"

    def test_reader_derives_only_the_admin_host_from_one_domain(self):
        values = module.reader_environment("admin.example.test")
        self.assertEqual(values["TINKERCLOUD_VPS_DOMAIN"], "example.test")
        self.assertNotIn("TINKERCLOUD_VPS_PLATFORM_HOST", values)
        with self.assertRaises(module.DeploymentError):
            module.reader_environment("other.example.test")

    def test_malformed_or_duplicate_deploy_json_and_unsafe_url_are_denied(self):
        for raw in ('{"valid":true,"valid":false}', '{"valid":true,"name":"https://demo.example.test/"} trailing', '{"valid":NaN}'):
            with self.assertRaises(module.DeploymentError): module.parse_json(raw)
        with self.assertRaises(module.DeploymentError, msg="large output must be denied"):
            module.parse_json("{" + '"x":"' + ("a" * module.MAX_JSON_OUTPUT_BYTES) + '"}')
        with self.assertRaises(module.DeploymentError):
            self.run_with([(0, {"valid": True, "name": "dev@automation.example"}), (0, {"valid": True, "name": "http://demo.example.test/"})])


if __name__ == "__main__":
    unittest.main()
