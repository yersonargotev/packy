# Tasks: memory-conflict-surfacing

Phase: SDD tasks
Change: memory-conflict-surfacing
Engram topic_key: `sdd/memory-conflict-surfacing/tasks`
Artifact store: hybrid

---

## Phase A — Migration test infrastructure (foundation)

> All downstream phases depend on this. Must ship first.

- [x] A.1 **[RED]** Extract `legacyDDLPreMemoryConflictSurfacing` constant from the CURRENT `observations` + `obs_fts` + `sync_mutations` DDL in `internal/store/store.go` BEFORE any schema changes are applied. Add as a package-level `const` in a new file `internal/store/store_legacy_ddl_test.go`.
  - REQ-008 | Design §2.1
  - Acceptance: constant compiles; string contains `CREATE TABLE observations` but does NOT contain `review_after` or `memory_relations`

- [x] A.2 **[RED]** Create `newTestStoreWithLegacySchema(t, fixtureRows)` helper in `internal/store/store_migration_test.go`. Opens temp SQLite with legacy DDL, inserts fixture rows, calls `New(cfg)` so `migrate()` runs against the legacy DB.
  - REQ-008 | Design §2.1
  - Acceptance: helper compiles and returns a `*Store`

- [x] A.3 **[RED]** Write `TestMigrate_PreMemoryConflictSurfacing_PreservesData` in `internal/store/store_migration_test.go`: insert 5 fixture rows via legacy schema, run migrate, assert all 5 rows intact (id, sync_id, content, created_at unchanged), `review_after`/`expires_at` NULL, `memory_relations` exists. Test FAILS (new columns and table not yet added).
  - REQ-008 scenarios | Design §11.1
  - Acceptance: test compiles but fails (red)

- [x] A.4 **[RED]** Write `TestMigrate_Idempotent`: call `migrate()` twice on same DB, assert no error and schema identical.
  - REQ-008 edge case | Design §11.1
  - Acceptance: test compiles but fails (red)

- [x] A.5 **[RED]** Write `TestMigrate_DoesNotTouchFTS5OrSyncMutations`: assert `obs_fts` and `sync_mutations` are unchanged after migration.
  - REQ-008 negative | Design §11.1
  - Acceptance: test compiles but fails (red)

---

## Phase B — Schema additions (makes Phase A tests green)

> Sequential after A. Blocks Phase C.

- [x] B.1 **[GREEN]** Extend `migrate()` in `internal/store/store.go`: add `addColumnIfNotExists` calls for 5 new `observations` columns (`review_after`, `expires_at`, `embedding`, `embedding_model`, `embedding_created_at`).
  - REQ-008 | Design §2
  - Acceptance: `TestMigrate_PreMemoryConflictSurfacing_PreservesData` passes

- [x] B.2 **[GREEN]** Add `CREATE TABLE IF NOT EXISTS memory_relations` + 3 `CREATE INDEX IF NOT EXISTS` statements to `migrate()` in `internal/store/store.go`, after the column adds.
  - REQ-008 | Design §1.1, §2
  - Acceptance: A.3, A.4, A.5 all pass; `go test ./internal/store/... -run TestMigrate` green

- [x] B.3 **[VERIFY]** Run full existing store test suite to confirm no regressions: `go test ./internal/store/...`
  - REQ-007, REQ-008 | Design §11
  - Acceptance: zero failures

---

## Phase C — Store layer relations API

> Sequential after B. Blocks Phase D.

- [x] C.1 **[RED]** Create `internal/store/relations_test.go`. Write `TestFindCandidates_HappyPath`: insert two similar observations, call `FindCandidates`, assert at least 1 candidate with `id`, `sync_id`, `title`, `type`, `score`, `judgment_id` (prefixed `rel-`). Test fails (method not yet exist).
  - REQ-001 | Design §3

- [x] C.2 **[RED]** Write `TestFindCandidates_ExcludesSelf`: just-saved obs not returned in its own candidates.
  - REQ-001 edge case | Design §3.4

- [x] C.3 **[RED]** Write `TestFindCandidates_BM25Floor`: two candidates, one borderline; raise `CandidateBM25Floor` to exclude borderline; assert only high-score candidate returned.
  - REQ-001 edge case | Design §3.3

- [x] C.4 **[RED]** Write `TestFindCandidates_UnrelatedTitle`: dissimilar title → empty candidates slice.
  - REQ-001 negative | Design §3.4

- [x] C.5 **[RED]** Write `TestSaveRelation` + `TestGetRelationsForObservations_HappyPath` + `TestGetRelationsForObservations_SkipsOrphaned`.
  - REQ-002, REQ-010 | Design §5.2, §8

