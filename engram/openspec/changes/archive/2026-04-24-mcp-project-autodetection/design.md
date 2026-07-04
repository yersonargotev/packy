# Design: MCP Project Auto-Detection

## Context

MCP handlers today read `project` from LLM arguments. Different agents produce different buckets for the same repo. We already have `project.DetectProject(dir) string` used by the CLI, but it only exposes 3 cases and no structured error. This change makes MCP authoritative (auto-detect on writes, validate override on reads) while keeping CLI callers untouched.

## Technical Approach

Introduce `DetectProjectFull(dir) (DetectionResult, error)` returning a rich value object. Reimplement `DetectProject(dir) string` on top of it as a thin fallback wrapper so CLI callers keep their current semantics. Add `store.ProjectExists` for read-side override validation. In `internal/mcp/mcp.go`, funnel every handler through two `*MCP`-style resolver methods (package-level funcs taking `*store.Store`+`MCPConfig`) and wrap every successful result through `respondWithProject` which injects `{project, project_source, project_path}` into the structured content. Remove `project` from 6 write schemas; keep it on 5 read schemas. Register `mem_current_project` in default set and `ProfileAgent`.

## Architecture Decisions

| # | Topic | Choice | Alternatives | Rationale |
|---|-------|--------|--------------|-----------|
| 1 | Result shape | Single `DetectionResult` struct + separate `error` return (Go idiom). `AvailableProjects` only populated when `err == ErrAmbiguousProject`. | Result-only (`Error` embedded) or separate Result/Error types | Go convention: errors as return value, struct for success data. Lets callers use `errors.Is(err, ErrAmbiguousProject)`. |
| 2 | Child scan | Single-threaded with `time.Now()` deadline check per entry, `os.ReadDir` + slice filter + early break on 2nd hit. Skip list: `.git, node_modules, vendor, .venv, __pycache__, target, dist, build, .idea, .vscode` + any `strings.HasPrefix(".")`. Cap 20 entries (after skip filter), 200 ms. | Goroutine + `context.WithTimeout` | Simpler, no goroutine lifecycle, depth=1 is cheap; deadline sufficient. |
| 3 | Resolver placement | Package-level funcs `resolveWriteProject(ctx, cfg) (DetectionResult, error)` and `resolveReadProject(ctx, cfg, override string) (DetectionResult, error)` called by handlers. No `*MCP` struct exists today. | Add `*MCP` struct | Preserves current handler closure style. Handlers already receive `s, cfg` via closures. |
| 4 | Response envelope | Helper `respondWithProject(res DetectionResult, text string, extra map[string]any) *mcp.CallToolResult` that adds structured content via existing `NewToolResultText` + meta map. Every handler calls it exactly once on success path. | Per-tool manual JSON | Single-seam guarantees every tool response is enveloped; test via handler tests only. |
| 5 | Error envelope | `mcp.NewToolResultError(msg)` with structured fields `error_code` (`ambiguous_project` / `unknown_project`), `available_projects` (array), `hint` (string). Implemented via `errorWithMeta(code, msg, meta)` helper returning `*mcp.CallToolResult` with `IsError: true`. | HTTP-style status | MCP already uses `IsError`; code+hint is enough for LLM retry. |
| 6 | Write schema removal | Delete `mcp.WithString("project", ...)` lines from `mem_save`, `mem_save_prompt`, `mem_session_start`, `mem_session_end`, `mem_capture_passive`, `mem_update`. Handlers ignore the field if still sent (tolerant parse). | Hard reject | Tolerant parse avoids breaking JSON-strict clients during rollout. |
| 7 | `ProjectExists` | Single `SELECT EXISTS(SELECT 1 FROM (...UNION ALL...) LIMIT 1)` across `observations`, `sessions`, `prompts`, `enrollment`. Inline (no prepared statement) — called once per read tool. | Four separate queries | One round-trip, stable cost. |
| 8 | `mem_current_project` | Register in default set AND `ProfileAgent`. On ambiguity returns **success** with `warning` + `available_projects` + empty `project` — NOT an error (discovery must work even when writes would fail). | Error on ambiguity | LLM uses this tool to resolve ambiguity; erroring defeats the purpose. |
| 9 | CLI compat | `DetectProject(dir) string` = `res, _ := DetectProjectFull(dir); return res.Project`. On `ErrAmbiguousProject`, `res.Project` is the basename fallback (we still populate it). | Return "unknown" on ambiguity | CLI callers (main.go:656, 736, 1151, 1696) never expect a failure; current behavior is "always a non-empty name". |
| 10 | Test strategy | Unit: `DetectProjectFull` via `t.TempDir` + `initGit` covering 5 cases. Store: `ProjectExists` on empty/partial/populated. MCP: `t.Chdir` helper for handlers; assert envelope fields present. Regression: existing `TestDetectProject_*` must still pass unchanged. | Mocks | Real temp git repos match existing pattern and are fast. |

