# Cloud Sync Audit Log Specification

## Purpose

Persistent, admin-visible audit trail of every push rejection that occurs when a project's sync is paused. Covers persistence layer, server-handler integration, and admin dashboard UI.

---

## Requirements

### REQ-400: cloud_sync_audit_log Table Schema and Migration

The system MUST append idempotent DDL to `cs.migrate()` that creates the `cloud_sync_audit_log` table and its indexes. The DDL MUST use `CREATE TABLE IF NOT EXISTS` and `CREATE INDEX IF NOT EXISTS`. The table MUST contain columns: `id` (auto-incrementing PK), `occurred_at` (timestamp with default NOW), `contributor` (text, not null), `project` (text, not null), `action` (text, not null), `outcome` (text, not null), `entry_count` (integer, default 0), `reason_code` (text, nullable), `metadata` (JSONB, nullable). Indexes MUST cover `occurred_at DESC`, `(contributor, project)`, and `outcome`.

#### Scenario: First migration creates table and indexes
- GIVEN a fresh database with no `cloud_sync_audit_log` table
- WHEN `cs.migrate()` is called
- THEN the table exists with all required columns
- AND all three indexes exist

#### Scenario: Repeated migration is idempotent
- GIVEN a database where `cs.migrate()` has already been called once
- WHEN `cs.migrate()` is called again
- THEN no error is returned
- AND no duplicate tables or indexes are created

#### Scenario: Concurrent or out-of-order migration
- GIVEN a database where the `cloud_sync_audit_log` table already exists
- WHEN `cs.migrate()` is called
- THEN the call succeeds without error

---

### REQ-401: AuditEntry, AuditFilter, and DashboardAuditRow Types

The system MUST define three Go types exported from the `cloudstore` package. `AuditEntry` MUST carry: `Contributor string`, `Project string`, `Action string`, `Outcome string`, `EntryCount int`, `ReasonCode string`. `AuditFilter` MUST carry optional fields: `Contributor`, `Project`, `Outcome`, `OccurredAtFrom`, `OccurredAtTo`. `DashboardAuditRow` MUST carry all `AuditEntry` fields plus `ID int64` and `OccurredAt time.Time`.

#### Scenario: AuditEntry fields round-trip through insert and list
- GIVEN an `AuditEntry` with all fields populated
- WHEN `InsertAuditEntry` is called then `ListAuditEntriesPaginated` with no filter
- THEN the returned `DashboardAuditRow` matches all inserted fields

#### Scenario: Zero-value AuditFilter is valid
- GIVEN an `AuditFilter` with all fields at their zero value
- WHEN `ListAuditEntriesPaginated` is called
- THEN no SQL error is returned and all rows are returned (unfiltered)

---

### REQ-402: CloudStore.InsertAuditEntry Synchronous Insert Contract

`CloudStore` MUST implement `InsertAuditEntry(ctx context.Context, entry AuditEntry) error`. The insert MUST be synchronous — no goroutine or buffered channel. On DB transient error, the method MUST return the error to the caller. The caller MUST log at WARN and MUST NOT promote the error to an HTTP 5xx — the 409 rejection semantics remain stable regardless of audit insert outcome.

#### Scenario: Successful insert persists one row
- GIVEN a running database with the audit table
- WHEN `InsertAuditEntry` is called with a valid `AuditEntry`
- THEN exactly one row is inserted and no error is returned

#### Scenario: Context cancellation surfaces as error
- GIVEN a context already cancelled
- WHEN `InsertAuditEntry` is called
- THEN an error is returned and no row is inserted

#### Scenario: DB error is returned to caller, not suppressed
- GIVEN the database is unavailable
- WHEN `InsertAuditEntry` is called
- THEN the error is returned to the caller
- AND no panic occurs

---

### REQ-403: CloudStore.ListAuditEntriesPaginated Filter and Pagination

`CloudStore` MUST implement `ListAuditEntriesPaginated(filter AuditFilter, limit, offset int) (entries []DashboardAuditRow, total int, err error)`. Results MUST be sorted by `occurred_at DESC`. The `total` return value MUST be the count of rows matching the filter (not just the current page). All `AuditFilter` fields MUST be independently optional; when set they are applied as AND conditions.

