# Approved-issue delivery rework root-cause research

Research date: 2026-07-31

Status: Historical root-cause baseline, reconciled with the completed
[delivery-rework Wayfinder map](https://github.com/yersonargotev/packy/issues/386)
on 2026-07-31.

## Question and evidence boundary

What actually causes Packy issue-delivery rework and latency, and which
observable Packy-owned contracts could reduce it without recreating Packy
Delivery?

This read-only investigation covers the complete GitHub issues/comments/events
for [#101](https://github.com/yersonargotev/packy/issues/101),
[#384](https://github.com/yersonargotev/packy/issues/384), and
[#386–#393](https://github.com/yersonargotev/packy/issues/386);
[PR #385](https://github.com/yersonargotev/packy/pull/385) and its
[first-party CI run](https://github.com/yersonargotev/packy/actions/runs/30606890117);
current source, tests, ADRs, and research notes at
[`5cf9558`](https://github.com/yersonargotev/packy/tree/5cf9558df5f5dc03674d0b6fc913e2f5311c3ce0);
and local Git-common-directory Packy Delivery v2 evidence for
[#349](https://github.com/yersonargotev/packy/issues/349),
[#378](https://github.com/yersonargotev/packy/issues/378),
[#380](https://github.com/yersonargotev/packy/issues/380), and #384.
The four local runs are a recent comparison set, not a random sample.

The completed-outcomes reconciliation additionally covers the final state of
the map and all ten child issues, merged PRs #394–#406, and the durable research
reports linked from that section. Those later sources validate or supersede the
baseline recommendations; they are not part of the original causal sample.

No GitHub state, ref, release, workflow, or real user configuration was changed.

## Decision-ready answer

This is principally an **architecture and executable acceptance-seam problem**,
not an issue-schema problem.

For #384, qualification took **10m49s**, initial implementation **14m33s**,
review/repair/focused checks **59m33s**, final boundary plus exhaustive
validation **13m21s**, and CI wait **6m13s**. The issue already stated exact
same-tag recovery, no rebuild, governance freshness, immutable identity,
credential isolation, and privileged-boundary verification. Review nevertheless
found candidates that rebuilt or recreated release state, used stale governance,
diverged on provenance identity, and duplicated observation logic
([issue](https://github.com/yersonargotev/packy/issues/384)). The first and
second candidate-review records remain in the local Packy Delivery run state;
the decision-relevant aggregate is preserved here without publishing that
operational state.

Across #349, #378, and #380, every issue already had an explicit
`## Acceptance criteria` section, yet Packy Delivery still requested
qualification corrections for owning seams, exact test locators, negative
mutations, preservation proof, and validator binding. That is Packy Delivery's
evidence-plan compilation, not missing product intent.

The smallest supported response is:

1. measure a minimal human-visible issue rubric before adding a parser;
2. prioritize one process scenario through real release adapters;
3. measure validator phases from existing logs before adding a schema; and
4. leave qualification, candidate binding, evidence identity/reuse, run state,
   and GitHub observation in Packy Delivery.

## Measured timeline

The successful #384 Packy Delivery run started at `03:49:20Z` and completed at
`05:36:15Z`; [PR #385](https://github.com/yersonargotev/packy/pull/385)
opened at `05:28:15Z` and merged at `05:35:34Z`. The exact completed-run
identity is recorded under **Evidence provenance** below.

| Interval | Wall time | Share |
| --- | ---: | ---: |
| Qualification: four correction/review cycles | 10m49s | 10.1% |
| Initial implementation | 14m33s | 13.6% |
| Reviews, three repairs, focused checks, specialists | 59m33s | 55.7% |
| Boundary and exhaustive validation | 13m21s | 12.5% |
| Required-check wait | 6m13s | 5.8% |
| Push, PR, merge, adoption, synchronization, cleanup | 2m26s | 2.3% |
| **Total** | **1h46m55s** | **100%** |

The first qualification correction generated one large criterion from #384's
user stories and decisions. Three subsequent cycles refined evidence locators:
security/preservation, complete release-integrity proof, then exact test and
fixture names. These corrections are in `qualification_corrections` and
`qualification_reviews` in the completed run above.

Candidate review, not qualification, dominated. The first candidate violated
explicit issue requirements and existing
[ADR 0014](../adr/0014-build-once-release-publication.md) and
[ADR 0015](../adr/0015-detect-governance-drift-without-repair-authority.md).
Repairs centralized decisions in
[`internal/release/admission.go`](../../internal/release/admission.go) and ref
observation in
[`scripts/verify-release-ref-state.sh`](../../scripts/verify-release-ref-state.sh).
That is architecture deepening, not issue normalization.

## Cross-delivery evidence

“Review/repair” includes review, adjudication, repair, focused validation, and
specialist review. Timings come from the completed local v2 run records for
[#349](https://github.com/yersonargotev/packy/issues/349),
[#378](https://github.com/yersonargotev/packy/issues/378),
[#380](https://github.com/yersonargotev/packy/issues/380), and
[#384](https://github.com/yersonargotev/packy/issues/384). Their immutable
local record identities are listed under **Evidence provenance**.

| Issue | Total | Qualification | Implementation | Review/repair | Final validation | CI |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| #349 | 42m19s | 6m30s | 11m59s | 4m44s | not separate | 6m07s |
| #378 | 2h33m01s | 8m11s | 3m35s | 1h00m08s | 42m37s | 8m28s |
| #380 | 1h03m36s | 4m01s | 3m54s | 40m34s | not separate | 0m25s |
| #384 | 1h46m55s | 10m49s | 14m33s | 59m33s | 13m21s | 6m13s |

This proves only that explicit criteria did not eliminate qualification work in
three recent cases and that review/repair far exceeded initial implementation
on #378, #380, and #384. It does not establish a repository-wide average.

## Recurring categories and ownership

| Category | Observation | Owner | Strength |
| --- | --- | --- | --- |
| Explicit product criteria | #384 lacked a literal acceptance heading; the other three runs had one. | Packy issue convention | Medium |
| Owning seam and evidence plan | All four runs refined exact seams, tests, mutations, preservation, or validator evidence. | Packy Delivery compiler, observing Packy | High for sample |
| Sensitive failure/recovery | #384 stated the invariants, but fragmented workflow/domain adapters composed them incorrectly. | Packy release architecture/tests | High |
| Architecture placement | Review moved decisions from YAML shell into `internal/release` and removed duplicated observers. | Packy ADRs/source/tests | High |
| Dependencies/outcomes | No sampled pause was caused by dependency state. Native `blocked_by` already defines frontier. | GitHub convention observed by Packy Delivery | No support for new mechanism |

At baseline commit `5cf9558`, the release tests executed event normalization
with fake `git` and `gh`, but privately extracted one named `run:` block from
YAML; many other checks asserted workflow text and ordering
([`runReleaseEventFixture`](../../internal/release/release_automation_test.go)).
A single process scenario is therefore aligned with an observed seam gap.

## Baseline validation: regression exists, receipt is redundant

Issue #101 removed recursive and duplicate work, reducing the recorded Validate
job median from **588s to 142s** and its validation-step median to **120s**
([issue](https://github.com/yersonargotev/packy/issues/101),
[investigation](validate-packy-performance.md),
[post-change evidence](ci-validation-performance-evidence.md)).

The eight successful validation steps inspected at the baseline took 303, 308,
319, 320, 353, 355, 365, and 372 seconds: median **336.5s**. For
[PR #385's job](https://github.com/yersonargotev/packy/actions/runs/30606890117/job/91080915486),
existing log timestamps already show:

| Interval | Approximate time |
| --- | ---: |
| Vercel + Addy acceptance | 99.6s |
| Format + build + vet | 3.4s |
| Ordinary tests | 158.2s |
| Race tests | 103.3s |

The same log reports `internal/release` at **139.333s** in ordinary tests and
`internal/cli` at **89.966s** under race.

[`scripts/validate-packy.sh`](../../scripts/validate-packy.sh) already emits
phase markers, and
[`TestCIUsesOnlyTheValidationEntrypoint`](../../internal/ci/validation_test.go)
enforces one CI authority. GitHub adds timestamps. Packy Delivery already emits
`packy.exhaustive-validation/v1` with commit/tree, checkout digest, validator
identity/digest, sandbox, command, completion, and success in the #384
completed-run record identified under **Evidence provenance**.
At the baseline, portable local intra-validator timing was clearly missing;
whether failed-phase reporting was sufficient remained to be tested. The later
[timing report](validator-phase-timing.md) confirmed that existing output
already identifies the failed phase.

## ADR 0021 boundary

[ADR 0021](../adr/0021-extract-issue-delivery.md) retains Packy's observable
validation, workflow, issue/governance, architecture, and domain contracts, but
moves the evidence model, resumable engine, adapters, workflow lifecycle,
persistence, and recovery to `packy-delivery`.

| Packy owns | Packy Delivery owns |
| --- | --- |
| Human-visible issue/label/dependency conventions | Authority/evidence compilation and readiness outcomes |
| Product ADRs and domain seams | Acceptance matrix, receipts, reuse/invalidation |
| Release workflow, adapters, executable scenarios | Review/repair sequencing and run state |
| Validator phases, output, exit, required checks | Candidate binding, CI polling, effects, resume, cleanup |

Optional validator phase timing fits the boundary. A Packy-owned GitHub
snapshot collector, delivery outcome model, contract digest, candidate receipt,
or preflight lifecycle duplicates the extracted product.

## Smallest experiments

1. **Issue rubric:** for five approved issues require only objective, enumerated
   criteria, preservation/out of scope, and dependencies/prerequisites. Measure
   correction count/category/time. No marker, IDs, digest, JSON, or linter.
2. **Release scenario:** replay absent-release recovery, moved/lost-ancestry
   drift, and sealed-release disappearance through checked-in normalization,
   admission, and ref adapters with fake `git`/`gh`. Promote a seam only if it
   catches a known escaped defect.
3. **Timing:** collect three local warm-cache, three disposable-cache, and three
   CI runs using current markers, Go durations, and Actions timestamps. Add
   opt-in human timestamps only if local failures remain opaque.
4. **Packy Delivery:** compare its initial and final evidence matrices for the
   four sampled runs; test source-aware seam discovery and batched findings
   there, outside Packy.

## Original recommendations and disposition

The recommendations below are retained as the decision baseline that shaped
the map. Every in-scope investigation and implementation has since completed;
the disposition after them links the durable results.

### #387

Keep [#387](https://github.com/yersonargotev/packy/issues/387) as research
through the five-issue rubric experiment. Do not implement the withdrawn v1
marker, ten-section grammar, `AC-NNN`, sensitive-effect enum, dependency mirror,
snapshot, digest, outcomes, or exit-code protocol. If evidence supports a
change, start with a Markdown template; add a read-only linter only for a
repeated mechanically detectable omission, never as a Governance merge gate.

### #389

Narrow [#389](https://github.com/yersonargotev/packy/issues/389) to opt-in phase
timestamps and failed-phase reporting, or close it if the timing experiment
shows logs suffice. Remove candidate/tree identity, validator digest, receipt
lifecycle, interruption-success semantics, and CI artifact upload: Packy
Delivery and GitHub already provide them. Do not block #390 on a schema.

### #390

Retain [#390](https://github.com/yersonargotev/packy/issues/390) as research,
unblocked from #389. Use the prior 120s median as control and the current 336.5s
median as observation. Investigate, in order: `internal/release` ordinary tests,
`internal/cli -race`, duplication across Vercel/Addy and Go phases, then only
proven-safe caching or parallelism. Preserve one exhaustive authority, explicit
allowlist, exact-candidate run, meaningful race coverage, and sandboxed
HOME/XDG; use repeated measurements, not a wall-clock gate.

For the [#386 map](https://github.com/yersonargotev/packy/issues/386), keep two
tracks: release acceptance architecture (#388, then evidence-supported
#391/#392) and validation diagnosis (#390). Do not put full #387 or #389 on the
implementation frontier. #393 must not invoke the scenario twice:
`internal/release` is already in the canonical ordinary-test allowlist and
excluded only from race
([validator](../../scripts/validate-packy.sh),
[#393](https://github.com/yersonargotev/packy/issues/393)).

## Completed outcomes

The map's ten child issues are closed and its native frontier is empty. The
[Wayfinder resolution](https://github.com/yersonargotev/packy/issues/386#issuecomment-5145846888)
records why no `to-spec` handoff remains: this effort carried its evidence-led
changes through execution.

- [Measure whether an issue rubric reduces qualification rework](https://github.com/yersonargotev/packy/issues/387)
  found no repeated mechanical issue-body omission. The durable
  [qualification experiment](issue-rubric-qualification-experiment.md)
  supports adding neither a template nor a linter.
- [Establish one process-level release acceptance scenario](https://github.com/yersonargotev/packy/issues/388),
  [fail closed on release identity drift at every privileged boundary](https://github.com/yersonargotev/packy/issues/391),
  [prove retained-candidate recovery through the release scenario seam](https://github.com/yersonargotev/packy/issues/392),
  and [integrate release scenarios exactly once into Packy validation](https://github.com/yersonargotev/packy/issues/393)
  were merged through [PR #394](https://github.com/yersonargotev/packy/pull/394),
  [PR #395](https://github.com/yersonargotev/packy/pull/395),
  [PR #396](https://github.com/yersonargotev/packy/pull/396), and
  [PR #397](https://github.com/yersonargotev/packy/pull/397). Together they
  provide one checked-in normalization seam, deterministic sandboxed release
  scenarios, fail-closed drift and recovery coverage, and exactly one canonical
  validator invocation.
- [Measure whether validator logs need additional phase timing](https://github.com/yersonargotev/packy/issues/389)
  concluded that existing logs suffice for failure diagnosis and CI timing.
  The [timing report](validator-phase-timing.md) leaves only opt-in local
  timestamps as a separate usability enhancement outside this map.
- [Diagnose the current validation performance regression](https://github.com/yersonargotev/packy/issues/390)
  attributed the regression primarily to Packy-owned assurance growth rather
  than cache or runner drift. Its
  [performance report](validate-packy-performance-regression.md) produced three
  evidence-scoped implementation tickets.
- [Profile and reduce Vercel acceptance validation cost](https://github.com/yersonargotev/packy/issues/398),
  [preserve meaningful race coverage while reducing internal/cli validation cost](https://github.com/yersonargotev/packy/issues/399),
  and [profile and reduce internal/release validation cost](https://github.com/yersonargotev/packy/issues/400)
  were merged through [PR #404](https://github.com/yersonargotev/packy/pull/404),
  [PR #405](https://github.com/yersonargotev/packy/pull/405), and
  [PR #406](https://github.com/yersonargotev/packy/pull/406). Their durable
  reports record controlled median reductions of 54.65%/50.52% for warm and
  disposable Vercel acceptance, 46.75%/37.75% for the CLI race phase, and
  14.05% for disposable-cache release validation; the overlapping warm release
  result remains inconclusive.

The destination was therefore met without importing Packy Delivery's evidence
model, weakening release or validation assurance, adding an issue-body
protocol, or creating a second validation authority.

## Evidence provenance

The detailed Packy Delivery run records remain intentionally outside tracked
repository content. These identities make the summarized measurements
traceable in an operator workspace that retains the corresponding local run
state without publishing operational receipts or relying on broken `.git`
links in rendered documentation:

| Issue | Revision identity | Completed-run identity |
| --- | --- | --- |
| #349 | `3c59f0f959d1786c6eace078725d5e7d3ee2cefdbdc2a350e6a4b38f6b2decd1` | `582fc6a53a9e501db56a89bf65d480724734a736b62854da42eeb2040889592c` |
| #378 | `833adfa9e7d42f2f374e08eed3a2ea5ecd9430d15f7bb9df4be8f4664b689b79` | `d8f2e8ca60cb648e5a948804cc78c5c308ae91601e681d33ccb1a3cf9914bb03` |
| #380 | `b282d3b301d5bb68885ac58998a40fdd7a9d65179d9702a6467b9cc9fae3b80d` | `e1888278fb2ddecc234ed7ce2024ca7e89acbf71d9c281ca6e9281dcf7ef34c9` |
| #384 | `0282c0865dac956d093cb34904b9c003ede19e4b03092e7ae75e5786c7bc82b7` | `922ddee34a8d2f4228ba1588914101775401e08deeeb0b5d252a19fd4b7bd10a` |

## Risks and conclusion

The detailed Packy Delivery evidence remains untracked local Git state; this
document and the qualification experiment preserve the redacted durable
aggregate. Phase time includes agent/tool activity, the sample is selected, and
no counterfactual proves a rubric or scenario would prevent a repair. Detailed
validator phase data comes from one PR, though the eight-run regression is
repeated.

The measured root problem is the interaction of Packy Delivery's evidence-plan
compilation, Packy's fragmented release acceptance seam, and renewed validator
cost. Deepen the release seam and diagnose validation at its existing boundary;
formalize issue text only as measured omissions require.
