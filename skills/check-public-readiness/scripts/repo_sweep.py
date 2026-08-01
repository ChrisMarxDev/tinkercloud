#!/usr/bin/env python3
"""Redacting Git repository sweep for public-source readiness."""

from __future__ import annotations

import argparse
import collections
import ipaddress
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
from urllib.parse import unquote


TOKEN_RULES = (
    ("private-key", re.compile(b"-----" b"BEGIN [A-Z0-9 ]*PRIVATE " b"KEY-----")),
    ("github-token", re.compile(rb"(?:ghp_[A-Za-z0-9]{36}|github_pat_[A-Za-z0-9_]{30,})")),
    ("gitlab-token", re.compile(rb"glpat-[A-Za-z0-9_-]{20,}")),
    ("npm-token", re.compile(rb"npm_[A-Za-z0-9]{20,}")),
    ("openai-token", re.compile(rb"sk-(?:proj-)?[A-Za-z0-9_-]{20,}")),
    ("stripe-live-key", re.compile(rb"(?:sk|rk)_live_[A-Za-z0-9]{16,}")),
    ("slack-token", re.compile(rb"xox[baprs]-[A-Za-z0-9-]{20,}")),
    ("aws-access-key", re.compile(rb"AKIA[A-Z0-9]{16}")),
    ("google-api-key", re.compile(rb"AIza[A-Za-z0-9_-]{35}")),
    ("sendgrid-key", re.compile(rb"SG\.[A-Za-z0-9_-]{16,}\.[A-Za-z0-9_-]{16,}")),
    ("resend-key", re.compile(rb"re_[A-Za-z0-9]{20,}")),
)
EMAIL_RE = re.compile(rb"[A-Za-z0-9.!#$%&'*+/=?^_`{|}~-]+@([A-Za-z0-9.-]+\.[A-Za-z]{2,})")
ABSOLUTE_PATH_RULES = (
    ("macos-home-path", re.compile(rb"/Users/[A-Za-z0-9._-]+/")),
    ("linux-home-path", re.compile(rb"/home/[A-Za-z0-9._-]+/")),
    ("windows-home-path", re.compile(rb"[A-Za-z]:\\\\Users\\\\[^\\\r\n]+")),
)
IPV4_RE = re.compile(rb"(?<![0-9])(?:[0-9]{1,3}\.){3}[0-9]{1,3}(?![0-9])")
ENVIRONMENT_HOST_RE = re.compile(
    rb"\b(?:[a-z0-9-]+\.)*(?:dev|internal|preview|private|stage|staging|testing)\."
    rb"(?:[a-z0-9-]+\.)+(?:app|cloud|co|com|de|dev|eu|fun|io|me|net|org|uk|xyz)\b",
    re.IGNORECASE,
)
ARCHIVE_SUFFIXES = {".7z", ".bz2", ".gz", ".rar", ".tar", ".tgz", ".xz", ".zip"}
BINARY_ASSET_SUFFIXES = {
    ".ai", ".avi", ".blend", ".docx", ".fig", ".gif", ".ico", ".jpeg",
    ".jpg", ".keynote", ".mov", ".mp3", ".mp4", ".otf", ".pdf", ".png",
    ".pptx", ".psd", ".sketch", ".svg", ".tiff", ".ttf", ".wav", ".webm", ".webp", ".woff",
    ".woff2", ".xlsx", ".tldraw", ".tldr",
}
AGENT_LOCAL_PREFIXES = (
    ".codex/plans/",
    ".claude/plans/",
    ".cursor/plans/",
    ".agents/plans/",
)
SENSITIVE_EXACT_NAMES = {
    ".env", ".npmrc", ".pypirc", ".netrc", "credentials", "id_dsa",
    "id_ecdsa", "id_ed25519", "id_rsa", "known_hosts", "secrets",
}
SENSITIVE_SUFFIXES = {".db", ".key", ".log", ".p12", ".pfx", ".sqlite", ".sqlite3"}
SENSITIVE_NAME_FRAGMENTS = ("api-key", "api_key", "private-key", "private_key", "signing-key", "signing_key")
REQUIRED_FILES = {
    "README": ("README.md", "README.rst", "README.txt"),
    "license": ("LICENSE", "LICENSE.md", "COPYING"),
    "security policy": ("SECURITY.md", ".github/SECURITY.md"),
    "contribution guide": ("CONTRIBUTING.md", ".github/CONTRIBUTING.md"),
    "code of conduct": ("CODE_OF_CONDUCT.md", ".github/CODE_OF_CONDUCT.md"),
}
MARKDOWN_LINK_RE = re.compile(r"(?<!!)\[[^\]]+\]\(([^)]+)\)")
HTML_LINK_RE = re.compile(r"(?:href|src)\s*=\s*[\"']([^\"']+)[\"']", re.IGNORECASE)


