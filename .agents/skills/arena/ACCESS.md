---
name: arena
description: "Spawn N parallel candidates at the same task, pick a base, graft the strongest parts of the losers into it. Use for pstack:arena, 'arena this', 'throw it in the arena', or when one attempt at a non-trivial artifact would lock in the wrong shape."
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

# Arena

Open a todolist with one entry per phase (Frame, Fan out, Cross-judge, Pick, Graft, Verify) before launching anything.

Match reasoning to task difficulty with supported controls and confirmed role preferences. Use standard speed unless urgency calls for fast mode; verification preserves its configured speed. For `auto` or `inherit-parent`, omit model, effort, and speed overrides.

## Phase A: Frame

Every candidate gets the same prompt, so the prompt is the contract.

1. State the artifact each candidate is producing.
2. Derive the rubric. State what success looks like for *this* task, then turn it into 3-6 concrete gradeable criteria. The rubric is the picker's tool in Phase D. Candidates only see the task.
3. Pick the runners. Use the `arena runners` line in `~/.pstack/codex-models.md`. Without that line, use two independent agents inheriting the host model. For `auto` or `inherit-parent`, omit model, effort, and speed overrides. If a configured model is unavailable, use a confirmed supported equivalent from the same family or drop the seat and disclose it. If fewer than two seats remain, add independent runners on a remaining supported model, or inherit the host model when none remain. Keep one seat when the configured list has only one entry. Report any loss of model diversity. Spawn more runners when the task covers multiple design directions.
4. Assign output paths. Each candidate writes to its own location (a git worktree where possible, otherwise `/tmp/arena-<slug>/candidate-<n>/`), per the [pstack:principle-separate-before-serializing-shared-state]({{packy.access:pstack:principle-separate-before-serializing-shared-state}}).

## Phase B: Fan out

Spawn all N subagents in one message with `background execution only when the live host tool supports it`, each with the task, the path to the shared grounding, its own output path, and instructions to produce the artifact and a short rationale naming the alternatives it considered and what it rejected.

If a candidate produces no output, proceed with N-1 and note the dropout.

## Phase C: Cross-judge

After all Phase B candidates complete, try the confirmed `arena cross-judge pool` entries in `~/.pstack/codex-models.md` in preference order. The judge must use a different model family from the parent. `auto` and `inherit-parent` resolve to the parent and never qualify as a diverse judge. Use only model choices supported by the actual host. If no supported different-family model runs, skip the judge, choose the base in Phase D, and disclose the missing independent judgment. If the parent's model family cannot be established, disclose that the family check did not run. The judge reads the rubric and candidate paths, scores each criterion, and recommends a base. Run it alongside your Phase D reading, after all candidates stop writing.

## Phase D: Pick a base

Read every candidate end to end before picking. Score each against the rubric criterion by criterion, not on holistic feel, and compare with the cross-judge when Phase C ran one. Agreement on the base confirms the pick. Disagreement means one of you is biased or the rubric was ambiguous. Read both rationales before deciding.

Pick the base a future maintainer can extend most easily without breaking invariants. When two feel tied, prefer the cleaner boundary or smaller API, per [pstack:principle-laziness-protocol]({{packy.access:pstack:principle-laziness-protocol}}).

## Phase E: Graft

Walk each losing candidate once more for what is worth porting into the base, usually one or two things per candidate, not most of it. Fold each graft in by hand, per the [pstack:principle-redesign-from-first-principles]({{packy.access:pstack:principle-redesign-from-first-principles}}), so the result stays coherent under one mental model. Don't paste mechanically.

If the candidates converge on one shape, that is strong agreement. Note it and ship the consensus shape. No graft is needed. If they wildly diverge, Phase A was under-specified. Reframe and re-run rather than averaging the divergence.

## Phase F: Verify

Verify the synthesized artifact like any other output, per the [pstack:principle-prove-it-works]({{packy.access:pstack:principle-prove-it-works}}). If verification finds a problem the arena missed, either Phase A was wrong (re-frame and re-run) or a candidate caught it and you missed the graft (go back to Phase E). Don't paper over it.

## Outputs

One synthesized artifact, with a short synthesis note alongside naming the base and why, the cross-judge's verdict or that Phase C skipped the judge, each graft with its source candidate, what was rejected and why, any convergence or dropouts, and the verification result.
