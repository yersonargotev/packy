# ADR 0016: Register composite Pack Sources atomically

## Status

Accepted.

## Context

One capability Pack can require resources owned by several Pack Sources before
any valid observable contract exists. Registering those sources sequentially
would expose invalid intermediate bundle generations and allow classification,
validation, or publication evidence from different candidate sets to be mixed.

[ADR 0007](0007-serialize-complete-bundle-transactions.md) already owns atomic
complete-bundle replacement and recovery.
[ADR 0008](0008-orchestrate-classification-outside-packsync.md) classifies the
observable contract per affected Pack.
[ADR 0009](0009-own-manual-synchronization-orchestration.md) owns the private
manual workflow.
[ADR 0011](0011-publish-versioned-pack-source-schema-suite.md) requires complete
immutable schema suites.
[ADR 0012](0012-adopt-source-scoped-pack-source-provenance.md) assigns exclusive
resource ownership to individual sources while preserving global freshness.
This decision adds the missing all-or-nothing admission contract without
moving any of those responsibilities.

## Decision

### Domain and transaction

A **Composite Pack Source Bundle** is the complete ordered set of two or more
Pack Sources admitted together as the initial provenance of exactly one
previously absent capability Pack. Every member contributes resources
exclusively to that Pack. Single-source registration retains its existing
contract; multi-Pack composite registration is not part of this decision.

The composite is a sealed transactional value, not a persistent entity.
During admission, `registration_bundle_sha256` identifies the canonical
registrations ordered by `source_id`. After commit, the canonical state remains
the individual source configurations, their exclusive source locks, and the
derived complete `lock_set_sha256`; Packy persists no composite index or record.

`internal/packsync` exposes one complete-set Check seam and one complete-set
Apply seam. Callers cannot Check or Apply composite members independently.
Check rejects fewer than two members, duplicate or unsafe source IDs, an
existing target Pack or member source, bindings outside the declared Pack,
ownership conflicts, and any invalid complete resulting configuration.

Every member carries one complete source registration with an exact full commit
selector. There is no shared or duplicated top-level selector. Candidate
acquisition may use independent disposable roots, but any acquisition or
validation failure blocks the complete operation, discards all temporary
results, writes nothing, and requires a fresh Inspect. Initial composite
registration has no no-op or partial-convergence state.

All resources are materialized into one sibling-staged bundle before Pack-level
dependency closure and validation. Individual sources retain exclusive
provenance ownership even when a resource owned by one source depends on a
resource owned by another. Canonical source order establishes identity and
hashing, not a source registration order.

Apply reuses ADR 0007's complete-bundle lock, staging, durable marker, two
renames, validation, and old/new tree-hash recovery authority. The marker may
record the operation, ordered source IDs, and composite seals for diagnosis,
but Recover never completes or rolls back individual sources.

### Sealing, classification, and legal admission

The operation uses two seals:

1. `registration_bundle_sha256` seals the canonical ordered registration
   intent.
2. The canonical plan identity seals the repository base, every exact
   candidate, resulting configuration, proposed source locks, complete
   `lock_set_sha256`, affected Pack, proposed version, legal admission evidence,
   and all other complete-bundle preconditions.

Changing any sealed member or bundle fact invalidates the complete plan and
requires a fresh Inspect. Classification remains one evidence document per
affected Pack and binds to the composite plan. No source-level classification,
subplan, diagnostic, or successful acquisition carries independent authority.

Each source must provide a durable reference and digest for exact legal
admission evidence with an explicit redistributable disposition. Public
provenance, partial licensing, or a later licensing commit never grants
retroactive authority. Any missing or changed legal fact blocks the whole
operation.

Inspect may aggregate safe blockers deterministically by `source_id` and code,
but emits only one global blocked state. Diagnostics contain no credentials,
secrets, or retained upstream bytes and prescribe correcting the bundle and
repeating Inspect rather than resuming a partial result.

### Workflow and schema contract

Composite admission is the explicit private operation `register_bundle` in a
new complete immutable Pack Source workflow schema suite `v3.0.0`. Existing
v1/v2 `synchronize` and single-source `register` contracts remain unchanged.
A v3 dispatch cannot consume or emit v1/v2 artifacts.

Every v3 inspection, failure, classification, validation, and publication
artifact carries:

- the declared `pack_id`, ordered `source_ids`, and
  `registration_bundle_sha256`;
- an ordered member set containing each `source_id`, exact candidate identity,
  source-lock digest, and sealed legal-admission evidence;
- the plan, repository base, resulting configuration, manifests, complete lock
  set, and result-tree identities appropriate to its phase; and
- the existing attestations that artifacts contain neither secrets nor
  upstream bytes.

The private `syncpacksource` adapter and manual
**Inspect → Classify → Validate → Publish** workflow remain the only admission
surface. No public `packy` command is added. Validate and Publish independently
reacquire every exact candidate in disposable roots and must reproduce the
same complete result.

The declared `pack_id` owns workflow concurrency, branch, and pull-request
identity. Publication uses `sync/<pack-id>` and at most one open proposal for
that Pack. Another workflow may proceed independently, but any changed
repository base or bundle generation makes the composite plan stale; Packy
never patches evidence forward or merges competing state automatically.

## Consequences

- A first multi-source Pack generation is either wholly absent or wholly
  present in every observable and recoverable bundle state.
- Existing per-source provenance, single-source synchronization, Pack-level
  classification, and complete-tree recovery remain authoritative.
- Composite admission does not create a second durable aggregate that can
  drift from source configuration and locks.
- Later one-source updates remain valid only through their existing full
  lock-set freshness and complete-Pack revalidation contract.
- Composite removal, ownership transfer, later multi-source synchronization,
  and multi-Pack admission require separate decisions.
- This decision does not authorize implementation, synchronization,
  activation, publication, or release of any capability Pack.

## Enforcement

Implementation must prove:

- successful admission produces configuration, resources, manifests, and every
  source lock in one complete new tree;
- every member or cross-source validation failure leaves the old tree
  byte-for-byte unchanged;
- fault injection before, between, and after the two renames lets Recover
  produce only the complete old or complete new tree;
- changed base, candidate, registration, legal evidence, source lock, lock set,
  plan, or workflow artifact fails stale;
- mixed schema versions and any already-present member fail before writes;
- Validate and Publish independently reacquire all members and reproduce the
  same complete result; and
- existing v1/v2 single-source registration and synchronization remain green.