def git(root: Path, *args: str, input_bytes: bytes | None = None) -> bytes:
    return subprocess.run(
        ["git", "-C", str(root), *args], input=input_bytes, stdout=subprocess.PIPE,
        stderr=subprocess.PIPE, check=True,
    ).stdout


def zlist(raw: bytes) -> list[str]:
    return [item.decode("utf-8", "surrogateescape") for item in raw.split(b"\0") if item]


def sensitive_path(path: str) -> bool:
    name = Path(path).name.lower()
    if (
        name in SENSITIVE_EXACT_NAMES
        or any(fragment in name for fragment in SENSITIVE_NAME_FRAGMENTS)
        or any(name.endswith(suffix) for suffix in SENSITIVE_SUFFIXES)
    ):
        return True
    return name.startswith(".env.") and not name.endswith((".example", ".sample", ".template"))


def is_text(data: bytes) -> bool:
    return b"\0" not in data[:8192]


def reserved_email_domain(domain: str) -> bool:
    domain = domain.lower().rstrip(".")
    return domain in {"example.com", "example.net", "example.org", "users.noreply.github.com"} or domain.endswith(
        (
            ".example", ".example.com", ".example.net", ".example.org", ".invalid",
            ".localhost", ".test", ".users.noreply.github.com",
        )
    )


def reserved_host(host: str) -> bool:
    host = host.lower().rstrip(".")
    return host in {"example.com", "example.net", "example.org"} or host.endswith(
        (
            ".example", ".example.com", ".example.net", ".example.org", ".invalid",
            ".localhost", ".test",
        )
    )


def relative_link_exists(source: Path, path_text: str) -> bool:
    if (source.parent / path_text).exists():
        return True
    # Some checked-in source templates deliberately refer to a build-copied
    # peer. Accept that link only when the repository also contains the exact
    # built target beside the built copy of the source document.
    if source.parent.name == "src":
        built_source = source.parent.parent / "dist" / source.name
        built_target = source.parent.parent / "dist" / path_text
        return built_source.is_file() and built_target.exists()
    return False


class Report:
    def __init__(self, limit: int) -> None:
        self.limit = limit
        self.items: list[dict[str, str]] = []
        self.seen: set[tuple[str, str, str]] = set()

    def add(self, severity: str, rule: str, location: str, detail: str = "") -> None:
        key = (severity, rule, location)
        if key in self.seen:
            return
        self.seen.add(key)
        self.items.append({"severity": severity, "rule": rule, "location": location, "detail": detail})

    def emit(self, as_json: bool) -> None:
        counts = collections.Counter(item["severity"] for item in self.items)
        if as_json:
            print(json.dumps({"summary": dict(counts), "findings": self.items}, indent=2, sort_keys=True))
            return
        grouped: dict[str, list[dict[str, str]]] = collections.defaultdict(list)
        for item in self.items:
            grouped[item["severity"]].append(item)
        for severity in ("BLOCKER", "REVIEW", "INFO"):
            values = sorted(grouped.get(severity, []), key=lambda item: (item["rule"], item["location"]))
            if not values:
                continue
            print(f"\n{severity} ({len(values)})")
            for item in values[: self.limit]:
                suffix = f" — {item['detail']}" if item["detail"] else ""
                print(f"- {item['rule']}: {item['location']}{suffix}")
            if len(values) > self.limit:
                print(f"- ... {len(values) - self.limit} more {severity.lower()} findings omitted")
        print(f"\nSummary: {counts['BLOCKER']} blocker, {counts['REVIEW']} review, {counts['INFO']} info")


