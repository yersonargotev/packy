# Spec: memory-conflict-surfacing

Phase: SDD spec
Change: memory-conflict-surfacing
Engram topic_key: `sdd/memory-conflict-surfacing/spec`
Artifact store: hybrid

---

## Purpose

Define what MUST be true after Phase 1 of memory-conflict-surfacing ships.
Covers schema, API contracts, behavior, and migration invariants.
Does NOT specify how — that is the design phase.

---

## Requirements

### REQ-001 — Conflict candidate detection on save

The system MUST run a post-transaction FTS5 candidate query after every `mem_save`
(new insert or topic_key revision). If top-N candidates above the BM25 floor exist,
the response MUST include `candidates[]`, `judgment_id`, `judgment_status: "pending"`,
and `judgment_required: true`. If no candidates exist, those fields MUST be absent
or zero-valued and the existing result string MUST be unchanged.

Default N = 3. Default BM25 floor = -2.0. Both MUST be configurable via `Config`.

#### Scenario: Happy path — similar title triggers candidates

- GIVEN a store containing one observation titled "We use sessions for auth"
- WHEN `mem_save` is called with title "Switched from sessions to JWT for auth"
- THEN the response includes `candidates` with at least 1 entry containing `id`, `sync_id`, `title`, `type`, `topic_key`, and `score`
- AND `judgment_id` is a non-empty string prefixed with `rel_`
- AND `judgment_required` is `true`
- AND the `result` string contains "CONFLICT REVIEW PENDING"

#### Scenario: Edge case — topic_key revision also returns candidates

- GIVEN a store containing observation A and B with similar titles
- WHEN `mem_save` is called with a topic_key that matches observation A (a revision)
- THEN candidates are returned (revisions are when conflicts emerge)
- AND the just-saved observation is excluded from its own candidates list

#### Scenario: Negative — unrelated title produces no candidates

- GIVEN a store containing observations with unrelated titles
- WHEN `mem_save` is called with a dissimilar title
- THEN `candidates` is empty or absent, `judgment_required` is `false`
- AND the `result` string is identical to today's format (no "CONFLICT REVIEW PENDING" line)

#### Scenario: Edge case — BM25 floor drops borderline candidates

- GIVEN two observations, one very similar (high BM25) and one borderline (below floor)
- WHEN `CandidateBM25Floor` is raised to a value that excludes the borderline candidate
- THEN only the high-BM25 candidate appears in `candidates`

---

### REQ-002 — Conflict markers on search results

The system MUST annotate each `mem_search` result entry with relation markers when
the observation is involved in judged relations. Markers MUST appear as lines
immediately following the standard result line for that observation.

Marker format:
```
supersedes: #<id> (<title>)
superseded_by: #<id> (<title>)
conflict: contested by #<id> (pending)
```

Observations with no relations MUST produce results identical to today's format.

#### Scenario: Happy path — superseded observation annotated

- GIVEN observation #18 has a judged relation where observation #42 supersedes it
- WHEN `mem_search` returns #18 in results
- THEN the result entry for #18 includes the line `superseded_by: #42 (<title>)`

#### Scenario: Happy path — source observation annotated

- GIVEN observation #42 has a judged `supersedes` relation targeting #18
- WHEN `mem_search` returns #42 in results
- THEN the result entry for #42 includes the line `supersedes: #18 (<title>)`

#### Scenario: Edge case — pending relation shown as contested

- GIVEN observation #77 has a relation with `judgment_status = 'pending'`
- WHEN `mem_search` returns #77
- THEN the result entry includes `conflict: contested by #<other_id> (pending)`

#### Scenario: Negative — no relations means no annotation

- GIVEN an observation has no rows in `memory_relations`
- WHEN `mem_search` returns it
- THEN its result entry is byte-for-byte identical to today's format (regression guard)

---

### REQ-003 — Agent-driven judgment via mem_judge

The system MUST expose a `mem_judge` MCP tool in the `agent` profile.
It MUST accept `judgment_id` (required), `relation` (required), `reason` (optional),
`evidence` (optional TEXT, free-form), `confidence` (optional 0.0..1.0).

