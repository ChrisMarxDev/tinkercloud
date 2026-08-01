#!/usr/bin/env python3
"""Perform one fail-closed Tinkercloud automation deployment without secret output."""
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
LOCAL_AUTOMATION_DEPLOYER_DOMAIN = "christopher-marx.de"
TIMEOUT_SECONDS = 120
MAX_JSON_OUTPUT_BYTES = 32 * 1024
FORBIDDEN_CREDENTIAL_ENV = ("TINKER_TOKEN", "TINKER_OTP")

INPUT_VALIDATION = "input_validation"
SAVED_IDENTITY_CHECK = "saved_identity_check"
FORCED_LOGIN = "forced_login"
POST_LOGIN_IDENTITY_CHECK = "post_login_identity_check"
DEPLOYMENT_RESULT_VALIDATION = "deployment_result_validation"
INTERNAL_FAILURE = "internal_failure"
PUBLIC_FAILURE_STAGES = frozenset((
    INPUT_VALIDATION,
    SAVED_IDENTITY_CHECK,
    FORCED_LOGIN,
    POST_LOGIN_IDENTITY_CHECK,
    DEPLOYMENT_RESULT_VALIDATION,
    INTERNAL_FAILURE,
))


class DeploymentError(Exception):
    """A deliberately redacted deployment error."""


def fail(stage: str) -> None:
    raise DeploymentError(stage)


def at_stage(stage: str, action: Callable[[], object]) -> object:
    """Map all expected implementation details to one fixed public stage."""
    try:
        return action()
    except DeploymentError:
        fail(stage)


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
    return Path(__file__).resolve().parents[2] / "tinkercloud-full-stack-test" / "scripts" / "read-resend-otp.py"


def reader_environment(server_host: str) -> dict[str, str]:
    required = ("TINKERCLOUD_RESEND_READER_API_KEY_FILE", "TINKERCLOUD_RESEND_OTP_LEDGER_FILE",
                "TINKERCLOUD_VPS_EMAIL_FROM", "TINKERCLOUD_VPS_DOMAIN")
    values = {key: os.environ.get(key, "") for key in required}
    if not all(values.values()) or not EMAIL.fullmatch(values["TINKERCLOUD_VPS_EMAIL_FROM"]):
        fail("OTP reader configuration is unavailable")
    domain = values["TINKERCLOUD_VPS_DOMAIN"].lower()
    if not HOST.fullmatch(domain) or server_host != "admin." + domain:
        fail("OTP reader configuration is unavailable")
    env = dict(os.environ)
    env.pop("TINKERCLOUD_VPS_PLATFORM_HOST", None)
    env.pop("TINKERCLOUD_VPS_APP_SUFFIX", None)
    env.update(values)
    return env


def forced_login(tinker: Path, server: str, email: str, reader: Path, env: dict[str, str]) -> None:
    private_regular(reader, executable=True)
    try:
        proc = subprocess.Popen([str(tinker), "--json", "--server", server, "login", "--force"],
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


def deploy_once(tinker_raw: str, server_raw: str, email_raw: str, app_raw: str,
                login_driver: Callable[[Path, str, str, Path, dict[str, str]], None] = forced_login) -> dict:
    def validate_inputs() -> tuple[Path, Path, str, str, str]:
        if any(os.environ.get(name) for name in FORBIDDEN_CREDENTIAL_ENV):
            fail("credential environment input is forbidden")
        email = email_raw.lower()
        if (not EMAIL.fullmatch(email) or
                email.rsplit("@", 1)[1] != LOCAL_AUTOMATION_DEPLOYER_DOMAIN):
            fail("required input is invalid")
        tinker, app = Path(tinker_raw), Path(app_raw)
        if not tinker.is_absolute() or not app.is_absolute():
            fail("required input is invalid")
        private_regular(tinker, executable=True)
        owned_directory(app)
        private_regular(app / "tinker.yaml")
        server, host = parse_server(server_raw)
        return tinker, app, email, server, host

    tinker, app, email, server, host = at_stage(INPUT_VALIDATION, validate_inputs)  # type: ignore[misc]
    status, whoami = at_stage(SAVED_IDENTITY_CHECK, lambda: run_json(
        [str(tinker), "--json", "--server", server, "whoami"]
    ))  # type: ignore[misc]
    reused = status == 0 and exact_identity(whoami, email)
    if not reused:
        if status == 0 and valid_identity_shape(whoami):
            pass
        elif status != 0 and missing_login(whoami):
            pass
        else:
            fail(SAVED_IDENTITY_CHECK)

        def login() -> None:
            login_driver(tinker, server, email, reader_path(), reader_environment(host))
        at_stage(FORCED_LOGIN, login)
        status, whoami = at_stage(POST_LOGIN_IDENTITY_CHECK, lambda: run_json(
            [str(tinker), "--json", "--server", server, "whoami"]
        ))  # type: ignore[misc]
        if status != 0 or not exact_identity(whoami, email):
            fail(POST_LOGIN_IDENTITY_CHECK)

    def deploy_and_validate() -> str:
        status, deployment = run_json([str(tinker), "--json", "--server", server, "deploy", str(app)])
        if status != 0 or deployment.get("valid") is not True or not isinstance(deployment.get("name"), str):
            fail("deployment failed")
        url, _host = parse_server(deployment["name"])
        return url

    url = at_stage(DEPLOYMENT_RESULT_VALIDATION, deploy_and_validate)
    return {"valid": True, "reused_saved_identity": reused, "url": url}


class SafeArgumentParser(argparse.ArgumentParser):
    def error(self, _message: str) -> None:
        fail(INPUT_VALIDATION)


def main(argv: list[str]) -> int:
    parser = SafeArgumentParser(add_help=False)
    parser.add_argument("--tinker", required=True)
    parser.add_argument("--server", required=True)
    parser.add_argument("--deployer-email", required=True)
    parser.add_argument("--app-dir", required=True)
    try:
        args = parser.parse_args(argv)
        print(json.dumps(deploy_once(args.tinker, args.server, args.deployer_email, args.app_dir), separators=(",", ":")))
        return 0
    except DeploymentError as error:
        stage = error.args[0] if error.args and error.args[0] in PUBLIC_FAILURE_STAGES else INTERNAL_FAILURE
        print(stage, file=sys.stderr)
        return 1
    except Exception:
        print(INTERNAL_FAILURE, file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
