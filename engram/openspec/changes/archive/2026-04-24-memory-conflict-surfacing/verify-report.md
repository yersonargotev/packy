# Verify Report: memory-conflict-surfacing (Re-verification)

Phase: SDD verify (re-run after followup fixes)
Change: memory-conflict-surfacing
Engram topic_key: `sdd/memory-conflict-surfacing/verify-report`
Artifact store: hybrid
Mode: Strict TDD

---

## Executive Summary

Status: done — 0 CRITICAL, 0 WARNING, 1 SUGGESTION (deferred to Phase 2).
Recommendation: **READY_TO_ARCHIVE**.

Both verify-followup fixes landed cleanly. The previously-flagged WARNING on REQ-001 (MCPConfig wire-through) is RESOLVED. The previously-flagged SUGGESTION 2 (floor=0 default-collision) is RESOLVED via pointer-nil semantics. The remaining SUGGESTION 1 (annotation `(<title>)` enrichment) was explicitly deferred to Phase 2 by user decision.

`go test ./...` PASS across all 18 packages. Both new RED→GREEN tests verified individually.

---

## Section 1: Status of previous findings

| Previous finding | Status | Evidence |
|------------------|--------|----------|
| WARNING: MCPConfig did not expose BM25Floor/Limit (REQ-001 wire-through gap) | **RESOLVED** | `internal/mcp/mcp.go:34-50` — `MCPConfig.BM25Floor *float64` and `MCPConfig.Limit *int` declared with full doc comments. Wired through `handleSave` at `internal/mcp/mcp.go:965-974` building `store.CandidateOptions`. New test `TestHandleSave_MCPConfig_OverridesDefaults` at `internal/mcp/mcp_test.go:3802-3854` PASS. |
| SUGGESTION 2: BM25Floor=0.0 zero-value collision in FindCandidates | **RESOLVED** | `internal/store/relations.go:65` — `CandidateOptions.BM25Floor` changed to `*float64`. Default detection at `internal/store/relations.go:162-167` uses pointer-nil check (`if opts.BM25Floor != nil { floor = *opts.BM25Floor }`). All 6 caller sites in `internal/store/relations_test.go` migrated to `ptrFloat64(...)` (zero stale float64 literals remain). New test `TestFindCandidates_ExplicitZeroFloor` at `internal/store/relations_test.go:784-828` PASS. |
| SUGGESTION 1: Annotation format omits `(<title>)` per spec example | **DEFERRED** | Per user decision, deferred to Phase 2. No code change in this batch. Tests still assert prefix-only — behavior is intentional and documented in `docs/PLUGINS.md`. |

---

## Section 2: New findings

None. The fix surfaces are minimal and mechanically correct.

- `MCPConfig` now uses pointer types (`*float64`, `*int`) — same idiom as Fix-2; nil = "use store default", explicit pointer = forwarded as-is. This is the canonical Go pattern for distinguishing "unset" from "set to zero value".
- `CandidateOptions.BM25Floor` uses pointer semantics; `CandidateOptions.Limit` retains `int` because `Limit <= 0` already meant "default" semantically (no zero-value ambiguity for a count). This asymmetric design is intentional and noted in apply-progress discoveries.
- Doc comments on both new MCPConfig fields explicitly call out the nil/explicit-pointer contract.
- All 6 BM25Floor caller sites in `relations_test.go` updated mechanically — verified via `rg`: no stale `BM25Floor: <literal>` patterns remain.
- The new test `TestFindCandidates_ExplicitZeroFloor` is a robust regression guard: asserts that explicit 0.0 returns 0 candidates (BM25 scores are always negative, so floor=0.0 is impossible to satisfy). Pre-fix, the zero would have collided with the default sentinel and produced ≥1 candidate.
- The new test `TestHandleSave_MCPConfig_OverridesDefaults` exercises the full wire path through `handleSave` with `MCPConfig{BM25Floor: ptr(0.0)}` and asserts the JSON envelope reports `judgment_required=false` and empty `candidates[]`. This proves the override travels end-to-end.

### Spot-check of unchanged surfaces (still PASS)

