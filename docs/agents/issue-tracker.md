# Issue tracker: GitHub

Specs and tickets for this repo live as GitHub issues. Use the `gh` CLI for all operations.

## Conventions

- **Create an issue**: `gh issue create --title "..." --body "..."`. Use a heredoc for multi-line bodies.
- **Read an issue**: `gh issue view <number> --comments`, filtering comments by `jq` and also fetching labels.
- **List issues**: `gh issue list --state open --json number,title,body,labels,comments --jq '[.[] | {number, title, body, labels: [.labels[].name], comments: [.comments[].body]}]'` with appropriate `--label` and `--state` filters.
- **Comment on an issue**: `gh issue comment <number> --body "..."`
- **Apply / remove labels**: `gh issue edit <number> --add-label "..."` / `--remove-label "..."`
- **Close**: `gh issue close <number> --comment "..."`

Infer the repo from `git remote -v` — `gh` does this automatically when run inside a clone.

## Pull requests as a triage surface

**PRs as a request surface: no.** `/triage` processes issues only.

## Local working material

`.scratch/` may still hold temporary local research, drafts, and implementation notes. It is not the issue tracker: work that skills publish or track durably must be represented as GitHub issues.

## When a skill says "publish to the issue tracker"

Create a GitHub issue.

## When a skill says "fetch the relevant ticket"

Run `gh issue view <number> --comments`.

## Wayfinding operations

Used by `/wayfinder`. The **map** is a single issue with **child** issues as tickets.

- **Map**: a single issue labelled `wayfinder:map`, holding the Notes / Decisions-so-far / Fog body. `gh issue create --label wayfinder:map`.
- **Child ticket**: an issue linked to the map as a GitHub sub-issue (`gh api` on the sub-issues endpoint). Where sub-issues aren't enabled, add the child to a task list in the map body and put `Part of #<map>` at the top of the child body. Labels: `wayfinder:<type>` (`research`/`prototype`/`grilling`/`task`). Once claimed, the ticket is assigned to the driving dev.
- **Blocking**: GitHub's **native issue dependencies** — the canonical, UI-visible representation. Add an edge with `gh api --method POST repos/<owner>/<repo>/issues/<child>/dependencies/blocked_by -F issue_id=<blocker-db-id>`, where `<blocker-db-id>` is the blocker's numeric **database id** (`gh api repos/<owner>/<repo>/issues/<n> --jq .id`, _not_ the `#number` or `node_id`). GitHub reports `issue_dependencies_summary.blocked_by` (open blockers only — the live gate). Where dependencies aren't available, fall back to a `Blocked by: #<n>, #<n>` line at the top of the child body. A ticket is unblocked when every blocker is closed.
- **Frontier query**: list the map's open children (`gh issue list --state open`, scoped to the map's sub-issues / task list), drop any with an open blocker (`issue_dependencies_summary.blocked_by > 0`, or an open issue in the `Blocked by` line) or an assignee; first in map order wins.
- **Claim**: `gh issue edit <n> --add-assignee @me` — the session's first write.
- **Closure sweep**: during charting and after each resolution, before treating `Not yet specified` as empty, account for every known concern involving (1) external dependencies and authority or licensing; (2) source cardinality, ownership, provenance, and reproducibility; (3) domain or schema expressiveness and migrations; and (4) validation, acceptance, publication, and delivery prerequisites. Put each concern in exactly one place: an already resolved decision, a sharp live ticket, genuinely unphraseable fog, or Out of scope. Do not pre-slice unknown fog or turn implementation steps into Wayfinder decisions.

### Resolve planning-only work without a repository asset

1. Claim the open, unblocked child before work.
2. Resolve it through its declared HITL or AFK mode.
3. Post the complete resolution comment.
4. Close the issue.
5. Append one gist-and-link context pointer to the map's Decisions-so-far.

### Resolve work that publishes a durable repository asset

1. Claim the open, unblocked child before work.
2. Complete the decision or research and obtain final human confirmation for HITL work.
3. Apply `status:approved` only when executing an explicit Integrator decision.
4. Use one short issue branch and one pull request with a GitHub-recognized closing keyword for exactly that child.
5. Keep the child open through authorization checks and merge; never close it manually before the pull request integrates.
6. After merge and GitHub closure, post or finalize the resolution comment with links that resolve on `main`.
7. Append the map's one-line context pointer only after the issue is closed and the asset is durable.
8. Verify the merged commit, closed issue, map entry, deleted branch, and clean local and remote state.

Issues organize work but do not grant merge authority. Complete issue delivery
uses a protected pull request, ordinary Packy CI, CodeQL, traceable Standards
and Spec review, resolved conversations, and human merge through branch
protection. Release publication remains a separate workflow.
