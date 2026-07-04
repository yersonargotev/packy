# Tasks: cloud-sync-audit-log

> Strict TDD — every implementation task is preceded by its RED test task.
> Postgres-gated cloudstore tests require `CLOUDSTORE_TEST_DSN`; skip when absent.

---

## Batch 1 — Persistence (blocks Batches 2 and 3)

### Phase 1.1 — DDL + Schema

- [x] 1.1.1 [RED] Write `cloudstore/audit_log_test.go`: test `cs.migrate()` idempotency — call twice, assert no error and no duplicate table/indexes. Refs: REQ-400 scenario 2.
- [x] 1.1.2 [RED] Write `cloudstore/audit_log_test.go`: test fresh migration creates `cloud_sync_audit_log` with all required columns and 3 indexes. Refs: REQ-400 scenario 1.
- [x] 1.1.3 [GREEN] Append idempotent DDL to `cs.migrate()` in `cloudstore/cloudstore.go`: `CREATE TABLE IF NOT EXISTS cloud_sync_audit_log (id SERIAL PK, occurred_at TIMESTAMPTZ DEFAULT NOW(), contributor TEXT NOT NULL, project TEXT NOT NULL, action TEXT NOT NULL, outcome TEXT NOT NULL, entry_count INT DEFAULT 0, reason_code TEXT, metadata JSONB); CREATE INDEX IF NOT EXISTS` on `occurred_at DESC`, `(contributor, project)`, `outcome`. Tests 1.1.1 and 1.1.2 must pass.

### Phase 1.2 — Types and Constants

- [x] 1.2.1 [GREEN] Create `internal/cloud/cloudstore/audit_log.go`: export `AuditEntry`, `AuditFilter`, `DashboardAuditRow` structs; export constants `AuditOutcomeRejectedProjectPaused = "rejected_project_paused"`, `AuditActionMutationPush = "mutation_push"`, `AuditActionChunkPush = "chunk_push"`. No test needed — pure type definitions; compiled by downstream tests. Refs: REQ-401, REQ-413.

### Phase 1.3 — InsertAuditEntry

- [x] 1.3.1 [RED] Write test: `InsertAuditEntry` happy path inserts one row; round-trip via `ListAuditEntriesPaginated` returns matching `DashboardAuditRow`. Refs: REQ-401 scenario 1, REQ-402 scenario 1.
- [x] 1.3.2 [RED] Write test: `InsertAuditEntry` with cancelled context returns error, no row inserted. Refs: REQ-402 scenario 2.
- [x] 1.3.3 [GREEN] Implement `(*CloudStore).InsertAuditEntry(ctx, AuditEntry) error` in `audit_log.go`: parameterized INSERT; return error to caller; do NOT suppress. Tests 1.3.1 and 1.3.2 must pass.

### Phase 1.4 — ListAuditEntriesPaginated

- [x] 1.4.1 [RED] Write test: 25 rows seeded; `ListAuditEntriesPaginated(filter{}, 10, 0)` returns 10 rows, total=25; rows sorted `occurred_at DESC`. Refs: REQ-403 scenario 1.
- [x] 1.4.2 [RED] Write test: contributor filter — 3 "alice" rows + 2 "bob" rows; filter by `Contributor="alice"` returns 3 rows, total=3. Refs: REQ-403 scenario 2.
- [x] 1.4.3 [RED] Write test: outcome filter — rows with `outcome=AuditOutcomeRejectedProjectPaused` and one other; filter narrows to matching rows only. Refs: REQ-403 (implied by REQ-410).
- [x] 1.4.4 [RED] Write test: time-range filter with `OccurredAtFrom` and `OccurredAtTo` set; only rows within inclusive range returned. Refs: REQ-403 scenario 3.
- [x] 1.4.5 [RED] Write test: empty result — filter matches nothing; returns empty slice, total=0, err=nil. Refs: REQ-403 scenario 4.
- [x] 1.4.6 [GREEN] Implement `(*CloudStore).ListAuditEntriesPaginated(ctx, AuditFilter, limit, offset int) ([]DashboardAuditRow, int, error)` in `audit_log.go`: parameterized SQL with AND-chained optional filters, `ORDER BY occurred_at DESC`, `LIMIT/OFFSET`; separate `COUNT(*)` for total. All 1.4.x tests must pass.