## Data Flow

    LLM tool call          ┌─ handler ─┐          ┌─ store ─┐
      │                    │            │          │         │
      │  (write)           │ resolveWrite         ops (Add*, Update*)
      ├───────────────────►│   ├── DetectProjectFull(os.Getwd)
      │                    │   └── err? → errorWithMeta("ambiguous", available)
      │                    │                       │
      │  (read, override)  │ resolveRead          Search / Context / ...
      ├───────────────────►│   ├── override? → store.ProjectExists
      │                    │   │        └── false → errorWithMeta("unknown", available)
      │                    │   └── empty → DetectProjectFull
      │                    │                       │
      │  ◄──── respondWithProject(res, text, extra) ◄─┘
      │         {..., project, project_source, project_path, warning?}

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/project/detect.go` | Modify | Add `DetectionResult`, `Source` enum, `ErrAmbiguousProject`, `DetectProjectFull`, `scanChildren`; rewrite `DetectProject` as wrapper. |
| `internal/project/detect_test.go` | Modify | Add 5-case tests; keep existing tests (regression). |
| `internal/store/store.go` | Modify | Add `ProjectExists(name) (bool, error)`. |
| `internal/store/store_test.go` | Modify | Tests for empty/partial/populated DB. |
| `internal/mcp/mcp.go` | Modify | Remove `project` from 6 schemas; add `resolveWriteProject`, `resolveReadProject`, `respondWithProject`, `errorWithMeta`; refactor 11 handlers; register `mem_current_project`. |
| `internal/mcp/mcp_test.go` | Modify | `t.Chdir` helper; handler tests per tool. |
| `CHANGELOG.md` | Modify | Document breaking write-schema change. |

## Testing Strategy

| Layer | What | How |
|-------|------|-----|
| Unit | `DetectProjectFull` 5 cases, `ProjectExists` 3 states | Temp dirs + real git, real SQLite |
| Integration | MCP handlers resolve project from cwd | `t.Chdir` to a temp git repo, call handler via existing harness |
| Regression | CLI `detectProject` behavior, existing MCP handler tests | Keep existing tests green |

## Rollout Plan

Batch 1: `DetectionResult` + `DetectProjectFull` + wrapper + unit tests.
Batch 2: `store.ProjectExists` + tests.
Batch 3: MCP resolver helpers + envelope + error helper + tests (no handler changes yet).
Batch 4: Refactor write handlers (remove schema, use resolveWrite).
Batch 5: Refactor read handlers (use resolveRead with override validation).
Batch 6: `mem_current_project` tool + profile registration.
Batch 7: CHANGELOG + docs.

## Open Questions

- Does `mem_current_project` need a `cwd` input arg for debugging, or is `os.Getwd` enough? (Default: no arg — matches MCP auto-detect contract.)
- Should the envelope meta live in `CallToolResult.Meta` or as structured content fields? (Default: structured content — Meta is client-internal per spec.)
