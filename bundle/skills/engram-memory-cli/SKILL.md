---
name: engram-memory-cli
description: Recall and preserve durable project memory with Engram's CLI. Use before work that may depend on prior project knowledge, after work produces durable project knowledge, or for explicit memory curation. Scope this skill to project memory.
---

# Engram Memory CLI

Engram is local-first, best-effort project memory. Memory can inform or preserve
work; the primary deliverable remains independent from memory availability.

## Best-effort protocol

1. Confirm that `engram` is available. For tasks about Engram itself, keep a CLI
   failure as task evidence and diagnose it within scope. For other tasks,
   continue without memory when the CLI is unavailable or fails.
2. Run `engram current-project --json` before the first project-scoped operation.
3. Use the returned project only when it is exact. When `project` is empty,
   ask the user to select from `available_projects` for an explicit memory
   task; otherwise skip memory and continue. Similar names are not an exact
   selection.
4. Pass the exact project to every command that accepts it: use
   `--project <project>` for project-scoped flags and positional `[project]` for
   `engram context`.
5. Use `--json` for agent operations. Parse successful stdout as JSON and
   non-zero stderr as `{"code","message","details?"}`.

Complete this protocol when one exact project is known or memory use has been
skipped without delaying the primary deliverable.

## Recall

Recall only when prior project knowledge could materially change the work.

1. Generate one Task briefing from the current task intent:

   ```bash
   engram context "<project>" --brief --task "<current task intent>" \
     --scope project --limit 5 --json
   ```

2. Account for every selected memory, Selection evidence, diagnostic, and
   omission before acting. Use `engram get <id> --json` when relation context
   could change the task.
3. Run a targeted search when the briefing command is unavailable, when a
   material memory is expected but absent, or when the task needs an exact
   known fact. Search one lookup intent with one to three distinctive anchors:

   ```bash
   engram search "<narrow query>" --project "<project>" --match-mode all --limit 5 --json
   ```

4. Inspect every search result's content, state, pin, and relations. If a
   material memory is expected and the first search is empty, refine once.
   Remove generic terms or switch to `--match-mode any`; keep the same intent.
5. Prefer the newest applicable memory while honoring `supersedes`,
   `superseded_by`, and `conflicts_with` relations. Surface unresolved conflicts
   instead of silently choosing one side.

Use `--all-projects` only for an explicitly cross-project request. Complete
recall when every relevant briefing/search result and diagnostic is accounted
for, or when the briefing and up to two targeted searches are empty.

## Preserve

Preserve after the primary work produces reusable project knowledge.

1. Save only non-obvious decisions, root causes, conventions, configurations,
   or discoveries. Content must be safe to persist, free of secrets and personal
   facts, and not already maintained in project documentation.
2. Save one concise subject with `What`, `Why`, `Where`, and `Learned`:

   ```bash
   engram save --title "<concise title>" --content "What: <durable result>
   Why: <future value>
   Where: <subsystem or path>
   Learned: <non-obvious implication>" --project "<project>" --json
   ```

3. An older binary without named save inputs can return `unknown_flag`. Retry
   once with `engram save "<same title>" "<same content>" ...` only when the
   JSON error names `--title` or `--content`; preserve every remaining flag.
   Other failures follow the best-effort protocol.
4. Add `--topic-key <stable-key>` only for an evolving subject that later
   observations should update or group. When a stable key is warranted but
   unclear, run:

   ```bash
   engram suggest-topic-key --title "<title>" --content "<content>" --json
   ```
5. Inspect `judgment_required` after every save. For each candidate, inspect the
   candidate memory and resolve its own `judgment_id` as described in
   [references/curation.md](references/curation.md).

Complete preservation when the durable result is saved and every returned
candidate is resolved or explicitly raised to the user. Routine or low-value
work completes without a write.

## Curate

For editing, review, context priority, relations, diagnosis, or project merges,
read [references/curation.md](references/curation.md). Load it only for those
branches and follow every completion and authorization gate it defines.
