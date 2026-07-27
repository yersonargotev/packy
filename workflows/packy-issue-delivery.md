# Packy Issue Delivery

Status: Active

## Goal

Turn a requested Packy GitHub issue into a verified change merged to `main`
through one predictable delivery loop:

`LOCAL[qualify -> optional bug diagnosis -> LOOP(implement -> code-review)]`
`-> NON-LOCAL[PR -> wait for CI -> green CI -> merge]`

## Skill shape

The implementation is the project-local, model-invoked skill
`deliver-packy-issue` at `.agents/skills/deliver-packy-issue/SKILL.md`. Its
model-facing description triggers only complete delivery of a named Packy issue;
consultations, isolated reviews, and releases do not trigger it.

The skill is a thin orchestrator over the two execution modes below. This
workflow is the full contract. The skill points here, to `AGENTS.md`, and to
existing diagnosis, implementation, delegation, and code-review skills instead
of copying their rules.

The primary agent retains requirements, decisions, integration, final
verification, and every GitHub mutation. Safe bounded implementation and
independent review slices may be delegated.

## Workflow

Remote reads are allowed in **LOCAL**. GitHub mutations begin only in
**NON-LOCAL**. Before every local commit, run the repository validation
authority, `./scripts/validate-packy.sh`.

### Trigger

The user identifies one Packy GitHub issue by number or URL and explicitly asks
for complete delivery. Record the immutable issue contents and the starting base
commit fetched from `origin/main` before changing project or tracker state.

### 1. LOCAL — Qualify

Read this contract and the repository instructions. Fetch the issue, confirm it
is open and labeled `status:needs-review` or `status:approved`, classify it as a
bug, feature, or non-code change, and verify that its acceptance criteria remain
current and implementable.

For a bug, apply `diagnosing-bugs` only while its reproduction, cause, or failure
boundary remains uncertain. When the issue already supplies sufficient diagnosis
and a clear reproducible regression, proceed directly to the local loop. Feature
and non-code branches do not invoke `diagnosing-bugs`.

For a needs-review issue, gather enough evidence to approve or reject it, but
record approval only as deferred intent. LOCAL performs no label or other GitHub
mutation. For an approved issue, perform the lighter currency check.

From the immutable issue, accepted ADRs or specification, dependency graph, and
qualification evidence, record a complete **qualified scope ledger**. Classify
each distinct obligation into exactly one of these mutually exclusive sections:

- **Owned now** — behavior this issue must implement and prove. Link every entry
  to an issue criterion, exact specification section, or accepted decision.
- **Deferred** — real behavior intentionally assigned elsewhere. Link the
  evidence and name a concrete owning issue, delivery-graph slice, prerequisite,
  or exact specification section; "future work" without an owner is invalid.
- **Forbidden** — behavior this issue must not introduce, including explicit
  exclusions, architecture boundaries, real-user mutations, publication or
  release effects, and premature product content.
- **External prerequisites** — human authority, legal evidence, credentials,
  infrastructure, or other facts the local implementation cannot produce.
  Record each prerequisite's current disposition and exception boundary.

Use Packy's accepted domain vocabulary and evidence-link every ledger entry. If
an obligation cannot be classified safely, appears in more than one section, or
has no owner when deferred, produce the existing decision-ready qualification
exception. Never turn ambiguity into silent deferral.

Before branch creation or edits, derive a complete **acceptance-evidence
matrix** from the immutable issue snapshot. Every acceptance criterion has one
traceable row containing:

- its criterion text or a stable row ID;
- the owning production seam, artifact, or documented boundary;
- the positive evidence that will prove the behavior;
- the required negative, failure, or mutation evidence;
- compatibility, preservation, migration, or a concise reason why that evidence
  is not applicable; and
- its current state: `planned`, `implemented`, or `proved`.

Rows may name the same implementation seam, but distinct acceptance obligations
must not be silently collapsed. Needs-review and approved issues use the same
matrix contract. An incomplete, ambiguous, contradictory, or unowned row enters
the existing decision-ready exception boundary.

Inspect `main`, `origin/main`, and the working tree. Use the normal checkout when
it is clean and synchronized; otherwise prepare a temporary clean worktree from
the fetched `origin/main` commit without changing operator state.

**Complete when:** the issue is valid, its type and acceptance evidence are
recorded in a complete matrix with every row `planned`, the qualified scope
ledger is complete, mutually exclusive, and evidence-linked, any approval
mutation is deferred, the immutable starting `origin/main` commit is known, no
exception boundary is active, and the chosen workspace is isolated from
unrelated changes. Failed validation produces an exception brief and stops
before branch creation or code edits.

### 2. LOCAL — Implement-review loop

Create `fix/issue-N-slug`, `feat/issue-N-slug`, or `chore/issue-N-slug`
according to issue type. Use CodeGraph before source discovery when the change
needs architecture, symbol, call-flow, or impact analysis.

Run these steps for every iteration:

1. Record `iteration-base-sha = HEAD` and an **iteration brief** that states the
   exact behavior or review repair this iteration must deliver and names the
   exact matrix rows it advances or repairs. Carry the qualified scope ledger
   and every prior scope adjudication into the brief.
2. Run Delegation Preflight. Delegate only a bounded local implementation slice
   with explicit file or module ownership. Keep small, cross-cutting,
   architectural, decision-dependent, or overlapping work inline. The primary
   agent inspects and integrates delegated changes and records the accepted or
   rejected handoff evidence.
3. Apply `implement` to the iteration brief. For a bug with a valid regression
   seam, apply `tdd`; for a feature, advance one vertical tracer bullet with
   public-seam tests where behavior is testable; for non-code work, use targeted
   artifact verification. Keep the delta surgical, run the required checks, and
   update the affected matrix rows with implementation and focused-check
   evidence without rewriting the immutable issue snapshot. Then create one
   coherent local commit. Do not push it.
