---
name: arena
description: "Spawn N parallel candidates at the same task, pick a base, graft the strongest parts of the losers into it. Use for /arena, 'arena this', 'throw it in the arena', or when one attempt at a non-trivial artifact would lock in the wrong shape."
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

## Claude Code execution

Apply this workflow when explicitly requested, or read it as a dependency of an explicitly requested pstack workflow. Read required pstack skills through the prepared qualified references in this entry. Project skills belong under `.claude/skills`; personal skills under `~/.claude/skills`.

Delegate through the Agent tool with an available agent type, such as general-purpose; use only parameters exposed by this session. Delegate only when current instructions permit it. Read `~/.pstack/claude-models.md` for confirmed role preferences; otherwise inherit the host model. `inherit-parent` means omit an override, not a model ID. Same-model reviewers are not a multi-model panel. When delegation is unavailable, label sequential work as such; a required independent-review gate remains incomplete.

Use accessible current-workspace history and authorized tools. Missing history, live control, scheduling, or external integrations is a reported limitation. Monitoring lasts only while the session actually runs unless a supported scheduler is verified. Preserve the user's scope and authority for messages, merges, deployments, and destructive actions. Required evidence gates remain blocked until real evidence exists.

Claude manual-only dependencies must be read as files; do not invoke them through the Skill tool or preload them as model-invoked skills.

# Arena

Open a todolist with one entry per phase (Frame, Fan out, Cross-judge, Pick, Graft, Verify) before launching anything.

Match reasoning to task difficulty with supported controls and confirmed role preferences. Claude Code fast mode is a session setting, not a per-subagent control. For `auto` or `inherit-parent`, omit model and effort overrides.

## Phase A: Frame

Every candidate gets the same prompt, so the prompt is the contract.

1. State the artifact each candidate is producing.
2. Derive the rubric. State what success looks like for *this* task, then turn it into 3-6 concrete gradeable criteria. The rubric is the picker's tool in Phase D. Candidates only see the task.
3. Pick the runners. Use the `arena runners` line in `~/.pstack/claude-models.md`. If the rule or that line is missing, use two independent agents inheriting the host model. An `auto` or `inherit-parent` runner entry means the parent model, so omit `model` for it. If a configured model is unavailable, use only another confirmed supported choice or inherit the host model, and report the loss of model diversity. Never retry a rejected model. Keep the configured number of runner seats, defaulting to two when no preference exists. Spawn more when the arena covers multiple design directions. Same model N times when the work is generation-bound rather than judgment-sensitive.
4. Assign output paths. Each candidate writes to its own location (a git worktree where possible, otherwise `/tmp/arena-<slug>/candidate-<n>/`), per the [pstack:principle-separate-before-serializing-shared-state]({{packy.access:pstack:principle-separate-before-serializing-shared-state}}).

## Phase B: Fan out

Spawn all N subagents in one message with `background execution only when the live host tool supports it`, each with the task, the path to the shared grounding, its own output path, and instructions to produce the artifact and a short rationale naming the alternatives it considered and what it rejected.

If a candidate produces no output, proceed with N-1 and note the dropout.

## Phase C: Cross-judge

After all Phase B candidates complete, read the `arena cross-judge pool` line in `~/.pstack/claude-models.md` in preference order. Run one judge only on a confirmed available model from a different family than the parent's. An `auto` or `inherit-parent` entry is the parent model and cannot judge. Never retry a model the host rejected. If no supported different-family model is available, skip the judge, pick the base alone in Phase D, and disclose the missing cross-model check. If the parent's family is unknown, disclose that the family check could not run. The judge sees the rubric and candidates by path label, scores each criterion, and recommends a base with rationale. It runs alongside the parent's reading in Phase D after every candidate has finished writing.

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
