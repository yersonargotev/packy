# Design: cloud-dashboard-visual-parity

## Context

This document is the technical design layer for the change `cloud-dashboard-visual-parity`. It resolves every open implementation question flagged by the spec phase and locks the rollout plan.

References:
- Proposal: `openspec/changes/cloud-dashboard-visual-parity/proposal.md`
- Exploration: `openspec/changes/cloud-dashboard-visual-parity/exploration.md`
- Spec (delta): `openspec/changes/cloud-dashboard-visual-parity/specs/cloud-dashboard/spec.md`
- Copy strategy (engram): `sdd/cloud-dashboard-visual-parity/copy-strategy`

Key platform facts that shape the design:
- Integrated `CloudStore` is backed by **Postgres** (`pgx` + `database/sql`, DSN in `cloud.Config.DSN`). The proposal erroneously referenced a SQLite shape; the design corrects this.
- The integrated dashboard is mounted via `dashboard.Mount(mux, MountConfig{...})` — a closure-based auth bridge (no middleware-injected request context).
- Go 1.22 `net/http.ServeMux` pattern-matching is already in use (`GET /dashboard/projects/{project}`), so new routes MUST follow the same registration style.
- `templ_policy.go` already declares `checked-in-generated` mode with a `go:generate templ generate` directive on `templ_policy.go`. Generated files are the source of truth at runtime.

## Technical Approach

The work is a verbatim port of the legacy `engram-cloud` dashboard (95%) plus a small adapter seam (5%). It is organized as four concentric layers, each testable in isolation:

1. **Asset layer** (`static/*`): embedded binary assets copied byte-for-byte from legacy. Proved real by `fs.Stat`-based floor assertions.
2. **Templ layer** (`*.templ` + generated `*_templ.go`): port legacy components verbatim, with import-path adaptation and flat-row type substitution. Generated files are committed. A `templ_policy` test verifies presence and generator header.
3. **Store layer** (`cloudstore`): extend `DashboardXxxRow` types additively with payload fields already present in chunk JSON; add a `cloud_project_controls` table via the existing `migrate()` sequence; add `SystemHealth()` and paginated list helpers built as in-memory slicing of the existing read model.
4. **Handler / mount layer** (`dashboard.go` + `cloudserver.go`): replace string-concat renders with templ component calls, register the 11 missing routes, wire a `principalFromRequest` helper that maps cookie → `Principal`, and insert a push-path pause guard in `handlePushChunk`.

Data flow is uniform across tabs:

```
HTTP request
  → authorizeDashboardRequest (cookie → bearer → auth service)
  → h.cfg.RequireSession      (dashboard closure returns err or nil)
  → handler derives Principal via principalFromRequest(r)
  → handler reads store (CloudStore.loadDashboardReadModel → slice)
  → templ component renders (Layout(...) wraps partial)
  → renderHTML(w, ...) / isHTMXRequest short-circuit
```

The RED-first test discipline drives every batch: each batch opens with a failing test that concretely pins the public contract, then implementation flips it GREEN. Apply batches are ordered so later layers cannot compile without earlier ones.

## Architecture Decisions

The ten decisions below resolve the spec-phase open questions in order.

### Decision 1: Pagination placement — in-memory slicing in `dashboard_queries.go`

- **Decision**: All paginated list methods slice the in-memory `dashboardReadModel` cache inside `cloudstore/dashboard_queries.go`. The handler layer only parses `limit` and `cursor` (integer `offset`) from the query string and passes them through. No `LIMIT/OFFSET` SQL is added.
- **Rationale**: The read model is already fully materialized on first call and cached under `cs.dashboardReadModelMu`. Legacy `engram-cloud` used SQL `LIMIT/OFFSET` because its data lived in chunk-indexed SQL tables; the integrated store has no such tables — it reconstructs every list from the chunk payload each cache miss. Issuing SQL pagination would force either (a) a second deterministic sort at the database layer, which does not exist yet, or (b) a fallback to in-memory slicing anyway. Slicing inside the store keeps sort stability in ONE place and keeps handlers thin. At single-operator scale (tens of thousands of chunks max) the full read-model build is already on the order of milliseconds per cache miss; pagination latency is memory-bound.
- **Trade-offs**: In-memory slicing means the full read model is always loaded even for `limit=10`. This is acceptable because the alternative — SQL-based pagination over a live query — would require a completely different read path and does not benefit a single-operator workload. Sort stability is guaranteed because the read model already sorts deterministically (`CreatedAt DESC, SessionID ASC`, etc.).
- **Impact**:
  - `internal/cloud/cloudstore/dashboard_queries.go`: add five `ListXxxPaginated(...) ([]Row, int, error)` methods. Each returns `(page, total, err)`.
  - `internal/cloud/cloudstore/dashboard_queries_test.go`: extend with `TestDashboardPaginationSortStability` — seeds 50 rows across two projects and verifies page 3 / size 10 always returns the same SessionIDs across reinvocations.
  - Handler layer does NOT change sort behavior.

### Decision 2: SQL migration sequencing — existing `CloudStore.migrate()` + new Postgres statement