4. Apply `code-review` with independent Standards and Spec axes against exactly
   `iteration-base-sha...HEAD`. Give the Spec review the immutable issue and the
   iteration brief, including the ledger and prior adjudications; it judges the
   obligations of this delta rather than treating earlier out-of-delta work as
   missing. Through this caller-owned context, instruct the Spec axis to report
   missing or wrong **Owned now** obligations assigned to the iteration; not
   report **Deferred** obligations as missing unless the delta contradicts,
   prematurely implements, or invalidates their named owner; treat **Forbidden**
   behavior as scope creep; and apply each unsatisfied **External prerequisite**
   according to its recorded exception boundary. Do not modify, vendor, or fork
   the shared `code-review` skill to supply these semantics.
5. Adjudicate every finding and update the affected matrix rows with the review
   evidence. Preserve scope adjudications in later iteration briefs. Rejected
   findings retain concise evidence and are not raised again without new
   evidence. Each accepted finding becomes a new iteration brief and returns to
   step 1, so its repair receives its own implementation commit and review
   delta; a finding cannot waive or silently reclassify an **Owned now**
   obligation.

Maintain cumulative evidence in the matrix for every issue acceptance criterion
and **Owned now** obligation, advancing rows from `planned` to `implemented` to
`proved` as their implementation, focused checks, and review evidence become
complete, while preserving the evidence and disposition of every other ledger
entry. Once the latest iteration has zero actionable
findings, run the final local gate on the unchanged `HEAD`: all acceptance
checks, `./scripts/validate-packy.sh`, `git diff --check`, relevant sandboxed
real-boundary checks, and confirmation that every matrix row is `proved`. Then
run the repository-owned machine-verifiable LOCAL gate against the canonical
evidence bundle and unchanged current repository, and retain its successful
canonical report. Any unproved row or failed machine-verifiable gate fails the
local gate. Do not add a cumulative code review; every committed delta has
already received its paired review.

**Complete when:** every implementation commit has a paired review of exactly
its preceding `iteration-base-sha...HEAD` delta, every finding is adjudicated,
the latest review has zero actionable findings, every acceptance criterion has
a `proved` matrix row, the issue branch contains only intended commits, and
every local gate, including the repository-owned machine-verifiable gate, passes
on its unchanged final `HEAD`.

### 3. NON-LOCAL — Deliver

Enter NON-LOCAL only with the successful canonical machine-verifiable LOCAL gate
report for the exact current `HEAD`.

Re-read the issue and its authoritative specification before the first
mutation. If either changed materially, return to LOCAL qualification and
replace the active ledger from the new immutable evidence; do not patch the
qualified ledger forward. If a needs-review issue still matches the qualified
snapshot, replace `status:needs-review` with `status:approved`. If safe
requalification is impossible or the mutation fails, stop before the PR with an
exception brief. Preserve the prior ledger and adjudication history as evidence.

Push the issue branch and create a PR to `main` with `Closes #N`, the change
summary, and validation evidence. Wait for every required CI check on the exact
locally proved `HEAD`.

Classify a failed check before acting:

- Retry an infrastructure failure or flake without changing code.
- For a failure attributable to the change, return to the LOCAL
  implement-review loop, record a new `iteration-base-sha`, implement and review
  the repair, rerun the final local gate, push the new `HEAD`, and wait for CI
  again.
- If a bug failure restores uncertainty about reproduction, cause, or boundary,
  apply `diagnosing-bugs` before the repair iteration.

Merge only when every required check is green for the exact proved PR `HEAD`.
Merge through GitHub with a merge commit and delete the remote branch. Fetch
with pruning and verify that `origin/main` contains the merge. Fast-forward local
`main` only when Git can preserve the operator checkout; otherwise leave it
untouched and report that it remains behind. Then clean up the local issue
branch. For temporary-worktree runs, remove the worktree before deleting its
branch; for in-place runs, switch to `main` before deleting the branch.

**Complete when:** the PR is merged, the issue is closed, the issue branch is
absent locally and remotely, `origin/main` contains the merge commit, the
integration workspace is clean, operator changes remain untouched, and the
success brief reports the local `main` synchronization result. Release
publication is outside this workflow.

## Checkpoints

There are no routine checkpoints after successful qualification. Technical
failures, failing tests, review repairs, and red CI remain autonomous loop work.
Stop only when acceptance criteria conflict or permit materially different
behavior; no deterministic reproduction or valid regression seam exists;
implementation requires a material scope, architecture, or real-user
configuration change; the issue or its authoritative specification changes
materially before the first mutation and cannot be requalified safely; an
acceptance-evidence row is incomplete, ambiguous, contradictory, or unowned; or
a review finding needs an unstated product decision.

Failed qualification leaves issue labels and state unchanged. Every exception
presents one decision-ready brief before the workflow continues.

## Briefs

The success brief links the issue and PR; names the merge commit; summarizes the
change and the completed acceptance-evidence matrix; summarizes relevant scope
decisions and preserved deferrals; reports local validation, every iteration
review, and CI; confirms cleanup; and notes preserved local state.

An exception brief consumes the affected matrix rows directly, presents their
evidence, summarizes the relevant scope decisions, external-prerequisite
dispositions, and preserved deferrals, explains why the workflow cannot choose
safely, lists concrete options, recommends one, and asks for exactly one
decision. Briefs link artifacts and omit raw logs.

## Definition of done

This workflow run is complete only when the NON-LOCAL criterion is satisfied or
an exception brief is waiting on the user's decision. This specification is
ready when an implementer can run `deliver-packy-issue` without asking another
question.
