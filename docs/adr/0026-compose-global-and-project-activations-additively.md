# ADR 0026: Compose global and project activations additively

## Status

Accepted.

## Context

Packy is adding project-scoped activation while preserving global activation, which applies to every project. Allowing a project to mask a global activation would introduce negative intent, precedence rules, and ownership ambiguity when the same pack or resource is contributed by both scopes.

## Decision

Global and project activations are independent contributions to a CLI surface. Effective activation within a project is their additive composition: project activation may add capabilities but cannot exclude or mask a global activation. Removing a project contribution leaves any global contribution effective, while removing a global contribution preserves any explicit project contributions.

## Consequences

- A compatible globally active pack is effective in every project, including projects without their own activation; an incompatible project contribution blocks readiness as an activation scope conflict.
- Project deactivation removes only the project's contribution.
- Packy does not support project-local opt-outs from global activation.
