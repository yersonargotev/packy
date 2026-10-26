# Skill discovery and host coexistence

Packy selects a reviewed resource definition from the requested CLI surface.
One logical skill can have different bodies for Agents and Claude. Each
surface discovers its own roots, so distinct surface bodies can coexist.

## Native targets

| Surface | Global skills | Project skills |
| --- | --- | --- |
| Agents | `$HOME/.agents/skills/<name>` | `.agents/skills/<name>` |
| Claude Code | `$HOME/.claude/skills/<name>` | `.claude/skills/<name>` |

Names above are projected names; the logical resource ID remains the Pack
selection and dependency identity. Skills include their entire selected tree,
including references, helper files, and metadata.

## What preview can establish

Packy checks divergent or unverified same-name skills within the selected
surface's discovery roots, including global/project combinations. A different
directory basename alone does not isolate duplicate frontmatter names. The
observation covers global roots, the current project, and ancestor directories
up to the Git worktree boundary. It does not establish discovery for every
other worktree, custom configured root, IDE launch, plugin, or future host
version. Retired OpenCode roots are not inspected or configured.

Unverified simultaneous intent blocks before mutation. Global previews report
`host-discovery`; project previews report `host_discovery_conflict`. Readiness
retains the existing `runtime-usability` condition with `usable=unknown` and
reason `runtime-unobservable`, with an explicit discovery explanation, rather
than claiming the host loaded the intended skill. Inspect the reported paths
and names, then choose nonconflicting intent or remove an affected Pack
installation through its ordinary previewed lifecycle. User-owned content
remains user-owned. Two Packs claiming one physical target still conflict,
and drift protection covers the complete receipt-owned tree.

## Historical evidence and limits

The [dated discovery evidence](research/evidence/skill-discovery-2026-10-02.md)
records the binaries, methods, and limits of earlier host probes. Its retired
host observations remain historical evidence, not current support policy.
A successful Packy projection or update does not prove that a host loaded,
authorized, or completed a workflow. Current opt-in probes cover Codex and
Claude only; successful model-backed workflows remain separately scoped.

For an installation using retired contracts, follow the
[clean adoption procedure](catalog-adoption.md) before upgrading and reinstalling.

## Reviewed skill templates

A Pack can opt a skill into the `skill-template` capability. A `pack`
binding uses `<pack-id>-<binding-name>` as its deterministic default
native name; a `native` binding retains its name while resolving explicit
references to other skills. An explicit user alias overrides the default.

For example, pstack's names are `pstack-tdd` and `pstack-teach` on every surface,
without requiring aliases. Within one surface, identical verified same-name
trees are allowed, while divergent or unverified trees still block. Agents and
Claude use separate roots. The surface selects the body and native target
directory, rather than becoming part of the default name.

The renderer gives `SKILL.md` the effective native name and expands only authored
`{{packy.skill:<logical-id>}}` tokens. For example, a reviewed Agents template
containing `` `${{packy.skill:tdd}}` `` and
`../{{packy.skill:tdd}}/SKILL.md` resolves both to its own namespaced TDD skill.
A consumer declares the target as a direct `requires` dependency. Ordinary text
such as shell `$tdd`, code identifiers, and historical names is left untouched.

These skills use generated, receipt-owned trees rather than links to unchanged
source. Preview observes the generated frontmatter before Apply. Catalog sources
remain immutable, and drift and removal are checked against the final installed
bytes. Non-template skills and unresolved collisions still block as above.
The design is recorded in [ADR 0043](adr/0043-render-reviewed-skill-identities-and-references.md).
