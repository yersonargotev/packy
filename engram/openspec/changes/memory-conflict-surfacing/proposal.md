# Proposal: memory-conflict-surfacing

Status: proposal
Phase: SDD propose
Author: SDD orchestrated session
Date: 2026-04-24
Engram topic_key: `sdd/memory-conflict-surfacing/proposal`

---

## 1. Why

Engram is the agent's persistent memory across sessions and projects. As more
than one agent (or one agent across many sessions) writes into a shared store,
two structural problems emerge that today have **no first-class surfacing
mechanism**:

1. **Semantic contradictions go undetected.** Two observations may disagree
   ("we use sessions" vs "we switched to JWT") but the store has no way to flag
   that fact. They sit side-by-side, indexed by FTS5, equally retrievable.
2. **Provenance is invisible.** Even when a human or agent realizes one memory
   supersedes another, that judgment is not recorded in a way later sessions can
   read. There is no audit trail of "this fact was retired, here is why, by
   whom, when".

Engram already has the substrate to detect candidates for conflict (FTS5 with
BM25 scoring is wired into `Search()` in `internal/store/store.go`) and the
substrate to record relations (additive SQLite migrations via
`addColumnIfNotExists` plus new tables are an established pattern). What is
missing is the **product layer**: a way for the agent to ask "does this new
memory contradict anything I already know?", judge the answer, and persist that
judgment with provenance so the next session can read it.

This change is **memory-conflict-surfacing**, deliberately named — surfacing
*is* the product. The end-user (developer) NEVER touches conflict resolution
through CLI commands or dashboards in Phase 1. The agent does the work,
asking the user via natural conversation only when the judgment is ambiguous.

### Why now

- Multi-agent and multi-session usage of Engram is rising in real use, so the
  contradiction surface is growing.
- The schema/API decisions made now (especially `source_id` / `target_id` shape
  in relations, embedding column placement, judgment workflow) lock in the
  trajectory for cloud sync, decay activation, and admin observability in
  Phase 2+. Getting them wrong now is expensive later.
- Strict TDD is enabled for this project, so the new tests we add (including
  the first migration N→N+1 tests this codebase has) raise the safety floor for
  every future schema change.

### What success looks like

- When the agent calls `mem_save` and the new memory has plausible candidates
  for conflict in the local store, the response includes structured
  `candidates[]` and a `judgment_id`, plus a `judgment_status: "pending"` flag.
- The agent calls `mem_judge` with one of the supported relation verdicts
  (`compatible`, `scoped`, `conflicts_with`, `supersedes`, `not_conflict`,
  `related`). The judgment is persisted in `memory_relations` with full
  provenance.
- When the agent later calls `mem_search`, returned observations include
  `conflict_markers`, `supersedes`, and `superseded_by` so the agent sees the
  contested status of each result without an extra call.
- Phase 1 ships with no regressions to the existing save/search/sync pipeline,
  and with the first migration tests this codebase has ever had.

---

## 2. What changes (high-level shape)

### 2.1 Schema additions

**New table: `memory_relations`** (local SQLite only in Phase 1).

Columns:

```
id                       INTEGER PRIMARY KEY AUTOINCREMENT
sync_id                  TEXT NOT NULL UNIQUE
source_id                TEXT NOT NULL    -- observations.sync_id (cross-machine portable)
target_id                TEXT NOT NULL    -- observations.sync_id
relation                 TEXT NOT NULL    -- related|compatible|scoped|conflicts_with|supersedes|not_conflict
reason                   TEXT             -- agent's natural-language explanation
evidence                 TEXT             -- optional structured evidence (JSON string)
confidence               REAL             -- 0.0..1.0 from the agent
judgment_status          TEXT NOT NULL DEFAULT 'pending'  -- pending|judged|orphaned|ignored
actor                    TEXT             -- "agent:<model>" or "user"
session_id               TEXT             -- which session produced the judgment
superseded_at            TEXT             -- when this relation itself was retracted
superseded_by_relation_id INTEGER         -- FK self-ref to memory_relations.id (nullable)
created_at               TEXT NOT NULL DEFAULT datetime('now')
updated_at               TEXT NOT NULL DEFAULT datetime('now')
```

