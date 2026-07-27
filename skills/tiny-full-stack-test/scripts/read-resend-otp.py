#!/usr/bin/env python3
"""Read one exact TinyHost OTP from Resend for the opt-in VPS acceptance run.

This program intentionally has no endpoint override and prints only the OTP to
stdout. Import its small pure functions from the companion unit tests; network
I/O is injected through ``fetch_json``.
"""
from __future__ import annotations

import datetime as dt
import json
import os
import re
import stat
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path
from typing import Any, Callable, Iterable

API_ORIGIN = "https://api.resend.com"
LIST_URL = API_ORIGIN + "/emails?limit=100"
MAX_RESPONSE_BYTES = 1_000_000
SUBJECT = "Your sign-in code"
BODY = re.compile(r"\AYour code: ([0-9]{4,12})\Z")
MESSAGE_ID = re.compile(r"\A[A-Za-z0-9_-]{1,200}\Z")
HOST = re.compile(r"\A[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+\Z")
EMAIL = re.compile(r"\A[^\s@]+@[^\s@]+\.[^\s@]+\Z")


class ReaderError(Exception):
    """A deliberately redacted reader failure."""


def fail(message: str) -> None:
    raise ReaderError(message)


class RejectRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, request, fp, code, message, headers, newurl):
        return None


NO_REDIRECT_OPENER = urllib.request.build_opener(RejectRedirect())


def open_fixed(request: urllib.request.Request, timeout: int):
    return NO_REDIRECT_OPENER.open(request, timeout=timeout)


def require_private_file(raw: str, label: str) -> Path:
    if not raw or not os.path.isabs(raw):
        fail(f"{label} is unavailable")
    path = Path(raw)
    try:
        info = os.lstat(path)
    except OSError:
        fail(f"{label} is unavailable")
    if stat.S_ISLNK(info.st_mode) or not stat.S_ISREG(info.st_mode):
        fail(f"{label} is unsafe")
    if info.st_uid != os.geteuid() or stat.S_IMODE(info.st_mode) != 0o600:
        fail(f"{label} is unsafe")
    return path


def read_key() -> str:
    path = require_private_file(os.environ.get("TINYHOST_RESEND_READER_API_KEY_FILE", ""), "reader key")
    try:
        key = path.read_text(encoding="utf-8").strip()
    except (OSError, UnicodeError):
        fail("reader key is unavailable")
    if not re.fullmatch(r"re_[A-Za-z0-9_-]{8,300}", key):
        fail("reader key is invalid")
    return key


def ledger_path() -> Path:
    configured = os.environ.get("TINYHOST_RESEND_OTP_LEDGER_FILE", "")
    if configured:
        if not os.path.isabs(configured):
            fail("consumed ledger is unavailable")
        return Path(configured)
    key_path = Path(os.environ["TINYHOST_RESEND_READER_API_KEY_FILE"])
    return key_path.with_name("resend-otp-consumed.json")


def load_ledger(path: Path) -> set[str]:
    try:
        info = os.lstat(path)
    except FileNotFoundError:
        return set()
    except OSError:
        fail("consumed ledger is unavailable")
    if stat.S_ISLNK(info.st_mode) or not stat.S_ISREG(info.st_mode):
        fail("consumed ledger is unsafe")
    if info.st_uid != os.geteuid() or stat.S_IMODE(info.st_mode) != 0o600:
        fail("consumed ledger is unsafe")
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, UnicodeError, json.JSONDecodeError):
        fail("consumed ledger is invalid")
    if not isinstance(data, list) or len(data) > 1000 or not all(isinstance(x, str) and MESSAGE_ID.fullmatch(x) for x in data):
        fail("consumed ledger is invalid")
    return set(data)


def save_ledger(path: Path, ids: Iterable[str]) -> None:
    items = sorted(set(ids))[-1000:]
    if not all(MESSAGE_ID.fullmatch(item) for item in items):
        fail("consumed ledger is invalid")
    try:
        parent = path.parent
        parent_info = os.lstat(parent)
        if stat.S_ISLNK(parent_info.st_mode) or not stat.S_ISDIR(parent_info.st_mode):
            fail("consumed ledger is unsafe")
        temporary = path.with_name("." + path.name + ".new")
        descriptor = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(descriptor, "w", encoding="utf-8") as handle:
            json.dump(items, handle, separators=(",", ":"))
            handle.flush()
            os.fsync(handle.fileno())
        os.replace(temporary, path)
        os.chmod(path, 0o600)
    except FileExistsError:
        fail("consumed ledger is busy")
    except OSError:
        fail("consumed ledger is unavailable")


def parse_time(raw: Any) -> dt.datetime | None:
    if not isinstance(raw, str):
        return None
    try:
        parsed = dt.datetime.fromisoformat(raw.replace("Z", "+00:00"))
    except ValueError:
        return None
    if parsed.tzinfo is None:
        return None
    return parsed.astimezone(dt.timezone.utc)


def normalize_recipients(raw: Any) -> list[str] | None:
    if isinstance(raw, str):
        return [raw.lower()]
    if isinstance(raw, list) and all(isinstance(value, str) for value in raw):
        return [value.lower() for value in raw]
    return None


