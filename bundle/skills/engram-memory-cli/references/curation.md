# Engram Memory Curation

Use these branches only after establishing an exact project as described in
`SKILL.md`. Prefer `--json` for every supported operation.

## Inspect and edit

- Retrieve a complete observation and its relations:
  `engram get <id> --json`.
- Partially edit one observation:
  `engram update <id> [--title V] [--content V] [--type V] [--scope V]
  [--topic-key V|--clear-topic-key] --json`.
- Inspect chronological neighbors:
  `engram timeline <id> --before <n> --after <n> --json`.
- Soft-delete by default with `engram delete <id> --json`. Treat `--hard` as
  irreversible and use it only when the user explicitly requests permanent
  deletion.

An update cannot move a memory between projects. Complete an edit only after
retrieving the observation again and confirming the intended fields and
relations remain intact.

## Review and local context

- List due memories in one project:
  `engram review list --project <project> --limit <n> --json`.
- After verifying a memory remains useful and accurate, mark it reviewed:
  `engram review mark <id> --json`.
- Pin a repeatedly relevant memory with `engram pin <id> --json`; restore normal
  recency with `engram unpin <id> --json`.

Review timestamps and pins are local-only and do not sync. Complete review when
every listed memory is either confirmed and marked, corrected, or surfaced for
human judgment.

## Resolve relations

Allowed relations are `related`, `compatible`, `scoped`, `conflicts_with`,
`supersedes`, and `not_conflict`. Resolving an existing candidate with
`engram conflicts judge` persists every verdict, including `not_conflict`.
Direct comparison has a different contract, described below.

When `engram save --json` returns candidates:

1. Handle each candidate independently using its own `judgment_id`.
2. Compare the saved observation with `engram get <candidate-id> --json`.
3. Ask the user when confidence is below `0.7`, or when the relation is
   `supersedes` or `conflicts_with` and either memory captures architecture,
   policy, or a decision.
4. Resolve autonomously only at confidence `>= 0.7` for `related`, `compatible`,
   `scoped`, or `not_conflict`:

   ```bash
   engram conflicts judge "<judgment-id>" --relation "<relation>" \
     --confidence "<0..1>" --reason "<brief reason>" --json
   ```

Use `engram conflicts compare <id-a> <id-b> --relation <relation> --confidence
<0..1> --reasoning "<brief reasoning>" --json` only after reaching a verdict.
It never performs semantic analysis. It persists `related`, `compatible`,
`scoped`, `conflicts_with`, and `supersedes`; `not_conflict` succeeds with an
empty `sync_id` and no new relation.

Complete relation work when every candidate and intended direct-comparison pair
has an evidence-backed verdict or is explicitly awaiting the user's decision.

## Diagnose

Run `engram doctor --project <project> --json` for read-only operational checks.
Use `--check <code>` to narrow a known finding. Keep repair outside ordinary
memory use: inspect the plan or dry run first, and apply only when the user has
explicitly authorized repair.

## Merge project names

Use deterministic merge only after the canonical project and every source name
are exact:

1. Preview without mutation:

   ```bash
   engram projects merge --from "<source>" --to "<canonical>" --dry-run --json
   ```

   Repeat `--from` for additional sources.

2. Report the preview counts and obtain explicit authorization.
3. Apply the same source and target set with `--yes --json`.
4. Verify the returned canonical name and every updated observation, session,
   prompt, and shadow-run count.

Complete a merge only after preview and applied results agree on the intended
source set. Use `projects merge` for exact agent-driven work; reserve heuristic
`projects consolidate` for an explicit user request.
