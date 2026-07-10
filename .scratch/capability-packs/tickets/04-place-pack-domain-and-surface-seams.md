Type: grilling
Status: resolved
Blocked by: 01, 02

## Question

Where should the pack catalog, dependency resolver, desired-state model, ownership logic, and per-surface adapters live so Matty core remains a set of deep modules and `internal/cli` stays a thin lifecycle adapter?

## Answer

Use one deep `internal/capabilitypack` module as the owner of the portable domain and its use cases. It owns strict manifest decoding and validation, catalog construction, dependency/conflict resolution, desired-state computation, persisted logical state, resource ownership, readiness, and global-plan validation. These are implementation details behind one CLI-facing facade; they are not separate shallow packages.

The facade is the only workflow interface crossed by `internal/cli`. Its conceptual operations plan activation, deactivation, and reconciliation; report status; and apply an already-approved plan. Exact method names and the proof that an applied plan is the one the user approved remain for [Prototype reconciliation state machine](05-prototype-reconciliation-state-machine.md). The CLI may construct dependencies at the composition root, but it only translates flags, presents plans/checkpoints, invokes the facade, and renders results. It does not understand the state schema or orchestrate catalog, ownership, tools, or surfaces.

Each CLI surface is a real adapter at a sibling package seam:

```text
internal/capabilitypack  portable domain, state, ownership, facade
internal/codex           Codex inspection, projection, and approved execution
internal/opencode        OpenCode inspection, projection, and approved execution
internal/skillbundle     physical Matty-owned bundle-root discovery only
internal/engrambin       initial concrete resolver for the global Engram tool
internal/cli             composition, input, consent presentation, output
```

The surface-adapter interface belongs to `internal/capabilitypack`; concrete adapters inspect their own host and translate portable desired resources into observed state and planned host actions. Codex TOML, OpenCode JSON, hook/plugin files, credential stores, and host paths never enter the portable domain or CLI interface. The exact split among inspect, plan, and apply is intentionally left to the reconciliation prototype. Existing `internal/opencode` can evolve in place; Codex-specific behavior currently split between `internal/prompt` and `internal/cli` should move toward `internal/codex` rather than preserving `internal/prompt` as a false cross-host abstraction.

`internal/skillbundle` resolves only the physical production/repository/dev source root required by the existing bundle rule. `internal/capabilitypack` receives that root and owns the logical catalog; it never searches external clones or resolves `HOME`. Do not rename or generalize the locator until the implementation fixes whether packs live under `bundle/packs` or another Matty-owned bundle path.

Global tool requirements are separate from surfaces. `internal/capabilitypack` interprets `requires.tools` and consumes observed presence/version/path facts through a narrow resolver seam. `internal/engrambin` remains the one real resolver initially. Detection is not acquisition: package-manager installation and tool-owned setup are previewed executable actions subject to the already-defined consent boundary, not behavior hidden in the resolver.

Persisted ownership has one source of truth inside `internal/capabilitypack`; the current CLI-owned state must be replaced rather than mirrored. Adapters report artifacts and execute approved actions but never decide whether a shared or modified resource may be removed. Avoid a public state-store interface and a generic installer/provider framework until independent implementations actually make those seams real.