- [x] C.6 **[RED]** Write `TestJudgeRelation_HappyPath`, `TestJudgeRelation_OptionalFieldsNullWhenOmitted`, `TestJudgeRelation_UnknownID`, `TestJudgeRelation_InvalidVerb`.
  - REQ-003 | Design §6.3

- [x] C.7 **[RED]** Write `TestMultiActor_TwoRowsForSamePair` and `TestSyncIDUnique`.
  - REQ-004 | Design §1.1

- [x] C.8 **[RED]** Write `TestProvenance_FullRowPersisted` and `TestProvenance_HumanActorNullModel`.
  - REQ-005 | Design §9

- [x] C.9 **[RED]** Write `TestOrphaning_DeleteSourceOrphansRelation`, `TestOrphaning_OrphanedSkippedInAnnotations`, `TestOrphaning_OrphanedDoesNotBlockCandidate`.
  - REQ-010 | Design §8

- [x] C.10 **[GREEN]** Create `internal/store/relations.go`. Implement `CandidateOptions`, `Candidate` structs, `FindCandidates`, `SaveRelation`, `GetRelation`, `JudgeRelation`, `GetRelationsForObservations`. Relation vocab validation in Go. `sync_id` via `newSyncID("rel")`.
  - REQ-001, 003, 004, 005, 010 | Design §3, §5.2, §6

- [x] C.11 **[GREEN]** Extend `DeleteObservation` in `internal/store/store.go` (hard-delete branch): after existing DELETE, run `UPDATE memory_relations SET judgment_status='orphaned' WHERE source_id=? OR target_id=?` in same transaction.
  - REQ-010 | Design §8

- [x] C.12 **[VERIFY]** `go test ./internal/store/...` — all C tests green, no regressions.

---

## Phase D — MCP layer enrichment

> Sequential after C. Blocks Phase G integration.

- [x] D.1 **[RED]** Append to `internal/mcp/mcp_test.go`: `TestHandleSave_CandidatesReturned` — assert envelope has `judgment_required=true`, `candidates` non-empty, `result` contains `"CONFLICT REVIEW PENDING"`.
  - REQ-001 | Design §4

- [x] D.2 **[RED]** Append: `TestHandleSave_NoCandidates_ResultUnchanged` — assert `result` string starts with `Memory saved: "`, no `judgment_id`, no `candidates`.
  - REQ-007 | Design §4

- [x] D.3 **[RED]** Append: `TestHandleSave_TopicKeyRevision_ReturnsCandidates`.
  - REQ-001 edge case | Design §4

- [x] D.4 **[RED]** Append: `TestHandleSearch_SupersededAnnotation`, `TestHandleSearch_PendingAsContested`, `TestHandleSearch_NoRelationsUnchanged`.
  - REQ-002 | Design §5

- [x] D.5 **[RED]** Create `internal/mcp/mcp_judge_test.go`. Write `TestHandleJudge_HappyPath`, `TestHandleJudge_OptionalFieldsStayNull`, `TestHandleJudge_UnknownID_IsError`, `TestHandleJudge_InvalidVerb_IsError`, `TestHandleJudge_Idempotent_Overwrite`.
  - REQ-003 | Design §6

- [x] D.6 **[GREEN]** Modify `handleSave` in `internal/mcp/mcp.go`: post-transaction call to `FindCandidates`; build enriched JSON envelope with `id`, `sync_id`, `judgment_required`, `judgment_id`, `judgment_status`, `candidates[]`; append `"CONFLICT REVIEW PENDING"` line to result only when candidates exist. Errors from `FindCandidates` logged + swallowed.
  - REQ-001, REQ-007 | Design §4

- [x] D.7 **[GREEN]** Modify `handleSearch` in `internal/mcp/mcp.go`: batch call to `GetRelationsForObservations(syncIDs)`; append annotation lines (`supersedes:`, `superseded_by:`, `conflict:`) below each result entry; skip orphaned.
  - REQ-002, REQ-010 | Design §5

- [x] D.8 **[GREEN]** Create `handleJudge` function in `internal/mcp/mcp.go` and register `mem_judge` tool in `ProfileAgent`. Implement param validation, `JudgeRelation` call, provenance fields populated from session context.
  - REQ-003, REQ-005 | Design §6, §6.5

- [x] D.9 **[VERIFY]** `go test ./internal/mcp/...` — all D tests green, no regressions.

---

## Phase E — Agent behavior instructions

