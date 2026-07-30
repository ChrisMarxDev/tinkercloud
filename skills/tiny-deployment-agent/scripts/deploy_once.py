#!/usr/bin/env python3
"""Perform one fail-closed TinyHost automation deployment without secret output."""
from __future__ import annotations

import argparse
import json
import os
import re
import select
import stat
import subprocess
import sys
import time
from pathlib import Path
from typing import Callable
from urllib.parse import urlsplit

EMAIL = re.compile(r"\A[^\s@]+@[^\s@]+\.[^\s@]+\Z")
HOST = re.compile(r"\A[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+\Z")
TIMEOUT_SECONDS = 120
MAX_JSON_OUTPUT_BYTES = 32 * 1024
FORBIDDEN_CREDENTIAL_ENV = ("TINY_TOKEN", "TINYHOST_DEPLOYER_TOKEN", "TINYHOST_DEPLOYMENT_TOKEN", "TINY_OTP", "TINYHOST_OTP")


class DeploymentError(Exception):
    """A deliberately redacted deployment error."""


def fail(message: str) -> None:
    raise DeploymentError(message)


def private_regular(path: Path, executable: bool = False) -> None:
    try:
        info = os.lstat(path)
    except OSError:
        fail("required local path is unavailable")
    if stat.S_ISLNK(info.st_mode) or not stat.S_ISREG(info.st_mode) or info.st_uid != os.geteuid():
        fail("required local path is unsafe")
    if executable and not os.access(path, os.X_OK):
        fail("required local path is unsafe")


def owned_directory(path: Path) -> None:
    try:
        info = os.lstat(path)
    except OSError:
        fail("app directory is unavailable")
    if stat.S_ISLNK(info.st_mode) or not stat.S_ISDIR(info.st_mode) or info.st_uid != os.geteuid():
        fail("app directory is unsafe")


def parse_server(raw: str) -> tuple[str, str]:
    try:
        parsed = urlsplit(raw)
        port = parsed.port
    except ValueError:
        fail("server is invalid")
    host = (parsed.hostname or "").lower()
    if (parsed.scheme != "https" or not host or not HOST.fullmatch(host) or
            parsed.username is not None or parsed.password is not None or
            parsed.path not in {"", "/"} or parsed.query or parsed.fragment or
            (port is not None and port != 443)):
        fail("server must be an exact HTTPS platform URL")
    return "https://" + host, host


def parse_json(raw: str) -> dict:
    if len(raw.encode("utf-8", "replace")) > MAX_JSON_OUTPUT_BYTES:
        fail("CLI returned malformed JSON")
    def no_duplicates(pairs: list[tuple[str, object]]) -> dict:
        value: dict[str, object] = {}
        for key, item in pairs:
            if key in value:
                fail("CLI returned malformed JSON")
            value[key] = item
        return value
    try:
        value = json.loads(raw, object_pairs_hook=no_duplicates,
                           parse_constant=lambda _value: fail("CLI returned malformed JSON"))
    except (json.JSONDecodeError, UnicodeError):
        fail("CLI returned malformed JSON")
    if not isinstance(value, dict):
        fail("CLI returned malformed JSON")
    return value


def run_json(command: list[str], timeout: int = TIMEOUT_SECONDS) -> tuple[int, dict]:
    try:
        completed = subprocess.run(command, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                                   stderr=subprocess.DEVNULL, text=True, timeout=timeout, check=False)
    except (OSError, subprocess.TimeoutExpired):
        fail("CLI command failed")
    return completed.returncode, parse_json(completed.stdout)


def exact_identity(payload: dict, email: str) -> bool:
    return payload == {"valid": True, "name": email}


def valid_identity_shape(payload: dict) -> bool:
    return set(payload) == {"valid", "name"} and payload.get("valid") is True and isinstance(payload.get("name"), str)


def missing_login(payload: dict) -> bool:
    return payload == {"valid": False, "error": {"code": "not_authenticated", "message": "Login required."}}


def reader_path() -> Path:
    return Path(__file__).resolve().parents[2] / "tiny-full-stack-test" / "scripts" / "read-resend-otp.py"


def reader_environment(server_host: str) -> dict[str, str]:
    required = ("TINYHOST_RESEND_READER_API_KEY_FILE", "TINYHOST_RESEND_OTP_LEDGER_FILE",
                "TINYHOST_VPS_EMAIL_FROM", "TINYHOST_VPS_DOMAIN")
    values = {key: os.environ.get(key, "") for key in required}
    if not all(values.values()) or not EMAIL.fullmatch(values["TINYHOST_VPS_EMAIL_FROM"]):
        fail("OTP reader configuration is unavailable")
    domain = values["TINYHOST_VPS_DOMAIN"].lower()
    if not HOST.fullmatch(domain) or server_host != "admin." + domain:
        fail("OTP reader configuration is unavailable")
    env = dict(os.environ)
    env.pop("TINYHOST_VPS_PLATFORM_HOST", None)
    env.pop("TINYHOST_VPS_APP_SUFFIX", None)
    env.update(values)
    return env


