#!/usr/bin/env python3
"""Validate an issue-delivery policy against the bundled structural contract.

This validator checks deterministic document structure, required values, and
repository-relative file pointers. It deliberately cannot establish whether a
policy command is semantically safe; that evidence remains the setup skill's
human audit responsibility.
"""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path


POLICY_PATH = Path("docs/agents/issue-delivery.md")
HEADINGS = (
    "# Issue delivery policy",
    "## Authority",
    "## Normative contracts",
    "## Local proof",
    "## Advisory checks",
    "## Sensitive surfaces",
    "## External-effect boundary",
)
AUTHORITY_FIELDS = (
    "Approval condition",
    "Qualification additions",
    "Corrective issue closure after verified merge",
)
LOCAL_PROOF_FIELDS = (
    "Canonical validation",
    "Focused proof",
    "Manual verification",
    "User-path isolation",
)
NORMATIVE_ROLES = (
    "Repository instructions",
    "Issue tracker",
    "Architecture and domain",
    "Validation",
    "Standards review",
    "Spec review",
)
NONE_PERMITTED_NORMATIVE = {
    "Issue tracker",
    "Architecture and domain",
    "Spec review",
}
NONE_PERMITTED_FIELDS = {
    "Qualification additions",
    "User-path isolation",
}
PLACEHOLDER = re.compile(r"\b(?:TODO|TBD)\b|<[^>\n]+>|\{[^}\n]*\}", re.IGNORECASE)
FIELD = re.compile(r"^- \*\*(?P<name>[^:]+):\*\*\s*(?P<value>.*)$")


def section_lines(lines: list[str], heading: str) -> list[str]:
    """Return the content under one required heading, excluding later headings."""
    start = lines.index(heading) + 1
    end = len(lines)
    for index in range(start, len(lines)):
        if lines[index].startswith("#"):
            end = index
            break
    return lines[start:end]


def nonempty(value: str, label: str, errors: list[str], *, none_permitted: bool = False) -> None:
    if not value.strip():
        errors.append(f"{label} must be nonempty.")
    elif value.strip() == "None" and not none_permitted:
        errors.append(f"{label} may not be None.")


def safe_existing_file(root: Path, value: str, label: str, errors: list[str]) -> None:
    if value == "None":
        return
    path = Path(value)
    if path.is_absolute() or ".." in path.parts or value in {".", ""}:
        errors.append(f"{label} must be a safe repository-relative path.")
        return
    candidate = root / path
    try:
        candidate.resolve().relative_to(root.resolve())
    except ValueError:
        errors.append(f"{label} must stay inside the consumer repository.")
        return
    if not candidate.is_file():
        errors.append(f"{label} must point to an existing file: {value}")


def parse_table(lines: list[str], expected_header: str, label: str, errors: list[str]) -> list[tuple[str, str]]:
    compact = [line.strip() for line in lines if line.strip()]
    if len(compact) < 2 or compact[0] != expected_header or compact[1] != "| --- | --- |":
        errors.append(f"{label} must begin with the required two-column Markdown table.")
        return []
    rows: list[tuple[str, str]] = []
    for line in compact[2:]:
        match = re.fullmatch(r"\|\s*([^|]+?)\s*\|\s*([^|]+?)\s*\|", line)
        if not match:
            errors.append(f"{label} contains an invalid table row: {line}")
            continue
        rows.append((match.group(1).strip(), match.group(2).strip()))
    return rows


