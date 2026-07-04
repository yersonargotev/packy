# Tasks: Restore cloud dashboard parity

## Phase 1: Foundation restore (assets, templates, mount seams)

- [x] 1.1 RED: Expand `internal/cloud/dashboard/dashboard_test.go` with parity route matrix (health/login/logout/dashboard/browser/projects/contributors/admin) and non-htmx POST fallback assertions.
- [x] 1.2 GREEN: Restore dashboard package scaffolding in `internal/cloud/dashboard/{config.go,embed.go,helpers.go}` and wire static FS contract used by handlers.
- [x] 1.3 GREEN: Restore UI sources from `engram-cloud` into `internal/cloud/dashboard/{layout.templ,components.templ,login.templ}` plus real `static/{styles.css,pico.min.css,htmx.min.js}`.
- [x] 1.4 REFACTOR: Decide templ runtime path in `go.mod` + dashboard package (commit generated `_templ.go` or enforce generation), keeping builds deterministic and additive.

## Phase 2: Route/static/template restoration in cloudserver

- [x] 2.1 RED: Add `internal/cloud/cloudserver/cloudserver_test.go` cases for `/dashboard/static/*` serving, shareable URL handling, and dashboard route ownership without breaking `/sync/*`.
- [x] 2.2 GREEN: Replace status-only mount in `internal/cloud/cloudserver/cloudserver.go` with full dashboard mount entrypoint and static asset routing.
- [x] 2.3 GREEN: Keep existing login/logout endpoints as HTTP-first forms; ensure htmx is enhancement only and redirects remain meaningful for full-page clients.
- [x] 2.4 REFACTOR: Isolate dashboard wiring via option/config helpers so cloudserver stays thin and boundary-safe.

## Phase 3: Auth/session adaptation (integrated token model)

- [x] 3.1 RED: Add auth-flow tests in `internal/cloud/cloudserver/cloudserver_test.go` + `internal/cloud/dashboard/dashboard_test.go` for cookie/session redirects, invalid token errors, and already-authenticated login redirect.
- [x] 3.2 GREEN: Add `internal/cloud/dashboard/middleware.go` adapting dashboard context/middleware to `engram_dashboard_token` + bearer-token validation (no legacy username/password reintroduction).
- [x] 3.3 GREEN: Reconcile admin gating with `ENGRAM_CLOUD_ADMIN` across `internal/cloud/dashboard` and `internal/cloud/config.go` expectations.
- [x] 3.4 REFACTOR: Keep auth seams explicit between `internal/cloud/auth/auth.go`, dashboard middleware, and server handlers; remove duplicated auth checks.

## Phase 4: Cloudstore read-model backfill by vertical slices

- [x] 4.1 RED: Add `internal/cloud/cloudstore/cloudstore_test.go` failing tests for dashboard read models (overview stats/activity, browser search/detail, projects, contributors, admin/system health).
- [x] 4.2 GREEN: Implement additive query/read APIs in `internal/cloud/cloudstore/cloudstore.go` (or split files) to satisfy restored handlers without changing sync write contracts.
- [x] 4.3 GREEN: Add/verify backfill paths for dashboard indexes/read models so existing chunk data remains queryable after deploy.
- [x] 4.4 REFACTOR: Keep cloudstore adapters thin; move formatting/filtering to dashboard helpers, preserving local-first semantics.

## Phase 5: Docs alignment and parity verification

- [x] 5.1 Update `README.md`, `DOCS.md`, and `docs/ARCHITECTURE.md` with restored dashboard routes, env vars (`ENGRAM_CLOUD_TOKEN`, `ENGRAM_JWT_SECRET`, `ENGRAM_CLOUD_ADMIN`), and fallback behavior.
- [x] 5.2 Add regression tests in `internal/cloud/cloudserver/cloudserver_test.go` proving dashboard parity changes do not regress sync push/pull boundaries.
- [x] 5.3 Verify full suite with `go test ./...` and `go test -cover ./...`; fix flakes/non-determinism before marking parity complete.
