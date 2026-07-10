# Trust and consent boundaries for capability packs

Research snapshot: **2026-07-10**. Scope is the initial `matty` and `engram` packs on Codex CLI and OpenCode only; web and mobile remain deferred. Local checks used sandboxed `HOME`, `XDG_CONFIG_HOME`, `XDG_CACHE_HOME`, `XDG_DATA_HOME`, and `CODEX_HOME` with Codex CLI 0.144.1, OpenCode 1.17.18, and Engram 1.19.0.

## Executive answer

A pack activation is consent to a **declared, previewed Matty reconciliation**, not blanket consent to execute everything the pack mentions. Matty may validate and write its own deterministic files/config after explicit activation, but it must preserve host trust gates, host execution permissions, credential ownership, and a separate human checkpoint for actions that execute unreviewed code, authenticate an account, grant external access, weaken policy, overwrite non-owned state, or irreversibly remove credentials/data.

This produces three distinct states that must not be collapsed:

1. **Configured** — Matty-owned files/config are reconciled.
2. **Authorized/trusted** — the human completed any host-native hook review, OAuth/account connection, external-tool installation, or explicit high-risk setup approval.
3. **Usable** — required processes restart successfully and the capability is available under the host's runtime permission policy.

A pack can therefore be active but `pending human action`; it must not be reported fully usable merely because config was written.

## Evidence-backed host boundaries

### Codex

