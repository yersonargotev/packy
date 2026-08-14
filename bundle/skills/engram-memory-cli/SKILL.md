---
name: engram-memory-cli
description: Use Engram's CLI for durable project memory. Activate before project work when a prior decision, root cause, convention, configuration, or discovery could change the approach; after work that produced durable project knowledge; or when inspecting, reviewing, pinning, diagnosing, relating, or merging Engram memories. Keep personal memory outside this skill.
---

# Engram Memory CLI

Use Engram as a local-first, best-effort project memory. Keep the primary task
deliverable independent from memory availability.

## Establish the project

1. Confirm that `engram` is available. Continue the primary task without memory
   when it is missing or fails.
2. Run `engram current-project --json` before the first project-scoped operation.
3. Use the returned project when it is unambiguous. When `project` is empty,
   present `available_projects` and obtain an exact selection before writing or
   searching. Never infer a project from a similar name.
4. Pass `--project <project>` to subsequent project-scoped commands when the
   working directory does not resolve to that exact project.

Use `--json` for agent and script operations. Parse successful stdout as JSON;
parse non-zero stderr as `{"code","message","details?"}`. Treat failures as
best-effort unless the user's task is explicitly about Engram itself.

Complete project establishment when one exact project is known or memory use has
been skipped without blocking the primary task.

## Recall

Recall only when prior project knowledge could materially change the work.

1. Search one lookup intent with one to three distinctive anchors:

   ```bash
   engram search "<narrow query>" --project "<project>" --match-mode all --limit 5 --json
   ```

2. Inspect every result's content, state, pin, and relations. Use
   `engram get <id> --json` when a selected memory needs complete metadata or
   relation context.
3. If a material memory is expected and the first search is empty, refine once.
   Remove generic terms or switch to `--match-mode any`; keep the same intent.
4. Prefer the newest applicable memory while honoring `supersedes`,
   `superseded_by`, and `conflicts_with` relations. Surface unresolved conflicts
   instead of silently choosing one side.

Use `--all-projects` only for an explicitly cross-project request. Complete
recall when relevant context is accounted for or two targeted searches are
empty.

## Preserve

Preserve after the primary work has a durable result. Save one concise
observation for one durable subject; split only genuinely independent subjects.

1. Exclude transcripts, progress updates, speculation, secrets, personal facts,
   and facts already obvious from maintained project documentation.
2. Write structured content with `What`, `Why`, and `Where`:

   ```bash
   engram save "<concise title>" "What: <durable result>
   Why: <future value>
   Where: <subsystem or path>" --project "<project>" --json
   ```

3. Add `--topic-key <stable-key>` only for an evolving subject that later
   observations should update or group. When a stable key is warranted but
   unclear, run:

   ```bash
   engram suggest-topic-key --title "<title>" --content "<content>" --json
   ```
4. Inspect `judgment_required` after every save. For each candidate, inspect the
   candidate memory and resolve its own `judgment_id` as described in
   [references/curation.md](references/curation.md).

Complete preservation when the durable result is saved and every returned
candidate is resolved or explicitly raised to the user. Routine or low-value
work completes without a write.

## Curate

Read [references/curation.md](references/curation.md) when the task requires
editing or reviewing memories, changing local context priority, recording
relations, diagnosing the store, or merging project names. Follow its preview
and confirmation gates for destructive or multi-record changes.
