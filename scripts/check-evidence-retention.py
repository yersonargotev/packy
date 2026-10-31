#!/usr/bin/env python3
"""Check cited local files against a proposed cleanup without changing them."""
import argparse
import csv
from dataclasses import dataclass
import hashlib
import os
from pathlib import Path
import re
import stat
import sys
from urllib.parse import urlsplit


@dataclass(frozen=True)
class Reference:
    path: Path
    location: str


@dataclass(frozen=True)
class Relocation:
    source: Path
    destination: Path
    location: str


@dataclass(frozen=True, order=True)
class Finding:
    location: str
    message: str


LINK = re.compile(r"\[([^\[\]\n]+)\]\(([^()\s]+)\)")
CODE = re.compile(r"`([^`\n]+)`")
LOCAL = re.compile(r"(?:(?:/|\./|\.\./|[A-Za-z0-9_.-]+/)[^\s,;`\[\]()<>]*|[A-Za-z0-9_-]+\.[A-Za-z0-9_.-]+)")
EXTERNAL = re.compile(r"https://[^\s]+|commit [0-9a-fA-F]{7,64}|PR #[0-9]+|#[0-9]+|[0-9a-fA-F]{7,64}")
HEADER = ["ts", "phase", "decision", "why", "evidence", "result"]


class Checker:
    def __init__(self, base, targets):
        self.base = base
        self.targets = targets
        self.references = []
        self.relocations = []
        self.findings = []

    def fail(self, location, message):
        self.findings.append(Finding(location, message))

    def path(self, text, base, location, absolute=False, allow_missing=False):
        if re.search(r"[\\\s?#*%\[\]{}$~]", text) or not text:
            self.fail(location, f"unsupported path: {text}")
            return None
        if absolute and not os.path.isabs(text):
            self.fail(location, f"expected absolute path: {text}")
            return None
        raw = os.path.join(base, text)
        current = ""
        missing = False
        for part in raw.split("/")[1:]:
            current += "/" + part
            if missing:
                if part == "..":
                    self.fail(location, f"cannot traverse missing path: {current}")
                    return None
                continue
            try:
                mode = os.lstat(current).st_mode
            except FileNotFoundError as exc:
                if not allow_missing:
                    self.fail(location, f"cannot inspect {current}: {exc}")
                    return None
                missing = True
                continue
            except OSError as exc:
                self.fail(location, f"cannot inspect {current}: {exc}")
                return None
            if stat.S_ISLNK(mode):
                self.fail(location, f"symlink path is unsupported: {current}")
                return None
        return Path(os.path.abspath(raw))

    def local(self, text, base, location):
        text = re.sub(r":\d+$", "", text)
        if not LOCAL.fullmatch(text):
            self.fail(location, f"unsupported or ambiguous evidence: {text}")
            return
        path = self.path(text, base, location)
        if path:
            self.references.append(Reference(path, location))

    def link(self, destination, base, location):
        if destination.startswith("https://") and urlsplit(destination).netloc:
            return
        if "://" in destination or destination.startswith("#"):
            self.fail(location, f"unsupported link: {destination}")
            return
        self.local(destination, base, location)

    def trail(self, path):
        try:
            with path.open(encoding="utf-8", newline="") as stream:
                rows = list(csv.reader(stream, delimiter="\t", quoting=csv.QUOTE_NONE))
        except (OSError, UnicodeError, csv.Error) as exc:
            self.fail(str(path), f"cannot read trail: {exc}")
            return
        if not rows or rows[0] != HEADER:
            self.fail(str(path), "expected six-column TSV header")
            return
        for number, row in enumerate(rows[1:], 2):
            location = f"{path}:{number}"
            if len(row) != 6:
                self.fail(location, "expected six TSV columns")
                continue
            cell = row[4].strip()
            if row[1] == "relocation":
                match = LINK.fullmatch(cell)
                if not match:
                    self.fail(location, "relocation must be [old-absolute-path](new-absolute-path)")
                    continue
                source = self.path(match[1], self.base, location, absolute=True)
                destination = self.path(match[2], self.base, location, absolute=True)
                if source and destination:
                    self.relocations.append(Relocation(source, destination, location))
                continue
            for token in re.split(r"[;,]", cell):
                token = token.strip()
                match = LINK.fullmatch(token)
                if match:
                    self.link(match[2], self.base, location)
                elif EXTERNAL.fullmatch(token):
                    if token.startswith("https://") and not urlsplit(token).netloc:
                        self.fail(location, f"invalid HTTPS reference: {token}")
                    continue
                else:
                    self.local(token, self.base, location)

    def handoff(self, path):
        try:
            lines = path.read_text(encoding="utf-8").splitlines()
        except (OSError, UnicodeError) as exc:
            self.fail(str(path), f"cannot read handoff: {exc}")
            return
        for number, line in enumerate(lines, 1):
            location = f"{path}:{number}"
            def link(match):
                self.link(match[2], path.parent, location)
                return " " * len(match[0])
            rest = LINK.sub(link, line)
            def code(match):
                content = match[1]
                if LOCAL.fullmatch(re.sub(r":\d+$", "", content)):
                    self.local(content, path.parent, location)
                elif not EXTERNAL.fullmatch(content):
                    self.fail(location, f"unsupported or ambiguous inline code: {content}")
                return " " * len(match[0])
            rest = CODE.sub(code, rest)
            rest = re.sub(r"https://[^\s<>()]+", "", rest)
            if any(c in rest for c in "[]`()<>$~") or LOCAL.search(rest) or "\\" in rest:
                self.fail(location, "unsupported or ambiguous Markdown citation; use inline links or code paths")

    def survives(self, path, location):
        for target in self.targets:
            if path == target or target in path.parents:
                return False
            try:
                target_stat = target.stat()
            except FileNotFoundError:
                continue
            except OSError as exc:
                self.fail(location, f"cannot inspect cleanup target {target}: {exc}")
                return False
            for ancestor in (path, *path.parents):
                try:
                    ancestor_stat = ancestor.stat()
                except FileNotFoundError:
                    continue
                except OSError as exc:
                    self.fail(location, f"cannot inspect {ancestor}: {exc}")
                    return False
                if (target_stat.st_dev, target_stat.st_ino) == (ancestor_stat.st_dev, ancestor_stat.st_ino):
                    return False
        return True

    def regular(self, path, location):
        try:
            if not stat.S_ISREG(path.stat().st_mode):
                self.fail(location, f"not a regular file: {path}")
                return False
        except OSError as exc:
            self.fail(location, f"cannot read file {path}: {exc}")
            return False
        return True

    def digest(self, path, location):
        try:
            hasher = hashlib.sha256()
            with path.open("rb") as stream:
                for chunk in iter(lambda: stream.read(1024 * 1024), b""):
                    hasher.update(chunk)
            return hasher.digest()
        except OSError as exc:
            self.fail(location, f"cannot hash {path}: {exc}")
            return None

    def check(self):
        mappings = {}
        for relocation in self.relocations:
            old = mappings.get(relocation.source)
            if old and old != relocation.destination:
                self.fail(relocation.location, f"conflicting relocations for {relocation.source}")
            mappings[relocation.source] = relocation.destination
            source_ok = self.regular(relocation.source, relocation.location)
            dest_ok = self.regular(relocation.destination, relocation.location)
            if not self.survives(relocation.destination, relocation.location):
                self.fail(relocation.location, f"destination is inside a cleanup target: {relocation.destination}")
            if source_ok and dest_ok:
                source_hash = self.digest(relocation.source, relocation.location)
                dest_hash = self.digest(relocation.destination, relocation.location)
                if source_hash is not None and dest_hash is not None and source_hash != dest_hash:
                    self.fail(relocation.location, f"different bytes: {relocation.source} -> {relocation.destination}")
                elif source_hash is not None and dest_hash is not None:
                    print(f"Verified SHA-256 {source_hash.hex()}: {relocation.source} -> {relocation.destination}")
        for relocation in self.relocations:
            if relocation.destination in mappings:
                self.fail(relocation.location, f"chained or cyclic relocation: {relocation.destination}")
        for ref in self.references:
            if self.regular(ref.path, ref.location):
                self.digest(ref.path, ref.location)
                if not self.survives(ref.path, ref.location) and ref.path not in mappings:
                    self.fail(ref.location, f"no verified relocation for cleanup target reference: {ref.path}")
        return sorted(set(self.findings))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base", required=True)
    trails = parser.add_mutually_exclusive_group(required=True)
    trails.add_argument("--trail", action="append")
    trails.add_argument("--no-trail", action="store_true")
    handoffs = parser.add_mutually_exclusive_group(required=True)
    handoffs.add_argument("--handoff", action="append")
    handoffs.add_argument("--no-handoff", action="store_true")
    parser.add_argument("--target", action="append", required=True)
    args = parser.parse_args()
    checker = Checker(Path("/"), [])
    base = checker.path(args.base, "/", "--base", absolute=True)
    if base and not base.is_dir():
        checker.fail("--base", f"not a directory: {base}")
    checker.base = base or Path("/")
    for value in args.target:
        target = checker.path(value, "/", "--target", absolute=True, allow_missing=True)
        if target:
            checker.targets.append(target)
    for kind, paths in (("trail", args.trail or []), ("handoff", args.handoff or [])):
        for value in paths:
            path = checker.path(value, "/", f"--{kind}", absolute=True)
            if not path:
                continue
            if not checker.survives(path, f"--{kind}"):
                checker.fail(f"--{kind}", f"document is inside a cleanup target: {path}")
            if checker.regular(path, f"--{kind}"):
                getattr(checker, kind)(path)
    findings = checker.check()
    for finding in findings:
        print(f"{finding.location}: {finding.message}", file=sys.stderr)
    if findings:
        return 1
    print("Evidence retention check passed; cleanup requires separate authorization.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
