---
name: release-packy
description: Release Packy end to end. Use when the user asks to publish a new Packy release from origin/main or recover an incomplete publication for an existing immutable tag.
---

# Release Packy

Read the complete [workflow contract](../../../workflows/packy-release.md),
[repository instructions](../../../AGENTS.md), [release
contract](../../../docs/release.md), and [Release
workflow](../../../.github/workflows/release.yml) before mutating project or
external state. The workflow contract owns orchestration; keep this skill as its
thin **release gate**.

## 1. Establish

Run **Establish** from the workflow contract.

**Complete when:** the contract's Establish criterion is satisfied.

## 2. Prove

Run **Prove** from the workflow contract.

**Complete when:** the contract's Prove criterion is satisfied.

## 3. Approve

Run **Approve** from the workflow contract. The contract's publication brief is
the only routine checkpoint.

**Complete when:** the contract's Approve criterion is satisfied.

## 4. Publish once

Run **Publish once** from the workflow contract.

**Complete when:** the contract's Publish once criterion is satisfied.

## 5. Verify and close

Run **Verify and close** from the workflow contract.

**Complete when:** the contract's Verify and close criterion is satisfied,
including the clean final governance publication-boundary rerun.

## Repository-change exception

When the workflow contract classifies a release failure as requiring a change
to repository code, scripts, workflows, or this skill, pause publication and
recommend one explicit handoff: create one numbered `status:approved` issue and
complete it through `deliver-issue`. Preserve the failed immutable version as
evidence. After issue delivery reaches its Definition of done, restart
**Establish** with a monotonically newer version.

**Complete when:** the failure needs no repository change, or the approved issue
delivery is complete and a monotonically newer release has restarted from
**Establish**.
