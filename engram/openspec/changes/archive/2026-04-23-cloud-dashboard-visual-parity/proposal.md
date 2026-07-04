# Proposal — cloud-dashboard-visual-parity

## Intent

Restore full visual and interaction parity between the integrated engram dashboard (`internal/cloud/dashboard/`) and the deployed `work/engram-cloud` dashboard WITHOUT changing the auth model (`ENGRAM_CLOUD_TOKEN` + signed `engram_dashboard_token` cookie + synthetic Principal), the sync write contract, or the chunk-centric cloudstore boundary. The previous change `cloud-dashboard-parity` validated routes and auth redirects but left static assets as byte-size placeholders and `*.templ` files as 23–63 line stubs; the deployed dashboard shows a raw Pico-CSS fallback shell with no HTMX nav, no status ribbon, no footer, no project tabs, no admin toggles. This change completes the port verbatim (95 %) with surgical type-system adaptation (4 %) and a small amount of new glue (1 %) for the auth bridge, pagination, and the SystemHealth/ProjectSyncControl surfaces the integrated cloudstore does not yet expose.

## Scope

### IN scope

**Static assets (verbatim REPLACE, direct copy from `/Users/alanbuscaglia/work/engram-cloud/internal/cloud/dashboard/static/`)**
- `internal/cloud/dashboard/static/htmx.min.js` (42 B stub → 50 917 B real)
- `internal/cloud/dashboard/static/pico.min.css` (46 B stub → 71 072 B real)
- `internal/cloud/dashboard/static/styles.css` (2 284 B thin → 23 185 B full)

**Templ component surface (REPLACE + surgical adapt)**
- `internal/cloud/dashboard/components.templ` (23 L → ~1 134 L, import path adapted, flat-row types extended)
- `internal/cloud/dashboard/layout.templ` (37 L → ~52 L with `status-ribbon`, `shell-footer`, `NavTabs`, `brand-subtitle`, `user-info`)
- `internal/cloud/dashboard/login.templ` (38 L → ~63 L with sidepanel copy + `next` hidden input)
- Corresponding `components_templ.go`, `layout_templ.go`, `login_templ.go` (generated + checked in)

**Helpers (REPLACE → port from legacy, 9 L → ~424 L)**
- `internal/cloud/dashboard/helpers.go` (pagination, formatTimestamp, truncateContent, typePillClass, renderStructuredContent, controlsByProject, projectControlReasonValue, etc.)

**Handlers (SURGICAL EDIT, not rewrite)**
- `internal/cloud/dashboard/dashboard.go`: switch `renderLoginPage` / `renderLayout` / `renderHTML` raw-string calls to templ component calls; register missing routes; add `h.cfg.GetDisplayName(r)` bridge; feed new paginated store methods; add detail-page handlers.

**Auth bridge (additive MountConfig field)**
- `internal/cloud/dashboard/dashboard.go` — add `MountConfig.GetDisplayName func(r *http.Request) string`, default `"OPERATOR"` when unset.
- `internal/cloud/cloudserver/cloudserver.go` — wire `GetDisplayName` in the single `dashboard.Mount` call site. Return `"OPERATOR"` until the session codec surfaces a display name (out of scope for this change).

**Cloudstore read-model extensions (ADDITIVE, no chunk-write changes)**
- `internal/cloud/cloudstore/dashboard_queries.go` — extend `DashboardObservationRow` with `Content string`, `TopicKey string`, `ToolName string`; extend `DashboardSessionRow` with `EndedAt string`, `Summary string`, `Directory string`; extend `DashboardPromptRow` with `ID string`. All fields are materializable from the existing chunk JSON payloads already read by `buildDashboardReadModel`.
- Add paginated query methods (in-memory slicing of the read model):
  - `ListProjectsPaginated(query string, limit, offset int) ([]DashboardProjectRow, int, error)`
  - `ListRecentObservationsPaginated(project, query, obsType string, limit, offset int) ([]DashboardObservationRow, int, error)`
  - `ListRecentSessionsPaginated(project, query string, limit, offset int) ([]DashboardSessionRow, int, error)`
  - `ListRecentPromptsPaginated(project, query string, limit, offset int) ([]DashboardPromptRow, int, error)`
  - `ListContributorsPaginated(query string, limit, offset int) ([]DashboardContributorRow, int, error)`
  - `GetSessionDetail(project, sessionID string) (DashboardSessionRow, []DashboardObservationRow, []DashboardPromptRow, error)`
  - `GetObservationDetail(project, sessionID, chunkID string) (DashboardObservationRow, DashboardSessionRow, []DashboardObservationRow, error)`
  - `GetPromptDetail(project, sessionID, chunkID string) (DashboardPromptRow, DashboardSessionRow, []DashboardPromptRow, error)`
