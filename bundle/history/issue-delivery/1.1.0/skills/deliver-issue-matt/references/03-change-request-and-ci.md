# Open the change request and pass CI

Push the issue branch without rewriting history. Create one ready pull request,
merge request, or configured equivalent targeting the integration branch. Use
the tracker's recognized closing mechanism for exactly the ready issue. In the
body, map the implementation and acceptance criteria to automated proof,
Standards and Spec review, and manual verification.

Wait for every required CI check or pipeline to reach a terminal state. Inspect
advisory results too. Classify operational failures with evidence; retry an
unchanged transient operation once. Adjudicate deterministic correctness,
dependency, or security findings like review findings.

For accepted CI findings, run one repair cycle through
[`implement.md`](implement.md) and push its new commit. The workflow establishes
full-suite and independent-review proof for the new candidate; then rerun every
affected manual scenario and the complete required CI set. Apply the repair
limit from the invariants.

Before declaring the gate complete, require an unchanged candidate, passing
required CI, resolved substantive conversations, and every required repository
protection. Prepare compact evidence naming the exact SHA and its acceptance
coverage, automated proof, Standards, Spec, manual verification, and CI
results.

**Complete when:** one change request targets the integration branch, is
configured to close exactly the ready issue, and all required CI, protection,
conversation, review, and verification evidence pass on its unchanged exact
head SHA.
