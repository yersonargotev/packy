---
name: reflect
description: Spawn three parallel review subagents over the active transcript, surface learnings, and route each to a concrete edit on an existing skill. Use when the user says reflect.
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

## Codex execution

This is a Codex-specific workflow carried by the Agents convention. A shared Agents binding does not provide Codex tools, model choice, or delegation; use only capabilities authorized and available in the actual client.

Apply this workflow when explicitly requested, or read it as a dependency of an explicitly requested pstack workflow. Read required pstack skills through the prepared qualified references in this entry. Project skills belong under `.agents/skills`; personal skills under `~/.agents/skills`.

Delegate through the available Codex subagent API; use only its advertised agent types and parameters. Delegate only when current instructions permit it. Read `~/.pstack/codex-models.md` for confirmed role preferences; otherwise inherit the host model. `inherit-parent` means omit an override, not a model ID. Same-model reviewers are not a multi-model panel. When delegation is unavailable, label sequential work as such; a required independent-review gate remains incomplete.

Use accessible current-workspace history and authorized tools. Missing history, live control, scheduling, or external integrations is a reported limitation. Monitoring lasts only while the session actually runs unless a supported scheduler is verified. Preserve the user's scope and authority for messages, merges, deployments, and destructive actions. Required evidence gates remain blocked until real evidence exists.

# Reflect

Mine the current conversation for durable learnings, then route them into skill edits.

Invoke when the user says "reflect" or "pstack:reflect". Skip when the conversation is trivial, off-topic, or already covered by an existing skill the parent followed correctly. One-offs are not learnings.

## 1. Locate the active transcript

Use the host-exposed record for the current workspace and session. Verify its session identity before reading it. If the host exposes no record, write a concise digest from this conversation and label it as a digest. Pass that digest to the reviewers; do not invent transcript paths or assume another host's JSONL layout.

## 2. Spawn three reviewers in parallel

One message, three native subagent calls, the available general-purpose agent type, with `model` set as below, read-only review intent with the required authorized lookup tools. Reviewers need MCP access for context lookups (tickets, chat threads, observability traces referenced in the transcript). Inspect the delegated tool set before relying on any MCP lookup.

Each reviewer and the synthesizer name a role line in the `~/.pstack/codex-models.md` file and a default. Set `model` to that line's value, or to the default if the rule or the line is missing. Leave `model` unset when the value is `auto` or `inherit-parent`. If a configured model is unavailable, inherit the host model and disclose the fallback.

| Lens | Role line | Default `model` | Prompt template |
|---|---|---|---|
| Judgment | `reflect judgment, divergent, synthesizer` | `inherit-parent` | `references/ACCESS-judgment-reviewer.md` |
| Tooling | `reflect tooling` | `inherit-parent` | `references/ACCESS-tooling-reviewer.md` |
| Divergent | `reflect judgment, divergent, synthesizer` | `inherit-parent` | `references/ACCESS-divergent-reviewer.md` |

Pass each template verbatim, substituting the transcript path or digest where marked. Reviewers return findings in the native subagent response body.

## 3. Synthesize

One native subagent call, the available general-purpose agent type, with `model` from the `reflect judgment, divergent, synthesizer` line (default `inherit-parent`), read-only review intent with the required authorized lookup tools. The synthesizer's quality check includes spot-verifying citations, which can require MCP access. Inspect the delegated tool set before relying on any MCP lookup. Use `references/ACCESS-synthesizer.md` verbatim, with each reviewer's full output inlined where marked. The synthesizer returns a structured Accepted / Rejected / Backlog list.

## 4. Structural enforcement check

Move any Accepted item that a lint rule, script, metadata flag, or runtime check would enforce more reliably to Backlog. See the [pstack:principle-encode-lessons-in-structure]({{packy.access:pstack:principle-encode-lessons-in-structure}}).

## 5. Apply

Present the synthesizer's full Accepted / Rejected / Backlog output and wait for explicit approval before applying any Accepted edit. The user picks the subset and may redirect routings. Skill changes affect every future agent in the org. Do not auto-apply.

Keep Backlog items in the local report. File them externally only when the user has authorized publication and the correct tracker/project is available; otherwise label them unfiled.

Preserve Catalog resources, prepared trees, installed managed copies, and plugin caches. Before approval, show its destination, `.agents/skills/<skill>/` or `~/.agents/skills/<skill>/`. After approval, copy the skill folder there and edit the copy. Never open a PR or push.

Follow each approved row's Routing exactly:

- Trivial existing-skill edit (a one-line bullet, a tightened sentence, a stale fact corrected): parent does directly.
- Substantive existing-skill edit (a new section, a new pattern table, more than ~10 lines): use an available host skill creator and run its draft / test / iterate loop. If none exists, edit and verify the skill directly.
- `tune description: <skill path>` (the skill exists but didn't trigger when it should have): use an available skill-authoring guide and test realistic triggering requests; otherwise refine and verify the description directly.
- `new skill via create-skill: <kebab-name>`: use an available skill-authoring guide; otherwise write a minimal skill and verify its frontmatter, references, and realistic behavior.

If your environment ships a SKILL.md validator, run it on every touched skill before declaring done.

## 6. Summarize for the user

Short list, no preamble:

- Edits applied: `<skill path>`. What changed, one line each.
- New skills created: `<skill path>`. One line each (rare).
- Backlog (filed or explicitly unfiled): `<issue title>` (`<tags>`). One line each.
- Dropped: one line per rejected finding + reason from the synthesizer.
