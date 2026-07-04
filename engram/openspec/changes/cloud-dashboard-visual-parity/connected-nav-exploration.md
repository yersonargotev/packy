# Connected Navigation Gap Exploration

**Change**: cloud-dashboard-visual-parity (gap remediation)
**Phase**: sdd-explore (follow-up)
**Scope classification**: **Batch 6 extension** (11 concrete work items)

---

## Problem

The `cloud-dashboard-visual-parity` change passed verify (PASS WITH WARNINGS) but the user reported that after deployment the dashboard lacks connected-navigation UX:

1. Observation/Session/Prompt rows/cards are NOT clickable — cannot drill into detail from list views.
2. Session detail does NOT link to the session's observations and prompts.
3. Contributor detail page does NOT render sessions/observations/prompts for the contributor.
4. Admin project pause toggle is not visible/interactive from `/dashboard/admin/projects`.
5. Type pills are not sourced from DB (hardcoded/empty).
6. Paused badge does not appear in project cards.

## Root Cause — Handler wiring, not component gap

**The `.templ` components for ALL detail pages are FULLY IMPLEMENTED and correct.** The gap is entirely in Go handler wiring:

- Three browser tab handlers (`handleBrowserObservations`, `handleBrowserSessions`, `handleBrowserPrompts`) render raw HTML builders instead of calling the existing templ components. The templ components have correct `<a href="/dashboard/..."}` links; the raw HTML builders don't.
- `handleContributorDetail` renders a stub raw HTML instead of calling `ContributorDetailPage` templ (which is fully implemented).
- `handleAdminProjects` delegates to `handleProjects` (which never renders admin toggles) instead of a dedicated handler rendering `AdminProjectsPage`.
- `handleProjectsList` passes `nil` controls to `ProjectsListPartial`, so the Paused badge conditional (`!projectControl(controls, s.Project).SyncEnabled`) always defaults to true and never renders.
- `handleBrowser` passes an empty `obsTypes []string` slice to `BrowserPage` because no `ListDistinctTypes` method exists in the store.
- `handleProjectDetail` renders raw HTML instead of calling `ProjectDetailPage` templ and doesn't fetch the sync control record, so pause audit never appears.

## Feature-by-feature current state

| # | Feature | Status | Root cause |
|---|---|---|---|
| a | Observation card clickable link | MISSING | `renderObservationsTable()` has no href; `ObservationsPartial` templ not called from browser handler |
| b | Session row link to detail | MISSING | `handleBrowserSessions` raw HTML; `SessionsPartial` not called |
| c | Prompt row link to detail | MISSING | `handleBrowserPrompts` raw HTML; `PromptsPartial` not called |
| d | Session detail → observations sub-list | IMPLEMENTED | `SessionDetailPage` calls `ObservationsPartial` |
| e | Session detail → prompts sub-list | IMPLEMENTED | `SessionDetailPage` calls `PromptsPartial` |
| f | Observation detail back-link + related | IMPLEMENTED | `ObservationDetailPage` fully wired |
| g | Contributor row → `/dashboard/contributors/{name}` | IMPLEMENTED | `ContributorsPage` renders hrefs |
| h | Contributor detail with sessions/obs/prompts | PARTIAL | `ContributorDetailPage` templ exists and correct; handler uses stub raw HTML; `GetContributorDetail` query does not exist |
| i | Project card shows Paused badge | PARTIAL | Templ renders badge when controls passed; `handleProjectsList` passes `nil` controls so badge never shows |
| j | Project detail shows pause audit | PARTIAL | `ProjectDetailPage` templ is wired; `handleProjectDetail` renders raw HTML and doesn't fetch sync control |
| k | Admin projects toggle POST via hx-post | PARTIAL | `AdminProjectsPage` + POST handler work; `/dashboard/admin/projects` route calls `handleProjects` not a dedicated admin-controls handler |
| l | Admin toggle reason field | PARTIAL | Blocked by (k) |
| m | Type pills sourced from DB DISTINCT types | MISSING | `ListDistinctTypes` doesn't exist; `obsTypes` always empty |
| n | Active type filter preserved on project change | PARTIAL | Htmx wiring is correct; blocked by (m) — pills don't exist |
| o | Pagination on Tier 1/2/3 | PARTIAL | Tier 2 and 3 correct; Tier 1 browser handlers use non-paginated store calls and no `HtmxPaginationBar` |

## Cloudstore gap — two missing queries

### `GetContributorDetail`
```go
func (cs *CloudStore) GetContributorDetail(name string) (DashboardContributorRow, []DashboardSessionRow, []DashboardObservationRow, []DashboardPromptRow, error)
```
Implementation: purely in-memory scan of `dashboardReadModel` for rows where `CreatedBy == name`. No new DB query.

### `ListDistinctTypes`
```go
func (cs *CloudStore) ListDistinctTypes() ([]string, error)
```
Implementation: scan `dashboardReadModel.projectDetails` for all `Observation.Type` values, collect into sorted set, return. Purely in-memory.

Both methods must be added to `DashboardStore` interface and `parityStoreStub` accordingly.

## Templ gap — NONE (components all exist)

All templ components exist and are correct. The work is handler wiring.

