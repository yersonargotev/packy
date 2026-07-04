# Design: memory-conflict-surfacing

Phase: SDD design
Change: memory-conflict-surfacing
Engram topic_key: `sdd/memory-conflict-surfacing/design`
Artifact store: hybrid

---

## Technical Approach

The change is a thin product layer atop the existing local-first SQLite + FTS5 substrate. We add one local table (`memory_relations`), five additive nullable columns on `observations`, two new MCP fields on `mem_save`/`mem_search`, and one new MCP tool (`mem_judge`). Detection runs post-transaction in `handleSave` and never blocks the save. Provenance is recorded per-row. Cloud sync is deliberately untouched in Phase 1. Strict TDD: every REQ has a unit test plus a new migration N→N+1 helper this change introduces as standing infrastructure.

Boundary respected: persistence in `internal/store`, transport in `internal/mcp`, no cross-cutting changes in `cloud/*`, `sync/*`, `dashboard/*`.

---

## 1. Schema design (concrete DDL)

### 1.1 `memory_relations` table (NEW)

```sql
CREATE TABLE IF NOT EXISTS memory_relations (
    id                         INTEGER PRIMARY KEY AUTOINCREMENT,
    sync_id                    TEXT    NOT NULL UNIQUE,
    source_id                  TEXT,
    target_id                  TEXT,
    relation                   TEXT    NOT NULL DEFAULT 'pending',
    reason                     TEXT,
    evidence                   TEXT,
    confidence                 REAL,
    judgment_status            TEXT    NOT NULL DEFAULT 'pending',
    marked_by_actor            TEXT,
    marked_by_kind             TEXT,
    marked_by_model            TEXT,
    session_id                 TEXT,
    superseded_at              TEXT,
    superseded_by_relation_id  INTEGER REFERENCES memory_relations(id) ON DELETE SET NULL,
    created_at                 TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at                 TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_memrel_source     ON memory_relations(source_id, judgment_status);
CREATE INDEX IF NOT EXISTS idx_memrel_target     ON memory_relations(target_id, judgment_status);
CREATE INDEX IF NOT EXISTS idx_memrel_supersede  ON memory_relations(superseded_by_relation_id);
```

Source/target are TEXT `sync_id` (cross-machine portable). NO `UNIQUE(source_id,target_id)` — multi-actor disagreement permitted at schema level.

`relation` vocabulary (locked, validated in Go): `pending`, `related`, `compatible`, `scoped`, `conflicts_with`, `supersedes`, `not_conflict`. Default `pending` lives in DB; `mem_judge` flips it to one of the verbs.

`judgment_status` (locked): `pending`, `judged`, `orphaned`, `ignored`.

### 1.2 `observations` additive columns

```sql
ALTER TABLE observations ADD COLUMN review_after          TEXT;
ALTER TABLE observations ADD COLUMN expires_at            TEXT;
ALTER TABLE observations ADD COLUMN embedding             BLOB;
ALTER TABLE observations ADD COLUMN embedding_model       TEXT;
ALTER TABLE observations ADD COLUMN embedding_created_at  TEXT;
```

All nullable, no defaults, no backfill — applied via the existing `addColumnIfNotExists` helper.

### 1.3 Locked column names (provenance, resolves spec risk #2)

`marked_by_actor`, `marked_by_kind`, `marked_by_model`. Long names chosen for dashboard/audit clarity (Phase 2 will surface them).

---

## 2. Migration mechanism

`migrate()` in `internal/store/store.go` is extended with two additive blocks (placed AFTER the existing `observationColumns` loop and BEFORE FTS triggers):

| Step | Action | Idempotent via |
|------|--------|----------------|
| 1 | `addColumnIfNotExists("observations", "review_after", "TEXT")` ×5 | PRAGMA `table_info` check |
| 2 | `CREATE TABLE IF NOT EXISTS memory_relations (...)` | `IF NOT EXISTS` |
| 3 | `CREATE INDEX IF NOT EXISTS` for the four indexes | `IF NOT EXISTS` |

Order: column adds first → table create → indexes. No DROP. No ALTER COLUMN. No data migration. FTS5 untouched. `sync_mutations` untouched.

**Rollback**: explicit decision — none. SQLite cannot drop columns cleanly; the columns are nullable and harmless if we revert code. A second-run of `migrate()` is provably a no-op.

### 2.1 NEW test infrastructure: `newTestStoreWithLegacySchema(t)`