---

## Batch 2 — Handler Integration + Dashboard UI (depends on Batch 1)

### Phase 2.1 — Mutation Push Handler Audit Emission

- [x] 2.1.1 [RED] Write test in `cloudserver/mutations_test.go`: POST to mutation push endpoint with paused project → 409 returned AND fake store captures one `InsertAuditEntry` call with `Action=AuditActionMutationPush`, `Outcome=AuditOutcomeRejectedProjectPaused`. Refs: REQ-404 scenario 1.
- [x] 2.1.2 [RED] Write test: non-paused project mutation push → 200 returned AND fake store captures zero `InsertAuditEntry` calls. Refs: REQ-404 scenario 2.
- [x] 2.1.3 [RED] Write test: mutation push with `"created_by": "alice"` + paused → audit row `Contributor="alice"`. Refs: REQ-406 scenario 1.
- [x] 2.1.4 [RED] Write test: mutation push without `created_by` + paused → audit row `Contributor="unknown"`. Refs: REQ-406 scenario 2.
- [x] 2.1.5 [RED] Write test: fake store `InsertAuditEntry` returns error → handler still returns 409 (no 5xx). Refs: REQ-404 scenario 3.
- [x] 2.1.6 [GREEN] Modify `cloudserver/mutations.go`: add optional `CreatedBy string \`json:"created_by,omitempty"\`` to push request struct; inside the `sync-paused` 409 branch add structural type assertion `s.store.(interface{ InsertAuditEntry(ctx, AuditEntry) error })`; if ok, call `InsertAuditEntry`; on error log WARN; if !ok log WARN; contributor = `req.CreatedBy` if non-empty else `"unknown"`. All 2.1.x tests must pass.

### Phase 2.2 — Chunk Push Handler Audit Emission

- [x] 2.2.1 [RED] Write test in `cloudserver/cloudserver_test.go`: chunk push endpoint with paused project → 409 AND fake store captures one `InsertAuditEntry` with `Action=AuditActionChunkPush`. Refs: REQ-405 scenario 1.
- [x] 2.2.2 [RED] Write test: non-paused project chunk push → 200 AND zero `InsertAuditEntry` calls. Refs: REQ-405 scenario 2.
- [x] 2.2.3 [RED] Write test: store lacking `InsertAuditEntry` (assertion fails) → 409 returned, no panic. Refs: REQ-405 scenario 3, REQ-412 scenario 2.
- [x] 2.2.4 [GREEN] Modify `cloudserver/cloudserver.go:handlePushChunk`: apply identical structural-assertion pattern as 2.1.6; `Action=AuditActionChunkPush`; contributor from `req.CreatedBy`. All 2.2.x tests must pass.

### Phase 2.3 — Pull Path Negative Test

- [x] 2.3.1 [RED] Write test: pull request against paused project → 200 AND zero `InsertAuditEntry` calls. Refs: REQ-407 scenario 1.
- [x] 2.3.2 [GREEN] Verify `handleMutationPull` source contains no `InsertAuditEntry` call (no code change expected; test is a structural assertion). Refs: REQ-407 scenario 2.

### Phase 2.4 — DashboardStore Interface Extension

- [x] 2.4.1 [RED] Extend `parityStoreStub` in `dashboard/dashboard_test.go` with a `ListAuditEntriesPaginated` no-op method; verify existing dashboard tests still compile and pass. Refs: REQ-409 (interface parity).
- [x] 2.4.2 [GREEN] Add `ListAuditEntriesPaginated(ctx, AuditFilter, int, int) ([]DashboardAuditRow, int, error)` to `DashboardStore` interface in `dashboard/dashboard.go`. Test 2.4.1 must pass.

