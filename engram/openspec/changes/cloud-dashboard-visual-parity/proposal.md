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

**Tests (STRICT TDD — RED first, all in `internal/cloud/dashboard/*_test.go` unless noted)**
- `TestStaticAssetsExceedSizeFloors` — `StaticFS` sizes: `htmx.min.js` ≥ 40 000 B, `pico.min.css` ≥ 50 000 B, `styles.css` ≥ 20 000 B.
- `TestTemplGeneratedFilesAreCheckedIn` — `components_templ.go` ≥ 100 000 B, file starts with `// Code generated by templ`.
- `TestDashboardLayoutHTMLStructure` — `shell-body`, `shell-backdrop`, `app-shell`, `shell-header`, `brand-stack`, `shell-nav`, `shell-main`, `shell-footer`.
- `TestStatusRibbonAndFooterPresent` — `status-ribbon`, `status-pill`, `CLOUD ACTIVE`, `ENGRAM CLOUD / SHARED MEMORY INDEX / LIVE SYNC READY`.
- `TestNavTabsRenderedCorrectly` — `href="/dashboard/"`, `/dashboard/browser`, `/dashboard/projects`, `/dashboard/contributors`, `/dashboard/admin`.
- `TestLoginPageTokenFormAndCopy` — `name="token"`, `Engram Cloud`, `CLOUD ACTIVE`, `hidden` input `next`.
- `TestDashboardHomeHTMXWiring` — `hx-get="/dashboard/stats"`, `hx-get="/dashboard/activity"`, `hx-trigger="load"`.
- `TestBrowserPageHTMXWiring` — `hx-get="/dashboard/browser/observations|sessions|prompts"`, `hx-target="#browser-content"`, `hx-include="#browser-project"`.
- `TestProjectsPageHTMXWiring` — `hx-get="/dashboard/projects/list"`, `hx-target="#projects-content"`, `hx-trigger="keyup"`.
- `TestAdminPageSurfacePresent` — admin overview metrics + `hx-post` toggle presence when `IsAdmin` returns true.
- `TestAdminHealthPageRendersMetrics` — DB status + project/contributor/session/observation/prompt counts.
- `TestAdminUsersPageRendersContributors` — uses existing contributor rows.
- `TestAdminSyncTogglePosts` — POST flips `SetProjectSyncEnabled`, redirects, records `updatedBy` from the Principal display name.
- `TestAdminSyncToggleRequiresAdmin` — POST is 403 for non-admin Principal.
- `TestCopyParityStrings` — kicker strings on every page (`KNOWLEDGE BROWSER`, `PROJECT ATLAS`, `CONTRIBUTOR SIGNAL`, `ADMIN SURFACE`).
- `TestGetDisplayNameFallback` — when `MountConfig.GetDisplayName` is nil, handler renders `OPERATOR`.
- `TestCloudstoreSystemHealthAggregates` — `internal/cloud/cloudstore/dashboard_queries_test.go` — asserts counts from seeded chunks.
- `TestProjectSyncControlPersists` — `internal/cloud/cloudstore/project_controls_test.go` — round-trip enable/disable + list.
- `TestInsecureModeLoginRedirects` — `internal/cloud/cloudserver/cloudserver_test.go` — `GET /dashboard/login` with `auth == nil` redirects straight to `/dashboard/` (confirms exploration finding; adds regression guard).

**Docs**
- `docs/ARCHITECTURE.md` — add short subsection on dashboard visual-parity layer + helper bridges.
- `docs/AGENT-SETUP.md` — contributor templ command.
- `README.md` — templ prerequisite note for contributors touching the dashboard.

### OUT of scope (explicit exclusions)

