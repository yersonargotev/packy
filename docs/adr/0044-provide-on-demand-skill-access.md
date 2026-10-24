---
status: accepted
---

# Provide reviewed skills on demand without installation

## Context

A temporary request for a Pack's skill should not create installation intent or
native-name collisions. File availability alone does not establish that a skill
can be followed outside its installed layout. The confirmed design in
[issue 859](https://github.com/yersonargotev/packy/issues/859) separates reviewed
resource provision from the consuming agent's workflow execution.

## Decision

Extend ADR 0034's Pack-oriented namespace with `packy get skill <pack>:<skill>`.
The qualified identity selects one logical skill from an explicitly selected
`codex` or `claude` surface. Resolve the Effective Resource before supporting
closure; native bindings and exclusions remain the host support boundary.
Installed copies and aliases do not participate in retrieval.

The optional typed resource declaration `on_demand` establishes reviewed
compatibility independently of a native binding. Its closed `skill-tree-v1`
protocol promises a standalone `SKILL.md` resource tree with authored relative
references, declared supporting assets and notices, and a non-null sorted list
of known prerequisites. Surface variants may replace the complete declaration.
Pack schema v3 gains this vocabulary without a legacy reader or inferred
Markdown dependencies. Older engines reject the unknown field. Catalog
Publication using it follows release of the implementing engine.

On-demand Skill Access reads the selected local Catalog Snapshot, or the exact
retained identity supplied through `--snapshot`. It never acquires or refreshes
implicitly and never substitutes a different revision for missing or invalid
retained content. Prepare independent Packy-owned copies under the user data
root, partitioned by preparation protocol, snapshot, surface, Pack, and skill.
Preserve the selected source/supporting/legal paths relative to the Pack root,
file bytes, executable modes, and the manifest's provenance declarations.
Keep prepared resources separate from target projects and writable scratch or
output. No automatic expiration, eviction, or cleanup applies.

Preparation publishes a complete verified tree atomically. Reuse verifies the
prepared bytes and modes; drift fails instead of overwriting a consumer's
changes. Catalog Snapshot sources remain unchanged. There is no installation
intent, projection ownership, receipt, activation, or permanent instruction
contribution. Existing native installation, aliases, and same-Pack
`skill-template` rendering keep their contracts.

Packy is the provider of reviewed resources. The consuming agent reads the
returned entry document, verifies prerequisites, chooses target and writable
locations, and follows the skill's procedure, approvals, domain assumptions,
and retirement requirements. Retrieval never executes catalog scripts, installs
tools, registers a native skill, or asserts workflow success or runtime readiness.
The Catalog Project continues to own content, adaptations, prerequisites,
provenance, notices, and publication; the CLI remains an adapter to engine domain
behavior.

## Consequences

Readable output and the versioned `skill-access` report describe availability,
qualified identity, Pack version, surface, snapshot, entry document, resource
root, and declared prerequisites. Returned absolute paths are intentional access
references, not redacted diagnostic paths. Failures are identifiable and never
contain a successful partial result.

The first implementation slice (#860) accepts dependency-free reviewed skill
trees and supporting assets/notices. Skill dependencies fail explicitly until
#861 and #862 add reviewed closure and reference semantics. Native templates and
Claude composite capabilities require a reviewed standalone on-demand tree;
they are not silently rendered as native invocations. OpenCode on-demand access,
arbitrary version selection, workflow execution, discovery enhancements,
bootstrap content, and real-catalog adaptations remain outside this slice.