On success, it MUST update the `memory_relations` row: set `relation`, `reason`,
`evidence`, `confidence`, `actor`, `session_id`, `updated_at`, and flip
`judgment_status` to `judged`. It MUST return the updated relation row.

`mem_judge` MUST NOT mutate the target observation (retraction is a separate agent action).

Valid `relation` values: `related`, `compatible`, `scoped`, `conflicts_with`, `supersedes`, `not_conflict`.

#### Scenario: Happy path — agent records a verdict

- GIVEN a pending relation with `judgment_id = "rel_abc123"`
- WHEN `mem_judge` is called with `judgment_id="rel_abc123"`, `relation="not_conflict"`, `confidence=0.9`
- THEN the relation row has `judgment_status = 'judged'`, `relation = 'not_conflict'`, `confidence = 0.9`
- AND the updated relation row is returned in the response

#### Scenario: Edge case — optional fields are preserved as-is when omitted

- GIVEN a pending relation exists
- WHEN `mem_judge` is called with only `judgment_id` and `relation`
- THEN `reason`, `evidence`, `confidence` remain NULL in the row (not defaulted to empty string)

#### Scenario: Negative — unknown judgment_id returns typed error

- GIVEN no relation with id "rel_does_not_exist" exists
- WHEN `mem_judge` is called with that id
- THEN the response has `IsError: true` and a descriptive message
- AND no row is mutated

#### Scenario: Negative — invalid relation verb rejected

- GIVEN a pending relation exists
- WHEN `mem_judge` is called with `relation = "invalidverb"`
- THEN the response has `IsError: true`
- AND the relation row remains `judgment_status = 'pending'`

---

### REQ-004 — Multi-actor support at schema level

The schema MUST NOT have a UNIQUE constraint on `(source_id, target_id)`.
Two independent agents calling `mem_judge` for the same pair MUST both succeed
and produce two separate rows. Phase 1 surfaces both via `mem_search`; resolution logic is deferred.

#### Scenario: Happy path — two actors produce two rows

- GIVEN a pending relation for pair (A, B) exists
- WHEN agent-1 calls `mem_judge` with `relation="compatible"` and agent-2 calls `mem_judge` with `relation="conflicts_with"` for the same pair
- THEN both calls succeed and two rows exist in `memory_relations` for (A, B)
- AND both rows appear in `GetRelationsForObservation(A)`

#### Scenario: Negative — UNIQUE constraint on sync_id still enforced per-row

- GIVEN a relation row with `sync_id = "rel_xyz"` exists
- WHEN a second insert attempts to use `sync_id = "rel_xyz"`
- THEN the insert fails with a constraint error (each row has its own unique sync_id)

---

### REQ-005 — Provenance on every relation row

Every row inserted into `memory_relations` via `mem_judge` MUST include:
`marked_by_actor` (actor field in schema), `marked_by_kind` (`human|agent|system`),
`marked_by_model`, `confidence`, `evidence`, `created_at`. `marked_by_model` and
`evidence` MAY be NULL. `created_at` MUST NOT be NULL.

#### Scenario: Happy path — full provenance persisted

- GIVEN `mem_judge` is called by an agent model "claude-sonnet-4-6" with `confidence=0.85` and `evidence='{"basis":"title overlap"}'`
- WHEN the row is written
- THEN `actor = "agent:claude-sonnet-4-6"`, `confidence = 0.85`, `evidence = '{"basis":"title overlap"}'`, `created_at` is not NULL

#### Scenario: Edge case — human actor records a verdict (future path)

- GIVEN a relation is created with `actor = "user"` and no model field
- WHEN the row is read back
- THEN `actor = "user"`, `marked_by_model` is NULL, all other non-optional fields are populated

---

### REQ-006 — Decay schema populated on save (no activation)

When a new observation is saved, the system MUST populate `review_after` according
to type-default offsets. `expires_at` MUST be NULL for all types in Phase 1.
These values MUST NOT be applied retroactively to existing rows.

