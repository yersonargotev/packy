# ADR 0017: Prove Pack Sync before publication

## Status

Accepted.

## Context

ADR 0009 separates Inspect, Classify, Validate, and Publish by permission and
keeps branch and pull-request mutation behind protected `main`. Its sandboxed
fakes prove domain and ownership policy, but changes to the live GitHub Models,
source acquisition, runner validation, and GitHub observation adapters could be
exercised together only after merge. Sequential production dispatches therefore
exposed independent integration failures after each prerequisite had already
landed.

A branch proof must not solve this by giving branch-controlled code the current
Publish job's `contents: write` and `pull-requests: write` token. It also cannot
seal a plan against the issue-branch commit and then claim that identity is the
current protected-main publication base.

## Decision

The existing manual workflow admits an explicit preparation-only transport mode
for v3 `register_bundle` requests from approved issue branches. The canonical
request and digest are unchanged. Preparation runs in branch-scoped concurrency
that cannot block or supersede the production Pack operation.

Inspect, Classify, and Validate compile the issue branch's private adapter but
operate on a disposable checkout of the current protected `main`. A distinct
Prepare job has only `contents: read` and `pull-requests: read`. It independently
reacquires every exact member, Applies, reruns Packy validation, seals and
verifies the result tree, constructs the local proposal commit and managed
metadata, revalidates provenance, and requires two identical read-only
publication-state observations.

`internal/packsyncworkflow.Publisher` owns one reusable preparation prefix.
Production `Run` continues from that exact prefix into `Publish` and `Finalize`;
preparation stops before both mutation methods. Its evidence explicitly records
stable observation, `repository_mutated: false`, and `decision_ready: false`.
It is development proof, not a synchronization result or review authority.

The write-capable Publish job remains protected-main-only. Preparation cannot
push, create, edit, ready, or comment on a pull request. Before merging a change
to these external seams, delivery runs the exact intended v3 request through
the branch preparation path and records unchanged target ref/PR state.

## Consequences

- Real model, acquisition, validation, proposal, provenance, and read-only
  GitHub integration failures can be repaired before merge.
- Preparation artifacts may be retained as ordinary Actions evidence, but they
  cannot authorize publication or decision readiness.
- Protected-main production plans remain fresh because branch code is exercised
  against a separate disposable main checkout and production still starts with
  a new Inspect after merge.
- The path is intentionally v3-only until another accepted decision assigns a
  concrete need for v1/v2 branch preparation.

## Non-goals

- Publishing, merging, or accepting the Pack proposal.
- Replacing the canonical protected-main dispatch or its artifacts.
- Automatic retries, AI-to-human fallback, or weaker evidence validation.
- Granting branch code repository/ref/pull-request mutation authority.

## Enforcement

Structural tests require separate production/preparation concurrency, strict
issue-branch admission, read-only Prepare permissions, credential-free
checkouts, and an unchanged protected-main Publish gate. Domain tests prove the
shared preparation prefix never calls `Publish` or `Finalize`. Adapter tests run
the composite lifecycle through Prepare and assert non-authoritative evidence.
A live exact-request branch run plus before/after remote mutation observations is
required delivery evidence for changes to these external seams.