| Component | Status | Fix target |
|---|---|---|
| `SessionDetailPage` | IMPLEMENTED | Reachable once (b) fixed |
| `ObservationDetailPage` | IMPLEMENTED | Reachable once (a) fixed |
| `PromptDetailPage` | IMPLEMENTED | Reachable once (c) fixed |
| `ContributorDetailPage` | IMPLEMENTED | Handler fix in (h) |
| `AdminProjectsPage` | IMPLEMENTED | Handler fix in (k) |
| `ProjectDetailPage` | IMPLEMENTED | Handler fix in (j) |
| Type-pills strip in `BrowserPage` | IMPLEMENTED (conditioned) | Populate `obsTypes` in `handleBrowser` |
| Pause badge in `ProjectsListPartial` | IMPLEMENTED (conditioned) | Pass controls map in `handleProjectsList` |

## Link helpers — NOT a blocker

URL construction is correctly inlined in the templ components (`/dashboard/observations/{o.Project}/{o.SessionID}/{o.ChunkID}` etc.). Adding `observationDetailURL`, `sessionDetailURL`, `contributorDetailURL` helpers would be a polish improvement but is NOT the root cause.

## Scope classification

| Category | Count |
|---|---|
| Cloudstore methods to add | 2 |
| `DashboardStore` interface additions | 2 |
| `parityStoreStub` stubs to add | 2 |
| Handlers to fix | 6 (`handleBrowserObservations/Sessions/Prompts`, `handleContributorDetail`, `handleProjectsList`, `handleAdminProjects`, `handleProjectDetail`, `handleBrowser`) |
| Test seams to add | ≥ 6 RED tests |
| **Total work items** | **11** |

**Threshold**: ≤ 12 items → Batch 6 extension of existing change. No new change needed.

## Known UX decision needed before apply

`handleBrowserSessionDetail` currently handles `GET /dashboard/browser/sessions/{sessionID}` (sessionID only) via search redirect. The new `GetSessionDetail` method requires both project AND sessionID (composite key). Options:

- **A**: Keep the browser session URL (sessionID only) as a search redirect; rely on new direct links at `/dashboard/sessions/{project}/{sessionID}` emitted by `SessionsPartial`.
- **B**: Rewrite `handleBrowserSessionDetail` to parse project from query param (`?project=X`) and call `GetSessionDetail`.

**Decision (for Batch 6)**: Option A. The `SessionsPartial` template already emits the composite URL directly; the legacy browser URL remains as a search redirect for backward compatibility. No extra handler work needed.

## Recommended TDD implementation sequence (15 steps)

1. **RED**: `TestGetContributorDetailReturnsScopedData` in `dashboard_queries_test.go` — seed read model, call method, assert slices filtered by contributor's projects.
2. **GREEN**: Implement `GetContributorDetail` in `dashboard_queries.go`.
3. **RED**: `TestListDistinctTypesScansReadModel` — seed read model with observations of 3 types (one empty), assert sorted distinct non-empty set.
4. **GREEN**: Implement `ListDistinctTypes` in `dashboard_queries.go`.
5. Add both methods to `DashboardStore` interface (`dashboard.go`) + `parityStoreStub` stubs (`dashboard_test.go`).
6. **RED**: `TestContributorDetailPageRendersDrillDown` — assert response contains `CONTRIBUTOR DETAIL`, `Recent Sessions`, `Recent Observations`.
7. **GREEN**: Rewrite `handleContributorDetail` to call `GetContributorDetail` + render `ContributorDetailPage` templ.
8. **RED**: `TestBrowserObservationsAreClickable`, `TestBrowserSessionsAreClickable`, `TestBrowserPromptsAreClickable` — assert response body contains `href="/dashboard/observations/"` etc.
9. **GREEN**: Rewrite `handleBrowserObservations` to call `ListRecentObservationsPaginated` + render `ObservationsPartial` + `HtmxPaginationBar`.
10. **GREEN**: Same for `handleBrowserSessions` + `SessionsPartial`.
11. **GREEN**: Same for `handleBrowserPrompts` + `PromptsPartial`.
12. **RED**: `TestBrowserTypePillsSourcedFromStore` — assert response contains type pill elements after seeding read model with varied types.
13. **GREEN**: Modify `handleBrowser` to call `ListDistinctTypes()`, pass result to `BrowserPage`.
14. **RED**: `TestProjectCardShowsPausedBadge` — seed sync control with `SyncEnabled=false`, assert `ProjectsListPartial` output contains `Paused`.
15. **GREEN**: Modify `handleProjectsList` to call `ListProjectSyncControls()` and pass map to `ProjectsListPartial`.
16. **RED**: `TestAdminProjectsPageRendersToggles` — assert `/dashboard/admin/projects` contains `hx-post` toggle form.
17. **GREEN**: Add dedicated `handleAdminProjectControls` that renders `AdminProjectsPage` with controls; rewire route.
18. **RED**: `TestProjectDetailShowsPauseAudit` — seed paused control, assert `ProjectDetailPage` renders `reason` + `updated_by`.
19. **GREEN**: Rewrite `handleProjectDetail` to call `ProjectDetailPage` templ + fetch sync control.
20. **REFACTOR**: Scan for residual raw-HTML render paths where templ equivalents exist; remove.
21. **GREEN**: `go test ./...` full suite PASS.

## Risks

- **`handleProjectDetail` raw-HTML → `ProjectDetailPage` switch** alters rendered text tokens — existing tests that assert on literal strings like `"Project: proj-a"` may need updates to match templ output `"PROJECT DETAIL"`. Low-medium risk; fixable.
- **`parityStoreStub` extension** must add two methods before interface assertion compiles — mechanical.
- **No templ regeneration** needed — all `.templ` files stay unchanged. No `go:generate` invocation.
- **No `go.mod` changes** — purely Go code additions.
