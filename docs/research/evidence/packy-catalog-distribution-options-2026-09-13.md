# Packy catalog distribution options

Date: 2026-09-13

Status: research and proposal, not an accepted architectural decision.

## Question and recommendation

The maintainer's concrete problem is the work required to add or update a
Pack. The existing bundle remains useful for selecting and activating reviewed
content. The decision should therefore optimize the maintainer's content flow,
without assuming that the bundle and its binary must share a release lifecycle.

**Proposed direction for this same-maintainer catalog:** keep Packy's
multi-resource activation behavior, place
the curated catalog's canonical content in one separate repository, and publish
one immutable catalog snapshot independently of the executable. Use ordinary
content review and validation there. Do not recreate the present seven-project
release-and-promotion arrangement inside the new repository.

This is a design inference from the current contract and the stated maintenance
pain, not evidence of external product demand or an implementation already
available in Packy. It explicitly changes ADR 0038's public one-repository,
one-Pack authoring decision, one-to-one registry, and per-Pack promotion flow.
An accepted superseding ADR is required before implementing that architecture.
[ADR 0038](../../adr/0038-promote-releases-from-managed-pack-projects.md)

## Current cost that the proposal removes

Packy currently requires a public Managed Pack Project for each Pack, a root
manifest, immutable external-origin declarations where applicable, preventive
validation, an immutable `pack-v<version>` release, and independent Packy
promotion. Promotion validates the candidate again and proposes its admission
to the bundled catalog. The bundle is generated distribution content, not a
second authoring location.
[Managed Pack Projects](https://github.com/yersonargotev/packy/blob/b54df8353421c07878df21a9766b856d935165f6/docs/managed-pack-projects.md),
[Capability Packs](../../capability-packs.md)

The initial registry has seven separate project repositories. This creates
separate repository, release, and admission work even when one maintainer owns
the catalog. Pack versions are independent, but the installed source catalog is
pinned to the released Packy version, and `update` targets its bundled Pack
version. The bundle is not literally embedded in the Go executable: bootstrap
clones Packy's repository, with the released binary version selecting its
default source ref. Installed-source validation enforces that identity. Content
availability is therefore release-coupled despite separate version numbers.
[ADR 0038](../../adr/0038-promote-releases-from-managed-pack-projects.md),
[README](../../../README.md),
[default source ref](../../../internal/cli/root.go),
[installed-source validation](../../../internal/cli/pack.go)

Consolidation removes repeated release boundaries. It does not remove the
substantive work of deciding which content to include, adapting it for each
surface, reviewing upstream changes, retaining notices, or testing projections.
If adaptation dominates the time spent, moving repositories alone offers only
partial relief.

## What existing tools actually replace

| Option | Confirmed capability | Replacement boundary for this maintainer |
| --- | --- | --- |
| mise HTTP backend | Fetch a binary, script, or archive from a URL with a version label and checksum; configure extraction. | Can distribute a catalog archive. It does not define Packy's surface projection or Pack ownership contract. |
| mise Packslip backend | Verify signed releases, install tools, resolve versions, and carry artifact and signer commitments in `mise.lock`. | Can own executable acquisition and release verification when publishers supply the manifest. Publishing the manifest remains publisher work. |
| mise Packslip skills | Fetch declared skills with the installed tool version and synchronize links into an agent skill directory. | Can replace tool-attached skill installation when that is the entire requirement. |
| Vercel `skills` CLI | Install selected skills from repository URLs or local paths for selected agents; copy or symlink; update and remove skills. | A direct alternative for largely unchanged skill collections. |
| Packy today | Select resource dependency closure; adapt to Codex, OpenCode, and Claude Code; keep owned-path receipts; check drift and collisions; manage project intent and verification. | Retains a concrete role for curated, adapted, multi-resource Packs. |

Sources for table capabilities:
[mise HTTP](https://mise.jdx.dev/dev-tools/backends/http.html),
[mise Packslip](https://mise.jdx.dev/dev-tools/backends/packslip.html),
[mise skills](https://mise.jdx.dev/dev-tools/packslip-resources.html),
[skills CLI source repository](https://github.com/vercel-labs/skills),
[Packy capability contract](../../capability-packs.md).
Replacement boundaries are inferences; the documentation does not promise
equivalence to Packy's contract.

It would be inaccurate to describe mise as only a binary downloader. Its
skills can come from the tool archive, a separate signed asset, or the source
repository at the release commit. Sync follows the active tool version,
preserves unrelated entries, and can prune its own stale links. Automatic sync
is optional and runs after install/use in a mise project; it does not itself
reload an agent. Generated skills require opt-in execution, whereas fetched
skills do not.
[mise Packslip resources](https://mise.jdx.dev/dev-tools/packslip-resources.html)

Packslip is also broader than a skills format: it can describe additional
release resources, but each consumer decides what it installs. Describing a
resource is not evidence that mise implements Packy's instruction contribution,
configuration mutation, dependency closure, or project-receipt semantics.
[Packslip documentation](https://packslip.dev/docs/),
[Packslip specification](https://packslip.dev/release/v1/)

The `skills` CLI already supports multi-skill repositories, selecting agents
and skills, local project/global scope, and direct download archives. A content
monorepo does not require building a new installer merely to copy skills.
[skills CLI](https://github.com/vercel-labs/skills)
Skills.sh additionally offers shareable skill collections assembled from
repositories and uploaded files, with installs resolving their current content.
That makes grouping skills alone a weak reason for Packy; it is not the same
contract as a reviewed immutable multi-resource catalog.
[Skills.sh packs](https://www.skills.sh/docs/packs)

## Minimal canonical content repository

The proposed repository would contain the actual reviewed manifests and
resource bytes for all curated Packs, for example `packs/<id>/pack.json` and
that Pack's resource directories. A release process would materialize the
runtime bundle from these paths. Generated archives are distribution artifacts,
not another place to edit content.

Separate three concepts:

1. **Authoring repository:** where a maintainer changes a Pack once.
2. **Catalog snapshot:** an immutable artifact containing the complete catalog,
   identified by release/commit and digest, with the constituent Pack versions.
3. **Installed receipt:** the existing record of the Pack version, selected
   resources, surface, owned paths, and digests actually installed.

One catalog release can change one Pack while retaining every other Pack
version. This preserves individual Pack identities and activation choices;
one repository does not mean one giant selectable Pack. Keep per-Pack SemVer
because existing lifecycle and receipt semantics use it, and give the catalog
its own simple release identity rather than introducing a dependency resolver.

The complete immutable snapshot preserves an offline, internally coherent
source for activation. Fetch each snapshot into a new immutable directory and
validate it before switching the current catalog reference. Current surface
adapters create skill symlinks, so replacing bytes in a shared mutable source
directory could change active content without an explicit update. Keep active
links pinned to their snapshot until normal previewed activation/update, and
retain every snapshot referenced by installed receipts or links. Downloading
the whole catalog must not apply every Pack.
[Codex projection](../../../internal/codex/surface.go),
[OpenCode projection](../../../internal/opencode/surface.go),
[Claude projection](../../../internal/claudecode/surface.go)

A failed download or incompatible catalog must leave the previous usable
snapshot selected; that is a fetch invariant, not a promise of rollback for
every lifecycle operation. Check the catalog's required schema and capabilities
against the engine before changing state. New content using supported behavior
should not require an executable release; new engine capabilities still do.

### Content ownership and upstream updates

For original Packy-specific content or its maintained adaptations, edit only
the canonical content repository. Independent upstream products such as Engram
keep their own source repositories and releases; consolidating Pack definitions
does not mean relocating those products.
An external origin is provenance, not a second authoring location. For an
unchanged upstream skill, an optional import operation can acquire the exact
declared commit/path, replace its canonical copied resource, and present the
byte diff for review, including removal of stale resource files. Retain notices
and prove whole-resource `exact-copy` when
that relationship is claimed. Adapted resources require reviewing and applying
upstream changes intentionally; a synchronization command cannot decide which
customizations remain correct.

“Optional origins” should mean no artificial upstream for original work. Under
the present contract, derived resources require origin attribution and notices;
the proposal should preserve that useful provenance rather than silently
weakening it. A catalog does not need a bespoke immutable publisher release for
each exact-copy upstream when an immutable source commit supplies the input.
[Historical origin contract](https://github.com/yersonargotev/packy/blob/b54df8353421c07878df21a9766b856d935165f6/docs/managed-pack-projects.md)

### Maintainer flow after adoption

Adding a Pack creates its canonical directory, manifest, and reviewed resources.
Refreshing from upstream changes those authoring inputs through an explicit
reviewed diff. Refreshing the delivered catalog only downloads/selects a
published snapshot locally. Updating an installed Pack is a fourth, separate
operation that changes its projections and receipt. Keeping these operations
distinct prevents a convenient refresh command from silently overwriting
adaptations or active resources.

1. Edit one Pack or import a pinned upstream revision in the content repository.
2. Review the content diff and adjust that Pack's version and manifest.
3. Run the existing useful manifest, closure, notices, and production projection
   checks through a validator against the canonical tree.
4. Merge the content PR and publish the immutable complete catalog snapshot.
5. Refresh the local catalog, then preview/apply the affected Pack update.

Steps 4 and 5 require implementation; these are proposed operations, not
existing CLI commands. The normal content PR is the review boundary. A second
promotion PR into Packy's source repository, per-Pack publisher releases, or
append-only admission records duplicated across repositories would largely
reintroduce the maintenance burden this proposal is meant to remove.

## Constraints and smallest useful next step

The current tracked bundle contains 224 files and 1,136,131 bytes, about 1.1 MB;
its present size gives no reason to reject whole-catalog snapshots. This is a
local inventory observation, not a projection of future growth.

The content repository adds one repository and an independent release artifact;
it should replace the multiple authoring repositories as catalog authorities,
not sit above them as another orchestration product. Its whole-catalog releases
also mean that publication validation covers a coherent snapshot and that
content errors can block a catalog release. Keep checks deterministic and
report the affected Pack clearly.

Repository count and release coupling are independent decisions. Keeping code
and catalog in one repository with separate content releases is also viable;
it avoids an immediate repository move but needs the same source-identity and
snapshot changes. A separate content repository is recommended here for its
distinct ownership and review cadence, not because Git requires it. Preserve
the existing runtime bundle layout initially where it fits; a new directory
taxonomy is not the source of the intended maintenance saving.

Transport is a separate choice. A checked archive over HTTPS is sufficient in
principle; mise can deliver archives and Packslip can standardize signed release
metadata. Neither requires moving Packy's activation semantics into shell tasks.
Start with one supported delivery method and reuse an existing reliable
acquisition capability where it reduces the total implementation. Adding a
mandatory mise dependency solely for the catalog needs a concrete complexity
benefit, not just overlap in the word “package.”

The smallest implementation slice, after an accepted decision, is one canonical
catalog repository plus one reproducible snapshot that the production Pack
loader can consume and activate. Demonstrate one new Pack and one existing Pack
update through a single content PR and catalog release, without releasing a
new Packy executable. This directly verifies the maintainer's desired shorter
workflow. It does not need a marketplace, arbitrary registries, several
publisher trust classes, a separate publisher product, or an external-demand
experiment.

Research was limited to repository contracts and current primary documentation;
no third-party installers were executed and no repositories, releases, or
workstation Pack state were changed.
