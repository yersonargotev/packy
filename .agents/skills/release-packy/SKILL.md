---
name: release-packy
description: Release Packy end to end. Use when the user asks to publish a new immutable Packy release from origin/main.
---

# Release Packy

Read the complete [workflow contract](../../../workflows/packy-release.md),
[repository instructions](../../../AGENTS.md), [release
contract](../../../docs/release.md), and [Release
workflow](../../../.github/workflows/release.yml) before mutating project or
external state. The workflow contract owns orchestration; keep this skill as its
thin **release gate**.

Run the complete workflow contract. Its publication brief approves only the
exact release version and commit. For a repository-change exception, use the
normal development flow and retain the pending PR handoff. On resumption, check
that handoff before Establish. Restart Establish only after observing the
correction merged into fetched `origin/main`.
