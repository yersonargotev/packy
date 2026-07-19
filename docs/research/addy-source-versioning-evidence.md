# Addy source identity, versioning, and compatibility evidence

## Evidence question and boundary

This note gathers repository-local evidence for the Wayfinder question **“Decide
Addy source identity, versioning, and compatibility policy.”** It records facts,
existing invariants, and unresolved policy seams; it does **not** choose the final
policy, synchronize Addy, change Pack schemas, activate a pack, or publish a
release. Upstream bytes remained inert during this investigation.

## Established vocabulary and ownership

- A **Pack observable contract** is the user-visible capability behavior and is
  explicitly distinct from the upstream source version or textual diff size.
  **Pack compatibility** asks whether a new observable contract preserves the
  active older one without an incompatible migration or newly mandatory user
  action ([`CONTEXT.md`](../../CONTEXT.md#L50-L57)).
- A **decision-ready synchronization proposal** binds exact identity,
  provenance, content changes, compatibility evidence, migrations, and
  validation; human acceptance is the only remaining decision
  ([`CONTEXT.md`](../../CONTEXT.md#L56-L57)).
- Pack lifecycle policy and host projection policy are separate. Capability-pack
  owns composition, blockers, readiness, plan sealing, stale-plan rejection, and
  verification, while Codex and OpenCode own host syntax, paths, translation,
  readiness evidence, and authorized application
  ([ADR 0005](../adr/0005-capability-pack-surface-adapter.md#L41-L52)).

These terms establish three different identities that must not be conflated:
the configured Pack Source, its exact acquired upstream candidate, and the
Packy's own observable-contract version.

## Current Pack Source identity model

### Configured identity and selection

- Source configuration schema v1 names a source by `id`, `provider`,
  `repository`, `selector`, and selected resource bindings
  ([`SourceConfig`](../../internal/packsync/types.go#L23-L39)). Unknown fields,
  duplicate source IDs, non-GitHub providers, malformed `owner/name`
  repositories, unsafe paths, and duplicate bindings are rejected
  ([configuration validation](../../internal/packsync/config.go#L13-L64)).
- The checked-in example currently identifies `mattpocock-skills` separately
  from repository `mattpocock/skills` and selects published stable releases
  ([`bundle/sources.json`](../../bundle/sources.json#L1-L12)). A caller must name
  the source when more than one source is configured; omission is accepted only
  when exactly one source exists
  ([source selection](../../internal/packsync/check.go#L709-L718)).
- Allowed selectors are: `stable-release` with no ref, one exact published
  prerelease tag, or one full lowercase 40-character commit SHA. Floating and
  unknown selectors are forbidden
  ([selector validation](../../internal/packsync/config.go#L78-L95)). The engine
  selects the newest stable release deterministically by publication time and
  release ID, resolves an exact prerelease tag, or requires exact commit
  equality ([candidate resolution](../../internal/packsync/check.go#L187-L224));
  focused tests reject branch, abbreviated-commit, and stable-release-with-ref
  inputs ([selector tests](../../internal/packsync/check_test.go#L498-L521)).

### Exact candidate and retained provenance

- A resolved candidate retains repository and owner textual/numeric/node
  identities; public/archive/disabled state; release metadata; the tag-ref and
  any annotated-tag chain; commit, tree, parents, commit verification; and the
  acquired archive digest
  ([`Candidate`](../../internal/packsync/types.go#L85-L110)).
- Admission requires the configured repository/owner identity, an active public
  repository, a full immutable commit and tree, continuous tag-to-commit
  provenance for releases, exact selector agreement, and complete verification
  evidence. Automatic stable selection additionally requires eligible verified
  evidence; explicit candidates may be unsigned but not carry malformed or
  inconsistent verification evidence
  ([candidate validation](../../internal/packsync/check.go#L380-L433)).
- The production lock retains source ID, repository and owner identity,
  selector, the full candidate, selected-resource evidence, and a snapshot
  digest ([`Lock`](../../internal/packsync/types.go#L135-L150)). Check rejects a
  moved repository/owner identity, a moved tag ref or changed release evidence,
  invalid retained provenance, and a mismatched snapshot
  ([lock validation](../../internal/packsync/check.go#L436-L469)). It also
  re-resolves the currently locked release/tag/commit and blocks if that exact
  provenance has changed
  ([continuity check](../../internal/packsync/check.go#L227-L255)).
- Current code reads and writes one repository-global
  `bundle/sources.lock.json`, even though `sources.json` can contain multiple
  configured sources ([Check input](../../internal/packsync/check.go#L146-L170),
  [proposed lock construction](../../internal/packsync/check.go#L317-L325)).
  Therefore, independent multi-source lock layout/ownership is a current model
  gap that the Addy decision must account for; the evidence does not choose its
  remedy.

## Addy upstream identity facts

- The existing inert inventory is pinned to repository
  `addyosmani/agent-skills`, release/tag `0.6.4`, and exact commit
  `98967c45a42b88d6b8fb3a88b7ff6273920763d6`
  ([inventory identity](addy-upstream-inventory.md#L7-L17)). At inspection time,
  `main` had advanced beyond that release
  ([version signals](addy-upstream-inventory.md#L186-L199)).
- The root and Codex plugin manifests report `1.0.0`, while the release is
  `0.6.4` and the Claude manifest has no version. The inventory consequently
  establishes these manifest values as conflicting host-distribution metadata,
  not sufficient source identity
  ([inventory findings](addy-upstream-inventory.md#L27-L34),
  [version signals](addy-upstream-inventory.md#L186-L201)).
- The already-decided first Addy observable contract requires the exact release
  and commit identity plus MIT notice/attribution as bundle metadata, alongside
  the complete dependency-closed 24-skill, four-agent, eight-workflow graph
  ([observable-contract boundary](addy-observable-contract.md#L15-L31)). Host
  manifests, source-maintainer material, and validation artifacts are excluded
  from consumer activation; the license/notice remains inert bundle material
  ([observable exclusions](addy-observable-contract.md#L89-L102)).

These facts constrain source provenance but do not establish the first Packy
`addy` pack version or a mapping from upstream release numbers to Pack versions.

## Version namespaces already enforced by Packy

### Pack version

- The current local catalog stores a Pack version in its Packy-owned manifest
  and exposes that value on the decoded Pack
  ([catalog model](../../internal/capabilitypack/catalog.go#L46-L55),
  [manifest decoding](../../internal/capabilitypack/catalog.go#L266-L295)).
  User-facing update means the current version in the local Packy-owned catalog,
  not selection of an upstream version
  ([capability-pack guide](../capability-packs.md#L64-L76)).
- When selected resources change, Check derives affected Pack impacts from the
  **current Pack manifest version**. Adding a selected resource has a minor
  mechanical floor, removing one has a major floor, and modifying upstream-owned
  bytes requires semantic evidence with no preselected bump
  ([impact derivation](../../internal/packsync/check.go#L334-L373)).
- Classification evidence binds the exact sealed plan, base SHA, complete
  candidate, current Pack version, mechanical floor, changed aspects, final
  level, and proposed Pack version
  ([classification contract](../../internal/packsync/classification.go#L20-L98)).
  A classifier cannot lower the engine floor or choose an arbitrary version:
  Packy requires the exact next canonical three-part SemVer patch, minor, or
  major version. Major results additionally require migration text and mandatory
  actions ([classification admission](../../internal/packsync/classification.go#L129-L163),
  [version calculation](../../internal/packsync/classification.go#L185-L214)).
  Tests cover contradictory current versions/floors, below-floor results,
  arbitrary versions, missing major migration/actions, and stale
  plan/base/candidate bindings
  ([classification tests](../../internal/packsync/classification_test.go#L13-L63)).

### Pack Source schema-suite version

- The five synchronization artifact schemas are one immutable suite, initially
  `v1.0.0`, under a versioned canonical Pages identity
  ([ADR 0011](../adr/0011-publish-versioned-pack-source-schema-suite.md#L16-L29)).
  Machines must select an exact suite version; there is no `latest` alias or
  network fallback. Suite patch/minor/major compatibility rules are defined
  independently, and suite versions are explicitly independent of Packy
  application releases
  ([ADR 0011](../adr/0011-publish-versioned-pack-source-schema-suite.md#L31-L37)).
- The existing Addy mapping requires Pack-schema and Pack Source evolution for
  agents, commands, dependency assets/source sets, and notice metadata; it notes
  that current bindings provide only one `kind`, `resource_id`, and
  `upstream_path` per resource
  ([mapping gaps](addy-capability-mapping.md#L148-L162)). Any change to the
  published synchronization artifact schemas must therefore follow ADR 0011's
  immutable-suite rule rather than silently changing v1.0.0 in place.

Upstream release version, Pack version, Pack Source schema-suite version, and
Packy application release are thus four separate namespaces in current evidence.

## Compatibility and migration evidence model

- Synchronization classifies the Pack observable contract, not the upstream
  release label. Byte changes require semantic review; selected-resource adds or
  removals impose engine-owned floors
  ([impact derivation](../../internal/packsync/check.go#L334-L373)).
- Existing accepted compatibility history demonstrates a distinct migration
  artifact from Pack version `1.0.0` to `2.0.0`. Validation expects a human
  major decision, rationale, replacement semantics, historical artifact hashes,
  unchanged source selection, and an explicit refusal to replace source
  provenance
  ([compatibility validation](../../internal/packsync/compatibility.go#L107-L191),
  [selection validation](../../internal/packsync/compatibility.go#L210-L257)).
- Current validation requires exact from/to versions, human major classification,
  rationale, non-upstream replacement semantics, a hash-bound historical
  artifact, exact unchanged selection evidence, and exact divergent-file
  coverage. Evidence cannot claim upstream provenance or replace the source lock
  ([compatibility validation](../../internal/packsync/compatibility.go#L107-L191),
  [selection validation](../../internal/packsync/compatibility.go#L210-L257)).
- The current accepted-contract registry is hard-coded for one Pack's `1.0.0`
  to `2.0.0` transition
  ([registry](../../internal/packsync/compatibility.go#L62-L83)). This is evidence
  of an enforced historical migration pattern, not yet a general Addy policy.

## Manual workflow constraints

- ADR 0009 assigns dispatch admission, phase sequencing, failure/retry policy,
  publication ownership, and exact-identity readiness to
  `internal/packsyncworkflow`; candidate admission, floors, version selection,
  evidence validation, Apply, and Recover stay in their owning domain modules
  ([ADR 0009](../adr/0009-own-manual-synchronization-orchestration.md#L22-L46)).
- The canonical operation is Inspect → Classify → Validate → Publish. Inspect
  seals identity without copying upstream bytes into its artifact; Validate
  reacquires and applies the exact candidate in a disposable checkout; Publish
  reacquires again and reruns Packy-owned validation before any write
  ([workflow phases](../../workflows/pack-source-synchronization.md#L71-L131)).
- A dispatch may select latest stable, an exact published prerelease tag, or an
  exact commit. Branches, floating refs, arbitrary tags, and versions without an
  exact prerelease tag are rejected; Pack IDs are not dispatch inputs
  ([request normalization](../../.agents/skills/sync-pack-source/REQUESTS.md#L39-L63)).
- Publication readiness is bound to exact plan, base, candidate, provenance,
  result tree, head, and PR state. Any later change invalidates readiness and
  requires a fresh Inspect; readiness is never patched forward
  ([decision readiness](../../workflows/pack-source-synchronization.md#L190-L211)).
  Auto-merge is disabled and merge remains a manual maintainer decision
  ([ADR 0009](../adr/0009-own-manual-synchronization-orchestration.md#L48-L53)).

## Unresolved policy seams exposed by the evidence

The final Wayfinder decision still needs to resolve the following without
collapsing the identity/version namespaces above:

1. the canonical configured `source_id` and how the independent Addy source is
   represented alongside the current singular production lock;
2. whether ordinary Addy refresh intent tracks `latest-stable`, while exact
   initial/retry/human-evidence operations use the already-supported pinned
   selectors;
3. the initial Packy `addy` Pack version, which current evidence does not derive
   from upstream `0.6.4` or host-manifest `1.0.0`;
4. which observable changes are patch/minor/major within the already-decided
   atomic 24-skill/four-agent/eight-workflow contract, including new mandatory
   actions or changed declared degradation;
5. how future major Addy migrations obtain durable historical artifacts and
   admitted compatibility evidence without conflating Packy-owned migration
   semantics with upstream provenance; and
6. which required schema changes belong to a new immutable Pack Source suite
   version versus Pack manifest/schema evolution outside that five-artifact
   suite.

## Evidence limitations and risks

- This is static repository evidence. It does not prove runtime host behavior,
  and ADR 0005 requires fresh host observation for readiness rather than
  filesystem inference ([ADR 0005](../adr/0005-capability-pack-surface-adapter.md#L34-L39)).
- The pinned Addy inventory records time-bound GitHub release observations; its
  commit-pinned file citations are immutable, but live release state may change
  ([inventory limitations](addy-upstream-inventory.md#L246-L253)).
- Current Pack Source code and tests are centered on a single production lock.
  Treating the existing multi-source config parser as proof of independent
  multi-source transactional support would exceed the evidence.
- No upstream hook, command, installer, validator, eval, helper, test, or CI
  workflow was executed while producing this note.
