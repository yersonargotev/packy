---
name: deliver-issue
description: Deliver exactly one approved GitHub issue through its repository's protected pull-request workflow when the user explicitly asks for end-to-end delivery, including implementation, proof, review, merge, and cleanup.
---

# Deliver Issue

Use this skill only for an explicit request to deliver one numbered GitHub
issue end to end. It does not start from an approval label alone, and it does
not cover triage, planning, a partial implementation, a release, deployment,
real migration, production configuration, or another irreversible external
effect outside repository integration.

## Bootstrap

Require exactly one issue number and explicit delivery intent. Before observing
or changing GitHub or Git state, verify `git` and `gh` are available, then read
the repository-owned policy at `docs/agents/issue-delivery.md` in full. Read
the repository instructions and
other governing material named by that policy. If the policy is absent,
incomplete, contradictory, or incompatible with the request, pause with an
exception brief.

Load [`references/policy-contract.md`](references/policy-contract.md) and
[`references/common-invariants.md`](references/common-invariants.md). Observe
`gh` authentication and repository permissions, then observe the issue,
candidate branch and pull request, commits, checks, reviews, merge
state, and local Git state. Resume one uniquely compatible delivery from its
earliest incomplete or invalid proof; otherwise begin there.

## Phase gate

Load only the reference for the earliest incomplete phase. Complete its stated
criterion before loading the next phase. A later candidate change invalidates
candidate-bound proof and returns the run to the earliest affected phase.

1. [`references/01-qualify-and-isolate.md`](references/01-qualify-and-isolate.md)
2. [`references/02-implement-and-prove.md`](references/02-implement-and-prove.md)
3. [`references/03-pull-request-and-review.md`](references/03-pull-request-and-review.md)
4. [`references/04-freshness-and-merge.md`](references/04-freshness-and-merge.md)
5. [`references/05-verify-and-clean-up.md`](references/05-verify-and-clean-up.md)