Indexes:
- `(source_id, judgment_status)` — for "is this observation contested?" lookups
- `(target_id, judgment_status)` — for reverse lookups
- `(sync_id)` UNIQUE — for future sync addressability
- `(superseded_by_relation_id)` — for retraction chains

**Why TEXT `sync_id` for source/target (not INTEGER `id`)**: Phase 2 needs
cross-machine portability for relations once cloud sync is enabled. Locking the
right design now avoids a painful schema migration later. The exploration
confirmed `sync_id` is already populated on every observation row; using it as
the relation key is essentially free in Phase 1.

**Why no UNIQUE on `(source_id, target_id)`**: multi-actor disagreement (two
agents reaching different conclusions on the same pair) is allowed at the
schema level. The *resolution* logic for that disagreement is deferred to Phase
2, but the schema must not preclude it. A composite UNIQUE would make Phase 2
require a destructive migration.

**New columns on `observations`** (all nullable, additive via
`addColumnIfNotExists`):

```
review_after          TEXT NULL    -- "consider revisiting after this date"
expires_at            TEXT NULL    -- "this observation is stale after this date"
embedding             BLOB NULL    -- local embedding (generation deferred)
embedding_model       TEXT NULL    -- which model produced the embedding
embedding_created_at  TEXT NULL    -- when the embedding was generated
```

**Decay defaults** (encoded as in-code constants, applied to NEW saves only,
not retroactively):

```
type=decision    → review_after = created_at + 6 months
type=policy      → review_after = created_at + 12 months
type=preference  → review_after = created_at + 3 months
type=observation → review_after = NULL (no auto-decay)
type=*           → expires_at   = NULL (never auto-expire in Phase 1)
```

The actual *querying* on these columns ("show me what's stale") is **out of
scope** in Phase 1. The columns are populated, indexed nowhere yet, and ready
for Phase 2 to activate.

### 2.2 MCP API extensions

**Enriched `mem_save` response.** Today `mem_save` returns a JSON envelope from
`respondWithProject` whose `result` is a human-readable string. We extend the
envelope with structured fields:

```json
{
  "project": "...",
  "project_source": "...",
  "project_path": "...",
  "id": 12345,
  "sync_id": "obs_...",
  "candidates": [
    {"id": 1234, "sync_id": "obs_...", "title": "...", "type": "...",
     "topic_key": "...", "score": -1.42}
  ],
  "judgment_id": "rel_...",
  "judgment_status": "pending",
  "judgment_required": true,
  "result": "Memory saved: \"...\" (decision)\nCONFLICT REVIEW PENDING: 2 candidates may relate. Call mem_judge(judgment_id=\"rel_...\") with your verdict.\nSuggested topic_key: ..."
}
```

`candidates` and `judgment_id` are present only when at least one candidate
passes the BM25 floor. When no candidates exist, both fields are absent (or
`candidates: []`, `judgment_id: null`, `judgment_required: false`) and the
human-readable `result` string is unchanged from today.

**Belt-and-suspenders**: the `result` string also embeds a "CONFLICT REVIEW
PENDING" line so agents that parse only the human-readable text still see the
nudge. This is the same pattern `SessionActivity.NudgeIfNeeded` already uses.

**Enriched `mem_search` results.** Each result line in the human-readable
result string gets optional annotations when the observation is involved in
relations:

```
[1] #42 (decision) — Switched to JWT
    preview... [preview]
    2026-04-20 12:00 | project: engram | scope: project
    supersedes: #18 (Use sessions)         ← only if this is a "supersedes" target
    superseded_by: #99 (Switched back to sessions)  ← only if relevant
    conflict: contested by #77 (pending)   ← unresolved relation
```

Old agents that read the string ignore unfamiliar lines. New agents (with the
serverInstructions update) know to look for these markers.

**New tool: `mem_judge`** (in the `agent` profile — always available).

Input parameters:

