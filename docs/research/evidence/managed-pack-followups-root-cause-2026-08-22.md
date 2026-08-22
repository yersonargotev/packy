# Managed Pack follow-ups: root-cause analysis

Date: 2026-08-22

Scope: issues [#722](https://github.com/yersonargotev/packy/issues/722), [#723](https://github.com/yersonargotev/packy/issues/723), and [#725](https://github.com/yersonargotev/packy/issues/725), connected to the Managed Pack specification [#696](https://github.com/yersonargotev/packy/issues/696), repairs [#718](https://github.com/yersonargotev/packy/pull/718) and [#719](https://github.com/yersonargotev/packy/pull/719), and infrastructure follow-up [#724](https://github.com/yersonargotev/packy/issues/724).

## Conclusion

There is no single implementation defect or single architectural root cause behind all four follow-ups. The Managed Pack migration was the exposing event: it exercised new authoring, promotion, review, fixture, and subprocess paths together and revealed several independent responsibility gaps:

1. preventive validation stops at the authoring closure and does not prove the materialized runtime behavior (#722);
2. promotion enforcement and proposal explanation do not share a semantic change model (#723);
3. generic behavior tests still borrow mutable real-catalog identities and evidence (#725); and
4. real-subprocess tests construct isolation independently and incompletely (#724).

The live-catalog part of the third gap directly explains #718. A separate fixture-construction defect inside the same #725 scope explains the contradictory promotion fixture repaired in #719: bytes changed while provenance still claimed `exact-copy`. The cleanup races came from incomplete descendant policy at two production subprocess boundaries; #724 addresses recurrence in test-owned subprocess setup but would not have prevented those production omissions. The first gap is a real preventive risk at the immutable-release boundary, but the record does not show defective pstack runtime content that it would have rejected earlier. The second is a reviewer-information gap discovered during migrations, not a cause of those test failures.

The useful common observation is therefore about architecture sequencing, not causality: Packy's source of truth moved to Managed Pack Projects before every surrounding proof received a cohesive owner. Authoring does not yet prove runtime fitness, review does not receive a canonical semantic delta, generic tests borrow production catalog state, and test subprocess setup borrows ambient host policy. These responsibilities belong behind separate, cohesive interfaces; combining them into one new Managed Pack subsystem would recreate the shallow coupling that ADR 0038 removed.

## Symptoms and causal chains

### Runtime fitness appears after the immutable boundary (#722)

`managedpackvalidate` and the promotion offline worker both call only `managedpack.ValidateProject`; the latter validates manifest structure, provenance, exact-copy equality, and closure, and `MaterializeClosure` is a separate operation ([validator](https://github.com/yersonargotev/packy/blob/ceff6bc1dbcebd5dde9c10e7e47d39c86c6e700c/internal/managedpack/validator.go#L159-L243), [preventive CLI](https://github.com/yersonargotev/packy/blob/ceff6bc1dbcebd5dde9c10e7e47d39c86c6e700c/internal/tools/managedpackvalidate/main.go#L79-L93), [offline worker](https://github.com/yersonargotev/packy/blob/ceff6bc1dbcebd5dde9c10e7e47d39c86c6e700c/internal/managedpackpromotion/offlinevalidation/worker.go#L48-L67)). Runtime/catalog fitness happens later, after candidate materialization, through repository gates including the capability-pack tests ([candidate preparation](https://github.com/yersonargotev/packy/blob/ceff6bc1dbcebd5dde9c10e7e47d39c86c6e700c/internal/managedpackpromotion/repositorycandidate/preparer.go#L98-L135), [production gates](https://github.com/yersonargotev/packy/blob/ceff6bc1dbcebd5dde9c10e7e47d39c86c6e700c/internal/managedpackpromotion/repositorycandidate/gates.go#L23-L39)).

Causal chain: reusable pre-release validation proves the declared closure but not `ValidateProject -> MaterializeClosure -> runtime load -> every surface/selection` -> a genuinely runtime-invalid immutable release could pass preventive checks and fail only during promotion. The pstack 1.0.1 release demonstrates that the late promotion gate exists, but not a runtime defect: its `resource-surfaces` rejection came from tests that still encoded 1.0.0 legacy admission facts ([#718 root cause](https://github.com/yersonargotev/packy/pull/718)). #722 closes the real late-detection window; #725 removes the unrelated live-catalog coupling that produced the observed rejection.

This is narrower than a missing validator in general. ADR 0038 already requires preventive validation and byte-exact provenance ([ADR 0038](https://github.com/yersonargotev/packy/blob/ceff6bc1dbcebd5dde9c10e7e47d39c86c6e700c/docs/adr/0038-promote-releases-from-managed-pack-projects.md#L54-L60)); the missing proof is runtime materialization and the complete deterministic matrix.

### Reviewers see candidate state, not semantic change (#723)

The candidate summary currently enumerates release identity, current candidate origins/adaptations/notices, one compatibility-floor reason, and Git paths ([summary implementation](https://github.com/yersonargotev/packy/blob/ceff6bc1dbcebd5dde9c10e7e47d39c86c6e700c/internal/managedpackpromotion/repositorycandidate/preparer.go#L472-L532)). Separately, `compatibilityFloor` short-circuits on the first applicable category and returns a single reason ([SemVer implementation](https://github.com/yersonargotev/packy/blob/ceff6bc1dbcebd5dde9c10e7e47d39c86c6e700c/internal/managedpackpromotion/repositorycandidate/preparer.go#L260-L343)).

Causal chain: comparison logic is encoded as an enforcement-only decision tree -> summary rendering independently inventories candidate state -> simultaneous semantic changes and all their reasons are not represented -> reviewers reconstruct resource, graph, binding, provenance, and legal deltas from JSON/Git diffs. This partially misses #696 user story 20 and was observed during Matty and Addy migration/version decisions ([#723 problem and scope](https://github.com/yersonargotev/packy/issues/723)). It did not make promotion validation incorrect; it made review evidence incomplete and allowed explanation/enforcement to drift.

### Generic tests depend on live Pack migration state (#725)

Before #718, the pstack compatibility test combined an 81-case runtime matrix with legacy manifest version, source reference, source lock, history, exclusions, and a 156-path review inventory. Commit [`c73661e`](https://github.com/yersonargotev/packy/commit/c73661eee1510fb3f3a5d679720d12aee5e03075) removed 174 lines of legacy-admission assertions, derived CLI-visible versions from the checked-in manifest, and moved the temporary real legacy-loader check to Issue Delivery. The remaining pstack-specific test now protects only its public 3-surface x (all + 26 skills) contract ([current matrix](https://github.com/yersonargotev/packy/blob/ceff6bc1dbcebd5dde9c10e7e47d39c86c6e700c/internal/capabilitypack/pstack_pack_test.go#L10-L54)).

The promotion composition test still names fixtures `small`, `matty`, and `pstack` and derives a patch candidate by copying and mutating their trees ([integration fixture](https://github.com/yersonargotev/packy/blob/ceff6bc1dbcebd5dde9c10e7e47d39c86c6e700c/internal/managedpackpromotion/repositorycandidate/integration_test.go#L24-L43), [mutation construction](https://github.com/yersonargotev/packy/blob/ceff6bc1dbcebd5dde9c10e7e47d39c86c6e700c/internal/managedpackpromotion/repositorycandidate/integration_test.go#L196-L234)). Its initial byte mutation left `exact-copy` provenance unchanged, so strict validation correctly rejected it; commit [`1749f30`](https://github.com/yersonargotev/packy/commit/1749f302e3dbb2c98fde078c31134ce643a266d2) repaired the fixture by declaring the mutated whole resource `adapted`.

Causal chain A: generic tests reuse live catalog Packs because their fixtures offer convenient realism -> migrations change versions/layout/provenance while generic behavior is unchanged -> tests assert obsolete legacy evidence (#718) -> each migration moves the dependency to whichever real Pack remains legacy. ADR 0035 already requires differently named synthetic Packs for catalog-level readiness fitness ([ADR 0035](https://github.com/yersonargotev/packy/blob/ceff6bc1dbcebd5dde9c10e7e47d39c86c6e700c/docs/adr/0035-make-pack-readiness-capability-driven.md#L88-L99)); #725 proposes extending that principle to broader generic lifecycle and promotion tests.

Causal chain B: a mutation helper changes resource bytes without changing the origin tree or `exact-copy` relationship -> strict provenance validation correctly rejects the candidate (#719). A synthetic fixture could make the same mistake, so synthetic identities alone are insufficient. #725 must also make fixture mutations domain-coherent by construction.

### Test isolation can repeat subprocess defects but did not cause the production races (#724)

After the fixture repair, CI exposed two independent writers that can outlive their direct parent: Go telemetry and Git automatic maintenance ([#719 root causes](https://github.com/yersonargotev/packy/pull/719)). Commit [`d442d75`](https://github.com/yersonargotev/packy/commit/d442d75126a3ec07abb753f641e6125df52ef6f0) set `GO_TELEMETRY_CHILD=2` in the production authority-phase environment and disabled `maintenance.auto`/`gc.auto` in production promotion clones ([authority environment](https://github.com/yersonargotev/packy/blob/ceff6bc1dbcebd5dde9c10e7e47d39c86c6e700c/internal/managedpackpromotion/authorityphase/adapter.go#L207-L262), [Git environment](https://github.com/yersonargotev/packy/blob/ceff6bc1dbcebd5dde9c10e7e47d39c86c6e700c/internal/managedpackpromotion/repositorycandidate/preparer.go#L601-L617)). The observed root was incomplete descendant policy at two distinct production process boundaries, not test helper duplication.

Separately, direct test subprocesses construct overlapping HOME/XDG/Git/Go environments inconsistently ([CI Go helper](https://github.com/yersonargotev/packy/blob/ceff6bc1dbcebd5dde9c10e7e47d39c86c6e700c/internal/ci/inert_bundle_go_boundary_test.go#L54-L70), [promotion Git helper](https://github.com/yersonargotev/packy/blob/ceff6bc1dbcebd5dde9c10e7e47d39c86c6e700c/internal/managedpackpromotion/repositorycandidate/preparer_test.go#L594-L632), [sync fixture](https://github.com/yersonargotev/packy/blob/ceff6bc1dbcebd5dde9c10e7e47d39c86c6e700c/internal/tools/syncpacksource/single_source_admission_test.go#L377-L404)), while the repository candidate gate owns another production environment ([gate environment](https://github.com/yersonargotev/packy/blob/ceff6bc1dbcebd5dde9c10e7e47d39c86c6e700c/internal/managedpackpromotion/repositorycandidate/gates.go#L42-L88)). #724 proposes a test-only module so new tests do not recreate the same class of omission. It is recurrence prevention, not the root fix for the two production paths, and it must not merge their authority policies into test plumbing.

## Causal classification

Observed failure causes, ranked by demonstrated recurrence:

1. **Generic behavioral tests use mutable real-catalog and legacy state (#725).** This directly caused #718 and remains exposed during the remaining migrations.
2. **Fixture mutation is byte-oriented rather than domain-coherent (#725).** This directly caused the exact-copy rejection in #719; replacing names with synthetic identities is not enough.
3. **Production subprocess boundaries omitted complete descendant policy (#719).** Go telemetry and Git maintenance were separate mechanisms with the same lifecycle consequence, each fixed in its owning production module.

Preventive and review gaps exposed by the same migration, but not causes of those failures:

1. **Preventive Managed Pack validation ends before runtime fitness (#722).** A real irreversible-boundary risk: project authors cannot exercise the same materialized surface/selection contract that promotion later exercises.
2. **Semantic comparison has no single typed representation (#723).** An important review and drift-risk defect; no incorrect admission caused by it is evidenced.
3. **Direct test subprocess isolation has no cohesive owner (#724).** A cross-package recurrence risk after the production defects were discovered.

## What is not a root cause

- **Strict exact-copy rejection is correct.** ADR 0038 requires byte-for-byte comparison, and `ValidateProject` performs provenance validation before sealing the closure. The fixture, not the validator, was inconsistent. Automatic reclassification would weaken reviewed provenance.
- **pstack 1.0.1 content or Pack runtime behavior is not shown defective.** #718 reports the managed candidate reproduction passed after removing legacy-admission coupling while preserving all 81 compatibility cases.
- **Immutable releases are not the defect.** They intentionally make late feedback expensive; #722 should move runtime fitness earlier without treating a pre-release report as admission authority. Promotion must reacquire and revalidate, per ADR 0038's separated authority model ([ADR 0038](https://github.com/yersonargotev/packy/blob/ceff6bc1dbcebd5dde9c10e7e47d39c86c6e700c/docs/adr/0038-promote-releases-from-managed-pack-projects.md#L25-L39)).
- **Go telemetry and Git maintenance are not one shared domain bug.** They are two independent immediate writers; each production process boundary omitted a policy needed to prevent descendants from outliving cleanup.
- **The lack of durable checkpoint/resume is not causal.** #696 explicitly defines one idempotent promotion operation, and ADR 0038 requires transient protocol/candidate state to be removed after the typed result. None of #718/#719 failed because state could not be resumed.
- **Readiness semantics are not implicated.** ADR 0038 explicitly preserves ADR 0035 readiness and distinguishes exact projection from runtime usability ([ADR 0038](https://github.com/yersonargotev/packy/blob/ceff6bc1dbcebd5dde9c10e7e47d39c86c6e700c/docs/adr/0038-promote-releases-from-managed-pack-projects.md#L70-L75)).

## Recommended sequencing

1. **Clarify and then prefer #724 first.** Establish one reliable test subprocess environment before broad fixture work creates more real-process tests; keep the production fixes in their owning modules.
2. **#725 second and before the remaining Issue Delivery/Orchestrate migrations.** Replace generic live-Pack dependencies and pin the temporary synthetic legacy-loader fixture; delete that fixture with the legacy loader.
3. **#722 before the next immutable Managed Pack publication.** Reuse `ValidateProject` and `MaterializeClosure`, then run deterministic runtime fitness through `internal/capabilitypack`; promotion must still independently reacquire and repeat validation.
4. **#723 independently once its SemVer authority split is accepted.** One private typed comparison should drive both all mechanical-floor reasons and Markdown. Keep behavioral compatibility and legal conclusions human-owned.

This advisory ordering matches the retrospective recommendation recorded on [#696](https://github.com/yersonargotev/packy/issues/696#issuecomment-5382519782): #724 before broad fixture migration, #725 before remaining migrations, #722 before the next immutable publication, and #723 independently. The same comment explicitly leaves #722–#725 in `status:needs-review`; it does not approve their scopes or order. No ADR change is required if the eventual approved scopes preserve these authority boundaries.

## Uncertainty

The issue/PR record establishes that Matty and Addy exposed practical semantic-version review difficulty, but it does not identify a specific incorrect version admitted because of the summary gap. Therefore #723 is supported as a visibility and drift-risk gap, not as evidence of an already incorrect admission.
