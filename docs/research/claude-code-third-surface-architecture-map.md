# Claude Code third-surface architecture map

Research date: 2026-07-20

## Question and boundary

Where does Packy's current architecture assume exactly the Codex and OpenCode
CLI surfaces, and which owners would have to participate to add Claude Code as
a third surface without moving domain policy into `internal/cli` or weakening
the accepted deep-module boundaries?

This is an impact map, not an implementation specification. It does not choose
Claude Code's final projection contract, classic lifecycle behavior, Pack
resource behavior, compatibility gates, or operator experience. Those remain
decisions on the parent Wayfinder map.

The map follows the accepted ownership decisions in
[ADR 0003](../adr/0003-core-lifecycle-deep-module.md),
[ADR 0004](../adr/0004-setup-health-deep-module.md),
[ADR 0005](../adr/0005-capability-pack-surface-adapter.md), and
[ADR 0006](../adr/0006-own-workstation-layout-by-domain.md).

## Architectural answer

Packy already has the right capability-pack extension seam: every supported
host implements one complete `capabilitypack.SurfaceAdapter`, while
`capabilitypack` owns lifecycle meaning, composition, consent, ownership,
readiness, plan sealing, recovery, and verification. Adding Claude Code should
therefore extend the host catalog and composition root rather than generalize
host syntax into capability-pack or the CLI.

Classic lifecycle and setup health are intentionally concrete compositions,
not host registries. They must acquire explicit Claude Code participation and
state/health semantics rather than being replaced with speculative generic
frameworks. A future Claude Code host module is the natural owner of Claude
paths, syntax, projections, collisions, fingerprints, detached observations,
and authorized application, but the exact public seam is left for
**Decide the Claude Code host integration contract**.

The principal implementation path is therefore:

```text
CLI composition root
├── corelifecycle facade ── explicit classic Claude actions/state/ownership
├── setuphealth Diagnose ── detached Claude host observations
└── capabilitypack facade
    └── complete Claude SurfaceAdapter
        ├── pure inspection
        └── authorized projection application
```

## Impact map

### 1. Host enumeration and Pack catalog

| Current seam | Two-surface assumption | Required third-surface impact |
|---|---|---|
| `internal/capabilitypack/catalog.go` | `SurfaceCodex` and `SurfaceOpenCode` are the complete surface vocabulary; initial catalog entries and binding validation encode that set. | Add Claude Code to the stable surface vocabulary, catalog composition, binding validation, and any surface-specific degradation rules. Keep portable Pack resource semantics here; keep Claude schemas and paths out. |
| `internal/capabilitypack/reconcile.go` | Surface-wide reconcile validation enumerates Codex and OpenCode. | Include Claude Code in complete-surface reconciliation and its validation coverage. |
| `bundle/packs/*/pack.json` | Explicit bindings name only existing surfaces where a resource is not generic. | Review every explicit binding. Add Claude bindings only after the resource-projection decision; do not infer support from host capability alone. |

The highest-risk catalog detail is the existing Codex-specific degraded-command
validation in `catalog.go`: Claude Code must not accidentally inherit another
host's exception simply because a switch has a default branch.

### 2. Capability-pack composition and lifecycle

| Current seam | Two-surface assumption | Required third-surface impact |
|---|---|---|
| `internal/cli/pack.go` (`packComposition`, `resolvePackComposition`, `activationFacade`) | The composition root derives two host layouts, constructs two adapters, and registers two surfaces. | Derive Claude layout through its owning module, construct the adapter, and register it. CLI remains wiring only. |
| `internal/capabilitypack/activation.go` | Lifecycle is surface-neutral once an adapter is registered. | No new lifecycle architecture is needed: route Claude through `InspectSurface`, the private gateway, plan/consent/state, `ApplyProjections`, and fresh verification. |
| `internal/capabilitypack/surface_architecture_test.go` | Architecture assertions expect exactly two adapters and prevent host policy from leaking into capability-pack/CLI. | Expand the expected adapter set to three while preserving the same dependency and policy guards. |

Claude-specific merging, precedence, collision detection, readiness evidence,
and filesystem/CLI mutation must live in the host adapter. Portable lifecycle
meaning, deletion authority, destructive consent, composition, blockers, and
recovery must remain in capability-pack.