```
judgment_id   (string, required)   the rel_... id from mem_save
relation      (string, required)   one of: related|compatible|scoped|
                                   conflicts_with|supersedes|not_conflict
reason        (string, optional)   agent's natural-language explanation
evidence      (string, optional)   JSON-encoded supporting data
confidence    (number, optional)   0.0..1.0
```

Behavior:
- Loads the pending relation by `judgment_id`.
- Updates `relation`, `reason`, `evidence`, `confidence`, `actor`, `session_id`.
- Sets `judgment_status = 'judged'`, `updated_at = now()`.
- If `relation = "supersedes"`, the agent is responsible for separately calling
  `mem_save` again or another path that retracts the target — `mem_judge`
  itself does NOT mutate the target observation, only records the judgment.
  This keeps the tool single-purpose.
- Returns the saved relation row plus a confirmation message.

**Relation vocabulary** (locked):

| relation | meaning |
|---|---|
| `related` | weakly connected (same topic, no contradiction) |
| `compatible` | both true together (no conflict) |
| `scoped` | both true in different scopes (e.g. project A vs project B) |
| `conflicts_with` | mutually exclusive, neither is yet superseded |
| `supersedes` | source replaces target (target is now stale) |
| `not_conflict` | the candidate was a false positive |

**Judgment status vocabulary** (locked):

| judgment_status | meaning |
|---|---|
| `pending` | created by mem_save, awaiting agent verdict |
| `judged` | agent has called mem_judge |
| `orphaned` | source or target observation was deleted |
| `ignored` | session ended without judgment, will not be retried |

### 2.3 Agent skill instructions

The MCP `serverInstructions` constant in `internal/mcp/mcp.go` is updated to:

- Document `mem_judge` and the contract: "if `mem_save` response contains
  `judgment_required: true` or the result string contains `CONFLICT REVIEW
  PENDING`, call `mem_judge` before continuing".
- Document when to ask the user vs decide autonomously:
  - `not_conflict`, `related`, `compatible` → agent decides silently.
  - `scoped` → agent decides silently if the scopes are obvious from project /
    topic_key; otherwise asks user.
  - `supersedes`, `conflicts_with` → agent always asks the user via natural
    conversation before recording a destructive verdict.
- Document the new `mem_search` annotation lines so agents know what
  `supersedes:`, `superseded_by:`, and `conflict:` mean.

### 2.4 Detection strategy (FTS5 + agent judgment)

Local detection runs **post-transaction in `handleSave`** (Option 3 from
exploration §A). The save commits first; then a candidates query runs. The
result is included in the `mem_save` response.

Query shape:

- FTS5 MATCH on `title` of the incoming observation (title is the most
  signal-dense column).
- Filter: same `project`, same `scope`, exclude the just-saved row by `id`,
  exclude soft-deleted rows.
- Limit: top 3 by BM25 rank.
- Threshold: configurable BM25 floor (default starting value `-2.0`, exposed
  as a `Config` field). Below this floor, candidates are dropped.
- Candidates are returned for both **new inserts** and **topic_key revisions**
  (revisions are exactly when conflicts emerge — confirmed in pre-decided
  context §4).
- A pending `memory_relations` row is created per candidate, with
  `judgment_status = 'pending'` and `relation = 'related'` as a placeholder.

**No vector DB locally.** Cloud schema reserves embedding columns for Phase 2;
Phase 1 ships them empty.

### 2.5 Out-of-scope agent affordances (deferred)

These are explicitly NOT in Phase 1 — listed here so reviewers do not expect
them:

- `engram conflicts list` / `engram conflicts show` / `engram conflicts mark`
  CLI for end-users.
- Cloud admin dashboard for cross-team conflict visibility.
- pgvector embedding generation (schema only).
- `engram review` command for stale observations (decay query).
- Cloud sync of `memory_relations` rows.
- Multi-actor disagreement *resolution* logic (the schema permits it, the
  resolution algorithm is Phase 2+).

---

## 3. Out of scope (explicit deferrals)