- **Decision**: Add the `cloud_project_controls` table creation as an additional `CREATE TABLE IF NOT EXISTS` entry in the existing `CloudStore.migrate()` queries slice in `internal/cloud/cloudstore/cloudstore.go`. Schema (Postgres-native, derived from legacy `engram-cloud/internal/cloud/cloudstore/schema.go`):

  ```sql
  CREATE TABLE IF NOT EXISTS cloud_project_controls (
      project       TEXT PRIMARY KEY,
      sync_enabled  BOOLEAN NOT NULL DEFAULT TRUE,
      paused_reason TEXT,
      updated_by    TEXT,
      updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
  )
  ```

  The legacy schema uses `UUID REFERENCES cloud_users(id) ON DELETE SET NULL` for `updated_by`. The integrated change drops the FK and stores a free-text display name — the integrated single-operator model does not link a user row to every admin action. Also add `CREATE INDEX IF NOT EXISTS idx_cloud_project_controls_enabled ON cloud_project_controls(sync_enabled)` to match legacy.

  Insertion point: append two statements to the existing `queries` slice in `migrate()` (after the `cloud_project_sessions` block, before `backfillProjectSessionsFromChunks`).

- **Rationale**: `migrate()` is the existing, tested migration mechanism. It is idempotent (every statement is `IF NOT EXISTS` or `DO $$ BEGIN ... END $$`). Adding a dedicated `migrations/` mechanism would be architecture inflation for a one-table addition. First boot of an existing deployment: the new statements run, the table is created, and no existing rows are touched. Rollback: `DROP TABLE cloud_project_controls` is a single statement.

- **Trade-offs**: The proposal mistakenly described this as SQLite (`CREATE TABLE IF NOT EXISTS ... BOOLEAN NOT NULL DEFAULT 1`). Postgres uses `BOOLEAN ... DEFAULT TRUE` and `TIMESTAMPTZ`. This is a spec correction, not a scope change.

- **Impact**:
  - `internal/cloud/cloudstore/cloudstore.go`: +2 lines in the `queries` slice inside `migrate()`.
  - `internal/cloud/cloudstore/project_controls.go` (new): `ProjectSyncControl` struct + four methods (`IsProjectSyncEnabled`, `SetProjectSyncEnabled`, `GetProjectSyncControl`, `ListProjectSyncControls`). Adapted from legacy with the FK drop and minor SQL rewrites: the legacy `ListProjectSyncControls` references tables (`cloud_sessions`, `cloud_observations`, `cloud_prompts`) that do NOT exist in the integrated store. The integrated version derives the project set from `cloud_chunks` (`SELECT DISTINCT project_name FROM cloud_chunks`) UNION `cloud_project_controls`. Exact SQL lives in the new file, not here.
  - `internal/cloud/cloudstore/project_controls_test.go` (new): `TestProjectSyncControlPersists`, `TestProjectSyncControlUnknownProjectDefaultsEnabled`, `TestProjectSyncControlListIncludesKnownChunkProjects`.

### Decision 3: Composite-ID URL scheme — `net/http.ServeMux` Go 1.22 patterns

- **Decision**: Detail routes use the URL scheme:

  - `GET /dashboard/sessions/{project}/{sessionID}`
  - `GET /dashboard/observations/{project}/{sessionID}/{chunkID}`
  - `GET /dashboard/prompts/{project}/{sessionID}/{chunkID}`

  All three register as `mux.HandleFunc("GET /dashboard/sessions/{project}/{sessionID}", h.requireSession(h.handleSessionDetail))` following the existing style in the integrated `dashboard.go` (`/dashboard/projects/{project}`). Path values are extracted with `r.PathValue(name)`, validated via `strings.TrimSpace(...) != ""` and `store.NormalizeProject(project)` for the project segment. ChunkIDs are validated as non-empty + `len <= 128` (max hex SHA hash length in the integrated `chunkcodec`).

- **Rationale**: `net/http.ServeMux` Go 1.22+ patterns natively support multi-segment routes. The existing codebase already uses them (`GET /dashboard/projects/{project}`). Using a non-stdlib router would break the established style without benefit. The scheme is deliberately chosen so the composite key order matches the URL order: reading left-to-right gives project → session → chunk, which mirrors the dashboard navigation hierarchy.

- **URL collision check**: The existing routes `/dashboard/sessions/...` and `/dashboard/observations/...` / `/dashboard/prompts/...` do NOT exist today — the existing legacy-port has `/dashboard/browser/sessions/{sessionID}` (single-segment, under `/browser/`). The new routes live at a different prefix and cannot collide. The existing `/dashboard/browser/sessions/{sessionID}` route REMAINS for HTMX-driven partial loads inside the browser tab; the new `/dashboard/sessions/{project}/{sessionID}` is the full-page detail route triggered by list-view links.

- **Trade-offs**: Composite URLs are longer than legacy numeric IDs. A future addition could introduce a short hash-based ID (e.g., `base64url(hash(project||sessionID||chunkID))`) if URL length becomes a problem, but single-operator dashboards are not bandwidth-constrained.

- **Impact**: Three new routes in `dashboard.go:Mount` + three new handlers (`handleSessionDetail`, `handleObservationDetail`, `handlePromptDetail`). Three new templ components (`SessionDetailPage`, `ObservationDetailPage`, `PromptDetailPage`) ported from legacy and adapted to flat-row types. Three route-level tests in `dashboard_test.go`.

### Decision 4: Templ bootstrap — pinned version + `tools.go` + Makefile target + DOCS.md

