# Common invariants

Git and GitHub are the only durable delivery state. Inspect them before acting
and on every resumed invocation. Do not create a run ledger or silently adopt
ambiguous state.

The primary agent owns qualification, the candidate, finding adjudication,
commits, GitHub mutations, merge, and final verification. It may use available
repository capabilities for bounded investigation, implementation, validation,
or either independent review axis when repository policy permits; final
authority remains with the primary agent.

Preserve the operator checkout. Record its branch, HEAD, and worktree status
before delivery. Never stash, reset, clean, discard, overwrite, or commit its
pre-existing changes. Work in it only when its state is clean, on the observed
default branch, and its HEAD exactly equals the fetched remote default-branch
commit; otherwise create a workflow-owned temporary worktree from that commit.

Use the deterministic branch `codex/issue-<number>-<short-slug>`. Do not create
a second branch or pull request for the same issue. Adopt an existing identity
only when its issue identity, owner, and history uniquely match. Competing or
ambiguous identities require an exception brief.

Bind every local proof, manual verification, CI result, review result, and
final evidence statement to the exact candidate SHA it exercised. A commit
that changes the candidate invalidates all affected proof. Re-run the required
proof for the new SHA; evidence may be reused only when both its SHA and its
applicable scenario remain unchanged.

Keep published history append-only: no rebase or force-push. Diagnose failures
before retrying. Retry an unchanged operation once only when evidence classifies
it as transient; after a second transient failure, pause. Repair deterministic
failures in a new commit within approved scope. Treat permissions, protection
rejections, competing remote state, and unclear ownership as exceptions.

Do not post progress comments. Use the pull-request body for implementation
and proof summary, one final evidence comment for the final SHA, and normal
replies to actual review conversations. Pause only when safe continuation needs
a product or scope decision, broader authority, changed acceptance criteria,
an irreversible effect outside repository integration, or a required-control
override. Technical repairs inside the approved issue remain autonomous.

The delivery authority covers repository integration only. Releases,
deployments, real migrations, production configuration, and other irreversible
external effects require separate explicit authority, even when the issue
mentions them. Apply any additional boundary declared by repository policy.

An exception brief names the blocking fact, phase, evidence, available choices,
and a recommended answer. A success brief links the issue and pull request and
states the final candidate and merge SHAs, required CI, final Standards and
Spec results, manual verification, issue closure, cleanup, and operator-state
preservation.