| Item | Why deferred |
|---|---|
| pgvector / embedding generation | Cost + latency + model choice need their own design. Schema reserves columns. |
| `engram review` decay command | Decay columns are populated; activation is a separate UX surface. |
| Cloud sync of `memory_relations` | Avoids touching the cloud mutation pipeline in Phase 1. Phase 2 will add the entity type. |
| Cloud admin dashboard for conflicts | Cross-team observability is a separate concern; Engram is local-first first. |
| Multi-actor disagreement resolution | Schema supports it (no UNIQUE on source+target), logic is Phase 2+. |
| End-user CLI for conflicts | Engram is the agent's memory, not the user's database. The agent surfaces, not the CLI. |
| Per-column FTS weighting for candidate query | modernc.org/sqlite v1.45.0 doesn't expose per-column weights; title-only query is the workaround. |
| Retraction of target observation on `supersedes` verdict | `mem_judge` records the judgment only; mutating the target is a separate path the agent invokes deliberately. |

---

## 4. Open questions

The pre-decided context resolved most architectural forks. These are the
residual questions for the spec/design phases:

1. **Exact column types in cloud Postgres mirror.** When relations eventually
   sync (Phase 2), the cloud table schema must be defined now if we want type
   parity. Should we define `cloud_memory_relations` in cloud schema in Phase 1
   even though it stays empty? Recommendation: yes, in design phase, to lock
   types — but no migration runs until Phase 2 enables sync.

2. **`superseded_by_relation_id` self-FK enforcement.** SQLite supports
   self-referential FKs with `PRAGMA foreign_keys = ON` (already set). The
   spec/design phase must confirm whether to declare it as a real `FOREIGN KEY
   (... ) REFERENCES memory_relations(id) ON DELETE SET NULL` or leave it as a
   plain INTEGER and enforce in code. Recommendation: declare the FK; matches
   our existing `session_id` FK pattern.

3. **BM25 floor empirical tuning.** Default `-2.0` is a starting point. The
   spec/design phase should document how to tune it (turn knob via Config,
   measure precision/recall on a real corpus). Phase 1 ships the constant; the
   tuning loop is operational, not architectural.

4. **`evidence` field shape.** It's a TEXT column holding a JSON string in the
   schema. Should the spec define a JSON schema for it now, or leave it free-form
   for Phase 1? Recommendation: free-form in Phase 1, document common shapes in
   comments, formalize the JSON schema in Phase 2 when we have real usage data.

5. **Server-instructions wording for ambiguity threshold.** "When in doubt,
   ask the user via natural conversation" — how does the agent decide what
   "ambiguous" means? The design phase should propose concrete heuristics
   (e.g. confidence < 0.7, relation in {supersedes, conflicts_with}).

---

## 5. Affected modules

### Touched

- **`internal/store/store.go`**
  - `migrate()` — new columns on `observations`, new `memory_relations` table,
    new indexes.
  - `AddObservation` return signature — additionally returns candidates and a
    pending `judgment_id` (or refactor: keep AddObservation pure, add a separate
    `FindCandidates(observationID)` method called by handleSave).
  - New methods: `FindCandidates`, `SaveRelation`, `GetRelation`,
    `JudgeRelation`, `GetRelationsForObservation`.
  - Decay defaults applied in `AddObservation` based on `type`.

- **`internal/mcp/mcp.go`**
  - `serverInstructions` constant updated.
  - `handleSave` enriched response (post-transaction candidate query).
  - `handleSearch` result string enriched with relation annotations.
  - New `mem_judge` tool registered (in `agent` profile).
  - New `handleJudge` handler.

- **`internal/store/store_test.go`**
  - First migration N→N+1 tests in this codebase. Pattern: build store with
    pre-migration schema directly via SQL, run `migrate()`, assert columns
    and tables exist, assert old observation rows survive untouched.
  - Tests for `FindCandidates`, `SaveRelation`, `JudgeRelation`,
    `GetRelationsForObservation`.

- **`internal/mcp/mcp_test.go`**
  - Tests for `handleSave` candidates response shape.
  - Tests for `handleJudge` (success, unknown judgment_id, invalid relation).
  - Tests for `handleSearch` annotation lines.

### NOT touched in Phase 1