Pattern (introduced this change, becomes standing infra for every future schema change):

```go
func newTestStoreWithLegacySchema(t *testing.T, fixtureRows []legacyObsRow) *Store {
    t.Helper()
    dir := t.TempDir()
    dbPath := filepath.Join(dir, "engram.db")
    raw, err := sql.Open("sqlite", dbPath)
    if err != nil { t.Fatal(err) }
    if _, err := raw.Exec(legacyDDLPreMemoryConflictSurfacing); err != nil {
        t.Fatal(err)
    }
    for _, r := range fixtureRows {
        // INSERT into legacy tables with whatever columns existed pre-change
    }
    raw.Close()
    cfg := mustDefaultConfig(t); cfg.DataDir = dir
    s, err := New(cfg)
    if err != nil { t.Fatal(err) }
    return s
}
```

`legacyDDLPreMemoryConflictSurfacing` is a string constant pinned to the pre-change schema — once committed, it never changes. Each future schema change appends a new constant and a new helper, so we accumulate a versioned snapshot trail.

Migration tests assert: (a) all fixture rows survive byte-for-byte on key columns (id, sync_id, content, created_at), (b) new columns are NULL on existing rows, (c) `memory_relations` exists, (d) running `migrate()` again is a no-op (idempotency), (e) FTS5 virtual table and triggers are unchanged.

---

## 3. Candidate detection algorithm

### 3.1 Where it runs

Post-transaction in `handleSave` — AFTER `s.AddObservation(...)` returns successfully and BEFORE the `respondWithProject` envelope is built. The save commits regardless of detection success; any error from `FindCandidates` is logged and swallowed (best-effort signal, never blocks save).

### 3.2 New store method `FindCandidates`

```go
type Candidate struct {
    ID       int64
    SyncID   string
    Title    string
    Type     string
    TopicKey *string
    Score    float64
}

func (s *Store) FindCandidates(savedID int64, opts CandidateOptions) ([]Candidate, error)

type CandidateOptions struct {
    Project    string
    Scope      string
    Type       string
    Limit      int
    BM25Floor  *float64
}
```

### 3.3 FTS5 query strategy

Tokens: words from the saved observation's `title` only (signal-dense, matches existing `sanitizeFTS` behavior). The query reuses `sanitizeFTS(savedTitle)` to wrap each token in quotes.

```sql
SELECT o.id, o.sync_id, o.title, o.type, o.topic_key, fts.rank
FROM observations_fts fts
JOIN observations o ON o.id = fts.rowid
WHERE observations_fts MATCH ?
  AND o.id != ?
  AND o.deleted_at IS NULL
  AND ifnull(o.project,'') = ifnull(?,'')
  AND o.scope = ?
  AND fts.rank <= ?
ORDER BY fts.rank
LIMIT ?
```

### 3.4 Filtering rules

| Filter | Phase 1 behavior |
|--------|-------------------|
| Same project | enforced |
| Same scope | enforced |
| Type compatibility | NOT enforced Phase 1 |
| Soft-deleted | excluded |
| Just-saved row | excluded by `o.id != savedID` |

### 3.5 Returned shape

Top-3 by BM25 rank with floor `-2.0` (configurable via `Config.CandidateBM25Floor`). Empty slice when no rows clear the floor.

### 3.6 `judgment_id` generation

Each candidate row gets its own `sync_id`, surfaced as `candidates[i].judgment_id`. The top-level `judgment_id` is the first candidate's `judgment_id` (display convenience). Agents iterate `candidates[]` to call `mem_judge` per pair.

### 3.7 Performance

Worst case per save: 1 FTS5 MATCH query + N inserts (N ≤ 3). FTS5 MATCH on `title` over a 10k-row corpus measures sub-millisecond on modern SSDs. Total added latency budget: < 5 ms p95. Detection failure never fails the save.

---

## 4. `mem_save` response envelope (concrete JSON)

### 4.1 Existing fields preserved

```json
{
  "project": "engram",
  "project_source": "git_remote",
  "project_path": "/Users/.../engram",
  "result": "Memory saved: \"Switched from sessions to JWT\" (decision)"
}
```

### 4.2 New fields (additive)