### 3. Pack resource projection

The official host inventory establishes candidate native mechanisms, not a
completed contract:

| Portable intent | Candidate Claude Code projection | Architectural owner / unresolved point |
|---|---|---|
| `skill` | `~/.claude/skills/<name>/SKILL.md` | Claude host adapter owns layout, link inspection, collision identity, and application. Symlink-backed projection implies the researched 2.1.203 compatibility floor. |
| `instruction` | A uniquely marked Packy block in `~/.claude/CLAUDE.md` | Claude adapter owns block parsing and preservation. The later contract must define duplicate-marker, foreign-edit, ordering, and recovery behavior. |
| `mcp_server` | Official `claude mcp add(-json) --scope user` commands | Claude adapter owns translation and exact-definition ownership. It must not rewrite mixed-content `~/.claude.json`; inert preview must not use inspection that may launch the server. |
| `lifecycle` | Select command-hook entries in `~/.claude/settings.json` | No automatic generic mapping is safe. The resource decision must define event-specific semantics, trust/consent, handler identity, and explicit exclusions. |

These constraints come from
[Claude Code's official global integration surface](./claude-code-global-integration-surface.md).
They expose, but do not settle, the contract questions assigned to later
Wayfinder tickets.

### 4. Classic install, update, uninstall, and recovery

| Current seam | Two-surface assumption | Required third-surface impact |
|---|---|---|
| `internal/corelifecycle/install.go` | `FacadeConfig` carries concrete Codex/OpenCode layouts; install planning emits their setup and prompt actions explicitly. | Add explicit Claude layout/observation input and action planning only after classic behavior is decided. Claude actions must remain structured lifecycle actions, not CLI helpers. |
| `internal/corelifecycle/uninstall.go` | Inspection and safe removal enumerate Codex/OpenCode-owned artifacts and containers. | Add Claude ownership evidence, conflict classification, safe removal, and recovery handling. Preserve every foreign file and config fragment. |
| `internal/corelifecycle/state.go` | Classic state records expectations for two configured surfaces. | Define a backward-compatible state/schema migration for existing installations before making Claude a required golden-path surface. Do not silently reinterpret old state as proof of Claude ownership. |
| `internal/cli/root.go` | The composition root supplies the two host layouts to lifecycle. | Wire the Claude owner into the facade; do not derive `~/.claude` paths in CLI. |

The current Engram setup sequence explicitly targets Codex and OpenCode.
Whether an official, supported Engram command can configure Claude Code must be
verified before any analogous classic action is specified. If no such command
exists, the contract needs another Packy-owned mechanism rather than an
invented `engram setup claude` command.

Changing the golden path from two surfaces to three is a state migration, not
only another install action. Otherwise an existing healthy installation could
be reclassified as unhealthy before the operator has run an update that
creates and records the new projections.

### 5. Setup health and doctor

| Current seam | Two-surface assumption | Required third-surface impact |
|---|---|---|
| `internal/setuphealth/setuphealth.go` | `Diagnose` consumes concrete Codex/OpenCode observations and appends their checks explicitly; Engram expectations name those two surfaces. | Consume a detached Claude observation, define check names/order/severity/remediation, and decide how pre-Claude classic state is classified. |
| `internal/cli/root.go` | CLI constructs two host observations and passes them to setup health. | Construct the Claude observation through its owner and pass it into diagnosis. |
| `internal/cli/setup_health_adapter.go` | Rendering consumes a generic structured report. | No schema architecture change appears necessary; output and golden fixtures will expand once new checks are chosen. |

Setup health must remain read-only. Version discovery may use bounded
executable inspection, but diagnosis must not create Claude files, repair
settings, register MCP servers, start a Claude session, authenticate, or make a
model call.

### 6. CLI commands and rendering

`internal/cli/pack.go` contains several `--surface` help strings that name only
`codex` and `opencode`. Its `surfaceName` helper also treats every value other
than OpenCode as Codex, so adding a third constant without making the mapping
exhaustive would mislabel Claude output.

Pack list/show output and most lifecycle/readiness rendering already consume
surface-neutral structured values. The required work is explicit parsing,
validation, help text, exhaustive names, ordering, and expanded human/JSON
goldens—not a new rendering domain layer.

### 7. Test and validation surface

The implementation specification will need at least these test families:

- focused `internal/claudecode` layout, observation, inspection, projection,
  collision, preservation, and application tests against sandboxed workstation
  facts;
- adapter architecture tests updated from exactly two to exactly three hosts;
- capability-pack gateway, inspection-contract, activation, status,
  reconcile, state, stale-plan, recovery, and readiness scenarios including
  Claude;
- core lifecycle state migration, action planning, ownership conflict,
  uninstall, recovery, and post-apply verification scenarios;
- setup-health semantic tests plus CLI doctor/rendering contracts;
- `internal/cli/pack_test.go`, `root_test.go`, CLI identity-equivalence tests,
  and human/JSON golden fixtures;
- Addy acceptance coverage for every binding actually declared for Claude;
  unsupported resources must have explicit observable exclusions; and
- package-install smoke coverage using sandboxed `HOME` and
  `XDG_CONFIG_HOME`, proving no write reaches the operator's real home.

Both `scripts/validate-packy.sh` and `scripts/validate-changed.sh` use explicit
internal-package allowlists; a new `internal/claudecode` package must be added
to both. Release artifact construction appears host-neutral, so the impact is
validation evidence and installed-package smoke expectations rather than a new
archive format.

### 8. Product and operator documentation

The following documents encode Codex/OpenCode as the supported product scope
and must be updated when support is implemented:

- `README.md`: product promise, golden path, global paths, safety, commands,
  and current exclusions;
- `docs/capability-packs.md`: supported surfaces, examples, readiness,
  ownership, and projection limitations;
- `docs/product/packy-v0.md`: accepted first-class surface scope;
- `docs/roadmap.md`: move Claude Code out of the future-adapter list; and
- `CONTEXT.md`: update the **CLI surface**, **Golden path**, and related
  glossary entries without changing the underlying domain vocabulary.

The final integration contract and hook-consent decision should be recorded in
an accepted ADR or the implementation-ready specification; this impact map and
the external research note are evidence, not substitutes for that decision.

## Cross-cutting risks and invariants

1. **No ambient or CLI-owned layout.** The Claude module derives its paths from
   one immutable workstation snapshot; CLI only composes owners.
2. **No direct `~/.claude.json` mutation.** It mixes MCP configuration with
   credentials, session data, trust, and project state.
3. **Foreign authority always wins.** Unknown skill paths, changed blocks,
   altered hooks, same-name MCP definitions, managed restrictions, and
   higher-precedence project/local settings are preservation/reporting cases.
4. **Configured is not usable.** Static files do not prove Claude loaded a
   skill, an MCP server connected, or a hook is permitted.
5. **Hooks are executable authority.** Generic lifecycle resources cannot
   silently become full-permission Claude command hooks.
6. **Inspection can have effects.** Approved MCP inspection may start the
   configured server and cannot participate in inert preview.
7. **Classic migration is explicit.** Old two-surface state cannot be treated
   as malformed merely because the new golden path includes Claude Code.
8. **No plugin lifecycle initially.** The initial candidate path uses native
   global skills, instructions, MCP, and selected hooks; Claude's marketplace
   and plugin cache would introduce a competing acquisition/ownership model.

## Decisions this map makes reachable

The local impact map sharpens the remaining work without resolving it:

1. **Decide the Claude Code host integration contract** can now specify the
   new host owner's layouts, observations, adapter inputs/outputs, command
   runner boundary, and preservation rules against concrete consumers.
2. **Decide Claude Code classic lifecycle and health behavior** can now define
   classic actions, state migration, ownership, version diagnosis, recovery,
   and doctor checks at the correct module seams.
3. **Decide Claude Code capability-pack behavior and exclusions** can now map
   each portable resource to the complete surface adapter and make hook/MCP
   limitations observable.
4. **Decide Claude Code compatibility and validation gates** can now enumerate
   the exact package allowlists, semantic suites, sandboxed smoke evidence,
   documentation, and release gates it must cover.

No additional fog became sharp enough to require another Wayfinder ticket; the
existing tickets already own every decision exposed by this map.
