# Apply Progress — integrate-engram-cloud

## Mode
Strict TDD

## Completed Tasks (cumulative)
- [x] 1.1 non-regression tests for unconfigured cloud preserving local behavior
- [x] 1.2 transport contract tests for remote parity with sync.Transport
- [x] 1.3 additive `internal/cloud/*` package + dashboard templ/static assets import
- [x] 1.4 conflict reconciliation notes + deferred script touch points documented as non-goals
- [x] 1.5 minimal cloud constructors/wiring stubs so packages compile without changing local defaults
- [x] 1.6 extracted shared cloud constants/reason-code literals
- [x] 2.1 cloud CLI help/unknown/required-arg tests
- [x] 2.2 cloud CLI isolation test (no local DB/cloud config mutation on status)
- [x] 2.3 implemented `engram cloud ...` command tree with actionable stderr
- [x] 2.4 moved cloud parsing/validation into dedicated `cmd/engram/cloud.go`
- [x] 3.1 store deterministic reason-code tests (`blocked_unenrolled`, `paused`, `auth_required`, `transport_failed`)
- [x] 3.2 sync-state helpers persist reason code/message while preserving lifecycle semantics
- [x] 3.3 sync tests for enrolled push/pull, unenrolled preflight rejection before transport, idempotent pull import
- [x] 3.4 cloud transport path enabled in real sync flows via remote transport + cloud-mode syncer
- [x] 3.5 centralized cloud preflight (auth + enrollment) shared by CLI sync and autosync wiring
- [x] 4.1 startup tests for autosync lifecycle gating (cloud-enabled vs local-only)
- [x] 4.2 cloud autosync lifecycle wiring in `cmd/engram/main.go` behind explicit enablement and enrollment checks
- [x] 4.3 `/sync/status` parity tests for deterministic reason code/message surfaces
- [x] 4.4 `/sync/status` payload extended with cloud reason fields while preserving API ownership
- [x] 4.5 dashboard deterministic reason rendering/parity tests + template integration
- [x] 5.1 docs alignment (`README.md`, `DOCS.md`, cloud docs pages)
- [x] 5.2 deferred plugin/script boundary docs updates (`docs/AGENT-SETUP.md`, `docs/PLUGINS.md`)
- [x] 5.3 focused verification suite executed for cmd/sync/store/server/cloud packages
- [x] 5.4 full regression + coverage gates executed (`go test ./...`, `go test -cover ./...`)