- **Decision**:
  - Pin `github.com/a-h/templ v0.3.1001` in `go.mod` as a direct dependency (matches legacy `engram-cloud` version exactly — generated output will be byte-identical when re-generated by a contributor).
  - Create `tools/tools.go` with `//go:build tools` and `//go:generate` kept out of that file; its only purpose is to keep `templ` in `go.sum`:
    ```go
    //go:build tools
    // +build tools

    package tools

    import (
        _ "github.com/a-h/templ/cmd/templ"
    )
    ```
  - `go:generate` directive lives in `internal/cloud/dashboard/templ_policy.go` (already present: `//go:generate templ generate`). No change needed there.
  - Add a `Makefile` target:
    ```makefile
    templ:
    	go tool templ generate ./internal/cloud/dashboard/...
    ```
  - Contributor setup is documented in `DOCS.md` under a new "Dashboard templ regeneration" subsection. DOCS.md (not ARCHITECTURE.md) is chosen because DOCS.md is the contributor-facing quickstart; ARCHITECTURE.md describes invariants and boundaries, not commands. README.md gets a one-line pointer to the DOCS.md section (per the proposal).

- **Rationale**: Pinning the templ version is critical — a contributor regenerating with a different version produces diff churn and breaks byte-level equality of committed generated files. The `tools/tools.go` pattern is the canonical Go-way to track dev-only binaries. Making `go tool templ generate` the invocation (rather than `templ generate`) uses Go 1.22+'s `go tool` resolution so contributors don't need a separate `templ` install.

- **Trade-offs**: `go tool` support only works cleanly for modules that have been pulled via `go mod tidy`. First-time contributors must run `go mod download` before `make templ`. This is documented explicitly.

- **Impact**:
  - `go.mod`: +1 direct dep (+1 transitive, `github.com/a-h/parse` v0.0.0-20...).
  - `go.sum`: regenerated by `go mod tidy`.
  - `tools/tools.go`: new file (~10 L).
  - `Makefile`: +3 lines.
  - `DOCS.md`: +1 subsection (~15 L).
  - `README.md`: +1 line pointing to DOCS.md.

### Decision 5: Push-path pause check — inside `handlePushChunk`, before `WriteChunk`, uncached

- **Decision**: Insert the pause check in `internal/cloud/cloudserver/cloudserver.go:handlePushChunk` immediately after `authorizeProjectScope(w, project)` succeeds (which is line ~380 in the current file) and before `WriteChunk`:

  ```go
  if storeForControls, ok := s.store.(interface {
      IsProjectSyncEnabled(project string) (bool, error)
  }); ok {
      enabled, err := storeForControls.IsProjectSyncEnabled(project)
      if err != nil {
          writeActionableError(w, http.StatusInternalServerError,
              constants.UpgradeErrorClassBlocked,
              constants.UpgradeErrorCodeInternal,
              fmt.Sprintf("check project control: %v", err))
          return
      }
      if !enabled {
          writeActionableError(w, http.StatusConflict,
              constants.UpgradeErrorClassPolicy,
              "sync-paused",
              fmt.Sprintf("sync is paused for project %q", project))
          return
      }
  }
  ```

  Uncached: the Postgres query `SELECT sync_enabled FROM cloud_project_controls WHERE project = $1` is indexed on the primary key, sub-millisecond. The `ChunkStore` interface is NOT extended — the check uses a structural type assertion (`interface { IsProjectSyncEnabled(string) (bool, error) }`) so test doubles that don't implement it continue to work and the push-path falls through to the existing behavior.

- **Rationale**: Placing the check after `authorizeProjectScope` means pause is a second-layer gate, distinct from access control — admin can pause a project they CAN access. The `writeActionableError` helper already emits the standardized `{error_class, error_code, error}` JSON shape; setting `error_code = "sync-paused"` matches spec REQ-109 exactly. Using a structural interface (not adding to `ChunkStore`) preserves test ergonomics: existing fake stores in `cloudserver_test.go` need zero changes.

- **Trade-offs**: Uncached means every push issues one extra indexed SELECT. At observed push rates (seconds-to-minutes apart) this is negligible. Caching (e.g., TTL map) would add complexity and a cache-invalidation vector when the admin toggles — worse than the observed cost.

- **Impact**:
  - `internal/cloud/cloudserver/cloudserver.go`: +~20 lines in `handlePushChunk`.
  - `internal/cloud/cloudserver/cloudserver_test.go`: new test `TestPushPathPauseEnforcement` — creates a `fakeStore` that implements `IsProjectSyncEnabled` returning `false`, POSTs to `/sync/push`, asserts 409 + body contains `"sync-paused"`.

### Decision 6: Principal bridge — new `principal.go` with a small `Principal` type

- **Decision**: Create a new file `internal/cloud/dashboard/principal.go` with:

  ```go
  type Principal struct {
      displayName string
      isAdmin     bool
  }

  func (p Principal) DisplayName() string {
      if strings.TrimSpace(p.displayName) == "" {
          return "OPERATOR"
      }
      return p.displayName
  }

  func (p Principal) IsAdmin() bool { return p.isAdmin }

  func (h *handlers) principalFromRequest(r *http.Request) Principal {
      name := ""
      if h.cfg.GetDisplayName != nil {
          name = strings.TrimSpace(h.cfg.GetDisplayName(r))
      }
      admin := false
      if h.cfg.IsAdmin != nil {
          admin = h.cfg.IsAdmin(r)
      }
      return Principal{displayName: name, isAdmin: admin}
  }
  ```

  `MountConfig` gains one optional field:
  ```go
  type MountConfig struct {
      // ... existing fields ...
      GetDisplayName func(r *http.Request) string
  }
  ```

  Default wiring in `internal/cloud/cloudserver/cloudserver.go:routes()`:
  ```go
  GetDisplayName: func(r *http.Request) string { return "OPERATOR" },
  ```
  Until the session codec surfaces a display name (out of scope), the production wiring literally returns `"OPERATOR"`. When `MountConfig.GetDisplayName` is `nil`, `principalFromRequest` falls back to `""`, which `DisplayName()` then converts to `"OPERATOR"`. Both paths converge on the same fallback string — single source of truth.