| type        | review_after offset |
|-------------|---------------------|
| decision    | +6 months           |
| policy      | +12 months          |
| preference  | +3 months           |
| observation | NULL                |
| (other)     | NULL                |

No query or command reads these columns in Phase 1.

#### Scenario: Happy path — decision gets review_after

- GIVEN a store with no observations
- WHEN `mem_save` is called with `type = "decision"`
- THEN the saved row has `review_after` approximately 6 months from `created_at`
- AND `expires_at` is NULL

#### Scenario: Edge case — observation type has no decay date

- GIVEN `mem_save` is called with `type = "observation"`
- WHEN the row is saved
- THEN `review_after` is NULL and `expires_at` is NULL

#### Scenario: Negative — pre-existing rows are not retroactively updated

- GIVEN a store migrated from a prior version with existing observation rows
- WHEN `migrate()` runs
- THEN existing rows still have `review_after = NULL` (migration is schema-only, not data-migration)

---

### REQ-007 — Backwards compatibility

The system MUST NOT break any existing MCP client behavior.

The `mem_save` response MUST preserve its existing leading `result` string line
(`Memory saved: "..." (type)`). New fields are additive. Old clients ignoring
unknown JSON fields MUST continue to work without modification.

The `mem_search` response text format MUST be unchanged for observations with
no relation markers. Annotation lines are additive and appear only when relevant.

The `mem_judge` tool being absent from older sessions MUST cause graceful degradation:
pending relations accumulate unjudged; nothing errors or breaks.

#### Scenario: Happy path — old client ignores new fields

- GIVEN a client that only reads `result` from the `mem_save` JSON envelope
- WHEN `mem_save` returns a response with `candidates`, `judgment_id`, etc.
- THEN the client reads `result` successfully without error (unknown fields ignored per JSON spec)

#### Scenario: Negative — existing result string assertions still pass

- GIVEN an existing test that asserts the `result` string starts with `Memory saved: "`
- WHEN the enriched `handleSave` runs with no candidates
- THEN the assertion still passes (result string unchanged when no candidates)

---

### REQ-008 — Migration safety

`migrate()` MUST be purely additive. After running on a pre-change database:
1. All existing observation rows MUST be intact (id, sync_id, content, created_at unchanged).
2. New columns on `observations` (`review_after`, `expires_at`, `embedding`, `embedding_model`, `embedding_created_at`) MUST exist with NULL for existing rows.
3. `memory_relations` table MUST exist.
4. Running `migrate()` a second time MUST be a no-op (idempotent).

Migration MUST use `addColumnIfNotExists` — no `DROP COLUMN`, no `ALTER COLUMN`.

#### Scenario: Happy path — legacy schema migrates cleanly

- GIVEN a SQLite DB created with the pre-change DDL containing 5 observation rows of mixed types
- WHEN `migrate()` runs on that DB
- THEN all 5 rows are present with identical id/sync_id/content/created_at values
- AND `review_after` and `expires_at` are NULL for all 5 pre-existing rows
- AND `memory_relations` table exists

#### Scenario: Edge case — idempotency

- GIVEN `migrate()` has already run successfully
- WHEN `migrate()` is called again on the same DB
- THEN no error is returned and the schema is identical to after the first run

#### Scenario: Negative — migration does not touch FTS5 virtual table or sync mutations

- GIVEN the DB has obs_fts virtual table and sync_mutations rows
- WHEN `migrate()` runs
- THEN obs_fts and sync_mutations are unchanged (no accidental DDL on them)

---

### REQ-009 — Local-only relations in Phase 1

The system MUST NOT enqueue `memory_relations` inserts or updates into `sync_mutations`.
Cloud servers MUST receive the same `syncObservationPayload` shape as before Phase 1.
New observation columns (`review_after`, `expires_at`, `embedding*`) MUST NOT appear
in the sync wire format in Phase 1.

#### Scenario: Happy path — relation row does not appear in sync_mutations

