---
status: accepted
---

# Focus surfaces on Agents and Claude conventions

The design review on 2026-10-25 narrows Packy's projection model to the shared
Agents convention and the Claude-native convention. This records the confirmed
target design; implementation and catalog adoption remain separate delivery
work.

## Confirmed decisions

- Remove OpenCode support and replace the `codex` surface identity with
  `agents`. These are the only supported surfaces.
- `agents` means the shared `AGENTS.md` and `.agents/skills` convention. It
  must not silently treat every consumer as Codex. Additional custom-agent
  projections explicitly name their concrete client and supported scope.
- `claude` retains the `CLAUDE.md` and `.claude/skills` convention.
- Agents instruction contributions are project-only. Agents skills support
  project `.agents/skills` and global `~/.agents/skills`; no shared global
  instruction path is invented.
- Produce native plugin projections from reviewed Packs in the Claude plugin
  format and the Agent Plugins format. Arbitrary external-plugin import is
  outside this request.
- Plugin delivery generates local packages. Native clients own installation
  and enablement; Packy does not administer their plugin managers.
- The supported content is instructions, complete skill trees, and explicitly
  reviewed custom agents required by workflows. Preserve skill-local metadata,
  including `agents/openai.yaml`. Remove direct MCP, hooks, and commands.
  Plugins contain skills only; they do not automatically carry or install
  client-specific custom-agent definitions.
- Preserve the reviewed catalog as the source of Pack selection and provenance.
- Direct installation and plugin export are separate explicit operations.
  Installing or activating a Pack does not also generate or install a plugin.
- Export publishes a complete package to a caller-selected new or empty
  directory. A later export produces another package rather than modifying a
  delivered package. Export creates no installation receipt or managed plugin
  activation, update, or removal state.
- Export one plugin per Pack. By default include all skills eligible for the
  chosen plugin format; allow explicit skill selection. Resolve the effective
  resource before adding transitive same-Pack native dependencies, scripts,
  references, supporting assets, provenance, and legal notices. Instruction
  contributions remain outside the package. No dependency may escape the
  standalone package's root.
- Map `agents` to Agent Plugins and `claude` to the Claude plugin format.
  Use the Pack ID as the plugin name. Each exported skill needs explicit
  reviewed compatibility for the chosen format, including its names and
  authored references. A native skill binding alone is insufficient.
- If any explicitly selected skill or required dependency is incompatible,
  fail the complete export with the cause before publishing output. Do not
  silently omit a selected skill or infer runtime adaptation from its prose.
- Adopt the change through a coordinated clean cut in Packy and
  `yersonargotev/packy-catalog`. Remove the `codex` and `opencode` surface
  identities without aliases, legacy readers, or automatic migration. Codex
  remains an explicitly identified client for its custom-agent projections.
  Use the previous binary to
  remove affected installations before replacement; preserve unrelated user
  files and retained sources needed for that handoff. Release the engine
  before publishing the compatible reviewed catalog.

## Final confirmation

The user confirmed all ten initial interview recommendations and the revised
custom-agent scope on 2026-10-25. Preserve the rest of the agreed design with
that adjustment; do not retire required custom agents as part of removing
unrelated runtime integrations.

## Preserve required custom-agent definitions

Official OpenAI documentation checked on 2026-10-25 distinguishes two separate
mechanisms:

- A skill may contain optional `agents/openai.yaml` metadata for presentation,
  invocation policy, and tool dependencies. This belongs to the skill resource
  tree; it is not a custom subagent definition. The skill-only scope must not
  be interpreted as stripping this reviewed content.
- Codex defines custom subagents in standalone `.codex/agents/*.toml` or
  `~/.codex/agents/*.toml`, with required `name`, `description`, and
  `developer_instructions`. Codex also has built-in agents, so custom files
  are not a prerequisite for ordinary skill loading or all delegation.
  Workflows requesting particular custom roles can still depend on them.

The current Packy Codex adapter already projects global agent resources as
TOML via `codexAgentTOML`; its project adapter does not currently support agent
resources. Removing the global projection therefore removes existing behavior,
not just an unused configuration mechanism.

Retain complete reviewed skill trees, including `agents/openai.yaml`, and
support explicitly reviewed custom-agent projections for concrete clients
when workflows require them. The shared Agents convention alone cannot select
a Codex-specific role format. Identify the client and supported scope in the
reviewed projection contract and expose that target in selection and previews;
never infer the client from executable discovery or silently write Codex files
for every Agents consumer. Codex definitions use its native TOML format; Claude
definitions retain its native agent-document format. These client identities
do not create additional CLI surfaces.

Retain custom agents in declared native resource closure, ownership, drift
protection, update, and removal. If a required agent cannot be represented for
the selected client and scope, fail before mutation instead of dropping the
dependency or replacing it with a generic role. Supporting a custom-agent
document does not restore direct MCP, hooks, commands, or unrelated client
settings management.

