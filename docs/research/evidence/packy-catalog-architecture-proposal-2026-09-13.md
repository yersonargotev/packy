# Independent Packy catalog architecture

## Recommendation and status

Use one canonical content repository, provisionally named `packy-catalog`, to
author all Packs maintained as part of this catalog. Publish the complete
validated content as an immutable snapshot. Packy downloads snapshots and
continues to own selection, preview, activation, update, receipts, and readiness.

This is a proposal for discussion, not an accepted ADR or implementation. It
refines the [distribution options](packy-catalog-distribution-options-2026-09-13.md)
against Packy revision `40a4e93661ed2a94582d35a6d0936177c7b81ded`.
It would replace [ADR 0038](../../adr/0038-promote-releases-from-managed-pack-projects.md)'s
one-repository-per-Pack admission and promotion model while preserving the
typed capability and readiness principles of
[ADR 0035](../../adr/0035-make-pack-readiness-capability-driven.md).

**One content PR in `packy-catalog` may add one Pack and update another.** After
review and merge, it produces one catalog release, without a Packy executable
release, provided both Packs use capabilities the installed engine supports.
An initial engine implementation and release are necessary to enable this flow.

## Repository ownership

| Repository | Owns | Changes when |
| --- | --- | --- |
| `packy` | CLI/TUI, capability domain, surface adapters, schemas and validator, catalog acquisition and local snapshot storage | Behavior, supported schema, or host integration changes |
| `packy-catalog` | Canonical Pack manifests, resources, notices, pinned external origins, content review and catalog publication | A Pack is added, updated, or removed from current availability |
| Independent upstream projects | Their original products and content | Their maintainers release or revise those projects |

The catalog does not move Engram's program source or other independent products
into Packy. It owns the reviewed content and adaptations Packy distributes.
Dedicated intermediate Pack-authoring repositories stop being canonical for
these Packs after the new model is adopted. They are not retained as mandatory
publishers underneath a new aggregation layer.

### Recommended initial tree

Preserve the existing bundle-relative paths to keep repository consolidation
separate from a directory redesign:

```text
packy-catalog/
  README.md
  bundle/
    packs/
      argote/pack.json
      matty/pack.json
      example/pack.json
    skills/
    instructions/
    agents/
    commands/
    assets/
    notices/
  .github/workflows/
    validate.yml
    publish.yml
```

Each Pack manifest names its exact resources in the bundle. Namespacing and
exclusive resource ownership must remain valid across the complete catalog.
An unreferenced file does not become a Pack resource merely by being present.
Release content is the deterministic union of admitted manifest closures;
repository workflows and authoring documentation are excluded.

Keep descriptions, Pack SemVer, surfaces, resource dependencies, bindings,
origins, and notices in the Pack manifest. Reuse the existing vocabulary and
validation logic. The loader must be revised to admit multiple canonical Pack
manifests without the current one-project/one-Pack registry contract. This is
not achievable by moving files alone.

Do not add a manually maintained registry listing the same Pack IDs and paths.
Discover manifests in the conventional `bundle/packs/*/pack.json` location and
generate the release index. The initial product has one official catalog;
arbitrary registries and plugin-defined acquisition backends are out of scope.

## Version and artifact contract

Keep three identities distinct:

| Identity | Meaning | Assignment |
| --- | --- | --- |
| Pack version | Reviewed revision of one Pack's content and contract | Maintainer changes SemVer when that Pack changes |
| Catalog snapshot | Exact combination of Packs published together | Generated from the reviewed source commit and artifact digest |
| Packy version | Engine implementation | Ordinary executable release |

An illustrative immutable release tag is `catalog-<full-source-commit>`. The
tag is a locator, not the only identity check. The generated index records the
full source identity, format version, builder identity, and each Pack's ID,
version, manifest digest, closure digest, and ordered path/mode/content index.
It describes included content; it grants no additional runtime permissions.

Publish one archive containing the generated index and runtime bundle, plus a
checksum manifest for the archive. Do not place the archive's own checksum
inside the archive. Packs keep their versions when their closures and contract
have not changed; adding a Pack does not bump every existing Pack.

