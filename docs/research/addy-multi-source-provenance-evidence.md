# Addy multi-source provenance and lock ownership evidence

## Evidence question and boundary

This note gathers repository-local evidence for the Wayfinder question
**“Decide multi-source provenance and lock ownership.”** It records current
invariants and implementation gaps; it does not change schemas, bundle data,
runtime behavior, GitHub workflow behavior, or Addy registration. Acquired
upstream content remained inert.

## Existing ownership decisions

- ADR 0007 gives `internal/bundletransaction` the sole repository-local lock
  for complete bundle observations and mutations. `internal/packsync` alone
  owns full-bundle Apply, replacement, and Recover. Its first-production-lock
  exception does not authorize later partial or dual-write migrations.
- ADR 0009 places per-source dispatch serialization, branch and pull-request
  ownership, decision readiness, and invalidation in
  `internal/packsyncworkflow`. These are workflow ownership scopes, not
  transaction locks.
- ADR 0011 makes all five Pack Source schemas one exact, immutable suite. A new
  lock topology or artifact identity must ship as another complete suite
  rather than reinterpret `v1.0.0`.
- ADR 0005 keeps Pack Source synchronization separate from capability-pack
  lifecycle and host projection adapters. Source provenance must remain owned
  by synchronization rather than activation or host modules.

## Configuration is multi-source but locks are singular

- [`Config`](../../internal/packsync/types.go) already contains a list of
  `SourceConfig` values. [`LoadConfig`](../../internal/packsync/config.go)
  rejects duplicate source IDs and canonicalizes sources and resources.
- Binding duplicate detection currently resets for each source. The same
  `(pack_id, kind, resource_id)` can therefore be declared by two sources
  without a configuration error.
- [`Lock`](../../internal/packsync/types.go) represents exactly one source,
  repository, owner, selector, candidate, snapshot, and resource set.
- [`readCheckInputsUnlocked`](../../internal/packsync/check.go) reads that
  value only from `bundle/sources.lock.json`, and lock validation requires its
  source ID to match the selected source.

The checked-in bundle demonstrates the mismatch: `bundle/sources.json` is a
source collection, while `bundle/sources.lock.json` contains the single
`mattpocock-skills` provenance object. A second configured source cannot own an
independent prior candidate in the current topology.

## Current Check assumes one global source contribution

- Check targets one configured source but materializes one complete local
  observation under the shared bundle lock. It acquires the upstream candidate
  outside that lock, then reacquires the lock and byte-for-byte revalidates
  configuration, prior lock, source selection, manifests, and bundle facts
  before sealing a plan.
- Plan preconditions include repository base, configuration, manifests,
  complete bundle, and the singular lock. Exact candidate provenance is
  reacquired and compared before Apply.
- Plan construction compares the selected source's bindings against all
  resources in the old singular lock and creates a replacement lock containing
  only the selected source.
- Resource materialization deletes entries owned by the current lock that are
  absent from the next lock.

Without a per-source ownership model, applying a second source would therefore
treat unrelated resources from the first source as removals or replace their
only provenance record.

## Current transaction and recovery are correctly global

- [`Engine.Apply`](../../internal/packsync/transaction.go) reacquires the exact
  candidate before obtaining the bundle lock. Under the lock it verifies the
  sealed plan and local authority, clones and validates a complete staged
  bundle, records durable recovery evidence, and swaps the complete directory.
- [`Engine.Recover`](../../internal/packsync/transaction.go) accepts only the
  canonical marker paths, recorded phase, and compatible observed old/new
  complete-tree hashes. Missing, ambiguous, manipulated, or incompatible
  evidence blocks recovery.
- [`bundletransaction`](../../internal/bundletransaction) also protects bundle
  observations made by synchronization, skill-bundle, and capability-pack
  readers, preventing mixed generations.

These invariants should not be decomposed by source. Per-source files can be
carried independently in a staged generation while the mutation, verification,
and repair unit remains the complete bundle.

## Manual workflow is already source-scoped

- The manual workflow serializes runs by source ID without cancelling an
  active run.
- Publication derives `sync/<source-id>` and owns at most one open pull request
  for the source. Divergence, stale identity, changed provenance, moved base,
  or edited publication state blocks without overwrite.
- Validation and publication artifacts already bind source ID, exact plan,
  candidate, base, result tree, head, and a provenance digest. Decision-ready
  invalidation explicitly includes base and provenance changes.

The existing workflow can retain per-source operational ownership, but its
artifacts need separate target-lock and complete-lock-set digests to describe
both source provenance and the complete validated generation.

## Gaps the execution specification must close

1. Introduce the canonical per-source lock path and strict, path-safe source ID
   validation without adding another transaction lock.
2. Require an exact configuration-to-lock bijection and reject missing,
   orphaned, duplicate, or unexpected documents.
3. Reject duplicate portable bindings across sources and assign removal
   authority to exactly one source.
4. Make target-source Check load its own prior lock, validate the complete lock
   set, and carry every non-target lock and resource forward unchanged.
5. Bind plans and workflow artifacts to both target-lock and canonical
   lock-set digests while retaining all complete-bundle preconditions.
6. Add explicit atomic registration, removal, and multi-source transfer
   operations; routine synchronization remains a complete one-source update.
7. Preserve one complete-bundle recovery marker and fail closed on ambiguous
   source or transaction evidence.
8. Migrate the current singular lock in a separate clean cut and publish the
   new artifact contracts as one immutable schema suite.

## Evidence gathering performed

CodeGraph was used first to trace `packsync` configuration, Check, Apply,
publication, and recovery ownership. The accepted ADRs, checked-in source
configuration and lock, Pack Source workflow, and prior Addy source decision
were then read directly. No runtime behavior is claimed solely from CodeGraph.

After adding the decision and evidence assets, the repository authority passed:

```text
./scripts/validate-packy.sh
```