Skills-only plugin exports do not automatically carry or install these
client-specific definitions. A skill requiring a custom agent needs a reviewed
plugin-compatible adaptation or an explicit reviewed prerequisite supplied by
the client environment. The export report must disclose such prerequisites;
generation must not assert they are satisfied. Otherwise the complete export
fails. Never silently omit a required role or claim that the portable plugin
format registers it.

## Architectural implications

This decision revises the concrete-host meaning of CLI surface in ADR 0031
and the `codex`/`claude` selection vocabulary in ADR 0044. It retires ADR 0043's
OpenCode-specific coexistence obligations while preserving its explicitly
reviewed reference-rendering principle. ADR 0035's
domain-owned lifecycle and readiness, ADR 0039's independent reviewed catalog,
and ADR 0041's logical resource identity remain relevant constraints.

Keep shared selection, ownership, drift, and lifecycle behavior in the owning
domain module. Surface adapters should implement the confirmed conventions;
plugin packaging and client installation must not be conflated with runtime
readiness. A valid plugin package is not evidence that every client can load
all its components.

Retain the small surface-adapter interface and domain-owned selection and
ownership behavior. Replace Codex-specific projection with the Agents
convention rather than hiding the former adapter behind the new identity.
Plugin packaging should expose one cohesive operation from retained catalog
identity, Pack, format, and selection to a deterministic package and report.
Reuse effective resource resolution, declared native dependency closure, and
filesystem staging where their contracts fit. The on-demand closure is a
different contract and must not be substituted for native plugin closure.

The existing `packy.skill` token has one kebab-case value used across native
names, invocations, and paths. A Claude plugin invocation namespace cannot
simply be substituted into all these contexts. Any plugin adaptation must
remain explicitly reviewed rather than infer references from arbitrary prose.
Keep skill identity, invocation syntax, and relative paths distinct in the
reviewed export contract. Agent Plugins does not establish one invocation
syntax or runtime capability shared by all consuming clients.

Agent Plugins standardizes portable skills and MCP packaging while leaving
installation and enablement to clients. Claude uses a different native plugin
format, and a plugin-root `CLAUDE.md` is not loaded as project instructions.
Instruction contributions therefore remain separate from plugin packaging.

## Implementation and validation scope

Deliver the direct instruction/skill lifecycle on the two conventions and the
explicitly reviewed custom-agent projections, retiring obsolete surface
adapters and unrelated runtime projection paths. Update CLI,
TUI, schemas, receipts, locks, discovery, on-demand access, and documentation
consistently; neither `codex` nor `opencode` remains a supported surface value.
Do not change shared selection and ownership semantics merely to rename a
surface.

Add the explicit plugin compatibility contract and a local export operation.
The package/report must identify the retained Catalog Snapshot, Pack version,
surface/format, and selected dependency closure. Validate the whole package
before publishing from staging. A failure must leave the destination unchanged;
a non-empty destination must not be overwritten. Export neither executes
content nor changes native client settings or Pack installation state.

Review the Catalog Project's manifests, instructions, skill content, and
capabilities against this narrower model. Renaming `codex` in catalog metadata
does not establish that runtime-dependent instructions work under the Agents
convention or that a skill is plugin-compatible. Preserve attribution and
notices, bump changed Pack versions, and validate the complete compatible
snapshot against the released engine before publication.

Validate focused domain and adapter behavior first, then the cross-cutting
repository checks. Manual CLI acceptance must sandbox `HOME` and
`XDG_CONFIG_HOME`. Demonstrate direct install, update, drift protection,
inspection, and removal for both surfaces and applicable scopes; project-only
Agents instructions; rejection of retired surface identities; and on-demand access
using the new surface vocabulary. Demonstrate both plugin formats, partial
skill selection with transitive closure, legal/supporting-file containment,
reviewed references, repeatable package bytes, and complete failure for an
incompatible selection or unsafe destination. Runtime client installation and
execution remain separate from package-generation acceptance.

Also demonstrate that skill-local `agents/openai.yaml` survives direct
installation, on-demand preparation, and compatible plugin export; required
custom agents are projected and removed through their declared client and
scope; unsupported dependencies fail without partial mutation; and direct
Agents skill installation without a declared Codex projection does not write
`.codex` configuration. Verify generated role definitions separately from any
claim that a runtime has loaded or executed them.

Sources checked on 2026-10-25:

- [Agent Plugins specification](https://agent-plugins.org/specification)
- [Agent Plugins client implementation](https://agent-plugins.org/client-implementers/implement-an-agent-plugins-client)
- [Claude plugin reference](https://code.claude.com/docs/en/plugins-reference)
- [Claude plugin components](https://code.claude.com/docs/en/plugins/components)
- [Agent Plugins skills](https://agent-plugins.org/plugin-authors/skills)
- [OpenAI skill structure and optional metadata](https://learn.chatgpt.com/docs/build-skills)
- [Codex built-in and custom subagents](https://learn.chatgpt.com/docs/agent-configuration/subagents)
