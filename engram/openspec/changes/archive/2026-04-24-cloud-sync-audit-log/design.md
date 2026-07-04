# Design: cloud-sync-audit-log

## Context

- **Proposal**: `openspec/changes/cloud-sync-audit-log/proposal.md` — adds persistent audit log for pause-rejections + admin dashboard UI.
- **Exploration**: `openspec/changes/cloud-sync-audit-log/exploration.md` — maps the two push paths (`handleMutationPush`, `handlePushChunk`), the `DashboardStore` interface surface, and the four admin-nav blocks.
- **Spec**: to be authored in parallel; capability is `cloud-sync-audit`.

## Technical Approach

A single idempotent DDL block adds `cloud_sync_audit_log` inside `CloudStore.migrate()`. A new domain file `internal/cloud/cloudstore/audit_log.go` carries the types, exported outcome/action constants, and two methods: `InsertAuditEntry` (synchronous INSERT) and `ListAuditEntriesPaginated` (filtered LIMIT/OFFSET + COUNT). Both push handlers acquire the audit capability via a structural type assertion against `s.store`, mirroring the existing `IsProjectSyncEnabled` gate at `cloudserver.go:390-408` — neither `ChunkStore` nor `MutationStore` grows a new method. The dashboard layer extends `DashboardStore` with one new method, adds two admin-gated HTMX routes, two templ components appended to `components.templ`, and one new `adminNav(active string)` partial to replace the duplicated `<div class="admin-nav">` blocks in all four admin pages.

## Architecture Decisions

### 1. File organization — new file `cloudstore/audit_log.go`
**Choice**: new file with types + constants + CRUD; `cloudstore.go:migrate()` only grows by the DDL strings.
**Alternatives**: inline in `cloudstore.go`.
**Rationale**: matches `project_controls.go` domain-separation pattern already in the package; keeps `cloudstore.go` focused on connection/migration/chunks.

### 2. Handler file — inline in `dashboard.go`
**Choice**: add `handleAdminAuditLog` and `handleAdminAuditLogList` inside existing `dashboard.go` alongside the admin handlers (after `handleAdminHealth` ~line 842).
**Alternatives**: new `dashboard_audit.go`.
**Rationale**: integrated repo convention — all admin handlers currently co-locate in `dashboard.go`. A split would be inconsistent with the established admin-pages pattern.

### 3. Templ component placement — append to `components.templ`
**Choice**: append `AdminAuditLogPage(displayName, filter)`, `AdminAuditLogListPartial(entries, pg, filter)`, and a shared `adminNav(active string)` partial to `components.templ`, then regenerate `components_templ.go`.
**Alternatives**: new `components_audit.templ`.
**Rationale**: the file already contains every other admin component; domain split is not the project norm. A single `go generate` regenerates the `.go` file.

### 4. Insert helper signature — struct argument
**Choice**: `InsertAuditEntry(ctx context.Context, entry AuditEntry) error`.
**Alternatives**: positional fields.
**Rationale**: seven fields, some optional (`ReasonCode`, `Metadata`); struct is maintainable and matches `ProjectSyncControl` ergonomics. Callers construct a literal at the rejection site.

### 5. List filter shape — `AuditFilter` struct
**Choice**: `ListAuditEntriesPaginated(ctx, filter AuditFilter, limit, offset int) ([]DashboardAuditRow, int, error)`.
**Alternatives**: named string/time parameters.
**Rationale**: five optional filters (Contributor, Project, Outcome, OccurredAtFrom, OccurredAtTo); struct avoids parameter-list churn and matches the pagination precedents in the package.

### 6. Filter parsing in handler — RFC3339 + strings.TrimSpace, no extra sanitization
**Choice**: parse `from`/`to` query params with `time.Parse(time.RFC3339, v)`; text filters passed through `strings.TrimSpace`. All values flow into parameterized SQL (`$1`, `$2`, ...) — no string interpolation.
**Alternatives**: Unix seconds; regex sanitization.
**Rationale**: RFC3339 is human-readable, matches existing timestamp format (`ProjectSyncControl.UpdatedAt`). `database/sql` parameter binding handles injection; additional escaping is redundant.

### 7. Outcome constants — exported from `cloudstore`
**Choice**: package-level exported constants in `audit_log.go`:
- `AuditOutcomeRejectedProjectPaused = "rejected_project_paused"` (unified — Decision 5 in proposal)
- `AuditActionMutationPush = "mutation_push"`
- `AuditActionChunkPush = "chunk_push"`

**Rationale**: single source of truth consumed by both push handlers and the templ outcome-dropdown filter. Prevents vocabulary drift.

### 8. Structural assertion pattern — mirror `handlePushChunk` pause gate
**Choice**: at both push rejection sites:
```go
if auditor, ok := s.store.(interface {
    InsertAuditEntry(ctx context.Context, entry cloudstore.AuditEntry) error
}); ok {
    if err := auditor.InsertAuditEntry(r.Context(), cloudstore.AuditEntry{
        Contributor: contributor, Project: project,
        Action: cloudstore.AuditActionMutationPush,
        Outcome: cloudstore.AuditOutcomeRejectedProjectPaused,
        EntryCount: len(req.Entries), ReasonCode: "sync-paused",
    }); err != nil {
        log.Printf("cloudserver: audit insert failed: %v", err)
    }
}
```
**Rationale**: identical shape to `cloudserver.go:390-408`; never panic on rejection path; audit failure degrades to a log line.

