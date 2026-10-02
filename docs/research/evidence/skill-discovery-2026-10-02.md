# Skill discovery evidence, 2026-10-02

This is dated, non-normative evidence for issues
[#821](https://github.com/yersonargotev/packy/issues/821) and
[#824](https://github.com/yersonargotev/packy/issues/824). The accepted ADRs own
architecture; [the maintained discovery guide](../../skill-discovery.md) owns
the operator-facing behavior. These probes do not establish full acceptance of
every workflow in #824.

## Executed host versions

| Host | Binary exercised | Latest stable release found on the probe date |
| --- | --- | --- |
| Codex | `codex-cli 0.160.0` | [`rust-v0.160.0`](https://github.com/openai/codex/releases/tag/rust-v0.160.0) |
| Claude Code | `2.1.220` | [`v2.1.287`](https://github.com/anthropics/claude-code/releases/tag/v2.1.287) |
| OpenCode | `1.18.15` | [`v1.18.34`](https://github.com/anomalyco/opencode/releases/tag/v1.18.34) |

Binary versions came from actual sandboxed `--version` calls. Release tags
came from the GitHub release API. The installed Claude/OpenCode binaries were
older than the latest releases; no latest-version runtime acceptance is implied.
The workstation binaries were not upgraded.

## Isolation and fixture

Each probe used a disposable Git repository and explicit temporary `HOME`, XDG
roots, `TMPDIR`, `CODEX_HOME`, and `CLAUDE_CONFIG_DIR`. The Claude fixture's
personal skill root followed that explicitly configured disposable directory.
No workstation credentials or host configuration were copied into the fixture.

The fixture populated native global/project roots for all three hosts with
uniquely named probes and, for the duplicate-name checks, a `same-name` skill
in each root. Each body identified its host and scope and referenced
`references/evidence.txt`; each auxiliary file contained a corresponding unique
marker. Native roots and compatibility roots were all present at once, so a
positive native result alone could not hide unexpected cross-host discovery.

The Claude dispatch probe used an explicit fake API key and a localhost HTTP
recorder as `ANTHROPIC_BASE_URL`, with nonessential traffic disabled. The
recorder captured requests and deliberately returned HTTP 400. It was not a
model simulator and never returned a successful inference.

## OpenCode discovery

Ran the actual `opencode debug skill` command with default plugins disabled
and six distinct skills in native, Codex-compatible, and Claude-compatible
roots, across global and project scope.

- Default launch discovered all six probes and reported their bodies/locations.
- A launch with `OPENCODE_DISABLE_EXTERNAL_SKILLS=1` discovered only the two
  native OpenCode probes.
- Both commands exited zero on `1.18.15` without model credentials.

This establishes process-specific root isolation and compatibility discovery.
It does not establish a same-name winner, successful skill invocation,
auxiliary consumption, or persistent isolation of arbitrary user launches.

The [stable skill documentation](https://opencode.ai/docs/skills/) lists native
and compatibility roots and recommends unique names across locations. Tagged
[`v1.18.34` discovery source](https://github.com/anomalyco/opencode/blob/v1.18.34/packages/opencode/src/skill/index.ts)
scans external roots unless disabled, retains native/configured scans, and
combines concurrent loading with duplicate-name replacement. This does not
provide a reliable duplicate winner. The
[tagged runtime flag](https://github.com/anomalyco/opencode/blob/v1.18.34/packages/opencode/src/effect/runtime-flags.ts)
is a launch requirement, not a persistent configuration setting. V2 website
precedence documentation was not treated as evidence for a stable binary.

## Codex native discovery

Generated the installed app-server JSON schemas and launched
`codex app-server --stdio`. Sent `initialize`, `initialized`, and `skills/list`
with the fixture project cwd and `forceReload: true`, following the
[official app-server protocol](https://learn.chatgpt.com/docs/app-server).

- The response identified the disposable `CODEX_HOME`.
- Both native global/project probe names had the expected paths/descriptions.
- Both native `same-name` definitions appeared, with user/repository scopes.
- No Claude or OpenCode probe appeared; the skill error list was empty.
- No thread, turn, or model request was started.

This verifies native discovery on `0.160.0`. It does not verify body execution
or auxiliary reads. The
[official skill-location contract](https://learn.chatgpt.com/docs/build-skills#where-codex-loads-local-skills)
describes native `.agents/skills` roots and duplicate definitions without
promising a duplicate winner.

## Claude discovery, dispatch, and auxiliary expansion

Started normal print mode with stream-json input/output and sent the SDK
`control_request` initialization message. Startup reported `tokenSource: none`
and the explicit environment API-key source. Native command inventory included
only Claude global/project probes, plus both Claude `same-name` descriptors.
Neither Codex nor OpenCode probes appeared.

A separate direct `/same-name` dispatch to the localhost recorder expanded the
personal/global Claude body into the outbound user message with its correct
skill base directory. This demonstrates actual body selection before
inference, consistent with the documented personal-over-project rule. The
command exited one because the recorder intentionally rejected the request.

To observe a real auxiliary read without simulating inference, the disposable
global fixture declared `allowed-tools: Bash(cat *)` and appended the documented
native dynamic-context expression:

```markdown
Auxiliary content: !`cat ${CLAUDE_SKILL_DIR}/references/evidence.txt`
```

The first invocation failed its permission check because the global skill was
outside the project's allowed directories. A repeat with explicit
`--add-dir <fixture>/claude/skills` allowed the native read. The recorder then
captured `Auxiliary content: claude-global auxiliary` inside the expanded body.
This is evidence of a real body-relative auxiliary read under that explicit
permission baseline, not proof that ordinary launches grant the same access.
No successful inference or completed skill workflow occurred.

In this installed version, `--bare` initialization omitted custom skills from
inventory. Discovery assertions therefore used normal mode with isolated
configuration and fake local-only authentication, not `--bare` output.

Sources: [official Claude skill locations and precedence](https://code.claude.com/docs/en/skills#choose-where-skills-load),
[dynamic context](https://code.claude.com/docs/en/skills#inject-dynamic-context),
[SDK command inventory/dispatch](https://code.claude.com/docs/en/agent-sdk/slash-commands),
and [SDK initialization protocol implementation](https://github.com/anthropics/claude-agent-sdk-python/blob/main/src/claude_agent_sdk/_internal/query.py).

## Acceptance boundary

| Claim | Evidence and limit |
| --- | --- |
| Native Codex/Claude discovery | Actual installed binaries; both scopes observed. |
| OpenCode native and compatibility discovery | Actual installed binary; both scopes observed. |
| OpenCode isolation flag | Only the explicitly configured probe process. |
| Claude body selection | Global body expanded before inference. |
| Claude auxiliary consumption | Global dynamic context with explicit additional-directory permission. |
| Codex/OpenCode auxiliary consumption | Not verified. |
| Claude project auxiliary consumption | Not verified. |
| Latest stable Claude/OpenCode behavior | Source/docs evidence only; binaries not exercised. |
| Successful model-backed workflows | Not performed for any host. |
| All worktrees, configured custom roots, or launch contexts | Not established by these fixtures. |

Separate native targets can preserve Packy-owned trees while a host still
loads unintended compatibility content. The evidence therefore supports
retaining the explicit divergent OpenCode coexistence block, including against
Claude roots, in both installation orders and scopes. A different directory
basename does not change the frontmatter identity. File projection, lifecycle,
update/removal, and ownership tests remain distinct from real host invocation
acceptance; they cannot fill the unverified cells above.

Raw probe output and helper scripts were temporary working material. This
record retains the fixture design, operations, observed results, and limits
without making disposable machine-specific paths a required artifact.
