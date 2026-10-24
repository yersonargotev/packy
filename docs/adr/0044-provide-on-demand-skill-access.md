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
root, and declared prerequisites, including known Pack external requirements.
Only selectable Packs are available. Returned absolute paths are intentional access
references, not redacted diagnostic paths. Failures are identifiable and never
contain a successful partial result.

## Same-Pack procedures and access references

Issue #861 extends preparation to the complete transitive closure of same-Pack
`requires` skill edges, assets, and notices after effective surface resolution.
Every skill must have a supported binding and reviewed on-demand contract.
Missing, excluded, cyclic, or incompatible resources fail without a successful
partial report. Repeated and diamond edges share one Pack-relative resource tree
in a single atomic preparation; dependencies retain the initiating snapshot and
surface. Cross-Pack declarations remain outside this extension.

An `on_demand.entry_document` optionally selects a standalone instruction file
relative to the skill source; omission selects `SKILL.md`. Native templates and
Claude composite skills require a distinct reviewed entry document. This permits
native instructions to keep their original invocation contracts while providing
an authored access procedure within the same declared and attributed source tree.

An optional sorted `on_demand.reference_files` list opts specific skill-relative
Markdown documents into explicit `{{packy.access:<logical-skill-id>}}` rendering.
Each token becomes a URL-escaped relative Markdown path from that document to the target's prepared
effective entry document. A target must be self or a directly required same-Pack
skill. Malformed and undeclared references fail explicitly. Supporting relative
links remain authored content. Only listed documents are rendered: arbitrary
prose, scripts, host invocations, and native `{{packy.skill:...}}` semantics remain
unchanged. Sources remain immutable; preparation reuse verifies the expected
rendered bytes and modes across the complete closure.

The versioned access report includes a sorted `dependencies` array with each
transitively prepared skill's qualified identity, version, surface, snapshot,
entry document, resource root, and prerequisites. This is resource availability,
not native registration or workflow execution. Existing native installation,
aliases, and installed-copy invocation retain their contracts. Engine release
still precedes Catalog Publication using the extended vocabulary.

OpenCode on-demand access, arbitrary version selection, workflow execution,
discovery enhancements, bootstrap content, and real-catalog adaptations remain
outside this slice.