## TDD Cycle Evidence
| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 1.1 | `cmd/engram/main_test.go`, `cmd/engram/main_extra_test.go` | Unit | ✅ `go test ./cmd/engram -run "TestPrintUsage|TestMainUnknownCommand|TestMainUsageAndHelp|TestMainSyncSubcommands"` | ✅ Added failing expectations for `cloud` usage + cloud-related exit cases + unconfigured local-path regression | ✅ `go test ./cmd/engram -run "TestPrintUsage|TestMainExitPaths|TestCloudCommandIsolationDoesNotMutateLocalState|TestUnconfiguredCloudKeepsLocalCommandDefaults"` | ✅ multiple command paths (`serve/mcp/search/context/sync`) and multiple error branches | ✅ extracted cloud command flow into separate file to keep main readable |
| 1.2 | `internal/sync/remote_transport_contract_test.go` | Unit | ✅ `go test ./internal/sync -run "TestFileTransport|TestNewWithTransport|TestNewLocal"` | ✅ Added failing contract test importing new remote transport | ✅ `go test ./internal/sync -run "TestRemoteTransportImplementsTransportContract|TestFileTransportReadManifestMissing"` | ✅ validated manifest read + no-op write contract parity | ➖ none needed |
| 1.3 | `go test ./internal/cloud/...` (compilation gate) | Unit | N/A (new packages) | ✅ cloud package imports initially missing | ✅ `go test ./internal/cloud/...` | ➖ structural import task | ✅ normalized package boundaries (`cloudserver` separate from `server`) |
| 1.4 | `openspec/changes/integrate-engram-cloud/design.md` | Doc/Design | N/A | ✅ explicit non-goal section absent before edit | ✅ verified file update | ➖ structural/doc task | ✅ deferred script touch points made explicit |
| 1.5 | `go test ./internal/cloud/...` | Unit | N/A (new packages) | ✅ constructors/types missing before stubs | ✅ compile passes with stubs | ➖ structural constructors | ✅ no runtime wiring side-effects introduced |
| 1.6 | `internal/cloud/constants/constants.go` + CLI usage | Unit | N/A (new package) | ✅ literals were duplicated/implicit | ✅ constants package used by cloud CLI | ✅ reasons list + target key centralized | ✅ single owner package for shared literals |
| 2.1 | `cmd/engram/main_test.go` | Unit | ✅ baseline cmd package test run | ✅ tests for cloud discoverability/invalid/missing args added first | ✅ cmd tests passing | ✅ multiple invocation shapes (`cloud`, `cloud nope`, `cloud enroll`) | ✅ helper-based exit coverage kept in existing style |
| 2.2 | `cmd/engram/main_extra_test.go` | Unit | ✅ baseline cmd package test run | ✅ isolation test fails before `cmdCloud` exists | ✅ passes after implementation | ✅ checks both DB and cloud config mutation absence | ➖ none needed |
| 2.3 | `cmd/engram/main.go`, `cmd/engram/cloud.go` + cmd tests | Unit | ✅ cmd suite baseline | ✅ invalid command path tests added first | ✅ `go test ./cmd/engram` | ✅ status/enroll/config branches + validation errors | ✅ command tree moved out of `main.go` |
| 2.4 | `cmd/engram/cloud.go` | Unit | N/A | ✅ refs to command helpers missing before split | ✅ compile + tests pass after split | ➖ structural split task | ✅ dedicated cloud command module |
| 3.1 | `internal/store/store_test.go` | Unit | ✅ `go test ./internal/store -run "TestStoreLocalSyncFoundationStateHelpers|TestListPendingFiltersNonEnrolledProjects|TestSkipAckNonEnrolledMutationsBasic"` | ✅ Added failing reason assertions over missing API/fields (`MarkSyncBlocked`, `MarkSyncPaused`, `MarkSyncAuthRequired`, `reason_code`, `reason_message`) | ✅ `go test ./internal/store -run "TestSyncStateDeterministicReasonCodes|TestStoreLocalSyncFoundationStateHelpers"` | ✅ covered four deterministic reasons with distinct mark paths/messages | ✅ reused generic blocked helper + wrappers to reduce drift |
| 3.2 | `internal/store/store.go` + `internal/store/store_test.go` | Unit | ✅ same store baseline | ✅ tests referenced reason persistence fields/methods that did not exist | ✅ store tests pass with schema migration + state read/write updates | ✅ healthy path verifies reason fields clear; transport failure retains existing failure semantics | ✅ reason fields added as additive migration columns (no lifecycle contract break) |
| 3.3 | `internal/sync/sync_test.go` | Unit | ✅ `go test ./internal/sync -run "TestExportImportFlowWithProjectFilter|TestStatus|TestRemoteTransportImplementsTransportContract"` | ✅ Added failing cloud-mode tests referencing missing constructor/preflight flow | ✅ `go test ./internal/sync -run "TestCloudSyncPreflightBlocksUnenrolledBeforeTransport|TestCloudSyncEnrolledExportImportAndIdempotentPull"` | ✅ validated rejection (no transport calls) and two-step import idempotency | ✅ extracted reusable fake cloud transport for deterministic transport accounting |
| 3.4 | `internal/sync/sync.go` | Unit | ✅ sync baseline above | ✅ cloud constructor/preflight not present before test | ✅ cloud-mode syncer now enforces preflight while local constructors unchanged | ✅ export+import paths exercise preflight and normal flow | ✅ additive `NewCloudWithTransport` preserves local default constructors |
| 3.5 | `cmd/engram/main.go` | Unit | ✅ cmd baseline (`TestCmdServeParsesPortAndErrors`, `TestUnconfiguredCloudKeepsLocalCommandDefaults`) | ✅ new tests failed on missing shared preflight/autosync seams | ✅ `go test ./cmd/engram -run "TestCmdServeAutosyncLifecycleGating|TestCmdSyncCloudPreflightAuthRequired"` | ✅ same helper now used by sync cloud mode and autosync startup gate | ✅ single `preflightCloudSync` owner for auth + enrollment semantics |
| 4.1 | `cmd/engram/main_extra_test.go` | Unit | ✅ cmd baseline above | ✅ startup lifecycle tests introduced before runtime wiring changes | ✅ targeted cmd tests pass for local-off/cloud-on lifecycle gating | ✅ both disabled and enabled+enrolled branches covered | ✅ seam-based manager injection avoids brittle process-level integration |
| 4.2 | `cmd/engram/main.go` | Unit | ✅ `TestCmdServeParsesPortAndErrors` baseline | ✅ no autosync manager seam existed for gating tests | ✅ serve path now conditionally starts manager and wires `NotifyDirty` | ✅ cloud enablement/env + enrollment gate + config/auth gate all validated via tests | ✅ store-backed sync status provider added without moving server ownership boundaries |
| 4.3 | `internal/server/server_test.go` | Unit | ✅ `go test ./internal/server -run "TestSyncStatusNotConfigured|TestSyncStatusDegraded"` | ✅ new failing test asserted reason parity fields absent from response | ✅ `go test ./internal/server -run "TestSyncStatusIncludesReasonParityFields|TestSyncStatusDegraded|TestSyncStatusNotConfigured"` | ✅ degraded + reason-specific parity scenarios exercised | ✅ kept provider abstraction intact; added only additive response fields |
| 4.4 | `internal/server/server.go` | Unit | ✅ server status baseline above | ✅ response lacked reason fields before test | ✅ payload now includes `reason_code` + `reason_message` with existing fields unchanged | ➖ single payload contract extension | ✅ additive API evolution, no route ownership changes |
| 4.5 | `internal/cloud/dashboard/dashboard_test.go` | Unit | ✅ `go test ./internal/cloud/dashboard` (baseline no tests) | ✅ added failing parity test referencing missing `SyncStatus`/`HandlerWithStatus` contract | ✅ `go test ./internal/cloud/dashboard -run TestHandlerWithStatusRendersDeterministicReasonParity` | ✅ blocked/auth/transport scenarios assert exact `reason_code` + `reason_message` parity and human label mapping | ✅ extracted `reasonHeadline` and status-provider seam; updated `.templ` status component fields |
| 5.1 | `README.md`, `DOCS.md` | Docs | N/A | ✅ missing executable cloud workflow/examples for actual env/config behavior | ✅ docs updated and validated against existing CLI contract (`cloud status|enroll|config --server`, `sync --cloud`) | ✅ covers opt-in flow + deterministic reason-code set + runtime env toggles | ✅ copy consolidated around local-first source-of-truth |
| 5.2 | `docs/AGENT-SETUP.md`, `docs/PLUGINS.md` | Docs | N/A | ✅ deferred validation boundary not explicit enough for setup/plugin cloud automation | ✅ docs now explicitly scope plugin/setup validation to memory/session flows only | ➖ structural/doc-only | ✅ clearer rollout boundary language to avoid implied automation |
| 5.3 | Focused verification commands | Integration | ✅ focused package baseline already green | ✅ parity matrix requirement pending command proof | ✅ `go test ./cmd/engram ./internal/sync ./internal/store ./internal/server ./internal/cloud/...` | ✅ matrix covered by existing tests: unconfigured, unenrolled, auth-required, transport-failed + dashboard parity | ➖ none needed |
| 5.4 | Full regression + coverage gates | Integration | ✅ focused suite green before full run | ✅ completion gate pending full commands | ✅ `go test ./...` + `go test -cover ./...` both pass | ➖ gate task only | ➖ none needed |

