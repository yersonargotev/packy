# Exploration — cloud-dashboard-visual-parity

## Context
The previous change `cloud-dashboard-parity` is 19/19 tasks complete and tests are green, but the deployed dashboard has ZERO visual parity with the legacy `engram-cloud` dashboard. Tests validated route ownership, auth redirects, and read-model queries, but did NOT verify that static assets were real or that templ components rendered the rich HTMX layout. This exploration maps every file/seam gap and prescribes the implementation sequence.

---

## 1. Direct-Port List (VERBATIM copies from legacy)

These files can be copied byte-for-byte. No adaptation needed. All paths are relative to `internal/cloud/dashboard/`.

| Source (legacy) | Destination (integrated) | Current size (integrated) | Legacy size | Action |
|---|---|---|---|---|
| `static/htmx.min.js` | `static/htmx.min.js` | 42 bytes (placeholder) | 50,917 bytes | REPLACE |
| `static/pico.min.css` | `static/pico.min.css` | 46 bytes (placeholder) | 71,072 bytes | REPLACE |
| `static/styles.css` | `static/styles.css` | 2,284 bytes (thin stub) | 23,185 bytes | REPLACE |

Static assets are binary-identical to what is served by the legacy binary. No adaptation needed. They are embedded via `//go:embed static` in `embed.go` (identical in both repos).

---

## 2. Adaptation List

### 2a. `components.templ` (1,134 lines → REPLACE)

The integrated file has 23 lines and defines only `StatusBadge`, `EmptyState`, and `SyncStatusPanel`. The legacy file defines the full 30+ component surface. The legacy file imports:
```go
import (
    "fmt"
    "github.com/Gentleman-Programming/engram-cloud/internal/cloud/cloudstore"
)
```

**Adaptation required**: change import path to `github.com/Gentleman-Programming/engram/internal/cloud/cloudstore`. All type references must use the integrated `cloudstore` types. The integrated `cloudstore` exposes `DashboardProjectRow`, `DashboardContributorRow`, `DashboardSessionRow`, `DashboardObservationRow`, `DashboardPromptRow`, `DashboardProjectDetail`, and `DashboardAdminOverview` (all defined in `dashboard_queries.go`).