### 9. Admin nav — factor to `adminNav(active string)` partial
**Choice**: extract a single `templ adminNav(active string)` component and replace the four inline `<div class="admin-nav">` blocks in `AdminPage`, `AdminProjectsPage`, `AdminUsersPage`, `AdminHealthPage`. Add the "Audit Log" link once, in the partial.
**Alternatives**: update each of the four inline blocks mechanically.
**Rationale**: the blocks are identical except for which anchor carries `class="active"`; a partial keyed on `active` removes drift risk permanently (this bug will recur on every future admin page). Templ test asserts the `Audit Log` link renders in all four shells after the refactor.

### 10. Test strategy
| Layer | Test | Approach |
|-------|------|----------|
| cloudstore (Postgres) | `audit_log_test.go` | Skip if `CLOUDSTORE_TEST_DSN` empty; DDL idempotency (`migrate` twice), insert round-trip, list with each filter combo, COUNT correctness, pagination bounds. |
| cloudserver | `mutations_test.go`, `cloudserver_test.go` | Extend `fakeStore` helper with `InsertAuditEntry(ctx, AuditEntry) error` that records calls to a slice; assert exactly one call on 409, zero on success; mutation push with/without `created_by`. |
| dashboard | `dashboard_test.go` | Extend `parityStoreStub` with `ListAuditEntriesPaginated`; route gating (admin-only 403), partial rendering with filters, pagination bar, `adminNav` asserts "Audit Log" link in all four shells. |
| integration | `cloudserver_test.go` | Full stack: 409 push → `GET /dashboard/admin/audit-log/list` returns the logged row. |

### 11. Date range filter shape — RFC3339
**Choice**: `from`/`to` query params as RFC3339 strings (e.g. `2026-04-24T00:00:00Z`). Empty = no bound.
**Rationale**: see Decision 6 — consistent with existing timestamp handling; form inputs use `<input type="datetime-local">` with a helper that formats to RFC3339 before submit.

## Data Flow

```
Client push (mutation or chunk)
    │
    ▼
handleMutationPush / handlePushChunk
    │
    ▼
IsProjectSyncEnabled(project) ──► enabled? ──yes──► normal path
    │ no
    ▼
writeActionableError 409 "sync-paused"
    │
    ▼ (NEW)
s.store.(interface{ InsertAuditEntry }) ──ok──► CloudStore.InsertAuditEntry
                                                        │
                                                        ▼
                                              cloud_sync_audit_log (INSERT)

Admin
    │
    ▼
GET /dashboard/admin/audit-log (shell)
    │ hx-trigger=load
    ▼
GET /dashboard/admin/audit-log/list?contributor=&project=&outcome=&from=&to=&page=&pageSize=
    │
    ▼
DashboardStore.ListAuditEntriesPaginated(filter, limit, offset)
    │
    ▼
AdminAuditLogListPartial(rows, pg, filter)
```

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/cloud/cloudstore/audit_log.go` | Create | `AuditEntry`, `AuditFilter`, `DashboardAuditRow`, outcome/action constants, `InsertAuditEntry`, `ListAuditEntriesPaginated`. |
| `internal/cloud/cloudstore/cloudstore.go` | Modify | Append `cloud_sync_audit_log` DDL + three `CREATE INDEX IF NOT EXISTS` to the `queries` slice in `migrate()`. |
| `internal/cloud/cloudstore/audit_log_test.go` | Create | Postgres-gated store tests (DDL idempotency, insert, filter matrix, pagination). |
| `internal/cloud/cloudserver/mutations.go` | Modify | Add optional `created_by` to the push request envelope; emit audit on 409 via structural assertion. |
| `internal/cloud/cloudserver/cloudserver.go` | Modify | Emit audit on `handlePushChunk` 409 via structural assertion. |
| `internal/cloud/cloudserver/mutations_test.go` | Modify | Extend `fakeStore` with `InsertAuditEntry`; cover 409 emission and success no-op. |
| `internal/cloud/cloudserver/cloudserver_test.go` | Modify | Chunk-push 409 emission test + end-to-end `GET /dashboard/admin/audit-log/list` returns the row. |
| `internal/cloud/dashboard/dashboard.go` | Modify | Extend `DashboardStore` with `ListAuditEntriesPaginated`; register two routes; add `handleAdminAuditLog` + `handleAdminAuditLogList`. |
| `internal/cloud/dashboard/components.templ` | Modify | Append `AdminAuditLogPage`, `AdminAuditLogListPartial`, `adminNav(active)`; replace four inline `admin-nav` blocks with `@adminNav(...)`. |
| `internal/cloud/dashboard/components_templ.go` | Regenerate | `go generate ./internal/cloud/dashboard/...` after templ edits. |
| `internal/cloud/dashboard/dashboard_test.go` | Modify | Extend `parityStoreStub` with audit stub; add route-gating, partial-rendering, admin-nav-link-present-in-all-four tests. |
| `DOCS.md` | Modify | Audit log section: schema, outcomes, retention caveat, manual prune recipe. |
| `docs/ARCHITECTURE.md` | Modify | Note the structural-interface boundary (why `ChunkStore`/`MutationStore` are NOT extended). |

## Interfaces / Contracts

```go
// internal/cloud/cloudstore/audit_log.go
const (
    AuditOutcomeRejectedProjectPaused = "rejected_project_paused"
    AuditActionMutationPush           = "mutation_push"
    AuditActionChunkPush              = "chunk_push"
)

