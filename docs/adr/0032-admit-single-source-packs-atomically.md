---
status: superseded by ADR 0038
---

# Admit single-source Packs atomically

## Context

Pack Source v2 can register a source only when its bindings target an existing
Pack manifest. Pack Source v3 can create a previously absent Pack, but only as
a Composite Pack Source Bundle with at least two sources. Precreating a Pack
manifest for a one-source Pack would expose a partial generation without
sealed provenance.

This gap blocks the accepted Orchestrate Pack. Orchestrate contributes one
coordination skill to Codex, and its content must remain reproducible without
weakening Packy's transaction or legal-admission boundaries.

## Decision

Packy will publish an immutable Pack Source v2.3.0 five-schema suite. Its v2
`register` operation supports initial single-source Pack admission by requiring
the complete proposed Pack manifest, its canonical digest, the proposed Pack
version, and durable digest-bound legal admission evidence. The source
bindings and the manifest's source-backed resources must match in both
directions.

The registration plan seals the source and candidate identities, proposed
manifest and version, legal evidence, base generation, classification, source
lock, complete lock set, and result tree. A successful transaction publishes
the source configuration and lock, selected resources and legal notice, Pack
manifest, catalog entry, and immutable initial history as one bundle
generation.

Inspection rejects an existing target Pack or source identity. Invalid,
unavailable, stale, conflicting, legally unresolved, no-op, or partially
applicable proposals fail closed and leave the previous bundle byte-for-byte
unchanged. Publication reuses the complete-bundle staging, validation,
replacement, and recovery boundary.

Pack Source v3 continues to require two or more sources for a Composite Pack
Source Bundle. A one-source admission does not become a one-member composite.
The private synchronization workflow remains the only admission surface.

The official `orchestrate@1.0.0` Pack will support Codex only. Stable releases
of `yersonargotev/orchestrate-skill` are its canonical exact-content source.
The initial admission selects the complete upstream `orchestrate/` directory
as `skill:orchestrate`, preserves the native `$orchestrate` identity, and
includes `LICENSE` as `notice:mit`. That notice retains its MIT copyright
attribution to Eric Provencher; repository ownership does not replace the
upstream attribution.

Packy may report Orchestrate as configured only after projecting its exact
tree. Runtime delegation remains unverified until a controlled manual Codex
check observes it. Adding the v2.3.0 capability, publishing the upstream
release, and admitting Orchestrate are separate issue-bound deliveries.

## Consequences

Packy can admit an absent Pack with exactly one source without exposing a
manifest, provenance, content, or attribution independently. Existing
immutable schema suites remain unchanged, and v3 retains a clear multi-source
boundary.

Orchestrate does not enter the Reviewed Pack catalog merely because this
decision is accepted. A later approved registration must use a stable upstream
release and pass the normal classification, validation, review, CI, and
protected-merge controls before the Pack becomes selectable.
