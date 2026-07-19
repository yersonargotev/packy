# Addy execution-ready integration specification

## Purpose and authority

This specification is the implementation handoff for the `addy` capability
pack. It assembles the decisions made by the Wayfinder map
[Chart the Addy capability-pack integration](https://github.com/yersonargotev/packy/issues/71)
without reopening or weakening them. When this document is shorter than a
linked decision asset, the linked asset remains authoritative.

The integration is accepted only when the complete 24-row blocking matrix in
[Addy validation and acceptance matrix](addy-validation-acceptance-matrix.md)
is green. A red row is implementation work, not permission for a bootstrap
exception.

## Immutable decision inputs

| Decision | Contract carried into implementation |
| --- | --- |
| [Upstream inventory](addy-upstream-inventory.md) | The inspected basis is release `0.6.4`, commit `98967c45a42b88d6b8fb3a88b7ff6273920763d6`, tree `3808d3bac44683c5af8979a169b31cb99af47de8`; acquisition and validation execute none of its content. |
| [Capability mapping](addy-capability-mapping.md) | Skills are the only direct current fit. Agents, commands, dependency assets, notices, surface bindings, declared degradation, and readiness need explicit Packy contracts. |
| [Observable contract](addy-observable-contract.md) | Addy `1.0.0` is one dependency-closed graph of 24 skills, four agents, eight workflows, seven shared references, support files, and MIT/source attribution on both Codex and OpenCode. |
| [Source identity and versioning](addy-source-versioning-policy.md) | Source intent is latest stable, every admitted candidate is exact, Pack SemVer follows observable behavior, and every proposal uses Inspect -> Classify -> Validate -> Publish without bypass. |
| [Naming and composition](addy-naming-composition-policy.md) | Portable identities are Addy-namespaced; conflict-free upstream names are preserved; collisions fail closed unless the user approves a surface-local alias; sharing is explicit and contributor-tracked. |
| [Activation and readiness](addy-activation-readiness-behavior.md) | Preview discloses the complete contract and prompt authority before one `reversible-local` receipt; readiness is fresh `yes`/`no`/`unknown`; pending actions, optional modes, and exclusions remain distinct. |
| [Multi-source provenance](addy-multi-source-provenance-policy.md) | Each configured source has one canonical lock and complete contribution, while `internal/bundletransaction` retains one global transaction and recovery boundary. |
| [Acceptance matrix](addy-validation-acceptance-matrix.md) | Five sequential gates and 24 rows are blocking, reproducible, Packy-owned, sandboxed, surface-specific where applicable, and include negative twins. |

Accepted ADRs continue to govern module ownership:

- [ADR 0003](../adr/0003-core-lifecycle-deep-module.md): capability-pack
  lifecycle stays in `internal/capabilitypack`; CLI is an adapter.
- [ADR 0005](../adr/0005-capability-pack-surface-adapter.md): one complete
  adapter per host; capability-pack owns lifecycle meaning and adapters own
  host translation and observation.
- [ADR 0006](../adr/0006-own-workstation-layout-by-domain.md): host modules
  derive their paths from injected workstation facts, never ambient HOME/XDG.
- [ADR 0007](../adr/0007-serialize-complete-bundle-transactions.md): one
  complete-bundle transaction and recovery owner.
- [ADR 0008](../adr/0008-orchestrate-classification-outside-packsync.md):
  classification proposes evidence without acquiring version, mutation, or
  recovery authority.
- [ADR 0009](../adr/0009-own-manual-synchronization-orchestration.md):
  `internal/packsyncworkflow` owns manual operation policy while
  `internal/packsync`, `internal/packclassification`, and
  `internal/bundletransaction` retain their existing authorities.
- [ADR 0011](../adr/0011-publish-versioned-pack-source-schema-suite.md): a
  Pack Source suite is complete, immutable, exact-version selected, published
  once, and resolved offline during repository validation.
- [ADR 0012](../adr/0012-adopt-source-scoped-pack-source-provenance.md): one
  canonical lock per source and target/set digests refine ADR 0007 without
  changing its complete-bundle transaction or recovery boundary.

## Exact first contract

### Portable inventory

The initial current manifest is Pack `addy` version `1.0.0`. It must describe
exactly:

- 24 `skill` resources, including every skill-local support file;
- four `agent` resources;
- eight logical `command` resources, one per workflow rather than one per
  upstream host projection file;
- seven dependency `asset` resources for the shared references, with explicit
  consumer edges;
- the required `idea-refine.sh` helper as inert content of its owning skill,
  preserving its safe file mode but creating no execution action; and
- MIT notice and attribution bytes/display metadata represented by a
  non-projecting `notice` intent, while repository identity, exact candidate
  identity, and selected-content digests remain exclusively in source
  provenance locks and workflow artifacts.

Portable resource identity is `(pack_id, kind, id)`. Logical capability
identity, source ownership, host binding, local alias, observed occupancy, and
projection ownership are separate facts and must not be inferred from a path
or spelling.

Pack manifest schema version 1 keeps its current immutable meaning for existing
current and historical Packs. Addy uses a new manifest schema version 2; a v1
reader remains only to preserve existing Packs and history, not as a fallback
for v2 data. Producers must never encode new resource kinds or fields under
schema version 1.

### Manifest v2 wire contract

Manifest v2 retains the v1 top-level `id`, `version`, `provides`, `requires`,
`conflicts`, and `resources` fields and adds a required `contract` object.
Decoders reject unknown fields, null required arrays, duplicate identities,
unsorted producer output, escaping paths, dangling references, and a binding
for a surface the Pack does not declare. Producers canonicalize every identity
array by `(kind, id)` and every binding array by `surface`.

The discriminated resource shapes are exact:

| Kind | Required fields after `kind`, `id` | Meaning |
| --- | --- | --- |
| `skill` | `source`, `requires`, `bindings` | `source` is one closed bundle-relative tree; `requires` names asset identities; bindings are native on both surfaces. |
| `agent` | `source`, `description`, `mode`, `tools`, `permissions`, `requires`, `bindings` | `source` is the persona prompt; `mode` is `primary` or `subagent`; tool IDs and prompt-authority permissions are sorted portable sets; `requires` names skill/asset identities. |
| `command` | `source`, `arguments`, `requires`, `bindings` | One logical workflow prompt; `arguments` declares its portable input contract; `requires` names skill/agent/asset identities; bindings express the intentional host asymmetry. |
| `asset` | `source`, `requires` | Non-projecting dependency content. `requires` may name only other assets and must remain acyclic. |
| `notice` | `source`, `license`, `attribution`, `requires` | Bundle-only metadata; `license` is `MIT` for Addy, `requires` is empty, and no surface binding is legal. |

Every `requires` item is the canonical string `<kind>:<id>`. Dependencies must
exist in the same Pack, may not target `notice`, and form an acyclic graph.
Adapters receive the transitive, deduplicated, path-safe source closure rather
than rediscovering Markdown references.

Each surface binding is:

```json
{
  "surface": "codex | opencode",
  "projection": "skill | agent | command",
  "name": "requested host-visible name",
  "invocation": "user-visible invocation",
  "mode": "native | degraded",
  "degradation": "required when mode is degraded; absent otherwise",
  "sharing": "exclusive | shared"
}
```

For Addy commands, Codex uses `projection: "skill"`, `invocation: "$<name>"`,
`mode: "degraded"`, and degradation identity
`codex-command-as-workflow-skill`. OpenCode uses `projection: "command"`,
`invocation: "/<name>"`, and `mode: "native"`. Skill and agent bindings are
native and use the host's documented invocation form. A resource with bindings
must have exactly one binding for each Pack surface; assets and notices have
none. `sharing` is required and defaults are forbidden: `exclusive` rejects a
second contributor even when bytes match; `shared` permits composition only
when every contributor declares `shared` and normalized projection plus
rendered behavior are identical. Changing sharing is observable compatibility
input and is never inferred from existing ownership.

`command.arguments` is exactly
`{"mode":"none"}` or
`{"mode":"freeform","placeholder":"$ARGUMENTS"}`. The placeholder is
portable logical prompt input; adapters translate host syntax without deleting
or narrowing it. `agent.tools` contains portable tool IDs and
`agent.permissions` uses the same authority vocabulary as `optional_modes`.
An adapter may report an unavailable optional tool or permission mode, but it
cannot silently remove the persona instruction or claim a weaker agent as the
same binding.

The required `contract` object is:

```json
{
  "exclusions": [
    {"id": "...", "source_paths": ["..."], "reason": "..."}
  ],
  "optional_modes": [
    {
      "id": "...",
      "authorities": ["filesystem | process | network | browser | subagent | package-manager | commit | deploy"],
      "fallback": "coherent fallback description or none"
    }
  ]
}
```

Exclusion paths are relative to the exact acquired snapshot and cannot overlap
a selected runtime resource. Authorities are a sorted set. `fallback: "none"`
means the affected invocation fails when the mode is unavailable; any other
value is user-visible contract text and therefore participates in Pack
compatibility.

Source bindings retain one `upstream_path` per resource. The seven shared
references are seven separately owned `asset` bindings rather than implicit
paths attached to a skill. Exact source identity remains in the source lock;
the `notice` resource owns only selected notice bytes and display metadata, so
it is retained, validated, and shown without ever becoming host configuration.

### Surface bindings

Each required logical resource carries an explicit binding for every declared
surface. The binding records logical identity, projection kind, native or
degraded status, requested invocation/name, and sharing intent. The rendered
source set is derived from the portable dependency closure. Loading,
authorization, and usability requirements follow from the projection kind and
host contract; adapters observe that evidence freshly rather than accepting a
producer-authored readiness claim.

| Capability | Codex binding | OpenCode binding |
| --- | --- | --- |
| Skill | Native `$name` skill projection | Native skill projection |
| Agent | Native custom-agent projection | Native agent projection |
| Command | Required same-named workflow-skill degradation invoked as `$name`; never claim `/name` | Native `/name` command |
| Dependency asset | Materialized only through the declared consumers' closed source sets | Same |
| Notice | Retained in the bundle; no host projection | Same |

The command degradation preserves logical name, prompt behavior, arguments,
agent/skill composition, and an explicit user-visible explanation of the
syntax difference. A missing or weaker fallback blocks Codex rather than
silently reducing the contract.

### Exclusions and invocation-time modes

The four upstream hooks, root maintainer guidance, host manifests and setup
descriptors, projection symlinks, evals, validators, tests, CI, and fixtures
create no runtime resource or projection action. Selected items may be retained
only as inert evidence. No `hook` resource kind or hook activation path is
needed for Addy `1.0.0`; introducing one is a future contract decision.

Browser/network access, package managers, subagents, and privileged
commit/push/deploy effects are declared invocation-time modes. Their absence
does not block base activation. It affects only an invocation without a
coherent fallback, and the unavailable authority must remain observable.

## Source and provenance contract

### Clean topology migration

Before Addy registration, move the current singular lock to:

```text
bundle/sources/mattpocock-skills.lock.json
```

The migration is one clean cut. It updates every producer, consumer,
validator, fixture, workflow artifact, and document; publishes the next
complete immutable Pack Source schema suite; proves the candidate, snapshot,
selected resources, modes, bytes, and digests unchanged; and deletes
`bundle/sources.lock.json`. There is no legacy reader, fallback, or dual write.

The topology and new required digest fields are incompatible with workflow
artifact instances using schema version 1, so the next Pack Source suite is
`v2.0.0` with instance `schema_version: 2`. The five v1 documents remain
immutable and available. The v2 suite is checked in and published as one
complete five-document suite; all v2 producers, fixtures, offline validators,
and structural tests adopt it together.

### Source ownership and freshness

For every committed bundle generation:

- configured source IDs are path-safe and have an exact bijection with
  canonical `bundle/sources/<source-id>.lock.json` documents;
- a lock owns one exact candidate and the source's complete contribution across
  all affected Packs;
- each `(pack_id, kind, resource_id)` binding has exactly one source owner;
- the ordered `(source_id, canonical_lock_digest)` sequence deterministically
  produces `lock_set_sha256`, without a persisted aggregate index; and
- plans and workflow artifacts seal both the target `source_lock_sha256` and
  complete `lock_set_sha256` alongside existing configuration, manifest, base,
  result-tree, and publication preconditions.

Registration, removal, or an exceptional ownership transfer is one explicit
complete-bundle change. Any bundle generation change invalidates every older
proposal, including one for an otherwise unchanged source. The stale source
restarts Inspect -> Classify -> Validate -> Publish; it is never rebased and
its evidence is never patched forward.

`internal/bundletransaction` remains the single lock, staging, swap, and
recovery owner. Per-source locks do not create per-source mutations or recovery
markers.

## Capability-pack and adapter contract

### Portable model and composition

`internal/capabilitypack` owns:

- parsing and validating manifest v2 and the new resource intents;
- dependency closure and exact logical inventory;
- per-surface desired bindings and declared degradation;
- explicit sharing declarations and complete contributor sets;
- collision, alias, lifecycle, ownership, blocker, consent, readiness, and
  recovery meaning; and
- contract diffs, mandatory actions, migrations, and Pack compatibility facts.

Composition compares portable identities and normalized rendered behavior. It
never treats equal bytes as shared ownership. A shared projection is legal only
when every contributor opts in, rendered behavior is identical, and ownership
records all contributors. Removal preserves a projection while any contributor
still requires it.

A surface-local alias is desired activation intent attached to a portable
logical identity. It is not Pack content, source configuration, provenance, or
Pack version. The default alias spelling is `addy-<upstream-name>` in the
surface's native syntax. Every preview and Apply freshly revalidates the alias;
removed identities and secondary collisions require an explicit migration.

The minimal CLI input is a repeatable
`--alias <kind>:<logical-id>=<host-name>` flag on `pack activate`, `pack update`,
and targeted `pack reconcile`. `--surface` supplies the surface scope. The CLI
parses syntax only; capability-pack validates the portable identity, persists
the desired alias in activation intent, and seals it into the preview. Apply
cannot accept an alias that was not in that preview. Deactivation accepts no
new alias and removes only through recorded intent and exact ownership.

Alias persistence moves the capability-pack activation document to schema
version 3 and each contained activation state to schema version 2. Every v2
`ActivationIntent` has a required canonical `aliases` array of:

```json
{"kind": "skill | agent | command", "id": "logical-id", "name": "host-name"}
```

The surface remains on the enclosing intent; `(kind, id)` is unique within it.
The existing document v1/v2 readers migrate to document v3 with empty aliases
and write only v3. That reader is a user-state migration, not a fallback for
source-lock or manifest v2 data.

### Host adapters

`internal/codex` and `internal/opencode` each implement the complete
`capabilitypack.SurfaceAdapter`. Inspection is pure and returns canonical:

- all normalized occupied names for every relevant projection kind, including
  reserved/native, unmanaged, Packy-owned, and active-pack names;
- desired and observed fingerprints and contributor ownership;
- authorization, loading/usability evidence, and pending human actions; and
- one revision covering every fact used to seal the plan.

Normalized occupancy is represented once per adapter observation as
`(namespace, normalized_name, owner_type, owner_id, fingerprint)`, where
`owner_type` is `reserved`, `unmanaged`, or `packy`; `owner_id` is required for
`packy`, absent for `reserved`, and host-derived when known for `unmanaged`.
Desired bindings carry the same `(namespace, normalized_name)` collision key.
Adapters define which projection kinds share a namespace, but capability-pack
alone compares keys, explains both owners, applies alias policy, and seals the
result. Duplicate occupancy records or inconsistent owner/fingerprint facts
invalidate the complete observation.

Application is the only adapter mutation boundary and applies exactly the
sealed projection actions. Adapters translate syntax, paths, and host evidence;
they do not decide alias policy, sharing, lifecycle disposition, consent, or
whether a degradation is acceptable.

An unresolved required collision blocks the complete affected surface, names
both owners, and performs no write. It never picks precedence, overwrites,
adopts matching unmanaged content, or silently renames. A failure on one
surface does not alter the other surface's authorized state.

### CLI and readiness

Human and JSON preview/status/update output are two renderings of the same
structured domain result. They include:

- exact logical counts and dependency closure;
- each native or degraded surface binding and invocation;
- explicit exclusions and optional invocation-time modes;
- filesystem, process, browser/network, subagent, package-manager, and
  privileged commit/deploy prompt authority;
- collisions, owners, selected aliases, contributor sets, pending human
  actions, contract diffs, migrations, and mandatory actions; and
- independent `configured`, `authorized`, and `usable` values of `yes`, `no`,
  or `unknown`.

Dry-run is mutation-free and asks for no approval. Apply accepts one exact
`reversible-local` receipt only after authority disclosure. The receipt grants
only the sealed local projection actions and never pre-authorizes later
workflow effects. Destructive removal continues to require its separate exact
consent.

Files can establish only `configured`. Authorization and usability require
fresh host evidence bound to the current plan, projection, and host revisions.
If a host has no reliable usability probe, `usable=unknown`; the human renderer
must not collapse it to `no`. `--require usable` fails for both `no` and
`unknown`.

## Manual admission of Addy

The repository must not hand-edit acquired Addy bytes into the bundle. Source
registration is a new mode of the same sealed Check/Apply and manual workflow,
not a prerequisite commit or bootstrap writer.

The v2 dispatch request has mutually exclusive operations:

- `operation: "synchronize"` requires an already configured `source_id` and
  forbids `registration`; or
- `operation: "register"` requires `source_id` to be absent from committed
  configuration and carries one strict `registration` object identical to a
  complete `SourceConfig` (`id`, `provider`, `repository`, `selector`, and all
  configured resource bindings). Its `id` must equal `source_id`.

For registration, Inspect validates the proposed configuration and exclusive
binding ownership without persisting it, resolves and inventories the exact
candidate, and seals the complete registration object and SHA-256 into the
ordinary plan. Classify receives that plan. Validate and Publish independently
reacquire the candidate and use ordinary Apply to add configuration, the first
source lock, selected resources, affected Pack/history/evidence, and derived
digests to their disposable complete bundle. Publish's PR contains that atomic
result. If the source or any binding appears before Apply, or the sealed
registration digest changes, the plan is stale with zero mutation.

After all implementation prerequisites and Packy-owned acceptance fixtures are
green:

1. Dispatch `operation: "register"` for source `addy`, repository
   `addyosmani/agent-skills`, stable-release intent, and the complete configured
   contribution.
2. **Inspect** resolves one exact candidate and seals registration,
   source/bundle provenance, inventory, mechanical compatibility floor, base,
   and preconditions.
3. **Classify** supplies complete human or AI-proposed/human-admitted evidence
   for every affected Pack and both surfaces.
4. **Validate** independently reacquires the exact candidate in disposable
   roots, executes no upstream content, applies the complete candidate, and
   runs all Packy-owned gates.
5. **Publish** reacquires it again, reproduces the exact result tree and diff,
   reruns validation, freshly reobserves repository/branch/PR and provenance
   state, and creates at most one non-draft, auto-merge-disabled decision-ready
   PR on `sync/addy`.

If latest stable no longer equals the inspected `0.6.4` basis, the workflow
must freshly inventory and classify the exact candidate. It may proceed only
when the candidate still satisfies the fixed `1.0.0` observable contract or a
separately accepted contract/version decision exists. It must not silently
rewrite the 24/4/8 contract.

Merge remains a maintainer decision. Initial registration can succeed only by
producing the decision-ready PR because the source is absent at its starting
state. A schema-valid no-op remains valid only for later `synchronize`
operations on an already configured source. Neither outcome activates Addy in
a real operator home or publishes a Packy release.

### Canonical provenance and artifact fields

`source_lock_sha256` is SHA-256 of the exact canonical checked-in source-lock
bytes, including their single trailing LF. `lock_set_sha256` is SHA-256 of the
UTF-8 concatenation, in ascending source-ID order, of:

```text
<source-id> NUL <64-lowercase-hex source_lock_sha256> LF
```

No bytes for that ordered sequence are persisted as an index. Producers sort
lock resources by `(pack_id, kind, resource_id)` before marshaling and use the
repository's two-space-indented JSON plus one LF representation.

The v2 workflow artifacts carry provenance as follows:

| Artifact | Required v2 provenance fields |
| --- | --- |
| Dispatch | `operation`; `registration` and `registration_sha256` only for registration. A dispatch is intent, not post-Inspect provenance evidence. |
| No-op | `source_lock_sha256`, `lock_set_sha256`, `config_sha256`, and `manifests_sha256`. |
| Operational failure | The same four fields are required whenever `plan_id` is present and absent only for failures before a plan/provenance observation exists. |
| Validation | The same four fields plus `result_tree_sha`; all bind the independently reacquired candidate. |
| Publication | The same four fields plus `result_tree_sha`, `head_sha`, and managed PR-state digest. |

The sealed `packsync.Plan`, classification requests/evidence, review brief, and
recovery marker also carry the target/set digests. The recovery marker uses
them for diagnosis and stale-plan proof only; recovery authority remains its
phase and complete old/new tree hashes under ADR 0007 and ADR 0012.

## Reproducible acceptance

Every acceptance case uses Packy-owned canonical fixtures, disposable
repository/acquisition/state roots, sandboxed `HOME` and `XDG_CONFIG_HOME`, a
stable structured oracle, exact allowed-diff or zero-mutation proof, one-fact
negative twin, and deterministic rerun. Upstream code and prompts are never the
test authority and are never executed by acquisition or validation.

The implementation issues below retain the exact row definitions in
[the acceptance matrix](addy-validation-acceptance-matrix.md). Row ownership
indicates the slice that must turn an implementation target or prerequisite
green; proven-existing rows still receive Addy fixtures or regression coverage
where the matrix requires them.

## Tracer-bullet delivery graph

The slices are intentionally ordered around observable end-to-end proofs rather
than package-layer batches. Each issue must keep `go test ./...` green and run
`./scripts/validate-packy.sh` before reporting success.

| Slice | Smallest independently verifiable outcome | Matrix ownership | Blocked by |
| --- | --- | --- | --- |
| A. Publish the next immutable Pack Source and manifest contracts | The v2 five-schema suite, manifest v2 decoders/types, canonicalization rules, registration dispatch, provenance fields, and offline producer/validator parity are executable while every v1 byte and meaning remains unchanged. | Contributes to 3-9 and 24; closes no row alone | None |
| B. Migrate Pack Sources to source-scoped provenance | The unchanged `mattpocock-skills` source completes Check/Apply/Recover and workflow artifact validation through one canonical lock and target/set digests; singular topology is gone. | Closes 3 and 5; contributes to 24 | A |
| C. Carry one Addy workflow through the portable model | A synthetic manifest v2 round-trips one skill-agent-command-asset-notice workflow through source binding, lock, catalog, history, composition, classification, and validation, with exclusions/modes/degradation, alias intent, and negative twins. | Contributes to 4 and 6-10; closes no Addy inventory row | B |
| D. Project the Addy contract coherently on Codex | The synthetic workflow plans/applies/verifies a native skill, native agent, `$name` degraded workflow, closed assets, display-only notice, collision, alias, ownership, readiness, and recovery without claiming `/name`. | Contributes the Codex cohort to 11-20 | C |
| E. Project the Addy contract coherently on OpenCode | The same portable workflow plans/applies/verifies a native skill, native agent, native `/name` command, closed assets, display-only notice, collision, alias, ownership, readiness, and recovery. | Contributes the OpenCode cohort to 11-20 | C |
| F. Deliver the shared lifecycle and tri-state CLI contract | Human/JSON preview, consent, alias input, status, update, failure, recovery, removal, and `--require usable` expose identical structured facts across both adapters, including dual-surface isolation. | Contributes shared behavior to 11 and 14-22 | D, E |
| G. Build and close the complete Addy acceptance cohort | Packy-owned fixtures encode the exact 24/4/8/7 inventory, support files, exclusions, notices, supported version history, both surfaces, negative twins, and deterministic oracles without executing upstream content. | Closes 1, 2, 4, and 6-22 | F |
| H. Admit Addy through the manual Pack Source workflow | Registration is sealed inside Inspect and the double-reacquired exact candidate yields one decision-ready `sync/addy` PR containing the complete Addy `1.0.0` generation; later synchronization retains the exact no-op path. | Closes 23 and 24 | G |
| I. Accept the exact Addy bundle generation | The candidate PR is reviewed and merged only if all 24 rows are already green; the merged SHA is revalidated with no activation or Packy release side effect. | Audits all rows; closes none | H |

Native blocking edges are therefore:

```text
A -> B
B -> C
C -> D
C -> E
D -> F
E -> F
F -> G
G -> H
H -> I
```

D and E are the only planned parallel frontier. The host slices own disjoint
translation modules but must not independently alter shared capability-pack
policy; any shared contract change belongs in C or F.

## File and module impact guide

This is an impact guide, not permission to move ownership between modules.

- `bundle/sources.json`, `bundle/sources.lock.json`, and
  `bundle/sources/<source-id>.lock.json`: source registration and provenance
  topology.
- `schemas/pack-source/v2.0.0/`, schema validators, Pages verification, and
  workflow fixtures: immutable artifact suite and offline parity.
- `internal/packsync`, `internal/packsync/githubsource`,
  `internal/packclassification`, `internal/packsyncworkflow`, and
  `internal/tools/syncpacksource`: exact candidate, source contribution,
  target/set digests, classification, and manual workflow admission.
- `internal/bundletransaction`: unchanged complete-bundle mutation and recovery
  authority; adjust only the sealed provenance inputs needed by the new
  topology.
- `internal/capabilitypack/catalog.go`, `composition.go`, `activation.go`,
  `status.go`, history and their tests: manifest v2, logical contracts,
  bindings, aliases, sharing, lifecycle, and readiness.
- `internal/codex` and `internal/opencode`: complete host inspection,
  translation, application, and fresh readiness evidence.
- `internal/cli/pack.go` and CLI tests: structured rendering, alias input,
  consent, tri-state status, failure, update, recovery, and readiness gates.
- `bundle/packs/addy/**` and `bundle/history/addy/**`: produced only by the
  admitted synchronization result, never by an installer or ad hoc copy.
- `scripts/validate-packy.sh` and `internal/ci`: repository validation authority,
  suite structure, inertness, and acceptance orchestration.

## Decision-closure audit

No product, architecture, safety, compatibility, naming, lifecycle,
provenance, validation, or delivery-order decision remains before slice A can
start.

The following are execution observations, not open decisions:

- the exact stable Addy candidate is re-resolved at Inspect time;
- current red acceptance rows name their owner and blocking predecessor above;
- Codex and OpenCode may report `usable=unknown` until a fresh host signal
  exists; that is the decided truthful result, not missing scope;
- hooks and real-operator activation remain outside Addy `1.0.0`; and
- Packy release publication is not required to merge the Addy bundle
  generation and would need its own request.

If implementation discovers a fact that contradicts a fixed decision, it must
stop and open a new decision ticket. It must not reinterpret this specification
or weaken an acceptance row locally.