The publication validator requires a version change for changed Pack content
and rejects reuse of a previously published Pack ID/version for different
content. Published releases provide the historical evidence; there should not
be a second manually authored admission ledger in the engine repository.

Only the configured official repository is a publication authority in the
initial design. Acquisition verifies that source, immutable release identity,
and expected artifact digest. A checksum downloaded from that publisher is an
integrity check within this trust model, not independent proof of harmless
content or protection against a compromised publisher. Publication policy and
credentials remain separate from processing Pack content.

## Review and publication

1. A content PR changes the canonical manifests and resource bytes. It may
   contain one coherent multi-Pack change, including a new Pack and an update.
2. Read-only CI uses a pinned version of the Packy-owned validator. It checks
   manifests, origins and notices, resource closures, version invariants,
   collisions, and production projection fitness. It reads Pack content as
   inert data and does not execute included scripts, hooks, or builds.
3. Maintainer review accepts the exact diff. Merge is the content acceptance
   event; publication is a mechanical continuation under the catalog's agreed
   release policy. That policy is proposed here, not authorization to publish
   anything in the current session.
4. The release workflow packages the accepted commit, verifies the generated
   artifact using the same contract, and publishes one immutable catalog
   release. The write-authorized publication job receives the checked artifact,
   not authority to execute arbitrary catalog content.
5. An unchanged runtime bundle need not produce another release. A failed run
   may resume for the same exact candidate; an existing matching release is
   reused, and conflicting published bytes cause rejection. Publication must
   serialize or reject stale candidates so an older run cannot advance the
   default release after a newer one.

This is one content PR and one catalog publication. It does not introduce a
second promotion PR into `packy`, a release per changed Pack, or a Homebrew
formula update for content-only changes. GitHub review and CI still have work
to do; the saving comes from removing repeated publication and admission hops.

## Packy's runtime modules

The catalog repository contains data and release automation. It is not a
runtime service and does not run in the background on the developer's machine.

| Module | Small interface | Implementation responsibility |
| --- | --- | --- |
| Catalog distribution | Refresh the official catalog; open a verified local snapshot | Source resolution, download, bounded extraction, identity/integrity checks, compatibility validation, publication of local snapshot selection |
| Existing capability-pack domain | Inspect, preview, apply, verify | Resource selection, constraints, ownership, readiness, receipts, and update semantics |
| Existing surface adapters | Project and observe declared capabilities | Host-native paths, configuration, and observations |
| CLI/TUI | Invoke operations and present results | User interaction and explicit application of previews |

The distribution module returns an immutable verified snapshot descriptor.
The capability domain does not learn HTTP, release discovery, or download
layout. Schema and Pack semantics remain owned by the domain and its validator;
distribution calls that validator rather than copying its rules. The CLI and
TUI do not implement either kind of policy.

A repository filesystem input is sufficient for authoring validation. The
runtime source is the verified snapshot. Keep development-source selection
explicit; an unrelated ancestor directory must not silently become an
alternative trusted publisher. Reuse current acquisition dependencies when
appropriate instead of introducing mandatory mise integration or a general
package-manager abstraction.

### Local snapshot lifecycle

An illustrative layout is:

```text
<Packy data directory>/catalog/
  current.json
  snapshots/
    <artifact-digest-A>/
      catalog-index.json
      bundle/...
    <artifact-digest-B>/
      catalog-index.json
      bundle/...
```

`current.json` names the verified snapshot available for future selections and
updates. Existing activations are not symlinked through `current`. They reference
their immutable snapshot, so refreshing A to B cannot change active skill bytes.

Refresh stages a download in a new directory, checks all archive paths and
file types before admitting it, verifies content and compatibility, then
atomically selects the completed snapshot. A failure leaves the previously
selected snapshot usable. Validation must use the exact staged bytes subsequently
installed, and preview/apply must retain a fixed snapshot identity even if
another refresh happens concurrently.

Existing receipts need a portable source-snapshot identity where needed to
bind installed content and retain its source. Personal state remains local;
project locks contain no absolute cache paths, credentials, or runtime evidence.
Project verification continues to use committed projections offline. Neither
catalog refresh nor snapshot selection is a command to rewrite project locks.

