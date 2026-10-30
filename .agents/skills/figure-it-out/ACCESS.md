---
name: figure-it-out
description: "Design an auditable playbook when no narrower one fits: a large migration, an ambitious multi-part change, or work a human reviews after stepping away. Scales rigor to the task, runs a hypothesis loop, and logs decisions. Use for pstack:figure-it-out or 'figure it out'."
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

## Run context

Read the prepared pstack:poteto-mode Principles before framing. Its figure-it-out
route is optional and does not recursively activate this procedure. Preserve the
long-run checkpoint, independently verifiable units and artifact-based verdicts.
The decision trail uses the prepared pstack:show-me-your-work entry with an explicit
writable output and its independent/diverse-review gate. Read every named principle
through its prepared entry, including foundational-thinking, sequence-verifiable-units
and encode-lessons-in-structure used by this body beyond the native dependency list.

## Prepared pstack dependencies

- [pstack:architect]({{packy.access:pstack:architect}})
- [pstack:arena]({{packy.access:pstack:arena}})
- [pstack:poteto-mode]({{packy.access:pstack:poteto-mode}})
- [pstack:principle-encode-lessons-in-structure]({{packy.access:pstack:principle-encode-lessons-in-structure}})
- [pstack:principle-foundational-thinking]({{packy.access:pstack:principle-foundational-thinking}})
- [pstack:principle-laziness-protocol]({{packy.access:pstack:principle-laziness-protocol}})
- [pstack:principle-never-block-on-the-human]({{packy.access:pstack:principle-never-block-on-the-human}})
- [pstack:principle-prove-it-works]({{packy.access:pstack:principle-prove-it-works}})
- [pstack:principle-separate-before-serializing-shared-state]({{packy.access:pstack:principle-separate-before-serializing-shared-state}})
- [pstack:principle-sequence-verifiable-units]({{packy.access:pstack:principle-sequence-verifiable-units}})
- [pstack:show-me-your-work]({{packy.access:pstack:show-me-your-work}})

## Codex execution

This is a Codex-specific workflow carried by the Agents convention. A shared Agents binding does not provide Codex tools, model choice, or delegation; use only capabilities authorized and available in the actual client.

Apply this workflow when explicitly requested, or read it as a dependency of an explicitly requested pstack workflow. Read required pstack entries through the prepared qualified references in this entry. Project skills belong under `.agents/skills`; personal skills under `~/.agents/skills`.

Delegate through the available Codex subagent API; use only its advertised agent types and parameters. Delegate only when current instructions permit it. Read `~/.pstack/codex-models.md` for confirmed role preferences; otherwise inherit the host model. `inherit-parent` means omit an override, not a model ID. Same-model reviewers are not a multi-model panel. When delegation is unavailable, label sequential work as such; a required independent-review gate remains incomplete.

Use accessible current-workspace history and authorized tools. Missing history, live control, scheduling, or external integrations is a reported limitation. Monitoring lasts only while the session actually runs unless a supported scheduler is verified. Preserve the user's scope and authority for messages, merges, deployments, and destructive actions. Required evidence gates remain blocked until real evidence exists.

# Figure it out

When the task matches no playbook, design one. The deliverable before any code is the workflow itself: a sequence of phases that scales rigor to the task, runs the scientific method, and leaves a decision trail a human can audit after stepping away.

## Start

Open a todolist whose first item is to read the Principles section of the [pstack:poteto-mode]({{packy.access:pstack:poteto-mode}}) skill. Then add the phases below as todos.

## Phase A: Frame

Ground first, then commit. Don't start the run until you can state:

- The definition of done as a falsifiable predicate (the [pstack:principle-prove-it-works]({{packy.access:pstack:principle-prove-it-works}}) principle skill).
- Scope, quantified: rough units and effort, plus the blockers grounding surfaced.
- The rigor level, biased high. One-way doors and high blast radius get more. Reversible low-stakes steps get less. Rigor is gates and artifacts, not "try harder".

Present the framing and tradeoffs before committing to a long run. Reversible work proceeds (the [pstack:principle-never-block-on-the-human]({{packy.access:pstack:principle-never-block-on-the-human}}) principle skill), but a multi-hour run earns one checkpoint.

## Phase B: Design the workflow

Decompose into atomic, independently-landable units. Sequence riskiest-unknown-first. Scaffold and verification come before features (the [pstack:principle-foundational-thinking]({{packy.access:pstack:principle-foundational-thinking}}) principle skill).

- Build the verification harness before the work, with the baseline captured from the pre-change state, so the check reads as "old value vs new value".
- For one-way-door design decisions, run the [pstack:architect]({{packy.access:pstack:architect}}) skill (it runs [pstack:arena]({{packy.access:pstack:arena}})). Skip it for mechanical work whose shape is already concrete. A second arena over a settled design is over-engineering (the [pstack:principle-laziness-protocol]({{packy.access:pstack:principle-laziness-protocol}}) principle skill).
- Decide what fans out. Parallelize only across seams, and give each worker its own worktree or branch (the [pstack:principle-separate-before-serializing-shared-state]({{packy.access:pstack:principle-separate-before-serializing-shared-state}}) principle skill). Don't over-fan. Match reasoning to task difficulty with supported controls and confirmed role preferences. Use standard speed unless urgency calls for fast mode; verification preserves its configured speed. For `auto` or `inherit-parent`, omit model, effort, and speed overrides.
- Write the designed phase list down. That list is what the human reviews.

Then execute the design. Add its steps to the todolist as concrete items between the Phase C and Phase D entries. Run each under the Phase C loop discipline, and weave the Phase D log through them, a row as each step lands, rather than saving the whole trail for the end.

## Phase C: Run the loop

Each unit is an experiment. State the hypothesis, make the smallest change, measure against the predicate on the real artifact, keep it if it advanced, revert it if it didn't. Apply the [pstack:principle-sequence-verifiable-units]({{packy.access:pstack:principle-sequence-verifiable-units}}) principle skill, verifying each unit before starting the next instead of batching checks at the end.

- Verify by inspecting the artifact, never a self-report. When something passes too easily, suspect the observation method before the system.
- Pair delegated work with a judge. If a worker games the gate, reset and harden the contract. If the gate itself is wrong, fix the gate in its own change rather than routing around it.
- A verdict is VERIFIED, NOT VERIFIED, or INCONCLUSIVE. Inconclusive is not a pass. Don't hide a negative.

## Phase D: Keep the audit trail

Log the run via the [pstack:show-me-your-work]({{packy.access:pstack:show-me-your-work}}) skill. figure-it-out's work is usually ambitious enough to commit the trail so the reviewer can read it in the PR. The trail plus the diff is what lets the human come back and trust the work.

## Phase E: Verify and hand back

Check the whole against the Phase A predicate on the real product, not just the harness. Encode any recurring correction as a gate, a lint rule, a check, or a script (the [pstack:principle-encode-lessons-in-structure]({{packy.access:pstack:principle-encode-lessons-in-structure}}) principle skill).

**Reply:** the playbook you designed, the rigor level and why, the decision-trail path, what's verified against the predicate, and what's still open.