### Phase 2.5 — Templ Components

- [x] 2.5.1 [RED] Write render test in `dashboard/dashboard_test.go`: `AdminAuditLogPage` renders with HTMX attrs `hx-get="/dashboard/admin/audit-log/list"` and `hx-trigger="load"`. Refs: REQ-408 scenario 1.
- [x] 2.5.2 [RED] Write render test: `AdminAuditLogListPartial` renders a table with filter inputs (contributor, project, outcome dropdown, from, to) and `HtmxPaginationBar`. Refs: REQ-409 scenario 1.
- [x] 2.5.3 [RED] Write render test: outcome dropdown option values match exported constants `AuditOutcomeRejectedProjectPaused`. Refs: REQ-410 scenario 2, REQ-413 scenario 2.
- [x] 2.5.4 [RED] Write structural render tests: `AdminPage`, `AdminProjectsPage`, `AdminUsersPage`, `AdminHealthPage` each contain `<a href="/dashboard/admin/audit-log">` with text "Audit Log". Refs: REQ-411 all scenarios.
- [x] 2.5.5 [GREEN] Refactor `components.templ`: extract shared `adminNav(active string)` partial; replace four duplicated admin-nav `<div>` blocks with `@adminNav("...")` calls; add "Audit Log" link inside `adminNav`. Refs: REQ-411, design decision 9.
- [x] 2.5.6 [GREEN] Add `AdminAuditLogPage(displayName string, filter AuditFilter)` and `AdminAuditLogListPartial(entries []DashboardAuditRow, pg PaginationMeta, filter AuditFilter)` components to `components.templ`. All 2.5.x tests must pass.
- [x] 2.5.7 [GREEN] Run `go tool templ generate ./internal/cloud/dashboard/...` to regenerate `components_templ.go`. Verify compile succeeds.

### Phase 2.6 — Admin Routes and Handlers

- [x] 2.6.1 [RED] Write HTTP handler test: `GET /dashboard/admin/audit-log` as admin → 200 with shell HTML. Refs: REQ-408 scenario 1.
- [x] 2.6.2 [RED] Write HTTP handler test: `GET /dashboard/admin/audit-log` as non-admin → 401 or 403. Refs: REQ-408 scenario 2.
- [x] 2.6.3 [RED] Write HTTP handler test: `GET /dashboard/admin/audit-log/list` as admin → 200 HTML partial with audit rows. Refs: REQ-409 scenario 1.
- [x] 2.6.4 [RED] Write HTTP handler test: `GET /dashboard/admin/audit-log/list?contributor=alice` → only alice rows rendered. Refs: REQ-409 scenario 2.
- [x] 2.6.5 [RED] Write HTTP handler test: `GET /dashboard/admin/audit-log/list` as non-admin → 401 or 403. Refs: REQ-409 scenario 3.
- [x] 2.6.6 [GREEN] Add handlers `handleAdminAuditLog` (shell) and `handleAdminAuditLogList` (partial) to `dashboard/dashboard.go`: parse query params (`contributor`, `project`, `outcome`, `from`/`to` as RFC3339 `time.Parse`); apply reclamp pagination pattern; call `DashboardStore.ListAuditEntriesPaginated`; render components. All 2.6.x tests must pass.
- [x] 2.6.7 [GREEN] Register `GET /dashboard/admin/audit-log` and `GET /dashboard/admin/audit-log/list` routes in `dashboard.Mount`; apply existing admin-gate middleware. Refs: REQ-408, REQ-409.

### Phase 2.7 — End-to-End Integration Test