#### Scenario: Pagination returns correct page and total
- GIVEN 25 audit rows in the table with no filters applied
- WHEN `ListAuditEntriesPaginated` is called with limit=10, offset=0
- THEN 10 rows are returned and total equals 25

#### Scenario: Contributor filter narrows result set
- GIVEN rows for contributors "alice" (3 rows) and "bob" (2 rows)
- WHEN `ListAuditEntriesPaginated` is called with `AuditFilter{Contributor: "alice"}`, limit=10, offset=0
- THEN 3 rows are returned and total equals 3

#### Scenario: Time-range filter applies inclusive bounds
- GIVEN audit rows with varied `occurred_at` timestamps
- WHEN `AuditFilter.OccurredAtFrom` and `OccurredAtTo` are set to a specific range
- THEN only rows within that range (inclusive) are returned

#### Scenario: Empty result set returns zero total and no error
- GIVEN a filter that matches no rows
- WHEN `ListAuditEntriesPaginated` is called
- THEN an empty slice is returned, total is 0, and error is nil

---

### REQ-404: Mutation Push Handler Emits Audit on 409 Project-Paused

`handleMutationPush` MUST, on the HTTP 409 `sync-paused` rejection path, call `InsertAuditEntry` synchronously before writing the 409 response. The audit row MUST have `Action = "mutation_push"` and `Outcome = OutcomeRejectedProjectPaused`. On audit insert failure, the handler MUST log at WARN and MUST still return the 409 to the client.

#### Scenario: 409 path emits exactly one audit row
- GIVEN a paused project and a valid mutation push request
- WHEN `handleMutationPush` processes the request
- THEN the handler returns HTTP 409
- AND exactly one `cloud_sync_audit_log` row is inserted with `action='mutation_push'` and `outcome='rejected_project_paused'`

#### Scenario: Non-409 path emits zero audit rows
- GIVEN a project with sync enabled and a valid mutation push request
- WHEN `handleMutationPush` processes the request successfully
- THEN no `cloud_sync_audit_log` row is inserted

#### Scenario: Audit insert failure does not change HTTP response
- GIVEN a paused project and a DB error on `InsertAuditEntry`
- WHEN `handleMutationPush` processes the request
- THEN the handler still returns HTTP 409 with the `sync-paused` error body

---

### REQ-405: Legacy Chunk Push Handler Emits Audit on 409 Project-Paused

`handlePushChunk` MUST, on the HTTP 409 `sync-paused` rejection path, call `InsertAuditEntry` synchronously before writing the 409 response. The audit row MUST have `Action = "chunk_push"` and `Outcome = OutcomeRejectedProjectPaused`. On audit insert failure, the handler MUST log at WARN and MUST still return the 409.

#### Scenario: 409 path emits exactly one audit row
- GIVEN a paused project and a valid chunk push request
- WHEN `handlePushChunk` processes the request
- THEN the handler returns HTTP 409
- AND exactly one `cloud_sync_audit_log` row is inserted with `action='chunk_push'` and `outcome='rejected_project_paused'`

#### Scenario: Non-409 path emits zero audit rows
- GIVEN a project with sync enabled and a valid chunk push request
- WHEN `handlePushChunk` processes the request successfully
- THEN no `cloud_sync_audit_log` row is inserted

#### Scenario: Structural assertion failure logs and does not panic
- GIVEN a store type that does not implement `InsertAuditEntry`
- WHEN `handlePushChunk` reaches the 409 path
- THEN the handler logs the failure and returns HTTP 409
- AND no panic occurs

---

### REQ-406: Mutation Push Envelope Accepts Optional created_by

The mutation push request envelope MUST accept an optional `created_by` field. The field MUST be backward-compatible: clients that omit it MUST still receive a valid (non-error) response. When `created_by` is absent or empty, the persisted `contributor` value MUST be `"unknown"`.

#### Scenario: Request with created_by populates contributor correctly
- GIVEN a mutation push request body containing `"created_by": "alice"`
- WHEN `handleMutationPush` processes a paused-project rejection
- THEN the audit row has `contributor='alice'`

#### Scenario: Request without created_by defaults contributor to "unknown"
- GIVEN a mutation push request body with no `created_by` field
- WHEN `handleMutationPush` processes a paused-project rejection
- THEN the audit row has `contributor='unknown'`

