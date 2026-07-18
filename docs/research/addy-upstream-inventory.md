# Upstream inventory: `addyosmani/agent-skills`

## Research question

What exactly does the upstream `addyosmani/agent-skills` repository contain, how do its capability layers relate, and what provenance, licensing, versioning, executable behavior, and trust boundaries constrain safe redistribution as a Packy Pack Source?

## Immutable source identity

- Repository: [`addyosmani/agent-skills`](https://github.com/addyosmani/agent-skills)
- Default branch observed: `main`
- Inspected commit: [`98967c45a42b88d6b8fb3a88b7ff6273920763d6`](https://github.com/addyosmani/agent-skills/tree/98967c45a42b88d6b8fb3a88b7ff6273920763d6)
- Release/tag: `0.6.4`
- Commit timestamp: `2026-07-12T10:58:04-07:00`
- Commit subject: `Merge pull request #396 from nucliweb/docs/adoption-guide`
- Inspection date: 2026-07-18

All repository citations below are pinned to that commit. The live repository URL is used only where GitHub itself is the primary source for repository/release metadata.

## Method and safety boundary

I resolved the remote `HEAD` with `git ls-remote`, cloned without checking out a working branch, detached at the exact SHA above, and performed static inspection only. Inspection used `git ls-tree`, `git show`, `find`, `grep`, `sed`, `awk`, `jq`, file hashing, and read-only GitHub API calls through `gh api`.

I **did not execute** any upstream hook, command, installer, JavaScript validator/eval runner, skill helper, test, or CI workflow. In particular, none of the shell files under [`hooks/`](https://github.com/addyosmani/agent-skills/tree/98967c45a42b88d6b8fb3a88b7ff6273920763d6/hooks) or [`skills/idea-refine/scripts/`](https://github.com/addyosmani/agent-skills/tree/98967c45a42b88d6b8fb3a88b7ff6273920763d6/skills/idea-refine/scripts) was run.

The pinned tree contains 128 tracked entries representing about 695 KB of tracked file content. It has no Git submodules and no Git LFS pointers. It has one symlink, `.opencode/skills -> ../skills/`.

## Executive findings

1. Upstream is a **layered capability system**, not merely a skill collection: 24 skills, 4 agent personas, 8 logical commands with three host projections, one automatically registered Claude `SessionStart` hook, three additional opt-in hook scripts, one skill-local executable helper, seven shared references, host manifests/marketplace descriptors, host guidance, and validation/eval assets.
2. The intended composition is explicit: **commands are the user-facing/orchestration layer, personas are role/perspective, and skills are workflow/how**. Personas may use skills but must not invoke other personas; `/ship` is the one endorsed multi-persona fan-out ([agent relationship specification](https://github.com/addyosmani/agent-skills/blob/98967c45a42b88d6b8fb3a88b7ff6273920763d6/docs/agents.md#L12-L46)).
3. The reusable capability set is not identical on every host. Claude exposes skills, personas, slash commands, and its registered hook. Codex deliberately exposes only the 24 root skills and declares empty hooks; upstream says its slash commands and personas have no native Codex equivalent ([Codex setup](https://github.com/addyosmani/agent-skills/blob/98967c45a42b88d6b8fb3a88b7ff6273920763d6/docs/codex-setup.md#L21-L31)). OpenCode relies on `AGENTS.md`, its `skill` tool, and the `.opencode/skills` symlink rather than a plugin or native command projection ([OpenCode setup](https://github.com/addyosmani/agent-skills/blob/98967c45a42b88d6b8fb3a88b7ff6273920763d6/docs/opencode-setup.md#L5-L23)).
4. Redistribution is permitted under MIT, but copies or substantial portions must retain the copyright and permission notice ([LICENSE](https://github.com/addyosmani/agent-skills/blob/98967c45a42b88d6b8fb3a88b7ff6273920763d6/LICENSE)). Contributors also agree their contributions are MIT-licensed ([CONTRIBUTING](https://github.com/addyosmani/agent-skills/blob/98967c45a42b88d6b8fb3a88b7ff6273920763d6/CONTRIBUTING.md#L106-L116)).
5. A Pack Source must use the exact commit as its source identity. The stable source identity is the GitHub release/tag `0.6.4` at the pinned commit. However, `plugin.json` and `.codex-plugin/plugin.json` both say `1.0.0`, so manifest version fields conflict with the release version and cannot independently identify this tree.
6. Markdown prompts are operational capabilities, not passive documentation. They direct agents to modify files, run tests/builds, install/use tools, commit, push, deploy, invoke MCP/browser/network tooling, and spawn subagents. Scripts and hooks add direct filesystem/network/process behavior. Safe packaging therefore requires capability classification and explicit activation policy, not blind copying or execution.

## Exact capability inventory

### 1. Skills: 24

The root [`skills/`](https://github.com/addyosmani/agent-skills/tree/98967c45a42b88d6b8fb3a88b7ff6273920763d6/skills) directory contains 24 kebab-case skill directories, each with a `SKILL.md` containing `name` and `description` frontmatter. Upstream describes these as 23 lifecycle skills plus the `using-agent-skills` meta-skill ([catalog and phase descriptions](https://github.com/addyosmani/agent-skills/blob/98967c45a42b88d6b8fb3a88b7ff6273920763d6/README.md#L196-L257)).

| Phase | Exact skill names |
|---|---|
| Meta | `using-agent-skills` |
| Define | `interview-me`, `idea-refine`, `spec-driven-development` |
| Plan | `planning-and-task-breakdown` |
| Build | `incremental-implementation`, `test-driven-development`, `context-engineering`, `source-driven-development`, `doubt-driven-development`, `frontend-ui-engineering`, `api-and-interface-design` |
| Verify | `browser-testing-with-devtools`, `debugging-and-error-recovery` |
| Review | `code-review-and-quality`, `code-simplification`, `security-and-hardening`, `performance-optimization` |
| Ship | `git-workflow-and-versioning`, `ci-cd-and-automation`, `deprecation-and-migration`, `documentation-and-adrs`, `observability-and-instrumentation`, `shipping-and-launch` |

The meta-skill is an actual router and shared-policy layer: it maps intents to the other 23 skills and defines assumptions, confusion handling, pushback, simplicity, scope discipline, verification, and a typical lifecycle sequence ([`using-agent-skills`](https://github.com/addyosmani/agent-skills/blob/98967c45a42b88d6b8fb3a88b7ff6273920763d6/skills/using-agent-skills/SKILL.md#L12-L42), [operating behaviors](https://github.com/addyosmani/agent-skills/blob/98967c45a42b88d6b8fb3a88b7ff6273920763d6/skills/using-agent-skills/SKILL.md#L44-L113), [sequence](https://github.com/addyosmani/agent-skills/blob/98967c45a42b88d6b8fb3a88b7ff6273920763d6/skills/using-agent-skills/SKILL.md#L130-L163)). Multiple skills are intended to compose sequentially or concurrently, rather than act as isolated prompts.

Only `idea-refine` contains skill-local supporting files: `examples.md`, `frameworks.md`, `refinement-criteria.md`, and executable `scripts/idea-refine.sh`. The script creates `docs/ideas` in the current project and returns a JSON status payload ([helper source](https://github.com/addyosmani/agent-skills/blob/98967c45a42b88d6b8fb3a88b7ff6273920763d6/skills/idea-refine/scripts/idea-refine.sh)).

### 2. Agent personas: 4

The [`agents/`](https://github.com/addyosmani/agent-skills/tree/98967c45a42b88d6b8fb3a88b7ff6273920763d6/agents) directory contains:

| Persona | Declared role/capability | Composition |
|---|---|---|
| `code-reviewer` | Staff-level five-axis review | Direct, `/review`, or `/ship` |
| `security-auditor` | Threat modeling and vulnerability/hardening audit | Direct or `/ship` |
| `test-engineer` | Test strategy, coverage analysis, and Prove-It testing | Direct, `/test`, or `/ship` |
| `web-performance-auditor` | Quick static or deep evidence-backed web performance audit | Direct or `/webperf`; deliberately excluded from generic `/ship` |

Every persona is a Markdown system-prompt artifact with `name` and `description` frontmatter, a specialized report contract, and a Composition section. The governing rule is that personas may invoke skills but may not invoke other personas; orchestration belongs to commands or the user ([persona rules](https://github.com/addyosmani/agent-skills/blob/98967c45a42b88d6b8fb3a88b7ff6273920763d6/docs/agents.md#L99-L115)).

### 3. Commands: 8 logical commands, 24 projection files

There are eight logical user-facing workflows:

| Logical invocation | Canonical Antigravity file | Primary composition/behavior |
|---|---|---|
| `/build` | `commands/build.toml` | Incremental implementation + TDD; default one slice or approved `auto` mode; writes code/tests, runs suites/build, and commits |
| `/code-simplify` | `commands/code-simplify.toml` | Code simplification followed by review, preserving behavior |
| `/plan` | `commands/planning.toml` | Planning/task breakdown; writes `tasks/plan.md` and `tasks/todo.md` |
| `/review` | `commands/review.toml` | Five-axis code review using security/performance skills as relevant |
| `/ship` | `commands/ship.toml` | Parallel fan-out to `code-reviewer`, `security-auditor`, `test-engineer`, then go/no-go synthesis and rollback plan |
| `/spec` | `commands/spec.toml` | Spec-driven discovery; writes `SPEC.md` after user clarification |
| `/test` | `commands/test.toml` | TDD/Prove-It; conditionally uses browser testing |
| `/webperf` | `commands/webperf.toml` | Dispatches the `web-performance-auditor` persona in Quick or Deep mode |

The canonical Antigravity forms are in [`commands/`](https://github.com/addyosmani/agent-skills/tree/98967c45a42b88d6b8fb3a88b7ff6273920763d6/commands). Equivalent host projections exist as eight Claude Markdown files in [`.claude/commands/`](https://github.com/addyosmani/agent-skills/tree/98967c45a42b88d6b8fb3a88b7ff6273920763d6/.claude/commands) and eight Gemini TOML files in [`.gemini/commands/`](https://github.com/addyosmani/agent-skills/tree/98967c45a42b88d6b8fb3a88b7ff6273920763d6/.gemini/commands). The Claude filename is `plan.md`; the other two directories use `planning.toml`, even though the user-facing lifecycle calls it `/plan`.

The 24 files are projections, not 24 distinct capabilities. Several projection bodies differ by host-specific wording; they should not be deduplicated by byte hash or assumed interchangeable. Upstream CI runs a parity/description validator across these directories ([workflow](https://github.com/addyosmani/agent-skills/blob/98967c45a42b88d6b8fb3a88b7ff6273920763d6/.github/workflows/test-plugin-install.yml#L29-L41)).

### 4. Hooks and executable helpers

The tree has seven executable-mode (`100755`) files: four runtime hook scripts, two hook tests, and one skill helper.

#### Automatically registered upstream hook

[`hooks/hooks.json`](https://github.com/addyosmani/agent-skills/blob/98967c45a42b88d6b8fb3a88b7ff6273920763d6/hooks/hooks.json) registers exactly one `SessionStart` command for Claude. It locates and runs `hooks/session-start.sh`. That script reads the entire `using-agent-skills/SKILL.md` and emits it in a JSON message using `jq`; if `jq` is missing, it emits an informational fallback ([source](https://github.com/addyosmani/agent-skills/blob/98967c45a42b88d6b8fb3a88b7ff6273920763d6/hooks/session-start.sh)). This is automatic prompt/context injection at session creation.

Codex is deliberately insulated: [`.codex-plugin/plugin.json`](https://github.com/addyosmani/agent-skills/blob/98967c45a42b88d6b8fb3a88b7ff6273920763d6/.codex-plugin/plugin.json) declares an empty hook map so Codex does not auto-load the Claude hook ([upstream explanation](https://github.com/addyosmani/agent-skills/blob/98967c45a42b88d6b8fb3a88b7ff6273920763d6/docs/codex-setup.md#L25-L31)).

#### Present but opt-in, not registered by `hooks/hooks.json`

- `sdd-cache-pre.sh`: a Claude `PreToolUse` WebFetch cache hook. It hashes the URL, reads project-local cache state, makes a conditional network `HEAD` request with `curl`, and on HTTP 304 blocks WebFetch with exit 2 while emitting cached prompt-shaped content on stderr ([source](https://github.com/addyosmani/agent-skills/blob/98967c45a42b88d6b8fb3a88b7ff6273920763d6/hooks/sdd-cache-pre.sh)).
- `sdd-cache-post.sh`: a `PostToolUse` WebFetch cache hook. It reads tool input/response JSON, makes a network `HEAD` request, and creates/removes/moves JSON cache files under `.claude/sdd-cache` ([source](https://github.com/addyosmani/agent-skills/blob/98967c45a42b88d6b8fb3a88b7ff6273920763d6/hooks/sdd-cache-post.sh)).
- `simplify-ignore.sh`: a `PreToolUse Read`, `PostToolUse Edit|Write`, and `Stop` hook that backs up, rewrites, restores, and may recover project files via `.claude/.simplify-ignore-cache` ([source and behavior contract](https://github.com/addyosmani/agent-skills/blob/98967c45a42b88d6b8fb3a88b7ff6273920763d6/hooks/simplify-ignore.sh#L1-L20)). Its documentation requires the user to add hook configuration; it is not in the shipped registration file ([setup](https://github.com/addyosmani/agent-skills/blob/98967c45a42b88d6b8fb3a88b7ff6273920763d6/hooks/SIMPLIFY-IGNORE.md#L20-L47)).

The SDD cache pair depends on `jq`, `curl`, and `shasum`/`sha256sum`; simplify-ignore depends on `jq`, a SHA-1 utility, and also uses common filesystem/text utilities. These are direct network/filesystem mutation capabilities and should never become active merely because their files were synchronized.

### 5. Shared reference assets: 7

The [`references/`](https://github.com/addyosmani/agent-skills/tree/98967c45a42b88d6b8fb3a88b7ff6273920763d6/references) directory contains:

- `accessibility-checklist.md`
- `definition-of-done.md`
- `observability-checklist.md`
- `orchestration-patterns.md`
- `performance-checklist.md`
- `security-checklist.md`
- `testing-patterns.md`

These are progressive-disclosure dependencies consumed by skills, not standalone invocations. The README documents their intended topics ([reference catalog](https://github.com/addyosmani/agent-skills/blob/98967c45a42b88d6b8fb3a88b7ff6273920763d6/README.md#L276-L287)), and upstream contribution rules explicitly require shared reference material to live here rather than be duplicated inside skills ([CONTRIBUTING](https://github.com/addyosmani/agent-skills/blob/98967c45a42b88d6b8fb3a88b7ff6273920763d6/CONTRIBUTING.md#L55-L61)). A safe projection of a skill must preserve or rewrite its referenced relative paths.

### 6. Host manifests and integration descriptors

| Path | Meaning at the pinned commit |
|---|---|
| [`plugin.json`](https://github.com/addyosmani/agent-skills/blob/98967c45a42b88d6b8fb3a88b7ff6273920763d6/plugin.json) | Minimal Antigravity-style identity, version `1.0.0` |
| [`.claude-plugin/plugin.json`](https://github.com/addyosmani/agent-skills/blob/98967c45a42b88d6b8fb3a88b7ff6273920763d6/.claude-plugin/plugin.json) | Claude plugin metadata; points to both command directories, all skills, and all four personas; declares MIT |
| [`.claude-plugin/marketplace.json`](https://github.com/addyosmani/agent-skills/blob/98967c45a42b88d6b8fb3a88b7ff6273920763d6/.claude-plugin/marketplace.json) | Claude marketplace source `addyosmani/agent-skills` |
| [`.codex-plugin/plugin.json`](https://github.com/addyosmani/agent-skills/blob/98967c45a42b88d6b8fb3a88b7ff6273920763d6/.codex-plugin/plugin.json) | Codex identity/version `1.0.0`; exposes `./skills/`; explicitly empty hooks |
| [`.agents/plugins/marketplace.json`](https://github.com/addyosmani/agent-skills/blob/98967c45a42b88d6b8fb3a88b7ff6273920763d6/.agents/plugins/marketplace.json) | Local marketplace entry rooted at `./`, installation `AVAILABLE`, authentication `ON_INSTALL` |
| [`.opencode/skills`](https://github.com/addyosmani/agent-skills/blob/98967c45a42b88d6b8fb3a88b7ff6273920763d6/.opencode/skills) | Symlink `../skills/` for OpenCode discovery |

The root [`AGENTS.md`](https://github.com/addyosmani/agent-skills/blob/98967c45a42b88d6b8fb3a88b7ff6273920763d6/AGENTS.md#L1-L20) mixes repository-maintainer guidance with OpenCode routing. Critically, upstream says it configures agents working **on this repository** and is not meant to be copied into consumer projects or global configuration; the reusable assets are the skills. Packy must not blindly redistribute/activate this file as a consumer `AGENTS.md`.

### 7. Validation and eval assets (source-maintenance capabilities)

The release tree includes 24 JSON eval cases, three Node.js scripts under `scripts/`, two shell hook tests, and one GitHub Actions workflow; it contains no `evals/fixtures/` directory at this release. Each skill is expected to have an eval with positive/negative triggers and a behavioral case ([contribution contract](https://github.com/addyosmani/agent-skills/blob/98967c45a42b88d6b8fb3a88b7ff6273920763d6/CONTRIBUTING.md#L36-L53)). The workflow runs skill validation, evals, command-parity validation, Claude manifest validation, and a real plugin install ([CI workflow](https://github.com/addyosmani/agent-skills/blob/98967c45a42b88d6b8fb3a88b7ff6273920763d6/.github/workflows/test-plugin-install.yml)).

These files provide provenance and upstream quality evidence, but they are not user-facing pack capabilities. Eval JSON includes prompts and expected routing/behavior assertions; it must remain inert source/test data unless an explicit validation process runs it in a sandbox.

## Relationship model

The upstream model can be summarized without flattening it:

```text
user intent
  -> command (optional host-specific entry point / orchestration)
      -> persona(s) (optional specialist role and report contract)
          -> skill(s) (mandatory workflow/how)
              -> shared references and optional helper/tool calls

session/plugin activation
  -> registered host hook (Claude SessionStart only)
      -> injects using-agent-skills meta-router
          -> selects lifecycle skill(s)
```

Important constraints from upstream:

- Skills can compose and cross-reference other skills/references.
- Personas can invoke skills, but personas do not invoke personas.
- `/ship` is the only endorsed multi-persona orchestration: three independent personas in parallel, then a main-agent merge ([worked example](https://github.com/addyosmani/agent-skills/blob/98967c45a42b88d6b8fb3a88b7ff6273920763d6/docs/agents.md#L58-L77)).
- `/webperf` is single-persona and conditional on a browser-facing target; it is intentionally not part of generic `/ship`.
- The Claude, Gemini, Antigravity, Codex, and OpenCode surfaces are projections of the same conceptual system, but not capability-equivalent.

## Provenance and licensing

### Ownership and license

- The repository is not a GitHub fork and declares SPDX `MIT` in GitHub metadata.
- The only tracked license/notice file is the root MIT [`LICENSE`](https://github.com/addyosmani/agent-skills/blob/98967c45a42b88d6b8fb3a88b7ff6273920763d6/LICENSE), copyright 2025 Addy Osmani.
- MIT grants use, copy, modification, merge, publication, distribution, sublicensing, and sale rights, conditioned on including the copyright and permission notice in all copies or substantial portions.
- Contributions are explicitly accepted under MIT ([CONTRIBUTING](https://github.com/addyosmani/agent-skills/blob/98967c45a42b88d6b8fb3a88b7ff6273920763d6/CONTRIBUTING.md#L114-L116)).
- The README names Addy Osmani as creator and Federico Bartoli and Joan León as collaborators ([team](https://github.com/addyosmani/agent-skills/blob/98967c45a42b88d6b8fb3a88b7ff6273920763d6/README.md#L388-L402)); the pinned commit history also includes many external contributors.

### Embedded attribution and external dependencies

`code-simplification` states that it is inspired by and adapted from Anthropic's Claude Code Simplifier plugin ([source note](https://github.com/addyosmani/agent-skills/blob/98967c45a42b88d6b8fb3a88b7ff6273920763d6/skills/code-simplification/SKILL.md#L6-L8)). This is attribution/provenance worth preserving. The inspected tree does not contain a second license file for that influence.

Numerous skills and references cite external standards, documentation, tools, and example commands. Those links are dependencies for interpretation/runtime use, not vendored copies discovered in the tree. A Pack Source should preserve source text and attribution rather than rewriting away these links.

### Redistribution conclusion

The pinned source is legally redistributable as a Pack Source under MIT **if** Packy carries the upstream copyright and permission notice with copies/substantial portions. This research does not establish trademark rights, endorse changing author attribution, or audit the license of content fetched later through external links/tools. Preserve upstream identity and attribution distinctly from Packy's pack identity (`addy`).

## Version and synchronization signals

Observed first-party signals:

| Signal | Value |
|---|---|
| Exact inspected identity | `98967c45a42b88d6b8fb3a88b7ff6273920763d6` |
| Default branch | `main` |
| Pinned GitHub release/tag | `0.6.4`, released 2026-07-12, commit `98967c45a42b88d6b8fb3a88b7ff6273920763d6` |
| Other tags | `0.5.0`, `0.6.0`, `0.6.1`, `0.6.2`, `0.6.3` |
| Root `plugin.json` version | `1.0.0` |
| Codex manifest version | `1.0.0` |
| Claude manifest | no version field |
| Live branch observation | `main` had advanced beyond `0.6.4` at inspection; it was not used as the source identity |

Primary release metadata is available from [GitHub Releases](https://github.com/addyosmani/agent-skills/releases); tags are visible at [GitHub Tags](https://github.com/addyosmani/agent-skills/tags). Because manifest `1.0.0`, release `0.6.4`, and a moving post-release `main` coexist, synchronization must record both the release tag and exact commit SHA. Here the synchronized commit exactly matches `0.6.4`; do not relabel it “1.0.0” merely from a host manifest.

No source dependency lockfile or language package manifest exists at the repository root. The executable utilities rely on platform commands or CI-installed tools, so a source sync cannot derive a complete runtime dependency closure from a package lock.

## Trust-sensitive behavior inventory

### Direct executable behavior

| Artifact | Behavior/risk |
|---|---|
| `hooks/session-start.sh` | Reads a skill file and injects it into every Claude session; executes `jq`; alters model context automatically |
| `hooks/sdd-cache-pre.sh` | Reads cache files, makes network requests, can block WebFetch and substitute cached prompt-shaped content |
| `hooks/sdd-cache-post.sh` | Reads tool data, makes network requests, writes/removes/moves project-local cache files |
| `hooks/simplify-ignore.sh` | Rewrites real project files in place around Read/Edit/Write/Stop events, keeps backups, and may create `.recovered` files |
| `skills/idea-refine/scripts/idea-refine.sh` | Creates `docs/ideas` in the invoking project |
| `scripts/*.js`, hook tests, CI | Source-maintenance validation/eval/install activity; should not run during pack synchronization or activation |

### Prompt-directed behavior

Skills, agents, and commands can cause a capable host agent to:

- read, write, edit, delete, or reorganize consumer repository files;
- run tests, builds, linters, benchmarks, browser audits, and package-manager commands;
- create specs, plans, ADRs, checklists, telemetry, migrations, and rollout/rollback artifacts;
- create Git commits and, in shipping/version workflows, discuss or perform push/tag/release/deploy actions;
- access official documentation, web resources, APIs, Chrome DevTools MCP, Lighthouse/PageSpeed/CrUX, and other network tooling;
- inspect secrets/security boundaries and recommend or invoke dependency/security tooling;
- spawn independent specialist subagents (especially `/ship`) and merge their reports;
- enter autonomous multi-task execution after a single approval (`/build auto`).

These behaviors arise from instructions even when the artifact itself is Markdown. Therefore “non-executable file” is not equivalent to “non-operative capability.” Packy should classify prompt authority separately from OS executable bits.

## Constraints for a safe Packy Pack Source

1. **Pin and attest the exact commit.** Store release `0.6.4` plus commit `98967c45a42b88d6b8fb3a88b7ff6273920763d6` (and ideally content digests/tree identity) as the synchronization source. Branch names and manifest versions are insufficient.
2. **Keep acquisition inert.** Fetch/copy bytes only. Do not invoke upstream installers, CI, hooks, validators, eval runners, command prompts, or helpers during inspection, sync, packaging, or validation.
3. **Retain MIT notice and attribution.** Bundle the upstream license with any substantial redistributed subset and preserve embedded provenance notes and source URL/SHA metadata.
4. **Model capability types explicitly.** Do not flatten skills, personas, commands, hooks, references, host manifests, and maintenance tests into one “skills” directory. Preserve relationships and distinguish logical commands from their three projections.
5. **Make host projections declared.** Claude can natively project the broadest set. At this commit, upstream Codex intentionally exposes only skills and no hooks/personas/commands; OpenCode uses prompt + skill discovery and a symlink. Any Packy projection beyond upstream-native support is a Packy mapping decision, not an upstream fact.
6. **Never auto-enable hooks.** The one registered Claude `SessionStart` hook changes model context automatically; the optional cache and simplify hooks perform network/filesystem operations. Activation must be explicit, host-supported, reviewable, and sandbox-tested.
7. **Do not copy root maintainer instructions as consumer policy.** Upstream expressly scopes root `AGENTS.md` to maintaining the source repository. Extract only a deliberate OpenCode projection if Packy's host mapping later chooses to do so.
8. **Preserve relative dependency closure.** A selected skill may depend on `references/`, another skill, or its helper files. Partial projection must either include those dependencies intact or rewrite/test references explicitly; silent dangling references are unsafe.
9. **Handle `.opencode/skills` intentionally.** Archive/copy mechanisms may preserve, dereference, or drop the symlink. Validate the result and prohibit traversal outside the packaged root.
10. **Separate runtime capability from validation corpus.** Evals, Node scripts, hook tests, and CI are valuable synchronization evidence but should not be activated as user capabilities.
11. **Require sandboxed behavioral validation.** Static inventory cannot prove host interpretation, hook lifecycle semantics, command parity, filesystem boundaries, or network behavior. Test each supported projection with sandboxed `HOME`, XDG/config, project directory, network policy, and explicit approvals.
12. **Treat upstream changes as reviewable supply-chain events.** Future syncs can add new executable bits, hooks, symlinks, manifests, external tool requirements, or broaden prompt authority. Diff artifact types, modes, paths, references, and trust-sensitive instructions before publishing.

## Limitations and open questions

- This is a static source and first-party metadata inventory at one commit. It does not prove how Claude, Codex, OpenCode, Gemini, Antigravity, Cursor, or other hosts actually interpret the artifacts at runtime.
- I did not run upstream validation, so the report records upstream's claimed parity/quality gates rather than independently proving them.
- I did not follow or audit every external link embedded in skills/references. Their current content, availability, licenses, and safety remain outside this inventory.
- I did not perform a line-by-line intellectual-property provenance audit across 128 tracked files or the full Git history. The redistribution conclusion relies on the repository's MIT license and contributor licensing statement.
- GitHub repository and release metadata are live external state; the exact values above are time-bound to the inspection date. The commit-pinned file citations remain immutable.
- Whether Packy should redistribute optional hooks, host-specific command projections, personas, evals, docs, or only the dependency-closed runtime subset is a later product/host-mapping decision. This inventory provides the boundary facts but does not choose that policy.

## Reproduction commands (inspection only)

```bash
git ls-remote https://github.com/addyosmani/agent-skills.git HEAD
git clone --no-checkout https://github.com/addyosmani/agent-skills.git "$TMPDIR/agent-skills"
git -C "$TMPDIR/agent-skills" checkout --detach 98967c45a42b88d6b8fb3a88b7ff6273920763d6
git -C "$TMPDIR/agent-skills" rev-parse HEAD
git -C "$TMPDIR/agent-skills" ls-tree -r HEAD
git -C "$TMPDIR/agent-skills" show HEAD:<path>
gh api repos/addyosmani/agent-skills
gh api repos/addyosmani/agent-skills/releases
gh api repos/addyosmani/agent-skills/tags --paginate
```

The commands above fetch and inspect source/metadata only. Do not append execution of any retrieved script or installer.
