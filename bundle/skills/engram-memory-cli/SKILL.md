---
name: engram-memory-cli
description: Recall and finalize durable project Memory with Engram's CLI. Use for history-dependent work, Terminal Memory commits, explicit curation, or material loss-risk handoffs. Scope this skill to project memory.
metadata:
  version: "3.3.0"
---

# Engram Memory CLI

Engram is local-first, best-effort project memory. Memory can inform or preserve
work; the primary deliverable remains independent from memory availability.

## Best-effort protocol

1. Confirm that `engram` is available. For tasks about Engram itself, keep a CLI
   failure as task evidence and diagnose it within scope. For other tasks,
   continue without memory when the CLI is unavailable or fails.
2. Run `engram current-project --json` before the first project-scoped operation.
3. Treat detection and authority separately. Automatic candidate Recall and
   writes require `project_strength` to be `strong` or `explicit`. Never turn a weak
   `git_root`, `git_child`, or `dir_basename` result into authority by copying it
   into `--project`. Ask the user for the exact project on an explicit memory
   task; otherwise skip the write and continue. When `project` is empty, ask the
   user to select from `available_projects` for an explicit memory task.
4. Pass an exact project to every command that accepts it: use
   `--project <project>` for project-scoped flags and positional `[project]` for
   `engram context`.
5. Use `--json` for agent operations. Parse successful stdout as JSON and
   non-zero stderr as `{"code","message","details?"}`.

Complete this protocol when one exact project is known or memory use has been
skipped without delaying the primary deliverable.

## Recall

Recall only when prior project knowledge could materially change the work.

Use Recall for relevant prior decisions, tracked work, release state,
configuration, preferences, known failures, or an explicit request to remember.
Routine self-contained work needs no search. Personal or cross-project scope
requires explicit task relevance or user direction.

1. Search one lookup intent with one to three distinctive anchors:

   ```bash
   engram search "<narrow query>" --project "<project>" \
     --scope project --match-mode all --limit 5 --json
   ```

2. The initial response is limited to five candidate summaries and 4 KiB. Core
   excludes inactive, deleted, and superseded Memories; relevance/currentness
   rank before pins and recency. Account for every result and explicit conflict.
   Use the response's `recall_id` and one selected candidate's opaque
   `result_id` only when complete content can change the task:

   ```bash
   engram get --recall-id '<recall-id>' --result-id '<result-id>' \
     --project '<project>' --scope project --json
   ```

   One response returns at most 16 KiB of valid UTF-8 content and reports
   `original_bytes`, `delivered_utf8_bytes`, `limit_bytes`, and `truncated`.
   When `truncated` is true, request more only with the exact returned byte
   position; do not infer a position or widen the original scope:

   ```bash
   engram get --recall-id '<recall-id>' --result-id '<result-id>' \
     --position '<continuation_position>' --project '<project>' \
     --scope project --json
   ```
3. If a material memory is expected and the first search is empty or too broad,
   reformulate at most once. Remove generic terms, choose a more distinctive anchor, or
   switch to `--match-mode any`; keep the same lookup intent.
4. A deliberate follow-up may use `--limit 6` through `--limit 10` without
   widening scope or bypassing the 4 KiB candidate budget.
5. Request chronological context separately when recent session continuity can
   materially change the work:

   ```bash
   engram context "<project>" --scope project --json
   ```

6. Treat empty Recall as successful. If Recall is unavailable, continue the
   primary task after reporting the one warning and structured diagnostics.

Use `--all-projects` only for an explicitly cross-project request. Complete
Recall when every relevant result is accounted for, or when the initial search
and its single allowed reformulation are empty.

## Terminal Memory commit

For normal agent work, preserve reusable project knowledge through the root
turn's Terminal Memory commit. Use the opaque identity supplied by the host;
never synthesize a replacement identity.

1. Apply the canonical `engram-memory` disposition rubric after all causal work
   settles.
2. Draft prospective Memories, then run the bounded read-only preflight. Reuse
   exact duplicates and account for every returned candidate (at most three):

   ```bash
   engram checkpoint preflight --project '<project>' \
     --memory-json '{"title":"<concise title>","content":"<durable result>"}' --json
   ```
3. For `saved`, attach existing Memory IDs or create concise Memories atomically
   with repeatable `--memory-json` values:

   ```bash
   engram checkpoint record --host '<host>' --session-id '<session>' \
     --root-turn-id '<root-turn>' --disposition saved --project '<project>' \
     --memory-json '{"title":"<concise title>","content":"What: <durable result>\nWhy: <future value>\nWhere: <subsystem or path>\nLearned: <non-obvious implication>"}' --json
   ```
4. Use `skipped --reason no_durable_knowledge` only when the rubric finds no
   durable result. Use `needs_review` with one redacted `--proposal-json` when
   potentially durable knowledge cannot be admitted directly; include any
   independently settled `--memory-id` or `--memory-json` values in the same
   Mixed Memory checkpoint.
5. Treat `created` and same-disposition `already_recorded` as success. Surface
   conflicts and persistence failures rather than changing the disposition.

Complete normal preservation only when the exact root-turn identity has one
terminal result.

## Independent save

Use `engram save` only for explicit curation or a long-running, material
loss-risk handoff that must preserve knowledge before the root turn settles.
The later Terminal Memory commit still finalizes the root turn and may attach
the saved Memory by ID.

For this branch, read [references/curation.md](references/curation.md) and follow
its save, topic-key, candidate-judgment, and authorization rules.

## Curate

For editing, review, context priority, relations, diagnosis, or project merges,
read [references/curation.md](references/curation.md). Load it only for those
branches and follow every completion and authorization gate it defines.