def forced_login(tiny: Path, server: str, email: str, reader: Path, env: dict[str, str]) -> None:
    private_regular(reader, executable=True)
    try:
        proc = subprocess.Popen([str(tiny), "--json", "--server", server, "login", "--force"],
                                stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                text=False, env=env)
    except OSError:
        fail("CLI login could not start")
    assert proc.stdin is not None and proc.stdout is not None and proc.stderr is not None
    deadline = time.monotonic() + TIMEOUT_SECONDS
    pending = b""
    sent_email = False
    sent_code = False
    while proc.poll() is None:
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            proc.kill(); proc.wait()
            fail("CLI login timed out")
        readable, _, _ = select.select([proc.stderr], [], [], remaining)
        if not readable:
            proc.kill(); proc.wait()
            fail("CLI login timed out")
        chunk = os.read(proc.stderr.fileno(), 1024)
        if not chunk:
            continue
        pending = (pending + chunk)[-256:]
        if not sent_email and b"Email: " in pending:
            proc.stdin.write((email + "\n").encode("utf-8")); proc.stdin.flush()
            sent_email = True
        if sent_email and not sent_code and b"Code: " in pending:
            try:
                code = subprocess.run([str(reader), "deployer", email, urlsplit(server).hostname or ""],
                                      stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
                                      text=True, env=env, timeout=TIMEOUT_SECONDS, check=False)
            except (OSError, subprocess.TimeoutExpired):
                proc.kill(); proc.wait()
                fail("OTP reader failed")
            if code.returncode != 0 or not re.fullmatch(r"[0-9]{4,12}\n", code.stdout):
                proc.kill(); proc.wait()
                fail("OTP reader failed")
            proc.stdin.write(code.stdout.encode("ascii")); proc.stdin.flush()
            sent_code = True
    stdout = proc.stdout.read().decode("utf-8", "replace")
    result = proc.wait()
    proc.stdin.close(); proc.stdout.close(); proc.stderr.close()
    if result != 0 or not sent_email or not sent_code:
        fail("CLI login failed")
    if not exact_identity(parse_json(stdout), email):
        fail("CLI login returned the wrong identity")


def deploy_once(tiny_raw: str, server_raw: str, email_raw: str, app_raw: str,
                login_driver: Callable[[Path, str, str, Path, dict[str, str]], None] = forced_login) -> dict:
    if any(os.environ.get(name) for name in FORBIDDEN_CREDENTIAL_ENV):
        fail("credential environment input is forbidden")
    tiny, app = Path(tiny_raw), Path(app_raw)
    if not tiny.is_absolute() or not app.is_absolute() or not EMAIL.fullmatch(email_raw.lower()):
        fail("required input is invalid")
    email = email_raw.lower()
    private_regular(tiny, executable=True)
    owned_directory(app)
    manifest = app / "tiny.yaml"
    private_regular(manifest)
    server, host = parse_server(server_raw)
    status, whoami = run_json([str(tiny), "--json", "--server", server, "whoami"])
    reused = status == 0 and exact_identity(whoami, email)
    if not reused:
        if status == 0 and valid_identity_shape(whoami):
            pass
        elif status != 0 and missing_login(whoami):
            pass
        else:
            fail("saved CLI identity could not be verified")
        login_driver(tiny, server, email, reader_path(), reader_environment(host))
        status, whoami = run_json([str(tiny), "--json", "--server", server, "whoami"])
        if status != 0 or not exact_identity(whoami, email):
            fail("forced login identity could not be verified")
    status, deployment = run_json([str(tiny), "--json", "--server", server, "deploy", str(app)])
    if status != 0 or deployment.get("valid") is not True or not isinstance(deployment.get("name"), str):
        fail("deployment failed")
    url, _host = parse_server(deployment["name"])
    return {"valid": True, "reused_saved_identity": reused, "url": url}


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(add_help=False)
    parser.add_argument("--tiny", required=True)
    parser.add_argument("--server", required=True)
    parser.add_argument("--deployer-email", required=True)
    parser.add_argument("--app-dir", required=True)
    try:
        args = parser.parse_args(argv)
        print(json.dumps(deploy_once(args.tiny, args.server, args.deployer_email, args.app_dir), separators=(",", ":")))
        return 0
    except (DeploymentError, SystemExit):
        print("tiny deployment agent failed", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
