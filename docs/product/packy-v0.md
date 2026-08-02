# Packy v0 product scope

Packy v0 is a macOS-first installer/configurator for explicit capability packs.
It prepares Packy-owned source and catalog substrate, then projects selected
resources to Codex, OpenCode, and Claude Code only after per-surface activation.
It is not an always-on runtime orchestrator.

## Quick path

The [README quickstart](../../README.md#quickstart) is the canonical first-run
sequence. Package-installed users initialize the Installed Source with
`packy init`, inspect the catalog, and explicitly preview and activate a chosen
pack or resource roots for a chosen surface. Initialization and discovery do
not change any CLI surface.

After installation:

- use `packy doctor` for read-only core health and an active-pack summary;
- use `packy pack update <pack> --surface <surface>` to refresh one activation;
- use `packy pack deactivate <pack> --surface <surface>` to remove exact owned projections.

## Problem

The useful parts of Gentle AI, Matt Pocock skills, and Engram are valuable, but
stitching them together manually creates repeated config work and a heavy
always-on prompt surface. Packy makes that setup repeatable, inspectable,
updateable, and reversible while keeping startup instructions small.

## Product boundary

| Area | v0 scope |
| --- | --- |
| Role | Installer/configurator, not a runtime orchestrator. |
| Platform | macOS-first. Linux may be considered later but is not promised for v0. |
| CLI surfaces | Codex, OpenCode, and user-global Claude Code 2.1.203+. |
| Skills | Pack-selected resources projected to standard targets such as `~/.agents/skills`. |
| Sources | A package Installed Source plus explicitly managed Pack Sources and catalog data. |
| Memory | Engram requirements and host setup only when declared by an activated pack and separately approved where required. |
| Capability packs | Opt-in `matty` and `engram` packs managed independently on each supported surface. |
| State | Capability-pack intent, ownership, consent, recovery, and readiness state beneath `~/.packy`. |
| Prompts | Small pack-owned global instruction/config projections only. |
| Safety | Preserve user, Engram, and Gentle AI content outside Packy markers. |

## User outcomes

- Initialization leaves every Codex/OpenCode/Claude Code surface unchanged.
- A previewed full-pack or resource-scoped activation configures only a chosen surface.
- Repos are not polluted with copied skills or local prompt files by default.
- Repeated activation/update runs are idempotent.
- `doctor` is safe and read-only.
- Deactivation preserves foreign or modified content and does not uninstall shared executables or delete external data.
- Capability-pack changes are explicit, surface-scoped, ownership-aware, and separately gated from host readiness.
- Shared-target discovery can make a resource visible to compatible hosts, but it does not create activation intent for those surfaces.

## Implemented product areas

| Area | Outcome |
| --- | --- |
| 01 scaffold | Go+Cobra CLI with Installed Source, doctor, catalog, and pack lifecycle commands, plus sandboxable execution. |
| 02 state/dry-run | Immutable pack plans and intent/ownership state for file, symlink, and external-command actions. |
| 03 skill projections | Contributor-owned pack skill projections, including standard shared targets. |
| 04 requirements | Read-only detection plus separately approved acquisition and tool-owned host setup. |
| 05 Codex | Pack-selected skills and instructions that preserve user and foreign content. |
| 06 OpenCode | Pack-selected prompt/config projections without clobbering existing JSONC config. |
| 07 doctor | Read-only core health plus active-pack drift, requirements, and readiness summary. |
| 08 lifecycle | Explicit activation, update, reconcile, recovery, and contributor-safe deactivation. |
| 09 hardening | README docs and end-to-end sandbox lifecycle tests. |
| Package distribution | Versioned GitHub Release artifacts, Homebrew publication, `packy init`, and package-install smoke coverage. |
| Capability packs | Discovery, status, activation, update, reconciliation, deactivation, recovery, and readiness gates for `matty` and `engram`. |
| Automation | Versioned JSON output for doctor and pack status, with stable health and readiness exit behavior. |
| Internal ownership | Capability-pack lifecycle is the sole surface-mutation authority; host modules translate, inspect, and apply authorized actions. |
| Claude Code | User-global skills, marked instructions, explicit agents and typed hooks, and official user-scoped MCP operations, with inert health and independent readiness. |

## Out of scope for v0

- TUI installer or model picker.
- Runtime profile manager.
- SDD workflow installation or SDD orchestrators.
- Repo-local docs/config by default.
- Antigravity, GitHub Copilot CLI, Gemini, Cursor, or other adapters.
- Claude plugins, repository-local Claude configuration, authentication, or model invocation.
- Automatic Gentle AI cleanup or migration.
- Vendoring the Engram binary.
- Installing only a tiny skill subset; v0 controls tokens through lazy routing.

## Verification

The [repository validation contract](../../README.md#verification) defines the
authoritative delivery gate, compatibility check, and focused iteration aid.

Before using Packy against a real HOME, run the package lifecycle in a sandboxed
HOME/config environment. The canonical command sequence and focused automated
smoke test live in the [release guide](../release.md#sandboxed-package-install-smoke-expectations).
