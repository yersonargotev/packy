# Capability packs and manual transition

Capability packs are opt-in additions managed by **Packy core**. Packy remains
available when the optional pack named `matty` is inactive. Discovery, show,
status, and dry-run are inspection-only.

`packy init` prepares only Packy's Installed Source. Neither initialization nor
catalog presence authorizes any host projection. A complete pack or selected
operational roots and their dependency closure must be activated explicitly for
each surface before Packy can change that surface.

The selectable pack catalog currently contains `addy`, `argote`, `engram`, and `matty`.

| Current Pack | Purpose | Claude contract |
| --- | --- | --- |
| `argote` **1.0.0** | Yerson Argote's engineering and communication guidance | **complete**: every independent instruction or skill has a native binding and no Claude exclusion. |
| `matty` **4.0.0** | Workflow skills | **complete**: every skill has a native binding and no Claude exclusion. Matty no longer contributes instructions on any surface. Projection does not prove runtime usability. |
| `engram` **2.0.0** | Memory guidance and MCP | **degraded**, but activatable: its instruction and exact user MCP binding are native; `lifecycle:engram-memory` has the optional `generic-lifecycle-unsupported` exclusion because generic lifecycle translation is unsupported. Packy does not run `engram setup claude-code`. |
| `addy` **1.1.0** | Addy agent skills | **complete**: its manifest-v3 resources have native Claude bindings and remain selectable on Claude, Codex, and OpenCode. |

In plain release notation, the current contracts are **addy 1.1.0**,
**argote 1.0.0**, **engram 2.0.0**, and **matty 4.0.0**. Their Claude Code
behavior is described in the table above.

Addy's immutable history retains its exact manifest-v2 1.0.0 contract and its
manifest-v3 1.1.0 contract. Its sole update route is 1.0.0 to 1.1.0 for existing
Codex or OpenCode intent; it never adds Claude intent.

Historical `matty` 2.0.0 and 3.0.0 and `engram` 1.0.0 activations remain pinned
to their recorded versions and surfaces. Updating Matty 2.0.0 can select 3.0.0
only for already-active Codex or OpenCode intent. Updating Matty 3.0.0 can
select 4.0.0 on any already-active surface and removes the two retired
instructions only when their exact Packy ownership still authorizes cleanup.
Fresh Matty 4.0.0 activation projects only skills.

Remote/third-party sources, marketplaces, signing, version selection,
downgrades, unattended Apply, and background runtime management are excluded.

## Manifest discovery and resource selection

`packy pack list` discovers the current catalog, and `packy pack show <pack>`
shows the manifest contract before activation. Manifest v4 adds operational
resource roots and their dependency closure. Packy discovers those roots from
the manifest; it does not use presets, profiles, or an interactive picker.

Activation without `--resource` keeps the resource-less **all** mode and selects
every operational root:

```sh
packy pack activate example-pack --surface codex --dry-run
```

For a custom selection, repeat `--resource` with canonical `kind:id` roots.
Dependencies are included by discovery and are reported separately from the
selected roots:

```sh
packy pack activate example-pack --surface codex \
  --resource skill:build \
  --resource instruction:guidance \
  --dry-run
```

Applying another custom activation adds its roots to the persisted selection;
it does not replace the existing selection. `asset` and `notice` resources are
not selectable roots. Preview and status report the persisted selection mode,
each root, dependency chain, and unselected resource.

When a required capability has more than one valid provider, Packy does not
choose one implicitly. Select it explicitly with repeatable
`--provider <capability>=<pack>[/<kind>:<id>]`:

```sh
packy pack activate consumer --surface codex \
  --provider cap:storage=storage-pack/skill:storage \
  --dry-run
```

The chosen provider edge is persisted with the consumer intent. Status and
readiness use that recorded provider and its selected resource, not another
ambient provider. A provider activated only because it is required remains
while it has consumers; an explicitly activated provider remains independently
active. Invalid or stale persisted provider choices fail closed.

Project installation uses the same explicit resource, alias, and provider
intent without consulting global activation state:

```sh
packy pack install matty --surface codex \
  --resource skill:ask-matt \
  --alias skill:ask-matt=project-matt \
  --dry-run
```

Omitting `--resource` installs the complete pack. Repeating `--resource`
selects direct operational roots and installs their complete transitive
resource, asset, and notice closure. Repeatable `--alias` resolves a native
name collision explicitly, and repeatable `--provider` records a capability
provider decision. `packy.json` keeps that direct intent; `packy.lock.json`
fixes every requested and required pack version, admitted source, resource
role and chain, native binding or degradation, and projection contributor.
Packy does not use a machine-global provider to satisfy this project graph.
The complete installation, personal activation, status, update, recovery, and
uninstall workflow is documented in [Project capability-pack lifecycle](project-pack-lifecycle.md).

## Manifest v3 and v4 bindings and exclusions

Only manifest v3 and v4 can declare Claude. V4 preserves that explicit surface
contract and adds operational selection, capability composition, runtime modes,
and root migrations. The required, sorted, non-null `surfaces` array names the
Pack's surfaces. Every runtime resource (`skill`, `instruction`,
`mcp_server`, `lifecycle`, `agent`, or `command`) declares exactly one binding
or exclusion for every top-level surface. Assets inherit their consumers;
notices have empty outcome arrays and do not affect readiness. V1 and v2 remain
exact and never infer Claude support.

