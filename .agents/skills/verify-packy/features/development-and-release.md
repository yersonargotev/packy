# Development and release guidance

Maintainers ask agents to develop approved issues or resume release work.
Development produces a PR. Integration and publication have separate authority.

## Sub-features

- Development discovers Packy verification and consumes independent review.
- A release correction stops at its pending PR.
- Release resumption observes integration before fresh candidate proof and approval.
- An explicit integration request uses Shipping under branch protection.
- Active development and release cleanup refuse missing proof before deletion.
- Completed runs retire historical detail after a durable closeout; pending work blocks retirement.
- Release observation detects both environment waits and separately checks approval identity.

## How to get to it (user POV)

Ask poteto-mode to implement an approved Packy issue. Ask release-packy to
prepare or resume a release. Authorize integration separately when desired.

## Driving it with controlled invocations

Follow the [development routing probe](../../../../scripts/probes/development-routing.md).
Run fresh model turns against the candidate instructions in isolated fixture
workspaces. Read the retained commands, responses, and reports. Confirm
verification discovery and reviewer evidence precede PR creation. Check that
pending integration causes no duplicate development, review, or publication.
After observed integration, require proof and approval for the new main commit.
Only the explicitly authorized Shipping control may request a protected merge.
Unapproved broader and post-tag corrections stop at the numbered scope gate.

For release progress changes, run `go test ./internal/ci -run '^TestReleaseObserver$'`
and follow the [release progress routing probe](../../../../scripts/probes/release-progress-routing.md).
Inspect actual tool traces for both approval checks, a refusal, and stop-before-resume.
The fake forge cannot establish real approval or publication.

For cleanup changes, first run `python3 -B scripts/probes/evidence-retention.py`
to check the real repository script. Then follow the
[evidence-retention routing probe](../../../../scripts/probes/evidence-retention-routing.md)
with the repository checker from the same candidate checkout. Run its partial-copy
and complete-retention cases for development and release. Also exercise active
missing-proof refusal, completed-run retirement for both workflows, and a
pending-work retirement refusal. Require an
actual nonzero preflight and no deletion on missing proof. For completed work,
require a durable closeout, explicit retirement inventory, and exit zero before
a separate exact-path deletion. Inspect final files, retained references,
operator state, unrelated worktrees, and Git refs in the actual tool traces.
Fixture integration and release-readback statements remain prepared inputs.

## Gotchas

- The opt-in probe needs a working Codex login and model access. Those are prerequisites, not permission to mutate a real repository.
- Forge, Git, validation, artifact, and review responses are labelled continuation inputs. The probe proves route selection and evidence consumption. Actual Packy verification, independent review, CI, and branch protection remain separate obligations.
- A missing tool observation or an unexplained failure is incomplete proof. Static wording checks cannot replace a tool trace.