- Restoring legacy username/email/JWT claims or multi-user scoping in dashboard queries.
- Legacy credential login (`email + password` fields) — token-only login stays.
- Restructuring `dashboardReadModel` or the sync write contract.
- Unit tests for every legacy templ component individually — parity is proven at the rendered-HTML level via `httptest`.
- Browser-based visual regression (screenshot diff). The final visual match against user-supplied screenshots is confirmed manually on `engram.condetuti.com`; no automated pixel test.
- Reworking `cloudserver.go` route mount patterns beyond the additive `MountConfig.GetDisplayName` field and new route registrations.
- `cloud-dashboard-parity` or `cloud-upgrade-path-existing-users` scope — those remain independent changes.

## Approach

### Copy strategy (locked)

Reference: engram memory topic `sdd/cloud-dashboard-visual-parity/copy-strategy` (#2420). 95 % verbatim from `/Users/alanbuscaglia/work/engram-cloud/internal/cloud/dashboard/`, 4 % surgical adaptation (import paths, flat-row types, auth bridge), 1 % new (pagination slicing, `GetDisplayName`, `SystemHealth`, composite-ID detail routes, insecure-mode regression test). User-confirmed during exploration via explicit "copiar" decision.

### Layered implementation sequence (dependency-ordered)

1. **Dependency & tooling** (batch 1): add `github.com/a-h/templ v0.3.1001` to `go.mod`, create `tools/tools.go`, add `go:generate` directive, update `Makefile`, document in README.
2. **RED tests** (batch 1): write every test listed in Scope → Tests. They will all fail — confirm via `go test ./internal/cloud/dashboard/... ./internal/cloud/cloudstore/... ./internal/cloud/cloudserver/...` before touching code.
3. **Static assets** (batch 2): copy `htmx.min.js`, `pico.min.css`, `styles.css` byte-for-byte. `TestStaticAssetsExceedSizeFloors` goes GREEN.
4. **Cloudstore extensions** (batch 3): extend flat rows with detail fields, add paginated methods, add `SystemHealth`, add `project_controls.go` (SQL table + methods). `TestCloudstoreSystemHealthAggregates` + `TestProjectSyncControlPersists` GREEN.
5. **`DashboardStore` interface** (batch 3): add matching methods + `GetDisplayName` field to `MountConfig`.
6. **Templ source ports** (batch 3): `components.templ`, `layout.templ`, `login.templ` — copy verbatim, adapt import path (`github.com/Gentleman-Programming/engram/internal/cloud/cloudstore`), adapt component signatures to flat-row types. Run `go tool templ generate` — check in `*_templ.go`.
7. **`helpers.go`** (batch 3): replace with legacy version (change import path, drop `ProjectStat`-specific helpers by remapping to `DashboardProjectRow`).
8. **Handlers** (batch 3): in `dashboard.go`, replace every `renderLoginPage` / `renderLayout` / `renderHTML` string-concat call with `Layout(title, h.cfg.GetDisplayName(r), activeTab, h.cfg.IsAdmin(r), DashboardHome(...))`-style templ renders. Add new handlers: `handleProjectsList`, `handleProjectObservationsPartial`, `handleProjectSessionsPartial`, `handleProjectPromptsPartial`, `handleAdminUsers`, `handleAdminHealth`, `handleAdminSyncTogglePost`, `handleSessionDetail`, `handleObservationDetail`, `handlePromptDetail`. All structural, HTML, copy, and HTMX tests from batch 1 go GREEN.
9. **Cloudserver wiring** (batch 3): in `cloudserver.go:routes()`, add `GetDisplayName: func(r *http.Request) string { return "OPERATOR" }` to the `MountConfig` literal. Admin POST route is gated by `IsAdmin` closure that wraps `isDashboardAdmin`.
10. **Verify** (batch 4): `go test -cover ./...` — expect coverage ≥ 79 %. Manual spot-check with `ENGRAM_CLOUD_INSECURE_NO_AUTH=1 go run ./cmd/engram cloud serve`.

### Auth-bridge pattern (detailed)

The legacy handlers call `getUsernameFromContext(r)`, `getUserIDFromContext(r)`, `getEmailFromContext(r)`, `h.isAdmin(r)`. The integrated dashboard has a closure-based Principal model — `MountConfig.IsAdmin(r)`, `MountConfig.RequireSession(r)`. Mapping:

| Legacy call | Integrated bridge |
|---|---|
| `getUsernameFromContext(r)` | `h.cfg.GetDisplayName(r)` — new closure, fallback `"OPERATOR"` |
| `getUserIDFromContext(r)` | **removed** — integrated queries scope by `dashboardAllowedScopes`, not user |
| `getEmailFromContext(r)` | **removed** — admin check uses `h.cfg.IsAdmin(r)` |
| `h.isAdmin(r)` | `h.cfg.IsAdmin(r)` |
| `user.Claims["email"]` | not used; drop |

A tiny `principalFromRequest(r)` helper inside `dashboard.go` is NOT added — the closures (`GetDisplayName`, `IsAdmin`, `RequireSession`) collectively encode the Principal and are already passed via `MountConfig`. Any legacy helper that internally composed multiple context reads is refactored into a method on `handlers` that calls the closures directly (e.g., `func (h *handlers) displayName(r *http.Request) string { if h.cfg.GetDisplayName == nil { return "OPERATOR" }; return h.cfg.GetDisplayName(r) }`).

The `POST /dashboard/admin/projects/{name}/sync` handler calls `SetProjectSyncEnabled(project, enabled, h.displayName(r), reason)` so `updatedBy` is the display name — sufficient provenance for single-operator runtimes.

### Cloudstore read-model additions (exact signatures)

All additions go in `internal/cloud/cloudstore/dashboard_queries.go` (queries) and new `internal/cloud/cloudstore/project_controls.go` (mutations + persistence).

```go
// dashboard_queries.go additions
type DashboardSystemHealth struct {
    DBConnected  bool
    Projects     int
    Contributors int
    Sessions     int
    Observations int
    Prompts      int
    Chunks       int
}

func (cs *CloudStore) ListProjectsPaginated(query string, limit, offset int) ([]DashboardProjectRow, int, error)
func (cs *CloudStore) ListRecentObservationsPaginated(project, query, obsType string, limit, offset int) ([]DashboardObservationRow, int, error)
func (cs *CloudStore) ListRecentSessionsPaginated(project, query string, limit, offset int) ([]DashboardSessionRow, int, error)
func (cs *CloudStore) ListRecentPromptsPaginated(project, query string, limit, offset int) ([]DashboardPromptRow, int, error)
func (cs *CloudStore) ListContributorsPaginated(query string, limit, offset int) ([]DashboardContributorRow, int, error)
func (cs *CloudStore) GetSessionDetail(project, sessionID string) (DashboardSessionRow, []DashboardObservationRow, []DashboardPromptRow, error)
func (cs *CloudStore) GetObservationDetail(project, sessionID, chunkID string) (DashboardObservationRow, DashboardSessionRow, []DashboardObservationRow, error)
func (cs *CloudStore) GetPromptDetail(project, sessionID, chunkID string) (DashboardPromptRow, DashboardSessionRow, []DashboardPromptRow, error)
func (cs *CloudStore) SystemHealth() (DashboardSystemHealth, error)

// project_controls.go (new file)
type ProjectSyncControl struct {
    Project      string
    SyncEnabled  bool
    PausedReason *string
    UpdatedAt    string
    UpdatedBy    *string
}

func (cs *CloudStore) IsProjectSyncEnabled(project string) (bool, error)
func (cs *CloudStore) SetProjectSyncEnabled(project string, enabled bool, updatedBy, reason string) error
func (cs *CloudStore) GetProjectSyncControl(project string) (*ProjectSyncControl, error)
func (cs *CloudStore) ListProjectSyncControls() ([]ProjectSyncControl, error)
```

Flat-row extensions:

```go
type DashboardObservationRow struct {
    Project    string
    SessionID  string
    ChunkID    string  // NEW — composite-ID key
    Type       string
    Title      string
    Content    string  // NEW — materialized from chunk payload
    TopicKey   string  // NEW
    ToolName   string  // NEW
    CreatedAt  string
}

type DashboardSessionRow struct {
    Project    string
    SessionID  string
    StartedAt  string
    EndedAt    string  // NEW — if sync payload contains session close
    Summary    string  // NEW — from session chunk
    Directory  string  // NEW — from session chunk
}

type DashboardPromptRow struct {
    Project    string
    SessionID  string
    ChunkID    string  // NEW — composite-ID key
    Content    string
    CreatedAt  string
}
```

These fields come from chunk JSON that `buildDashboardReadModel` already decodes — the read-model aggregator just needs to retain them instead of discarding.

### Test seams (each in-scope item names its first failing test)

| In-scope item | First failing test |
|---|---|
| Static assets | `TestStaticAssetsExceedSizeFloors` |
| `*_templ.go` checked in | `TestTemplGeneratedFilesAreCheckedIn` |
| Layout shell structure | `TestDashboardLayoutHTMLStructure` |
| Status ribbon + footer | `TestStatusRibbonAndFooterPresent` |
| Nav tabs | `TestNavTabsRenderedCorrectly` |
| Login page | `TestLoginPageTokenFormAndCopy` |
| Home HTMX wiring | `TestDashboardHomeHTMXWiring` |
| Browser HTMX wiring | `TestBrowserPageHTMXWiring` |
| Projects HTMX wiring | `TestProjectsPageHTMXWiring` |
| Admin page | `TestAdminPageSurfacePresent` |
| Admin health page | `TestAdminHealthPageRendersMetrics` |
| Admin users page | `TestAdminUsersPageRendersContributors` |
| Admin sync toggle POST | `TestAdminSyncTogglePosts` |
| Admin sync toggle auth | `TestAdminSyncToggleRequiresAdmin` |
| Copy parity kickers | `TestCopyParityStrings` |
| `GetDisplayName` bridge | `TestGetDisplayNameFallback` |
| SystemHealth query | `TestCloudstoreSystemHealthAggregates` |
| Project sync controls | `TestProjectSyncControlPersists` |
| Insecure-mode redirect | `TestInsecureModeLoginRedirects` |

## Architectural Decisions

### AD-1 — DashboardXxxRow: extend flat types additively (Option A)

**Decision**: Extend the existing `DashboardObservationRow`, `DashboardSessionRow`, `DashboardPromptRow` types additively with `Content`, `TopicKey`, `ToolName`, `EndedAt`, `Summary`, `Directory`, `ChunkID`, `ID` — the fields detail pages require.

**Rationale**: The chunk JSON payloads already contain every field the legacy rich types exposed. `buildDashboardReadModel` currently discards them. Adding fields to the flat row is a pure additive change — zero cost to existing callers, zero new queries. Alternative (b) — a second `DashboardXxxDetail` type — doubles the surface and forces the rendering layer to branch between list and detail. Alternative (c) — hydrate on demand via a second cloudstore call — is strictly worse: extra round trip, plus the data lives in the same read-model pass. Option (a) keeps the rendering layer uniform and the query layer simple.

**Impact**: `internal/cloud/cloudstore/dashboard_queries.go` (field additions + upsert function updates); `internal/cloud/cloudstore/dashboard_queries_test.go` (new field assertions); no change to sync writers; no change to chunk payload format. Every templ component that receives a row uses the same type whether rendered in a card, a table, or a detail page — the renderer branches on which fields are blank, not on which type arrived.

### AD-2 — `GetDisplayName` in `MountConfig`: add additive optional field and update the single production caller in the same batch

**Decision**: Add `GetDisplayName func(r *http.Request) string` to `dashboard.MountConfig`. When nil, handlers fall back to `"OPERATOR"`. Update the single production instantiation in `internal/cloud/cloudserver/cloudserver.go` in the same batch. Test helpers in `dashboard_test.go` (7 instantiations — cloudserver.go + 6 test sites) can keep it nil and rely on the fallback.

**Rationale**: Adding a new field to a struct in Go is a non-breaking change at compile time unless callers use positional struct literals — none here do (all use named fields). Test-site tolerance (nil-safe fallback) is the idiomatic Go pattern and keeps the test surface small. The alternative — a named constructor `NewMountConfig(...)` — would require a wider refactor without benefit.

**Impact**: `internal/cloud/dashboard/dashboard.go` (`MountConfig` struct + `handlers.displayName(r)` helper); `internal/cloud/cloudserver/cloudserver.go` (one line: wire the closure); `internal/cloud/dashboard/dashboard_test.go` (unchanged for nil-defaulting sites; new test `TestGetDisplayNameFallback` confirms the default).

### AD-3 — `ProjectSyncControl` + `SystemHealthInfo`: include both as additive cloudstore + `DashboardStore` methods

**Decision**: Include both surfaces. `SystemHealth` is added as `DashboardSystemHealth` (integrated-naming) on `CloudStore`. `ProjectSyncControl` gets a new file `internal/cloud/cloudstore/project_controls.go` with a new SQL table `cloud_project_controls` — mirroring the legacy schema verbatim (integrated cloudstore already uses SQLite for chunk metadata, so a new table is a one-migration addition). Both are exposed on the `DashboardStore` interface.

**Rationale**: The user stated "complete as possible" — admin visual parity without a working toggle is partial. The legacy sync-pause behavior is a documented server-side safety mechanism (engram business rules say "blocked sync must fail loudly, never silent drops"). Dropping it would silently degrade the admin surface. The integrated cloudstore already has a SQL database (`cs.db`) and already runs migrations; adding one table is cheap and keeps the feature enforceable. `SystemHealth` is a pure read aggregate — no new persistence needed.

**Impact**: new file `internal/cloud/cloudstore/project_controls.go` (~120 L); new test file `internal/cloud/cloudstore/project_controls_test.go`; extension of `DashboardStore` interface in `dashboard.go`; push-path guard in `cloudserver.go` that calls `IsProjectSyncEnabled` before accepting chunks (loud 409 Conflict response on paused project — matches engram-business-rules "fail loudly"). The push-path guard is an essential-side effect of exposing the toggle; without it the admin toggle would be purely cosmetic, which would violate `engram-business-rules` ("UI controls must map to real, enforceable behavior").

### AD-4 — 8 missing HTMX endpoints: wire all 8 + 3 detail pages (11 total)

**Decision**: Register all 8 missing partial endpoints AND the 3 new detail pages (sessions, observations, prompts) in `cloudserver.go` mount + corresponding handlers in `dashboard.go`. Non-negotiable for parity per task constraints.

**Rationale**: The legacy `components.templ` emits `hx-get` attributes that target exactly these paths. If the server does not register them, the HTMX partial loads silently 404 and the UI renders blank regions — visual parity failure. Detail pages are triggered by `<a href="/dashboard/sessions/...">` links that the cards emit; if those links 404 the user experience degrades the moment they click any card.

**Impact**: `internal/cloud/dashboard/dashboard.go` (11 new handlers + 11 `mux.HandleFunc` registrations inside `Mount`); `internal/cloud/dashboard/dashboard_test.go` (11 route-level tests that assert 200 + `Content-Type: text/html` for authed Principal); `internal/cloud/cloudstore/dashboard_queries.go` (the 3 `GetXxxDetail` methods backing the detail pages).

### AD-5 — Detail page scope: IN-scope for first apply batch

**Decision**: The three detail pages (`observation-detail`, `session-trace` / `session-detail`, `prompt-detail`, plus the already-existing contributor detail) are in scope for batch 3 of apply. They use the extended flat-row types from AD-1 and the new `GetXxxDetail` cloudstore methods.

**Rationale**: User-stated "complete as possible" parity goal. With the AD-1 decision, detail pages cost one extra method per type (read model already has the data) and three templ components that are almost byte-for-byte copies from legacy. Deferring them would leave broken anchor links in the list views — worse UX than simplifying list cards to hide the links, which would itself be a parity regression.

**Impact**: Batch 3 gains 3 templ components, 3 handlers, 3 routes, 3 cloudstore methods. No new queries beyond AD-1 + AD-3. URL scheme uses composite IDs (see Scope → Routes 9–11). The composite-ID format is documented in the handler godoc.

### AD-6 — Admin sync toggle: IN-scope, admin-gated via `IsAdmin` closure

**Decision**: Include `POST /dashboard/admin/projects/{name}/sync` + the sync-form partial. Mutation is gated at handler entry by `if !h.cfg.IsAdmin(r) { 403 }`. `updatedBy` is sourced from `h.displayName(r)`. `reason` comes from the POST form.

**Rationale**: Admin visual parity without a working control violates `engram-business-rules` ("UI controls must map to real, enforceable behavior") and `engram-dashboard-htmx` ("Mutation forms must work as normal HTTP posts too"). `ENGRAM_CLOUD_ADMIN` already drives `isDashboardAdmin` — re-using the `IsAdmin` closure keeps the auth surface uniform. AD-3 provides the backing persistence; without AD-3 this toggle would be cosmetic.

**Impact**: one new handler (`handleAdminSyncTogglePost` + `handleAdminSyncToggleForm`) in `dashboard.go`; two routes; two tests (`TestAdminSyncTogglePosts` succeeds as admin, `TestAdminSyncToggleRequiresAdmin` gets 403 as non-admin). The POST responds with 303 redirect back to `/dashboard/admin/projects`, plus HTMX-aware `HX-Redirect` header when the request is HTMX-driven (standard dashboard pattern).

### AD-7 — Templ binary availability: commit generated files + `tools/tools.go` + pinned version

**Decision**: Generated `*_templ.go` files are the source of truth at runtime (enforced by `templ_policy.go`). Add `github.com/a-h/templ v0.3.1001` to `go.mod`. Add a `tools/tools.go` file with the standard build-tag pattern (`//go:build tools` + blank import of `github.com/a-h/templ/cmd/templ`) so `go mod tidy` keeps the dependency discoverable and `go tool templ` resolves. Add `//go:generate go tool templ generate` on `dashboard.go` and a `make templ` target. Contributor README updated.

**Rationale**: Legacy `engram-cloud` uses `v0.3.1001` — matching the version means the generated files in this repo will be byte-identical to a legacy regeneration, minimizing diff churn. `tools/tools.go` is the canonical Go-way to track dev-only binaries in `go.mod`. `go:generate` + `make templ` gives contributors a single command. Checking in generated files keeps CI simple (no templ binary install step).

**Impact**: `go.mod` (+1 direct dep + transitive), `tools/tools.go` (new), `Makefile` (new target), `internal/cloud/dashboard/dashboard.go` (generate directive), `README.md` + `docs/AGENT-SETUP.md` (contributor instructions), `openspec/changes/cloud-dashboard-visual-parity/design.md` later captures the bootstrap command sequence.

### AD-8 — Insecure-mode fixes: RE-VERIFIED — already mitigated on current branch; keep regression test only

**Re-verification**: Read `cmd/engram/cloud.go:81–115` and `internal/cloud/cloudserver/cloudserver.go:130–197` + `:223–251`:

- `cmd/engram/cloud.go:90–102` — `authenticator` is nil when `insecureNoAuth = true`. `auth.NewService` is NOT called.
- `cloudserver.go:168–171` — when `s.auth == nil`, both `validateLoginToken` and `createSessionCookie` are set to nil (no panic path).
- `cloudserver.go:223–226` — `authorizeDashboardRequest` short-circuits `return nil` when `s.auth == nil` (no cookie required → dashboard works in insecure mode without credentials).
- `cloudserver.go:246–251` — `dashboardSessionToken` uses `codec, ok := s.auth.(dashboardSessionCodec)`. On a `nil` interface this is a safe assertion (Go spec: nil-interface assertion returns `ok=false`, no panic). But it is NEVER reached in insecure mode because `createSessionCookie` is nil and `handleLoginSubmit` therefore never calls it.

**Decision**: No code changes needed for insecure-mode fixes. The memory #2375 gaps are resolved on the `feat/integrate-engram-cloud` branch. Add **regression guard test** `TestInsecureModeLoginRedirects` in `cloudserver_test.go` that asserts `GET /dashboard/login` with `auth == nil` returns 303 to `/dashboard/` (this is the user-visible correct behavior — login page should not render when auth is disabled).

**Rationale**: Closing a non-existent bug wastes time. Adding a regression test locks in the current correct behavior so future refactors cannot silently re-introduce the hang.

**Impact**: `internal/cloud/cloudserver/cloudserver_test.go` (one new test, ~20 L). Zero runtime code changes. Memory #2375 can be marked resolved after the regression test lands.

## Risks & mitigations

| Risk | Mitigation |
|---|---|
| **1. `templ generate` binary required at build time** — if a contributor regenerates without `v0.3.1001`, the generated files diff. | Pin `go.mod` to `v0.3.1001`, add `tools/tools.go` blank import so `go mod tidy` keeps it discoverable, document `make templ` and `go tool templ generate`, add a `TestTemplGeneratedFilesAreCheckedIn` guard that also asserts the generator header line matches the pinned version. |
| **2. `github.com/a-h/templ` added to `go.mod` may pull transitive deps that shift the module graph.** | Run `go mod tidy` locally, commit the resulting `go.sum` diff, verify `go build ./...` in CI. Legacy engram-cloud already uses this dep at this version — transitive surface is known and small (`uritemplate` is the only non-stdlib transitive). |
| **3. Type mismatch between extended flat rows and legacy rich-type component bodies (e.g., `observation.RevisionCount`, `observation.Scope`).** | Adapt each component during the copy — any field NOT present on the extended flat row is either removed (unused decorative) or mapped to an available field. Tests assert rendered HTML structure, not field-by-field value, so missing decorative fields are silent. For fields we DO need and they do NOT exist in chunk JSON, we simplify the component (documented case-by-case in the spec). |
| **4. Static `//go:embed static` silently returns empty FS if the directory name or file names change.** | `TestStaticAssetsExceedSizeFloors` catches this by asserting real file sizes via `fs.Stat(StaticFS, "static/htmx.min.js")`. A 42 B stub or a missing file fails this test loudly. |
| **5. Pagination rendering path mismatch between legacy templ `HtmxPaginationBar` and integrated raw-string helpers.** | Port `HtmxPaginationBar` as a templ component (part of the components.templ copy). Remove any duplicate pagination helpers from integrated `helpers.go`. Tests assert `hx-get` + `hx-push-url` presence and the correct partial target ID. |
| **6. Admin sync toggle persistence requires a new SQL table — migration risk.** | New table uses `CREATE TABLE IF NOT EXISTS` — idempotent on existing DBs. Migration runs inside `cloudstore.New(cfg)`. Rollback is trivial (drop table). `TestProjectSyncControlPersists` round-trips a full enable/disable cycle including server restart (in-test sqlite file). |
| **7. Adding `GetDisplayName` to `MountConfig` is visible surface** — external consumers who copy `MountConfig` literals break. | `MountConfig` is an internal package; only `cloudserver.go` instantiates it in production. Tests already use named fields. Even if external callers emerge later, nil-defaulting keeps the field optional. |
| **8. Push-path guard (`IsProjectSyncEnabled`) could reject in-flight pushes during a pause, causing sync lag.** | Guard returns 409 Conflict with `ReasonCode = "sync-paused"` — matches the existing sync error contract. Admin UI shows paused state prominently (status ribbon + project row indicator). Documented in `docs/ARCHITECTURE.md`. |
| **9. Composite IDs in detail-page URLs (`/dashboard/sessions/{project}/{sessionID}`) are a new URL scheme** — could break future deep-link assumptions. | Document the scheme in handler godoc + `docs/ARCHITECTURE.md`. Legacy used numeric IDs because of SQL auto-increment; integrated is chunk-centric so composite keys are the only stable identifier. No existing external links to break. |
| **10. Visual parity with `engram.condetuti.com` is inherently subjective** — no automated pixel diff. | Success criteria split: deterministic tests for markup, copy, HTMX wiring, asset sizes (all CI-enforced); subjective match documented via user-supplied screenshots saved in engram memory after manual verification. |

## Success criteria

Verifiable (CI):
- [ ] `fs.Stat(StaticFS, "static/htmx.min.js")` returns size ≥ 40 000 B (asserted in `TestStaticAssetsExceedSizeFloors`).
- [ ] `fs.Stat(StaticFS, "static/pico.min.css")` returns size ≥ 50 000 B.
- [ ] `fs.Stat(StaticFS, "static/styles.css")` returns size ≥ 20 000 B.
- [ ] `components_templ.go`, `layout_templ.go`, `login_templ.go` exist, start with `// Code generated by templ`, and `components_templ.go` ≥ 100 000 B.
- [ ] `GET /dashboard/` for authed Principal contains `<div class="status-ribbon">` with text `CLOUD ACTIVE` and footer text `ENGRAM CLOUD / SHARED MEMORY INDEX / LIVE SYNC READY`.
- [ ] `GET /dashboard/`, `/dashboard/browser`, `/dashboard/projects`, `/dashboard/contributors`, `/dashboard/admin` each render with the full shell structure (`shell-body`, `shell-header`, `shell-nav`, `shell-main`, `shell-footer`).
- [ ] All 11 new/previously-missing routes return `200 OK` with `Content-Type: text/html; charset=utf-8` when authed as Principal — `TestProjectsPageHTMXWiring`, `TestAdminHealthPageRendersMetrics`, `TestAdminUsersPageRendersContributors`, three detail-page tests, plus existing-route assertions.
- [ ] `POST /dashboard/admin/projects/{name}/sync` returns `303 See Other` + `HX-Redirect` header for admin, `403 Forbidden` for non-admin (`TestAdminSyncTogglePosts` + `TestAdminSyncToggleRequiresAdmin`).
- [ ] `GET /dashboard/login` with `auth == nil` returns `303` to `/dashboard/` (`TestInsecureModeLoginRedirects`).
- [ ] `MountConfig.GetDisplayName` nil fallback renders `OPERATOR` (`TestGetDisplayNameFallback`).
- [ ] `DashboardSystemHealth` populated from seeded chunks (`TestCloudstoreSystemHealthAggregates`).
- [ ] `SetProjectSyncEnabled` + `ListProjectSyncControls` round-trip (`TestProjectSyncControlPersists`).
- [ ] `go test ./...` passes.
- [ ] `go test -cover ./...` overall coverage ≥ 79 % (matching prior change baseline from `cloud-dashboard-parity`).
- [ ] `go vet ./...` and `gofmt -l` clean.

Manual (documented, not test-automated):
- [ ] Deployed dashboard on `engram.condetuti.com` matches the user-supplied screenshots from message 1: status ribbon visible, HTMX nav functional across all five tabs, project pages load without blank regions, admin sync toggle flips state.
- [ ] Browser DevTools shows no `404` for `/dashboard/static/htmx.min.js`, `/dashboard/static/pico.min.css`, `/dashboard/static/styles.css`, nor for any of the 11 new endpoints.