def scan_content(
    report: Report,
    data: bytes,
    location: str,
    allowed_emails: set[str],
    origin: str,
) -> None:
    if not is_text(data):
        return
    for rule, pattern in TOKEN_RULES:
        match = pattern.search(data)
        if match:
            line_start = data.rfind(b"\n", 0, match.start()) + 1
            line_end = data.find(b"\n", match.end())
            if line_end < 0:
                line_end = len(data)
            line = data[line_start:line_end].lower()
            placeholder = any(marker in line for marker in (
                b"placeholder", b"replace-me", b"replace_me", b"changeme",
                b"example-token", b"example_key", b"fixture-token", b"sk-test-",
            ))
            if placeholder:
                continue
            report.add("BLOCKER", rule, location, f"possible secret in {origin}; value redacted")
    for rule, pattern in ABSOLUTE_PATH_RULES:
        if pattern.search(data):
            report.add("REVIEW", rule, location, f"local absolute path in {origin}; value redacted")
    for match in EMAIL_RE.finditer(data):
        address = match.group(0).decode("ascii", "ignore").lower().strip("`*")
        domain = match.group(1).decode("ascii", "ignore").lower()
        if address not in allowed_emails and not reserved_email_domain(domain):
            report.add("REVIEW", "personal-email", location, f"non-reserved email in {origin}; value redacted")
            break
    for match in IPV4_RE.finditer(data):
        nearby = data[max(0, match.start() - 24):match.start()].lower()
        if any(marker in nearby for marker in (b"chrome/", b"firefox/", b"version/", b"section-", b"rfc")):
            continue
        try:
            address = ipaddress.ip_address(match.group(0).decode("ascii"))
        except ValueError:
            continue
        if address.is_global:
            report.add("REVIEW", "public-ip-address", location, f"public IP in {origin}; value redacted")
            break
    for match in ENVIRONMENT_HOST_RE.finditer(data):
        if not reserved_host(match.group(0).decode("ascii", "ignore")):
            report.add("REVIEW", "environment-host", location, f"non-reserved environment hostname in {origin}; value redacted")
            break


def current_candidates(root: Path, report: Report, allowed_emails: set[str], max_text_bytes: int) -> None:
    candidates = zlist(git(root, "ls-files", "--cached", "--others", "--exclude-standard", "-z"))
    for relative in candidates:
        path = root / relative
        if not path.is_file() or path.is_symlink():
            if path.is_symlink():
                report.add("REVIEW", "symlink", relative, "verify the public target and portability")
            continue
        if relative.startswith(AGENT_LOCAL_PREFIXES):
            report.add(
                "REVIEW",
                "agent-local-artifact",
                relative,
                "move durable contributor material into project-owned internal docs or remove local planning residue",
            )
        if sensitive_path(relative):
            report.add("BLOCKER", "sensitive-filename", relative, "version-controlled candidate")
        suffix = path.suffix.lower()
        if suffix in ARCHIVE_SUFFIXES:
            report.add("REVIEW", "archive", relative, "inspect contents and provenance")
        if suffix in BINARY_ASSET_SUFFIXES:
            report.add("INFO", "binary-asset", relative, "inspect metadata, authorship, and redistribution rights")
        size = path.stat().st_size
        if size >= max_text_bytes:
            continue
        try:
            scan_content(report, path.read_bytes(), relative, allowed_emails, "current candidate")
        except OSError as error:
            report.add("REVIEW", "unreadable-file", relative, str(error))