- **Rationale**: A dedicated file (`principal.go` not `helpers.go`) because:
  1. `helpers.go` is already targeted for a full replace (port of legacy 424-line version). Keeping the bridge in a separate file avoids merge churn.
  2. Principal is a named concept in the auth contract; it earns its own file.
  3. The type stays intentionally tiny — no new closures stored, no new lifecycle. It is a read-only view over MountConfig closures.

  `principalFromRequest` is a method on `*handlers` (not a free function) so every handler writes `p := h.principalFromRequest(r)` and never reads `r.Context()` for identity. This matches spec REQ-113.

- **Trade-offs**: Adding `GetDisplayName` to `MountConfig` is technically a breaking change for any caller that uses positional struct literals. Audit: the only caller outside tests is `cloudserver.go` — already uses named fields. Tests use named fields. No breakage.

- **Impact**:
  - `internal/cloud/dashboard/principal.go` (new): ~30 L.
  - `internal/cloud/dashboard/dashboard.go`: add `GetDisplayName` field to `MountConfig` struct (+1 line); every handler call that previously fabricated a display name now writes `p := h.principalFromRequest(r)`.
  - `internal/cloud/cloudserver/cloudserver.go`: +1 line in the `dashboard.Mount` call.
  - `internal/cloud/dashboard/dashboard_test.go`: new test `TestGetDisplayNameFallback` — assert `principalFromRequest(r).DisplayName() == "OPERATOR"` when `MountConfig.GetDisplayName == nil`; assert custom closure surfaces.

### Decision 7: Test package layout — `golang.org/x/net/html` tokenizer for structural assertions, substring match for copy and HTMX attributes

- **Decision**:
  - Structural tests (`TestDashboardLayoutHTMLStructure`, `TestStatusRibbonAndFooterPresent`, `TestNavTabsRenderedCorrectly`): use `golang.org/x/net/html`'s `Tokenizer` API (NOT `goquery`). The tokenizer walks the byte stream and matches `class` attribute values; zero DOM construction, zero CSS-selector surface.
  - Copy parity tests (`TestCopyParityStrings`, `TestLoginPageTokenFormAndCopy`) and HTMX attribute presence tests (`TestDashboardHomeHTMXWiring`, `TestBrowserPageHTMXWiring`, `TestProjectsPageHTMXWiring`): substring match via `strings.Contains(body, "CLOUD ACTIVE")`. Fast, deterministic, zero new deps.
  - Route-level tests (`TestFullHTMXEndpointSurface`, `TestAdminSyncTogglePosts`, etc.): `httptest.NewRecorder` + `mux.ServeHTTP` — already the established pattern in `dashboard_test.go`.

- **Rationale**: The spec requires deterministic rendered-HTML assertions. Three candidate approaches exist:
  1. **`goquery`**: CSS selectors, pleasant ergonomics, but pulls in a jQuery-like DOM surface and a large transitive dep tree. Overkill for a handful of class-name matches.
  2. **`golang.org/x/net/html` tokenizer**: already in the module graph transitively (via `net/http` HTTP/2 stack is a different module but the `x/net/html` package is a tiny focused dep). Gives a streaming walk over the HTML without constructing a tree. Perfect for "find me all elements with `class="shell-footer"`".
  3. **Substring match**: adequate for copy and attribute presence but fails for class-name uniqueness (false positives if the string appears inside a JS string literal, e.g., `htmx.min.js` contains `"shell-..."`).

  We use tokenizer for the THREE structural tests that need class-name precision, and substring match everywhere else. Lowest dep surface that still gives deterministic results.

- **Trade-offs**: Adding `golang.org/x/net/html` to `go.mod` is one more direct dep. `go mod tidy` confirms this module is the only change beyond templ. Zero `goquery`, zero `cascadia`.

- **Impact**:
  - `go.mod`: +1 direct dep (`golang.org/x/net v0.x.x`).
  - `internal/cloud/dashboard/dashboard_test.go`: new helpers `hasElementWithClass(body, tag, class) bool` and `countElementsWithClass(body, class) int` wrapping the tokenizer.

### Decision 8: CI gate for templ determinism — hash-compare checked-in files against expected floor + generator-header regex

- **Decision**: `TestTemplGeneratedFilesAreCheckedIn` does NOT shell out to `templ generate`. Instead it does three hash-free checks:

  1. `os.Stat` on each of `components_templ.go`, `layout_templ.go`, `login_templ.go` in the package directory (resolved via `runtime.Caller` → relative path).
  2. Read the first 1024 bytes of each and assert the header line matches `^// Code generated by templ .+ DO NOT EDIT\.$` — this confirms the file is templ-generated and NOT an accidental hand-written `.go`.
  3. Assert `components_templ.go` is at least 100 000 bytes (the legacy generated output is ~240 000 B; the floor catches an empty/trivial regeneration).

  Explicit failure message:
  ```
  components_templ.go size = 1234 bytes; expected >= 100000 bytes.
  Possible cause: the *.templ sources changed but the generated *_templ.go files
  were not committed. Run `make templ` (or `go tool templ generate ./internal/cloud/dashboard/...`)
  and commit the regenerated files.
  ```

