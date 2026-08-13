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

Run the complete workflow contract. Treat its publication brief as the only
routine checkpoint. If it reaches the repository-change exception, run
`deliver-issue` to completion and restart only when the contract authorizes a
new Establish run.
