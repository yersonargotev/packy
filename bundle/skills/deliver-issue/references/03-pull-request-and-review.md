# Open and prove the pull request

Push the issue branch without rewriting history. Open one ready pull request to
the policy's protected base branch. Its body uses a GitHub-recognized closing
keyword for exactly the approved issue and concisely maps implementation and
acceptance evidence to the policy-required validation and manual verification.

Run required CI and two independent final review axes for the complete
pull-request candidate:

- **Standards** compares the candidate with repository instructions, accepted
  architecture and domain guidance, and the policy's quality baseline.
- **Spec** compares the same candidate with the approved issue and every
  referenced normative source.

Run the axes in two independent reviewer contexts. Give each the candidate and
only its own authority sources; neither receives or relies on the other axis's
analysis, findings, or result. Available review skills may implement an axis,
but their names are not part of this contract. If two independent contexts are
unavailable, pause with an exception brief before merge. Bind both results to
the exact pull-request HEAD. Inspect advisory checks too:
operational advisory failures follow policy, while substantive correctness,
dependency, or security findings are adjudicated like review findings.

The primary agent adjudicates every finding. Repair an accepted finding in a
new commit, then return to local final candidate proof and repeat CI and both
final axes for the new SHA. Once the exact final HEAD is unchanged and proven,
publish one compact pull-request comment with its SHA, Standards and Spec
results, acceptance coverage, canonical validation, and manual verification;
link detailed evidence instead of raw logs.

**Complete when:** the ready pull request closes exactly the approved issue,
required CI and both independently executed final review axes pass on its
unchanged exact HEAD, all accepted findings are repaired, and the final
evidence comment records proof.
