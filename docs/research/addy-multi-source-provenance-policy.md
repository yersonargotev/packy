# Addy multi-source provenance and lock ownership policy

## Decision question

How should Packy represent and transact independent per-source locks and
provenance for Addy alongside existing Pack Sources while preserving
exact-candidate continuity, complete-bundle atomicity, recovery, and the
manual workflow's freshness contract?

This decision consumes the repository-local
[evidence](addy-multi-source-provenance-evidence.md), the decided
[Addy source policy](addy-source-versioning-policy.md), and ADRs
[0007](../adr/0007-serialize-complete-bundle-transactions.md),
[0009](../adr/0009-own-manual-synchronization-orchestration.md), and
[0011](../adr/0011-publish-versioned-pack-source-schema-suite.md). It plans the
required model; it does not migrate the bundle, register or synchronize Addy,
or execute acquired upstream content.

## Ownership layers

Pack Source ownership and bundle transaction ownership remain distinct:

- each configured Pack Source owns one exact provenance lock and its complete
  selected contribution;
- `internal/packsync` owns Check, sealed plans, exact-candidate admission,
  Apply, and Recover; and
- `internal/bundletransaction` remains the only repository-local lock owner
  for complete bundle observations and mutations.

Independent source locks are provenance documents, not filesystem transaction
locks. Packy never mutates, swaps, or recovers a source subtree independently
of the complete `bundle/` generation.

## Canonical per-source locks

Every configured source has exactly one canonical document:

```text
bundle/sources/<source-id>.lock.json
```

The source ID must satisfy the existing canonical ID rules and be safe as one
path segment. The document retains the current source-level lock contract:
repository and owner identity, selector, exact candidate and verification
chain, snapshot digest, and every selected resource and file digest.

Packy persists no aggregate lock or generated lock index. It enumerates the
canonical directory, rejects unexpected paths, orders documents by source ID,
and requires a bijection between `bundle/sources.json` and the lock set. A
missing, duplicate, malformed, or orphan lock invalidates the bundle; it is not
a pending or recoverable source state.

## Complete source contribution

One source lock describes one exact candidate and the complete configured
contribution selected from it. A routine source proposal replaces that source's
lock, provenance, and all selected resources across every affected Pack as one
logical slice in the newly staged complete bundle. It carries all other source
locks and owned resources forward unchanged.

Partial source updates by Pack, binding, or resource are forbidden. They would
combine multiple upstream candidates under one source identity and make
continuity, removal authority, and snapshot verification ambiguous.

Each portable binding `(pack_id, kind, resource_id)` has exactly one Pack
Source owner. Duplicate cross-source bindings block even when their bytes are
identical. Deliberately shared host projections remain a separate
capability-pack composition rule with explicit contributors; projection
sharing does not create shared source provenance.

## Registration, removal, and transfer

Source registration is an explicit sealed operation that adds configuration,
the first exact lock, selected resources, affected Pack manifests and evidence
in one complete-bundle transaction. No committed generation may contain a
configured-but-unlocked source.

Source removal is likewise explicit and atomic. Only the source owner may
authorize removal of its contribution, and the operation must classify and
validate every affected Pack contract before committing a generation without
that configuration, lock, or owned resource.

Moving a binding between sources is an exceptional multi-source migration, not
two ordinary synchronizations. It seals both provenance chains and atomically
updates both configurations and locks, the selected resources, affected Pack
versions, and evidence. Equal bytes never imply or authorize a transfer.

## Provenance identities and freshness

Every Inspect, validation, and publication artifact carries two complementary
identities:

- `source_lock_sha256`: the target source lock's canonical digest; and
- `lock_set_sha256`: the digest of the canonical ordered sequence of
  `source_id` and source-lock digest pairs.

The sealed plan also retains its existing base, configuration, manifest, and
complete-bundle preconditions. A change to either provenance digest or any
complete-bundle precondition makes the proposal stale before mutation.

Consequently, merging a proposal for any source invalidates open proposals
based on the prior bundle generation, even when their target locks did not
change. They restart Inspect -> Classify -> Validate -> Publish. There is no
automatic rebase, evidence patch-forward, or unrelated-source freshness
exemption. Previous semantic evidence may inform a new human decision but
cannot establish authority for the new base or result tree.

Manual workflow concurrency and publication ownership remain per source:
`sync/<source-id>`, at most one owned open pull request per source, and
non-cancelling source serialization. Those workflow scopes do not weaken the
single complete-bundle transaction or its global freshness boundary.

## Transaction and recovery

Apply reacquires the target exact candidate outside the bundle lock. Under the
one shared lock it freshly verifies the sealed target lock, complete lock set,
configuration, classifications, and local authority; constructs and validates
one complete sibling-staged bundle; and performs the ADR 0007 two-rename swap.

One repository recovery marker continues to seal the plan, phase, canonical
paths, and old/new complete-tree hashes. It may record the operation's source
IDs and provenance digests for diagnosis, but recovery authority remains the
observed phase and complete old/new trees. There are no per-source markers,
partial rollbacks, or attempts to reconstruct one source from surviving bytes.

## Clean migration before Addy

The current singular `bundle/sources.lock.json` moves in a separate preparatory
implementation change to:

```text
bundle/sources/mattpocock-skills.lock.json
```

That cutover must prove the existing candidate, snapshot, resources, file
digests, and selected bytes are unchanged while producers, consumers,
validators, fixtures, and documentation adopt the new topology together. It
then deletes the singular file. There is no legacy reader, dual write,
fallback, or topology migration mixed into Addy's initial proposal.

Only after that migration is merged may Addy use an ordinary explicit source
registration followed by the no-bypass manual workflow. The topology and
artifact changes require a new complete immutable Pack Source schema suite;
the checked-in `v1.0.0` suite retains its existing meaning and bytes.

## Answer

Packy gives each Pack Source one canonical provenance lock and one complete
exact-candidate contribution while retaining a single complete-bundle
transaction and recovery owner. Committed configuration and locks are
bijective, portable bindings have exclusive source owners, and registration,
removal, or transfer is explicit and atomic. Target and complete-lock-set
digests provide source auditability and global freshness; any new bundle
generation invalidates older proposals. The existing singular lock migrates
through a separate clean cut before Addy registration, with a new immutable
schema suite and no compatibility fallback.
