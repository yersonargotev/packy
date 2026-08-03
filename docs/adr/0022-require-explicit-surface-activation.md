# ADR 0022: Require explicit activation for every surface projection

## Status

Accepted.

Project installation intent is the explicit-authority exception for version-controlled project projections defined by [ADR 0027](0027-separate-project-installation-from-personal-activation.md). Runtime effects and personal surface mutation still require activation intent.

## Context

Surface mutation must be attributable to an operator's explicit choice of pack,
resource roots, and target CLI surface. Initialization and discovery cannot
imply that consent.

## Decision

`internal/capabilitypack` is the sole semantic authority for every CLI-surface mutation. A surface adapter remains the only implementation allowed to apply host projections, but it receives desired composition, ownership authority, and consent from capability-pack lifecycle rather than deciding them.

Every projection must be attributable to explicit, previewed activation intent for either a complete pack or selected operational resource roots and their declared dependencies. A standard global target such as `~/.agents/skills` is a shared projection: one explicit activation may make it discoverable by other compatible surfaces without activating the pack there. Contributor ownership keeps the physical projection until its last activation contributor is removed.

Without activation intent, Packy core may mutate only Packy-owned non-surface substrate requested by the operator, including Installed Source, Pack Sources, catalog data, metadata, and recovery state that does not represent activation. Inspection remains read-only. Packy core may detect an external executable, but acquisition must originate from an explicit pack activation or update plan and receive separate approval; deactivation does not uninstall the shared executable. Tool-owned host setup remains a separately authorized surface mutation.

`packy doctor` reports Packy core health and summarizes only active-pack drift, requirements, and readiness. Inactive packs are not health failures; detailed readiness and automation gates remain capability-pack status concerns.

The highest regression seam is a sandboxed product test proving that every permitted core operation leaves all host configuration, shared skill targets, and external processes unchanged when no activation intent exists. Positive capability-pack tests prove full-pack activation, resource selection, shared contributors, and separately approved external requirements still project normally.

This ADR refines the sole lifecycle authority in [ADR 0005](0005-capability-pack-surface-adapter.md). Domain-owned workstation layout remains unchanged.
