# Archive Report — cloud-dashboard-visual-parity

**Change**: cloud-dashboard-visual-parity
**Status**: CLOSED
**Archived**: 2026-04-23
**Verdict**: PASS — Zero CRITICAL issues, all test gates clear, all specs implemented.

---

## Executive Summary

The `cloud-dashboard-visual-parity` change has been fully implemented, verified, and archived. All 69 tasks (43 from Batches 1-5 plus 22 from Batch 6 hotfixes) are complete. All 14 spec requirements (REQ-100 through REQ-113) pass verification. Test coverage is 71.9% (above the 79% baseline from prior changes). The integrated cloud dashboard now has complete visual and functional parity with the legacy `engram-cloud` dashboard without changing the auth model, sync contract, or chunk-centric cloudstore boundary.

---

## Spec Compliance

All 14 delta requirements (REQ-100 to REQ-113) implemented and passing:

| REQ | Description | Status | Test Seam |
|-----|-------------|--------|-----------|
| REQ-100 | Static asset byte-size floors | PASS | `TestStaticAssetByteFloors` |
| REQ-101 | Templ deterministic regeneration + v0.3.1001 pin | PASS | `TestTemplGeneratedFilesAreCheckedIn` |
| REQ-102 | Row types carry detail fields | PASS | `TestDashboardRowDetailFields` |
| REQ-103 | MountConfig GetDisplayName fallback | PASS | `TestGetDisplayNameFallback` |
| REQ-104 | Project sync control persistence | PASS | `TestProjectSyncControlPersists` |
| REQ-105 | Dashboard system health aggregator | PASS | `TestCloudstoreSystemHealthAggregates` |
| REQ-106 | Full HTMX endpoint surface (11 endpoints) | PASS | `TestFullHTMXEndpointSurface` |
| REQ-107 | Layout structural parity | PASS | `TestDashboardLayoutHTMLStructure` + 2 more |
| REQ-108 | HTMX attribute presence | PASS | `TestDashboardHomeHTMXWiring` + 2 more |
| REQ-109 | Push-path pause enforcement | PASS | `TestPushPathPauseEnforcement` |
| REQ-110 | Insecure-mode login regression guard | PASS | `TestInsecureModeLoginRedirects` |
| REQ-111 | Copy parity strings | PASS | `TestCopyParityStrings` |
| REQ-112 | Admin gate on mutations | PASS | `TestAdminSyncTogglePosts` + `TestAdminSyncToggleRequiresAdmin` |
| REQ-113 | Principal bridge (no direct context) | PASS | `TestPrincipalBridgeNoPanicOnEmptyContext` |

Delta spec file: `openspec/changes/archive/2026-04-23-cloud-dashboard-visual-parity/spec.md`
Main merged spec: `openspec/specs/cloud-dashboard/spec.md`

---

## Test Results

### Test Gate Summary
- `go test ./... -count=1` — **19/19 packages PASS, zero failures**
- `go test -cover ./...` — **71.9% overall coverage** (baseline 71.6%)
  - cloudstore: 50.0% (up from 46.8%)
  - dashboard: 55.0% (up from 54.8%)
- `go test -race ./internal/cloud/cloudstore/... ./internal/cloud/dashboard/...` — **CLEAN**
- `go vet ./...` — **CLEAN**
- `gofmt -l` (modified files) — **CLEAN**

### Completed Tasks
- **69/69 tasks complete**
  - Batches 1-5: 43 tasks (original plan)
  - Batch 6: 22 tasks (hotfixes for bugs 1-5)
  - 4 follow-up documentation tasks

### Bugs Closed (5/5)
1. **Bug 1** ("No content captured") — CLOSED via `applyDashboardMutation` merge semantics
2. **Bug 2** (Observation link redirect to home) — CLOSED via ChunkID preservation in URLs
3. **Bug 3** (Session trace empty content) — CLOSED via Content field merge in read model
4. **Bug 4** (OBSERVATIONS label wraps) — CLOSED via CSS white-space:nowrap
5. **Bug 5** (Contributor table cut off) — CLOSED via div.table-scroll wrapper + min-width:0

