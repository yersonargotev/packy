# Claude Code global integration surface

Research date: 2026-07-20

## Question and evidence boundary

Which officially supported, user-global Claude Code CLI surfaces can Packy use
to project its classic workflow and capability packs without authentication or
model calls?

This note uses only Anthropic's Claude Code documentation and first-party CLI
evidence. I ran only `claude --version` and `claude --help`; the installed CLI
reported `2.1.215`. I did not start a Claude session, authenticate, call a
model, install a plugin, connect an MCP server, or modify Claude configuration.

## Decision-ready answer

Packy can safely make Claude Code a global surface with four native mechanisms:

1. project each Pack skill at `~/.claude/skills/<name>/SKILL.md`;
2. merge a Packy-owned section into `~/.claude/CLAUDE.md` for small global
   behavioral guidance that is context rather than enforcement;
3. register global stdio MCP servers through `claude mcp add(-json) --scope
   user`, while treating the CLI—not direct editing of `~/.claude.json`—as the
   write boundary; and
4. merge only explicitly selected command-hook entries into the `hooks` object
   in `~/.claude/settings.json` when a pack's lifecycle contract genuinely
   requires deterministic events.

Use plain skills/instructions/MCP/settings for the initial adapter. Do **not**
turn every Pack into a Claude plugin: plugins add a separate marketplace,
installation, cache, version, and arbitrary-code trust lifecycle. A future
plugin projection is appropriate only for a capability that must be distributed
as one atomic bundle of namespaced skills, agents, hooks, and MCP servers.