## Test Summary
- **Total tests written/updated (cumulative)**: prior phase tests + 5 new focused cloud-integration tests (`store` + `sync` + `cmd` + `server`)
- **Targeted commands executed (this pass)**:
  - `go test ./internal/store -run "TestStoreLocalSyncFoundationStateHelpers|TestListPendingFiltersNonEnrolledProjects|TestSkipAckNonEnrolledMutationsBasic"`
  - `go test ./internal/store -run "TestSyncStateDeterministicReasonCodes|TestStoreLocalSyncFoundationStateHelpers"`
  - `go test ./internal/store -run "TestSyncStateDeterministicReasonCodes|TestStoreLocalSyncFoundationStateHelpers|TestApplyRemoteMutationIdempotent"`
  - `go test ./internal/sync -run "TestExportImportFlowWithProjectFilter|TestStatus|TestRemoteTransportImplementsTransportContract"`
  - `go test ./internal/sync -run "TestCloudSyncPreflightBlocksUnenrolledBeforeTransport|TestCloudSyncEnrolledExportImportAndIdempotentPull"`
  - `go test ./internal/sync -run "TestCloudSyncPreflightBlocksUnenrolledBeforeTransport|TestCloudSyncEnrolledExportImportAndIdempotentPull|TestStatus"`
  - `go test ./cmd/engram -run "TestCmdServeParsesPortAndErrors|TestUnconfiguredCloudKeepsLocalCommandDefaults|TestCmdSyncAdditionalBranches"`
  - `go test ./cmd/engram -run "TestCmdServeAutosyncLifecycleGating|TestCmdSyncCloudPreflightAuthRequired"`
  - `go test ./cmd/engram -run "TestUnconfiguredCloudKeepsLocalCommandDefaults|TestCmdSyncAdditionalBranches|TestCmdServeParsesPortAndErrors"`
  - `go test ./internal/server -run "TestSyncStatusIncludesReasonParityFields|TestSyncStatusDegraded|TestSyncStatusNotConfigured"`
  - `go test ./cmd/engram ./internal/sync ./internal/store ./internal/server ./internal/cloud/...`
  - `go test ./internal/cloud/dashboard -run TestHandlerWithStatusRendersDeterministicReasonParity`
  - `go test ./internal/cloud/dashboard`
  - `go test ./...`
  - `go test -cover ./...`
