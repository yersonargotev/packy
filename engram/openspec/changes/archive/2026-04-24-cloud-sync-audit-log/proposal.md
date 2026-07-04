# Proposal: cloud-sync-audit-log

## Intent

When an admin pauses a project's sync, the cloud server currently rejects push attempts with HTTP 409 `sync-paused` but leaves NO server-side trace. Admins cannot answer "who tried to push what, when, and how often while paused?" — a blind spot for compliance, debugging, and operational visibility. This change introduces a persistent `cloud_sync_audit_log` table populated synchronously on every pause-rejection from both the mutation push and the legacy chunk push paths, plus an admin-only paginated dashboard UI to filter and browse entries. Contributor identity is captured via an optional `created_by` on the mutation push envelope (already present in chunk push). Contributor pause, audit retention/auto-prune, and accepted-outcome verbose logging are deferred.

## Scope

### IN

- `cloud_sync_audit_log` table — idempotent DDL appended to `cs.migrate()`, with indexes on `occurred_at DESC`, `(contributor, project)`, and `outcome`.
- Go types: `AuditEntry`, `AuditFilter`, `DashboardAuditRow` plus outcome constants (`OutcomeRejectedProjectPaused`, `OutcomeRejectedChunkPushPaused` — see Decision 5 for unification).
- `CloudStore.InsertAuditEntry(ctx, AuditEntry) error` — synchronous insert.
- `CloudStore.ListAuditEntriesPaginated(filter, limit, offset) ([]DashboardAuditRow, total int, err error)` — SQL `LIMIT/OFFSET + COUNT(*)`.
- Synchronous audit emission on HTTP 409 rejection in BOTH push handlers:
  - `handleMutationPush` (`internal/cloud/cloudserver/mutations.go`)
  - `handlePushChunk` (`internal/cloud/cloudserver/cloudserver.go`)
- Optional `created_by` field added to the mutation push request envelope — non-breaking, empty string logged as `"unknown"`.
- Structural type assertion boundary: `s.store.(interface{ InsertAuditEntry(ctx, AuditEntry) error })` in both push handlers. `ChunkStore` and `MutationStore` interfaces untouched.
- `DashboardStore.ListAuditEntriesPaginated` extension; `parityStoreStub` updated in `dashboard_test.go`.
- Admin routes:
  - `GET /dashboard/admin/audit-log` — shell page (admin-gated).
  - `GET /dashboard/admin/audit-log/list` — HTMX partial (admin-gated).
- Templ components: `AdminAuditLogPage(displayName, filter)` shell + `AdminAuditLogListPartial(entries, pg, filter)` table with filters (contributor, project, outcome, time-range-from, time-range-to) and `HtmxPaginationBar`.
- "Audit Log" admin-nav link added to ALL FOUR admin-nav blocks: `AdminPage`, `AdminProjectsPage`, `AdminUsersPage`, `AdminHealthPage`.
- Tests:
  - Store unit tests: DDL migration idempotency, insert round-trip, paginated list with every filter combination, COUNT correctness.
  - Handler tests: 409 path for both push handlers emits one audit row; non-409 path emits zero; mutation push with and without `created_by`.
  - Dashboard tests: route gating (admin-only), partial rendering with filters, pagination bar, parityStoreStub integration.
- Docs: DOCS.md audit-log section (schema, outcomes, retention caveat), ARCHITECTURE.md update noting the audit boundary and structural-interface pattern.

### OUT

- Contributor pause (deferred to future change).
- Automatic retention / pruning (documented as known v1 limitation; manual `DELETE FROM cloud_sync_audit_log WHERE occurred_at < NOW() - INTERVAL '90 days'` recommended).
- Audit entry detail page.
- JSONB `metadata` rendering in the dashboard UI (column reserved for future use).
- Verbose mode (logging accepted outcomes, not just rejections).
- Pull-path auditing (pull does not gate on pause; no rejection event to audit).
- Additional outcome vocabulary beyond project-paused rejections (`rejected_contributor_paused`, `rejected_unauthorized`, `accepted`).

## Approach

Layered, three-batch TDD sequence. Batch 1 unblocks Batches 2 and 3.

### Batch 1 — Persistence layer (`cloudstore`)

1. Append idempotent DDL to `cs.migrate()` queries slice (existing pattern in `cloudstore.go:427-492`).
2. Add Go types (`AuditEntry`, `AuditFilter`, `DashboardAuditRow`) and outcome constants in a new `cloudstore/audit.go`.
3. Implement `InsertAuditEntry` and `ListAuditEntriesPaginated` on `*CloudStore`.
4. Unit tests for DDL idempotency, insert, and every filter/pagination branch with a real sqlite/postgres test harness.

### Batch 2 — Server handlers + dashboard UI

