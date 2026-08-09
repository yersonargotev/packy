---
status: accepted
---

# Make Pack readiness capability-driven and condition-based

## Context

Packy's readiness behavior has mixed host observations with Pack-specific
policy. That makes an unobservable runtime look like a failed Pack, permits
host adapters to choose behavior from Pack or resource identities, and makes a
new reviewed Pack depend on unrelated code changes. It also loses the
distinction between a confirmed failure, a pending human action, and state that
Packy cannot observe.

The Reviewed Pack catalog is data-driven under ADR 0033. Readiness must have
the same property: a reviewed Pack that uses existing manifest and surface
vocabularies is admitted through data, while genuinely new host behavior is an
explicit reviewed capability.

## Decision

The capability-pack domain owns readiness policy. It evaluates the reviewed
readiness obligations that apply to a Pack from fresh surface observations and
approved controlled runtime checks, creates readiness conditions, aggregates
them, and decides strict readiness gates and health meaning. The existing
capability-pack facade is the use-case and test seam. CLI commands, Doctor,
structured output, and the TUI are driving adapters that faithfully present its
result.

CLI surface adapters are driven adapters. They report host facts and implement
the closed, reviewed surface-capability vocabulary; they do not choose semantic
readiness behavior by Pack identity, Pack version, or a fixed resource
identity. Identity remains valid for requested-Pack lookup and for ownership,
intent, receipts, locks, and lifecycle state.

Pack manifests take a clean schema cut. They declare only typed, validated
readiness obligations beside the resources and surface bindings to which they
apply. Existing external requirements and universal receipt-backed projection
integrity remain their own sources of truth and produce obligations without
duplicating manifest declarations. Unknown obligation vocabulary is invalid.
Arbitrary Pack-provided probes, executable readiness code, generic commands,
custom readiness blobs, and untyped extension data are prohibited.

This decision narrows ADR 0031's minimal installed-receipt shape: a receipt now
also seals the reviewed readiness obligations and external-requirement names
that governed the installed Pack version. These are policy references, not
runtime evidence. Keeping them with the exact installed identity permits
offline project inspection without treating a mutable catalog as the authority
for an older installation. Runtime observations and controlled-check evidence
remain excluded from project locks and Git history.

Each evaluated obligation produces a readiness condition with a stable type,
one readiness dimension (`configured`, `authorized`, or `usable`), a
three-valued result (`true`, `false`, or `unknown`), stable reason,
user-facing message, scoped evidence references, observation time, and
validity identity. Within a dimension, `false` dominates; otherwise any
`unknown` makes the dimension unknown; a dimension is true only when every
condition is true. This condition and aggregation model applies unchanged to
both global and project Pack lifecycle.

Activation may complete as configured while authorization or usability is
unknown. An explicit strict usability gate requires fresh true usability and
therefore fails closed for false or unknown evidence. Status, lifecycle output,
structured output, Doctor, and the TUI preserve the domain condition rather
than translating unknown into false.

A controlled runtime check is an explicit operation, separate from activation,
for approved host behavior that Packy cannot otherwise observe. It records
either positive or negative personal workstation evidence. That evidence lives
only in Packy Home, never in Pack or project manifests, project locks, or Git
history. Its validity identity includes the Pack identity and version, CLI
surface, selected resource closure, projection revision, adapter version, and
observable host version; a change to that identity makes it stale. No arbitrary
time-to-live is implied.

Doctor has informational conditions and a corresponding count. Informational
unknown conditions with no pending human action do not degrade overall health.
Confirmed false conditions, pending actions, and recoverable drift remain
warnings; unobservable or corrupt state that prevents inspection fails.

Formal surface capabilities are the only route for reusable host-native
projection and observation behavior. External tools receive generic PATH
observation; tool-specific acquisition is an explicit capability. This replaces
identity-selected behavior for existing host integrations without granting a
Pack a custom execution path.

## Consequences

Readiness has one domain-owned explanation across lifecycle commands, Status,
structured output, Doctor, and the TUI. Reviewed catalog growth using known
vocabulary does not require adapter policy changes. New host-native behavior
requires a reviewed vocabulary addition and reusable surface implementation.

Catalog-level behavioral fitness tests must demonstrate that differently named
synthetic Packs and resources using existing vocabulary work in global and
project scope. A narrow static guard must reject literal Pack or resource
identity dispatch in adapter policy while allowing the legitimate identity
comparisons described above.

This ADR establishes the target architecture only. The incremental delivery
work in issues 619 through 625 chooses and implements the individual tracer
bullets; it must not retain retired manifest or structured-output contracts as
compatibility paths. ADR 0032 remains authoritative that Orchestrate is
configured after exact projection and runtime-unknown until a fresh controlled
runtime check observes delegation.
