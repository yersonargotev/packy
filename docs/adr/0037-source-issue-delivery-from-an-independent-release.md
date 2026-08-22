---
status: superseded by ADR 0038
---

# Source issue delivery from an independent release

## Context

Issue delivery has a reusable core, but every consuming repository must retain
authority over its approval, validation, review, merge, and cleanup policy.
Keeping reusable content only inside Packy would make Packy's local policy look
canonical and would prevent other repositories from reviewing a neutral source.
Copying mutable upstream branches would make an admitted Pack generation
impossible to reproduce exactly.

## Decision

Stable releases of `yersonargotev/issue-deliver-pack` are the canonical authority
for reusable issue-delivery content. Packy admits only immutable stable releases
and binds each reviewed candidate to its exact commit, content scope, README
identity, license evidence, and digest.

The independent source owns the generic skills. Packy owns its repository
policy and every admission artifact: source admission configuration, reviewed snapshot,
legal notice, Pack manifest, catalog entry, and immutable history.
An upstream release does not register a Pack or grant merge authority in Packy.

When a source reconfiguration selects a newer immutable release, it may align
the Pack manifest's informational `source_reference.revision` only to the exact
release tag resolved for that sealed candidate. The source repository remains
protected metadata, an arbitrary or stale revision is rejected, and the
revision transition alone does not raise the Pack's compatibility floor.

## Consequences

The generic workflow can evolve and be reviewed independently without
embedding Packy-specific rules. Any release, candidate, selected content,
license, README identity, or scope change requires fresh viability, legal, and
admission review; Packy never follows a moving branch or silently carries an
earlier decision forward.
