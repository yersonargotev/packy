---
name: principle-laziness-protocol
description: "Apply when refactoring, evaluating diff size, or tempted to add abstractions, layers, or signal threading. Bias toward deletion and the smallest change that solves the problem."
disable-model-invocation: true
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

## Host execution

Apply this workflow when explicitly requested, or read it as a dependency of an explicitly requested pstack workflow. Read required pstack skills through the prepared qualified references in this entry. Project skills belong under `.agents/skills` on Agents and `.claude/skills` on Claude Code; personal skills under `~/.agents/skills` or `~/.claude/skills`.

On Agents, this workflow assumes Codex capabilities; a shared Agents binding does not provide Codex tools, model choice, or delegation. Use only capabilities authorized and available in the actual client. On Claude Code, read manual-only pstack dependencies as files; do not invoke them through the Skill tool or preload them as model-invoked skills.

# Laziness Protocol

- **Consolidate decisions.** Do not repeat the same choice in several places. Put it behind one source of truth and pass the result as a simple flag.
- **Minimize the diff.** Make the smallest change that solves the problem. Fewer lines beat "elegant" boilerplate.
