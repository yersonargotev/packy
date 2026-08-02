---
name: deliver-issue
description: Deliver one approved Packy GitHub issue through implementation, local validation and manual CLI verification, final Standards and Spec review, required CI, protected merge, and cleanup. Use only when the user explicitly asks to deliver a numbered Packy issue end to end.
---

# Deliver Issue

Read the complete [issue-delivery workflow](../../../workflows/issue-delivery.md),
[repository instructions](../../../AGENTS.md), and [issue-tracker
contract](../../../docs/agents/issue-tracker.md) before changing local or
external state. The workflow owns orchestration; keep this skill as its thin
project-local gate.

Require exactly one issue number. Applying `status:approved` without an explicit
delivery request does not trigger this skill. Reconstruct any prior progress
from Git and GitHub before acting, then resume the earliest incomplete workflow
phase.

## 1. Qualify and isolate

Run **Trigger**, **Workspace isolation**, **Qualification**, **Resume and
ownership**, and **Surface-owned assurance** from the workflow.

**Complete when:** the workflow's Qualification criterion is satisfied in the
selected isolated workspace.

## 2. Implement and prove locally

Run **Local implementation loop**. Use the repository's diagnosis,
implementation, testing, and proportional review skills when their triggers
apply.

**Complete when:** the workflow's Local implementation loop criterion is
satisfied on one clean exact candidate HEAD.

## 3. Open and prove the pull request

Run **Pull request and final review** and **Check interpretation**. Keep the two
final review axes independent and bind their durable result to the exact PR
HEAD.

**Complete when:** the workflow's Pull request and final review criterion is
satisfied.

## 4. Merge

Run **Freshness and merge**. The explicit trigger authorizes merge through
branch protection but never authorizes a bypass or a separate publication or
production effect.

**Complete when:** the workflow's Freshness and merge criterion is satisfied.

## 5. Verify and close

Run **Verify and clean up**, following **Retries and failures** and
**Communication and briefs** throughout the run.

**Complete when:** the workflow's Definition of done is satisfied and the
success brief has been delivered.