- GIVEN cloud sync enrollment is active
- WHEN `mem_judge` is called and a relation row is inserted
- THEN `sync_mutations` table contains no row referencing the new relation
- AND the existing sync test suite passes without modification

#### Scenario: Negative — observation sync payload is unchanged

- GIVEN an observation is saved with `review_after` populated
- WHEN the sync mutation for that observation is enqueued
- THEN the mutation payload does NOT contain `review_after`, `expires_at`, or `embedding*` fields

---

### REQ-010 — Orphaned relations on observation delete

When an observation referenced by a `memory_relations` row (as `source_id` or `target_id`)
is hard-deleted, the relation row MUST NOT be cascade-deleted.
Instead, `judgment_status` MUST become `orphaned`.
`mem_search` result annotations MUST skip `orphaned` relations.

#### Scenario: Happy path — deleting source orphans the relation

- GIVEN a relation row with `source_id = "obs_aaa"` and `judgment_status = "judged"`
- WHEN the observation with `sync_id = "obs_aaa"` is hard-deleted
- THEN the relation row still exists with `judgment_status = 'orphaned'`
- AND `source_id` is NULL (ON DELETE SET NULL) or the row is updated to orphaned status

#### Scenario: Edge case — orphaned relation is invisible in search annotations

- GIVEN an observation has one orphaned relation and one judged relation
- WHEN `mem_search` returns that observation
- THEN only the judged relation's annotation lines appear; the orphaned one is skipped

#### Scenario: Negative — orphaned relation does not block new relations on surviving observations

- GIVEN observation B has an orphaned relation (A was deleted) and a new similar observation C is saved
- WHEN `mem_save` for C triggers candidate detection
- THEN B is still eligible as a candidate (orphaned relations do not taint B's retrievability)

---

## Acceptance Criteria Summary

| REQ | Happy Path | Edge Case | Negative |
|-----|-----------|-----------|----------|
| 001 | similar title → candidates + judgment_id | topic_key revision → candidates | unrelated → no candidates |
| 002 | supersedes annotated on search | pending → contested marker | no relations → format unchanged |
| 003 | mem_judge records verdict | optional fields stay NULL | unknown id → IsError |
| 004 | two actors → two rows for same pair | sync_id unique per row | — |
| 005 | full provenance on row | human actor, NULL model | — |
| 006 | decision gets review_after +6mo | observation type → NULL | pre-existing rows untouched |
| 007 | old client reads result string | no-candidate response unchanged | — |
| 008 | legacy DB migrates cleanly | idempotent second run | FTS5/sync_mutations untouched |
| 009 | relation not in sync_mutations | obs payload unchanged | — |
| 010 | deleting source orphans relation | orphaned skipped in annotations | orphaned doesn't block B as candidate |

---

## Implementation Hints

- `internal/store/store.go`: `migrate()` adds columns via `addColumnIfNotExists`, creates `memory_relations` table via `CREATE TABLE IF NOT EXISTS`; new methods: `FindCandidates`, `SaveRelation`, `JudgeRelation`, `GetRelationsForObservation`.
- `internal/mcp/mcp.go`: `handleSave` calls `FindCandidates` post-transaction and enriches JSON envelope; `handleSearch` calls `GetRelationsForObservation` per result and appends annotation lines; `handleJudge` + `mem_judge` tool registration in `agent` profile.
- `internal/store/store_test.go`: `newTestStoreWithLegacySchema(t)` helper opens temp SQLite, runs pre-change DDL string, inserts fixture rows, calls `migrate()`. Decay constant tests use `time.Now().AddDate(0, N, 0)` with a small tolerance window.
- `internal/mcp/mcp_test.go`: envelope shape assertions on `handleSave`; regression: result substring `Memory saved: "` still present; `handleJudge` success + error table tests.
- Configurable constants: `DefaultCandidateBM25Floor = -2.0`, `DefaultCandidateLimit = 3`, decay offsets as named month constants.
- `sync_id` generation pattern for relations: reuse existing `newSyncID()` or equivalent; prefix `rel_` for human readability in tool output.
