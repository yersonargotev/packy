import importlib.util
import tempfile
import unittest
from pathlib import Path


SCRIPT = Path(__file__).parents[1] / "scripts" / "validate_policy.py"
SPEC = importlib.util.spec_from_file_location("validate_policy", SCRIPT)
validate_policy = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(validate_policy)


def valid_policy() -> str:
    return """# Issue delivery policy

## Authority

- **Approval condition:** GitHub issue has status:approved.
- **Qualification additions:** None
- **Corrective issue closure after verified merge:** Authorized

## Normative contracts

| Role | Path |
| --- | --- |
| Repository instructions | AGENTS.md |
| Issue tracker | None |
| Architecture and domain | None |
| Validation | docs/validation.md |
| Standards review | docs/standards.md |
| Spec review | None |

## Local proof

- **Canonical validation:** Run `python -m unittest` from the repository root.
- **Focused proof:** Run tests covering changed packages.
- **Manual verification:** Verify changed CLI paths; not applicable when no user-facing path changed.
- **User-path isolation:** None

## Advisory checks

Operational failures block delivery until retried; substantive findings are corrected.

## Sensitive surfaces

| Surface | Owning policy |
| --- | --- |
| None | None |

## External-effect boundary

None
"""


class ValidatePolicyTests(unittest.TestCase):
    def make_repo(self, policy: str) -> Path:
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        root = Path(directory.name)
        (root / "docs/agents").mkdir(parents=True)
        (root / "AGENTS.md").write_text("instructions", encoding="utf-8")
        (root / "docs/validation.md").write_text("validation", encoding="utf-8")
        (root / "docs/standards.md").write_text("standards", encoding="utf-8")
        (root / "docs/agents/issue-delivery.md").write_text(policy, encoding="utf-8")
        return root

    def test_accepts_complete_policy_with_permitted_none_values(self):
        self.assertEqual(validate_policy.validate_policy(self.make_repo(valid_policy())), [])

    def test_rejects_placeholder_and_required_none(self):
        policy = valid_policy().replace("GitHub issue has status:approved.", "TODO decide")
        policy = policy.replace("| Validation | docs/validation.md |", "| Validation | None |")
        errors = validate_policy.validate_policy(self.make_repo(policy))
        self.assertTrue(any("placeholder" in error for error in errors))
        self.assertTrue(any("Validation may not be None" in error for error in errors))

    def test_rejects_unsafe_or_missing_path(self):
        policy = valid_policy().replace("| Standards review | docs/standards.md |", "| Standards review | ../outside.md |")
        errors = validate_policy.validate_policy(self.make_repo(policy))
        self.assertTrue(any("safe repository-relative" in error for error in errors))

    def test_rejects_duplicate_required_field(self):
        policy = valid_policy().replace(
            "- **Qualification additions:** None",
            "- **Qualification additions:** None\n- **Qualification additions:** None",
        )
        errors = validate_policy.validate_policy(self.make_repo(policy))
        self.assertTrue(any("Authority must contain exactly" in error for error in errors))

    def test_rejects_heading_variation_and_incomplete_sensitive_none(self):
        policy = valid_policy().replace("## Advisory checks", "## Advisory check")
        policy = policy.replace("| None | None |", "| Credentials | None |")
        errors = validate_policy.validate_policy(self.make_repo(policy))
        self.assertTrue(any("Headings" in error for error in errors))


if __name__ == "__main__":
    unittest.main()