```json
{
  "project": "engram",
  "project_source": "git_remote",
  "project_path": "/Users/.../engram",
  "result": "Memory saved: \"Switched from sessions to JWT\" (decision)\nCONFLICT REVIEW PENDING — 2 candidate(s); use mem_judge to record verdicts.",
  "id": 4271,
  "sync_id": "obs-abc123...",
  "judgment_required": true,
  "judgment_status": "pending",
  "judgment_id": "rel-a1b2c3d4e5f6",
  "candidates": [
    {
      "id": 4189,
      "sync_id": "obs-789xyz",
      "title": "We use sessions for auth",
      "type": "decision",
      "topic_key": "decision/auth-model",
      "score": -3.42,
      "judgment_id": "rel-a1b2c3d4e5f6"
    }
  ]
}
```

When candidates is empty: `judgment_required = false`, no `judgment_id`, no `candidates[]`, and the `result` string is byte-identical to today's format (regression guard in REQ-007).

### 4.3 Backwards compatibility

JSON envelope is already the wire format today. New fields are additive — clients that read only `result` see the existing leading line untouched. The "CONFLICT REVIEW PENDING" line is appended ONLY when candidates exist (belt-and-suspenders for non-JSON-parsing agents).

---

## 5. `mem_search` response envelope

### 5.1 Per-result annotation lines

Format (additive lines below the existing standard line):

```
[1] #42 (decision) — Switched to JWT for auth
    **What**: Replaced sessions with JWT...
    2026-04-25T10:30:00Z | project: engram | scope: project
    supersedes: #18 ("We use sessions for auth")
    superseded_by: #66 ("Refactored auth to OAuth2")
    conflict: contested by #91 (pending)
```

### 5.2 New store method `GetRelationsForObservations` (batch)

To avoid N+1, ONE query fetches all relations for all returned IDs in a single round-trip:

```go
func (s *Store) GetRelationsForObservations(syncIDs []string) (map[string]ObservationRelations, error)

type ObservationRelations struct {
    Supersedes   []RelationLink
    SupersededBy []RelationLink
    Conflicts    []RelationLink
}

type RelationLink struct {
    ID         int64
    SyncID     string
    Title      string
    Relation   string
    Status     string
}
```

Query (single SQL with `WHERE source_id IN (...)` OR `target_id IN (...)`):

```sql
SELECT r.id, r.sync_id, r.source_id, r.target_id, r.relation, r.judgment_status,
       o.id, o.title
FROM memory_relations r
JOIN observations o
  ON (o.sync_id = r.target_id AND r.source_id IN (?,?,...))
  OR (o.sync_id = r.source_id AND r.target_id IN (?,?,...))
WHERE r.judgment_status != 'orphaned'
  AND o.deleted_at IS NULL;
```

