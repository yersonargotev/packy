> For on-demand use, read `../ACCESS.md` first and carry its initiating
> surface/snapshot, target, prepared resource paths and writable output context.
> Resolve playbook paths from the returned resource root.

> Apply the parent skill's host execution contract. Report unavailable
> scheduling, independent review, or live verification as incomplete gates.
> Keep external actions within the user's authorization.

### Autopilot-stack

Run the prepared [Autopilot-full playbook](ACCESS-autopilot-full.md), but do not merge. Each tick re-reads both prepared access playbooks from the initiating Catalog Snapshot, and this one wins where they differ.

1. **In place of step 5, stop at verified.** Stop when each PR is verified and green at its current head, then hand the PRs to the operator. After the handoff, nobody pushes without the operator's go. Every loop tick prompt names this file, so each tick sees the no-merge rule.
2. **Stack a PR only when it builds on another PR.** This is the one exception to step 3. Base the PR on the other PR's branch in place of trunk, per **Size and stacks** in `playbooks/ACCESS-opening-a-pr.md`. In place of step 2's rebase onto trunk, each PR rebases onto its parent PR's branch, and the stack root rebases onto trunk. A new head on the lower PR restacks the PRs above it, and a PR whose `git patch-id` the restack leaves unchanged keeps its verdict.

**Reply:** list each PR with its verdict, in place of what merged.
