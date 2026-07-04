# Apply Progress — cloud-dashboard-parity

## Mode

Strict TDD

## Completed Tasks

- [x] 1.1 dashboard route matrix + HTTP fallback RED/green cycle
- [x] 1.2 dashboard scaffolding (`config.go`, `embed.go`, `helpers.go`)
- [x] 1.3 styled templates + restored static stylesheet structure
- [x] 1.4 templ runtime policy locked to deterministic checked-in generation (`templ_policy.go` + `go:generate` contract)
- [x] 2.1 cloudserver RED tests for static/shareable route ownership and sync boundary safety
- [x] 2.2 cloudserver mounts full dashboard route tree + static assets
- [x] 2.3 login/logout remain HTTP form-first, htmx optional
- [x] 2.4 dashboard wiring isolated behind `dashboard.MountConfig`
- [x] 3.1 explicit auth-flow tests for redirects/session states in cloudserver + dashboard packages
- [x] 3.2 dashboard middleware bridge with signed `engram_dashboard_token`
- [x] 3.3 admin gating bridged with `ENGRAM_CLOUD_ADMIN`
- [x] 3.4 auth seams consolidated in cloudserver callback wiring
- [x] 4.1 cloudstore RED tests for read-model parity slices (projects/detail/search/contributors/admin)
- [x] 4.2 additive read-model query APIs implemented (`ProjectDetail`, query-aware browser/project/contributor methods)
- [x] 4.3 existing chunk-history queryability validated through read-model materialization from chunk rows
- [x] 4.4 cloudstore filtering/aggregation refactored into internal read-model helpers; dashboard handlers keep presentation concerns
- [x] 5.1 docs alignment updates (README, DOCS, architecture)
- [x] 5.2 sync push/pull boundary regression assertions added alongside route ownership parity checks
- [x] 5.3 full verification runs (`go test ./...`, `go test -cover ./...`)
- [x] verify-gap A: contributor success-path test proves `/dashboard/contributors` renders cloudstore-backed rows
- [x] verify-gap B: docs-backed operator enablement flow is executable (`cloud config` -> `status` -> `enroll` -> authenticated `cloud serve`)
- [x] verify-gap C (recommended): strengthened route-ownership parity test with project-detail content assertion

## TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|---|---|---|
| 1.1 | `internal/cloud/dashboard/dashboard_test.go` | Unit/HTTP | ✅ `go test ./internal/cloud/{dashboard,cloudserver,cloudstore}` | ✅ compile fail (`undefined: Mount`) | ✅ pass (`go test ./internal/cloud/dashboard -run TestMountRouteParityAndHTTPFallbacks`) | ✅ route matrix + static + login valid/invalid + shareable URL | ✅ extracted `MountConfig`, handler split, helper funcs |
| 1.4 | `internal/cloud/dashboard/templ_policy_test.go` | Unit | ✅ `go test ./internal/cloud/dashboard` | ✅ compile fail (`undefined: templRuntimePolicy`) | ✅ pass (`go test ./internal/cloud/dashboard -run TestTemplRuntimePolicyIsDeterministic`) | ✅ validated mode + runtime-generation disable + explicit command | ✅ policy isolated in `templ_policy.go` |
| 2.1 / 5.2 | `internal/cloud/cloudserver/cloudserver_test.go` | Integration | ✅ `go test ./internal/cloud/cloudserver` | ✅ new parity assertions initially failed (shareable project detail route + sync ownership checks) | ✅ pass (`go test ./internal/cloud/cloudserver -run TestHandlerDashboardRouteOwnershipParity`) | ✅ static + shareable route + sync boundary assertions in same flow | ✅ kept cloudserver thin; assertions remain at handler boundary |
| 2.2/2.3/2.4 | `internal/cloud/cloudserver/cloudserver_test.go` (existing parity/auth suite) | Integration | ✅ same baseline | ✅ interim failures after mount migration (301 + login content mismatch) | ✅ pass (`go test ./internal/cloud/dashboard ./internal/cloud/cloudserver`) | ✅ covered unauth redirect/login flow/signed cookie/static serving through route matrix + existing cloudserver tests | ✅ removed duplicated cloudserver dashboard handlers; mount callbacks now explicit |
| 3.1 | `internal/cloud/cloudserver/cloudserver_test.go`, `internal/cloud/dashboard/dashboard_test.go` | Integration + Unit/HTTP | ✅ package baselines above | ✅ added explicit auth/session redirect cases before parity handler updates | ✅ pass (`go test ./internal/cloud/dashboard -run TestMountHTMXAndProjectDetailParity` and cloudserver auth tests) | ✅ invalid token + already-authenticated login + non-admin deny + cookie/session redirect matrix | ✅ no duplicated auth logic introduced |
| 4.1 / 4.2 / 4.3 / 4.4 | `internal/cloud/cloudstore/cloudstore_test.go` | Unit | ✅ `go test ./internal/cloud/cloudstore` | ✅ compile fail (`undefined: buildDashboardReadModel`) | ✅ pass (`go test ./internal/cloud/cloudstore -run TestBuildDashboardReadModelSupportsParityQueries`) | ✅ multi-project fixtures validating detail, search, contributors, admin rollups | ✅ read-model builder and filters extracted to keep adapters thin |
| 5.3 | n/a | Verification | n/a | n/a | ✅ `go test ./...` and `go test -cover ./...` | ➖ | ➖ |
| verify-gap A | `internal/cloud/dashboard/dashboard_test.go` | Integration (HTTP/handler) | ✅ `go test ./internal/cloud/dashboard -run 'TestMountHTMXAndProjectDetailParity|TestMountRouteParityAndHTTPFallbacks'` | ✅ added contributor success-path assertions for `/dashboard/contributors` | ✅ `go test ./internal/cloud/dashboard -run TestMountContributorsSurfaceRendersCloudstoreBackedRows` | ✅ non-empty contributor row + aggregate metrics rendering path | ➖ none needed (test-only) |
| verify-gap B | `cmd/engram/main_extra_test.go` | Integration (CLI/runtime operator flow) | ✅ `go test ./cmd/engram -run 'TestCloudUpgradeDocsMatchHelpAndLocalFirstSemantics|TestCmdCloudStatusHonorsEnvServerOverride|TestCmdCloudServeStartsCloudRuntime'` | ✅ added docs-backed runtime flow test spanning `config/status/enroll/serve` sequence | ✅ `go test ./cmd/engram -run TestCloudDashboardDocsEnablementFlowIsExecutable` | ✅ docs token verification + runtime env/auth/admin wiring assertions | ➖ none needed (test-only) |
| verify-gap C | `internal/cloud/cloudserver/cloudserver_test.go` | Integration (HTTP/handler) | ✅ `go test ./internal/cloud/cloudserver -run TestHandlerDashboardRouteOwnershipParity` | ✅ strengthened existing shareable-route parity test to assert project detail content | ✅ `go test ./internal/cloud/cloudserver -run TestHandlerDashboardRouteOwnershipParity` | ✅ route ownership + project detail page body proof in same request path | ➖ none needed (test-only) |

## Verification Logs

- `go test ./internal/cloud/dashboard ./internal/cloud/cloudserver ./internal/cloud/cloudstore` ✅
- `go test ./internal/cloud/...` ✅
- `go test ./...` ✅
- `go test -cover ./...` ✅
- `go test ./internal/cloud/dashboard -run TestMountContributorsSurfaceRendersCloudstoreBackedRows` ✅
- `go test ./cmd/engram -run TestCloudDashboardDocsEnablementFlowIsExecutable` ✅
- `go test ./internal/cloud/cloudserver -run TestHandlerDashboardRouteOwnershipParity` ✅
- `go test ./...` (post-gap update) ✅
- `go test -cover ./...` (post-gap update) ✅

## Remaining Work

- None in current apply scope. Requested verify gaps were covered with executable tests and full-suite reruns.

## Deviations

- Read-model implementation remains additive and local-first safe, but materialization is computed from persisted chunk history at query time (`buildDashboardReadModel`) instead of separate persisted SQL dashboard tables. This is functionally parity-complete for current requirements while keeping sync write contracts unchanged.
