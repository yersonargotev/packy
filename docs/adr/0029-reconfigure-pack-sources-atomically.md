# ADR 0029: Reconfigure Pack Sources atomically

## Status

Accepted.

## Context

A configured Pack Source may need an explicitly approved binding-set change,
such as an upstream resource rename or a complete allowlist expansion. Packy's
configuration, source lock, Pack manifest, immutable history, compatibility
evidence, and vendored bytes form one generation. Changing any of those in an
intermediate commit would violate the complete-bundle transaction and the
source-scoped provenance bijection established by ADRs 0007 and 0012.

Routine synchronization deliberately cannot infer selection changes, while
absent-source registration cannot describe a replacement of an existing
source. A separate operation is required.

## Decision

Pack Source schema suite v2.1.0 adds an explicit `reconfigure` operation for
one existing source. Its request carries and canonically seals one complete
replacement `SourceConfig` and one complete current-version proposed Pack
manifest. The source ID, provider, repository, configured selector, and Pack
ownership remain unchanged. Proposed bindings and manifest resources must
match exactly in both directions, and bindings owned by another source cannot
be acquired.

Check observes the current generation, validates the complete proposal before
candidate acquisition, and seals both states with the existing base,
configuration, manifest-set, bundle, source-lock, and lock-set preconditions.
It derives additions, removals, the compatibility floor, and affected Pack
instead of accepting a requested version.

Apply writes the replacement configuration, classified manifest, changed-
selection compatibility evidence, immutable classified history, source lock,
and selected bytes in the existing sibling-staged complete-bundle transaction.
The evidence binds the ordered before/after binding sets, their exact delta,
old/new manifest digests, plan, base, candidate, and final classified version.
It cannot claim upstream provenance or replace the source lock. Existing
unchanged-selection evidence retains its current meaning and validation.

This operation cannot remove a source, change source identity, span multiple
Packs, transfer ownership, weaken stale-plan checks, or auto-merge publication.

## Consequences

- Approved selection changes no longer require an inconsistent intermediate
  repository generation.
- Routine synchronization and registration keep their narrow meanings.
- A removed or renamed resource retains the existing major floor; an isolated
  addition retains the existing minor floor.
- Recovery remains generation-based and needs no new partial-state authority.
- Every reconfiguration request is larger because it carries complete,
  reviewable configuration and manifest proposals.
