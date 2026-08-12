# Qualify and isolate

Use `docs/agents/issue-tracker.md` to fetch the named issue, its complete
conversation, dependencies, and referenced normative sources. Use
`docs/agents/triage-labels.md` to resolve the tracker value mapped from the
canonical `ready-for-agent` role.

Require the issue to be open, carry exactly that ready state among the
configured triage states, have no unresolved declared dependency, and contain
one objective with sufficient observable acceptance criteria and limits for
implementation. A current stopper produces an exception brief; the ready state
authorizes implementation only after every qualification condition passes.
Require the issue and every referenced normative source to agree on scope and
behavior. A conflict produces an evidence-backed exception brief and stops
qualification.

Use `docs/agents/issue-tracker.md` as the authority for tracker identity,
conventions, and interface. Through that configured interface, resolve and
verify fetch, change-request creation and inspection, CI or pipeline
inspection, merge, supported comments, and issue closure. Use GitHub pull
requests, GitLab merge requests, local Markdown filesystem operations, or the
configured equivalent. Local Markdown also requires a Git remote integration
path and a documented ticket-state update. An unresolved required operation
produces an exception brief.

Resolve the integration branch from an explicit repository instruction or
tracker rule, falling back to the remote default branch. Fetch that selected
branch and record its exact commit. Search all worktrees, branches, change
requests, and commits for an existing delivery of the issue.

Adopt existing local work only when all of these facts are proven:

- Its branch or worktree has an unambiguous identity with the issue.
- Every commit and uncommitted hunk maps to the issue's scope.
- The workspace contains only changes owned by the issue.
- Its starting point can be determined.
- Adoption requires no stash, discard, or history rewrite.

Adopt one uniquely compatible existing change request or local candidate and
resume its earliest incomplete gate. Multiple candidates produce an exception
brief. When no candidate qualifies, preserve the operator checkout exactly and
create an owned temporary worktree from the fetched integration branch. The
operator checkout is eligible instead when it is clean, on the integration
branch, and exactly at its fetched remote commit.

Use the deterministic branch `codex/issue-<number>-<short-slug>` when creating
a branch. Record the operator checkout, qualified starting commit, workspace,
and adopted or created delivery identity.

**Complete when:** one open ready issue, its executable acceptance surface,
dependencies, tracker operations, integration branch, qualified starting
commit, isolated or safely adopted workspace, and single delivery identity are
known with no stopper or competitor.