- **Rationale**: Shelling out to `templ generate` in CI requires the binary to be installed in the runner environment. With `tools/tools.go` and `go tool templ` it WOULD work, but that couples test pass/fail to `go tool` resolution — a flake vector. Hash-compare would require pinning the generator's exact output, which breaks when the templ generator itself fixes a whitespace bug. Size floor + header regex is coarse but robust: it catches the two realistic failure modes (file missing, file regenerated empty) without false positives.

- **Trade-offs**: A contributor who legitimately reduces the `components.templ` surface below 100 000 bytes would need to lower the floor. This is a known, rare maintenance cost.

- **Impact**:
  - `internal/cloud/dashboard/templ_policy_test.go` (new): `TestTemplGeneratedFilesAreCheckedIn`, `TestStaticAssetByteFloors` (Decision 9).

### Decision 9: Static asset regression — read bytes from the `//go:embed` FS (`StaticFS`), authoritative

- **Decision**: `TestStaticAssetByteFloors` reads sizes via `fs.Stat(StaticFS, "static/htmx.min.js")` etc — the embedded filesystem. NOT from disk.

- **Rationale**: The embedded FS is what the binary actually serves at runtime. If a contributor updates the disk file but rebuilds without rerunning `go build`, the old bytes remain embedded — a real regression class. `//go:embed` is authoritative because it is the served artifact. Disk bytes are a build-time input; embed bytes are a runtime truth. Floor: `htmx.min.js >= 40_000`, `pico.min.css >= 60_000`, `styles.css >= 20_000` (per spec REQ-100).

- **Trade-offs**: The test requires a re-`go build` after a disk edit to catch the new size. Acceptable — the test runs in `go test` which rebuilds packages.

- **Impact**: `internal/cloud/dashboard/templ_policy_test.go` (new): `TestStaticAssetByteFloors`.

### Decision 10: Apply batch ordering — five batches, each opens with RED tests

