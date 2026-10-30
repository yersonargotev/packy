# Packy development policy

Use the existing [poteto-mode playbooks](../../.agents/skills/poteto-mode/SKILL.md)
for development. This policy supplies repository obligations at their existing
verification, review, and PR stages. It adds no development phase sequence.

## Scope and ownership

Read the complete issue, its comments, and the current Agent Brief through the
[issue tracker](issue-tracker.md). Implementation requires an explicit request
for the numbered, approved scope. Approval alone starts no work.
The bounded pre-tag release-note exception belongs to
[release-packy](../../workflows/packy-release.md#repository-change-exception).

Before creating work, inspect matching issues, PRs, branches, and worktrees.
Adopt uniquely matching work when ownership and history are clear. Report
competing work without overwriting it or creating a duplicate PR.
Use an isolated worktree and preserve the operator's branch, HEAD, and changes.
Never edit Pack-owned installed or prepared resources to change their source.

## Verification in normal development

At the chosen playbook's verification step, use
[verify-packy](../../.agents/skills/verify-packy/SKILL.md).
Select the changed journeys from its feature map and update the map when a
user path changes. Use its maintenance loop for a complete map refresh.
For agent instructions, exercise controlled invocations with actual tool
traces for the changed decisions. Text assertions alone do not prove routing.

Retain canonical validation, the actual changed-path checks, and final-state
evidence for the candidate. Even a non-CLI change builds the binary and runs
`version`. Apply focused checks while iterating and `./scripts/validate-packy.sh`
for cross-cutting changes. Keep `go test ./...` green.
Keep proof outside disposable state and user configuration. Follow AGENTS.md
for sandboxed user paths and explicit test-process environments.

Apply specialist verification or assurance required by the affected surface.
Reuse [verify-projection-batches](../../.agents/skills/verify-projection-batches/SKILL.md)
when its trigger applies. A missing prerequisite, failed check, or known
specialist finding remains visible. It cannot become a passing result.

## Review at the existing PR stage

The poteto Opening a PR review point hosts two independent assessments of the
complete candidate. Existing review roles can carry these assignments when
they cover the obligation. Otherwise assign two fresh read-only host reviewers.
Each reviewer must be independent of the implementation and of the other axis.
The author adjudicates findings and implements accepted repairs.

| Assignment | Required assessment |
| --- | --- |
| Standards | Complete diff against AGENTS.md, relevant accepted ADRs, domain language, package ownership, engineering principles, process isolation, and applicable specialist policies. |
| Spec | Complete candidate against the approved issue, current Agent Brief, every referenced normative source, acceptance coverage, and authority limits. |

Apply the existing `no-comments` capability before review. Its comment and
suppression audit is part of Standards, not a substitute for either axis.
Use an available equivalent when an external review integration is missing.
Report unavailable independent review as incomplete.

Each result identifies the exact candidate HEAD, scope, reviewer identity,
findings, and verdict. Keep both results and their acceptance coverage with the
verification evidence. A candidate change invalidates both final reviews and
affected proof. Recheck the changed path and canonical validation on the new
HEAD. Record required ordinary CI and CodeQL on that same PR HEAD.
Publish a compact candidate evidence summary with the PR. Link detailed
artifacts instead of raw logs. Do not post periodic issue progress comments.

## Integration authority and cleanup

Normal development ends with a ready PR for human review and merge.
Issue approval, a development or release request, green checks, review approval,
and waiting for an owner approval never authorize merge or auto-merge.
Opening a PR does not start Babysit. An explicit status or repair request uses
the existing Babysit playbook and stops at its human boundary.

Only a separate explicit integration request routes to the existing Shipping
playbook. Apply its independent user-path verification and repository branch
protection. Immediately reobserve the approved issue, PR HEAD, reviews, resolved
conversations, required CI, CodeQL, and merge state. Never use an administrator
override or bypass a required check. Do not rewrite pushed history.

While a PR is pending, retain its branch and enough handoff evidence to resume.
Disposable verification state may be removed after preserving proof. Remove
owned development branches and worktrees only after observing integration and
checking that they contain no unexpected operator changes. Confirm issue
closure and the integrated candidate. Never reset the operator's checkout.
Release publication follows its own exact-version and commit approval.
