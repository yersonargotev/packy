# Claude Code

Packy supports stable Claude Code **2.1.203 or newer** alongside the shared Agents convention. Packy does not install or upgrade Claude Code.

## Prerequisite

Install Claude Code 2.1.203 or newer before activating a Pack on the `claude`
surface. A missing or unsupported executable is reported as a readiness
problem; it does not authorize Packy to change the installation.

## Global projections

Depending on the selected Pack resources, Packy may manage:

| Path or entry | Projection |
| --- | --- |
| `~/.claude/skills/<name>` | Complete reviewed Pack skill tree. |
| `~/.claude/CLAUDE.md` | Deterministic content inside the marked Packy block. |
| `~/.claude/agents/<name>.md` | Reviewed native custom-agent document. |

Packy does not project direct MCP servers, hooks, or commands, and it does not
convert retired command resources into skills. It leaves `~/.claude.json`,
`~/.claude/settings.json`, plugins, caches, authentication, and model selection
to the client and user.

## Project projections

Project installation uses `.claude/skills/<name>`, project `CLAUDE.md`, and
`.claude/agents/<name>.md`. Skills retain their complete trees, executable
modes, supporting files, metadata, and legal notices. Reviewed skill templates
and composite skills remain available for skill content.

```sh
packy install <pack> --surface claude --dry-run
packy install <pack> --surface claude
packy status <pack> --surface claude --project
packy verify
```

A required native agent stays in the selected dependency closure. Its binding
must explicitly declare client `claude` and the supported `global` and/or
`project` scopes in `client_projection`; selecting the Claude surface targets
that client without a separate `--client` option. An unsupported required scope
fails before mutation. Preview identifies the required resources, client, and
scope. Agent files retain normal ownership, drift protection, update, and removal.
Project intent and receipts belong in `packy.json` and `packy.lock.json`;
personal readiness evidence remains outside those committed files.

## Explicit activation

Initialization and catalog inspection do not activate Claude resources:

```sh
packy init
packy show engram
packy activate engram --surface claude --dry-run
packy activate engram --surface claude
packy status engram --surface claude
```

Application writes an installed Pack receipt before reporting success. Later
operations use the receipt to check exact ownership, drift, and collisions.

## Preservation and cleanup

Packy preserves foreign, changed, or ambiguous skills, instructions, and agents.
Ordinary update or deactivation stops before
changing a drifted receipt-owned path. An explicit force option remains limited
to the targeted receipt.

`packy deactivate <pack> --surface claude` removes only unchanged
receipt-owned projections. It never deletes credentials, Engram memory,
unrelated host configuration, or external data. For a project installation, use
`packy uninstall <pack> --surface claude` after reviewing its `--dry-run` preview;
it removes the targeted shared installation and its unchanged owned files.

## Readiness

Configured projections do not prove that Claude has loaded or authorized them.
Generated policy text expresses reviewed intent; it does not enforce runtime
permissions, isolation, or successful delegation.
Inspect current status explicitly:

```sh
packy status matty --surface claude
packy status matty --surface claude --require usable
```

`--require usable` exits nonzero until required host observations are ready.

## No authentication or model calls

`packy doctor` is read-only. It may resolve `claude`, run bounded
`claude --version`, and inspect named static files, but it never starts a
session, authenticates an account, invokes a model, or calls a paid API.

Real-Claude smoke checks use disposable `HOME`, `XDG_CONFIG_HOME`, and
`CLAUDE_CONFIG_DIR` values and remove credential-bearing environment variables.