- **Decision**: Five sequential apply batches. Each batch begins with RED test writes, ends with those tests going GREEN plus new code covered. A later batch MUST NOT compile without the earlier batch merged.

  **Batch 1 — Templ bootstrap + static assets + asset-floor test (RED first)**
  - [R] Write `TestStaticAssetByteFloors` (fails — current htmx.min.js is 42 B).
  - [R] Write `TestTemplGeneratedFilesAreCheckedIn` (fails — no `components_templ.go` yet, or is trivial).
  - Add `github.com/a-h/templ v0.3.1001` to `go.mod`, add `golang.org/x/net` (`go.mod` touch, `go mod tidy`).
  - Create `tools/tools.go`.
  - Add `Makefile` target `templ`.
  - Copy `htmx.min.js`, `pico.min.css`, `styles.css` verbatim from legacy.
  - Tests go GREEN for asset floors; `TestTemplGeneratedFilesAreCheckedIn` stays RED (waits for Batch 2).
  - **Depends on**: nothing.
  - **Blocks**: Batch 2 (generated files require the dep), Batch 3 (cloudstore tests use the new go.mod).

  **Batch 2 — Templ sources port + generated files + structural test (RED first)**
  - [R] Write `TestDashboardLayoutHTMLStructure`, `TestStatusRibbonAndFooterPresent`, `TestNavTabsRenderedCorrectly`, `TestLoginPageTokenFormAndCopy`, `TestCopyParityStrings` (all fail).
  - Copy `components.templ` (1134 L) from legacy, adapt imports (`github.com/Gentleman-Programming/engram/internal/cloud/cloudstore`) and type references (flat `DashboardXxxRow` instead of legacy rich types; see Batch 3).
  - Copy `layout.templ` (52 L) from legacy — ONE adaptation: swap `username string` parameter for accepting the pre-rendered display name from `Principal.DisplayName()` call site.
  - Copy `login.templ` (63 L) from legacy — adaptation: accept `next string` second parameter; handler renders `LoginPage(errorMsg, next)`.
  - Run `go tool templ generate ./internal/cloud/dashboard/...` and commit the generated `*_templ.go`.
  - Tests go GREEN.
  - **Depends on**: Batch 1 (templ binary + assets).
  - **Blocks**: Batch 3 (handler renders depend on templ component names), Batch 4 (route handlers call templ Layout).

  **Batch 3 — Row-type extensions + cloudstore additions + handler Principal bridge (RED first)**
  - [R] Write `TestDashboardRowDetailFields` (asserts `Content`, `TopicKey`, `ToolName`, `EndedAt`, `Summary`, `Directory`, `ChunkID` fields exist on the row types and are populated from seeded chunks — fails because fields are missing).
  - [R] Write `TestCloudstoreSystemHealthAggregates` (fails — no `SystemHealth()` method).
  - [R] Write `TestProjectSyncControlPersists` + `TestProjectSyncControlUnknownProjectDefaultsEnabled` (fail — no table, no methods).
  - [R] Write `TestGetDisplayNameFallback` (fails — no `GetDisplayName` field).
  - Extend `DashboardObservationRow`, `DashboardSessionRow`, `DashboardPromptRow` with the new fields (payload field additions, `upsertDashboard*` signature changes, `applyDashboardMutation` updates for all three entities).
  - Extend `CloudStore` with `SystemHealth()` returning `DashboardSystemHealth` (new struct). Implementation: walk `cs.loadDashboardReadModel()` and count. `DBConnected` from `cs.db.Ping()` with a 500ms timeout.
  - Add `internal/cloud/cloudstore/project_controls.go` with the four control methods.
  - Extend `migrate()` with the `cloud_project_controls` statements.
  - Add `GetDisplayName func(r *http.Request) string` to `dashboard.MountConfig`. Add `principal.go` with `Principal` type and `principalFromRequest` method.
  - Extend `DashboardStore` interface with: five `ListXxxPaginated`, three `GetXxxDetail`, `SystemHealth`, `ListProjectSyncControls`, `GetProjectSyncControl`, `SetProjectSyncEnabled`, `IsProjectSyncEnabled`.
  - Tests go GREEN.
  - **Depends on**: Batches 1 + 2 (go.mod, templ generated files so handlers compile).
  - **Blocks**: Batch 4 (new routes call the new store methods).

  **Batch 4 — New routes + HTMX attribute tests + admin + push-path guard (RED first)**
  - [R] Write `TestDashboardHomeHTMXWiring`, `TestBrowserPageHTMXWiring`, `TestProjectsPageHTMXWiring`, `TestAdminPageSurfacePresent`, `TestAdminHealthPageRendersMetrics`, `TestAdminUsersPageRendersContributors`, `TestAdminSyncTogglePosts`, `TestAdminSyncToggleRequiresAdmin`, `TestFullHTMXEndpointSurface`, `TestPrincipalBridgeNoPanicOnEmptyContext` (all fail — routes not registered / handlers don't emit HTMX attrs).
  - [R] Write `TestPushPathPauseEnforcement` + `TestInsecureModeLoginRedirects` in `cloudserver_test.go` (fail — no pause guard, or login redirect not yet verified).
  - Rewrite `handleDashboardHome`, `handleBrowser*`, `handleProjects`, `handleContributors`, `handleAdmin` to use templ components (Layout, DashboardHome, BrowserPage, ProjectsPage, ContributorsPage, AdminPage).
  - Register 11 new routes in `dashboard.Mount`: `/dashboard/projects/list`, `/dashboard/projects/{name}/observations|sessions|prompts`, `/dashboard/admin/users`, `/dashboard/admin/health`, `POST /dashboard/admin/projects/{name}/sync`, `GET /dashboard/admin/projects/{name}/sync/form`, `/dashboard/sessions/{project}/{sessionID}`, `/dashboard/observations/{project}/{sessionID}/{chunkID}`, `/dashboard/prompts/{project}/{sessionID}/{chunkID}`.
  - Insert push-path pause check in `cloudserver.go:handlePushChunk`.
  - Wire `GetDisplayName: func(r *http.Request) string { return "OPERATOR" }` in `cloudserver.go:routes()` MountConfig literal.
  - Tests go GREEN.
  - **Depends on**: Batch 3 (new store methods + Principal bridge).
  - **Blocks**: Batch 5 (docs/polish).

  **Batch 5 — Docs + final verify**
  - Update `DOCS.md` with the `make templ` contributor section.
  - Update `README.md` with the one-line pointer.
  - Update `docs/ARCHITECTURE.md` with: a new subsection "Dashboard visual-parity layer" describing the Principal bridge + push-path pause guard + composite URL scheme.
  - Update `docs/AGENT-SETUP.md` with the templ regeneration command.
  - Run `go test -cover ./...` — confirm ≥ 79 % overall.
  - Run `go vet ./...` and `gofmt -l` — must be clean.
  - **Depends on**: Batches 1–4.

- **Rationale**: RED first in every batch enforces strict TDD. Ordering is dictated by compile-time dependencies: Batch 2's templ files import cloudstore types — but only the EXISTING types (sufficient for layout/login — which don't touch row types). Batch 3's extensions to the row types don't break Batch 2 because the templ components were written against the flat row API (they ignore new fields when not rendered). Batch 4 uses new store methods added in Batch 3 and templ components from Batch 2.

- **Trade-offs**: Splitting "new methods" (Batch 3) from "new routes" (Batch 4) means Batch 3 adds code that is initially unused by production. The coverage ratchet will dip temporarily after Batch 3 and recover after Batch 4. Acceptable — the alternative (merging the two) would produce a single enormous batch.

- **Impact**: Five pull-request-sized batches, each independently reviewable.

## Data Flow & Contracts

Example: `GET /dashboard/browser/observations?project=engram&type=bugfix&limit=20&cursor=40`