Legacy `components.templ` references 12 cloudstore types:
- `cloudstore.ProjectStat` — NOT in integrated cloudstore (it's a legacy type from the old SQL-backed cloudstore). Used by `DashboardStatsPartial`, `DashboardActivityPartial`, `ProjectsListPartial`. **Must be replaced** with `DashboardProjectRow` or a new `ProjectStat` adapter.
- `cloudstore.CloudObservation` — NOT in integrated (legacy SQL-backed). Used by `ObservationsPartial`, `ObservationDetailPage`, `SessionDetailPage`, `ContributorDetailPage`. **Must be replaced** with `DashboardObservationRow`.
- `cloudstore.CloudSession` — NOT in integrated. Used by `SessionDetailPage`, `ObservationDetailPage`. **Must be replaced** with a simpler struct or `DashboardSessionRow`.
- `cloudstore.CloudSessionSummary` — NOT in integrated. Used by `SessionsPartial`. **Replace** with `DashboardSessionRow`.
- `cloudstore.CloudPrompt` — NOT in integrated. Used by `PromptsPartial`, `PromptDetailPage`. **Replace** with `DashboardPromptRow`.
- `cloudstore.CloudUser` — NOT in integrated. Used by `ContributorDetailPage`. Integrated uses contributor string (CreatedBy).
- `cloudstore.ContributorStat` — NOT in integrated. Used by `ContributorsPage`, `ContributorDetailPage`. **Replace** with `DashboardContributorRow`.
- `cloudstore.ProjectSyncControl` — NOT in integrated `dashboard_queries.go` (it IS in `cloudstore.go`). Verify whether it is accessible from the dashboard package.
- `cloudstore.SystemHealthInfo` — NOT in integrated. Used by `AdminPage`, `AdminHealthPage`.
- `cloudstore.DashboardProjectDetail` — EXISTS in integrated.
- `cloudstore.DashboardProjectRow` — EXISTS in integrated.
- `cloudstore.DashboardAdminOverview` — EXISTS in integrated.

**Key adaptation decision**: The legacy `components.templ` renders rich entity types (`CloudObservation`, `CloudSession`, etc.) that map to the old SQL-backed cloudstore. The integrated cloudstore is chunk-centric and exposes only flat `DashboardXxxRow` types via the read model. The adaptation must map:
- `CloudObservation` fields used in components → `DashboardObservationRow` (Project, SessionID, Type, Title, CreatedAt). Fields like `ID`, `Content`, `Scope`, `TopicKey`, `ToolName`, `RevisionCount` are NOT in `DashboardObservationRow` and NOT queryable from the chunk read model. Detail pages that require full observation content require a new `GetObservation`-style query or must be simplified to show what the read model has.
- `CloudSession` fields → `DashboardSessionRow` (Project, SessionID, StartedAt). No EndedAt, Summary, Directory.
- `CloudPrompt` fields → `DashboardPromptRow` (Project, SessionID, Content, CreatedAt). No `ID`.

### 2b. `layout.templ` (52 lines → REPLACE from legacy, minor adaptation)

Current integrated `layout.templ` defines `CloudLayout` (not used by `dashboard.go`) with no HTMX nav. Legacy defines `Layout(title, username, activeTab, isAdmin, content templ.Component)` with:
- `status-ribbon` div with "CLOUD ACTIVE" + "shared memory index / live sync ready"
- `brand-subtitle`: "An elephant never forgets."
- `user-info` with username + Change Password link + Logout button
- Full `NavTabs` HTMX nav
- `shell-footer` with "ENGRAM CLOUD / SHARED MEMORY INDEX / LIVE SYNC READY"

**Adaptation**: The legacy `Layout` takes a `username` string parameter (from `getUsernameFromContext`). The integrated auth model does NOT put a username in context — it uses `RequireSession` / cookie-based Principal. The integrated `dashboard.go` handlers do NOT call `getUsernameFromContext`. The adaptation must either:
- Extract a display name from the signed `engram_dashboard_token` cookie (if the codec encodes one), OR
- Show a static placeholder like "OPERATOR" (per legacy kicker style), OR
- Not show a username in the header (simplest)

The cleanest approach: the `MountConfig.IsAdmin` closure already exists. Add a `MountConfig.GetUsername(r *http.Request) string` helper that the session codec can populate, defaulting to "OPERATOR" if absent.

### 2c. `login.templ` (63 lines → REPLACE from legacy, minimal adaptation)

Legacy defines a token-based login form matching the integrated auth flow:
- `name="token"` input (password type) — already compatible with the integrated `handleLoginSubmit` which reads `r.PostForm.Get("token")`
- Sidepanel with "CLOUD ACTIVE", "Engram Cloud", login-lead copy, and `hero-console` element
- "SIGN IN" kicker and "Dashboard Login" heading
- No username/password fields (those are from OLD legacy — the target is the 63-line version in `engram-cloud`)

**Adaptation**: Change the copy in the sidepanel from "Use your cloud runtime token..." (current integrated 38-line stub) to the richer legacy version. The `LoginPage(errorMsg string)` signature is identical. Also needs the `next` hidden input support from the integrated `handleLoginPage` (which the integrated 38-line stub lacks). The integrated `renderLoginPage` in `dashboard.go` already handles `next` — but that is a raw string function, not templ. When we port the templ version, we need `LoginPage(errorMsg, next string)` signature. Check: the integrated `dashboard.go:handleLoginPage` calls `renderLoginPage(w, "", next)` which is NOT a templ call. After porting to templ, `handleLoginPage` must call `LoginPage(errorMsg, next).Render(...)`.

### 2d. `dashboard.go` (integrated — keep structure, surgical changes)

The integrated `dashboard.go` uses:
- `renderLoginPage` / `renderLayout` / `renderHTML` — raw string functions, not templ. These must be replaced by calls to the new templ components.
- `getUsernameFromContext`, `getUserIDFromContext` — these do NOT exist in the integrated middleware because the integrated middleware uses `RequireSession` (a closure) not direct context injection. The handlers must derive username differently.
- `shellNavLink`, `browserSubtabLink` — these generate HTMX nav links as raw strings in integrated `dashboard.go`. After porting templ, these can be removed and replaced by templ components from `components.templ`.

Key handler changes after porting:
- `handleDashboardHome`: currently returns inline HTML with `content` string. After port: call `DashboardHome(username).Render(...)` and wrap in `Layout(...)`.
- `handleLoginPage`: currently calls `renderLoginPage(w, "", next)`. After port: call `LoginPage("", next).Render(...)`.
- `handleAdmin`: currently limited (no users/health subroutes). After port: must add `handleAdminUsers` and `handleAdminHealth` and register routes `GET /dashboard/admin/users` and `GET /dashboard/admin/health`.
- `handleProjectDetail`: currently returns flat HTML. After port: uses `ProjectDetailPage` which needs `cloudstore.ProjectStat` (not in integrated). Must adapt signature.

### 2e. `helpers.go` (integrated vs legacy — MERGE)

The integrated `helpers.go` (9 lines + path utilities) is minimal. The legacy `helpers.go` (424 lines) adds:
- `Pagination` struct + `parsePagination` + `paginationURL` — needed by all list partials
- `totalSessionCount`, `totalObservationCount`, `totalPromptCount` — needed by `DashboardStatsPartial`
- `browserURL`, `typePillClass` — needed by `BrowserPage`
- `formatTimestamp`, `formatTimestampPtr` — needed by all detail views
- `countPausedProjects`, `controlsByProject`, `projectControlReasonValue`, `projectControl` — needed by admin components
- `truncateContent` — needed by observation/prompt cards
- `renderStructuredContent`, `renderInlineStructuredPreview`, `renderHeadingSections`, etc. — needed by observation detail
- `typeBadgeVariant` — needed by observation cards

**Strategy**: REPLACE integrated `helpers.go` with the legacy version. Change import path. Remove legacy-only imports (`cloudstore.ProjectStat`, `cloudstore.ProjectSyncControl`) if those types are adapted away.

### 2f. `middleware.go` (integrated — KEEP, minimal)

The integrated `middleware.go` defines `requireSession` as a method on handlers. It does NOT inject username/userID into context (unlike legacy). This is by design. No replacement needed. However, if the legacy `Layout` component requires a username, a new helper `GetUsername(r *http.Request) string` must be plumbed through `MountConfig`.

### 2g. `config.go` (integrated vs legacy — KEEP integrated)

Integrated `config.go` defines `DashboardConfig{AdminToken string}`. Legacy defines `DashboardConfig{AdminEmail string}`. These differ. The integrated version feeds `WithDashboardAdminToken` in cloudserver. No change needed to config.

---

## 3. Missing Cloudstore Queries

The legacy `dashboard.go` calls the following cloudstore methods that do NOT have exact matches in the integrated `dashboard_queries.go`:

| Legacy call | Integrated equivalent | Gap |
|---|---|---|
| `store.ProjectStats(userID)` | No equivalent (legacy SQL-backed) | `ProjectStat` type missing |
| `store.UserProjects(userID)` | No equivalent | Missing |
| `store.ObservationTypes(userID, activeProject)` | No equivalent | Missing |
| `store.SearchPaginated(userID, search, opts, offset)` | No equivalent | Missing |
| `store.FilterObservationsPaginated(userID, project, search, type, limit, offset)` | No equivalent | Missing |
| `store.RecentSessionsPaginated(userID, project, limit, offset)` | No equivalent | Missing |
| `store.SearchPromptsPaginated(userID, search, project, limit, offset)` | No equivalent | Missing |
| `store.RecentPromptsPaginated(userID, project, limit, offset)` | No equivalent | Missing |
| `store.SearchProjectStatsPaginated(userID, search, limit, offset)` | No equivalent | Missing |
| `store.ProjectStatsPaginated(userID, limit, offset)` | No equivalent | Missing |
| `store.ListProjectSyncControls()` | Not in dashboard_queries.go | Missing (may exist in cloudstore.go) |
| `store.GetProjectSyncControl(project)` | Not in dashboard_queries.go | Missing |
| `store.SetProjectSyncEnabled(...)` | Not in dashboard_queries.go | Missing |
| `store.GetSession(userID, sessionID)` | No equivalent | Missing |
| `store.SessionObservations(userID, sessionID, limit)` | No equivalent | Missing |
| `store.SessionPrompts(userID, sessionID, limit)` | No equivalent | Missing |
| `store.GetObservation(userID, obsID)` | No equivalent | Missing |
| `store.GetPrompt(userID, promptID)` | No equivalent | Missing |
| `store.GetUserByID(contributorID)` | No equivalent | Missing |
| `store.ContributorStats()` | No equivalent | Missing |
| `store.ContributorStatsPaginated(limit, offset)` | Closest: `ListContributors(query)` | Different signature |
| `store.RecentSessions(contributorID, project, limit)` | Closest: `ListRecentSessions(project, query, limit)` | Different (no userID filter) |
| `store.RecentObservations(contributorID, project, type, limit)` | Closest: `ListRecentObservations(project, query, limit)` | Missing type filter |
| `store.RecentPrompts(contributorID, project, limit)` | Closest: `ListRecentPrompts(project, query, limit)` | Different |
| `store.ListAllUsersPaginated(limit, offset)` | No equivalent | Missing |
| `store.SystemHealth()` | No equivalent | Missing |
| `store.GetProjectSyncControl(project)` | Not in dashboard_queries.go | Missing |

**Reality check**: The integrated dashboard does NOT use `userID` scoping (unlike legacy which was multi-user). The integrated model is single-user (or uses `dashboardAllowedScopes` for project scoping). The integrated `DashboardStore` interface in `dashboard.go` is:

```go
type DashboardStore interface {
    ListProjects(query string) ([]cloudstore.DashboardProjectRow, error)
    ProjectDetail(project string) (cloudstore.DashboardProjectDetail, error)
    ListContributors(query string) ([]cloudstore.DashboardContributorRow, error)
    ListRecentSessions(project string, query string, limit int) ([]cloudstore.DashboardSessionRow, error)
    ListRecentObservations(project string, query string, limit int) ([]cloudstore.DashboardObservationRow, error)
    ListRecentPrompts(project string, query string, limit int) ([]cloudstore.DashboardPromptRow, error)
    AdminOverview() (cloudstore.DashboardAdminOverview, error)
}
```

These 7 methods are sufficient to power the **visual parity target** if the components are adapted to use `DashboardXxxRow` instead of `CloudXxxObservation`-style rich types. Pagination in the integrated model is client-side slicing of in-memory read model results (no SQL `LIMIT/OFFSET` needed). The approach is:

- Add a `ListProjectsSummary() ([]DashboardProjectRow, error)` alias (already `ListProjects("")`) for stats strip
- Add `filterX(rows, limit, offset int)` helper functions inside `dashboard_queries.go` for pagination slicing
- The following NEW methods should be added to `CloudStore` and `DashboardStore`:
  - `ListProjects(query string) ([]DashboardProjectRow, error)` — EXISTS
  - `ListProjectsPaginated(query string, limit, offset int) ([]DashboardProjectRow, int, error)` — NEW (slice of read model)
  - `ListRecentObservationsPaginated(project, query string, limit, offset int) ([]DashboardObservationRow, int, error)` — NEW
  - `ListRecentSessionsPaginated(project, query string, limit, offset int) ([]DashboardSessionRow, int, error)` — NEW
  - `ListRecentPromptsPaginated(project, query string, limit, offset int) ([]DashboardPromptRow, int, error)` — NEW
  - `ListContributorsPaginated(query string, limit, offset int) ([]DashboardContributorRow, int, error)` — NEW
  - `SystemHealth() (DashboardSystemHealth, error)` — NEW (simple struct: db ok, total projects/contributors/chunks)

All of these are in-memory slicing of the already-built `dashboardReadModel`. Implementation cost is low.

**Types missing entirely from integrated (must add or simplify away)**:
- `ProjectSyncControl` — exists in cloudstore but not exposed via `DashboardStore`. Needed for admin project toggle. Must add to interface or keep admin projects page read-only for this change.
- `SystemHealthInfo` — not in integrated. Simple struct (DBConnected, TotalUsers, TotalSessions, TotalMemories, TotalPrompts, TotalMutations). Can be simplified to just `DashboardAdminOverview` (Projects, Contributors, Chunks).

**Simplification for this change**: The `AdminPage` component in legacy uses `SystemHealthInfo` with 7 fields. The integrated `AdminOverview()` returns `DashboardAdminOverview` (Projects, Contributors, Chunks). Adapt `AdminPage` to use `DashboardAdminOverview` instead. This gives visual parity for the metric strip without a new query.

---

## 4. Auth Bridge Surface

The integrated dashboard uses a `RequireSession func(r *http.Request) error` closure (passed via `MountConfig`) instead of per-handler middleware. There is NO username/userID/email in context. The legacy handlers read:

| Legacy call | Handler | Integrated bridge |
|---|---|---|
| `getUsernameFromContext(r)` | All full-page handlers | Must add `MountConfig.GetDisplayName(r *http.Request) string` closure, defaulting to "OPERATOR" |
| `getUserIDFromContext(r)` | All data handlers | NOT needed in integrated (no per-user scoping; all data is scope-filtered at `dashboardAllowedScopes` level) |
| `getEmailFromContext(r)` | `isAdmin()` | NOT needed in integrated (`isDashboardAdmin` already checks admin token via cookie) |
| `h.isAdmin(r)` | All full-page handlers | Already exists in integrated as `MountConfig.IsAdmin(r *http.Request) bool` |

**Helper additions needed**:
1. Add `GetDisplayName func(r *http.Request) string` to `MountConfig` (optional, defaults to "OPERATOR")
2. All handlers calling `getUsernameFromContext` must be changed to `h.cfg.GetDisplayName(r)`
3. In `cloudserver.go:routes()`, wire `GetDisplayName` from the session cookie (parse the admin token or use a static "OPERATOR" if no name is available from the bearer token)

**No JWT claims bridge needed** — the signed `engram_dashboard_token` cookie is already parsed by `authorizeDashboardRequest` via `dashboardBearerToken`. The bearer token itself is the `ENGRAM_CLOUD_TOKEN` (a static bearer) or the admin token. There is no username/email claim in this token flow.

---

## 5. Insecure-Mode Gap Analysis

### Gap 1: `createSessionCookie` is nil in insecure mode → login hangs

**File**: `cmd/engram/cloud.go:92–101` and `internal/cloud/cloudserver/cloudserver.go:167–171`

In `newCloudRuntime` (cloud.go:90): `insecureNoAuth := token == "" && envBool("ENGRAM_CLOUD_INSECURE_NO_AUTH")`. When `insecureNoAuth` is true, `authenticator` is set to `nil`.

In `cloudserver.go:routes()` (lines 168–171):
```go
if s.auth == nil {
    validateLoginToken = nil
    createSessionCookie = nil
}
```

And `MountConfig.CreateSessionCookie` is set to `nil`. In `dashboard.go:handleLoginSubmit` (integrated), when `CreateSessionCookie` is nil, the login silently succeeds (no cookie set), redirects to `/dashboard/`, then `authorizeDashboardRequest` is called:

```go
func (s *CloudServer) authorizeDashboardRequest(r *http.Request) error {
    if s.auth == nil {
        return nil  // <-- passes immediately, no cookie needed
    }
    ...
```

So in insecure mode: `RequireSession` returns `nil` immediately (no cookie check). The login flow sets NO cookie, redirects to `/dashboard/`, and `requireSession` passes because `RequireSession` returns nil. **This actually works** — insecure mode bypasses auth entirely.

BUT: `ValidateLoginToken` is also `nil` in insecure mode, which means the login form accepts ANY token (or empty token). The integrated `handleLoginSubmit` checks:
```go
if token == "" {
    renderLoginPage(w, "token is required", next)
    return
}
if h.cfg.ValidateLoginToken != nil {
    if err := h.cfg.ValidateLoginToken(token); err != nil { ... }
}
```
So empty token is blocked, but any non-empty token is accepted. This is correct for insecure mode.

**Actual gap**: The `/dashboard/login` page is EXPOSED in insecure mode. Submitting any token redirects to `/dashboard/` without setting a cookie. Since `RequireSession` returns `nil` immediately, the user sees the dashboard without a session. This works functionally but means the login page is decorative in insecure mode.

**Fix needed**: In insecure mode, redirect `GET /dashboard/login` directly to `/dashboard/` (skip login). The `handleLoginPage` already does this:
```go
if h.cfg.RequireSession != nil {
    if err := h.cfg.RequireSession(r); err == nil {
        http.Redirect(w, r, dashboardPostLoginPath(next), http.StatusSeeOther)
        return
    }
}
```
Since `RequireSession` always returns `nil` in insecure mode, this redirect fires immediately — the login page never renders. **Gap 1 is already handled** in the integrated code for the login page GET. But the `handleLoginSubmit` POST still fires and sets no cookie, which is fine.

### Gap 2: `dashboardSessionToken` returns `ErrDashboardSessionCodecRequired` when auth is nil

**File**: `internal/cloud/cloudserver/cloudserver.go:246–251`

```go
func (s *CloudServer) dashboardSessionToken(bearerToken string) (string, error) {
    if codec, ok := s.auth.(dashboardSessionCodec); ok {
        return codec.MintDashboardSession(bearerToken)
    }
    return "", ErrDashboardSessionCodecRequired
}
```

When `s.auth == nil`, the type assertion `s.auth.(dashboardSessionCodec)` panics on nil interface. Wait — actually in Go, a type assertion on a nil interface does NOT panic; it simply fails the assertion. So `ok` is `false` and `ErrDashboardSessionCodecRequired` is returned.

In `routes()`:
```go
if s.auth == nil {
    validateLoginToken = nil
    createSessionCookie = nil
}
```

`createSessionCookie` is nil, so `handleLoginSubmit` never calls `dashboardSessionToken`. **Gap 2 is NOT a runtime panic** — it's guarded by the nil-check in routes(). The gap only applies when `s.auth != nil` but does NOT implement `dashboardSessionCodec`. In practice, `auth.NewService` returns a value that DOES implement this interface.

**Real gap from memory #2375**: The gap was that in insecure mode the runtime constructs a JWT auth service unconditionally. Looking at `cloud.go:90–101`:
```go
insecureNoAuth := token == "" && envBool("ENGRAM_CLOUD_INSECURE_NO_AUTH")
var authenticator cloudserver.Authenticator
if !insecureNoAuth {
    authSvc, err := auth.NewService(cs, cfg.JWTSecret)
    ...
    authenticator = authSvc
}
```
This is already conditioned. `auth.NewService` is NOT called in insecure mode. The fix is already in place in the current branch. **Gap 2 as stated in memory #2375 may have been fixed** in the current `feat/integrate-engram-cloud` branch. Verify with tests.

**Remaining actual gap**: When `insecureNoAuth = false` but `ENGRAM_CLOUD_TOKEN` is set, `auth.NewService` is called, and `authSvc.SetDashboardSessionTokens([]string{cfg.AdminToken})` is called with potentially an empty `AdminToken`. This is safe (empty slice entry is ignored). No gap here.

**Conclusion**: The insecure-mode gaps from memory #2375 are largely mitigated in the current integrated branch. The remaining UX issue is that the login page in insecure mode redirects immediately (correct behavior), so no fix needed for visual parity.

---

## 6. HTMX Endpoint Inventory

### Endpoints invoked from legacy `components.templ`

| HTMX call | Target | Trigger | Registered in integrated? |
|---|---|---|---|
| `hx-get="/dashboard/stats"` | `#dashboard-stats` | `load` | YES (`GET /dashboard/stats`) |
| `hx-get="/dashboard/activity"` | `#activity-content` | `load` + `keyup` | YES (`GET /dashboard/activity`) |
| `hx-get="/dashboard/browser/observations"` | `#browser-content` | `load` + `keyup` + project select | YES |
| `hx-get="/dashboard/browser/sessions"` | `#browser-content` | tab click | YES |
| `hx-get="/dashboard/browser/prompts"` | `#browser-content` | tab click | YES |
| `hx-get="/dashboard/projects/list"` | `#projects-content` | `load` + `keyup` | NOT registered in integrated |
| `hx-get="/dashboard/projects/{name}/observations"` | `#project-content` | `load` | NOT registered in integrated |
| `hx-get="/dashboard/projects/{name}/sessions"` | `#project-content` | tab | NOT registered in integrated |
| `hx-get="/dashboard/projects/{name}/prompts"` | `#project-content` | tab | NOT registered in integrated |

### Endpoints invoked from legacy `layout.templ` (NavTabs component)

| HTMX call | Element | Integrated? |
|---|---|---|
| `hx-get` on all nav links | Shell nav tabs | Handled by `shellNavLink` in integrated dashboard.go (full page with HTMX push) |

### Missing routes that must be added to integrated `dashboard.go`

1. `GET /dashboard/projects/list` — projects list partial (HTMX target for search)
2. `GET /dashboard/projects/{name}/observations` — project observations partial
3. `GET /dashboard/projects/{name}/sessions` — project sessions partial
4. `GET /dashboard/projects/{name}/prompts` — project prompts partial
5. `GET /dashboard/admin/users` — admin users page
6. `GET /dashboard/admin/health` — admin health page
7. `POST /dashboard/admin/projects/{name}/sync` — sync toggle (admin action)
8. `GET /dashboard/sessions/{id}` — session detail (legacy: `GET /dashboard/sessions/{id}`)
9. `GET /dashboard/observations/{id}` — observation detail
10. `GET /dashboard/prompts/{id}` — prompt detail

Routes 8, 9, 10 require rich entity types (`CloudSession`, `CloudObservation`, `CloudPrompt`) that are NOT in the integrated read model. For visual parity, these can be simplified to show what the flat model provides. Detail pages showing `DashboardObservationRow` (no content field) would degrade gracefully.

**Alternative for 8–10**: Skip detail pages in this change scope (not invoked by HTMX automatically) and mark as follow-up. This preserves the strict visual parity for all browseable surfaces without requiring new query types.

---

## 7. Visual Parity Test Strategy

All tests must be deterministic and run with `go test ./...` without a browser.

### 7a. Asset Size Floor Tests
**Package**: `internal/cloud/dashboard`
**File**: `internal/cloud/dashboard/templ_policy_test.go` (add assertions there)

```
TestStaticAssetsExceedSizeFloors
- htmx.min.js: stat > 50_000 bytes
- pico.min.css: stat > 70_000 bytes
- styles.css: stat > 20_000 bytes
```

Use `fs.Stat` on `StaticFS` (the embedded FS) to verify sizes without reading files.

### 7b. Templ Determinism Checks
**Package**: `internal/cloud/dashboard`
**File**: `internal/cloud/dashboard/templ_policy_test.go`

```
TestTemplGeneratedFilesAreCheckedIn
- Verify components_templ.go, layout_templ.go, login_templ.go exist on disk
- Verify each contains "// Code generated by templ" header
- Verify sizes exceed minimums (components_templ.go > 100_000 bytes)
```

Use `os.Stat` relative to `runtime.Caller(0)` path.

### 7c. HTML Structural Assertions
**Package**: `internal/cloud/dashboard`
**Test function**: `TestDashboardHTMLStructuralParity`

Mount the dashboard with `MountConfig{RequireSession: allowAll, Store: minimalStub, IsAdmin: alwaysTrue}` and make GET requests, then assert:

```
TestLayoutContainsVisualParityMarkers — GET /dashboard/
- body contains: class="shell-body"
- body contains: class="shell-backdrop"
- body contains: class="app-shell"
- body contains: class="shell-header"
- body contains: class="brand-stack"
- body contains: class="shell-nav"
- body contains: class="shell-main"
- body contains: class="shell-footer"
- body contains: "ENGRAM CLOUD / SHARED MEMORY INDEX / LIVE SYNC READY"
- body contains: "An elephant never forgets."

TestStatusRibbonPresent — GET /dashboard/
- body contains: class="status-ribbon"
- body contains: class="status-pill"
- body contains: "CLOUD ACTIVE"
- body contains: "shared memory index"

TestNavTabsPresent — GET /dashboard/ (isAdmin=true)
- body contains: href="/dashboard/"
- body contains: href="/dashboard/browser"
- body contains: href="/dashboard/projects"
- body contains: href="/dashboard/contributors"
- body contains: href="/dashboard/admin"

TestFooterParity — GET /dashboard/
- body contains: "ENGRAM CLOUD"
- body contains: "SHARED MEMORY INDEX"
- body contains: "LIVE SYNC READY"
```

### 7d. Copy Parity String Tests

```
TestLoginPageCopyParity — GET /dashboard/login
- "CLOUD ACTIVE"
- "Engram Cloud"
- name="token"

TestDashboardHeroCopy — GET /dashboard/ (after auth)
- "OPERATOR" or username placeholder
- DashboardHome renders hero-panel with section-kicker

TestBrowserPageKicker — GET /dashboard/browser
- "KNOWLEDGE BROWSER"

TestProjectsPageKicker — GET /dashboard/projects
- "PROJECT ATLAS" (or "PROJECTS" if simplified)

TestContributorsPageKicker — GET /dashboard/contributors
- "CONTRIBUTOR SIGNAL"

TestAdminPageKicker — GET /dashboard/admin
- "ADMIN SURFACE"
```

### 7e. HTMX Attribute Presence Tests

```
TestDashboardHomeHTMXWiring — GET /dashboard/
- rendered HTML contains: hx-get="/dashboard/stats"
- rendered HTML contains: hx-trigger="load"

TestBrowserHTMXWiring — GET /dashboard/browser
- hx-get="/dashboard/browser/observations"
- hx-get="/dashboard/browser/sessions"
- hx-get="/dashboard/browser/prompts"
- hx-target="#browser-content"
- hx-include="#browser-project"

TestProjectsHTMXWiring — GET /dashboard/projects
- hx-get="/dashboard/projects/list"
- hx-target="#projects-content"
- hx-trigger="keyup"
```

### 7f. Recommended Test Names + Packages

| Test name | Package |
|---|---|
| `TestStaticAssetsExceedSizeFloors` | `internal/cloud/dashboard` |
| `TestTemplGeneratedFilesAreCheckedIn` | `internal/cloud/dashboard` |
| `TestDashboardLayoutHTMLStructure` | `internal/cloud/dashboard` |
| `TestStatusRibbonAndFooterPresent` | `internal/cloud/dashboard` |
| `TestNavTabsRenderedCorrectly` | `internal/cloud/dashboard` |
| `TestLoginPageTokenFormAndCopy` | `internal/cloud/dashboard` |
| `TestDashboardHomeHTMXWiring` | `internal/cloud/dashboard` |
| `TestBrowserPageHTMXWiring` | `internal/cloud/dashboard` |
| `TestProjectsPageHTMXWiring` | `internal/cloud/dashboard` |
| `TestAdminPageSurfacePresent` | `internal/cloud/dashboard` |
| `TestCopyParityStrings` | `internal/cloud/dashboard` |

All tests use `httptest.NewRecorder` + `mux.ServeHTTP`. No browser. No external deps.

---

## 8. Risks

1. **`templ generate` binary required**: Generated `*_templ.go` files must be checked in (per `templ_policy.go`). If the CI environment does not have `templ` installed, the generated files must be committed manually. The `templ` binary version must match `go.mod` dependency (`v0.3.1001`). Risk: stale generated files cause runtime panics.

2. **`github.com/a-h/templ` missing from `go.mod`**: The integrated `go.mod` does NOT have `github.com/a-h/templ`. Adding it changes the module graph and may require `go mod tidy` to resolve transitive dependencies. Risk: build fails until dependency is added and generated files match the new runtime version.

3. **Type mismatch between `DashboardXxxRow` (flat) and legacy component expectations (rich)**: The legacy `components.templ` uses rich types (`CloudObservation.Content`, `CloudSession.EndedAt`, `CloudSession.Summary`, `CloudObservation.TopicKey`, etc.) that are NOT available in the flat `DashboardObservationRow`. Every usage must be mapped. Risk: missed field causes compile error or blank UI.

4. **Static embed path must remain `static/`**: The `//go:embed static` directive in `embed.go` assumes `static/` exists as a directory relative to the package. If files are renamed or the directory is restructured, the embed silently returns an empty FS. The current structure is correct — risk is low if the copy is direct.

5. **Pagination design difference**: The legacy `components.templ` uses `HtmxPaginationBar` (a templ component with dynamic hx-get) and `PaginationBar` (regular links). The integrated dashboard currently uses `HtmxPaginationBar` rendered as raw string inside handler functions (not as templ). After porting, the templ version must be invoked correctly. Risk: mismatched rendering path causes double-rendered pagination or missing navigation.

6. **Admin toggle route `POST /dashboard/admin/projects/{name}/sync`**: This mutation route needs `ProjectSyncControl` types. The integrated cloudstore likely has `SetProjectSyncEnabled` but it may not be plumbed to `CloudStore` (only to the old SQL store). Risk: admin sync toggle silently fails or panics if the method doesn't exist on the chunk-based `CloudStore`.

7. **`MountConfig.GetDisplayName` is new surface**: Adding a new field to `MountConfig` is a breaking change for any callers that construct the struct literally. Since cloudserver is the only caller (in `routes()`), this is manageable, but must be remembered when updating cloudserver.

---

## 9. Recommended Implementation Sequence

### Phase: Propose
Confirm the type adaptation decisions:
- Settle on `DashboardXxxRow` as the sole rendering types (no rich `CloudXxx` types)
- Confirm detail pages (observation/session/prompt detail) are in or out of scope
- Confirm `AdminPage` uses `DashboardAdminOverview` instead of `SystemHealthInfo`
- Confirm `GetDisplayName` approach vs static "OPERATOR"

### Phase: Spec
Write delta spec:
- Visual parity acceptance criteria (asset sizes, UI strings, HTMX wiring)
- Type mapping table (legacy → integrated)
- New `DashboardStore` interface methods
- `MountConfig` additions

### Phase: Design
Technical decisions:
- Where pagination lives (in-memory slicing in `dashboard_queries.go` vs handler-level)
- Where `GetDisplayName` is wired in cloudserver
- Whether `ProjectSyncControl` is added to `DashboardStore` or admin pages remain read-only
- `go.mod` update plan (templ dependency addition)

### Phase: Tasks
Split into apply batches:

**Batch 1 (foundation — RED tests first)**:
1. Add `github.com/a-h/templ` to `go.mod`
2. Write `TestStaticAssetsExceedSizeFloors` — FAILS (assets are stubs)
3. Write `TestTemplGeneratedFilesAreCheckedIn` — FAILS (no generated files)
4. Write `TestDashboardLayoutHTMLStructure` — FAILS (no `shell-footer`, no `status-ribbon`)
5. Write `TestStatusRibbonAndFooterPresent` — FAILS
6. Write copy parity tests — FAILS

**Batch 2 (static assets — GREEN batch 1 tests)**:
7. Replace `static/htmx.min.js` with legacy (50 KB)
8. Replace `static/pico.min.css` with legacy (71 KB)
9. Replace `static/styles.css` with legacy (23 KB)
10. Batch 1 asset floor tests go GREEN

**Batch 3 (templ port — GREEN component tests)**:
11. Add new pagination methods to `DashboardStore` interface + `CloudStore` implementation
12. Add `GetDisplayName` to `MountConfig`
13. Replace `components.templ` (adapted: DashboardXxxRow types, no rich CloudXxx)
14. Replace `layout.templ` (adapted: GetDisplayName, status-ribbon, footer)
15. Replace `login.templ` (add `next` param, rich copy)
16. Replace `helpers.go` (pagination, formatting, badge utilities)
17. Run `templ generate` and commit generated `*_templ.go`
18. Update `dashboard.go` handlers to call templ components instead of raw strings
19. Add missing routes: `/dashboard/projects/list`, `/dashboard/projects/{name}/observations|sessions|prompts`, `/dashboard/admin/users`, `/dashboard/admin/health`, `POST /dashboard/admin/projects/{name}/sync`
20. Wire `cloudserver.go:routes()` with `GetDisplayName`
21. All batch 2/3 tests go GREEN

**Batch 4 (verify + polish)**:
22. Run `go test ./...` — all green
23. Verify no regression in existing `cloudserver_test.go` + `dashboard_test.go`
24. Visual spot-check with `go run . cloud serve` in insecure mode
25. Archive

---

## Appendix: File-by-File Summary

| File | Action | Effort |
|---|---|---|
| `static/htmx.min.js` | REPLACE (verbatim copy) | trivial |
| `static/pico.min.css` | REPLACE (verbatim copy) | trivial |
| `static/styles.css` | REPLACE (verbatim copy) | trivial |
| `components.templ` | REPLACE + adapt types | medium |
| `layout.templ` | REPLACE + adapt GetDisplayName | small |
| `login.templ` | REPLACE + add next param | small |
| `helpers.go` | REPLACE (add pagination/formatting) | small |
| `dashboard.go` | SURGICAL (add routes, switch to templ calls, GetDisplayName) | medium |
| `middleware.go` | KEEP | none |
| `config.go` | KEEP | none |
| `embed.go` | KEEP | none |
| `templ_policy.go` | KEEP | none |
| `components_templ.go` | GENERATE (after templ generate) | trivial |
| `layout_templ.go` | GENERATE | trivial |
| `login_templ.go` | GENERATE | trivial |
| `dashboard_queries.go` | ADD paginated methods + DashboardSystemHealth | small |
| `go.mod` | ADD github.com/a-h/templ v0.3.1001 | trivial |
| `cloudserver.go` | WIRE GetDisplayName in MountConfig | small |
