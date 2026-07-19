# ADR 0012: Adopt source-scoped Pack Source provenance

## Status

Accepted.

## Context

Packy configuration can declare multiple Pack Sources, but production
synchronization currently persists one repository-global
`bundle/sources.lock.json`. That topology cannot identify each source's exact
candidate and complete contribution independently, cannot admit a second
source without ambiguous ownership, and cannot distinguish a target source
lock from the complete provenance generation.

[ADR 0007](0007-serialize-complete-bundle-transactions.md) correctly assigns
one complete-bundle transaction and recovery boundary to
`internal/bundletransaction` and `internal/packsync`. Its singular-lock
bootstrap exception was sufficient for the first production source but is not
the steady-state multi-source contract.

The detailed policy and evidence are recorded in the
[multi-source provenance decision](../research/addy-multi-source-provenance-policy.md)
and its [supporting evidence](../research/addy-multi-source-provenance-evidence.md).

## Decision

Packy persists one canonical provenance lock per configured Pack Source at:

```text
bundle/sources/<source-id>.lock.json
```

Every committed bundle generation has an exact bijection between path-safe
configured source IDs and canonical source lock documents. Each lock owns one
exact candidate and that source's complete configured contribution across all
affected Packs. Every portable `(pack_id, kind, resource_id)` binding has one
exclusive source owner; equal bytes do not imply shared ownership or transfer.

Packy persists no aggregate lock index. It canonicalizes the complete ordered
set of source IDs and source-lock digests to derive `lock_set_sha256`. A sealed
source operation retains both the target `source_lock_sha256` and complete
`lock_set_sha256` alongside its existing configuration, manifest, base, result
tree, and publication preconditions. A change to either digest or any complete
bundle precondition makes the operation stale.

Per-source provenance does not create per-source mutation. ADR 0007's single
complete-bundle lock, sibling-staged replacement, two-rename swap, recovery
marker, and observed old/new-tree recovery authority remain unchanged.
`internal/packsync` replaces one target source's complete contribution while
constructing and validating one complete new bundle generation.

Registration and removal atomically add or remove configuration, the exact
source lock, selected resources, affected Packs, history/evidence, and every
derived digest. Moving a binding between sources is an exceptional explicit
multi-source migration that seals both provenance chains and updates both
complete contributions in one transaction. Routine single-source proposals
cannot transfer ownership.

The current singular lock migrates first, in a separate clean change, to
`bundle/sources/mattpocock-skills.lock.json`. The migration proves its exact
candidate, snapshot, resources, modes, file digests, and selected bytes
unchanged while every producer, consumer, validator, fixture, workflow
artifact, and document adopts the new topology. It then deletes
`bundle/sources.lock.json`. There is no legacy reader, fallback, or dual write.

The topology and artifact contract move through a new complete immutable Pack
Source schema suite under ADR 0011. Existing published suite bytes and instance
meanings remain unchanged.

## Consequences

- Each Pack Source has independently auditable exact provenance and complete
  contribution ownership.
- The complete lock-set digest preserves a global bundle-generation freshness
  boundary, so merging any source proposal invalidates every proposal sealed
  against the prior generation.
- Workflow concurrency and publication ownership may remain source-scoped
  without weakening global transaction, recovery, or freshness semantics.
- Addy registration can occur only after the singular-lock migration is
  complete and must use the ordinary no-bypass manual workflow.
- ADR 0007's singular-lock bootstrap exception is historical and is superseded
  only for provenance topology; all of its transaction and recovery ownership
  remains accepted.

## Enforcement

Repository and domain tests require configuration-lock bijection, path-safe
source IDs, deterministic lock enumeration and digesting, exclusive binding
ownership, complete source replacement, stale target/set digest rejection, and
atomic registration/removal/transfer. Migration tests prove unchanged selected
content and reject mixed singular/per-source topology, missing or orphaned
locks, fallback reads, and dual writes.
