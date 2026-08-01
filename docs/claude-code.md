# Claude Code

Packy supports Claude Code as a first-class user-global CLI surface alongside
Codex and OpenCode. Claude Code **2.1.203 or newer stable** is required; Packy
does not install or upgrade Claude Code. A missing, older, prerelease,
unreadable, or timed-out executable is a pending prerequisite, not a failed
Packy effect. Safe Codex/OpenCode and local Claude work may still converge;
rerun the explicit `packy pack update <pack> --surface claude` or activation
preview after installing a supported stable version.

## Prerequisite

The prerequisite is stable Claude Code 2.1.203 or newer, as described above.

## Global projections and layout

Packy uses only Claude Code's user-global surface:

| Path or entry | Packy projection |
| --- | --- |
| `~/.claude/skills/<name>` | Owned directory symlink to the complete skill or command source tree in Packy's Installed Source. Commands use personal skills, not legacy `.claude/commands`. |
| `~/.claude/CLAUDE.md` | Deterministic contributions inside one marked outer Packy block. |
| `~/.claude/agents/<name>.md` | Exact owned agent file with explicit native tool and permission translations. |
| `~/.claude/settings.json` | Canonical, typed command-hook entries only. |
| user-scoped MCP entry | Added or removed only with official `claude mcp ... --scope user` commands. |

Packy may statically inspect the named entry in `~/.claude.json`, but **never
writes that file directly**. It does not use Claude plugins, plugin
marketplaces, caches, repository-local `CLAUDE.md`/`.claude` configuration, or
opaque hook injection.

## Explicit activation and incompatible cutover

Initialization and discovery never create Claude intent. Preview and activate
each chosen pack or resource selection explicitly for Claude:

```sh
packy init
packy pack show engram
packy pack activate engram --surface claude --dry-run
packy pack activate engram --surface claude
packy doctor
```

The removed classic lifecycle has no migration or compatibility path. Packy
does not read, adopt, translate, or delete its old state or artifacts. The sole
current operator removes those artifacts manually, initializes Packy afresh,
and then activates only the desired packs and surfaces. A leftover classic
artifact is unowned and normal collision protection preserves it.

Inspection and `--dry-run` are inert. Application rereads shared state and
documents before effects; a stale plan performs no unstarted work. Repeat the
originating command to obtain a fresh plan rather than replaying a failed one.

## Preservation, recovery, and cleanup

Ownership is based on Packy's recorded exact fragment identity and fingerprint,
not merely matching bytes. Packy preserves foreign, changed, duplicate, or
ambiguous skills, instructions, agents, hooks, and MCP definitions. It does not
adopt or migrate foreign Claude configuration. Shared contributions remain
until the last recorded contributor leaves.

A collision or invalid shared document is a blocker and does not by itself
mean recovery is required. A failed local effect that restores its exact prior
state is reported as rolled back. Recovery is required only after an attempted
effect fails and Packy cannot prove the complete intended prior or desired
state; repeat the originating verb for fresh inspection and approval.

`packy pack deactivate <pack> --surface claude` removes only unchanged, exactly
recorded projections after the applicable destructive consent and keeps state
while residual ownership remains. If Claude is unavailable, Packy preserves
user MCP ownership for a later fresh reconciliation; an attempted MCP removal
failure is recovery-required. Cleanup never deletes credentials, Engram memory,
foreign configuration, external data, or an unproven shared container.

## Capability packs: compatibility and readiness

Claude support is explicit only in manifest v3. Every v3 runtime resource has
exactly one Claude binding or exclusion. Compatibility is computed before
readiness:

- **complete**: all required resources have native bindings with no degradation;
- **degraded**: mandatory behavior remains coherent but an accepted degraded
  binding or optional exclusion exists;
- **blocked**: a mandatory outcome is absent, excluded, collided, stale, or
  otherwise unsafe to apply.

Preview shows bindings, exclusions, preservation, compatibility, expected
readiness, and plan-bound consent. Typed command hooks require
executable/external consent; last-contributor MCP or hook removal requires
destructive-cleanup consent. No generic `--yes` bypass exists.

Configuration is not usability. **Configured** means exact projections are
present; **authorized** also requires supported version and observable policy
or tool permission; **usable** additionally requires an explicit current
loading signal for skills, commands, agents, or instructions, connection
evidence for MCP, and firing evidence for hooks. Projection, policy,
definition, Pack-version, or Claude-version changes invalidate affected
evidence. Check it explicitly:

```sh
packy pack status matty --surface claude
packy pack status matty --surface claude --require usable
```

`--require usable` exits nonzero until all required signals are freshly known
true. Packy never manufactures runtime evidence from filesystem correctness.

## No authentication or model calls

This support does not install Claude Code; authenticate or log in; invoke its
REPL or print/model modes; call a model or paid API; configure Claude Desktop,
Claude web, or the Anthropic API/SDK; or manage repository-local configuration.
It does not translate generic lifecycle, prompt, agent, HTTP, or MCP-tool hooks.

`packy doctor` is read-only. It may resolve `claude`, run bounded
`claude --version`, and inspect named static files, but it never starts a
session or MCP server. Packy's real-Claude smoke likewise uses disposable
`HOME`, `XDG_CONFIG_HOME`, and `CLAUDE_CONFIG_DIR`, removes credentials and
provider variables, and permits only bounded version plus controlled
user-scoped MCP operations. No authentication or model call is evidence for
this feature.