def validate_request(argv: list[str]) -> tuple[str, str, str, str]:
    if len(argv) != 4 or argv[1] not in {"deployer", "viewer"}:
        fail("invalid reader invocation")
    purpose, email, hostname = argv[1], argv[2].lower(), argv[3].lower()
    sender = os.environ.get("TINYHOST_VPS_EMAIL_FROM", "")
    platform = os.environ.get("TINYHOST_VPS_PLATFORM_HOST", "").lower()
    suffix = os.environ.get("TINYHOST_VPS_APP_SUFFIX", "").lower()
    if not EMAIL.fullmatch(email) or not EMAIL.fullmatch(sender) or not HOST.fullmatch(hostname):
        fail("invalid reader invocation")
    if not HOST.fullmatch(platform) or not HOST.fullmatch(suffix) or platform == suffix:
        fail("invalid reader configuration")
    if purpose == "deployer" and hostname != platform:
        fail("invalid reader hostname")
    labels = hostname[: -(len(suffix) + 1)].split(".") if hostname.endswith("." + suffix) else []
    if purpose == "viewer" and (len(labels) != 1 or not labels[0] or hostname == platform):
        fail("invalid reader hostname")
    return purpose, email, hostname, sender


def candidates(payload: Any, email: str, sender: str, now: dt.datetime, consumed: set[str]) -> list[str]:
    rows = payload.get("data") if isinstance(payload, dict) else None
    if not isinstance(rows, list) or len(rows) > 100:
        fail("Resend response is invalid")
    result: list[str] = []
    for row in rows:
        if not isinstance(row, dict):
            fail("Resend response is invalid")
        recipients = normalize_recipients(row.get("to"))
        created = parse_time(row.get("created_at"))
        if recipients is None or created is None:
            fail("Resend response is invalid")
        matches = (row.get("from") == sender and row.get("subject") == SUBJECT and
                   len(recipients) == 1 and recipients[0].lower() == email and
                   now - dt.timedelta(minutes=5) <= created <= now + dt.timedelta(minutes=1))
        if matches:
            message_id = row.get("id")
            if not isinstance(message_id, str) or not MESSAGE_ID.fullmatch(message_id):
                fail("Resend response is invalid")
            if message_id not in consumed:
                result.append(message_id)
    return result


def extract_code(message: Any, email: str, sender: str) -> str:
    if not isinstance(message, dict):
        fail("Resend response is invalid")
    recipients = normalize_recipients(message.get("to"))
    text = message.get("text")
    if (message.get("from") != sender or message.get("subject") != SUBJECT or recipients != [email] or not isinstance(text, str)):
        fail("Resend message does not match request")
    match = BODY.fullmatch(text.rstrip("\r\n"))
    if match is None:
        fail("Resend message does not contain a TinyHost OTP")
    return match.group(1)


def fetch_json(url: str, key: str, opener: Callable[[urllib.request.Request, int], Any] = open_fixed) -> Any:
    if not url.startswith(API_ORIGIN + "/"):
        fail("Resend endpoint is invalid")
    request = urllib.request.Request(url, headers={"Authorization": "Bearer " + key, "Accept": "application/json", "User-Agent": "TinyHost-VPS-E2E/1"})
    try:
        with opener(request, 10) as response:
            if response.status < 200 or response.status >= 300:
                fail("Resend request failed")
            if response.geturl() != url:
                fail("Resend request failed")
            body = response.read(MAX_RESPONSE_BYTES + 1)
    except (urllib.error.URLError, TimeoutError, OSError):
        fail("Resend request failed")
    if len(body) > MAX_RESPONSE_BYTES:
        fail("Resend response is too large")
    try:
        return json.loads(body)
    except (UnicodeError, json.JSONDecodeError):
        fail("Resend response is invalid")


def read_once(argv: list[str], now: dt.datetime, request: Callable[[str, str], Any] = fetch_json) -> str:
    _purpose, email, _hostname, sender = validate_request(argv)
    key = read_key()
    ledger = ledger_path()
    consumed = load_ledger(ledger)
    available = candidates(request(LIST_URL, key), email, sender, now, consumed)
    if len(available) > 1:
        fail("Resend OTP selection is ambiguous")
    if len(available) != 1:
        fail("Resend OTP is not available")
    message_id = available[0]
    code = extract_code(request(API_ORIGIN + "/emails/" + urllib.parse.quote(message_id, safe=""), key), email, sender)
    save_ledger(ledger, consumed | {message_id})
    return code


def timeout_seconds() -> int:
    raw = os.environ.get("TINYHOST_RESEND_OTP_TIMEOUT_SECONDS", "60")
    if not re.fullmatch(r"[0-9]{1,3}", raw):
        fail("invalid reader timeout")
    value = int(raw)
    if value < 1 or value > 120:
        fail("invalid reader timeout")
    return value


def main() -> int:
    try:
        deadline = time.monotonic() + timeout_seconds()
        while True:
            try:
                code = read_once(sys.argv, dt.datetime.now(dt.timezone.utc))
                print(code)
                return 0
            except ReaderError as error:
                if "not available" not in str(error) or time.monotonic() >= deadline:
                    raise
                time.sleep(2)
    except (ReaderError, ValueError):
        print("tinyhost Resend OTP reader failed", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