- Add `SystemHealth() (DashboardSystemHealth, error)` — new small struct: `{DBConnected bool, Projects, Contributors, Sessions, Observations, Prompts, Chunks int}`. Sourced from the existing read model + a `cs.db.Ping()` style check.
- Add project sync controls (NEW persistence — mirror legacy SQL table in integrated DB):
  - `ListProjectSyncControls() ([]ProjectSyncControl, error)`
  - `GetProjectSyncControl(project string) (*ProjectSyncControl, error)`
  - `SetProjectSyncEnabled(project string, enabled bool, updatedBy, reason string) error`
  - `IsProjectSyncEnabled(project string) (bool, error)` — queried by push handler guard (pull path unchanged).
  - Schema: `CREATE TABLE IF NOT EXISTS cloud_project_controls (project TEXT PRIMARY KEY, sync_enabled BOOLEAN NOT NULL DEFAULT 1, paused_reason TEXT, updated_at TEXT NOT NULL, updated_by TEXT)`. Non-breaking migration.

**Dashboard `DashboardStore` interface — ADD methods**
- 5 paginated list methods (mirror cloudstore methods above)
- 3 detail methods (`GetSessionDetail`, `GetObservationDetail`, `GetPromptDetail`)
- `SystemHealth()`, `ListProjectSyncControls()`, `GetProjectSyncControl(string)`, `SetProjectSyncEnabled(...)`

**Cloudserver routes — 8 NEW HTMX endpoints + 3 detail pages**
1. `GET /dashboard/projects/list` (partial)
2. `GET /dashboard/projects/{name}/observations` (partial)
3. `GET /dashboard/projects/{name}/sessions` (partial)
4. `GET /dashboard/projects/{name}/prompts` (partial)
5. `GET /dashboard/admin/users` (page)
6. `GET /dashboard/admin/health` (page)
7. `POST /dashboard/admin/projects/{name}/sync` (mutation; admin-gated)
8. `GET /dashboard/admin/projects/{name}/sync/form` (form partial)
9. `GET /dashboard/sessions/{id}` (detail page) — new
10. `GET /dashboard/observations/{id}` (detail page) — new
11. `GET /dashboard/prompts/{id}` (detail page) — new

Items 9–11 use composite IDs (`{project}:{sessionID}[:{chunkID}]`) because the chunk-centric cloudstore does not have globally unique numeric IDs like the legacy SQL store. URL scheme will be `/dashboard/sessions/{project}/{sessionID}`, `/dashboard/observations/{project}/{sessionID}/{chunkID}`, `/dashboard/prompts/{project}/{sessionID}/{chunkID}`.

**Tooling (CI / build)**
- `go.mod`: add `github.com/a-h/templ v0.3.1001` (matches legacy version).
- `tools/tools.go`: build-tag `tools` file with blank import of `github.com/a-h/templ/cmd/templ` so `go mod tidy` keeps the binary discoverable.
- `go:generate` directive in `internal/cloud/dashboard/dashboard.go` — `//go:generate go tool templ generate`.
- `Makefile` target `templ` that runs `go tool templ generate ./internal/cloud/dashboard/...`.
- README contributor section updated with the templ regeneration command.

### OUT of scope (explicit exclusions)

- Restoring legacy username/email/JWT claims or multi-user scoping in dashboard queries.
- Legacy credential login (`email + password` fields) — token-only login stays.
- Restructuring `dashboardReadModel` or the sync write contract.
- Unit tests for every legacy templ component individually — parity is proven at the rendered-HTML level via `httptest`.
- Browser-based visual regression (screenshot diff). The final visual match against user-supplied screenshots is confirmed manually on `engram.condetuti.com`; no automated pixel test.
- Reworking `cloudserver.go` route mount patterns beyond the additive `MountConfig.GetDisplayName` field and new route registrations.
- `cloud-dashboard-parity` or `cloud-upgrade-path-existing-users` scope — those remain independent changes.

## Success criteria

- Static assets: `htmx.min.js` ≥ 40 KB, `pico.min.css` ≥ 60 KB, `styles.css` ≥ 20 KB
- Templ generated files: `components_templ.go` ≥ 100 KB, all start with `// Code generated by templ`
- Dashboard shell structure: all CSS class markers present (`shell-body`, `shell-header`, `shell-nav`, `shell-main`, `shell-footer`)
- All 11 new routes return 200 for authed Principal
- Admin mutations return 403 for non-admin, 303 for admin
- Principal bridge renders `OPERATOR` when nil
- Test coverage ≥ 79%
- All static checks pass (`go test`, `go vet`, `gofmt`)
