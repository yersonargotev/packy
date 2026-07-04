# Tasks: MCP Project Auto-Detection

> Strict TDD order per batch: RED (failing test) → GREEN (implementation) → REFACTOR (cleanup).
> All batches are sequential except where marked parallel.

---

## TDD Cycle Evidence

| Batch | Task | RED | GREEN | REFACTOR |
|-------|------|-----|-------|----------|
| 1 | 1.1 – 1.10 | ✅ Written | ✅ Passed | ✅ gofmt clean |
| 2 | 2.1 – 2.7 | ✅ Written | ✅ Passed | ✅ param binding verified |
| 3 | 3.1 – 3.8 | ✅ Written | ✅ Passed | ✅ helpers unexported |
| 4 | 4.1 – 4.10 | ✅ Written | ✅ Passed | ✅ tolerant parse verified |
| 5 | 5.1 – 5.7 | ✅ Written | ✅ Passed | ✅ admin tools unchanged |
| 6 | 6.1 – 6.5 | ✅ Written | ✅ Passed | ✅ ProfileAgent updated |
| 7 | 7.1 – 7.5 | N/A docs | ✅ All green | ✅ vet/fmt clean |

---

## Batch 1 — DetectionResult + DetectProjectFull + backward-compat wrapper
**Files**: `internal/project/detect.go`, `internal/project/detect_test.go`
**Satisfies**: REQ-300, REQ-301, REQ-302, REQ-303, REQ-304, REQ-305, REQ-306, REQ-307

- [x] 1.1 RED: `TestDetectProjectFull_Case1_Remote` — assert `Source=="git_remote"`, `Project==reponame`, `Error==nil` for a `t.TempDir` git repo with remote origin URL
- [x] 1.2 RED: `TestDetectProjectFull_Case1_NoRemote` — assert fallthrough to `git_root` source when no origin remote exists
- [x] 1.3 RED: `TestDetectProjectFull_Case2_Subdir` — assert `Source=="git_root"`, `Path==ancestor_root`, from a subdirectory two levels deep inside a git repo
- [x] 1.4 RED: `TestDetectProjectFull_Case3_SingleChild` — assert `Source=="git_child"`, `Warning!=""`, `Error==nil` for a temp dir with exactly one git-repo subdirectory
- [x] 1.5 RED: `TestDetectProjectFull_Case4_MultiChild` — assert `Error==ErrAmbiguousProject`, `len(AvailableProjects)==2`, `Project==""` for two git-repo children
- [x] 1.6 RED: `TestDetectProjectFull_Case5_Basename` — assert `Source=="dir_basename"`, `Project==filepath.Base(dir)`, `Error==nil` for a plain non-git dir
- [x] 1.7 RED: `TestChildScan_ShortCircuit`, `TestChildScan_SkipNoise`, `TestChildScan_SkipHidden` — constraint and edge-case seams
- [x] 1.8 RED: `TestDetectProject_MatchesFull`, `TestDetectProject_AmbiguousEmpty` — backward-compat wrapper assertions
- [x] 1.9 GREEN: Added `DetectionResult` struct, `ErrAmbiguousProject`, `Source` constants, `scanChildren`, `DetectProjectFull` to detect.go; rewrote `DetectProject` as wrapper
- [x] 1.10 REFACTOR: All existing `TestDetectProject_*` tests still pass; `scanChildren` cleanly extracted

---

## Batch 2 — store.ProjectExists
**Files**: `internal/store/store.go`, `internal/store/store_test.go`
**Satisfies**: REQ-315

- [x] 2.1 RED: `TestProjectExists_EmptyStore` — assert false, nil on fresh store
- [x] 2.2 RED: `TestProjectExists_Known` — insert observation, assert true
- [x] 2.3 RED: `TestProjectExists_KnownViaSession` — insert session only, assert true
- [x] 2.4 RED: `TestProjectExists_KnownViaPrompt` — insert prompt only, assert true
- [x] 2.5 RED: `TestProjectExists_Unknown` — assert false on populated store with different project
- [x] 2.6 GREEN: Added `ProjectExists(name string) (bool, error)` using UNION ALL query across observations, sessions, prompts
- [x] 2.7 REFACTOR: Verified parameter binding; no string interpolation in query

---

## Batch 3 — MCP resolver helpers + envelope + error helper
**Files**: `internal/mcp/mcp.go`, `internal/mcp/mcp_test.go`
**Satisfies**: REQ-309 (error shape), REQ-314 (envelope shape)

- [x] 3.1 RED: `TestResolveWriteProject_AutoDetects` — t.Chdir to temp git repo, assert Source!=""
- [x] 3.2 RED: `TestResolveWriteProject_AmbiguousError` — assert errors.Is(err, ErrAmbiguousProject)
- [x] 3.3 RED: `TestResolveReadProject_WithOverride` — known project override succeeds
- [x] 3.4 RED: `TestResolveReadProject_UnknownOverride` — unknown override returns *unknownProjectError
- [x] 3.5 RED: `TestRespondWithProject_MergesEnvelope` — assert project, project_source, project_path in result
- [x] 3.6 RED: `TestErrorWithMeta_WrapsResponse` — assert IsError==true, error_code, available_projects, hint
- [x] 3.7 GREEN: Implemented resolveWriteProject, resolveReadProject, respondWithProject, errorWithMeta
- [x] 3.8 REFACTOR: Added initTestGitRepo helper; all helpers unexported

---

## Batch 4 — Refactor write handlers
**Files**: `internal/mcp/mcp.go`, `internal/mcp/mcp_test.go`
**Satisfies**: REQ-308, REQ-309
**Depends on**: Batches 1, 2, 3

