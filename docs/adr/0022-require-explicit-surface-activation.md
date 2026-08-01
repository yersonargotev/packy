# ADR 0022: Require explicit activation for every surface projection

## Status

Accepted.

## Context

Packy retained two desired-state authorities after capability packs became opt-in: the classic root lifecycle projected one fixed workflow onto every supported CLI surface, while capability-pack lifecycle projected only explicitly activated packs and resources. The classic behavior made surface mutation possible without activation intent and could not be repaired by removing individual prompt fragments because it also owned skills, MCP configuration, lifecycle setup, and host-specific artifacts.

## Decision

`internal/capabilitypack` is the sole semantic authority for every CLI-surface mutation. A surface adapter remains the only implementation allowed to apply host projections, but it receives desired composition, ownership authority, and consent from capability-pack lifecycle rather than deciding them.

Every projection must be attributable to explicit, previewed activation intent for either a complete pack or selected operational resource roots and their declared dependencies. A standard global target such as `~/.agents/skills` is a shared projection: one explicit activation may make it discoverable by other compatible surfaces without activating the pack there. Contributor ownership keeps the physical projection until its last activation contributor is removed.

Without activation intent, Packy core may mutate only Packy-owned non-surface substrate requested by the operator, including Installed Source, Pack Sources, catalog data, metadata, and recovery state that does not represent activation. Inspection remains read-only. Packy core may detect an external executable, but acquisition must originate from an explicit pack activation or update plan and receive separate approval; deactivation does not uninstall the shared executable. Tool-owned host setup remains a separately authorized surface mutation.

The classic lifecycle and the root `packy install`, `packy update`, and `packy uninstall` commands are removed. This is an incompatible cutover with no compatibility shims and no migration command. Packy does not read, adopt, delete, or translate classic `config.json` state, managed-skill records, created-container records, or Claude ownership. Leftover artifacts are unowned and preserved; the sole current operator will clean the old installation manually before initializing Packy and activating desired packs afresh.

`packy doctor` reports Packy core health and summarizes only active-pack drift, requirements, and readiness. Inactive packs and absent classic artifacts are not health failures; detailed readiness and automation gates remain capability-pack status concerns.

The highest regression seam is a sandboxed product test proving that every permitted core operation leaves all host configuration, shared skill targets, and external processes unchanged when no activation intent exists. Structural checks forbid the removed commands and classic lifecycle authority, while positive capability-pack tests prove full-pack activation, resource selection, shared contributors, and separately approved external requirements still project normally.

This ADR supersedes [ADR 0003](0003-core-lifecycle-deep-module.md), refines the sole lifecycle authority in [ADR 0005](0005-capability-pack-surface-adapter.md), and supersedes only the classic-state provisions of [ADR 0006](0006-own-workstation-layout-by-domain.md). Domain-owned workstation layout remains unchanged.
