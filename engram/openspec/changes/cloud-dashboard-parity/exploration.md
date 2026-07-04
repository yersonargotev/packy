## Exploration: cloud-dashboard-parity

### Current State
- Integrated repo (`engram`) ships a **status-only** cloud dashboard:
  - Single `dashboard.HandlerWithStatus(...)` HTML page with reason parity text.
  - Token paste login at `/dashboard/login` handled in `cloudserver` (not in dashboard package).
  - No dashboard static route mount; current dashboard assets are placeholders.
  - No browser/projects/contributors/admin UI surface.
- Original repo (`engram-cloud`) ships a **full server-rendered + htmx-enhanced dashboard**:
  - Route tree across health, login/logout, change-password, dashboard stats/activity, browser, projects, contributors, admin.
  - Rich templ component system + helpers + embedded real static assets.
  - Cookie auth middleware based on JWT claims (`userID`, `username`, `email`).
  - Heavy reliance on cloudstore read models/pagination/search/admin methods.

### Affected Areas
- `internal/cloud/dashboard/*` (engram + engram-cloud) — core parity surface and route/component behavior.
- `internal/cloud/cloudserver/cloudserver.go` — route mounting strategy and dashboard auth boundary.
- `internal/cloud/auth/auth.go` — auth contract mismatch (bearer-token validation vs credential/JWT user claims).
- `internal/cloud/cloudstore/cloudstore.go` (+ additional files) — missing read models required by old dashboard handlers.
- `cmd/engram/cloud.go`, `internal/cloud/config.go` — runtime wiring for admin config and dashboard dependencies.
- `go.mod` — missing `github.com/a-h/templ` runtime dependency in integrated repo.

### File-by-File Parity Gap (exact)

#### Dashboard package files

| File (source: engram-cloud) | Integrated state (engram) | Parity gap | Portability |
|---|---|---|---|
| `internal/cloud/dashboard/dashboard.go` | Exists but reduced to ~status page only | Missing ~30 routes + all rich handlers | **Adapt** (not direct) |
| `internal/cloud/dashboard/dashboard_test.go` | Exists but only 3 status tests | Missing broad route/component/auth regression suite | **Adapt/expand** |
| `internal/cloud/dashboard/components.templ` | Exists but minimal (2 tiny components) | Missing all feature UI components/partials | **Direct copy possible**, then adapt imports/contracts |
| `internal/cloud/dashboard/layout.templ` | Exists but minimal shell | Missing full nav/header/layout/static includes | **Direct copy possible**, minor adapt |
| `internal/cloud/dashboard/login.templ` | Exists but minimal title only | Missing full login UX and error rendering | **Direct copy possible**, minor adapt |
| `internal/cloud/dashboard/helpers.go` | **Missing** | Pagination, formatting, structured content rendering absent | **Direct copy possible** |
| `internal/cloud/dashboard/middleware.go` | **Missing** | Cookie-auth context extraction absent | **Adapt required** (current auth model differs) |
| `internal/cloud/dashboard/config.go` | **Missing** | `DashboardConfig{AdminEmail}` absent in package | **Direct copy possible** |
| `internal/cloud/dashboard/embed.go` | **Missing** | Embedded static FS mount support absent | **Direct copy possible** |
| `internal/cloud/dashboard/components_templ.go` | **Missing** | Generated templ runtime file absent | **Port strategy decision needed** |
| `internal/cloud/dashboard/layout_templ.go` | **Missing** | Generated templ runtime file absent | **Port strategy decision needed** |
| `internal/cloud/dashboard/login_templ.go` | **Missing** | Generated templ runtime file absent | **Port strategy decision needed** |
| `internal/cloud/dashboard/static/styles.css` | Exists but 1-line placeholder | Missing full visual language | **Direct copy** |
| `internal/cloud/dashboard/static/pico.min.css` | Exists but placeholder | Missing real Pico asset | **Direct copy** |
| `internal/cloud/dashboard/static/htmx.min.js` | Exists but placeholder | Missing real htmx runtime | **Direct copy** |

#### Non-dashboard integration files (required for parity)

