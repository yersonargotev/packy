# Proposal: MCP Project Auto-Detection

## Intent

MCP tool handlers in `internal/mcp/mcp.go` currently take the `project` name from `req.GetArguments()["project"]`, letting the LLM decide where memory is stored. Across agents (Claude Code, OpenCode, Gemini, Codex, VSCode) this produces inconsistent buckets for the same repo (`engram` vs `engram-cloud`), and agents without plugins (Gemini/Codex/VSCode) have no safety net. We already have `internal/project.DetectProject` used by the CLI — we need to make MCP authoritative by auto-detecting from `cwd` on every write, validating overrides on reads, and exposing detection as a first-class tool so the LLM can discover and disambiguate.

## Scope

### In Scope
- Extend `internal/project` with `DetectProjectFull(dir) DetectionResult` returning `{project, source, path, available, warning, err}` covering 5 cwd cases (git_remote, git_root, git_child auto-promotion, ambiguous_project error, dir_basename fallback)
- Keep `DetectProject(dir) string` as a thin wrapper for CLI backward compat
- Refactor MCP write tools (`mem_save`, `mem_save_prompt`, `mem_session_start`, `mem_session_end`, `mem_capture_passive`, `mem_update`) to remove `project` from schema and auto-detect from cwd
- Refactor MCP read tools (`mem_search`, `mem_context`, `mem_timeline`, `mem_get_observation`, `mem_stats`) to keep `project` optional; validate override via `store.ProjectExists`; fallback to auto-detect on empty
- Keep admin tools (`mem_delete`, `mem_merge_projects`) with `project` required
- Add `ProjectExists(name string) (bool, error)` helper on `store.Store`
- Add new tool `mem_current_project` (discovery: returns detection result + `available_projects`)
- Standardize response envelope: every tool result carries `project`, `project_source`, `project_path`; errors carry `available_projects` when relevant
- Tests: unit tests for all 5 cwd cases, store `ProjectExists`, MCP handler tests via `os.Chdir`/helper
- Update CHANGELOG documenting the breaking schema change

### Out of Scope
- CLI detection behavior changes — `DetectProject(dir) string` signature unchanged
- Plugin-side changes (Claude Code / OpenCode can still pass `project`; server silently ignores it on writes) — passing `cwd` through MCP `initialize` is deferred future work
- Migrations or retroactive bucket merges for previously mis-scoped data — out of scope, user must use `mem_merge_projects` manually
- Dashboard / cloud server changes — MCP-only

## Capabilities

### New Capabilities
- `mcp-project-resolution`: authoritative project resolution for MCP tool calls (auto-detect on writes, validated override on reads, discovery tool, 5-case cwd handling, structured error envelope)

### Modified Capabilities
- None — `mcp-server` capability does not have a standalone spec today; behavior changes are captured by the new capability above

## Approach

1. `internal/project/detect.go` → add `DetectionResult` struct and `DetectProjectFull`. Implement case order: (1) git_remote, (2) git_root, (3) scan cwd children at depth=1 looking for git repos (max 20 entries, skip `node_modules|vendor|.venv|__pycache__|target|dist|build`/hidden, 200ms timeout, short-circuit on >1 hit → `ErrAmbiguousProject` with `available`), (4) basename fallback.
2. `internal/store/store.go` → add `ProjectExists(name) (bool, error)` using `UNION ... LIMIT 1` across `observations`, `sessions`, `prompts`, `enrollment`.
3. `internal/mcp/mcp.go` → introduce `resolveWriteProject(ctx) (DetectionResult, error)` and `resolveReadProject(ctx, override string) (DetectionResult, error)` helpers. Rewrite each tool's handler + schema through these resolvers. Wrap all JSON responses in a standard envelope helper `respondWithProject(result, payload)`.
4. Register new `mem_current_project` tool in both `ProfileAgent` (discovery is agent-facing) and default set.
5. Tests in `internal/project/detect_test.go`, `internal/store/store_test.go`, `internal/mcp/mcp_test.go` following strict-TDD order.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/project/detect.go` | Modified | Add `DetectionResult`, `DetectProjectFull`, error types, child-scan helper |
| `internal/project/detect_test.go` | Modified | New tests for 5 cwd cases |
| `internal/store/store.go` | Modified | Add `ProjectExists` |
| `internal/store/store_test.go` | Modified | Tests for `ProjectExists` on empty / partial / populated DB |
| `internal/mcp/mcp.go` | Modified | Remove `project` from 6 write schemas; validate on reads; add `mem_current_project`; standard response envelope |
| `internal/mcp/mcp_test.go` | Modified | Handler tests via cwd control |
| `CHANGELOG.md` | Modified | Document breaking schema change for write tools |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Breaking schema change for write tools rejected by JSON-strict MCP clients | Medium | Document in CHANGELOG; server tolerates extra `project` arg on writes by silently discarding instead of erroring |
| Existing MCP handler tests pass explicit `project` and will break | High | Refactor tests via `t.Chdir` helper; part of the change |
| Child-repo scan misses deeply nested monorepos (depth >1) | Medium | Document depth=1 as contract; user works from the actual repo dir; `mem_current_project` surfaces what was chosen |
| Scan on networked/slow filesystems | Low | 200ms timeout + 20-entry cap short-circuits |
| `DetectProjectFull` path divergence from `DetectProject` semantics | Low | `DetectProject` reimplemented as `DetectProjectFull(dir).Project` so both stay in lockstep |

## Rollback Plan

Revert the feature branch. The change is contained to `internal/project`, `internal/store`, `internal/mcp`, and their tests — no schema migrations, no on-disk format changes, no cloud-server changes. CLI behavior is unchanged because `DetectProject(dir) string` keeps its signature. Memory already written under auto-detected projects remains readable; the only user-visible effect of reverting is that LLMs regain the ability to pass arbitrary `project` values on writes.

## Dependencies

- Existing `internal/project.DetectProject` and its git-probe helpers
- `github.com/mark3labs/mcp-go` tool schema API (`mcp.NewTool`, option builders)
- No new third-party dependencies

## Success Criteria

- [ ] All 5 cwd cases return the documented `DetectionResult` shape in `DetectProjectFull` unit tests
- [ ] Write-tool handlers ignore any LLM-supplied `project` argument; writes always land under the detected project
- [ ] Read-tool handlers accept explicit `project`, validate via `store.ProjectExists`, and return a structured error with `available_projects` when the name is unknown
- [ ] `mem_current_project` returns `{project, project_source, project_path, cwd, available_projects, warning?}` for every case including ambiguous
- [ ] Every MCP tool response includes `project`, `project_source`, `project_path`
- [ ] Ambiguous-cwd case returns a structured error (not a silent fallback) on writes
- [ ] CHANGELOG entry documents the write-tool schema break
- [ ] No regression in CLI: `cmd/engram` tests unchanged and passing
