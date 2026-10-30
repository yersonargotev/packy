---
name: automate-me
description: "Use for \"automate me\", \"create/update/refresh my -mode skill\", \"turn/capture my preferences or working style into a skill\", or wanting agents to follow how the user works. Drafts or revises a personal -mode skill via create-skill + unslop, optionally pulling fresh evidence from recent transcripts."
---

## On-demand context

Use the initiating retrieval result's explicit `surface` and `snapshot_id` for
this procedure and every nested request. Read the prepared qualified pstack
entries linked below; native Skill-tool registration and installed siblings are
not required. For a fresh nested descriptor, use:

```sh
packy get skill pstack:<skill> --surface <initiating-surface> --snapshot <initiating-snapshot-id> --json
```

Keep that host and snapshot after catalog selection changes. Missing retained
content requires restoring that exact revision. Another Pack's same-name skill
or a modified native copy cannot replace the selected pstack resource. Optional
routes and comparisons name qualified skills but are not required dependencies.
Obtain an optional route only when selected, with the same host/snapshot and its
own reviewed compatibility; unavailable optional advice does not block this flow.

Resolve references relative to this entry and `resource_root`. Resolve project
inspection and outputs against the user's intended target, with writable scratch
separate from immutable resources. Pass host, snapshot, target, scratch/output
and prepared entry/resource locations to every delegate or handoff. Reading a
resource does not execute it, apply setup, install tools or verify prerequisites.
Preserve this procedure's invocation policy, approvals and capability limits.
Read supporting references only when their procedure branch needs them.

## Writable authoring context

Read the prepared pstack:poteto-mode entry only as a shape reference. Reading it
does not activate its workflow. Read pstack:unslop for the prose pass. An available
host skill-authoring guide is optional; retain direct authoring and validation
when none is available. Resolve the chosen existing or new personal/project skill
to an explicit writable destination before drafting. Immutable Catalog entries
are references, never the user's customization destination.

## Prepared pstack dependencies

- [pstack:poteto-mode]({{packy.access:pstack:poteto-mode}})
- [pstack:unslop]({{packy.access:pstack:unslop}})

## Codex execution

This is a Codex-specific workflow carried by the Agents convention. A shared Agents binding does not provide Codex tools, model choice, or delegation; use only capabilities authorized and available in the actual client.

Apply this workflow when explicitly requested, or read it as a dependency of an explicitly requested pstack workflow. Read required pstack entries through the prepared qualified references in this entry. Project skills belong under `.agents/skills`; personal skills under `~/.agents/skills`.

Delegate through the available Codex subagent API; use only its advertised agent types and parameters. Delegate only when current instructions permit it. Read `~/.pstack/codex-models.md` for confirmed role preferences; otherwise inherit the host model. `inherit-parent` means omit an override, not a model ID. Same-model reviewers are not a multi-model panel. When delegation is unavailable, label sequential work as such; a required independent-review gate remains incomplete.

Use accessible current-workspace history and authorized tools. Missing history, live control, scheduling, or external integrations is a reported limitation. Monitoring lasts only while the session actually runs unless a supported scheduler is verified. Preserve the user's scope and authority for messages, merges, deployments, and destructive actions. Required evidence gates remain blocked until real evidence exists.

# Automate me

A guided flow for turning the user's working conventions into a skill agents will follow. The output is one `-mode` skill tailored to them (e.g. `jay-mode`, `priya-mode`).

This skill sequences an inline mining pass, a skill-authoring workflow, and
the [pstack:unslop]({{packy.access:pstack:unslop}}) skill. Use an installed skill-authoring guide when available, or author and validate the `SKILL.md` directly.

## Flow

### 0. Check for an existing skill

Look in the active host's project and personal skill directories for an
existing `*-mode/SKILL.md` matching the user's handle. If one exists, confirm intent with
the host's question tool or a concise question (unless the user already asked
to update it):

- Update the existing skill (default for repeat runs)
- Start fresh (rare, ask why before doing it)