def ignored_state(root: Path, report: Report) -> None:
    try:
        ignored = zlist(git(root, "ls-files", "--others", "--ignored", "--exclude-standard", "-z"))
    except subprocess.CalledProcessError:
        return
    sensitive = []
    state_counts: collections.Counter[str] = collections.Counter()
    for relative in ignored:
        parts = {part.lower() for part in Path(relative).parts}
        if sensitive_path(relative):
            sensitive.append(relative)
        for state_root in (".private", ".tinker", "runtime", "state"):
            if state_root in parts:
                state_counts[state_root] += 1
    for relative in sensitive[:100]:
        report.add("REVIEW", "ignored-sensitive-filename", relative, "not normally published; protect against force-add and archives")
    if len(sensitive) > 100:
        report.add("INFO", "ignored-sensitive-filename-count", str(len(sensitive)), "only the first 100 paths are listed")
    for state_root, count in sorted(state_counts.items()):
        report.add("REVIEW", "ignored-local-state", f"{state_root}/", f"{count} ignored paths; review storage and access controls")


def history(root: Path, report: Report, allowed_emails: set[str], max_text_bytes: int, large_blob_bytes: int) -> None:
    raw_objects = git(root, "rev-list", "--objects", "--all").decode("utf-8", "surrogateescape").splitlines()
    paths: dict[str, str] = {}
    object_ids = []
    for line in raw_objects:
        oid, _, path = line.partition(" ")
        object_ids.append(oid)
        if path and oid not in paths:
            paths[oid] = path
        if path.startswith(AGENT_LOCAL_PREFIXES):
            report.add(
                "REVIEW",
                "history-agent-local-artifact",
                path,
                "local agent planning residue exists in reachable history; classify it before publication",
            )
    if not object_ids:
        return
    checks = git(
        root, "cat-file", "--batch-check=%(objectname) %(objecttype) %(objectsize)",
        input_bytes=("\n".join(object_ids) + "\n").encode(),
    ).decode().splitlines()
    blobs: list[tuple[str, int]] = []
    for line in checks:
        oid, kind, size_text = line.split()
        if kind == "blob":
            size = int(size_text)
            blobs.append((oid, size))
            path = paths.get(oid, "<unknown-path>")
            if sensitive_path(path):
                report.add("BLOCKER", "history-sensitive-filename", f"{path} @ {oid[:12]}", "reachable historical blob")
            if size >= large_blob_bytes:
                report.add("REVIEW", "large-history-blob", f"{path} @ {oid[:12]}", f"{size} bytes")
            if Path(path).suffix.lower() in ARCHIVE_SUFFIXES:
                report.add("REVIEW", "history-archive", f"{path} @ {oid[:12]}", "inspect reachable archive")
    process = subprocess.Popen(
        ["git", "-C", str(root), "cat-file", "--batch"],
        stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
    )
    assert process.stdin is not None and process.stdout is not None
    try:
        for oid, size in blobs:
            if size > max_text_bytes:
                continue
            process.stdin.write((oid + "\n").encode())
            process.stdin.flush()
            header = process.stdout.readline().decode().strip().split()
            if len(header) != 3 or header[1] != "blob":
                continue
            data = process.stdout.read(int(header[2]))
            process.stdout.read(1)
            path = paths.get(oid, "<unknown-path>")
            scan_content(report, data, f"{path} @ {oid[:12]}", allowed_emails, "reachable history")
    finally:
        process.stdin.close()
        process.wait(timeout=30)


