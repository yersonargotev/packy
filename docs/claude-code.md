# Claude Code

Packy supports stable Claude Code **2.1.203 or newer** alongside Codex and
OpenCode. Packy does not install or upgrade Claude Code.

## Prerequisite

Install Claude Code 2.1.203 or newer before activating a Pack on the `claude`
surface. A missing or unsupported executable is reported as a readiness
problem; it does not authorize Packy to change the installation.

## Global projections

Depending on the selected Pack resources, Packy may manage:

| Path or entry | Projection |
| --- | --- |
| `~/.claude/skills/<name>` | Pack skill or command. |
| `~/.claude/CLAUDE.md` | Deterministic content inside the marked Packy block. |
| `~/.claude/agents/<name>.md` | Pack agent definition. |
| `~/.claude/settings.json` | Typed command-hook entries. |
| user-scoped MCP entry | Exact entry changed through the official Claude CLI. |

Packy never writes `~/.claude.json` directly and does not manage Claude
plugins, caches, authentication, model selection, or repository-local Claude
configuration.

## Explicit activation

Initialization and catalog inspection do not activate Claude resources:

```sh
packy init
packy pack show engram
packy pack activate engram --surface claude --dry-run
packy pack activate engram --surface claude
packy pack status engram --surface claude
```

Application writes an installed Pack receipt before reporting success. Later
operations use the receipt to check exact ownership, drift, and collisions.

## Preservation and cleanup

Packy preserves foreign, changed, or ambiguous skills, instructions, agents,
hooks, and MCP definitions. Ordinary update or deactivation stops before
changing a drifted receipt-owned path. An explicit force option remains limited
to the targeted receipt.

`packy pack deactivate <pack> --surface claude` removes only unchanged
receipt-owned projections. It never deletes credentials, Engram memory,
unrelated host configuration, or external data.

## Readiness

Configured projections do not prove that Claude has loaded or authorized them.
Inspect current status explicitly:

```sh
packy pack status matty --surface claude
packy pack status matty --surface claude --require usable
```

`--require usable` exits nonzero until required host observations are ready.

## No authentication or model calls

`packy doctor` is read-only. It may resolve `claude`, run bounded
`claude --version`, and inspect named static files, but it never starts a
session, authenticates an account, invokes a model, or calls a paid API.

Real-Claude smoke checks use disposable `HOME`, `XDG_CONFIG_HOME`, and
`CLAUDE_CONFIG_DIR` values and remove credential-bearing environment variables.