def validate_policy(root: Path) -> list[str]:
    """Return all deterministic contract violations for ``root``'s policy."""
    root = root.resolve()
    policy = root / POLICY_PATH
    if not policy.is_file():
        return [f"Missing policy file: {POLICY_PATH}"]
    try:
        text = policy.read_text(encoding="utf-8")
    except UnicodeDecodeError:
        return [f"Policy file is not valid UTF-8: {POLICY_PATH}"]

    errors: list[str] = []
    if PLACEHOLDER.search(text):
        errors.append("Policy contains a placeholder (TODO, TBD, angle placeholder, or template braces).")
    lines = text.splitlines()
    actual_headings = [line for line in lines if line.startswith("#")]
    if actual_headings != list(HEADINGS):
        errors.append("Headings must exactly match the required headings and order.")
        return errors

    authority_pairs = [(match.group("name"), match.group("value").strip())
                       for line in section_lines(lines, "## Authority")
                       if (match := FIELD.match(line))]
    authority = dict(authority_pairs)
    if len(authority_pairs) != len(AUTHORITY_FIELDS) or set(authority) != set(AUTHORITY_FIELDS):
        errors.append("Authority must contain exactly the required fields.")
    for name in AUTHORITY_FIELDS:
        value = authority.get(name, "")
        nonempty(value, f"Authority: {name}", errors, none_permitted=name in NONE_PERMITTED_FIELDS)
    corrective = authority.get("Corrective issue closure after verified merge")
    if corrective and corrective not in {"Authorized", "Escalate"}:
        errors.append("Authority: Corrective issue closure after verified merge must be Authorized or Escalate.")

    normative_rows = parse_table(section_lines(lines, "## Normative contracts"), "| Role | Path |", "Normative contracts", errors)
    normative = dict(normative_rows)
    if len(normative_rows) != len(NORMATIVE_ROLES) or tuple(role for role, _ in normative_rows) != NORMATIVE_ROLES:
        errors.append("Normative contracts must contain exactly the required roles in order.")
    for role in NORMATIVE_ROLES:
        value = normative.get(role, "")
        nonempty(value, f"Normative contracts: {role}", errors, none_permitted=role in NONE_PERMITTED_NORMATIVE)
        if value and value != "None":
            safe_existing_file(root, value, f"Normative contracts: {role}", errors)

    proof_pairs = [(match.group("name"), match.group("value").strip())
                   for line in section_lines(lines, "## Local proof")
                   if (match := FIELD.match(line))]
    proof = dict(proof_pairs)
    if len(proof_pairs) != len(LOCAL_PROOF_FIELDS) or set(proof) != set(LOCAL_PROOF_FIELDS):
        errors.append("Local proof must contain exactly the required fields.")
    for name in LOCAL_PROOF_FIELDS:
        nonempty(proof.get(name, ""), f"Local proof: {name}", errors,
                 none_permitted=name in NONE_PERMITTED_FIELDS)

    advisory = "\n".join(section_lines(lines, "## Advisory checks")).strip()
    nonempty(advisory, "Advisory checks", errors)

    sensitive_rows = parse_table(section_lines(lines, "## Sensitive surfaces"), "| Surface | Owning policy |", "Sensitive surfaces", errors)
    if not sensitive_rows:
        errors.append("Sensitive surfaces must declare at least one surface or None.")
    for surface, path in sensitive_rows:
        nonempty(surface, "Sensitive surfaces: Surface", errors, none_permitted=True)
        nonempty(path, "Sensitive surfaces: Owning policy", errors, none_permitted=True)
        if (surface == "None") != (path == "None"):
            errors.append("Sensitive surfaces must use None for both columns when no specialist policy exists.")
        if path != "None":
            safe_existing_file(root, path, f"Sensitive surfaces: {surface}", errors)

    external = "\n".join(section_lines(lines, "## External-effect boundary")).strip()
    nonempty(external, "External-effect boundary", errors, none_permitted=True)
    return errors


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("consumer_repo", type=Path, help="path to the consumer repository root")
    args = parser.parse_args(argv)
    errors = validate_policy(args.consumer_repo)
    if errors:
        for error in errors:
            print(f"ERROR: {error}")
        return 1
    print(f"Policy is structurally valid: {args.consumer_repo / POLICY_PATH}")
    print("Note: command semantic safety is not validated by this tool.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
