#!/usr/bin/env python3
import datetime as dt
import contextlib
import importlib.util
import io
import os
import pathlib
import tempfile
import unittest
import urllib.error
from unittest.mock import patch

HERE = pathlib.Path(__file__).parent
SPEC = importlib.util.spec_from_file_location("reader", HERE / "read-resend-otp.py")
reader = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(reader)

ALLOWED_RECIPIENT_DOMAIN = "automation.example"
DEPLOYER_EMAIL = "dev@" + ALLOWED_RECIPIENT_DOMAIN
VIEWER_EMAIL = "viewer@" + ALLOWED_RECIPIENT_DOMAIN


class ReaderTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.root = pathlib.Path(self.temp.name)
        self.key = self.root / "reader-key"
        self.ledger = self.root / "ledger.json"
        self.key.write_text("re_abcdefgh12345678")
        self.key.chmod(0o600)
        self.old = dict(os.environ)
        os.environ.update({"TINKERCLOUD_RESEND_READER_API_KEY_FILE": str(self.key), "TINKERCLOUD_RESEND_OTP_LEDGER_FILE": str(self.ledger), "TINKERCLOUD_VPS_EMAIL_FROM": "tinker@example.test", "TINKERCLOUD_VPS_DOMAIN": "example.test", "TINKERCLOUD_AUTOMATION_RECIPIENT_DOMAIN": ALLOWED_RECIPIENT_DOMAIN})
        self.now = dt.datetime(2026, 7, 27, 12, 0, tzinfo=dt.timezone.utc)

    def tearDown(self):
        os.environ.clear(); os.environ.update(self.old); self.temp.cleanup()

    def row(self, identifier="msg_123", **changes):
        row = {"id": identifier, "from": "tinker@example.test", "to": [VIEWER_EMAIL], "subject": "Your sign-in code", "created_at": "2026-07-27T11:59:30Z"}
        row.update(changes)
        return row

    def test_reads_one_exact_message_then_marks_it_consumed(self):
        def request(url, _key):
            if url == reader.LIST_URL: return {"data": [self.row()]}
            self.assertEqual(url, reader.API_ORIGIN + "/emails/msg_123")
            return {"from": "tinker@example.test", "to": [VIEWER_EMAIL], "subject": "Your sign-in code", "text": "Your code: 123456"}
        argv = ["reader", "viewer", VIEWER_EMAIL, "admin.example.test"]
        self.assertEqual(reader.read_once(argv, self.now, request), "123456")
        self.assertEqual(reader.load_ledger(self.ledger), {"msg_123"})
        with self.assertRaises(reader.ReaderError): reader.read_once(argv, self.now, request)

    def test_denies_ambiguous_stale_and_wrong_recipient_messages(self):
        self.assertEqual(reader.candidates({"data": [self.row("one"), self.row("two")]}, VIEWER_EMAIL, "tinker@example.test", self.now, set()), ["one", "two"])
        self.assertEqual(reader.candidates({"data": [self.row(created_at="2026-07-27T11:50:00Z"), self.row("wrong", to=["other@example.test"])]}, VIEWER_EMAIL, "tinker@example.test", self.now, set()), [])

    def test_denies_non_exact_text_and_unsafe_key_file(self):
        with self.assertRaises(reader.ReaderError): reader.extract_code({"from": "tinker@example.test", "to": [VIEWER_EMAIL], "subject": "Your sign-in code", "text": "Your code: 123456\nextra"}, VIEWER_EMAIL, "tinker@example.test")
        self.key.chmod(0o644)
        with self.assertRaises(reader.ReaderError): reader.read_key()

    def test_accepts_platform_host_for_global_viewer_broker_only(self):
        self.assertEqual(reader.validate_request(["reader", "viewer", VIEWER_EMAIL, "admin.example.test"])[2], "admin.example.test")

    def test_accepts_exact_recipient_domain_case_insensitively(self):
        request = ["reader", "deployer", DEPLOYER_EMAIL.upper(), "admin.example.test"]
        self.assertEqual(reader.validate_request(request)[1], "dev@automation.example")

    def test_denies_non_exact_recipient_domain_before_local_or_provider_access(self):
        for email in ("dev@automationexample", "dev@evil" + ALLOWED_RECIPIENT_DOMAIN,
                      "dev@sub." + ALLOWED_RECIPIENT_DOMAIN,
                      "dev@" + ALLOWED_RECIPIENT_DOMAIN + ".example", "dev@example.test"):
            accessed = []
            def forbidden_key():
                accessed.append("key")
                self.fail("invalid recipient must not read the key")
            def forbidden_ledger():
                accessed.append("ledger")
                self.fail("invalid recipient must not read the ledger")
            def forbidden_provider(_url, _key):
                accessed.append("provider")
                self.fail("invalid recipient must not call the provider")
            original_key, original_ledger = reader.read_key, reader.ledger_path
            reader.read_key, reader.ledger_path = forbidden_key, forbidden_ledger
            try:
                with self.assertRaises(reader.ReaderError):
                    reader.read_once(["reader", "viewer", email, "admin.example.test"], self.now, forbidden_provider)
            finally:
                reader.read_key, reader.ledger_path = original_key, original_ledger
            self.assertEqual(accessed, [])

    def test_denies_missing_or_malformed_automation_domain_before_local_or_provider_access(self):
        try:
            for value in (None, "", "automation.example ", "AUTOMATION.EXAMPLE",
                          "automation.example\nother.example", "localhost", "-bad.example"):
                with self.subTest(value=value):
                    if value is None:
                        os.environ.pop("TINKERCLOUD_AUTOMATION_RECIPIENT_DOMAIN", None)
                    else:
                        os.environ["TINKERCLOUD_AUTOMATION_RECIPIENT_DOMAIN"] = value
                    accessed = []
                    with patch.object(reader, "read_key", side_effect=lambda: accessed.append("key")), \
                         patch.object(reader, "ledger_path", side_effect=lambda: accessed.append("ledger")):
                        with self.assertRaises(reader.ReaderError):
                            reader.read_once(["reader", "viewer", VIEWER_EMAIL,
                                              "admin.example.test"], self.now,
                                             lambda *_: accessed.append("provider"))
                    self.assertEqual(accessed, [])
        finally:
            os.environ["TINKERCLOUD_AUTOMATION_RECIPIENT_DOMAIN"] = ALLOWED_RECIPIENT_DOMAIN

    def test_denies_endpoint_override_and_non_platform_hostname(self):
        with self.assertRaises(reader.ReaderError): reader.fetch_json("https://example.test/emails", "re_abcdefgh12345678")
        with self.assertRaises(reader.ReaderError): reader.validate_request(["reader", "viewer", VIEWER_EMAIL, "two.labels.example.test"])
        with self.assertRaises(reader.ReaderError): reader.validate_request(["reader", "deployer", VIEWER_EMAIL, "slug.example.test"])
        with self.assertRaises(reader.ReaderError): reader.validate_request(["reader", "viewer", VIEWER_EMAIL, "slug.example.test"])
        with self.assertRaises(reader.ReaderError): reader.validate_request(["reader", "viewer", VIEWER_EMAIL, "example.test"])
        with self.assertRaises(reader.ReaderError): reader.validate_request(["reader", "viewer", VIEWER_EMAIL, "other.example.test"])

    def test_denies_redirect_without_exposing_provider_body(self):
        def redirect(_request, _timeout):
            raise urllib.error.HTTPError(reader.LIST_URL, 302, "redirect", {}, io.BytesIO(b"provider-secret-body"))
        with self.assertRaises(reader.ReaderError) as caught:
            reader.fetch_json(reader.LIST_URL, "re_abcdefgh12345678", redirect)
        self.assertNotIn("provider-secret-body", str(caught.exception))

    def test_uses_fixed_non_secret_resend_user_agent(self):
        class Response:
            status = 200
            def __enter__(self): return self
            def __exit__(self, *_): return False
            def geturl(self): return reader.LIST_URL
            def read(self, _limit): return b'{"data":[]}'
        def open_request(request, _timeout):
            self.assertEqual(request.get_header("User-agent"), "Tinkercloud-VPS-E2E/1")
            self.assertEqual(request.get_header("Accept"), "application/json")
            return Response()
        self.assertEqual(reader.fetch_json(reader.LIST_URL, "re_abcdefgh12345678", open_request), {"data": []})

    def test_timeout_is_bounded_and_invalid_timeout_has_generic_stderr(self):
        for value in ("0", "121", "-1", "abc", " 60"):
            os.environ["TINKERCLOUD_RESEND_OTP_TIMEOUT_SECONDS"] = value
            with self.assertRaises(reader.ReaderError): reader.timeout_seconds()
        os.environ["TINKERCLOUD_RESEND_OTP_TIMEOUT_SECONDS"] = "1"
        self.assertEqual(reader.timeout_seconds(), 1)
        os.environ["TINKERCLOUD_RESEND_OTP_TIMEOUT_SECONDS"] = "121"
        stderr = io.StringIO()
        with contextlib.redirect_stderr(stderr): self.assertEqual(reader.main(), 1)
        self.assertEqual(stderr.getvalue(), "tinkercloud Resend OTP reader failed\n")

    def test_detail_recipient_comparison_is_case_normalized(self):
        self.assertEqual(reader.extract_code({"from": "tinker@example.test", "to": [VIEWER_EMAIL.upper()], "subject": "Your sign-in code", "text": "Your code: 123456"}, VIEWER_EMAIL, "tinker@example.test"), "123456")

    def test_parses_resend_short_offset_timestamp_shapes(self):
        self.assertEqual(reader.parse_time("2026-07-27 12:34:56.12345+00"), dt.datetime(2026, 7, 27, 12, 34, 56, 123450, tzinfo=dt.timezone.utc))
        self.assertEqual(reader.parse_time("2026-07-27 12:34:56.123456-02"), dt.datetime(2026, 7, 27, 14, 34, 56, 123456, tzinfo=dt.timezone.utc))

    def test_rejects_naive_and_malformed_timestamps(self):
        for value in ("2026-07-27 12:34:56", "2026-07-27 12:34:56.123456+0", "2026-07-27T12:34:56+001", "not-a-time"):
            self.assertIsNone(reader.parse_time(value))


if __name__ == "__main__": unittest.main()
