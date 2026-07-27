# ADR 0018: Keep universal merge gates product-wide

## Status

Accepted.

## Context

Packy's protected `main` branch accumulated six required contexts while
qualifying its delivery governance. Four protect repository-wide boundaries:
Packy-owned validation, the supported Claude floor, issue authorization, and
CodeQL. The other two do not provide universal enforcement.

`Dependency review` is deliberately advisory through both `warn-only` and
`continue-on-error`. Requiring its always-successful job records execution but
cannot reject a vulnerable dependency change. `Addy 1.1.0 promotion gate`
protects one capability Pack and requires a second trusted workflow, exact-merge
evidence transport, and a post-merge replay on every repository change.

Pack Sync is a separate manual publication operation. Its current workflow is
disabled while a bounded replacement is designed. GitHub Pages publishes
immutable schema identities from protected `main`; requiring a second Owner
approval for every Pages deployment created a queue without adding another
source-integrity boundary.

## Decision

Protected `main` has exactly four universal required contexts:

- `Validate Packy-owned code`;
- `Claude 2.1.203 package smoke`;
- `Governance / Validate authorization`; and
- `CodeQL`.

Dependency review remains a visible advisory PR job. Its result must not be
described or configured as a blocking guarantee unless a later decision removes
its warning semantics and proves the new enforcement boundary.

Addy promotion is not a universal merge concern. Packy's ordinary Addy
acceptance validation and release evidence remain part of the repository
validation and release flows, but CI no longer publishes an Addy promotion
context, replays that context on `main`, or runs a separate trusted Addy
governance workflow.

The manually disabled Pack Sync workflow remains checked in for diagnosis and
rollback while its replacement is owned separately. Governance expected state
records it as `disabled_manually`; re-enabling it requires the replacement's
accepted design and live proof.

The `github-pages` environment keeps its exact `main` branch policy and no
required reviewer. Protected-main integration is the publication admission
boundary for the public schema tree.

## Consequences

- Ordinary PRs retain repository correctness, primary-host compatibility,
  authorization, and static-security enforcement with fewer global identities.
- Capability-specific promotion cannot lock unrelated development.
- Advisory dependency findings stay visible without pretending to block.
- Disabling Pack Sync intentionally blocks Pack-source publication but does not
  block ordinary PRs, releases, or Pages after expected state is updated.
- Pages deploys automatically after a protected-main update.

## Enforcement

The versioned governance expected-state contract lists only the four universal
required contexts, omits the retired Addy workflow, records Pack Sync as
`disabled_manually`, and records no Pages reviewers. Workflow structural tests
forbid reintroducing either Addy CI job or the trusted collector. Governance
shadow qualification proves the three Actions check-runs plus the separate
Governance commit status; Dependency review is not part of that qualification.

Because this change removes a currently required check producer, the transition
updates only the live required-status-check endpoint to the four already
qualified contexts immediately before opening the locally proved pull request.
All other branch protections remain unchanged. The resulting short transition
window is closed by merging the expected-state update and then collecting a
fresh governance observation.