Update mode changes the rest of the flow:
- Step 1 mines only history since the skill was last edited (`git log -1 --format=%cI <path>`).
- Step 2 asks what's changed or missing, not what to capture from zero.
- Step 4 edits the existing file in place. Preserve sections the user hasn't contradicted. Revise ones with new evidence. Add new sections only for genuinely new rules.

### 1. Mine their history

Locate the active workspace's transcript through the host when it exposes
one. Keep the search within the current project. If no transcript is accessible, use the current
conversation and ask the user for missing preferences; do not imply that a
history mining pass occurred.

Survey recent agent conversations within that scope for recurring patterns. Run multiple parallel subagents across slices of history (e.g. last 2-4 weeks, split into 3 slices so each has enough material). Match reasoning to task difficulty with supported controls and confirmed role preferences. Use standard speed unless urgency calls for fast mode; verification preserves its configured speed. For `auto` or `inherit-parent`, omit model, effort, and speed overrides. Each slice mining subagent reads transcripts from the workspace-scoped path the parent provides, looks for the signals below, and returns a short structured list of patterns it saw with evidence pointers. Default signals worth hunting:

- Response preferences (length, tone, format, "dumb it down" corrections)
- Delegation habits (subagents, models, specialized workflows, parallelism)
- Verification posture (what "done" means, unit tests vs live repro, reviewers)
- Code and prose discipline (style, principles cited, lint/format tools)
- Process conventions (worktrees, commits, PRs, review/merge tooling)
- Meta preferences (fixing skills mid-task, proposing new ones)

Cross-check across slices before elevating a signal. Patterns seen in 2+ slices are high-confidence. Lone signals are weak and usually get dropped.

### 2. Ask the user directly

Mining misses intent that hasn't come up yet. Use the host question tool (structured multi-choice when supported) rather than asking the user to type from scratch.

Shape: one or two questions with a concise set of supported options each, multiple selection only when supported by the live question tool for category questions. Start broad ("Which areas matter most?"), then follow up on selected areas with specific options. After the structured rounds, one free-form chat question catches anything the options missed.

Don't dump 20 questions.

### 3. Cluster findings

Group the combined signals into sections. Common ones (use only what applies):

- **Response style**: length, tone, format.
- **Autonomy**: how much to do without asking, MCP tool use.
- **Understand first**: which skills to reach for when scoping or investigating a change.
- **Subagents**: default, parallelism, model-to-task, specialized workflows.
- **Prose / code discipline**: principles, lint tools, style guides.
- **Review and verify**: repro posture, verification skills, live-testing tools.
- **Process**: git worktrees, commits, PRs, review/merge tooling.
- **Skills**: skill-authoring habits, fix-the-skill-first, proposing new skills.

The [pstack:poteto-mode]({{packy.access:pstack:poteto-mode}}) skill shows the shape. Read it for granularity. Don't copy its content. The user's rules are not the same as poteto-mode's.

### 4. Draft the skill

Use an available host skill creator to author the skill, or edit `SKILL.md`
directly when none is available. Placement:

- Path: preserve an existing skill location. New project skills use `.agents/skills/<handle>-mode/SKILL.md`; personal skills use `~/.agents/skills/<handle>-mode/SKILL.md`.
- Handle: the user's first name or chosen identifier.
- Frontmatter `description`: trigger on their name + `/<handle>-mode` + "work in their style", not on generic keywords like "write code" or "review PR".
- Frontmatter formatting: use valid YAML with a stable kebab-case name. Keep `description` as one YAML scalar. Quote it or use `description: >-` with indented continuation lines when punctuation or wrapping requires it.
- Preserve the requested invocation policy through `agents/openai.yaml`; manual-only uses `policy.allow_implicit_invocation: false`.

### 5. Iterate on prose

Apply the [pstack:unslop]({{packy.access:pstack:unslop}}) skill and the available skill-authoring guidelines to every line.

Show the draft to the user and take feedback. Expect multiple iterations. Cut ruthlessly. A mode skill is not a manual.

### 6. Deliver within the selected scope

For a personal skill, validate it in its selected local directory and report
its path. For a project skill in a versioned repository, follow that project's
branch, commit, review, and publication workflow when the user authorized it.
Do not create an unrelated or empty PR for an unversioned personal skill.
