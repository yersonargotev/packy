# Design: Restore cloud dashboard parity

## Technical Approach

Port the `engram-cloud` dashboard surface into `internal/cloud/dashboard`, but adapt the seams instead of copying the old cloud product model verbatim. The integrated repo keeps bearer-token auth, signed dashboard sessions, and chunk-based cloud storage; parity comes from restoring routes, templ/static assets, and a new additive dashboard read model derived from replicated chunks.

## Architecture Decisions

| Decision | Options | Choice | Rationale |
|---|---|---|---|
| Route ownership | Keep status-only mount; copy old handlers into `cloudserver`; restore `dashboard.Mount(...)` | Restore `dashboard.Mount(...)`, with `cloudserver` composing auth/session callbacks | Preserves richer route tree without bloating `cloudserver`; keeps HTTP composition thin and dashboard rendering local to the dashboard package. |
| Auth/session model | Reintroduce username/password JWT flow; keep current bearer-token session bridge | Keep current `ENGRAM_CLOUD_TOKEN` + signed `engram_dashboard_token`; adapt dashboard middleware to inject a synthetic principal | Proposal explicitly rejects a new identity model. This preserves current security/runtime rules and keeps `/dashboard/login` as an HTTP form fallback. |
| Cloud data access | Port old user-scoped cloudstore schema wholesale; query chunk JSON ad hoc; add read-model tables | Add additive dashboard read-model tables/indexes materialized from chunk payloads | Current cloudstore is chunk-centric. Materialized read models satisfy browser/admin queries without changing sync write contracts or local-first ownership. |
| templ/assets strategy | Runtime generation only; inline HTML; checked-in generated files | Copy `.templ` + static assets, add `embed.go`, add `github.com/a-h/templ`, commit generated `_templ.go` files | Matches repo convention to check generated templ output in, keeps builds deterministic, and avoids CI/runtime generation drift. |

## Data Flow

```text
POST /sync/push
  -> cloudserver auth/project checks
  -> cloudstore.WriteChunk(...)
  -> chunk persisted
  -> dashboard index upsert/backfill tables

GET /dashboard/browser?... 
  -> dashboard middleware validates signed session cookie
  -> synthetic principal + admin flag in request context
  -> dashboard handler calls cloudstore read-model queries
  -> templ renders full page or standalone partial HTML
```

## File Changes

| File | Action | Description |
|---|---|---|
| `internal/cloud/dashboard/dashboard.go` | Modify | Replace status-only handler with restored mount + route/handler set. |
| `internal/cloud/dashboard/{config.go,embed.go,helpers.go,middleware.go}` | Create | Dashboard config, static embedding, pagination helpers, session-context bridge. |
| `internal/cloud/dashboard/{components.templ,layout.templ,login.templ}` | Modify | Port richer UI, but keep token-based login form and HTTP-first fallbacks. |
| `internal/cloud/dashboard/{components_templ.go,layout_templ.go,login_templ.go}` | Create | Checked-in templ generated output. |
| `internal/cloud/dashboard/static/{styles.css,pico.min.css,htmx.min.js}` | Modify | Restore real assets. |
| `internal/cloud/cloudserver/cloudserver.go` | Modify | Mount restored dashboard/static routes, keep login/logout/session codec boundary, preserve `/sync/*`. |
| `internal/cloud/cloudstore/cloudstore.go` | Modify | Call dashboard index backfill/materialization from writes and migrations. |
| `internal/cloud/cloudstore/{dashboard_index.go,dashboard_queries.go,project_controls.go,search.go}` | Create | Add additive read-model schema/query surfaces for overview, browser, projects, contributors, and admin. |
| `cmd/engram/cloud.go`, `internal/cloud/config.go`, `go.mod` | Modify | Wire `DashboardConfig`, admin env, and templ runtime dependency. |
| `internal/cloud/{dashboard,cloudserver,cloudstore}/*_test.go` | Modify | Add parity, auth, and boundary regression coverage. |

## Interfaces / Contracts

```go
type DashboardReader interface {
    Overview(project string) (OverviewStats, error)
    ListProjects(filter string, page, pageSize int) ([]ProjectRow, int, error)
    ListObservations(project, query, obsType string, page, pageSize int) ([]ObservationRow, int, error)
    ListSessions(project string, page, pageSize int) ([]SessionRow, int, error)
    ListPrompts(project, query string, page, pageSize int) ([]PromptRow, int, error)
    ListContributors(page, pageSize int) ([]ContributorRow, int, error)
    AdminOverview() (AdminOverview, error)
}
```

The dashboard package depends on read-model contracts; `cloudstore.CloudStore` remains the concrete implementation.

## Testing Strategy

| Layer | What to Test | Approach |
|---|---|---|
| Unit | Middleware/session parsing, pagination helpers, admin gating | `dashboard_test.go` with synthetic requests/cookies. |
| Integration | Static serving, route ownership, login/logout redirects, shareable URLs, partial standalone HTML | `cloudserver_test.go` + expanded dashboard route matrix. |
| Integration | Read-model materialization/backfill from existing chunks; project controls/search queries | `cloudstore_test.go` using fixture chunks and migration/backfill paths. |
| Boundary | Sync push/pull unaffected by dashboard parity | Extend `cloudserver_test.go` around `/sync/push` and `/sync/pull`. |

## Migration / Rollout

No sync-contract migration. Additive Postgres read-model tables are created in `cloudstore.migrate`, then backfilled from existing `cloud_chunks` before handlers rely on them. Roll out in slices: (1) assets/mount, (2) auth bridge, (3) overview/browser read models, (4) projects/contributors/admin, (5) docs/tests.

## Open Questions

- [ ] Whether unsupported account routes from `engram-cloud` (`change-password`) should be hidden entirely or left as explicit “not available in token mode” screens.
