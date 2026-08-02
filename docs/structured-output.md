# Structured CLI output

Packy emits versioned JSON when `--json` is present. Doctor retains
`schema_version: 2`. Pack show uses `schema_version: 4`.
Capability-Pack lifecycle uses `schema_version: 8`, adding structured
`recovery_guidance` to the v7 closure-bound sensitive-effect plan. The guidance
names the originating operation, affected resources, consumers, truthful
completed/failed/not-started effects, and the next explicit lifecycle command.
Capability-Pack status uses `schema_version: 7`, adding the explicit
`lifecycle_state` values `active`, `inactive-clean`,
`inactive-with-residuals`, and `recovery-required` to the v6 activation-role
and canonical-consumer contract. Earlier versions remain unchanged.

| Command | `report` |
| --- | --- |
| `packy doctor --json` | `doctor` |
| `packy pack show PACK --json` | `pack-show` |
| Pack activate/update/deactivate/reconcile preview | `pack-lifecycle-preview` |
| successful Pack apply | `pack-lifecycle-apply` |
| completed structured Pack failure | `pack-lifecycle-failure` |
| `packy pack status --json` | `pack-status-overview` |
| targeted `packy pack status PACK --surface SURFACE --json` | `pack-status` |

The exact offline schemas are under `schemas/cli/v2/` through
`schemas/cli/v8/`.
Canonical redacted fixtures use the matching directories under
`internal/cli/testdata/structured-output/`; repository validation compiles the
schema selected by each document's `schema_version` and validates both the
fixtures and live producer examples.

## Doctor

Doctor contains ordered `checks` and a `summary`. It reports Packy core
availability through the `packy-core` check. Detailed capability-pack
configured, authorized, and usable evidence remains available through
`packy pack status`.

Each check uses `PASS`, `WARN`, or `FAIL`. Warnings exit zero. A complete report
containing a failure is written before `ErrDoctorUnhealthy` produces the
nonzero process exit. Workstation context is not part of doctor JSON.

## Capability packs

`pack-show` publishes declared surfaces and, per surface, compatibility,
bindings, exclusions, optional modes, prompt authorities, and aliases. For the
current catalog it reports `matty` 3.0.0 as complete and `engram` 2.0.0 as
degraded on that surface, with the optional `lifecycle:engram-memory`
exclusion. It
does not claim observed readiness.

Lifecycle preview publishes the sealed ordered phases and actions, contract,
compatibility, consent, preservation, blockers, expected readiness, observed
evidence, pending evidence, evaluated runtime modes, recovery, and exact
`all`/`custom` resource selection. Its ordered `capability_requirements` identify
the consumer Pack/resource, capability, provider Pack/resource, required tools
and authorities, and resulting readiness. A null resource denotes legacy
whole-Pack composition. Its ordered `sensitive_effects` bind each selected
resource's manifest-declared authorities and runtime effects to the owning Pack
and every exact introducing root-to-resource dependency chain, including
required provider Pack closures. Apply and failure reports retain that redacted,
sealed plan. Recovery previews additionally publish `recovery_guidance`; they
never encode replay authority for the historical plan.
Status publishes intent, canonical selected roots, and a full
resource inventory whose role is `root`, `dependency`, `asset`, `notice`, or
`unselected`, with a deterministic root-to-resource `dependency_chain`.
Its lifecycle state distinguishes a clean inactive intent from retained
Packy-owned residual projections and from an interrupted attempt that requires
recovery.
Lifecycle preview publishes the selected closure graph, including associated
notices; show publishes every resource's portable `requires` and `notices`
edges. Status also publishes projection health, compatibility, readiness for
each selected root and dependency, focused `--resource` status and
`--require usable` results, evaluated runtime modes, evidence, and pending
actions. Each runtime-mode result
keeps its declared role,
requirements, authorities, effects, fallback, fail-before-effects policy, and
sanitized observation facts together; unobservable facts remain `unverified`
and do not reduce whole-pack readiness. Each readiness dimension remains
explicitly `{state:"known",value:true|false}` or
`{state:"unknown",value:null}`. Projection contributors use canonical
`pack:<pack>:<kind>:<resource>` identities. `--require usable` writes the
complete status report before preserving the Pack- or focused-resource
readiness-gate exit result.

## Ordering and redaction

Arrays representing sets use their schema-defined canonical order (lexical
unless the owning contract defines another order); examples include surfaces,
capabilities, blockers, evidence, aliases, bindings, exclusions, and
contributors. Arrays representing work retain sealed execution order; examples
include lifecycle phases, actions, completed effects, and not-started effects.
Checks retain diagnostic order, and status entries are ordered by pack then
surface.

Reports never include action payload content, raw mixed-store documents,
authentication material, or MCP environment values. Environment-bearing command
arguments preserve the key and replace the value with `<redacted>`. Human output
uses the same owner-provided facts and stable label order without reconstructing
compatibility, readiness, recovery, or version policy in `internal/cli`.