1. In `handleMutationPush`: accept optional `created_by` in the request envelope; on 409 pause-rejection, perform structural type assertion and call `InsertAuditEntry` synchronously. Log contributor as `"unknown"` when empty.
2. In `handlePushChunk`: on 409 pause-rejection, same structural assertion + `InsertAuditEntry` using `req.CreatedBy`.
3. Extend `DashboardStore` interface with `ListAuditEntriesPaginated`; update `parityStoreStub`.
4. Register `GET /dashboard/admin/audit-log` and `GET /dashboard/admin/audit-log/list` routes (admin-gated, HTMX-driven).
5. Write `AdminAuditLogPage` shell and `AdminAuditLogListPartial` templ components, mirroring the `ContributorsPage` / `ContributorsListPartial` pattern (`hx-trigger="load"` on shell, partial loads via `hx-get`).
6. Add "Audit Log" nav link to ALL FOUR admin-nav blocks in `components.templ`.
7. Handler + templ tests; parity test against stub.

### Batch 3 — Documentation

1. `DOCS.md` audit-log section: schema description, outcome vocabulary, retention caveat, manual prune recipe.
2. `ARCHITECTURE.md`: note the structural-interface boundary (why we do NOT extend `ChunkStore`/`MutationStore`).

### Rationale

- Synchronous insert chosen over fire-and-forget: rejection path already performs a DB read for `IsProjectSyncEnabled`, so the marginal ~1ms of the additional INSERT is negligible, while silent audit drops are unacceptable for a compliance surface.
- Structural type assertion avoids polluting two narrow interfaces with an unrelated concern (audit). The cloud server already uses this pattern for the pause-gate itself — consistency.
- HTMX-driven shell + partial mirrors existing admin pages, zero new UI primitives needed.
- SQL pagination (not in-memory slicing) — the table grows unbounded with no retention, so `LIMIT/OFFSET + COUNT(*)` is non-negotiable.

## Architectural Decisions

### 1. Insert semantics: synchronous

**Decision**: `InsertAuditEntry` blocks the HTTP handler on the rejection path. No goroutine, no buffered channel.

**Tradeoffs**: +~1ms latency on an already-failing 409 path (acceptable). − one extra DB write per rejected push. + guaranteed durability — no dropped audit entries under load or panic.

**Alternative rejected**: fire-and-forget goroutine. Silent drops unacceptable for compliance-grade audit.

### 2. Interface boundary: structural type assertion

**Decision**: `s.store.(interface{ InsertAuditEntry(ctx, AuditEntry) error })` used in both push handlers. `ChunkStore` and `MutationStore` interfaces NOT extended.

**Tradeoffs**: + keeps narrow interfaces focused on their core concerns. + consistent with existing pause-gate pattern. − requires runtime assertion (zero cost in practice; Go inlines structural checks). − slightly less obvious to new readers; mitigated by doc comment at assertion site.

**Alternative rejected**: add `InsertAuditEntry` to both interfaces. Leaks audit concern into chunk and mutation abstractions; breaks interface-segregation principle.

### 3. Contributor identity in mutation push

**Decision**: add optional `created_by` field to the mutation push request envelope. Non-breaking (old clients omit it). Empty string logged as `"unknown"`.

**Tradeoffs**: + non-breaking wire contract. + mirrors chunk-push envelope (symmetry). − audit quality depends on client cooperation; older clients show `"unknown"`. Mitigated by updating the default CLI/agent to always send `created_by`.

**Alternative rejected**: derive contributor from auth token / claim. Would require plumbing auth context into `MutationEntry` iteration and is out of scope.

### 4. Pull path: no audit

**Decision**: `handleMutationPull` remains unchanged. Pausing gates push only; paused projects still serve reads to enrolled contributors.

**Tradeoffs**: + matches product semantics (pause = stop new writes, not reads). + no rejection event exists on pull, so no audit event exists either. − if product later adds pull-gating, we must revisit. Documented.

### 5. Outcome vocabulary — UNIFICATION

**Decision**: collapse `rejected_chunk_push_paused` into the single unified outcome `rejected_project_paused`. The existing `action` column (`mutation_push` vs `chunk_push`) distinguishes which code path produced the event.

**Rationale**: semantics are identical — the project was paused and a write was rejected. Having two outcomes for the same logical state forces the dashboard filter to offer a confusing choice. `(outcome, action)` is the correct composite key.

**Tradeoffs**: + single filter value in the UI. + clean semantic mapping. − requires the admin to combine `outcome + action` to distinguish mutation vs chunk rejections (acceptable; the table renders `action` as a column).

**Open question resolved**: v1 ships a single `OutcomeRejectedProjectPaused` constant. `action` column carries the push-type distinction.

### 6. Schema placement

**Decision**: idempotent DDL (`CREATE TABLE IF NOT EXISTS` + `CREATE INDEX IF NOT EXISTS`) appended to the existing `cs.migrate()` queries slice in `cloudstore.go`.

**Tradeoffs**: + zero new migration machinery. + consistent with how every other cloud table is managed. − cannot rename the table without a separate migration step later (accepted — table names are stable).

