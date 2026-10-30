---
name: principle-separate-before-serializing-shared-state
description: "Apply when concurrent actors might write to the same file, branch, key, or state object. Eliminate the sharing first; serialize structurally only when one shared writer is a real invariant."
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

# Separate Before Serializing Shared State

When concurrent actors might share mutable state, first ask whether they need the same mutable object. If not, eliminate the sharing. When sharing is real, enforce serialization structurally: lockfiles, sequential phases, exclusive ownership. Instructions and conventions are not concurrency control.

**Why:** Concurrent writes to shared state create race conditions that are intermittent, hard to reproduce, and expensive to debug.

**Pattern:**
1. **Identify shared mutable state** (files both read and write, branches both push to, APIs both define and consume).
2. **Default: eliminate the shared write target.** Ask: do these actors need one canonical object, or are they publishing independent facts? Give each actor its own owned file, key, branch, or state directory, and merge only at the read/reporting boundary. Two workers writing their own `lastX` field into one `state.json` is still shared mutation. `indexer-state.json` + `metrics-state.json` is not.
3. **Only when one shared write target is a real invariant, serialize access structurally** (lockfiles, sequential phases, single-writer actor, or atomic compare-and-swap). Treat "we need a lock" as a design smell to check, not as the default answer.