Claude translations are explicit:

- skills and commands use personal skills under `~/.claude/skills`, never
  legacy `.claude/commands`;
- instructions contribute deterministic text to the global marked Packy block;
- MCP uses official `claude mcp ... --scope user` operations;
- agents require explicit documented native tool and permission translations;
- lifecycle is supported only through an explicit typed command hook.

Generic lifecycle translation and opaque JSON injection are not supported. An
optional exclusion makes compatibility **degraded**. A mandatory exclusion, or
an excluded dependency of a mandatory resource, makes compatibility **blocked**
and the plan non-applicable. Compatibility is **complete** only when every
required resource has a native binding and no degradation or exclusion.

## Inspect and activate

```sh
packy pack list
packy pack show matty
packy pack show engram
packy pack status
packy pack status matty --surface claude

packy pack activate matty --surface claude --dry-run
packy pack activate matty --surface claude
```

There is no automatic adoption of existing content. Each mutation targets one
surface; activating Codex or OpenCode never activates Claude. Dry-run creates a
fresh Preview without approval or mutation. Apply requires an interactive
terminal and plan-bound typed approval; there is no generic `--yes`.

Preview reports compatibility before readiness, every binding/exclusion, exact
projections, preservation, blockers, expected readiness, and pending evidence.
It separates reversible local, executable/external, and destructive-cleanup
phases. Claude typed hooks require executable/external consent. Removing a
last-contributor hook or user MCP definition requires destructive-cleanup
consent. MCP environment values are always redacted. Any preview blocker
executes zero effects.

## Shared discovery is not activation

Some resources use targets shared by compatible agents: the standard global
`~/.agents/skills` tree and resource-scoped marked instruction contributions in
a project's `AGENTS.md`. Codex and OpenCode project activation compose identical
instruction bytes in that file. An activation on one surface may therefore make
the physical resource discoverable from another surface. That incidental
visibility does not activate the pack on the other surface, does not authorize
its surface-specific configuration, and does not create readiness there.

Every activation that requires the shared resource is recorded as a contributor.
Deactivation preserves the physical projection while any contributor remains
and removes it only after the final contributor is gone and its ownership
fingerprint still matches.

## Update, reconcile, recovery, and deactivation

```sh
packy pack update engram --surface claude --dry-run
packy pack update engram --surface claude
packy pack reconcile engram --surface claude --dry-run
packy pack deactivate engram --surface claude --dry-run
packy pack deactivate engram --surface claude
```

Update preserves the active all/custom selection and provider choices. For a
manifest-v4 custom selection, each old root must still exist in the target
manifest or have one exact declared `root_migrations` entry. Packy migrates that
recorded root identity during update; a missing, chained, cyclic, duplicate, or
ambiguous migration blocks the update rather than guessing a replacement.

Approvals belong to one immutable plan. A stale plan executes no actions;
repeat the originating verb for fresh inspection and consent. After a partial
attempt marked `recovery-required`, also repeat the originating verb. Packy
plans recovery from current evidence rather than replaying history. Recovery
keeps the recorded selection, provider edges, aliases, version transition, and
ownership evidence sealed to the fresh replacement plan.

Packy updates or removes only an exact unchanged recorded projection. It
preserves unmanaged, ambiguous, drifted, foreign, and higher-precedence content.
Shared resources remain while another contributor is active. Deactivation never
deletes credentials, Engram memory, foreign configuration, or external data. If
Claude is unavailable, local cleanup may proceed while user MCP ownership is
retained for later official removal.

When deactivation preserves a Packy-owned projection, the Pack intent becomes
inactive but retains its exact version, surface, all/custom selection, resource
contributors, fingerprint, and adapter provenance. Status reports this as
`inactive-with-residuals`, distinct from `inactive-clean` and
`recovery-required`. Repeating the same deactivation freshly inspects those
residuals without reactivating the Pack; a now-exact final-contributor residual
can be removed, while drifted or unmanaged content remains preserved.

Manifest-v4 custom selections can be deactivated incrementally by repeating
`--resource` for selected operational roots:

```sh
packy pack deactivate example-pack --surface codex \
  --resource skill:build \
  --resource instruction:guidance \
  --dry-run
```

Dependency-only resources cannot be removed directly; remove their consuming
root instead. Removing some roots persists the remaining custom selection.
Removing the last custom root deactivates the Pack. Omitting `--resource`
retains the resource-less whole-Pack deactivation behavior, including for an
activation recorded in all mode. Required providers and shared projections are
removed only when their last recorded consumer is gone. There is no force,
cascade, or automatic provider replacement path.

## Apply success is not readiness

Readiness is independent of compatibility and successful Apply:

- **configured**: exact required projections are present;
- **authorized**: configured, on a supported Claude version, with observable
  policy/tool permission;
- **usable**: explicit current evidence says skills, commands, agents, and
  instructions loaded; MCP connected; and hooks fired, as applicable.

Assets inherit their consumer; notices and exclusions do not participate. A
version, definition, projection, policy, or Pack-version change invalidates the
affected evidence. Filesystem correctness never manufactures usability.

```sh
packy pack status engram --surface claude
packy pack status engram --surface claude --require usable
```

The second command emits status and exits nonzero until every required signal
is freshly known true. Login, trust, reload, runtime loading, and external setup
remain human/host boundaries; Packy reports but does not perform them.