def repository_metadata(root: Path, report: Report, allowed_emails: set[str]) -> None:
    status = git(root, "status", "--short", "--branch").decode("utf-8", "replace").splitlines()
    if len(status) > 1:
        report.add("REVIEW", "dirty-worktree", str(root), f"{len(status) - 1} changed or untracked entries")
    tracked_ignored = zlist(git(root, "ls-files", "-ci", "--exclude-standard", "-z"))
    for path in tracked_ignored:
        report.add("REVIEW", "tracked-but-ignored", path, "ignore rules do not remove tracked content")
    tracked = zlist(git(root, "ls-files", "-z"))
    folded: dict[str, str] = {}
    for path in tracked:
        key = path.casefold()
        if key in folded and folded[key] != path:
            report.add("REVIEW", "case-collision", f"{folded[key]} | {path}", "not portable across common filesystems")
        folded[key] = path
        if any(ord(character) < 32 for character in path):
            report.add("BLOCKER", "control-character-path", repr(path), "unsafe public filename")
    for label, choices in REQUIRED_FILES.items():
        if not any((root / choice).is_file() for choice in choices):
            report.add("REVIEW", "missing-community-file", label, "add or explicitly document why it is unnecessary")
    if (root / ".gitmodules").is_file():
        report.add("REVIEW", "git-submodules", ".gitmodules", "verify every URL, commit, license, and public accessibility")
    for relative in tracked:
        suffix = Path(relative).suffix.lower()
        if suffix not in {".htm", ".html", ".md"}:
            continue
        source = root / relative
        try:
            body = source.read_text(encoding="utf-8")
        except (OSError, UnicodeError):
            continue
        link_pattern = MARKDOWN_LINK_RE if suffix == ".md" else HTML_LINK_RE
        for match in link_pattern.finditer(body):
            target = match.group(1).strip().strip("<>").split(maxsplit=1)[0]
            if not target or target.startswith(("#", "/", "mailto:")) or "://" in target:
                continue
            if any(marker in target for marker in ("{{", "}}", "${")):
                continue
            path_text = unquote(target.split("#", 1)[0].split("?", 1)[0])
            if path_text and not relative_link_exists(source, path_text):
                report.add("REVIEW", "broken-relative-link", f"{relative} -> {target}", "target does not exist")
    authors = git(root, "log", "--all", "--format=%ae%x00%ce%x00").split(b"\0")
    for raw in authors:
        email = raw.decode("utf-8", "ignore").strip().lower()
        if not email or email in allowed_emails:
            continue
        domain = email.rpartition("@")[2]
        if not reserved_email_domain(domain):
            report.add("REVIEW", "commit-identity", "reachable Git history", "unapproved author/committer email; value redacted")
            break
    branches = git(root, "for-each-ref", "--format=%(refname:short)", "refs/heads", "refs/remotes").decode().splitlines()
    tags = git(root, "tag", "--list").decode().splitlines()
    report.add("INFO", "reachable-refs", str(len(branches)), f"branches/remotes; {len(tags)} tags")
    for tool in ("gitleaks", "trufflehog"):
        if shutil.which(tool):
            report.add("INFO", "independent-scanner-available", tool, "run it with full-history redaction")
    if not any(shutil.which(tool) for tool in ("gitleaks", "trufflehog")):
        report.add("REVIEW", "independent-scanner-missing", "gitleaks/trufflehog", "full-history entropy-aware evidence is incomplete")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("repo", nargs="?", default=".")
    parser.add_argument("--allow-email", action="append", default=[], help="approved public email; repeatable")
    parser.add_argument("--max-text-bytes", type=int, default=5 * 1024 * 1024)
    parser.add_argument("--large-blob-bytes", type=int, default=5 * 1024 * 1024)
    parser.add_argument("--limit", type=int, default=100, help="maximum displayed findings per severity")
    parser.add_argument("--json", action="store_true")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    root = Path(args.repo).resolve()
    try:
        git(root, "rev-parse", "--git-dir")
    except (subprocess.CalledProcessError, FileNotFoundError):
        print(f"not a Git repository: {root}", file=sys.stderr)
        return 2
    allowed_emails = {email.strip().lower() for email in args.allow_email if email.strip()}
    report = Report(args.limit)
    repository_metadata(root, report, allowed_emails)
    current_candidates(root, report, allowed_emails, args.max_text_bytes)
    ignored_state(root, report)
    history(root, report, allowed_emails, args.max_text_bytes, args.large_blob_bytes)
    report.emit(args.json)
    return 1 if any(item["severity"] == "BLOCKER" for item in report.items) else 0


if __name__ == "__main__":
    raise SystemExit(main())
