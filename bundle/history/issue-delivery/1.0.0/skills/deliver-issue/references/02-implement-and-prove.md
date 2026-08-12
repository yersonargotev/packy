# Implement and prove locally

Create or resume the single issue branch from the qualified starting commit.
Make an acceptance matrix mapping every issue criterion to its owning change
surface and proof. Implement coherent deltas, using a practical test seam when
one exists. For each delta, format as required, run `git diff --check`, run the
smallest relevant policy-required checks, and commit the coherent result.

Run the policy's required local validation and manual verification against the
actual changed surface. Sandbox user-path state whenever the repository policy
or command can read or write it. If an acceptance tracer, interactive flow, or
durable side effect is part of the contract, exercise it and assert the complete
final observable state, including required absences. A residual effect is a
failure: diagnose, add practical regression proof, repair, and rerun it.

Apply every policy-required specialist assurance. Run proportional local
Standards and Spec review when the change's size, complexity, sensitivity,
ownership spread, or delayed-feedback cost warrants it; adjudicate each finding
and repair accepted findings. This local review never substitutes for the two
independent final axes.

Before opening a pull request, require every acceptance criterion to have
implementation and focused proof, applicable manual verification to pass, the
working tree to be clean, and all policy-required canonical validation to pass
on the exact candidate SHA. Record that SHA and its proof in durable Git or
GitHub evidence, without creating a separate ledger.

**Complete when:** the clean exact candidate implements and proves every
criterion, passes required local and specialist assurance, and has passed
canonical validation plus applicable manual verification.