- **Approval tests**: None — no pure behavior-preserving refactor-only task.
- **Pure functions created**: 4 (`parseRFC3339Ptr`, `derefString`, `envBool`, `resolveCloudRuntimeConfig`)

## Remaining Tasks
- [x] 4.5 dashboard deterministic reason rendering/parity tests + template integration
- [x] 5.1 docs alignment (`README.md`, `DOCS.md`, cloud docs pages)
- [x] 5.2 deferred plugin/script boundary docs updates (`docs/AGENT-SETUP.md`, `docs/PLUGINS.md`)
- [x] 5.3 verify explicit parity matrix coverage (unconfigured, unenrolled, auth-failed, transport-failed) in docs/checklist form
- [x] 5.4 full regression + coverage gates (`go test ./...`, `go test -cover ./...`) in completion batch

## Continuation Batch — Phase 6 Demoable Runtime

### Completed Tasks (new in this batch)
- [x] 6.1 RED CLI/runtime tests for `engram cloud serve` discovery and startup wiring.
- [x] 6.2 GREEN implementation of runnable `engram cloud serve` path with cloud runtime constructor seam.
- [x] 6.3 RED cloudserver handler tests for health, push/pull manifest+chunk, auth rejection, and dashboard mount.
- [x] 6.4 GREEN implementation of cloudserver HTTP handlers + Postgres-backed cloudstore manifest/chunk persistence.
- [x] 6.5 REFACTOR docs/runtime packaging alignment with `docker-compose.cloud.yml` + `docker/cloud/Dockerfile` and local smoke evidence.

### TDD Cycle Evidence (Phase 6)
| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 6.1 | `cmd/engram/main_extra_test.go`, `cmd/engram/main_test.go` | Unit | ✅ `go test ./cmd/engram -run "TestCloudCommandIsolationDoesNotMutateLocalState|TestPrintUsage"` | ✅ Added failing test requiring `cloud serve` runtime seam + usage line | ✅ `go test ./cmd/engram -run "TestCmdCloudServeStartsCloudRuntime|TestPrintUsage"` | ✅ startup path + discoverability path | ✅ extracted runtime seam (`newCloudRuntime`) to keep CLI parse logic clean |
| 6.2 | `cmd/engram/main_extra_test.go` | Unit | ✅ same cmd baseline | ✅ runtime constructor/start call absent before test | ✅ cmd package tests green with `cmdCloudServe` | ✅ validated runtime config propagation and start invocation | ➖ none needed |
| 6.3 | `internal/cloud/cloudserver/cloudserver_test.go` | Unit | N/A (new package tests) | ✅ added failing tests for `/health`, `/sync/push`, `/sync/pull`, `/sync/pull/{chunkID}`, `/dashboard`, and auth failure | ✅ `go test ./internal/cloud/cloudserver` | ✅ covered success + error/unauthorized branches | ✅ introduced handler/auth wrappers to avoid duplicated branch logic |
| 6.4 | `internal/cloud/cloudstore/cloudstore_test.go`, `internal/cloud/auth/auth_test.go`, `internal/cloud/cloudserver/cloudserver_test.go` | Unit | ✅ `go test ./internal/cloud/cloudserver` baseline | ✅ new tests fail before persistent cloudstore/auth behavior exists | ✅ `go test ./internal/cloud/auth ./internal/cloud/cloudstore ./internal/cloud/cloudserver` | ✅ token/no-token auth + chunk summary parse branches + push/pull path | ✅ `summarizeChunk` extracted as pure helper to keep handler/storage logic thin |
| 6.5 | Docs + runtime smoke artifacts | Integration/Docs | ✅ package tests green before docs/smoke | ✅ docker/local runtime path missing before this batch | ✅ smoke: Postgres container + `go run ./cmd/engram cloud serve` with 200 responses for health/push/pull/chunk | ✅ exercised health + mutation + manifest + chunk retrieval end-to-end | ✅ added Docker assets + docs mapping to real runtime paths |