- [x] 4.1 RED: `TestWriteSchema_NoProjectField` — all 6 write schemas lack project property
- [x] 4.2 RED: `TestMemSave_AutoDetectsProject` — write lands under detected project
- [x] 4.3 RED: `TestMemSave_IgnoresLLMProject` — LLM-supplied project silently discarded
- [x] 4.4 RED: `TestMemSave_AmbiguousEnvelope` — error_code=="ambiguous_project", no write
- [x] 4.5 RED: `TestMemSave_SuccessEnvelope` — project, project_source, project_path in response
- [x] 4.6 RED: Updated session/capture/prompt handler tests with t.Chdir
- [x] 4.7 RED: Updated session_start and session_end tests
- [x] 4.8 RED: Updated capture_passive tests
- [x] 4.9 GREEN: Removed project from 6 write schemas; routed handlers through resolveWriteProject; wrapped with respondWithProject / errorWithMeta
- [x] 4.10 REFACTOR: Confirmed tolerant parse on all 6 write handlers (project silently ignored if sent)

---

## Batch 5 — Refactor read handlers
**Files**: `internal/mcp/mcp.go`, `internal/mcp/mcp_test.go`
**Satisfies**: REQ-310, REQ-311, REQ-314
**Depends on**: Batches 2, 3

- [x] 5.1 RED: `TestMemSearch_NoProjectAutoDetects` — no project arg falls back to auto-detect
- [x] 5.2 RED: `TestMemSearch_ExplicitKnownProject` — valid override uses ProjectExists path
- [x] 5.3 RED: `TestMemSearch_UnknownProjectError` — unknown override returns structured error with available_projects
- [x] 5.4 RED: `TestAllTools_ReadResponseEnvelope` — project envelope in successful read responses
- [x] 5.5 RED: Envelope assertions across read tools
- [x] 5.6 GREEN: Routed handleSearch and handleContext through resolveReadProject; wrapped with respondWithProject/errorWithMeta
- [x] 5.7 REFACTOR: Confirmed admin tools (mem_delete, mem_merge_projects) unchanged and still require project

---

## Batch 6 — mem_current_project tool
**Files**: `internal/mcp/mcp.go`, `internal/mcp/mcp_test.go`
**Satisfies**: REQ-313
**Depends on**: Batches 1, 3

- [x] 6.1 RED: `TestMemCurrentProject_NormalResult` — full metadata in response
- [x] 6.2 RED: `TestMemCurrentProject_AmbiguousNoError` — IsError==false, project=="", available_projects non-empty
- [x] 6.3 RED: `TestMemCurrentProject_WarningCase3` — warning!="" and project_source=="git_child"
- [x] 6.4 GREEN: Registered mem_current_project in mcp.go; added to ProfileAgent and default set; always returns success
- [x] 6.5 REFACTOR: Verified ProfileAgent map updated; tool count updated to 16; full test suite green

---

## Batch 7 — Docs + final verify
**Files**: `CHANGELOG.md`, `DOCS.md`, `docs/AGENT-SETUP.md`
**Satisfies**: REQ-308 schema break documented, all success criteria
**Depends on**: Batches 1–6

- [x] 7.1 CHANGELOG.md: added [Unreleased] entry for write-tool schema breaking change
- [x] 7.2 DOCS.md: added "MCP Project Resolution" section (5-case algorithm, envelope fields, mem_current_project usage)
- [x] 7.3 docs/AGENT-SETUP.md: added note about not passing project on writes; documented mem_current_project as recommended first call
- [x] 7.4 go test ./internal/project/... ./internal/store/... ./internal/mcp/... — all green
- [x] 7.5 go test -race ./... && go vet ./... && gofmt -l . — zero findings

---

## Parallelism Notes

| Batches | Relationship |
|---------|-------------|
| 1 and 2 | Can run in parallel — no shared code |
| 3 | Depends on 1 (uses `DetectionResult`) and 2 (`store.ProjectExists`) |
| 4 and 5 | Both depend on 3; can run in parallel after 3 |
| 6 | Depends on 1 and 3; can overlap with 4/5 |
| 7 | Must run last |

---

## Spec Requirement Coverage

| Requirement | Batch(es) | Task(s) |
|------------|----------|---------|
| REQ-300 DetectProjectFull contract | 1 | 1.1–1.8 |
| REQ-301 Case 1 git_remote | 1 | 1.1, 1.2 |
| REQ-302 Case 2 git_root | 1 | 1.3 |
| REQ-303 Case 3 git_child | 1 | 1.4 |
| REQ-304 Case 4 ambiguous | 1 | 1.5 |
| REQ-305 Case 5 dir_basename | 1 | 1.6 |
| REQ-306 Child scan constraints | 1 | 1.7 |
| REQ-307 DetectProject wrapper | 1 | 1.8, 1.9 |
| REQ-308 Write schema removes project | 4 | 4.1, 4.9 |
| REQ-309 Write fail fast on ambiguous | 4 | 4.4, 4.5 |
| REQ-310 Read tools optional project | 5 | 5.1, 5.6 |
| REQ-311 Read tools validate override | 5 | 5.2, 5.3, 5.6 |
| REQ-312 Admin tools keep required project | 5 | 5.7 |
| REQ-313 mem_current_project tool | 6 | 6.1–6.4 |
| REQ-314 Standardized response envelope | 3, 4, 5 | 3.5, 4.5, 5.5 |
| REQ-315 store.ProjectExists | 2 | 2.1–2.6 |
