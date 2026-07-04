# Exploration: cloud-sync-audit-log

## Problem

When an admin pauses a project's sync, push attempts are currently rejected with HTTP 409 `sync-paused` but the rejection leaves NO trace on the server. Admins have no way to see which contributors were trying to push, when, against which projects, or how often. This change adds a persistent audit log + admin UI surface.

Contributor pause is explicitly DEFERRED to a future change (per user instruction).

## Pause-rejection code paths (must all emit audit entries)

### Path 1 — Mutation push
**File**: `internal/cloud/cloudserver/mutations.go` — `handleMutationPush` ~lines 101-119
- Calls `ms.IsProjectSyncEnabled(project)` via `MutationStore` interface
- On pause: `writeActionableError(w, 409, UpgradeErrorClassPolicy, "sync-paused", ...)`
- **GAP**: `MutationEntry` struct has `{Project, Entity, EntityKey, Op, Payload}` — NO `CreatedBy` field. Contributor identity is NOT available in the current handler.
- **Solution**: add optional `created_by` field to the mutation-push request envelope (mirrors chunk-push pattern, non-breaking).

### Path 2 — Legacy chunk push
**File**: `internal/cloud/cloudserver/cloudserver.go` — `handlePushChunk` ~lines 387-408
- Structural type assertion `s.store.(interface{ IsProjectSyncEnabled(string) (bool, error) })`
- On pause: same `writeActionableError` 409
- Contributor identity: available as `req.CreatedBy` from request body.

### Pull path — NO rejection, NO audit needed
`handleMutationPull` does not gate on pause. Paused projects still serve existing mutations to enrolled contributors (intentional: pausing stops new pushes, not reads).

## Schema & migration

Idempotent DDL appended to `cs.migrate()` queries slice (existing pattern in `cloudstore.go:427-492`):

```sql
CREATE TABLE IF NOT EXISTS cloud_sync_audit_log (
    id BIGSERIAL PRIMARY KEY,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    contributor TEXT NOT NULL,
    project TEXT NOT NULL,
    action TEXT NOT NULL,              -- 'mutation_push' | 'chunk_push'
    outcome TEXT NOT NULL,             -- 'rejected_project_paused' (v1); extensible
    entry_count INTEGER NOT NULL DEFAULT 0,
    reason_code TEXT,
    metadata JSONB
);
CREATE INDEX IF NOT EXISTS idx_audit_log_occurred_at ON cloud_sync_audit_log(occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_log_contributor_project ON cloud_sync_audit_log(contributor, project);
CREATE INDEX IF NOT EXISTS idx_audit_log_outcome ON cloud_sync_audit_log(outcome);
```

No auto-prune in v1 — accepted tradeoff, document as known limitation.

## Insert helper

**Signature**: `InsertAuditEntry(ctx context.Context, entry AuditEntry) error` on `*CloudStore`

**Where called**: inside both push handlers' 409 rejection branches.

**Semantics**: SYNCHRONOUS. Rejection path already does a DB read for `IsProjectSyncEnabled` — adding one more write is ~1ms marginal. Silent audit drops are unacceptable for a compliance log; fire-and-forget rejected.

**Integration pattern**: structural interface assertion `s.store.(interface{ InsertAuditEntry(ctx, AuditEntry) error })` — same pattern used for the pause-gate itself. Keeps `ChunkStore` and `MutationStore` interfaces clean.

## Query helper

**Signature**: `ListAuditEntriesPaginated(filter AuditFilter, limit, offset int) (entries []DashboardAuditRow, total int, err error)`

**Filter fields**: `Contributor`, `Project`, `Outcome`, `OccurredAtFrom`, `OccurredAtTo` — all optional.

**Pagination**: SQL `LIMIT/OFFSET` + `COUNT(*)` (NOT in-memory slicing — audit table grows unbounded).

**Sort**: `occurred_at DESC`.

## Dashboard integration

**Routes**:
- `GET /dashboard/admin/audit-log` — admin-gated, shell page with `<div id="audit-log-content" hx-get="/dashboard/admin/audit-log/list" hx-trigger="load" hx-target="this" hx-swap="innerHTML">`
- `GET /dashboard/admin/audit-log/list` — admin-gated, HTMX partial with filters + `HtmxPaginationBar`

Mirror the `ContributorsPage` + `ContributorsListPartial` pattern.

**Templ components**:
- `AdminAuditLogPage(displayName, filter)` — full shell wrapper
- `AdminAuditLogListPartial(entries, pg, filter)` — table + pagination

**Filters in form**: contributor input, project input, outcome dropdown, time-range-from, time-range-to.

**Admin nav**: add "Audit Log" link to ALL FOUR existing `<div class="admin-nav">` blocks in `components.templ` (`AdminPage`, `AdminProjectsPage`, `AdminUsersPage`, `AdminHealthPage`). Easy to forget one.

**DashboardStore interface**: add `ListAuditEntriesPaginated`. `parityStoreStub` in `dashboard_test.go` must stub it too.

## Outcome vocabulary (v1)

```go
const (
    OutcomeRejectedProjectPaused = "rejected_project_paused"
    OutcomeRejectedChunkPushPaused = "rejected_chunk_push_paused" // may collapse into above if semantics match
)
```

Future outcomes (deferred): `rejected_contributor_paused`, `rejected_unauthorized`, `accepted` (verbose mode).

Outcome is a string column, not an enum at DB level — extensible without migration.

## Risks

1. `MutationEntry` has no `CreatedBy` — solved by optional request-envelope field.
2. Four admin-nav blocks need the new link — apply agent must be told explicitly about all four.
3. Audit table unbounded growth — accepted v1 tradeoff; document in DOCS.md.
4. JSONB `metadata` column: do NOT render in the dashboard table for v1 (detail page out of scope).
5. Synchronous insert on push-rejection path adds ~1ms latency — acceptable.

## Recommended 3-batch TDD sequence

- **Batch 1**: `cloudstore` — DDL migration + types (`AuditEntry`, `DashboardAuditRow`, `AuditFilter`) + outcome constants + `InsertAuditEntry` + `ListAuditEntriesPaginated` + tests.
- **Batch 2**: `cloudserver` handler integration (both push paths emit audit on 409, optional `created_by` on mutation push) + dashboard (DashboardStore extension, route registration, handlers, templ components, admin nav links everywhere, parityStoreStub) + tests.
- **Batch 3**: docs (DOCS.md audit log section, ARCHITECTURE.md).