Retain snapshots used by the current selection, active receipts, links, or an
in-progress preview/apply. Defer automatic garbage collection in the first
slice; correctness is simpler than proving that every historical source is no
longer referenced. Preserving a snapshot is not a promise of general rollback
for host configuration or external effects.

### Compatibility

The engine checks the supported catalog format, manifest vocabulary, and
required capabilities before selecting a downloaded snapshot. Unknown required
behavior rejects the candidate and explains that an engine update is needed;
the current working snapshot remains selected. The initial design validates the
whole snapshot rather than silently accepting only part of an incompatible
catalog.

The catalog validator is pinned to a declared supported engine baseline. Its
version may change deliberately when the catalog starts using a new capability.
Checking that the engine implements a surface adapter does not require that
every user have all three host executables installed. Host availability and
personal authorization retain their existing readiness meanings.

## Content maintenance

Adding original content means adding a manifest and its resources in one PR.
Adding unchanged upstream content means selecting an exact upstream commit and
resource path, importing the bytes, and recording origin and notice facts in
that same PR. No separate Pack wrapper repository or publisher release is
required just to identify upstream bytes.

For later upstream refreshes, a small authoring helper may replace the exact
declared copied resource, including removing obsolete files, and present the
resulting diff. It must refuse silent overwrite of adaptations. Adapted content
requires deliberate reconciliation; the helper cannot decide whether a
customization still makes sense. This helper is subordinate to the same
manifest and PR flow, not a second source inventory or another product.

## Concrete single-PR example

All names and versions below are illustrative:

```text
Repository: packy-catalog
PR: Add team-review and update matty

Added:
  bundle/packs/team-review/pack.json       version 1.0.0
  bundle/skills/team-review/SKILL.md
  required supporting resources/notices

Changed:
  bundle/packs/matty/pack.json             1.1.0 -> 1.1.1
  selected Matty resources and origin pins

Output after accepted merge:
  one catalog snapshot containing both changes
  unchanged Pack versions for all other Packs
  no change to the Packy executable
```

After download, `team-review` becomes available for explicit activation. Matty
remains at its installed version until its own previewed update is applied.
Other installed Packs keep their content and source references unchanged.

Suggested user-facing command names, not current functionality:

```sh
packy catalog refresh
packy list
packy update matty --surface codex --dry-run
packy update matty --surface codex
packy activate team-review --surface codex
```

`catalog refresh` obtains availability; lifecycle commands apply selected
changes. The TUI should express the same distinction. Refresh may return “no
change,” an available verified snapshot, or a typed failure; it never installs
all Packs simply because it downloaded the complete bundle.

## First delivery and acceptance

The first engine delivery must replace release-coupled Installed Source lookup
with independent catalog consumption. Merely changing a clone URL or bypassing
the current identity check is not the new architecture. Current implementation
is documented in [release guidance](../../release.md),
[source initialization](../../../internal/cli/root.go), and
[catalog loading](../../../internal/cli/pack.go).

Adopt the decision, implement the engine support, establish the canonical
catalog, and retire the superseded promotion path as a clean cut. An adoption
plan must explicitly account for existing activations and links before old
source trees can be removed; this proposal does not authorize deleting them
or adding permanent compatibility loaders.

Acceptance should demonstrate a single catalog PR adding a Pack and updating
another against the same already-released engine. Verify that refresh leaves
active bytes unchanged, updating one Pack preserves the others, offline
activation uses the downloaded snapshot, and corrupt/incompatible/interrupted
downloads preserve the previous selection. Existing project verification,
drift/collision behavior, and personal-state separation must remain intact.
Run fixtures with sandboxed `HOME` and `XDG_CONFIG_HOME` and the repository's
explicit test-process environments.

After that slice works, prioritize the upstream-refresh helper using a real
frequent update. Broad registries, per-user catalogs, automatic synchronization,
cross-Pack dependency resolution, and full historical environment restoration
are not prerequisites for resolving the stated maintenance problem.