#### Scenario: Older client omitting created_by receives normal response
- GIVEN a mutation push request body with no `created_by` field and sync enabled
- WHEN `handleMutationPush` processes the request
- THEN the handler returns HTTP 200 (or appropriate success code) without error

---

### REQ-407: Pull Path Emits No Audit (Negative Requirement)

`handleMutationPull` MUST NOT be modified to emit audit entries. The pull path MUST NOT gate on project pause status. Paused projects MUST continue to serve reads to enrolled contributors without restriction.

#### Scenario: Pull request against paused project succeeds without audit
- GIVEN a paused project with existing mutations
- WHEN `handleMutationPull` processes a pull request from an enrolled contributor
- THEN the handler returns the mutations successfully
- AND no `cloud_sync_audit_log` row is inserted

#### Scenario: Pull path code contains no InsertAuditEntry calls
- GIVEN the source of `handleMutationPull`
- WHEN reviewed
- THEN no call to `InsertAuditEntry` is present

---

### REQ-408: Admin Route /dashboard/admin/audit-log (Shell, Admin-Gated)

The system MUST register `GET /dashboard/admin/audit-log` as an admin-gated route. The route MUST render the `AdminAuditLogPage` templ component. Non-admin requests MUST be rejected with an appropriate HTTP error (401 or 403). The shell MUST include an HTMX-enabled container div that triggers loading of the `/dashboard/admin/audit-log/list` partial on page load.

#### Scenario: Admin user receives audit log shell page
- GIVEN an authenticated admin user
- WHEN a GET request is made to `/dashboard/admin/audit-log`
- THEN the response is HTTP 200 with the shell page HTML
- AND the page contains an element with `hx-get="/dashboard/admin/audit-log/list"` and `hx-trigger="load"`

#### Scenario: Non-admin user is denied access
- GIVEN an authenticated non-admin user
- WHEN a GET request is made to `/dashboard/admin/audit-log`
- THEN the response is HTTP 401 or HTTP 403

---

### REQ-409: Admin Route /dashboard/admin/audit-log/list (Partial, Admin-Gated, HTMX)

The system MUST register `GET /dashboard/admin/audit-log/list` as an admin-gated HTMX partial route. The route MUST render `AdminAuditLogListPartial` with the current filter and pagination state derived from query parameters. Non-admin requests MUST be rejected.

#### Scenario: Admin user receives paginated audit list partial
- GIVEN an authenticated admin user and audit rows in the database
- WHEN a GET request is made to `/dashboard/admin/audit-log/list`
- THEN the response is HTTP 200 with an HTML table partial containing audit rows

#### Scenario: Filter query parameters narrow the returned rows
- GIVEN audit rows for contributors "alice" and "bob"
- WHEN `GET /dashboard/admin/audit-log/list?contributor=alice` is requested by an admin
- THEN only rows with contributor "alice" are rendered in the partial

#### Scenario: Non-admin request to list partial is denied
- GIVEN an authenticated non-admin user
- WHEN a GET request is made to `/dashboard/admin/audit-log/list`
- THEN the response is HTTP 401 or HTTP 403

---

### REQ-410: Audit Log Filters — Contributor, Project, Outcome, OccurredAt Range

The audit list route MUST support filtering by: `contributor` (text match), `project` (text match), `outcome` (exact match from outcome vocabulary), `occurred_at_from` (lower bound, inclusive), `occurred_at_to` (upper bound, inclusive). All filters MUST be independently optional. Active filters MUST be preserved and reflected back in the rendered filter form.

#### Scenario: All filters may be combined
- GIVEN rows spanning multiple contributors, projects, outcomes, and timestamps
- WHEN `ListAuditEntriesPaginated` is called with all filter fields set
- THEN only rows satisfying all conditions are returned

#### Scenario: Outcome filter uses only valid vocabulary constants
- GIVEN the outcome dropdown in `AdminAuditLogListPartial`
- WHEN rendered
- THEN the options match the exported constants from `cloudstore/audit.go` (e.g., `OutcomeRejectedProjectPaused`)

---

### REQ-411: "Audit Log" Link Present in All Four Admin-Nav Blocks

The system MUST add an "Audit Log" navigation link (href="/dashboard/admin/audit-log") to all four `<div class="admin-nav">` blocks in `components.templ`: the nav blocks rendered by `AdminPage`, `AdminProjectsPage`, `AdminUsersPage`, and `AdminHealthPage`.