```
1. HTTP request enters s.mux (CloudServer routes)
     │
2. dashboard.Mount has registered this path under h.requireSession(h.handleBrowserObservations)
     │
3. h.requireSession calls h.cfg.RequireSession(r)
     │  → s.authorizeDashboardRequest(r)
     │      → reads engram_dashboard_token cookie
     │      → s.dashboardBearerToken(cookie.Value)
     │          → codec.ParseDashboardSession(token) → bearerToken
     │      → auth.Service.Authorize(bearerToken) → nil | err
     │  On err: redirect to /dashboard/login (via h.requireSession wrapper)
     │
4. h.handleBrowserObservations(w, r):
     │  project := store.NormalizeProject(r.URL.Query().Get("project"))
     │  obsType := strings.TrimSpace(r.URL.Query().Get("type"))
     │  query   := strings.TrimSpace(r.URL.Query().Get("q"))
     │  limit   := parseInt(r.URL.Query().Get("limit"), 50)
     │  cursor  := parseInt(r.URL.Query().Get("cursor"), 0)
     │
5.   principal := h.principalFromRequest(r)
     │   → Principal{displayName:"OPERATOR", isAdmin: false}
     │
6.   rows, total, err := h.cfg.Store.ListRecentObservationsPaginated(project, query, obsType, limit, cursor)
     │   → cloudstore.loadDashboardReadModel() (cached)
     │   → scoped by dashboardAllowedScopes
     │   → filterObservations(project, query) → []DashboardObservationRow (sorted DESC CreatedAt)
     │   → apply obsType filter (in-memory)
     │   → slice [cursor:cursor+limit]
     │   → return (page, total, nil)
     │
7.   component := ObservationsPartial(rows, paginationFromCursor(cursor, limit, total))
     │   → templ component renders <table class="data-table">... with hx-get pagination links
     │
8.   if isHTMXRequest(r):
     │      renderHTML(w, component)   // fragment only
     │   else:
     │      page := Layout("Browser", principal.DisplayName(), "browser", principal.IsAdmin(), component)
     │      renderHTML(w, page)        // full shell
     │
9. HTTP response: 200 text/html; charset=utf-8
```

**Contracts:**
- `Principal.DisplayName()` — always non-empty (falls back to "OPERATOR").
- `ListRecentObservationsPaginated(project, query, obsType, limit, cursor)` — returns `(page []Row, total int, err error)`. Sort is stable. Empty page is NOT an error (empty slice).
- Templ component `ObservationsPartial(rows, pg)` — renders either the table + pagination bar, or an `EmptyState` component. Never panics on empty input.

## File Changes

### NEW
- `internal/cloud/dashboard/principal.go` — `Principal` type + `principalFromRequest` method.
- `internal/cloud/dashboard/templ_policy_test.go` — asset floor + generated-files checked-in tests.
- `internal/cloud/cloudstore/project_controls.go` — `ProjectSyncControl` + 4 methods.
- `internal/cloud/cloudstore/project_controls_test.go` — persistence round-trip test.
- `internal/cloud/dashboard/components_templ.go` — generated (committed).
- `internal/cloud/dashboard/layout_templ.go` — generated (committed).
- `internal/cloud/dashboard/login_templ.go` — generated (committed).
- `tools/tools.go` — build-tag `tools` file for templ binary retention.
- `Makefile` target `templ`.

### MODIFIED
- `internal/cloud/cloudstore/cloudstore.go` — `migrate()` appends two statements for `cloud_project_controls`.
- `internal/cloud/cloudstore/dashboard_queries.go` — add fields to flat rows; add 5 `ListXxxPaginated`, 3 `GetXxxDetail`, `SystemHealth` methods.
- `internal/cloud/cloudstore/dashboard_queries_test.go` — new row-field + pagination + system-health tests.
- `internal/cloud/dashboard/dashboard.go` — add `MountConfig.GetDisplayName` field; rewrite handlers to use templ; register 11 new routes; add handler methods.
- `internal/cloud/dashboard/dashboard_test.go` — new structural / HTMX wiring / copy parity / admin / principal tests.
- `internal/cloud/dashboard/helpers.go` — replace with legacy port; adapt types and imports.
- `internal/cloud/cloudserver/cloudserver.go` — wire `GetDisplayName` in MountConfig literal; insert push-path pause guard in `handlePushChunk`.
- `internal/cloud/cloudserver/cloudserver_test.go` — add `TestPushPathPauseEnforcement`, `TestInsecureModeLoginRedirects`.
- `go.mod` — add `github.com/a-h/templ v0.3.1001` and `golang.org/x/net` direct deps.
- `go.sum` — regenerated by `go mod tidy`.
- `DOCS.md` — add templ regeneration subsection.
- `README.md` — add one-line pointer.
- `docs/ARCHITECTURE.md` — add dashboard visual-parity subsection.
- `docs/AGENT-SETUP.md` — add templ contributor command.

### COPIED_VERBATIM (byte-for-byte from `/Users/alanbuscaglia/work/engram-cloud/internal/cloud/dashboard/`)
- `internal/cloud/dashboard/static/htmx.min.js` — 50 917 B (legacy) replaces 42 B stub.
- `internal/cloud/dashboard/static/pico.min.css` — 71 072 B replaces 46 B stub.
- `internal/cloud/dashboard/static/styles.css` — 23 185 B replaces 2 284 B thin stub.

### COPIED_WITH_ADAPTATION (from legacy, import path + types adjusted)
- `internal/cloud/dashboard/components.templ` — 1134 L; all `cloudstore.CloudObservation|CloudSession|CloudPrompt|ProjectStat|ContributorStat|SystemHealthInfo|CloudUser` substituted with `DashboardXxxRow` / `DashboardSystemHealth` / `DashboardContributorRow` / raw strings; import path swapped; fields not available on flat rows replaced with blank or adjacent fields documented per-component.
- `internal/cloud/dashboard/layout.templ` — 52 L; `username` parameter unchanged (receives `Principal.DisplayName()` from caller).
- `internal/cloud/dashboard/login.templ` — 63 L; add `next string` second parameter and `<input type="hidden" name="next" value="{ next }">` inside the form.

## Testing Strategy

