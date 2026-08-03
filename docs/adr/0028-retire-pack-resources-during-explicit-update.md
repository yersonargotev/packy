# ADR 0028: Retire pack resources during explicit update

## Status

Accepted.

## Context

An admitted capability-pack version may intentionally retire a projected
resource. A desired-only update inspection cannot identify the host projection
owned by the previous version, so it can advance durable intent while leaving
obsolete instructions or other resources active. Treating every update like
deactivation would be unsafe because historical content, a familiar path, or a
matching resource name is not deletion authority.

ADR 0005 reserved prior-versus-desired inspection for deactivation and required
a separate decision for new cleanup behavior. Matty 4.0.0 is the first admitted
contract that needs explicit update-time retirement.

## Decision

An explicit global pack update may supply both prior and desired composition to
the surface adapter only when Packy has an admitted update route and a trusted,
immutable historical artifact for the active version. Other updates continue
to inspect desired projections only.

Surface adapters translate the prior-versus-desired transition into host-native
absence goals and removal actions, but acquire no deletion authority.
`internal/capabilitypack` remains the sole owner of lifecycle meaning and may
schedule an update removal only when durable ownership identifies the retiring
pack as the exact sole contributor and the freshly observed fingerprint still
matches that ownership record. Drifted, foreign, shared, missing-ownership, or
otherwise ambiguous content is preserved and blocks the affected cleanup.

Authorized removals run in the separately approved `destructive-cleanup` phase.
Stale-plan preflight observes the same historical transition again before any
mutation. Post-apply verification requires every authorized retired projection
to be absent while all remaining desired projections match. Partial failure
retains the lifecycle journal and ownership evidence for recovery.

This decision refines the lifecycle-neutral adapter boundary in
[ADR 0005](0005-capability-pack-surface-adapter.md). It does not authorize
implicit version changes, cleanup during status or activation, deletion based
only on historical bytes, or reversal of external configuration without the
receipts required by [ADR 0024](0024-reverse-only-receipted-external-configuration.md).

## Consequences

- Explicit updates can converge the host to a contract that removes resources
  instead of leaving obsolete Packy-owned behavior active.
- Immutable history becomes inspection evidence, not deletion authority.
- Operators see and approve update cleanup separately from reversible writes.
- Content whose ownership or fingerprint is not exact survives the update for
  explicit resolution rather than being adopted or deleted.
- Every new retirement route requires immutable history, explicit route and
  compatibility evidence, adapter coverage, and post-cleanup verification.