- [x] 2.7.1 [RED] Write integration test (file: `internal/cloud/cloudserver/cloudserver_test.go`): POST to `/sync/mutations/push` with paused project → 409 → verify audit row captured → list via store method confirms row present with correct contributor. Refs: REQ-404 + REQ-409 combined.
- [x] 2.7.2 [GREEN] Confirm existing code changes satisfy the integration test without further modification. Tests pass.

---

## Batch 3 — Documentation (depends on Batch 1 only; parallelizable with Batch 2)

### Phase 3.1 — DOCS.md

- [x] 3.1.1 Add "Audit Log" section to `DOCS.md`: schema table (column name, type, description for all 9 columns), outcome vocabulary (`rejected_project_paused`), action discriminator (`mutation_push` / `chunk_push`), retention limitation and manual prune recipe. Refs: proposal OUT-of-scope retention note.

### Phase 3.2 — ARCHITECTURE.md

- [x] 3.2.1 Add paragraph to `docs/ARCHITECTURE.md` under cloud server section: structural-interface assertion pattern, why `ChunkStore`/`MutationStore` are NOT extended, synchronous-insert rationale, audit boundary at the pause-rejection site.

### Phase 3.3 — Final Verification

- [x] 3.3.1 Run `go vet ./...`; fix any issues found.
- [x] 3.3.2 Run `gofmt -l ./internal/cloud/... ./cmd/engram/...`; fix any formatting drift.
- [x] 3.3.3 Run `go test ./...` (non-Postgres-gated tests); confirm green.
- [x] 3.3.4 Run `go test -race ./internal/cloud/... ./cmd/engram/...`; confirm no race conditions.

---

## TDD Cycle Evidence Table

> sdd-apply fills this table as it works through each batch.

| Task | Cycle | Status | Notes |
|------|-------|--------|-------|
| 1.1.1 | RED | [x] | TestAuditLogMigrationIdempotent |
| 1.1.2 | RED | [x] | TestAuditLogMigrationCreatesTable |
| 1.1.3 | GREEN | [x] | DDL appended to cloudstore.go migrate() |
| 1.2.1 | GREEN | [x] | audit_log.go created with types+constants |
| 1.3.1 | RED | [x] | TestInsertAuditEntryRoundTrip |
| 1.3.2 | RED | [x] | TestInsertAuditEntryCancelledContext |
| 1.3.3 | GREEN | [x] | InsertAuditEntry implemented |
| 1.4.1 | RED | [x] | TestAuditListPaginationAndTotal |
| 1.4.2 | RED | [x] | TestAuditListContributorFilter |
| 1.4.3 | RED | [x] | TestAuditListOutcomeFilter |
| 1.4.4 | RED | [x] | TestAuditListTimeRangeFilter |
| 1.4.5 | RED | [x] | TestAuditListEmptyResult |
| 1.4.6 | GREEN | [x] | ListAuditEntriesPaginated implemented |
| 2.1.1 | RED | [x] | TestMutationPushPaused409EmitsAudit |
| 2.1.2 | RED | [x] | TestMutationPushNonPaused200EmitsNoAudit |
| 2.1.3 | RED | [x] | TestMutationPushPausedWithCreatedBy |
| 2.1.4 | RED | [x] | TestMutationPushPausedWithoutCreatedByDefaultsUnknown |
| 2.1.5 | RED | [x] | TestMutationPushAuditInsertFailureStill409 |
| 2.1.6 | GREEN | [x] | mutations.go updated with audit emission |
| 2.2.1 | RED | [x] | TestChunkPushPaused409EmitsAuditWithChunkAction |
| 2.2.2 | RED | [x] | TestChunkPushEnabled200EmitsNoAudit |
| 2.2.3 | RED | [x] | TestChunkPushStoreWithoutInsertAuditEntryDoesNotPanic |
| 2.2.4 | GREEN | [x] | cloudserver.go handlePushChunk updated |
| 2.3.1 | RED | [x] | TestMutationPullEmitsNoAuditOnPausedProject |
| 2.3.2 | GREEN | [x] | Verified — handleMutationPull has no InsertAuditEntry call |
| 2.4.1 | RED | [x] | parityStoreStub updated in dashboard_test.go |
| 2.4.2 | GREEN | [x] | DashboardStore interface extended |
| 2.5.1 | RED | [x] | TestAdminAuditLogPageHTMXWiring |
| 2.5.2 | RED | [x] | TestAdminAuditLogListPartialRendersFilterInputs |
| 2.5.3 | RED | [x] | TestAdminAuditLogListPartialOutcomeDropdown |
| 2.5.4 | RED | [x] | TestAdminNavAuditLogLinkInAllFourPages |
| 2.5.5 | GREEN | [x] | adminNav partial extracted; all 4 pages refactored |
| 2.5.6 | GREEN | [x] | AdminAuditLogPage + AdminAuditLogListPartial added |
| 2.5.7 | GREEN | [x] | templ generate ran; build passes |
| 2.6.1 | RED | [x] | TestAdminAuditLogShellRouteAdminAccess |
| 2.6.2 | RED | [x] | TestAdminAuditLogShellRouteNonAdminDenied |
| 2.6.3 | RED | [x] | TestAdminAuditLogListPartialAdminAccess |
| 2.6.4 | RED | [x] | TestAdminAuditLogListFilterByContributor |
| 2.6.5 | RED | [x] | TestAdminAuditLogListPartialNonAdminDenied |
| 2.6.6 | GREEN | [x] | handleAdminAuditLog + handleAdminAuditLogList implemented |
| 2.6.7 | GREEN | [x] | Routes registered in dashboard.Mount |
| 2.7.1 | RED | [x] | TestAuditLogE2E_MutationPushPausedThenListRendered |
| 2.7.2 | GREEN | [x] | All existing code satisfies the E2E test |
| 3.1.1 | GREEN | [x] | DOCS.md audit log section added |
| 3.2.1 | GREEN | [x] | ARCHITECTURE.md structural-interface section added |
| 3.3.1 | GREEN | [x] | go vet ./... clean |
| 3.3.2 | GREEN | [x] | gofmt clean |
| 3.3.3 | GREEN | [x] | go test ./... all pass |
| 3.3.4 | GREEN | [x] | go test -race clean |

