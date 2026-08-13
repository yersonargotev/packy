# Audit of `engram setup codex`

**Date:** 2026-08-13
**Question:** What does the current setup install or inject into Codex, and which
pieces should remain in a Packy/Codex configuration?

## Scope and method

This is an execution-backed audit, not a reading of an older integration
contract.  `engram 1.20.0` from Homebrew was executed once in a fresh temporary
`HOME`, `XDG_CONFIG_HOME`, `XDG_CACHE_HOME`, and `CODEX_HOME`; the real
`~/.codex` was never modified. The command completed successfully and reported
that it installed the Codex plugin and three direct files. The temporary Codex
CLI then cloned the marketplace at commit
[`509e676`](https://github.com/Gentleman-Programming/engram/tree/509e6762fdd9417ff7a39d30f426a9566220eaf0).

At the time of the audit, the real `~/.codex/config.toml` has no Engram MCP,
marketplace, plugin, instruction-file, or compact-prompt entries, and
`codex plugin list` does not list Engram. Thus the overload described below is
the effect of running the command, not a claim that it is currently active on
this machine.

The distinction matters: the binary provides the first three writes, but it
asks Codex to install `engram@engram` from Git `main`; plugin behaviour can
therefore change independently of the installed binary. The installer itself
does exactly those two CLI calls after writing the three direct resources
([source](https://github.com/Gentleman-Programming/engram/blob/509e6762fdd9417ff7a39d30f426a9566220eaf0/internal/setup/setup.go#L1157-L1210)).

Evaluation criteria:

1. It is necessary when it is the smallest reliable way to retain useful,
   durable *project* knowledge across sessions.
2. It is redundant when Codex already provides the mechanism or a selective
   CLI invocation gives the same outcome.
3. It is overload when it globally replaces Codex behaviour, forces tools or
   writes on routine work, captures all prompts/output, starts background
   processes, or introduces an unpinned supply-chain/runtime dependency.
4. A component may be conditional when a user deliberately opts into the
   larger automatic-memory product and accepts those trade-offs.

## Observed installation inventory

| Component | Exact effect | Execution model | Assessment | Decision |
| --- | --- | --- | --- | --- |
| Engram executable | No executable is installed by this command; the existing `engram` binary is referenced by absolute path. | Codex launches `engram mcp --tools=agent` as an MCP stdio child. | Necessary only for whichever selected memory workflow calls the CLI/MCP. | **Keep, conditional.** Keep the reviewed executable acquisition, not host setup. |
| Direct MCP registration | Adds/replaces `[mcp_servers.engram]` with `command = "/opt/homebrew/bin/engram"`, `args = ["mcp", "--tools=agent"]`. | Makes the agent MCP profile available in every Codex session. | It is broader than selective recall/save. The plugin also ships an equivalent `.mcp.json`: two declarations exist, although this audit does not assume that Codex launches two MCP processes. | **Remove** for selective memory; **conditional** only for explicit full MCP mode. |
| Global model instruction replacement | Writes `~/.codex/engram-instructions.md` (3,769 bytes) and sets `model_instructions_file` to it. | Replaces Codex's built-in model instructions with Engram's mandatory save/search/session protocol. | High risk and redundant with `AGENTS.md`/skills. OpenAI's config type explicitly says this field overrides built-in instructions and strongly discourages it because performance can degrade ([type](https://github.com/openai/codex/blob/ef596c68ca35dcd42283a0014f9b7fb057147269/codex-rs/config/src/config_toml.rs#L233-L237), [loader](https://github.com/openai/codex/blob/ef596c68ca35dcd42283a0014f9b7fb057147269/codex-rs/core/src/config/mod.rs#L3874-L3889)). | **Remove.** |
| Global compact-prompt replacement | Writes `~/.codex/engram-compact-prompt.md` (470 bytes) and sets `experimental_compact_prompt_file`. | Replaces the compact prompt to force an Engram summary, then a future `mem_context` call. | It replaces rather than augments the compaction policy, while both the instruction file and post-compaction hook repeat the same protocol. | **Remove.** |
| Marketplace | Adds `[marketplaces.engram]` pointing to `https://github.com/Gentleman-Programming/engram.git`, `ref = "main"`; the isolated run cloned about 78 MB including `.git` (about 51 MB without it). | Each install/update resolves mutable upstream `main`. | Unpinned mutable code plus a material local footprint. It is only needed to obtain the plugin. | **Remove.** |
| Plugin package | Enables `[plugins."engram@engram"]`; the cached package was 56 KB and contains a skill, MCP declaration, five hooks, and shell helpers. | Codex loads the plugin's skill and MCP declaration; the plugin declares its hook and MCP files ([manifest](https://github.com/Gentleman-Programming/engram/blob/509e6762fdd9417ff7a39d30f426a9566220eaf0/plugin/codex/.codex-plugin/plugin.json#L1-L17)). | The skill duplicates the global instructions; the MCP declaration duplicates the direct MCP registration; hooks add the rest of the automatic system. | **Remove.** |
| Session-start hook | Runs at startup/resume/clear (10 s budget). It starts `engram serve` if unavailable, migrates a project name, creates a session, may asynchronously import `.engram` Git chunks, fetches context, and injects another mandatory protocol. | HTTP server plus HTTP API, not only MCP stdio. ([hook declaration](https://github.com/Gentleman-Programming/engram/blob/509e6762fdd9417ff7a39d30f426a9566220eaf0/plugin/codex/hooks/hooks.json#L1-L25), [implementation](https://github.com/Gentleman-Programming/engram/blob/509e6762fdd9417ff7a39d30f426a9566220eaf0/plugin/codex/scripts/session-start.sh#L1-L180)). | Automatic server lifecycle, data import, project migration, and repeated injected instructions are beyond selective durable memory. | **Remove.** |
| User-prompt hook | Runs for every prompt (2 s budget). On the first one it forces `ToolSearch` for 13 memory tools and `mem_context`; on every prompt it posts the raw prompt to localhost asynchronously. Later it queries the local server and injects a save reminder after 15 minutes. | Per-prompt commands, background prompt capture, and nudge state in `/tmp`. ([hook declaration](https://github.com/Gentleman-Programming/engram/blob/509e6762fdd9417ff7a39d30f426a9566220eaf0/plugin/codex/hooks/hooks.json#L27-L36), [implementation](https://github.com/Gentleman-Programming/engram/blob/509e6762fdd9417ff7a39d30f426a9566220eaf0/plugin/codex/scripts/user-prompt-submit.sh#L1-L48) and [tool forcing/nudge](https://github.com/Gentleman-Programming/engram/blob/509e6762fdd9417ff7a39d30f426a9566220eaf0/plugin/codex/scripts/user-prompt-submit.sh#L87-L209)). | Clear overload: it loads far more tools than a task needs, captures every prompt, and adds a mandatory behavioural nudge. | **Remove.** |
| Post-compaction hook | Runs on compact (10 s budget); creates/ensures a session over HTTP, reads context, and injects the protocol and ordered recovery instructions. | Overlaps the compact-prompt replacement and the model instructions. ([source](https://github.com/Gentleman-Programming/engram/blob/509e6762fdd9417ff7a39d30f426a9566220eaf0/plugin/codex/scripts/post-compaction.sh#L1-L88)). | Triple implementation of one concern; it still depends on a server that is not the MCP child. | **Remove.** |
| Subagent-stop hook | Runs synchronously for every subagent stop (10 s budget) and POSTs its full last output to passive capture. | Automatic derived-memory write over localhost. ([declaration](https://github.com/Gentleman-Programming/engram/blob/509e6762fdd9417ff7a39d30f426a9566220eaf0/plugin/codex/hooks/hooks.json#L38-L48), [implementation](https://github.com/Gentleman-Programming/engram/blob/509e6762fdd9417ff7a39d30f426a9566220eaf0/plugin/codex/scripts/subagent-stop.sh#L1-L41)). | Cost/latency and unreviewed capture of subagent output do not meet selective-memory criteria. | **Remove.** |
| Stop hook | Runs synchronously on session stop (5 s budget) and calls `/sessions/{id}/end`. | Session bookkeeping over HTTP. ([source](https://github.com/Gentleman-Programming/engram/blob/509e6762fdd9417ff7a39d30f426a9566220eaf0/plugin/codex/scripts/session-stop.sh#L1-L25)). | Only useful to the automatic session system, not to targeted recall/save. | **Remove.** |
| Plugin skill | Declares an “always active” protocol: immediate proactive saves, automatic searches, mandatory close summary and compaction recovery. | Adds another instruction channel, overlapping both direct instruction file and hook output. ([source](https://github.com/Gentleman-Programming/engram/blob/509e6762fdd9417ff7a39d30f426a9566220eaf0/plugin/codex/skills/memory/SKILL.md)). | Useful content is the idea of structured project memories; automatic and mandatory policy is too broad. | **Replace** with the reviewed selective `engram-memory` skill. |

### Configuration mutation details

The direct installer overwrites the two global file paths and removes/re-adds
the `mcp_servers.engram` TOML block on every run
([MCP upsert](https://github.com/Gentleman-Programming/engram/blob/509e6762fdd9417ff7a39d30f426a9566220eaf0/internal/setup/setup.go#L1213-L1300),
[instruction/compact upsert](https://github.com/Gentleman-Programming/engram/blob/509e6762fdd9417ff7a39d30f426a9566220eaf0/internal/setup/setup.go#L1231-L1330)).
It does not merge Engram guidance with the user's existing model or compaction
guidance. This is especially important here because the current Codex
configuration has its own model, plugins, MCP servers and global `AGENTS.md`.

## Model of the resulting runtime

```text
every Codex session
  ├─ direct config: start `engram mcp --tools=agent` (MCP stdio)
  ├─ global model instructions: mandate Engram behaviour
  ├─ global compact prompt: mandate summary -> later memory recovery
  └─ enabled plugin
       ├─ start/resume/clear: start `engram serve`, create/migrate/import/fetch
       ├─ every prompt: persist raw prompt; first prompt forces 13 tools
       ├─ compact: fetch context and re-inject the same protocol
       ├─ each subagent end: passively capture full output
       └─ stop: close HTTP session
```

The stdio-only claim and this actual plugin runtime are not equivalent. The
Engram README says Codex normally starts `engram mcp` as a short-lived stdio
subprocess and says `engram serve` is unnecessary for stdio-only agents
([README](https://github.com/Gentleman-Programming/engram/blob/509e6762fdd9417ff7a39d30f426a9566220eaf0/README.md)).
The installed Codex plugin nevertheless starts and uses `engram serve` for its
hooks. That is a source-backed discrepancy, not an assumption.

## What should remain

The recommended target is **selective project memory, invoked by the agent
only when relevant**:

| Keep | Why | Operating limit |
| --- | --- | --- |
| The reviewed `engram` executable, acquired only when a user installs/enables the Engram Pack. | It supplies local SQLite-backed search/save when a durable project fact can materially change current work. | No `engram setup`; no server auto-start. |
| The Packy `engram-memory` skill and its helper. | It performs one narrow project-scoped CLI search before work only when prior knowledge could change the approach, and at most one save after a durable result. | Best effort; CLI failure never blocks the primary task. |
| Explicit `engram search` / `engram save --project …` calls made by that skill. | Delivers the core value without 13 MCP tools, session lifecycle, prompt capture, or global prompt replacement. | Project facts only; no personal/cross-project memory. |

This recommendation matches the accepted Packy architecture: external
executable acquisition never authorizes a tool to configure a CLI surface, and
the current Engram mode is deliberately a selective Codex skill rather than
MCP retrieval, lifecycle automation, or compaction recovery
([ADR 0036](../../adr/0036-separate-executable-acquisition-from-host-setup.md)).

## What should not remain

Do **not** run `engram setup codex` in the managed Packy/Codex setup, and
remove/avoid all resources it owns:

- `[mcp_servers.engram]`, `model_instructions_file` pointing at
  `engram-instructions.md`, and `experimental_compact_prompt_file` pointing at
  `engram-compact-prompt.md`.
- `[marketplaces.engram]` and `[plugins."engram@engram"]`.
- the two generated Markdown files and the cached Engram marketplace/plugin
  only after confirming no other deliberately installed workflow uses them.
- automatic `engram serve`, auto-import, project migration, prompt/output
  capture, all lifecycle hooks, forced ToolSearch, and save nudges.

## Conditional full-memory mode

Full MCP mode is defensible only if a user explicitly wants automatic session
tracking, raw prompt/subagent-output capture, compaction recovery, and Git
chunk import, accepts a local HTTP server and mutable marketplace code, and
can validate hook behaviour against the exact Codex release. It should be a
separate ADR and an opt-in pack/surface, with a pinned reviewed plugin revision
and a privacy/retention policy. It must not replace global model instructions
or Codex's compaction prompt.

## Risks and migration checks

1. **Functional:** Removing the setup stops automatic recalls/saves and
   compaction restoration. The selective skill preserves only intentional,
   durable project knowledge; this is the intended reduced scope.
2. **Instruction quality:** `model_instructions_file` is a replacement surface,
   not additive guidance. Leaving it installed can silently displace the
   optimised Codex instruction set.
3. **Privacy/data volume:** The prompt hook posts every user prompt, and the
   subagent hook posts full outputs, to a local service. Retention is now a
   product choice rather than an incidental side effect.
4. **Reliability/latency:** Five command hooks have 2–10 second budgets;
   several synchronously call HTTP. They can fail independently of MCP and
   have a different lifecycle from the MCP subprocess.
5. **Supply chain:** `ref = "main"` means a later `setup`/plugin refresh can
   alter scripts and instructions without a binary upgrade or Packy review.

Before removing live artifacts, inspect `~/.codex/config.toml`, list the
enabled Codex plugins, and confirm that the project pack's `engram-memory`
skill is active. Then start a fresh Codex session and verify: a targeted recall
works when relevant, a routine task does neither search nor save, and a
durable project decision produces one explicit project-scoped save.
