# Tasks: memory-conflict-surfacing

Phase: SDD tasks
Change: memory-conflict-surfacing
Engram topic_key: `sdd/memory-conflict-surfacing/tasks`
Artifact store: hybrid

---

## Phase A — Migration test infrastructure (foundation)

All downstream phases depend on this. Must ship first.

- [x] A.1 — Extract `legacyDDLPreMemoryConflictSurfacing` constant
- [x] A.2 — Create `newTestStoreWithLegacySchema(t, fixtureRows)` helper
- [x] A.3 — Write `TestMigrate_PreMemoryConflictSurfacing_PreservesData`
- [x] A.4 — Write `TestMigrate_Idempotent`
- [x] A.5 — Write `TestMigrate_DoesNotTouchFTS5OrSyncMutations`

---

## Phase B — Schema additions

Sequential after A. Blocks Phase C.

- [x] B.1 — Extend `migrate()`: add 5 new observation columns
- [x] B.2 — Add `memory_relations` table + indexes to `migrate()`
- [x] B.3 — Run full existing store test suite to confirm no regressions

---

## Phase C — Store layer relations API

Sequential after B. Blocks Phase D.

- [x] C.1 — Write `TestFindCandidates_HappyPath`
- [x] C.2 — Write `TestFindCandidates_ExcludesSelf`
- [x] C.3 — Write `TestFindCandidates_BM25Floor`
- [x] C.4 — Write `TestFindCandidates_UnrelatedTitle`
- [x] C.5 — Write `TestSaveRelation` + `TestGetRelationsForObservations_*`
- [x] C.6 — Write `TestJudgeRelation_*`
- [x] C.7 — Write `TestMultiActor_*` + `TestSyncIDUnique`
- [x] C.8 — Write `TestProvenance_*`
- [x] C.9 — Write `TestOrphaning_*`
- [x] C.10 — Create `internal/store/relations.go` implementation
- [x] C.11 — Extend `DeleteObservation` for orphaning
- [x] C.12 — `go test ./internal/store/...` all pass

---

## Phase D — MCP layer enrichment

Sequential after C. Blocks Phase G integration.

- [x] D.1 — `TestHandleSave_CandidatesReturned`
- [x] D.2 — `TestHandleSave_NoCandidates_ResultUnchanged`
- [x] D.3 — `TestHandleSave_TopicKeyRevision_ReturnsCandidates`
- [x] D.4 — `TestHandleSearch_*` annotation tests
- [x] D.5 — Create `internal/mcp/mcp_judge_test.go` with 5 tests
- [x] D.6 — Modify `handleSave` in `internal/mcp/mcp.go`
- [x] D.7 — Modify `handleSearch` in `internal/mcp/mcp.go`
- [x] D.8 — Create `handleJudge` function and register `mem_judge` tool
- [x] D.9 — `go test ./internal/mcp/...` all pass

---

## Phase E — Agent behavior instructions

Parallel with F and G (after C is done). No code dependencies.

- [x] E.1 — Append CONFLICT SURFACING block to `serverInstructions`

---

## Phase F — Decay defaults wiring

Parallel with E and G (after B). Depends only on schema columns from B.

- [x] F.1 — `TestAddObservation_DecayDefaults` table-driven test
- [x] F.2 — `TestAddObservation_DecayNotAppliedToExistingRows`
- [x] F.3 — Add decay constants and extend `AddObservation`

---

## Phase G — Integration tests

Parallel with E, F (after D is done).

- [x] G.1 — Create `internal/mcp/mcp_conflict_loop_test.go` with save→judge→search test
- [x] G.2 — Write `TestConflictLoop_MultiActor`
- [x] G.3 — Write `TestConflictLoop_Orphaning`
- [x] G.4 — Write sync regression test
- [x] G.5 — Make G.1–G.4 pass

---

## Phase H — Documentation

Parallel. Can run after D.8. No code changes.

- [x] H.1 — Update `docs/AGENT-SETUP.md` with `mem_judge` documentation
- [x] H.2 — Update `docs/PLUGINS.md` with mem_judge and response schema

---

## Task Summary

| Phase | Tasks | Status |
|-------|-------|--------|
| A — Migration infra | 5 | COMPLETE |
| B — Schema additions | 3 | COMPLETE |
| C — Store API | 12 | COMPLETE |
| D — MCP layer | 9 | COMPLETE |
| E — Agent instructions | 1 | COMPLETE |
| F — Decay wiring | 3 | COMPLETE |
| G — Integration tests | 5 | COMPLETE |
| H — Documentation | 2 | COMPLETE |
| **Total** | **40** | **COMPLETE** |

All 40 tasks marked [x] and ready for archive.