### Verification Report
Full verify report: `sdd/cloud-dashboard-visual-parity/verify-report-final` (Engram ID #2497)

**Verdict**: PASS (zero CRITICAL)
- **CRITICAL**: 0
- **WARNING**: 2 (carry-over structural — pre-existing, documented)
- **SUGGESTION**: 2 (optional polish items)

---

## Artifacts Produced

### OpenSpec Artifacts
- Delta spec: `openspec/changes/cloud-dashboard-visual-parity/specs/cloud-dashboard/spec.md`
- Main merged spec: `openspec/specs/cloud-dashboard/spec.md` (NEW, created during archive)
- Design doc: `openspec/changes/cloud-dashboard-visual-parity/design.md`
- Tasks doc: `openspec/changes/cloud-dashboard-visual-parity/tasks.md`
- Proposal: `openspec/changes/cloud-dashboard-visual-parity/proposal.md`

### Code Changes (Feature)

**Static Assets**
- `internal/cloud/dashboard/static/htmx.min.js` (50 917 B, ≥ 40 KB floor)
- `internal/cloud/dashboard/static/pico.min.css` (71 072 B, ≥ 60 KB floor)
- `internal/cloud/dashboard/static/styles.css` (23 185 B, ≥ 20 KB floor)

**Templ Components (Committed Generated Files)**
- `internal/cloud/dashboard/components_templ.go` (≥ 100 KB, fully generated)
- `internal/cloud/dashboard/layout_templ.go` (fully generated)
- `internal/cloud/dashboard/login_templ.go` (fully generated)

**Source Templ Files (Ported Verbatim)**
- `internal/cloud/dashboard/components.templ` (~1 134 lines)
- `internal/cloud/dashboard/layout.templ` (~52 lines, adapted)
- `internal/cloud/dashboard/login.templ` (~63 lines, adapted)

**CloudStore Extensions**
- `internal/cloud/cloudstore/dashboard_queries.go` — 5 paginated list methods, 3 detail methods, SystemHealth
- `internal/cloud/cloudstore/project_controls.go` (NEW) — ProjectSyncControl type + 4 methods
- `internal/cloud/cloudstore/cloudstore.go` — Postgres migration for cloud_project_controls table

**Dashboard Handlers**
- `internal/cloud/dashboard/dashboard.go` — 11 new route handlers, principal bridge, templ rendering
- `internal/cloud/dashboard/dashboard_test.go` — 30+ new test methods
- `internal/cloud/dashboard/helpers.go` — 424-line port of legacy version
- `internal/cloud/dashboard/principal.go` (NEW) — Principal type + principalFromRequest
- `internal/cloud/dashboard/templ_policy.go` (existing) — no changes
- `internal/cloud/dashboard/templ_policy_test.go` (NEW) — asset floor + generated file tests

**CloudServer Integration**
- `internal/cloud/cloudserver/cloudserver.go` — GetDisplayName wiring, push-path pause guard
- `internal/cloud/cloudserver/cloudserver_test.go` — TestPushPathPauseEnforcement, TestInsecureModeLoginRedirects

**Dependencies & Tooling**
- `go.mod` — github.com/a-h/templ v0.3.1001, golang.org/x/net direct deps
- `tools/tools.go` (NEW) — build-tag pattern for templ binary retention
- `Makefile` — templ target added

**Documentation**
- `DOCS.md` — templ regeneration contributor subsection
- `README.md` — one-line pointer to DOCS.md
- `docs/ARCHITECTURE.md` — dashboard visual-parity subsection
- `docs/AGENT-SETUP.md` — templ regeneration command

---

## Engram Artifact References

All SDD artifacts persisted to Engram for cross-session recovery:

| Artifact | Engram Topic Key | Observation ID |
|----------|------------------|-----------------|
| Proposal | `sdd/cloud-dashboard-visual-parity/proposal` | #2422 |
| Spec | `sdd/cloud-dashboard-visual-parity/spec` | #2429 |
| Design | `sdd/cloud-dashboard-visual-parity/design-decisions` | #2432 |
| Tasks | `sdd/cloud-dashboard-visual-parity/tasks` | #2442 |
| Verify Report (Final) | `sdd/cloud-dashboard-visual-parity/verify-report-final` | #2497 |
| Archive Report | `sdd/cloud-dashboard-visual-parity/archive-report` | (persisted with this document) |

Supporting decisions:
- Decision: DashboardXxxRow extend flat types additively — #2424
- Decision: ProjectSyncControl table ports to integrated cloudstore — #2425
- Discovery: insecure-mode gaps resolved — #2426
- Pattern: Batch rollout plan (5 batches, TDD RED-first) — #2434

---

## Architecture Summary

### Four-Layer Implementation
1. **Asset Layer**: Static assets copied byte-for-byte from legacy
2. **Templ Layer**: Components ported verbatim, import-path + type adapted
3. **Store Layer**: Flat-row extensions + pagination + sync controls + system health
4. **Handler/Mount Layer**: Templ rendering + 11 new routes + Principal bridge + push-path guard

### Key Design Decisions
1. **Pagination**: In-memory slicing of read model (not SQL LIMIT/OFFSET)
2. **Sync Table**: Postgres with `CREATE TABLE IF NOT EXISTS` (idempotent)
3. **Composite URLs**: `/dashboard/sessions/{project}/{sessionID}` (Go 1.22 patterns)
4. **Templ Version**: Pinned to v0.3.1001 (byte-identical regeneration)
5. **Push Guard**: Structural interface assertion (test-friendly)
6. **Principal Bridge**: Closure-based, nil-safe fallback to "OPERATOR"
7. **Test Approach**: Tokenizer for structural tests, substring for copy/HTMX
8. **Templ CI**: Hash-free (header regex + size floor, no shelling out)
9. **Assets**: `//go:embed` FS is authoritative (not disk)
10. **Batch Ordering**: RED-first, dependency-ordered, 5 sequential batches

### Coverage Metrics
- Total coverage: 71.9% (baseline 71.6%, +0.3%)
- cloudstore: 50.0% (baseline 46.8%, +3.2%)
- dashboard: 55.0% (baseline 54.8%, +0.2%)

---

## Completeness Checklist

### Specification (14/14 REQs)
- [x] REQ-100: Static asset floors
- [x] REQ-101: Templ version pin + generated files
- [x] REQ-102: Row detail fields
- [x] REQ-103: GetDisplayName bridge
- [x] REQ-104: Sync controls persistence
- [x] REQ-105: System health aggregator
- [x] REQ-106: 11 HTMX endpoints
- [x] REQ-107: Layout structural parity
- [x] REQ-108: HTMX attributes
- [x] REQ-109: Push-path pause
- [x] REQ-110: Insecure-mode guard
- [x] REQ-111: Copy parity
- [x] REQ-112: Admin gate
- [x] REQ-113: Principal bridge

### Implementation (69/69 Tasks)
- [x] Batch 1: Templ bootstrap + assets + RED tests
- [x] Batch 2: Templ sources + generated files + structural tests
- [x] Batch 3: Row extensions + cloudstore + Principal bridge
- [x] Batch 4: 11 routes + HTMX + admin + push guard
- [x] Batch 5: Docs + verify
- [x] Batch 6: Hotfixes for bugs 1-5

### Quality Gates
- [x] go test ./... — 19/19 packages PASS
- [x] go test -race — CLEAN
- [x] go vet — CLEAN
- [x] gofmt — CLEAN
- [x] Coverage ≥ 71.6% (actual: 71.9%)
- [x] Zero CRITICAL issues
- [x] Manual spot-check (engram.condetuti.com)

---

## File Movement (Hybrid Mode)

**Source**: `openspec/changes/cloud-dashboard-visual-parity/`
**Destination**: `openspec/changes/archive/2026-04-23-cloud-dashboard-visual-parity/`

Archived files:
- proposal.md
- design.md
- spec.md (link to merged main spec)
- tasks.md (reference)
- archive-report.md (this file)

Main spec merged to: `openspec/specs/cloud-dashboard/spec.md` (NEW)

---

## Next Steps

1. **Immediate**: No further work required on this change. It is fully closed.
2. **Follow-up**: Potential future changes on related surfaces:
   - `cloud-dashboard-parity` (prior) — complete as-is
   - `cloud-upgrade-path-existing-users` (independent) — not blocked by this change
3. **Maintenance**: The `cloud_project_controls` table and new cloudstore methods are part of the stable API. Any future dashboard work should use the existing `DashboardStore` interface additions.

---

## Rollback Plan (if needed)

**Rollback procedure**:
1. Delete `internal/cloud/cloudstore/project_controls.go` and `project_controls_test.go`
2. Remove the two `CREATE TABLE IF NOT EXISTS cloud_project_controls` statements from `cloudstore.go:migrate()` queries slice (or leave them — they are idempotent)
3. Revert `internal/cloud/dashboard/dashboard.go` to pre-change commit (handlers, MountConfig.GetDisplayName, 11 new routes)
4. Revert `internal/cloud/cloudserver/cloudserver.go` to pre-change commit (push-path guard, GetDisplayName wiring)
5. Delete `internal/cloud/dashboard/principal.go`
6. Revert static assets and templ files to stubs
7. Revert `go.mod` + `go.sum` to pre-change versions
8. Delete `tools/tools.go`
9. Remove `Makefile` templ target
10. Revert docs

**Impact**: Dashboard reverts to the `cloud-dashboard-parity` state (routes work, stubs present, no visual parity). This is a backwards-compatible revert because the change was purely additive to the cloudstore interface (new methods on DashboardStore, not changes to existing method signatures).

---

## Sign-off

**Change**: cloud-dashboard-visual-parity
**Status**: ARCHIVED & CLOSED
**Date**: 2026-04-23
**Verdict**: PASS — all requirements met, all tests green, zero CRITICAL issues

This change is ready for production deployment.

---

*Archive report generated by sdd-archive executor on 2026-04-23. Traceability: all observation IDs reference Engram artifacts; all files listed are committed to `feat/integrate-engram-cloud` branch.*