Then `handleSearch` groups results by saved sync_id and emits annotation lines per result. Orphaned relations are excluded by the WHERE clause (resolves spec risk #1, see §8).

### 5.3 Format

```
supersedes: #<id> ("<title>")          — when relation = supersedes AND row.source_id = result.sync_id
superseded_by: #<id> ("<title>")       — when relation = supersedes AND row.target_id = result.sync_id
conflict: contested by #<id> (pending) — when judgment_status = pending
```

Observations with no relations: byte-identical to today (regression-tested per REQ-002 negative scenario).

---

## 6. `mem_judge` tool (concrete schema)

### 6.1 MCP tool description (the contract — agent reads this)

```
mem_judge — Record a verdict on a pending memory conflict surfaced by mem_save.

When mem_save returns judgment_required=true, call mem_judge once per candidate
(judgment_id is in the candidates[] array). Use this to:
  - record that a new memory SUPERSEDES an old one (the old one is now stale)
  - mark a pair as CONFLICTS_WITH (contradiction; both stand pending Phase 2 resolution)
  - mark a pair as NOT_CONFLICT, RELATED, COMPATIBLE, or SCOPED to clear the pending flag

Ask the user when verdict is ambiguous (see "When to ask" in serverInstructions).
```

### 6.2 Required params

| Param | Type | Required | Description |
|-------|------|----------|-------------|
| `judgment_id` | string | yes | The relation's sync_id from mem_save's `candidates[i].judgment_id` |
| `relation` | string | yes | One of: `related`, `compatible`, `scoped`, `conflicts_with`, `supersedes`, `not_conflict` |
| `reason` | string | no | Human-readable rationale (one sentence) |
| `evidence` | string | no | Free-form; recommended JSON |
| `confidence` | number | no | 0.0..1.0; default 1.0 |

### 6.3 Validation

- Unknown `judgment_id` → `IsError: true`, message `"unknown judgment_id"`.
- Invalid `relation` verb → `IsError: true`, no row mutation.
- `confidence` out of [0,1] → clamped with a warning in `result`.

### 6.4 Idempotency (already-judged row)

Decision: **overwrite** with new verdict + new provenance + bumped `updated_at`. Rationale: an agent re-judging is a deliberate revision, not a duplicate save. Phase 2 multi-actor resolution will introduce per-(actor,pair) uniqueness; today's overwrite is acceptable because the row's provenance fields capture the latest verdict.

Idempotency tests: judging twice with same args → same row, same status, `updated_at` advanced.

### 6.5 Profile

Included in `ProfileAgent` (always available). Not deferred — agents need it eagerly because conflict surfacing is reactive within the same `mem_save` round-trip the agent just made.

---

## 7. Agent behavior heuristics (encoded in `serverInstructions`)

Append to existing serverInstructions string:

```
CONFLICT SURFACING:
  After mem_save, check the response for `judgment_required: true`.
  If true, iterate candidates[] and call mem_judge per pair.

  WHEN TO ASK THE USER:
    - confidence < 0.7 → ASK ("Should this supersede #N?")
    - relation in {supersedes, conflicts_with} AND type in {architecture, policy, decision} → ASK
    - otherwise → resolve silently with mem_judge

  HOW TO ASK (conversationally, never blocking):
    Surface the conflict in your next reply: "I noticed memory #18 ('X')
    might conflict with what we just saved. Want me to mark the new one as
    superseding it, or are they about different scopes?"
```

This is the natural-conversation discipline locked in proposal §11.10. NO CLI/dashboard prompt; the agent owns the user-facing conversation.

---

## 8. Orphaning mechanism (resolves spec risk #1)

Three-layer chain (locked):

| Layer | Mechanism | What it handles |
|-------|-----------|-----------------|
| 1. Schema | `superseded_by_relation_id INTEGER REFERENCES memory_relations(id) ON DELETE SET NULL` | self-FK, only for inter-relation supersede chains |
| 2. Application | In `Store.DeleteObservation(id, hardDelete=true)`, after the existing DELETE on observations: `UPDATE memory_relations SET judgment_status='orphaned', updated_at=datetime('now') WHERE source_id=? OR target_id=?` (in the same tx) | hard-delete of an observation marks any referencing relation row as orphaned |
| 3. On-read | `GetRelationsForObservations` query has `WHERE judgment_status != 'orphaned'` | annotations skip orphaned rows (REQ-010) |

Why NOT cascade-delete: relations are audit history; we want them visible in dashboards Phase 2 even when underlying observations are gone.

Why NOT `ON DELETE SET NULL` on source_id/target_id (foreign-key style): SQLite foreign keys to `observations.sync_id` would require `sync_id` to be UNIQUE on observations, which it is not (sync_ids can repeat across soft-deleted rows in current schema). Using app-layer status update is simpler and avoids touching `observations` constraints.

Soft-delete (`DeleteObservation(id, hardDelete=false)`) does NOT orphan — relations stay `judged`/`pending`. Annotations skip them naturally because the joined observation row is filtered by `o.deleted_at IS NULL`.

---

## 9. Provenance column names (resolves spec risk #2)

Locked: `marked_by_actor`, `marked_by_kind`, `marked_by_model`. Used everywhere — schema, Go struct fields, JSON envelope of relation row in `mem_judge` response. Phase 2 dashboards/audit logs read these names verbatim.

---

## 10. Decay defaults table (resolves spec risk #3)

Constants in `internal/store/store.go`:

```go
const (
    decayDecisionMonths   = 6
    decayPolicyMonths     = 12
    decayPreferenceMonths = 3
)
```

Applied in `AddObservation` AFTER the row is inserted (only for new inserts, not topic_key revisions or duplicates).

| type | review_after offset (Phase 1) | Phase 2 tunable |
|------|-------------------------------|------------------|
| `decision` | +6 months | yes |
| `policy` | +12 months | yes |
| `preference` | +3 months | yes |
| `observation` | NULL | yes |
| (others) | NULL | yes |

`expires_at` is NULL for all types in Phase 1.

---

## 11. Test strategy (strict TDD specifics)

### 11.1 Migration N→N+1 pattern (NEW infrastructure)

File: `internal/store/store_migration_test.go` (NEW).

```go
func TestMigrate_PreMemoryConflictSurfacing_PreservesData(t *testing.T) {
    s := newTestStoreWithLegacySchema(t, []legacyObsRow{...})
    // Assertions: rows present, ids unchanged, review_after NULL, memory_relations exists.
}

func TestMigrate_Idempotent(t *testing.T) {
    // Run migrate() twice; second run is no-op; schema identical.
}

func TestMigrate_DoesNotTouchFTS5OrSyncMutations(t *testing.T) {
    // Seed obs_fts and sync_mutations; run migrate; assert untouched.
}
```

### 11.2 Per-REQ test files

| Test file | Covers |
|-----------|--------|
| `internal/store/relations_test.go` (NEW) | REQ-001, 003, 004, 005, 010 |
| `internal/store/store_migration_test.go` (NEW) | REQ-008 |
| `internal/store/store_test.go` (append) | REQ-006 |
| `internal/mcp/mcp_test.go` (append) | REQ-001, 002, 003, 007 |
| `internal/mcp/mcp_judge_test.go` (NEW) | mem_judge tests |

### 11.3 Integration test (full save → judge → search loop)

`internal/mcp/mcp_conflict_loop_test.go` (NEW): seed obs A, mem_save B with similar title, assert candidates. Call mem_judge with `relation=supersedes`. Call mem_search and assert annotations appear.

### 11.4 Boundary regression

`internal/sync/sync_test.go`: existing tests pass without modification (REQ-009). New test: insert relation row, assert `sync_mutations` count unchanged.

---

## 12. Phase 2 hooks (what this design enables)

| Phase 2 capability | Phase 1 substrate that enables it |
|--------------------|-----------------------------------|
| Cloud sync of relations | TEXT `sync_id` keys; just add `SyncEntityRelation` constant + payload struct + apply funcs + Postgres mirror |
| `engram review` decay command | `review_after` populated; just add `WHERE review_after <= datetime('now')` query + CLI subcommand |
| pgvector hybrid retrieval | `embedding*` columns reserved; add generation pipeline + ANN search |
| Multi-actor disagreement resolution | Schema permits N rows per (source,target); add resolution algorithm reading `marked_by_*` provenance |
| Cloud admin dashboard for conflicts | Provenance columns surface in dashboard tables once Phase 2 sync mirrors them |
| `mem_judge` verdict vocabulary expansion | Validation centralized in one Go func; add new verb without breaking older clients |
| Retraction-on-supersede | `mem_judge` does NOT mutate target today; Phase 2 adds optional `auto_retract` field |

---

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/store/store.go` | Modify | `migrate()`: add observation columns + memory_relations table + indexes. New methods: `FindCandidates`, `SaveRelation`, `JudgeRelation`, `GetRelationsForObservations`. Hook into `DeleteObservation` for orphaning. Apply decay defaults in `AddObservation`. |
| `internal/store/relations.go` | Create | New methods grouped here for clarity (relations CRUD + candidate detection). |
| `internal/store/store_test.go` | Modify | Append decay default tests, orphaning regression. |
| `internal/store/store_migration_test.go` | Create | `newTestStoreWithLegacySchema` helper + 3 migration tests (REQ-008). |
| `internal/store/relations_test.go` | Create | All REQ-001/003/004/005/010 unit tests. |
| `internal/mcp/mcp.go` | Modify | `serverInstructions` adds CONFLICT SURFACING block. `handleSave` calls `FindCandidates` post-save and writes `SaveRelation` per candidate; envelope adds new fields. `handleSearch` calls `GetRelationsForObservations` and emits annotation lines. New `handleJudge` func + `mem_judge` tool registered in `ProfileAgent`. |
| `internal/mcp/mcp_test.go` | Modify | Envelope shape tests; backwards-compat regression. |
| `internal/mcp/mcp_judge_test.go` | Create | mem_judge happy/error/idempotency. |
| `internal/mcp/mcp_conflict_loop_test.go` | Create | save → judge → search integration. |

NOT TOUCHED: `internal/cloud/*`, `internal/sync/sync.go`, `internal/dashboard/*`, `cmd/engram/*`. Boundary discipline matches `engram-architecture-guardrails`.

---

## File mirror

- File: `openspec/changes/memory-conflict-surfacing/design.md`
- Engram: topic_key `sdd/memory-conflict-surfacing/design`
