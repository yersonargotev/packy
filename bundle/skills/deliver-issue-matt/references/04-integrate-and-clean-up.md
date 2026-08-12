# Integrate and clean up

Immediately before integration, re-fetch the issue, change request,
integration branch, candidate head, reviews, conversations, CI, and protected
merge state. Require the issue to remain ready and materially unchanged, the
head to equal the exact proven candidate, and every required control to remain
satisfied. A material authority change returns to qualification.

If the integration branch advanced, incorporate it only when the platform
requires currentness. Merge it into the candidate without rebasing or
force-pushing. Treat the result as a new candidate, then complete the
implementation and change-request gates again before returning here.

Once freshness and every protection are satisfied, publish the final evidence
defined in the invariants.

Use the repository's protected path without bypass or administrator override.
Choose the first available method in this order:

1. A required merge queue.
2. Squash merge.
3. Merge commit.

Do not use rebase merge. Record the exact candidate SHA, selected method, and
tracker-reported integration commit.

Read the integrated change request and commit back from the tracker and remote.
Require the recorded candidate to match the change-request head and the
integration commit to be reachable from the integration branch. For a merge
commit, also require the candidate as its ancestor. For squash merge or the
tracker's equivalent, use its merged-change association plus the recorded head
and integration commit as identity proof, then verify the accepted change on
the integration branch.

Verify that the issue closed through the configured closing mechanism. If it
did not, close it explicitly through the configured tracker workflow and link
the integrated change request.

Delete the local and remote issue branch, temporary worktree, and verification
state only when this delivery owns them. Recheck the operator checkout: an
adopted issue workspace ends clean on the verified integration branch; a
preserved workspace retains its original branch, HEAD, and status exactly.
Unexpected or ambiguously owned state produces a cleanup exception.

Deliver the success brief defined in the invariants.

**Complete when:** the exact candidate's protected integration and issue
closure are verified, delivery-owned resources are removed, operator state is
preserved, and the success brief supports every claim.