| Surface | Status | Evidence |
|---------|--------|----------|
| REQ-002 (search annotations) | PASS — unchanged | annotation switch intact |
| REQ-003 (mem_judge tool) | PASS — unchanged | tool implementation stable |
| REQ-004 (multi-actor schema) | PASS — unchanged | schema permits multiple rows |
| REQ-005 (provenance) | PASS — unchanged | provenance fields persisted |
| REQ-006 (decay schema) | PASS — unchanged | decay columns populated |
| REQ-007 (backwards compat) | PASS — unchanged | regression tests passing |
| REQ-008 (migration safety) | PASS — unchanged | migration is additive |
| REQ-009 (local-only relations) | PASS — unchanged | no sync mutations |
| REQ-010 (orphaning) | PASS — unchanged | orphaning mechanism works |
| 40 tasks marked done | CONFIRMED | apply-progress Phases A–H + Addendum all `[DONE]` |
| Strict TDD trail | INTACT | RED→GREEN evidence cited per task; addendum adds 2 explicit RED→GREEN cycles |

REQ-001 specifically: was flagged WARNING due to incomplete wire-through. Now PASS with full evidence (MCPConfig fields exist, `handleSave` forwards them, dedicated wire-through test PASSES).

---

## Section 3: Final recommendation

**READY_TO_ARCHIVE**

- 0 CRITICAL
- 0 WARNING (was 1 — resolved)
- 1 SUGGESTION (deferred to Phase 2 by user decision)
- All 10 REQs satisfied
- All 40 tasks complete
- Full test suite GREEN across 18 packages
- Strict TDD trail intact through addendum (RED→GREEN documented)

No blockers. The change is ready for `sdd-archive`.

---

## Section 4: Test execution

```
$ go test ./...
ok  	github.com/Gentleman-Programming/engram/cmd/engram	2.226s
ok  	github.com/Gentleman-Programming/engram/internal/cloud	(cached)
ok  	github.com/Gentleman-Programming/engram/internal/cloud/auth	(cached)
ok  	github.com/Gentleman-Programming/engram/internal/cloud/autosync	(cached)
ok  	github.com/Gentleman-Programming/engram/internal/cloud/chunkcodec	(cached)
ok  	github.com/Gentleman-Programming/engram/internal/cloud/cloudserver	(cached)
ok  	github.com/Gentleman-Programming/engram/internal/cloud/cloudstore	(cached)
ok  	github.com/Gentleman-Programming/engram/internal/cloud/dashboard	(cached)
ok  	github.com/Gentleman-Programming/engram/internal/cloud/remote	(cached)
ok  	github.com/Gentleman-Programming/engram/internal/mcp	2.502s
ok  	github.com/Gentleman-Programming/engram/internal/obsidian	(cached)
ok  	github.com/Gentleman-Programming/engram/internal/project	(cached)
ok  	github.com/Gentleman-Programming/engram/internal/server	(cached)
ok  	github.com/Gentleman-Programming/engram/internal/setup	(cached)
ok  	github.com/Gentleman-Programming/engram/internal/store	1.131s
ok  	github.com/Gentleman-Programming/engram/internal/sync	(cached)
ok  	github.com/Gentleman-Programming/engram/internal/tui	(cached)
ok  	github.com/Gentleman-Programming/engram/internal/version	(cached)
```

Targeted execution of the two new addendum tests:

```
$ go test -run TestFindCandidates_ExplicitZeroFloor ./internal/store/... -v
=== RUN   TestFindCandidates_ExplicitZeroFloor
--- PASS: TestFindCandidates_ExplicitZeroFloor (0.01s)

$ go test -run TestHandleSave_MCPConfig_OverridesDefaults ./internal/mcp/... -v
=== RUN   TestHandleSave_MCPConfig_OverridesDefaults
--- PASS: TestHandleSave_MCPConfig_OverridesDefaults (0.03s)
```

Coverage spot-check (unchanged from prior verify run, since fixes added tests + small wiring code): `internal/store` ≈78.5%, `internal/mcp` ≈94.9%.

---

## Files changed (verify-followup addendum only)

| File | Action | Notes |
|------|--------|-------|
| `internal/mcp/mcp.go` | Modified | Added `BM25Floor *float64` and `Limit *int` to `MCPConfig`. Wired both through `handleSave`. |
| `internal/mcp/mcp_test.go` | Appended | `TestHandleSave_MCPConfig_OverridesDefaults`. RED→GREEN cycle documented. |
| `internal/store/relations.go` | Modified | `CandidateOptions.BM25Floor` changed to `*float64`. Default detection updated to pointer-nil check. |
| `internal/store/relations_test.go` | Modified + Appended | All 6 BM25Floor caller sites migrated to `ptrFloat64(...)`. Added `TestFindCandidates_ExplicitZeroFloor` and `ptrFloat64` helper. |

Mirror: openspec/changes/archive/2026-04-24-memory-conflict-surfacing/verify-report.md
Engram topic_key: sdd/memory-conflict-surfacing/verify-report