| Spec REQ | Test file | Test name | Layer |
|---|---|---|---|
| REQ-100 | `internal/cloud/dashboard/templ_policy_test.go` | `TestStaticAssetByteFloors` | embed fs |
| REQ-101 | `internal/cloud/dashboard/templ_policy_test.go` | `TestTemplGeneratedFilesAreCheckedIn` | disk/package |
| REQ-102 | `internal/cloud/cloudstore/dashboard_queries_test.go` | `TestDashboardRowDetailFields` | cloudstore (postgres via testcontainers or sqlmock per existing convention) |
| REQ-103 | `internal/cloud/dashboard/dashboard_test.go` | `TestGetDisplayNameFallback` | handler |
| REQ-104 | `internal/cloud/cloudstore/project_controls_test.go` | `TestProjectSyncControlPersists` | cloudstore |
| REQ-105 | `internal/cloud/cloudstore/dashboard_queries_test.go` | `TestCloudstoreSystemHealthAggregates` | cloudstore |
| REQ-106 | `internal/cloud/dashboard/dashboard_test.go` | `TestFullHTMXEndpointSurface` | route-level httptest |
| REQ-107 | `internal/cloud/dashboard/dashboard_test.go` | `TestDashboardLayoutHTMLStructure` + `TestStatusRibbonAndFooterPresent` + `TestNavTabsRenderedCorrectly` | html tokenizer |
| REQ-108 | `internal/cloud/dashboard/dashboard_test.go` | `TestDashboardHomeHTMXWiring` + `TestBrowserPageHTMXWiring` + `TestProjectsPageHTMXWiring` | substring |
| REQ-109 | `internal/cloud/cloudserver/cloudserver_test.go` | `TestPushPathPauseEnforcement` | route-level httptest |
| REQ-110 | `internal/cloud/cloudserver/cloudserver_test.go` | `TestInsecureModeLoginRedirects` | route-level httptest |
| REQ-111 | `internal/cloud/dashboard/dashboard_test.go` | `TestCopyParityStrings` | substring |
| REQ-112 | `internal/cloud/dashboard/dashboard_test.go` | `TestAdminSyncTogglePosts` + `TestAdminSyncToggleRequiresAdmin` | route-level |
| REQ-113 | `internal/cloud/dashboard/dashboard_test.go` | `TestPrincipalBridgeNoPanicOnEmptyContext` | handler |

**RED seam** per batch (the concrete failing assertion at batch start):
- Batch 1 RED: `fs.Stat(StaticFS, "static/htmx.min.js").Size() >= 40_000` — fails (42 B stub).
- Batch 2 RED: `strings.Contains(body, "ENGRAM CLOUD / SHARED MEMORY INDEX / LIVE SYNC READY")` — fails (footer not rendered).
- Batch 3 RED: `len(row.Content) > 0` after seeded chunk — fails (field doesn't exist on row).
- Batch 4 RED: `strings.Contains(body, "hx-get=\"/dashboard/projects/list\"")` — fails (route not registered / handler not emitting).
- Batch 5: no new RED — this batch is docs + polish.

## Rollout Plan

```
Batch 1 (templ bootstrap + assets) ──► Batch 2 (templ sources + generated) ──┐
                                                                              ├──► Batch 4 (routes + guard) ──► Batch 5 (docs + verify)
                                      Batch 3 (rows + store + principal) ────┘
```

- **Batch 1** blocks Batches 2, 3, 4, 5 (go.mod, assets).
- **Batch 2** blocks Batch 4 (handler templ calls).
- **Batch 3** blocks Batch 4 (handler store calls).
- **Batch 2** and **Batch 3** can be implemented in parallel if reviewer bandwidth allows — their file sets do not overlap. Default: serialize for simpler review.
- **Batch 4** blocks Batch 5 (verify needs full feature).

Strict TDD discipline: every batch opens a new RED test file / new RED test function BEFORE any production code is written. A batch is never considered started until at least one failing test exists on the branch.

## Open Questions

None — all ten spec-phase open questions are resolved in the Architecture Decisions section above.

## References

- Proposal: `openspec/changes/cloud-dashboard-visual-parity/proposal.md`
- Exploration: `openspec/changes/cloud-dashboard-visual-parity/exploration.md`
- Spec: `openspec/changes/cloud-dashboard-visual-parity/specs/cloud-dashboard/spec.md`
- Engram topics:
  - `sdd/cloud-dashboard-visual-parity/proposal`
  - `sdd/cloud-dashboard-visual-parity/spec`
  - `sdd/cloud-dashboard-visual-parity/copy-strategy`
  - `sdd/cloud-dashboard-visual-parity/design-decisions` (written by this phase)
- Legacy reference:
  - `/Users/alanbuscaglia/work/engram-cloud/internal/cloud/dashboard/dashboard.go`
  - `/Users/alanbuscaglia/work/engram-cloud/internal/cloud/dashboard/helpers.go`
  - `/Users/alanbuscaglia/work/engram-cloud/internal/cloud/dashboard/middleware.go`
  - `/Users/alanbuscaglia/work/engram-cloud/internal/cloud/cloudstore/project_controls.go`
  - `/Users/alanbuscaglia/work/engram-cloud/internal/cloud/cloudstore/schema.go`
- Current integrated reference:
  - `/Users/alanbuscaglia/work/engram/internal/cloud/dashboard/dashboard.go`
  - `/Users/alanbuscaglia/work/engram/internal/cloud/cloudserver/cloudserver.go`
  - `/Users/alanbuscaglia/work/engram/internal/cloud/cloudstore/dashboard_queries.go`
  - `/Users/alanbuscaglia/work/engram/internal/cloud/cloudstore/cloudstore.go` (`migrate()` at line 427)