Set the initial Packy compatibility floor to **Claude Code 2.1.203** if Packy
projects its existing shared skill source as directory symlinks. Anthropic
explicitly documents personal/project skill-directory symlink support as
requiring 2.1.203. If implementation instead copies complete skill directories,
this research does not establish a lower safe floor; that would require a
separate version matrix against every exact feature Packy selects. Detect with
`claude --version`; do not write Claude's `minimumVersion` setting as Packy's
compatibility gate because it prevents downgrades but does not block an already
old CLI, while `requiredMinimumVersion` is managed-settings-only
([settings reference](https://code.claude.com/docs/en/settings#available-settings)).

## Stable, documented mechanisms Packy may project

| Pack behavior | Official Claude mechanism | User-global location / command | Packy recommendation |
|---|---|---|---|
| Skills and commands | Agent Skills `SKILL.md`; legacy `.claude/commands/*.md` still works, but skills are recommended | `~/.claude/skills/<name>/SKILL.md` | Project Pack skills here. Preserve supporting files. Prefer one Packy-owned symlink per skill; require 2.1.203 for symlink support. Claude watches existing skill roots for live `SKILL.md` changes, but a newly created top-level skills directory needs a restart ([skills: locations, precedence, symlinks, and reload](https://code.claude.com/docs/en/skills#where-skills-live)). |
| Small global behavioral guidance | User `CLAUDE.md` | `~/.claude/CLAUDE.md` | Add/remove only a uniquely marked Packy block. Keep it concise. Claude loads it as context in every session, but it is guidance—not enforcement ([memory and instruction scopes](https://code.claude.com/docs/en/memory#choose-where-to-put-claudemd-files)). |
| MCP capability | User-scoped MCP | `claude mcp add ... --scope user`; stored by Claude in `~/.claude.json` | Invoke the official CLI with the pack's exact stdio command/args. Inspect by server name; remove only a definition whose observed command/args still match Packy's recorded ownership. User scope is global and private ([MCP scopes and precedence](https://code.claude.com/docs/en/mcp#mcp-installation-scopes)). |
| Deterministic lifecycle integration | Command hooks in user settings | `hooks` in `~/.claude/settings.json` | Project only reviewed `type: "command"` hooks. Claude passes JSON on stdin; no model call is inherent. Useful events include `SessionStart`, `UserPromptSubmit`, `PreCompact`, `PostCompact`, `Stop`, and `SessionEnd`, but the selected event set must come from each pack's actual lifecycle contract ([hooks reference](https://code.claude.com/docs/en/hooks)). |
| Settings | Hierarchical JSON settings | `~/.claude/settings.json` | Parse strict JSON, deep-merge only Packy-owned keys/entries, preserve all foreign data, and validate the complete result. User settings are lowest priority; project/local/managed settings can override or disable Packy's effect ([settings scopes and precedence](https://code.claude.com/docs/en/settings#settings-precedence)). |

### Skill semantics that matter

Claude Code follows the Agent Skills standard and documents `description`,
`when_to_use`, invocation controls, arguments, tool controls, subagent context,
hooks, and supporting files. Names are derived from the directory name for
personal skills. Personal skills override project and bundled skills of the
same name, while enterprise skills override personal ones; plugin skills are
namespaced. That makes a name collision a real observable blocker, not
permission to replace an unknown path. Full skill content loads only when the
skill is invoked, while descriptions participate in every-session discovery
([skills reference](https://code.claude.com/docs/en/skills)).

### Settings schema and merge constraints

`settings.json` is the official hierarchical configuration mechanism. Anthropic
documents the official SchemaStore schema URL as
`https://json.schemastore.org/claude-code-settings.json`, but warns that the
published schema may lag newly documented CLI fields. User/project/local files
are strict: one validation failure rejects the whole file. Packy should
therefore combine schema validation with `claude doctor`, preserve unknown
foreign keys, and never treat schema lag alone as proof a documented field is
unsupported ([settings files and schema](https://code.claude.com/docs/en/settings#settings-files)).

Arrays do not uniformly behave like scalar replacement: documented array-valued
settings concatenate and deduplicate across scopes. Within a single user file,
hooks are nested event/matcher/handler arrays with no Packy fragment/include
mechanism. Ownership must therefore be entry-level, based on a canonical exact
handler identity and recorded fingerprint; Packy cannot own the whole `hooks`
object.

### Hook lifecycle, trust, and usability

Command hooks receive event JSON on stdin and return via exit codes/stdout;
prompt and agent hooks invoke a Claude model and are therefore outside the
no-model-call projection boundary. `SessionStart` supports command and MCP-tool
handlers and can add static/dynamic context; static context should instead use
`CLAUDE.md`. `SessionEnd` is non-blocking and has a short default shutdown
budget. Hook output and blocking semantics vary by event, so adapters must not
translate a generic `lifecycle` resource into an arbitrary hook without an
event-specific contract ([hook inputs, outputs, and events](https://code.claude.com/docs/en/hooks#hook-events)).

Hooks are high-trust executable configuration: Anthropic states that command
hooks run with the user's full permissions. `disableAllHooks` disables them,
managed `allowManagedHooksOnly` can block user hooks, and managed
`strictPluginOnlyCustomization` can block user skills, hooks, and MCP servers.
These are observable host-policy limitations and must produce degraded/blocked
readiness rather than being overwritten ([hook security](https://code.claude.com/docs/en/hooks#security-considerations),
[managed customization restrictions](https://code.claude.com/docs/en/settings#strict-plugin-only-customization)).

### MCP ownership and inspection

Claude stores both local and user MCP definitions in `~/.claude.json`, which
also contains OAuth/session and per-project state. That mixed-content file is
not a safe Packy-owned document. Use the official commands:

```text
claude mcp add --scope user ...
claude mcp add-json --scope user <name> '<definition>'
claude mcp list
claude mcp get <name>
claude mcp remove --scope user <name>
```

`list` and `get` are the documented inspection paths, and `/mcp` is the
interactive connection-status view. Current first-party `claude mcp get --help`
also says approved servers are health-checked, so it is not a purely inert
filesystem read: use it only for a Packy-recorded server when process launch is
appropriate, never to probe an unknown collision during dry-run. Packy must
regard a same-name, different definition as foreign. Local and project
definitions outrank user scope, so a correct Packy-owned user entry may still
be shadowed and should not be reported as effective/usable. Managed MCP
allowlists and plugin-only policies may also block it
([MCP management](https://code.claude.com/docs/en/mcp#managing-your-servers),
[scope precedence](https://code.claude.com/docs/en/mcp#scope-hierarchy-and-precedence)).

## Read-only inspection without authentication or model calls

The safe static/read-only evidence set is:

- `claude --version` / `claude -v` for the compatibility floor;
- `claude doctor`, officially documented as read-only installation and settings
  diagnostics, for install health and settings validation;
- filesystem `lstat`, target resolution, content hashes, and strict JSON parsing
  for Packy-owned skill links, the marked `CLAUDE.md` block, and exact hook
  entries;
- minimal read-only parsing of the named entry in `~/.claude.json` for inert MCP
  collision detection, taking care never to expose adjacent credentials or
  session state; `claude mcp list/get` are host/runtime checks because approved
  servers may be health-checked and launched;
- `claude plugin list` and `claude plugin details` only if a future adapter owns
  plugins; and
- `claude plugin validate <path> --strict` for an inert local plugin artifact.

The CLI reference explicitly documents `--version`, read-only `claude doctor`,
and the MCP/plugin command groups
([CLI reference](https://code.claude.com/docs/en/cli-usage)). None requires a
prompt or model call. A targeted MCP health check can still launch the configured
server process, so keep it out of inert preview. Do not use `claude -p` for
doctor/readiness. Treat actual MCP connection health, hook firing, skill loading in model context, and
interactive `/status`, `/hooks`, or `/mcp` observations as optional smoke-test
evidence, not automation that must run unauthenticated.

## Documented but unsuitable for the initial projection

### Plugins

Claude plugins can bundle namespaced skills, agents, hooks, MCP and LSP servers;
their documented schemas and `claude plugin validate --strict` are stable
integration points. However, installed marketplace plugins are copied to
`~/.claude/plugins/cache`, versioned and updated by Claude, and old versions are
garbage-collected later. Installation/trust is a distinct user decision, and
plugins may execute arbitrary code with user privileges. Since Packy already
has acquisition, provenance, consent, ownership, and lifecycle semantics,
silently mapping a Pack to a plugin would create two competing owners. Keep
plugins out of the first adapter unless a later decision explicitly chooses
Claude-owned plugin lifecycle
([plugin components and schema](https://code.claude.com/docs/en/plugins-reference),
[installation and security](https://code.claude.com/docs/en/discover-plugins#security)).

### Direct `~/.claude.json` mutation

Although Anthropic documents this as the storage location for user MCP servers,
the same file contains authentication/session, trust, per-project, and cache
state. Its whole-file schema is not published as a Pack extension contract.
Packy may read only the minimum needed for diagnosis if CLI output is
insufficient, but should not merge or rewrite it directly.

### Prompt/agent hooks and enforcement through instructions

Prompt and agent hooks call Claude models. They violate the no-model-call
validation boundary and introduce nondeterministic/runtime cost. Conversely,
`CLAUDE.md` and skills are instructions, not security controls. A pack that must
guarantee a guardrail needs an explicit reviewed command hook; it must not claim
enforcement from prompt text.

## Undocumented, experimental, or unsupported assumptions to reject

- Do not assume `~/.agents/skills` is a Claude Code discovery root. Anthropic's
  documented personal root is `~/.claude/skills`; Packy needs explicit Claude
  projections even if their targets point to the shared Packy bundle.
- Do not invent a `settings.d` user fragment directory or comments/markers
  inside JSON. Anthropic documents one `~/.claude/settings.json` user file.
- Do not depend on internal plugin cache layout as an installation API.
- Do not infer runtime loading merely because files exist. Higher-precedence
  settings, collisions, `disableAllHooks`, managed-only policies, or a restart
  requirement can make a configured projection unusable.
- Do not adopt fields merely because they appear in a current binary or
  changelog. Require current official reference documentation and an explicit
  version floor for version-gated fields.
- Do not treat `claude mcp add` as connection validation: Anthropic documents
  that it saves configuration without validating credentials. Static
  configured readiness and runtime connected readiness are different facts.

## Trust and ownership boundary for Packy

Packy may update or delete only artifacts it can prove it created and that still
match recorded intent:

- skill symlinks: exact path, symlink type, resolved target, and expected source;
- `CLAUDE.md`: exact unique Packy marker block, preserving all other text;
- settings hooks: exact canonical handler entries plus their fingerprints,
  preserving the containing file and all foreign entries;
- MCP: exact server name plus command/args/env identity as observed through the
  official CLI; and
- plugins, if ever adopted: exact `plugin@marketplace`, scope, source, and
  version, with explicit trust/consent.

Any unknown file, duplicate marker, changed target, altered hook, name collision,
or different MCP definition is a preservation/reporting case. Higher-precedence
project/local/managed configuration is foreign authority, not drift Packy may
repair.

## Decisions and fog this unlocks

This inventory makes the following next questions sharp:

1. **Choose the Claude projection contract:** direct user-global skills + marked
   `CLAUDE.md` + user MCP + selectively merged hooks (recommended), or a
   Claude-plugin lifecycle.
2. **Specify collision and composition rules:** skill names, one marked global
   instruction block, hook-entry identity/order, and same-name MCP shadowing.
3. **Map each current Pack resource:** `skill` maps natively; `instruction` maps
   to the marked user `CLAUDE.md`; `mcp_server` maps through `claude mcp`; the
   existing opaque `lifecycle` kind needs an event-specific contract before it
   may create a hook.
4. **Define readiness tiers:** configured static evidence versus effective host
   policy versus authenticated/runtime-loaded evidence. The required CI/smoke
   boundary can prove the first two without model calls, but not actual model
   consumption of instructions.
5. **Lock the minimum version:** 2.1.203 for symlink-backed skills, unless the
   implementation chooses copies and establishes another tested floor.
6. **Decide hook consent:** command hooks execute with full user privileges and
   should require capability-specific review/consent beyond ordinary inert
   skill projection.