### 7. Pagination: SQL `LIMIT/OFFSET + COUNT(*)`

**Decision**: server-side SQL pagination with a separate `COUNT(*)` query per page for total.

**Tradeoffs**: + correct under unbounded table growth. + matches existing admin-pagination idioms. − `COUNT(*)` cost grows with table size. Mitigated by `idx_audit_log_outcome` and filter indexes; acceptable for admin UI usage patterns (low QPS).

**Alternative rejected**: in-memory slicing. Unacceptable — no retention means the table is unbounded.

### 8. Dashboard route pattern: shell + HTMX partial

**Decision**: shell page at `/dashboard/admin/audit-log` renders a container `<div hx-get="/dashboard/admin/audit-log/list" hx-trigger="load" hx-target="this" hx-swap="innerHTML">`. Partial at `/dashboard/admin/audit-log/list` renders the filtered, paginated table.

**Tradeoffs**: + identical to ContributorsPage / ContributorsListPartial; zero new patterns. + filter changes and pagination work via HTMX form swaps. − two handlers to maintain (acceptable; same as all other admin pages).

### 9. Admin nav coverage

**Decision**: add "Audit Log" link to ALL FOUR existing `<div class="admin-nav">` blocks in `components.templ` — `AdminPage`, `AdminProjectsPage`, `AdminUsersPage`, `AdminHealthPage`. Explicit checklist item in the Tasks phase.

**Tradeoffs**: + discoverability across every admin page. − four files drift risk. Mitigated by a single failing templ test that asserts the link is present in all four renderings.

### 10. Retention: none in v1

**Decision**: no automatic prune or retention policy. Table grows until manually truncated. Documented in DOCS.md with a suggested recipe: `DELETE FROM cloud_sync_audit_log WHERE occurred_at < NOW() - INTERVAL '90 days'`.

**Tradeoffs**: + zero new infrastructure. − operational burden on admins; potential storage growth for large deployments. Accepted because a) v1 cohort is small, b) outcomes are cheap rows, c) retention is well-scoped for a follow-up change.

### 11. `metadata JSONB`: reserved, not rendered

**Decision**: include the `metadata JSONB` column in the DDL but do NOT populate or render it in v1. Reserved for future payload-snippet capture or structured context.

**Tradeoffs**: + forward compatibility without a migration later. − column is dead weight in v1. Acceptable — JSONB cost on NULL is negligible.

**Alternative rejected**: omit now, add later. Adding a JSONB column to a large table later is disruptive; pay the cost upfront.

## Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Developer forgets one of the four admin-nav blocks | Broken navigation on one admin page | Explicit checklist in tasks; templ test asserts link presence in all four |
| Older clients push without `created_by` | `"unknown"` clutters audit | Ship the default agent/CLI with `created_by` populated; document as expected v1 behavior |
| Table growth under sustained rejection attacks | Disk usage, slow `COUNT(*)` | Document retention recipe; follow-up change adds auto-prune |
| Synchronous insert fails (DB transient error) | 409 rejection still returned but audit row missing | Log error at WARN; do NOT promote audit failure to a 5xx (rejection semantics must remain stable). Document the tradeoff |
| Structural interface assertion fails at runtime (wrong store type) | Compile passes, runtime panic on first rejection | Unit test exercises the assertion branch with the real store type; add a fallback log + skip if assertion fails (never panic on a rejection path) |
| `outcome` vocabulary drift between code and dashboard filter | UI shows stale options | Single source of truth: outcome constants exported from `cloudstore/audit.go`, consumed by both handlers and templ filter component |
| Pull path later gains pause-gating without audit | Regression | Documented in Decision 4; future change reviewers must revisit audit coverage |

## Success Criteria

1. Every HTTP 409 `sync-paused` rejection from `handleMutationPush` produces exactly one `cloud_sync_audit_log` row with `action='mutation_push'`, `outcome='rejected_project_paused'`.
2. Every HTTP 409 `sync-paused` rejection from `handlePushChunk` produces exactly one `cloud_sync_audit_log` row with `action='chunk_push'`, `outcome='rejected_project_paused'`.
3. Admin users can `GET /dashboard/admin/audit-log` and see a paginated, filterable table (contributor, project, outcome, time range). Non-admin users are rejected.
4. `go test ./...` passes on the full tree.
5. No measurable latency regression on non-rejection (success) push paths — audit code must not execute when pause is not active.
6. DDL is idempotent: running `cs.migrate()` multiple times produces no error and no duplicate indexes.
7. The mutation push request with `created_by` omitted still succeeds (non-breaking) and audits as `contributor='unknown'` when rejected.
8. "Audit Log" link appears in the admin-nav on AdminPage, AdminProjectsPage, AdminUsersPage, AND AdminHealthPage.
9. Documentation in DOCS.md and ARCHITECTURE.md accurately describes the audit schema, synchronous-insert rationale, structural-interface boundary, and retention caveat.
