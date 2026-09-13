---
status: accepted
---

# Publish an independent canonical Pack catalog

Adding and updating Packs currently requires separate Managed Pack releases,
promotion into Packy, and delivery tied to a Packy release. For a catalog
maintained by the same maintainer, these repeated publication steps impede
ordinary content maintenance. The following decisions were confirmed during
design review and accepted after final shared-understanding confirmation.

## Confirmed decisions

The Catalog Project is the canonical authoring location for the manifests and
resources of all seven current Packs. Independent upstream products, including
Engram, keep their own repositories. Consolidating Pack authorship does not
authorize archiving or deleting those repositories.

Every Catalog Publication contains one complete immutable Catalog Snapshot.
Packs retain independent versions and activation choices. Downloading a new
snapshot does not update existing activations. Catalog publication is separate
from the Packy executable release lifecycle; content using existing engine
capabilities needs no new Packy release once independent consumption is
implemented.

Review and merge of a content pull request, with required validation passing,
authorize automatic publication of its resulting Catalog Snapshot. There is no
second manual publication confirmation or promotion pull request into Packy.
One content pull request may add one Pack and update another while preserving
the versions of unchanged Packs.

The initiative includes tools to create a Pack from a template, import selected
resources from an exact upstream commit, and refresh exact copies through a
reviewable diff. Maintained adaptations require explicit reconciliation rather
than automatic overwrite. Publication decoupling alone does not complete the
maintainer experience.

The first implementation consumes only the official public Catalog Project,
`yersonargotev/packy-catalog`. Initialization downloads the initial snapshot;
subsequent catalog refreshes require an explicit action. Ordinary catalog
listing, status inspection, and activation from downloaded content do not
discover newer publications over the network. Custom catalogs are outside
this initiative.

A new snapshot requiring vocabulary or capabilities the installed Packy does
not support is rejected as a whole, including when the incompatible capability
belongs to an unselected Pack. The previously selected snapshot remains usable,
and the failure explains the required engine update. Partial catalog loading
is not supported.

Upstream Refresh prepares and validates its proposed changes before writing.
Unexpected local modifications to a resource declared as an exact copy stop
the requested operation. For adapted resources, the tool presents differences
between upstream revisions and requires explicit reconciliation instead of
overwriting the maintained adaptation. Authoring operations do not publish
their changes automatically.

Adoption is an explicit clean cut, without an automatic converter for earlier
installed state. The previous Packy version is used to deactivate or uninstall
the installations being transferred before they are installed with the new
model. Old sources remain available while links may still reference them;
personal files, credentials, and Memory remain preserved. The adoption
instructions must be verifiable and do not include automatic deletion of old
source directories.

Downloaded Catalog Snapshots are retained without automatic cleanup in the
first delivery. Activations continue referencing their immutable source
snapshot until their own explicit lifecycle update. Refreshing catalog
availability never rewrites the content behind those active references.

Packy provides CLI authoring operations within the Catalog Project for creation,
import, Upstream Refresh, and validation. The TUI exposes catalog inspection and
refresh alongside the existing activation lifecycle; a TUI Pack editor is
outside scope. Maintainers explicitly select resources and their destinations,
supported hosts, and adaptations. Templates and standard metadata detection
reduce repetitive input without interpreting or converting arbitrary
repositories heuristically. Imports preserve provenance and notices, and
validation explains missing required information before admission.

Packs use independent semantic versions. Changed Pack content requires a version
bump for that Pack; unchanged Packs retain their versions. Catalog identity is
generated from its commit and digest, together with a generated index.
Consumption verifies the official publisher and artifact integrity. Published
bytes cannot be replaced, and retrying the same publication is idempotent rather
than creating duplicate publications.

Removing a Pack from the current catalog prevents new activations from that
catalog while leaving existing installations inspectable and deactivatable.
A defective published Pack is corrected by a newer Pack version in a new Catalog
Publication. Downgrades and automatic uninstall are outside scope.

The main acceptance seam is the real installed CLI, following existing
installed-source, catalog extensibility, offline, and lifecycle test patterns.
The end-to-end demonstration adds one Pack and updates another through one
content pull request, then uses the same Packy executable to acquire that
publication. Catalog Refresh preserves active state; a separate explicit
lifecycle operation updates only the selected Pack, and the new Pack can be
activated. Acceptance also covers offline consumption, failures without partial
changes, and equivalent TUI behavior. Lower-level tests target faults that are
difficult to reproduce through this observable seam.

## Consequences

This decision supersedes ADR 0031's fixed four-Pack catalog and its coupling of
Pack authoring to Packy releases. It also supersedes ADR 0038's
one-repository-per-Pack authoring, registry, and promotion model. It preserves
ADR 0031's installed-receipt, ownership, and protected-integration contracts,
along with the reviewed resource contract, provenance and notices, Pack
ownership, and the capability/readiness principles of ADR 0035. This is the
accepted target architecture; implementation and repository integration remain
separate delivery work.

## Final confirmation

All fourteen design-review recommendations and the final shared understanding
were confirmed on 2026-09-13. The earlier
[architecture proposal](../research/evidence/packy-catalog-architecture-proposal-2026-09-13.md)
is supporting evidence; details not captured in the confirmed decisions are not
implicitly accepted.
