# Delivery invariants

## Durable state

Git and the configured issue tracker are the only durable delivery state.
Reconstruct each run from them and adopt existing state only when its identity
and ownership are unambiguous.

## Ownership

The primary agent owns qualification, candidate changes, finding adjudication,
commits, tracker and remote mutations, integration, verification, and cleanup.
Use independent contexts where the vendored implementation workflow requires
them, but retain final authority in the primary agent.

## Candidate proof

Bind automated proof, manual verification, review, CI, and integration claims
to the exact candidate SHA they exercised. A candidate change invalidates all
affected proof. Reuse evidence only when its SHA and applicable scenario remain
unchanged.

## Repair cycles

Diagnose and classify a failed gate. Retry an unchanged operation once when the
evidence classifies the failure as transient. For deterministic findings, a
**repair cycle** gathers every accepted finding from that failed gate, runs the
vendored implementation workflow, commits the repair, and reruns every affected
gate. A reappearing deterministic failure consumes another cycle. Three repair
cycles are the limit for one gate; the next failure requires an exception
brief.

## Published history

Keep published history append-only and use a new commit for each repair. Amend,
rebase, and force-push are forbidden after publication.

## Communication

Confine routine tracker communication to the change-request body and one final
evidence comment when the tracker supports it. The body carries the issue link,
implementation summary, and proof. The final comment names the exact merge
candidate and its acceptance coverage, automated proof, Standards, Spec,
manual verification, and CI results.

## Authority

Continue autonomously through technical repairs inside the ready issue. Pause
when safe continuation requires a product or scope decision, broader authority,
changed acceptance criteria, unavailable required access, an irreversible
external effect, or a protected-control override.

Delivery authority ends at repository integration and issue closure. Releases,
deployments, real migrations, production configuration, package publication,
and other irreversible external effects require separate explicit authority,
including when the issue mentions them.

## Briefs

An exception brief states the blocking fact, current gate, evidence, available
choices, and recommended resolution. A success brief identifies the issue and
change request; candidate and integration SHAs; review, validation, and CI
results; issue closure; cleanup; and operator-state preservation.