- Codex separates the **sandbox** (what execution can technically touch) from the **approval policy** (when execution must stop and ask). Local defaults deny network and constrain writes to the active workspace; editing outside it or networking normally requires approval. Destructive app/MCP calls require approval when correctly annotated as destructive. Matty must not broaden these settings as a side effect of pack activation. [Agent approvals & security](https://learn.chatgpt.com/docs/agent-approvals-security)
- Project `.codex/config.toml` is loaded only for trusted projects. Pack installation must not mark a project trusted or use a user-global projection to evade that boundary. [Config basics](https://learn.chatgpt.com/docs/config-file/config-basic)
- Non-managed command hooks require review of the **exact hook-definition hash**. New or changed hooks are skipped until trusted; installing/enabling a plugin does not trust its bundled hooks. `/hooks` is the user review/disable surface. The bypass flag is explicitly dangerous and is not a valid Matty activation mechanism. [Codex hooks](https://learn.chatgpt.com/docs/hooks)
- Codex MCP supports stored OAuth, bearer-token environment variables, static headers, per-server/per-tool approval modes, enablement, and allow/deny tool lists. OAuth begins separately through `codex mcp login`; a server definition can exist without credentials. Plugin-provided MCP servers remain controllable by user config. Matty must keep definition, authentication, and tool-execution approval separate. [Codex MCP](https://learn.chatgpt.com/docs/extend/mcp?surface=cli)
- Plugins can expose MCP services that access external systems and hooks that execute commands; official docs tell users to connect/authenticate when prompted and separately review/trust plugin hooks. Pack opt-in is thus not equivalent to either account authorization or hook trust. [Codex plugins](https://learn.chatgpt.com/docs/plugins)

### OpenCode

- OpenCode permissions are ordered `allow`/`ask`/`deny` rules and cover shell, edits, skills, MCP/custom tools, and external-directory access. Most defaults are permissive, while `external_directory` and repeated identical calls default to `ask`; `.env` reads are denied by default. Matty must not infer safety from “OpenCode loaded it,” nor inject `allow` rules to suppress consent. [OpenCode permissions](https://opencode.ai/docs/permissions/)
- Local JS/TS plugin files are automatically loaded at startup. Configured npm plugins are automatically installed by Bun and cached, and every plugin hook runs in sequence. Unlike Codex hook trust, no documented per-definition hash review protects this boundary. Therefore adding a plugin file/package is itself the executable-code consent boundary and requires an explicit preview/confirmation before Matty writes the projection; activation cannot silently add executable OpenCode plugins. [OpenCode plugins](https://opencode.ai/docs/plugins/)
- Remote MCP OAuth starts automatically after an authentication challenge or manually with `opencode mcp auth`; the browser authorization is interactive, tokens persist in `~/.local/share/opencode/mcp-auth.json`, and `logout` removes them. MCP tools are governed through normal tool permissions. Matty may configure the server, but authentication and logout remain separate user actions. [OpenCode MCP servers](https://opencode.ai/docs/mcp-servers/), [OpenCode permissions](https://opencode.ai/docs/permissions/)
- Provider credentials entered through `/connect` persist separately in `~/.local/share/opencode/auth.json`. Packs must neither read nor claim ownership of that file merely because their tools run in OpenCode. [OpenCode providers](https://opencode.ai/docs/providers/)

## Required Matty policy

### Safe after explicit pack activation

Matty may perform these deterministic operations without a second prompt, provided the activation plan already displayed exact destinations and ownership:

- validate the strict `pack.json`, dependency/conflict rules, and pack-root-contained sources;
- create/update Matty-owned `skill` and `instruction` projections;
- merge a disabled or least-privilege `mcp_server` definition without authenticating it or invoking tools;
- record Matty-owned desired/observed state and report restart or authorization requirements;
- remove only unchanged Matty-owned projections on deactivation, retaining shared resources still required by another pack.

This is a design conclusion, not a host guarantee: it follows from separating deterministic reconciliation from execution, trust, and credentials.

### Human intervention required

Matty must stop and present the concrete action, actor, command/path, network destination, and rollback before:

1. **First activation or reactivation of any pack** — all packs are opt-in; show resources, affected hosts/paths, executable surfaces, external tools, and pending auth/trust.
2. **Executable OpenCode projection** — adding/updating a JS/TS or npm plugin, because presence/config causes code loading/installation without a Codex-like hook review.
3. **Codex hook trust** — leave the hook pending and direct the human to `/hooks`; never synthesize trust state or use `--dangerously-bypass-hook-trust`. Changed hook content requires renewed host review.
4. **Authentication or account connection** — OAuth/browser login, provider `/connect`, token/API-key entry, or choosing scopes/accounts. Matty must not capture secrets in manifests, logs, plans, or state.
5. **External tool acquisition/setup** — package-manager install, downloaded executable, privileged command, network fetch, or a tool-owned installer such as `engram setup`. Show the resolved executable/source/version and exact command first. A declared global tool requirement permits detection, not installation.
6. **Policy weakening or broadened reach** — changing sandbox/approval modes, OpenCode permission rules, external-directory access, network allowlists, enabled MCP tools, or approval modes toward greater authority.
7. **Ownership conflict or destructive cleanup** — overwriting a non-Matty file/config member, adopting an existing resource with different content, deleting user-modified state, logging out/removing credentials, deleting external-tool data, or uninstalling a shared executable.

### Credentials and secrets

- `pack.json` may name a tool requirement or environment-variable **name**, but must never contain secret values, OAuth tokens, client secrets, or copied host credential blobs.
- Matty state records only that authorization is `missing/present/unknown` when this can be obtained through a host command; it does not read credential stores directly or back them up.
- Deactivation removes the Matty-owned definition but does **not** imply logout. Credential removal is a separate explicit action using the host/tool owner (`codex mcp logout`, `opencode mcp logout`, etc.).
- Environment references remain references. Matty must not print resolved values in previews or diagnostics.

## Initial-pack application

### `matty`

The initial `matty` pack contains skills plus one behavioral instruction and no need for authentication. Its normal activation can be a single explicit, previewed reconciliation. Any future lifecycle projection still follows the host-specific executable boundary above; it does not inherit trust merely because the instruction is called `matty`.

### `engram`

The `engram` pack requires the global `engram` executable and contributes the persistent-memory instruction, MCP server declaration, and lifecycle intent.

- Detecting `engram` and reconciling an MCP definition is non-interactive.
- If the binary is absent, acquisition requires human approval; Matty must not silently install it.
- `engram setup <host>` is a tool-owned mutation of host configuration and therefore requires preview/approval rather than being an automatic consequence of the tool requirement.
- The lifecycle projection is pending until Codex hook trust is completed; an OpenCode executable plugin projection requires confirmation before it is installed/written.
- Local `engram mcp --tools=agent` needs no OAuth by default, but its memory database remains Engram-owned. Pack deactivation must not delete `~/.engram` or user memories.
- Optional Engram cloud enrollment/sync and its tokens are outside initial pack activation; they are separate network/account consent.

These Engram statements combine verified CLI behavior (`engram --help`, 1.19.0) with ownership conclusions derived from the portable pack contract.

## Sandboxed verification

With temporary HOME/XDG/Codex directories and OpenCode override variables unset:

- `codex mcp --help` exposed distinct `add/remove/login/logout` commands.
- `opencode mcp --help` exposed `add/list/auth/logout/debug` and `--pure`; auth and credential removal are distinct from definition management.
- `engram --help` identified `setup [agent]` as installation/setup, `mcp` as the stdio server, `ENGRAM_DATA_DIR` defaulting to `~/.engram`, and cloud sync/enrollment as opt-in operations with separate token/server settings.
- The read-only checks created no files under the sandbox roots. No operator configuration or credentials were accessed.

## Verified versus inferred

**Verified:** the linked host trust/permission/authentication behavior, automatic OpenCode plugin loading/install, CLI command separation, and sandboxed command output above.

**Inferred design boundary:** the exact Matty preview model, configured/authorized/usable states, non-destructive deactivation behavior, and which deterministic writes may follow one activation confirmation. These are conservative requirements derived from host behavior and the already-decided opt-in, host-adapter, Matty-owned-state architecture; they are not claims that either host implements a pack transaction.