| Integrated file | Gap vs old dashboard expectations | Required action |
|---|---|---|
| `internal/cloud/cloudserver/cloudserver.go` | Currently mounts only `/dashboard` status handler + token login/logout; no `dashboard.Mount(...)`; no `/dashboard/static/*` route | Introduce full mount path and split responsibilities cleanly |
| `internal/cloud/auth/auth.go` | No `Login`, `ValidateAccessToken` claims, `ChangePassword`; only fixed bearer token auth + signed dashboard session wrapper | Decide auth bridge: keep token-mode and adapt UI, or restore user auth surface |
| `internal/cloud/cloudstore/cloudstore.go` | Lacks virtually all methods used by old handlers (`ProjectStats*`, `Search*`, `Recent*`, admin/system health/project controls, etc.) | Add read-model API layer or port broader cloudstore/query files |
| `cmd/engram/cloud.go` | Uses `cloudserver.New(...)` with status provider; no `WithDashboard(cfg)` option | Extend runtime wiring for dashboard config and dependencies |
| `go.mod` | No `github.com/a-h/templ` dependency | Add templ runtime dependency if using generated templ output |

### Required Integration Points (cloudserver/auth/cloudstore/runtime)

1. **cloudserver ↔ dashboard route ownership**
   - Move from status-handler mount to full `dashboard.Mount(...)` equivalent.
   - Preserve existing `/sync/*` contracts and cloud auth behavior.
   - Add static mount `/dashboard/static/*` via embedded FS.

2. **dashboard auth boundary**
   - Old dashboard assumes identity-bearing JWT cookie (`engram_session`) from username/email/password login.
   - Integrated runtime uses bearer token (`ENGRAM_CLOUD_TOKEN`) + signed opaque dashboard cookie (`engram_dashboard_token`) that maps to one configured token.
   - Must adapt middleware/context helpers and admin checks without breaking current token-based security model.

3. **dashboard data access ↔ cloudstore API**
   - Old handlers call >25 cloudstore query methods not present in integrated store.
   - Need either:
     - a compatibility read-model layer in `cloudstore`, or
     - full/partial port of old cloudstore modules (`search.go`, `project_controls.go`, extra methods/schema).

4. **runtime/config wiring**
   - Ensure `ENGRAM_CLOUD_ADMIN` remains effective for admin gating in restored UI.
   - Keep local-first semantics explicit: dashboard is read/admin surface over replicated cloud data, not source of truth.

5. **templ runtime strategy**
   - Integrated repo currently has `.templ` files but no generated `_templ.go` nor templ dependency.
   - Choose deterministic build path: commit generated files + templ runtime dependency, or introduce generation step and CI enforcement.

### Risks of Restoring Old Dashboard Verbatim
- **Auth model breakage (High):** old login/change-password flow cannot run on current auth service without reintroducing user/password/JWT identity model.
- **Compile/runtime breakage (High):** direct copy references missing cloudstore methods and missing templ runtime.
- **Boundary erosion (Medium-High):** blindly porting old cloudstore internals may conflict with integrated local-first chunk-centric model.
- **Security mismatch (Medium):** old cookie flow sets `Secure: true` unconditionally in some paths; integrated code currently handles forwarded proto more defensively.
- **Operational drift (Medium):** restoring contributor/admin pages without aligned data contracts may expose empty or misleading controls.

### Approaches
1. **Verbatim Port (big-bang)**
   - Pros: fastest perceived parity on paper.
   - Cons: highest breakage risk (auth/store/build mismatches), difficult rollback.
   - Effort: **High**.

2. **Compatibility-first Port (recommended)**
   - Pros: keeps current security/runtime intact; phases reduce blast radius; easier regression testing.
   - Cons: slightly slower than brute-force copy.
   - Effort: **Medium-High**.

### Recommendation
Use **Compatibility-first Port**. Reuse old dashboard assets/templates/components aggressively, but adapt auth/store integration seams explicitly. This satisfies the user's “reuse, don’t redesign” intent while preserving integrated repo invariants.

### Best Implementation Sequence (minimize breakage)
1. **Foundation parity (low-risk copy):** copy static assets, `embed.go`, `config.go`, helper utilities, and templ sources; add templ runtime dependency strategy.
2. **Mount parity shell:** introduce `dashboard.Mount(...)` and route tree behind existing cloudserver auth boundary; keep data handlers initially stubbed to safe empty states.
3. **Auth bridge:** adapt dashboard middleware/login/logout/change-password paths to current token/session model (or explicitly gate unsupported flows) before enabling advanced pages.
4. **Data parity incrementally:** implement cloudstore read-model methods in vertical slices:
   - Slice A: Dashboard home/stats/activity
   - Slice B: Browser + detail pages
   - Slice C: Projects + controls
   - Slice D: Contributors + admin
5. **Regression lock-in:** port/adjust old dashboard tests in the same slices; add boundary tests for sync push/pull unaffectedness and auth redirects.
6. **Docs parity:** update docs/config instructions once behavior is live.

### Ready for Proposal
**Yes.** Exploration confirms parity is feasible, but only with adaptation at auth/cloudstore/cloudserver seams; verbatim restore is unsafe.