### Test Summary (Phase 6 batch)
- **Total tests written/updated (this batch)**: 7
  - `TestCmdCloudServeStartsCloudRuntime`
  - `TestPrintUsage` assertion for cloud serve discoverability
  - `TestHandlerMountsDashboardAndHealth`
  - `TestHandlerSyncPushPullRoundTrip`
  - `TestHandlerReturnsUnauthorizedWhenAuthFails`
  - `TestNewServiceSecretValidation` + `TestAuthorizeBearerToken`
  - `TestNewRequiresDSN` + `TestSummarizeChunkCountsEntities`
- **Commands executed**:
  - `go test ./cmd/engram ./internal/cloud/cloudserver -run "TestCmdCloudServeStartsCloudRuntime|TestPrintUsage|TestHandlerMountsDashboardAndHealth|TestHandlerSyncPushPullRoundTrip|TestHandlerReturnsUnauthorizedWhenAuthFails"` (RED then GREEN)
  - `go test ./internal/cloud/auth ./internal/cloud/cloudstore ./internal/cloud/cloudserver ./cmd/engram`
  - `go test ./...`
  - `go test -cover ./...`
  - Docker smoke (Postgres + local cloud runtime via `go run ./cmd/engram cloud serve`) with observed `health=200 push=200 manifest=200 chunk=200`.
- **Approval tests**: None.
- **Pure functions created**: 1 (`summarizeChunk`).

## Continuation Batch — Phase 6.6 Compose Host Reachability

### Completed Tasks (new in this batch)
- [x] 6.6 RED/GREEN compose host-reachability caveat fix for cloud runtime bind address.

### TDD Cycle Evidence (Phase 6.6)
| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 6.6 | `internal/cloud/config_test.go`, `internal/cloud/cloudserver/cloudserver_test.go`, `cmd/engram/main_extra_test.go` | Unit + Compose Smoke | ✅ `go test ./internal/cloud/cloudserver -run "TestHandlerMountsDashboardAndHealth|TestHandlerSyncPushPullRoundTrip|TestHandlerReturnsUnauthorizedWhenAuthFails"` and `go test ./cmd/engram -run "TestCmdCloudServeStartsCloudRuntime|TestPrintUsage"` | ✅ Added failing tests first for missing bind-host config (`BindHost` field + env parsing + runtime propagation + start bind address) | ✅ `go test ./internal/cloud ./internal/cloud/cloudserver ./cmd/engram -run "TestConfigFromEnvCloudHost|TestStartBindsConfiguredHost|TestCmdCloudServeStartsCloudRuntime"` | ✅ covered both host paths (`127.0.0.1` local default + `0.0.0.0` compose/container) and validated runtime config path from env to server | ✅ extracted host option seam in `cloudserver.WithHost`, kept loopback as default safety, documented compose override + updated compose env |

### Test Summary (Phase 6.6 batch)
- **Total tests written/updated (this batch)**: 3
  - `TestConfigFromEnvCloudHost` (default + env override)
  - `TestStartBindsConfiguredHost` (loopback + container bind assertions)
  - `TestCmdCloudServeStartsCloudRuntime` (bind host propagation assertion)
- **Commands executed**:
  - `go test ./internal/cloud ./internal/cloud/cloudserver ./cmd/engram -run "TestConfigFromEnvCloudHost|TestStartBindsConfiguredHost|TestCmdCloudServeStartsCloudRuntime"` (RED then GREEN)
  - `go test ./internal/cloud/... ./cmd/engram -run "TestConfigFromEnvCloudHost|TestStartBindsConfiguredHost|TestCmdCloudServeStartsCloudRuntime|TestHandlerMountsDashboardAndHealth|TestHandlerSyncPushPullRoundTrip|TestHandlerReturnsUnauthorizedWhenAuthFails"`
  - `go test ./...`
  - `go test -cover ./...`
  - `docker compose -f docker-compose.cloud.yml up -d postgres`
  - `docker compose -f docker-compose.cloud.yml up -d --build --pull never cloud`
  - `curl http://127.0.0.1:18080/health` => `200`
  - `curl http://127.0.0.1:18080/dashboard` => `200`
  - `curl -H "Authorization: Bearer smoke-token" -X POST http://127.0.0.1:18080/sync/push ...` => `200`
  - `curl -H "Authorization: Bearer smoke-token" http://127.0.0.1:18080/sync/pull` => `200`
  - `curl -H "Authorization: Bearer smoke-token" http://127.0.0.1:18080/sync/pull/compose-bind-smoke-1` => `200`
  - `docker compose -f docker-compose.cloud.yml down`
- **Approval tests**: None.
- **Pure functions created**: 0.