> Parallel with F and G (after C is done). No code dependencies.

- [x] E.1 **[IMPLEMENT]** Append CONFLICT SURFACING block to `serverInstructions` in `internal/mcp/mcp.go`: heuristic for when to ask user (confidence < 0.7 OR relation in {supersedes, conflicts_with} AND type in {architecture, policy, decision}), conversational pattern.
  - Design §7
  - Acceptance: `serverInstructions` string contains "CONFLICT SURFACING"; `go build ./...` passes

---

## Phase F — Decay defaults wiring

> Parallel with E and G (after B). Depends only on schema columns from B.

- [ ] F.1 **[RED]** Append to `internal/store/store_test.go`: `TestAddObservation_DecayDefaults` table-driven test for `decision` (+6mo), `policy` (+12mo), `preference` (+3mo), `observation` (NULL). Assert `review_after` within ±1 second of expected; assert `expires_at` NULL for all.
  - REQ-006 | Design §10

- [ ] F.2 **[RED]** Append: `TestAddObservation_DecayNotAppliedToExistingRows` — migrate legacy DB, assert existing rows have `review_after=NULL` after migration.
  - REQ-006 negative | Design §10

- [ ] F.3 **[GREEN]** Add decay constants in `internal/store/store.go` (`decayDecisionMonths=6`, `decayPolicyMonths=12`, `decayPreferenceMonths=3`). Extend `AddObservation`: after successful insert, compute and `UPDATE observations SET review_after=? WHERE id=?` for new inserts only; `expires_at` left NULL.
  - REQ-006 | Design §10
  - Acceptance: F.1 and F.2 pass; `go test ./internal/store/...` green

---

## Phase G — Integration tests

> Parallel with E, F (after D is done).

- [ ] G.1 **[RED]** Create `internal/mcp/mcp_conflict_loop_test.go`. Write `TestConflictLoop_SaveJudgeSearch`: save → candidates returned → `mem_judge` → search shows supersedes annotation.
  - REQ-001+002+003 integration | Design §11.2

- [ ] G.2 **[RED]** Write `TestConflictLoop_MultiActor`: two judge calls for same pair → both rows persist; search shows both annotations.
  - REQ-004 integration | Design §11.2

- [ ] G.3 **[RED]** Write `TestConflictLoop_Orphaning`: save two obs, judge, hard-delete source → relation orphaned → search annotation absent.
  - REQ-010 integration | Design §8

- [ ] G.4 **[RED]** Write sync regression test (append to `internal/sync/sync_test.go` or new file): insert relation row, assert `sync_mutations` count unchanged; assert observation sync payload does NOT contain `review_after`/`embedding*` fields.
  - REQ-009 | Design §11.3

- [ ] G.5 **[GREEN]** Make G.1–G.4 pass (wiring issues only — implementation already in place from C+D). No new logic expected.
  - Acceptance: `go test ./internal/mcp/... ./internal/sync/...` green

---

## Phase H — Documentation

> Parallel. Can run after D.8. No code changes.

- [ ] H.1 **[DOCS]** Update `docs/AGENT-SETUP.md`: document `mem_judge` tool (params, when it fires, example conversational flow). Note Phase 1 deferrals (cloud sync of relations, decay activation, pgvector).
  - REQ-007 | Design §6.1

- [ ] H.2 **[DOCS]** Update `docs/PLUGINS.md` (or equivalent MCP tools reference): add `mem_judge` entry with param table (`judgment_id`, `relation`, `reason`, `evidence`, `confidence`) and response shape. Document new `mem_save` response fields and `mem_search` annotation format.
  - REQ-007 | Design §4, §5, §6

---

## Task Counts per Phase

| Phase | Tasks | Parallel? |
|-------|-------|-----------|
| A — Migration infra | 5 | No — must run first |
| B — Schema additions | 3 | No — sequential after A |
| C — Store API | 12 | No — sequential after B |
| D — MCP layer | 9 | No — sequential after C |
| E — Agent instructions | 1 | Yes — parallel with F, G after D |
| F — Decay wiring | 3 | Yes — parallel with E, G after B |
| G — Integration tests | 5 | Yes — parallel with E, F after D |
| H — Documentation | 2 | Yes — parallel after D |
| **Total** | **40** | |

## Dependency Order

```
A (infra) → B (schema) → C (store API) → D (MCP)
                                              ↓
                              E (instructions), F (decay), G (integration), H (docs)
                              [E, F, G, H all run in parallel after D]
```

F can start after B (schema columns exist) — does not need C or D. E can start as soon as D.8 is done.