---

## Dependency Graph

```
Batch 1 (Persistence)
  └─> Batch 2 (Handlers + UI)
        └─> 2.7 (E2E — requires both 2.1 and 2.6 complete)
  └─> Batch 3 (Docs — can run parallel to Batch 2 once Batch 1 is done)
```

## Spec Coverage Matrix

| REQ | Covered by tasks |
|-----|-----------------|
| REQ-400 | 1.1.1, 1.1.2, 1.1.3 |
| REQ-401 | 1.2.1, 1.3.1 |
| REQ-402 | 1.3.1, 1.3.2, 1.3.3 |
| REQ-403 | 1.4.1 – 1.4.6 |
| REQ-404 | 2.1.1, 2.1.2, 2.1.5, 2.1.6 |
| REQ-405 | 2.2.1, 2.2.2, 2.2.3, 2.2.4 |
| REQ-406 | 2.1.3, 2.1.4, 2.1.6 |
| REQ-407 | 2.3.1, 2.3.2 |
| REQ-408 | 2.5.1, 2.6.1, 2.6.2, 2.6.7 |
| REQ-409 | 2.4.1, 2.4.2, 2.5.2, 2.6.3, 2.6.4, 2.6.5, 2.6.7 |
| REQ-410 | 1.4.3, 1.4.4, 2.5.3, 2.6.6 |
| REQ-411 | 2.5.4, 2.5.5 |
| REQ-412 | 2.1.5, 2.2.3, 2.1.6, 2.2.4 |
| REQ-413 | 1.2.1, 2.5.3 |
| REQ-414 | 2.1.3, 2.1.6 (response envelope fields) |