- `internal/cloud/cloudstore/cloudstore.go` — no relation entity until Phase 2.
- `internal/cloud/cloudserver/*` — no admin dashboard for conflicts.
- `internal/sync/sync.go` — `ChunkData` does not gain a `Relations` field.
- `cmd/engram/main.go` — MCP server creation is transparent; no flags added.
- Any plugin / hook scripts — no new endpoints to reference.

This boundary discipline matches the project standards
(`engram-architecture-guardrails`): keep adapters thin, fit new features to the
local-first model, respect the boundaries between store, cloudstore, and
cloudserver.

---

## 6. Test strategy (TDD, strict mode active)

Test runner: `go test ./...`

Strict TDD is enabled, so each task in the eventual `tasks.md` will be of the
form RED → GREEN → REFACTOR. The proposal commits to these test categories:

### 6.1 Migration tests (NEW infrastructure)

This codebase has no migration N→N+1 tests today. We establish the pattern:

1. `newTestStoreWithLegacySchema(t)` helper — opens a SQLite DB at a temp path,
   executes the *pre-change* DDL directly (a known-good snapshot of today's
   schema as a string constant), inserts one observation row of each type
   (decision, policy, preference, observation, manual), then constructs a
   `Store` over that DB which triggers `migrate()`.
2. Assert post-migration:
   - `observations` table has the new columns (`review_after`, `expires_at`,
     `embedding`, `embedding_model`, `embedding_created_at`).
   - `memory_relations` table exists with the documented columns and indexes.
   - All pre-existing observation rows are intact (id, content, sync_id,
     created_at unchanged).
   - Running `migrate()` a second time is a no-op (idempotency check).

### 6.2 Candidate detection tests

- Empty store + new save → no candidates, `judgment_required: false`.
- One unrelated observation + new save with different title → no candidates.
- One similar-title observation + new save → exactly 1 candidate, BM25 score
  populated, pending relation row created.
- Three similar-title observations + new save → top-3 returned, all below the
  configured floor are dropped.
- BM25 floor configurable via `Config.CandidateBM25Floor`; test that raising
  the floor drops borderline candidates.
- topic_key revision (re-save same topic_key) → candidates are still returned
  (revisions are when conflicts emerge).

### 6.3 Relation CRUD tests

- `SaveRelation(pending)` writes a row with `judgment_status='pending'` and
  generates a `sync_id`.
- `JudgeRelation(judgment_id, relation, ...)` updates the row; status flips to
  `judged`; reason/evidence/confidence/actor/session_id populated.
- `JudgeRelation` with unknown `judgment_id` → returns a typed error.
- `JudgeRelation` with invalid relation verb → returns a typed error.
- Deleting source or target observation → relation moves to `orphaned` (or:
  document that orphaning is on-read, not on-delete; design phase decides).
- `GetRelationsForObservation(sync_id)` returns both directions (source and
  target) and respects `judgment_status` filter.

### 6.4 MCP handler tests

- `handleSave` JSON envelope contains `id`, `sync_id` always; contains
  `candidates`, `judgment_id`, `judgment_required` only when candidates exist.
- `handleSave` human-readable `result` string includes "CONFLICT REVIEW PENDING"
  line when `judgment_required: true`.
- `handleJudge` updates the relation and returns the updated row.
- `handleJudge` with unknown id returns an error result (`IsError: true`).
- `handleSearch` result string includes `supersedes:`, `superseded_by:`,
  `conflict:` annotation lines when applicable.
- `handleSearch` result for an observation with no relations is unchanged from
  today (regression guard).

### 6.5 Boundary/regression tests (per `engram-server-api`, `engram-architecture-guardrails`)

- Existing `mem_save` happy-path tests still pass with the enriched response
  shape (old assertions on `result` substring should still hold).
- Existing `mem_search` happy-path tests still pass.
- Existing sync tests still pass (we did not touch the sync entity types).
- New cloudstore boundary test: a relation row in local SQLite does NOT enqueue
  a sync mutation (relations are local-only in Phase 1).

---

## 7. Migration safety

The migration is purely additive and uses the established
`addColumnIfNotExists` pattern.

Guarantees:

