---
status: accepted
---

# Adopt a Pack-local catalog layout

Keep `packy` and the canonical Catalog Project in separate repositories so the
catalog retains its independent ownership, review, and publication lifecycle.
Replace the catalog's global `bundle/` tree with `packs/<pack-id>/...`, placing
each Pack manifest and its reviewed resources together. Use the Pack-local
layout end to end in catalog authoring, Catalog Snapshots, and Packy consumption
rather than permanently normalizing publications back into the former global
resource tree. This improves locality and makes Pack ownership visible while
preserving independent Pack versions and Catalog Publications.

Resolve every file-backed resource source relative to the directory containing
its Pack manifest. Packy owns the format implementation: parsing, validation,
snapshot construction and consumption, and surface adapters. The Catalog
Project remains inert reviewed data and runs a pinned Packy version for
authoring and publication checks rather than maintaining a second validator.

Adopt the new format as a clean cut. Existing installations are deactivated
with the previous Packy version before upgrading and reinstalling from the new
catalog. Packy does not retain a legacy reader, converter, or dual-format
compatibility path because there is only one current installation to coordinate.

Introduce manifest schema v2 and Catalog Snapshot index schema v2 so the new
Pack-relative source semantics are explicit rather than silently reinterpreting
schema v1. A Pack directory contains no undeclared regular files: each file is
the manifest or belongs to one declared file-backed source root. Contract-only
resources without a source remain valid.

Keep Pack resources host-independent. Surface bindings remain the adapters;
surface-specific directories are used only when source content genuinely
differs, and empty `hooks/`, `mcp/`, or other speculative directory conventions
are not part of the Pack scaffold.

Catalog authoring prepares and atomically applies only the affected Pack
directory, while validating the complete candidate catalog before mutation.
This replaces whole-catalog tree copying without weakening catalog-wide
identity, version, compatibility, provenance, or publication checks.

A v2 Catalog Snapshot contains `catalog-index.json` and `packs/` directly; it
does not retain an outer `bundle/` directory. Remove Packy's checked-in catalog
copy rather than migrating it. Packy tests use focused fixtures, while the
canonical Catalog Project validates real-catalog integration with one pinned,
immutable Packy commit shared by pull-request validation and snapshot
construction.

Roll out engine-first. Release the Packy version that implements schema v2,
then migrate and publish the canonical catalog with that pinned version. The
sole existing installation is deactivated with the previous Packy version
before upgrading, refreshing to the v2 snapshot, and reinstalling. Older Packy
versions reject the incompatible complete snapshot and retain their selected v1
snapshot until that clean cut is performed.

The migration advances every current Pack in one catalog change: Addy to
3.0.0, Argote to 2.0.0, Engram to 4.0.0, and HumanLayer, Issue Delivery,
Matty, Orchestrate, pstack, and Web to 2.0.0. This makes the source-resolution
break visible in each Pack contract instead of treating it as catalog-only
metadata.

Pack-local authoring validates a composed view: unchanged Packs are read from
the worktree and the target Pack is read from its isolated stage. Immediately
before applying, Packy rechecks the identity of the complete `packs/` tree and
atomically creates or exchanges only `packs/<pack-id>`. A concurrent change to
any Pack aborts the operation.

Acceptance evidence covers deterministic v2 snapshots, traversal and symlink
rejection, undeclared-file rejection, schema v1 rejection, isolated Pack
authoring, clean snapshot acquisition, lifecycle drift and collisions, and
independent activation of Matty's and pstack's distinct `tdd` resources.