type AuditEntry struct {
    Contributor string
    Project     string
    Action      string // use AuditAction* constants
    Outcome     string // use AuditOutcome* constants
    EntryCount  int
    ReasonCode  string
    Metadata    map[string]any // marshalled to JSONB; nil → '{}'
}

type AuditFilter struct {
    Contributor    string
    Project        string
    Outcome        string
    OccurredAtFrom time.Time // zero value = no lower bound
    OccurredAtTo   time.Time // zero value = no upper bound
}

type DashboardAuditRow struct {
    ID          int64
    OccurredAt  string // RFC3339 UTC
    Contributor string
    Project     string
    Action      string
    Outcome     string
    EntryCount  int
    ReasonCode  string
}

func (cs *CloudStore) InsertAuditEntry(ctx context.Context, entry AuditEntry) error
func (cs *CloudStore) ListAuditEntriesPaginated(ctx context.Context, filter AuditFilter, limit, offset int) ([]DashboardAuditRow, int, error)
```

```go
// internal/cloud/dashboard/dashboard.go — DashboardStore extension
ListAuditEntriesPaginated(filter cloudstore.AuditFilter, limit, offset int) ([]cloudstore.DashboardAuditRow, int, error)
```

Mutation push envelope (non-breaking):
```go
type mutationPushRequest struct {
    Entries   []cloudstore.MutationEntry `json:"entries"`
    CreatedBy string                     `json:"created_by,omitempty"` // NEW
}
```

## Testing Strategy

- **Unit (no DB)**: handler 409 path with `fakeStore` records exactly one `InsertAuditEntry` call; success path records zero. Filter parsing in `handleAdminAuditLogList` (RFC3339 round-trip, empty values, trim).
- **Integration (Postgres-gated)**: `cloudstore` tests skip when `CLOUDSTORE_TEST_DSN` unset (existing pattern at `project_controls_test.go`). Cover DDL idempotency, insert, every filter combination, `COUNT(*)` correctness, pagination edge cases.
- **End-to-end**: in `cloudserver_test.go`, boot a real `CloudStore` + dashboard handler, push against a paused project, `GET /dashboard/admin/audit-log/list`, assert the row renders.
- **Templ**: test asserts `Audit Log` anchor is present in `AdminPage`, `AdminProjectsPage`, `AdminUsersPage`, `AdminHealthPage` renderings after `adminNav` refactor.

## Rollout / Batch Plan

Three batches per proposal, strict TDD inside each.

**Batch 1 — Persistence (blocks 2 & 3)**
- DDL in `migrate()`, `audit_log.go`, unit/integration tests. Tests must fail first.
- Deliverable: `InsertAuditEntry` + `ListAuditEntriesPaginated` green against Postgres.

**Batch 2 — Handlers + Dashboard UI**
- Mutation push `created_by` envelope extension + structural assertion emission.
- Chunk push structural assertion emission.
- `DashboardStore` extension + `parityStoreStub` update.
- Two admin routes, two handlers.
- `adminNav` partial + `AdminAuditLogPage` + `AdminAuditLogListPartial`; `go generate` regen.
- Handler, dashboard, and templ tests (all four admin-nav shells).
- End-to-end integration test.

**Batch 3 — Docs**
- `DOCS.md` audit-log section.
- `docs/ARCHITECTURE.md` structural-interface boundary note.

## Open Questions

- [ ] Should the admin list default to the last 7 days or "all time"? Proposal did not constrain; design recommends "all time" for v1 with UI pre-filled `to` = now, `from` empty — revisit with first user feedback.
- [ ] Should the mutation push audit `entry_count` equal `len(req.Entries)` or the number of distinct paused projects in the batch? Current design uses `len(req.Entries)` (one row per rejected batch, full size). Spec phase should pin this.

## References

- Proposal: `openspec/changes/cloud-sync-audit-log/proposal.md`
- Exploration: `openspec/changes/cloud-sync-audit-log/exploration.md`
- Pause-gate pattern: `internal/cloud/cloudserver/cloudserver.go:387-408`
- Domain-separation precedent: `internal/cloud/cloudstore/project_controls.go`
- Admin handler precedent: `internal/cloud/dashboard/dashboard.go:624-842`
- Admin templ precedent: `internal/cloud/dashboard/components.templ` — `AdminPage`/`AdminProjectsPage`/`AdminUsersPage`/`AdminHealthPage`
