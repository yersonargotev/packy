# Argote Pack Lifecycle Friction (v0.1.16 through HEAD)

## Question and scope

This report investigates why introducing and then making `argote` maintainable as
a Pack Source required more work than a single ordinary update.  The interval is
from `v0.1.16` (`35d8090`, 2026-08-06) through `main` at `3317aa7`.
All evidence is first-party: repository history and checked-in contracts, plus
the Packy GitHub issue and pull-request records linked below.

This historical report uses **Pack Source** only when naming the v0.1 domain
concept or its checked-in contracts. The accepted v0.2 vocabulary uses **Pack
source reference** for informational upstream metadata.

There has **not** been a post-registration Argote content update in this
interval: the only `argote` source-sync commit is the initial registration at
the same upstream commit used by the initial pack issue.  Therefore, claims
about subsequent-update friction are conclusions from the enforced lifecycle,
not measurements of repeated Argote updates. Local Git objects `35d8090`
(`v0.1.16`), `bafbe73` (initial pack), and `307aeea` (only Argote sync); [issue
#505](https://github.com/yersonargotev/packy/issues/505); [issue #506](https://github.com/yersonargotev/packy/issues/506).

## Executive finding

The friction was principally an intentional **bootstrap split**: Packy’s
single-source v2 registration requires the source to be absent, while the
approved Argote plan required the manifest and vendored resources to be on
protected `main` before source registration and project-install verification.
That produced a hand-authored pack PR (#507), then an automation-owned source
registration PR (#508), even though both were for upstream commit
`02762547c0c312f3cc7519ac7044964c43657cbb`. [Workflow contract,
lines 37-43](../../workflows/pack-source-synchronization.md#L37-L43); [issue
#505 scope](https://github.com/yersonargotev/packy/issues/505); [issue #506
prerequisite and source identity](https://github.com/yersonargotev/packy/issues/506); [PR #507](https://github.com/yersonargotev/packy/pull/507); [PR #508](https://github.com/yersonargotev/packy/pull/508).

The split is not a safety defect in the final state: HEAD has one configured
Argote source with the three declared bindings and a lock recording that exact
commit and the vendored-file hashes. [Source configuration](../../bundle/sources.json#L278-L304); [Argote lock](../../bundle/sources/argote.lock.json#L7-L92). It is a workflow ergonomics problem for a new **single-source** pack: v3 can atomically introduce an absent pack and manifest, but only when there are at least two sources; v2 `register` does not carry a proposed manifest. [Workflow contract,
lines 37-43 and 76-86](../../workflows/pack-source-synchronization.md#L37-L43).

The raw interval is larger than the pack lifecycle itself: `v0.1.16..HEAD`
changes 51 files (`+1907/-104`). The strict Argote add and final registration
account for 19 of those files (`+651/-12`): 17 in the initial pack PR and two in
the generated source-registration PR. The initial PR also stores the manifest
and three payloads twice, once as current content and once as immutable history,
then adds `artifact.json` and edits Go registries/tests and prose catalogs. The
remaining interval work belongs to project-installation and governance
capabilities exposed while delivering #506, not to the three Argote resources
themselves. Local Git: `git diff --stat v0.1.16..HEAD`, `git show --stat
bafbe73`, and `git show --stat 307aeea`; [PR #507
files](https://github.com/yersonargotev/packy/pull/507/files); [PR #508
files](https://github.com/yersonargotev/packy/pull/508/files).

## Evidence inventory

| Evidence | What it establishes |
| --- | --- |
| Git tag `v0.1.16` / `35d8090` | The baseline predates all Argote-specific commits in this report. (Local Git: `git show -s v0.1.16`.) |
| Local Git commit `bafbe73`, merged by [PR #507](https://github.com/yersonargotev/packy/pull/507) | Added `argote@1.0.0`, three resources, immutable history, catalog entry, docs, and tests; the commit changed 17 files / 529 lines according to its Git diff stat. |
| [Issue #505](https://github.com/yersonargotev/packy/issues/505) | Explicitly prohibited hand-authoring source config/lock and made those follow-up work after the manifest reached protected `main`. |
| Local Git commit `307aeea`, merged by [PR #508](https://github.com/yersonargotev/packy/pull/508) | Automation added only the Argote source configuration and lock (2 files / 122 lines), pinning the same upstream commit. |
| [`bundle/packs/argote/pack.json`](../../bundle/packs/argote/pack.json) | The resulting pack is schema v4, version `1.0.0`, with three independent resources and bindings for Claude, Codex, and OpenCode. |
| [`bundle/sources.json`](../../bundle/sources.json#L278-L304) and [`bundle/sources/argote.lock.json`](../../bundle/sources/argote.lock.json#L7-L92) | The final source/configuration-to-lock provenance record and exact three-resource mapping. |
| [Pack-source synchronization contract](../../workflows/pack-source-synchronization.md) and [ADR 0029](../adr/0029-reconfigure-pack-sources-atomically.md) | The v2/v3 admission, reconfiguration, sealing, validation, and publication rules that make the workflow deliberately multi-gated. |
| [Issue #506](https://github.com/yersonargotev/packy/issues/506) and [`docs/project-pack-lifecycle.md`](../project-pack-lifecycle.md#L1-L47) | Project-installation verification was a dependent concern, with its own lockfiles, projection ownership, and separate personal activation contract. |

## Lifecycle touchpoint matrix

| Touchpoint | Add Argote in the observed history | Change its selected resources / manifest | Update upstream bytes at the existing selection |
| --- | --- | --- | --- |
| Define portable content | #507 hand-authored the schema-v4 manifest, three resource files, immutable `1.0.0` history, catalog/docs, and coverage. [PR #507](https://github.com/yersonargotev/packy/pull/507); [`argote_pack_test.go`](../../internal/capabilitypack/argote_pack_test.go#L11-L79). | A v2 `reconfigure` requires a complete replacement `SourceConfig` *and* complete current-version proposed manifest, with exact bidirectional binding equality. [ADR 0029](../adr/0029-reconfigure-pack-sources-atomically.md); [workflow contract, lines 54-60](../../workflows/pack-source-synchronization.md#L54-L60). | Existing selected bindings are used by normal v1/v2 synchronization; source configuration remains the selected set. [Workflow contract, lines 37-43](../../workflows/pack-source-synchronization.md#L37-L43). |
| Admit source/provenance | #508 subsequently registered the previously absent source and created its lock. [PR #508](https://github.com/yersonargotev/packy/pull/508); [`sources.json`](../../bundle/sources.json#L278-L304). | Same v2 reconfigure operation, classified and applied as a complete generation including history, lock, selected bytes, and compatibility evidence. [Workflow contract, lines 54-60](../../workflows/pack-source-synchronization.md#L54-L60). | Normal sync resolves an explicit candidate; exact same identity is a no-op. [Workflow contract, lines 88-90](../../workflows/pack-source-synchronization.md#L88-L90); [lines 257-261](../../workflows/pack-source-synchronization.md#L257-L261). |
| Classify and validate | #508’s owner-produced PR records a plan, candidate, base/head/tree, and decision-ready state. [PR #508](https://github.com/yersonargotev/packy/pull/508). | Reconfigure derives additions/removals and compatibility floor; it does not accept a caller-selected version. [ADR 0029](../adr/0029-reconfigure-pack-sources-atomically.md). | Inspect validates a sealed plan; Publish reacquires, applies, validates staged Pack content, and reobserves state. [Workflow contract, lines 142-153](../../workflows/pack-source-synchronization.md#L142-L153); [lines 180-197](../../workflows/pack-source-synchronization.md#L180-L197). |
| Publish/merge | Two protected-main PRs: product pack #507, then automation source PR #508. [PR #507](https://github.com/yersonargotev/packy/pull/507); [PR #508](https://github.com/yersonargotev/packy/pull/508). | Automation owns only `sync/<source-id>` and refuses diverged/edited/ambiguous state. [Workflow contract, lines 255-280](../../workflows/pack-source-synchronization.md#L255-L280). | Same managed PR path; a ready PR is not equivalent to merge, which remains manual. [Workflow contract, lines 5-11](../../workflows/pack-source-synchronization.md#L5-L11). |
| Verify user-facing lifecycle | #506 also required full and selected-root project-install previews on all three surfaces, after registration. [Issue #506](https://github.com/yersonargotev/packy/issues/506). | Manifest-v4 update preserves custom selections only if roots still exist or declare exactly one valid migration. [`capability-packs.md`, lines 183-187](../capability-packs.md#L183-L187). | User `pack update` preserves recorded selection/provider choices and refuses guessed root replacements. [`capability-packs.md`, lines 173-194](../capability-packs.md#L173-L194). |

## Friction causes

1. **The initial single-source pack had no atomic admission route.** The normal
   v2 register operation admits only an absent source, whereas the Argote
   follow-up explicitly required the manifest/resources to exist on protected
   `main` first. V3 solves “no pre-created manifest” only for an absent pack
   with *two or more* sources. Thus Argote could not use v3 and was intentionally
   split across #507 and #508. [Issue #505](https://github.com/yersonargotev/packy/issues/505); [issue #506](https://github.com/yersonargotev/packy/issues/506); [workflow contract, lines 37-43](../../workflows/pack-source-synchronization.md#L37-L43); [lines 76-86](../../workflows/pack-source-synchronization.md#L76-L86).

2. **The first delivery had two ownership models.** #505 required normal issue
   delivery for product/catalog/history changes, while #506 required the
   `sync-pack-source` workflow to own `bundle/sources.json` and the lock. This
   prevents a single PR from simply carrying both types of changes. [Issue
   #505](https://github.com/yersonargotev/packy/issues/505); [issue
   #506](https://github.com/yersonargotev/packy/issues/506); [workflow contract,
   lines 5-17](../../workflows/pack-source-synchronization.md#L5-L17).

3. **Safety evidence is intentionally expensive.** Dispatch seals configuration,
   candidate, mode, and reason; human classification is a two-dispatch,
   inspection-bound flow; publication then reacquires and validates before every
   write. That is a sensible provenance boundary, but it adds waiting, artifact
   handling, and stale-plan retry opportunities to every source change. [Workflow
   contract, lines 37-43](../../workflows/pack-source-synchronization.md#L37-L43);
   [lines 92-104](../../workflows/pack-source-synchronization.md#L92-L104); [lines
   180-197](../../workflows/pack-source-synchronization.md#L180-L197).

4. **A resource-selection change is deliberately broad.** Reconfigure requires
   all selection/configuration, manifest, history, compatibility evidence, lock,
   and bytes to move as one generation. This avoids inconsistent provenance, but
   a small upstream path rename is not a small edit. [ADR 0029](../adr/0029-reconfigure-pack-sources-atomically.md); [workflow contract, lines 54-60](../../workflows/pack-source-synchronization.md#L54-L60).

5. **Project-install verification was coupled as a delivery dependency, not an
   Argote-source requirement.** The work was correctly separated from personal
   runtime activation and includes project locks, contributor accounting, and
   three-surface representation; it enlarged the apparent “add a pack” journey
   but did not cause source registration itself. [Issue #506](https://github.com/yersonargotev/packy/issues/506); [`project-pack-lifecycle.md`, lines 1-47](../project-pack-lifecycle.md#L1-L47); [`project-pack-lifecycle.md`, lines 67-78](../project-pack-lifecycle.md#L67-L78).

## Contemporaneous work that should not be attributed to Argote

The following commits occurred between #507 and #508 but addressed different
concerns, so they are not evidence that Argote itself required more source
work:

- `a5f49a3` / `c471dfb` / `cf3413a` introduced or refined project installation
  and shared Codex/OpenCode instruction contributions. The change itself says
  it generalizes project instructions, and the project lifecycle documents a
  separate installation/activation contract. Local Git commit `cf3413a`;
  [`project-pack-lifecycle.md`](../project-pack-lifecycle.md#L1-L30).
- `6f0d64b`, `937cac9`, and `d990fc4` introduced/fixed approved-issue binding
  for synchronization proposals. ADR 0030 says this is governance authority for
  managed PR metadata, not an Argote resource or source mapping. [ADR
  0030](../adr/0030-bind-pack-source-delivery-to-approved-issues.md); [workflow
  contract, lines 45-52](../../workflows/pack-source-synchronization.md#L45-L52).

These changes may have affected the timing and administrative surface of #508,
but the repository evidence does not show an Argote content or binding change
caused by them. The final #508 diff contains only the Argote source config and
lock. [PR #508 files](https://github.com/yersonargotev/packy/pull/508/files).

## Measured maintenance and validation cost

The architectural cost is larger than the two Argote pull requests. At the
investigated HEAD, the source-management vertical comprised approximately:

- `internal/packsync`: 32 files and 9,919 lines;
- `internal/packclassification`: 2 files and 519 lines;
- `internal/packsyncworkflow`: 15 files and 3,720 lines;
- the private source-sync tool: 14 files and 5,174 lines; and
- Pack-source schemas: 25 files and 2,829 lines.

The custom governance packages added 23 files / 941 lines for authorization
(including 21 fixtures / 467 lines) and 2 files / 876 lines for drift
detection. Across the repository, 118 of the 912 commits since July touched
these source, governance, or validation surfaces. These counts were obtained
from the checked-out tree and local Git history on 2026-08-07.

A sandboxed run of the repository's canonical validator on 2026-08-07 passed,
but its ordinary Go-test phase reported 84.182 seconds for
`internal/release`, 53.960 seconds for `internal/cli`, 30.399 seconds for the
source-sync tool, 28.093 seconds for `internal/ci`, and 25.279 seconds for
`internal/packsync`. Package times overlap because Go runs packages in
parallel, so they must not be added together; they identify the dominant
validation surfaces. The validator then ran a second race-enabled suite,
including packages without meaningful concurrent behavior. Governance itself
was not a dominant runtime cost (both governance packages completed in under
two seconds in the ordinary phase), but it added fixtures, workflow gates, and
rework to every delivery.

## Decision resulting from this research

After reviewing this evidence, the maintainer accepted [specification
#513](https://github.com/yersonargotev/packy/issues/513) and [ADR
0031](../adr/0031-simplify-packy-around-reviewed-packs.md). Those records own
the resulting architecture and implementation scope.

They supersede candidate remedies that would optimize the former source
admission system, such as generalizing source registration or adding request
renderers. This report remains the evidence for the decision rather than a
second architectural contract.