1. **No DROP / ALTER COLUMN.** Existing columns are untouched.
2. **No NOT NULL on new columns.** All new columns on `observations`
   (`review_after`, `expires_at`, `embedding`, `embedding_model`,
   `embedding_created_at`) are nullable. Existing INSERT statements that don't
   mention them continue to work.
3. **No data migration.** Existing observation rows are unchanged. Decay
   defaults apply to NEW saves only, not retroactively.
4. **`memory_relations` is a new CREATE IF NOT EXISTS table.** No collision
   with existing tables.
5. **Idempotency.** Running `migrate()` twice produces the same final schema
   (asserted by test §6.1).
6. **FTS5 untouched.** The FTS virtual table and its triggers
   (`obs_fts_insert/update/delete`) are not modified. FTS continues to mirror
   `observations` as it does today.
7. **Sync mutations unchanged.** The `sync_mutations` write path
   (`enqueueSyncMutationTx`) is not modified. New columns on `observations` are
   not added to `syncObservationPayload` in Phase 1 (they would be wire-format
   noise; we add them in Phase 2 when there's something to sync). Relations do
   not enqueue mutations at all.

The first migration N→N+1 tests added here become a regression floor for every
future schema change.

---

## 8. Backwards compatibility

### MCP clients

- **`mem_save` response.** Already a JSON envelope from `respondWithProject`.
  We add new top-level fields (`id`, `sync_id`, `candidates`, `judgment_id`,
  `judgment_status`, `judgment_required`). Old clients ignore unknown fields.
  The `result` string keeps its existing leading line (`Memory saved: "..." (type)`)
  and only appends new content; substring assertions on the existing first line
  continue to hold.
- **`mem_search` response.** Still a single text block. New annotation lines
  (`supersedes:`, `superseded_by:`, `conflict:`) appear only when relevant.
  Old agents that string-parse results either ignore the new lines or
  optionally surface them — neither breaks.
- **`mem_judge` tool.** Purely additive. Old agents that don't call it leave
  pending relations sitting at `judgment_status='pending'`. The system degrades
  gracefully — nothing breaks, conflicts simply accumulate unjudged.
- **Server instructions update.** Older agents that don't read
  `serverInstructions` continue to work; they just don't know about the new
  contract. Confirmed in pre-decided context §6.

### Sync / cloud

- No cloud schema changes in Phase 1.
- No `sync_mutations` payload changes for relations (relations are local-only).
- `syncObservationPayload` does not gain new columns in Phase 1, so old cloud
  servers continue to receive the same payload shape they receive today. New
  observation columns exist locally only.

### Existing tests

- Existing assertions on `mem_save` result strings continue to hold (we only
  append, never replace).
- Existing assertions on `mem_search` text format continue to hold for results
  not involved in any relation.
- Existing migration is unchanged for the first-run case (no observations + no
  relations); the legacy-schema migration test is a new path.

---

## 9. Phase 2 hooks (what Phase 1 enables)

The shape decisions in this proposal are deliberately chosen to make Phase 2
mechanical, not architectural:

1. **Cloud sync of relations.** `memory_relations` already has a `sync_id`
   UNIQUE column. Phase 2 adds:
   - A new `SyncEntityRelation = "relation"` constant.
   - `syncRelationPayload` struct mirroring the row.
   - `applyRelationUpsertTx` / `applyRelationDeleteTx`.
   - `cloud_memory_relations` table on Postgres.
   - No schema migration on the local table — it's already cloud-ready.

2. **Decay activation (`engram review`).** Columns `review_after` and
   `expires_at` are populated as of Phase 1. Phase 2 adds an indexed query
   (`WHERE review_after < datetime('now') AND deleted_at IS NULL`) plus a CLI
   subcommand. No schema migration.

3. **Embedding generation.** `embedding`, `embedding_model`, and
   `embedding_created_at` columns exist as nullable. Phase 2 adds:
   - A generation pipeline (model choice, batching, cost).
   - Possibly pgvector on the cloud side via a new Postgres column.
   - Possibly sqlite-vec on the local side (requires a build dependency
     change — separate proposal).

4. **Multi-actor disagreement resolution.** Schema permits multiple relations
   on the same `(source_id, target_id)` from different actors. Phase 2 adds
   the resolution algorithm (e.g. weighted-by-confidence aggregation, latest-
   wins, user-as-tiebreaker). No schema migration.

5. **Cloud admin dashboard for conflicts.** Once relations sync, the cloud
   server can expose a per-org view of pending and judged conflicts. No new
   data model — purely a read-side feature on top of the synced relations.

6. **`mem_judge` hardening.** The judgment vocabulary is locked in Phase 1 but
   could be extended in Phase 2 (e.g. `partially_supersedes`, `requires_review`)
   without breaking Phase 1 clients — they'd just see unfamiliar relations and
   render them generically.

7. **Embeddings-augmented candidate detection.** Phase 1 candidate retrieval is
   FTS5 only. Once embeddings exist, the same `FindCandidates` interface can be
   re-implemented to combine FTS5 + embedding similarity (hybrid retrieval) with
   no change to `handleSave` or the relation schema.

---

## 10. Risks and mitigations

| Risk | Severity | Mitigation |
|---|---|---|
| FTS candidate noise (false positives) | Medium | top-3 cap + BM25 floor + agent judgment filters; document the floor as configurable. |
| Agent doesn't reliably call `mem_judge` | High | structured `judgment_required` field + human-readable "CONFLICT REVIEW PENDING" line + serverInstructions update + tool description contract. Belt-and-suspenders. |
| `sync_id`-keyed relations confuse readers expecting `INTEGER id` | Low | document in code comments + spec; tests assert sync_id is the join key. |
| Migration test infrastructure is new and could be fragile | Medium | establish the pattern in this change, document the legacy-schema snapshot, treat the snapshot as committed test data. |
| Decay columns sit unused, future implementors don't know intent | Low | document defaults in code constants + comment block + this proposal as the design-of-record. |
| `mem_save` JSON envelope shape change confuses agents | Low | envelope is already JSON today; we add fields, not replace them. |
| Multi-actor disagreement creates relation table bloat | Low | indexes on `(source_id, judgment_status)` and `(target_id, judgment_status)` keep lookups fast; bloat is bounded by observation count. |
| BM25 floor `-2.0` is wrong for real corpora | Low | configurable via `Config.CandidateBM25Floor`; tuning is a Phase 2 ops loop. |

---

## 11. Decision summary (locked in by this proposal)

1. Detection runs **post-transaction in `handleSave`** (sync, not async).
2. Candidate query: FTS5 MATCH on **title only**, **top 3**, configurable BM25
   floor (default `-2.0`).
3. Candidates returned for **both new inserts and topic_key revisions**.
4. `memory_relations.source_id` / `target_id` are **TEXT `sync_id`**, not
   INTEGER.
5. **No UNIQUE** on `(source_id, target_id)` — schema allows multi-actor
   disagreement; resolution logic deferred.
6. `mem_judge` is in the **`agent` profile** (always available).
7. Relations are **local-only in Phase 1** — no cloud sync, no
   `sync_mutations` enqueue.
8. **Decay columns populated, decay query deferred.**
9. **Embedding columns exist, generation deferred.**
10. **No end-user CLI** for conflicts. Agent surfaces via natural conversation.

---

## 12. Recommended next phases

- **`sdd-spec`** — formalize the data contracts: SQL DDL for `memory_relations`,
  exact JSON schema for the `mem_save` enriched envelope, exact text format
  for `mem_search` annotation lines, exact input/output for `mem_judge`,
  decay-default constants, BM25 floor constant.
- **`sdd-design`** — formalize the architecture: where `FindCandidates` lives
  (store method? new package?), the orphaning policy for relations whose
  source/target observations are deleted, the `serverInstructions` rewrite,
  the migration-test helper structure, the boundary between `handleSave` and
  the new candidate query (transaction discipline).

These two phases can run **in parallel** — the spec defines *what*, the design
defines *how*. They converge in `sdd-tasks` which produces the TDD-shaped
checklist for `sdd-apply`.
