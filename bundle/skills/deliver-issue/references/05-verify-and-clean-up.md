# Verify and clean up

Read the merged pull request and integration commit back from GitHub. Require
the pull-request record's head SHA to equal the final candidate and the
reported integration commit to be reachable from the protected base. For a
merge commit, also require the candidate to be its ancestor. For squash or
rebase merge, use GitHub's merged-PR association plus the exact recorded head
and integration commit as the identity proof, and verify the integrated change
against the acceptance matrix on protected base. Verify that the approved
issue is closed by that pull request. If the verified merge did not close it,
close the
issue explicitly with a link to the pull request, when the policy authorizes
that corrective action.

Delete only the workflow-owned local and remote issue branch and temporary
worktree or disposable verification state, and only after merge and remote
verification succeed. Recheck the recorded operator checkout: a temporary
workspace leaves its branch, HEAD, and status exact; a selected operator
workspace returns clean to the policy's protected base and its verified merged
state without discarding unexpected changes. Retry cleanup only where ownership
remains unambiguous.

Deliver the compact success brief defined by the common invariants. A cleanup
failure leaves the run incomplete.

**Complete when:** the protected merge and issue closure are verified,
workflow-owned resources are removed, the operator state is preserved, and the
success brief supports every completion claim.