#### Scenario: Audit Log link present on AdminPage nav
- GIVEN a rendered `AdminPage`
- WHEN the HTML output is inspected
- THEN a link to `/dashboard/admin/audit-log` with text "Audit Log" is present in the admin-nav block

#### Scenario: Audit Log link present on all four admin pages
- GIVEN rendered output of AdminPage, AdminProjectsPage, AdminUsersPage, and AdminHealthPage
- WHEN each page's admin-nav block is inspected
- THEN all four contain a link to `/dashboard/admin/audit-log`

---

### REQ-412: Structural Interface Assertion for InsertAuditEntry

The push handlers MUST access `InsertAuditEntry` via a structural type assertion against an anonymous interface, NOT by extending `ChunkStore` or `MutationStore`. The assertion MUST have a safe-fallback path: if the assertion fails, the handler MUST log the failure and continue — it MUST NOT panic.

#### Scenario: Real store passes the structural assertion
- GIVEN a `CloudServer` initialized with a real `*CloudStore`
- WHEN the structural assertion `s.store.(interface{ InsertAuditEntry(ctx, AuditEntry) error })` is evaluated
- THEN the assertion succeeds (ok == true)

#### Scenario: Assertion failure triggers log-and-skip, not panic
- GIVEN a store mock that does not implement `InsertAuditEntry`
- WHEN the 409 path is reached and the assertion is evaluated
- THEN ok == false, a warning is logged, and the handler completes without panicking

---

### REQ-413: Outcome Constant Vocabulary and Action Discriminator

The system MUST define and export a single outcome constant for v1: `OutcomeRejectedProjectPaused = "rejected_project_paused"`. The `action` column MUST carry the push-type discriminator: `"mutation_push"` for mutation push rejections, `"chunk_push"` for chunk push rejections. The `outcome` column MUST NOT differentiate between push types. Both constants MUST be defined in `cloudstore/audit.go` and MUST be the single source of truth consumed by handlers, tests, and templ filter components.

#### Scenario: Both handlers write the same outcome constant
- GIVEN a paused project
- WHEN both `handleMutationPush` and `handlePushChunk` reject a request
- THEN both audit rows have `outcome='rejected_project_paused'`
- AND they differ only in the `action` column (`mutation_push` vs `chunk_push`)

#### Scenario: Outcome filter dropdown uses exported constants
- GIVEN the `AdminAuditLogListPartial` templ component
- WHEN the outcome filter dropdown is rendered
- THEN the option values match the exported `Outcome*` constants from `cloudstore/audit.go`

---

### REQ-414: Response Envelope Includes Project, Project Source, and Project Path

The mutation push response envelope MUST include `project`, `project_source`, and `project_path` fields consistent with the MCP project resolution conventions used elsewhere in the cloud server. These fields MUST be present on both successful and 409-rejected responses where the project was resolved before the rejection decision.

#### Scenario: 409 rejection response includes project fields
- GIVEN a paused project with a resolvable project path
- WHEN `handleMutationPush` returns a 409
- THEN the response body includes `project`, `project_source`, and `project_path` fields matching the resolved project

#### Scenario: Fields are consistent with non-rejection response
- GIVEN a non-paused project and a successful mutation push
- WHEN `handleMutationPush` returns success
- THEN the response body includes the same `project`, `project_source`, and `project_path` fields

---

## Test Seam Summary

| Layer | Test Type | Key Seams |
|-------|-----------|-----------|
| `cloudstore` | Unit (real DB) | DDL idempotency; `InsertAuditEntry` round-trip; `ListAuditEntriesPaginated` with every filter/pagination combination; `COUNT(*)` correctness |
| `cloudserver` handlers | Integration | 409 path emits 1 row (both handlers); non-409 path emits 0 rows; `created_by` present vs absent; structural assertion with real store (ok=true) and fake store (ok=false, no panic) |
| Dashboard routes | HTTP handler test | Admin gate (200 vs 401/403); shell contains HTMX attrs; partial renders filter + pagination; `parityStoreStub` updated |
| `components.templ` | Render test | "Audit Log" link present in all four admin-nav blocks |
| Outcome constants | Unit | `OutcomeRejectedProjectPaused` value matches string in DB rows; single source of truth consumed by filter dropdown |
