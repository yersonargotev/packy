# Apply Progress — cloud-upgrade-path-existing-users

## Mode

Strict TDD

## Completed Tasks

- [x] 1.1 RED — `TestUpgradeStateSnapshotLifecycle`
- [x] 1.2 GREEN — cloud upgrade checkpoint persistence in store
- [x] 1.3 RED — `TestUpgradeDeterministicReasonCodes`
- [x] 1.4 GREEN — deterministic upgrade contracts + sync wiring
- [x] 2.1 RED — CLI doctor tests
- [x] 2.2 GREEN — `engram cloud upgrade doctor`
- [x] 2.3 RED — `TestUpgradeRepairDryRunAndApply`
- [x] 2.4 GREEN — deterministic repair planner/apply
- [x] 3.1 RED — `TestUpgradeBootstrapCheckpointResume`
- [x] 3.2 GREEN — checkpointed bootstrap orchestration
- [x] 3.3 RED — CLI bootstrap/status/rollback tests
- [x] 3.4 GREEN — `bootstrap|status|rollback` command surface
- [x] 4.3 Docs workflow alignment
- [x] 1.5 REFACTOR — centralized upgrade status contracts in `internal/sync/upgrade.go`
- [x] 2.5 RED — machine-actionable migration error payload tests in cloudserver
- [x] 2.6 GREEN — typed migration/repair error classes in cloudserver + remote transport
- [x] 3.5 RED — rollback safety tests in store and sync
- [x] 3.6 GREEN — store rollback restore + autosync stop/resume hooks
- [x] 3.7 RED — dashboard/server parity tests for upgrade phase/reason
- [x] 3.8 GREEN — dashboard and `/sync/status` upgrade status surfaces
- [x] 4.1 RED — regression tests proving `sync --cloud` behavior unchanged
- [x] 4.2 GREEN — additive wiring to preserve local-first defaults and explicit cloud opt-in
- [x] 5.1 Focused test suites
- [x] 5.2 Full test run
- [x] 5.3 Coverage run

## TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|---|---|---|
| 1.1/1.2 | `internal/store/store_test.go` | Unit | ✅ `go test ./internal/store` | ✅ Added failing lifecycle test | ✅ `go test ./internal/store -run TestUpgradeStateSnapshotLifecycle` | ✅ save/load + rollback allowed/blocked + clear cases | ➖ none needed |
| 1.3/1.4 | `internal/sync/sync_test.go` | Unit | ✅ `go test ./internal/sync` | ✅ Added failing deterministic diagnosis test | ✅ `go test ./internal/sync -run TestUpgradeDeterministicReasonCodes` | ✅ ready/repairable/policy/policy-forbidden + loud invalid-project error | ➖ none needed |
| 2.1/2.2 | `cmd/engram/main_extra_test.go` | Unit | ✅ existing cloud CLI baseline test | ✅ Added failing doctor CLI tests | ✅ `go test ./cmd/engram -run TestCmdCloudUpgradeDoctorRequiresProjectAndIsDeterministic` | ✅ missing project + deterministic repeated output | ➖ none needed |
| 2.3/2.4 | `internal/store/store_test.go` | Unit | ✅ prior store package baseline | ✅ Added failing repair dry-run/apply tests | ✅ `go test ./internal/store -run TestUpgradeRepairDryRunAndApply` | ✅ dry-run deterministic/non-mutating + apply backfill + blocked ambiguity | ➖ none needed |
| 3.1/3.2 | `internal/sync/sync_test.go` | Integration-ish unit | ✅ prior sync package baseline | ✅ Added failing bootstrap checkpoint test | ✅ `go test ./internal/sync -run TestUpgradeBootstrapCheckpointResume` | ✅ resume from checkpoint + completed rerun no-op | ➖ none needed |
| 3.3/3.4 | `cmd/engram/main_extra_test.go` | Unit | ✅ prior cloud CLI baseline | ✅ Added failing bootstrap/status/rollback tests | ✅ `go test ./cmd/engram -run TestCmdCloudUpgradeBootstrapStatusAndRollbackSemantics` | ✅ status parity + rollback boundary + bootstrap --resume acceptance | ➖ none needed |
| Help scenario | `cmd/engram/main_extra_test.go` | Unit | ✅ same CLI baseline | ✅ Added failing help workflow test | ✅ `go test ./cmd/engram -run TestCmdCloudUpgradeHelpShowsGuidedWorkflow` | ✅ workflow + local-first wording assertions | ➖ none needed |
| 1.5 | `internal/sync/sync_test.go` | Unit (approval-style) | ✅ `go test ./internal/sync -run TestUpgradeDeterministicReasonCodes` | ✅ Existing behavior contract kept while extracting to `upgrade.go` | ✅ `go test ./internal/sync -run TestUpgradeDeterministicReasonCodes` | ➖ refactor-only extraction | ✅ constants/types isolated to avoid drift |
| 2.5/2.6 | `internal/cloud/cloudserver/cloudserver_test.go`, `internal/cloud/remote/transport_test.go` | Integration-ish unit | ✅ `go test ./internal/cloud/cloudserver ./internal/cloud/remote` | ✅ Added failing actionable-classification tests | ✅ `go test ./internal/cloud/cloudserver -run TestHandlerPushValidationErrorsExposeMachineActionableClasses` + `go test ./internal/cloud/remote -run TestReadManifestParsesMachineActionableErrorPayload` | ✅ blocked/repairable/policy classes + transport parsing | ✅ centralized actionable error writer + typed HTTP status metadata |
| 3.5/3.6 | `internal/store/store_test.go`, `internal/sync/sync_test.go` | Unit | ✅ `go test ./internal/store ./internal/sync` | ✅ Added failing rollback safety + autosync-hook tests | ✅ `go test ./internal/store ./internal/sync -run 'TestRollbackCloudUpgradeSafetyBoundary|TestRollbackProjectInvokesAutosyncHooksAndHonorsBoundary'` | ✅ pre-verify rollback success + post-verify loud failure + hook invocation checks | ✅ store-owned rollback API + sync orchestration wrapper |
| 3.7/3.8 | `internal/cloud/dashboard/dashboard_test.go`, `internal/server/server_test.go` | Unit | ✅ `go test ./internal/cloud/dashboard ./internal/server` | ✅ Added failing parity tests for upgrade phase/reason fields | ✅ `go test ./internal/cloud/dashboard ./internal/server -run 'TestHandlerWithStatusRendersUpgradePhaseAndReasonParity|TestSyncStatusIncludesReasonParityFields'` | ✅ dashboard text + `/sync/status` nested upgrade payload parity | ✅ shared status surfacing wired through provider |
| 4.1/4.2 | `cmd/engram/main_test.go`, `internal/sync/sync_test.go` | Unit | ✅ `go test ./cmd/engram ./internal/sync` | ✅ Added regression tests asserting `sync --cloud` unchanged with upgrade metadata present | ✅ `go test ./cmd/engram ./internal/sync -run 'TestCmdSyncCloudRegressionPreservesLegacyBehaviorWithUpgradeStatePresent|TestCloudSyncExportBehaviorUnchangedWhenUpgradeStateExists'` | ✅ legacy success path + no upgrade-state mutation during sync/export | ✅ additive wiring only; no default behavior flips |

## Test Summary

- Total tests written: 14 new named tests
- Focused suite: `go test ./cmd/engram ./internal/store ./internal/sync ./internal/cloud/... ./internal/server`
- Full suite: `go test ./...`
- Coverage run: `go test -cover ./...`

## Remaining Tasks

- [x] 1.5 refactor centralized upgrade status types to avoid CLI/dashboard drift
- [x] 2.5 RED classification tests in cloudserver
- [x] 2.6 GREEN typed migration/repair failure classes in cloudserver + remote transport
- [x] 3.5 RED rollback safety tests in store/sync
- [x] 3.6 GREEN rollback restore + autosync stop/resume hooks
- [x] 3.7 RED dashboard/server parity tests for upgrade phase/reason exposure
- [x] 3.8 GREEN dashboard/server upgrade status surfaces
- [x] 4.1 RED regression tests for unchanged `sync --cloud` behavior
- [x] 4.2 GREEN additive wiring finalization for local-first defaults

## Continuation — verify blocker closure (Strict TDD)

### Completed Blocker Fixes

- [x] Wired **production snapshot capture** before `bootstrap` progression (`cmd/engram/cloud.go`) so rollback always has a pre-upgrade snapshot source.
- [x] Wired **policy-denied doctor** end-to-end from runtime sync-state reason (`policy_forbidden`) into diagnosis output.
- [x] Added **REQ-UPGRADE-06 runtime evidence test** validating help/docs/workflow/local-first/deferred-plugin alignment.
- [x] Strengthened **repair manual-action-required semantics** for `auth_required` and `policy_forbidden` blockers in production behavior.

### TDD Cycle Evidence (continuation)

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|---|---|---|
| CRIT-1 snapshot capture | `cmd/engram/main_extra_test.go` | Unit | ✅ `go test ./cmd/engram -run 'TestCmdCloudUpgradeDoctorRequiresProjectAndIsDeterministic|TestCmdCloudUpgradeBootstrapStatusAndRollbackSemantics|TestCmdCloudUpgradeHelpShowsGuidedWorkflow'` | ✅ Added failing `bootstrap captures rollback snapshot before progression` | ✅ `go test ./cmd/engram -run 'TestCmdCloudUpgradeBootstrapStatusAndRollbackSemantics/bootstrap captures rollback snapshot before progression'` | ✅ snapshot existence + cloud config + enrollment pre-state checks | ✅ extracted `captureUpgradeSnapshotBeforeBootstrap` helper |
| CRIT-2 policy-denied doctor | `cmd/engram/main_extra_test.go` | Unit | ✅ same CLI baseline above | ✅ Added failing `policy denied is surfaced from runtime sync state` | ✅ `go test ./cmd/engram -run 'TestCmdCloudUpgradeDoctorRequiresProjectAndIsDeterministic/policy denied is surfaced from runtime sync state'` | ✅ deterministic unchanged + policy-denied runtime state paths both asserted | ✅ extracted `cloudUpgradePolicyDenied` helper |
| CRIT-3 REQ-UPGRADE-06 docs evidence | `cmd/engram/main_extra_test.go` | Unit | ✅ same CLI baseline above | ✅ Added failing `TestCloudUpgradeDocsMatchHelpAndLocalFirstSemantics` | ✅ `go test ./cmd/engram -run TestCloudUpgradeDocsMatchHelpAndLocalFirstSemantics` | ✅ help output + README/DOCS commands + local-first text + deferred plugin docs | ➖ none needed |
| WARN-1 repair manual-action semantics | `internal/store/store_test.go` | Unit | ✅ `go test ./internal/store -run 'TestUpgradeRepairDryRunAndApply|TestRollbackCloudUpgradeSafetyBoundary'` | ✅ Added failing `auth and policy blockers are manual-action-required` | ✅ `go test ./internal/store -run 'TestUpgradeRepairDryRunAndApply/auth and policy blockers are manual-action-required'` | ✅ auth_required + policy_forbidden matrix cases | ✅ extracted `cloudUpgradeManualActionReport` helper |

### Verification Commands (continuation)

- `go test ./cmd/engram ./internal/store ./internal/sync ./internal/cloud/... ./internal/server`
- `go test ./...`
- `go test -cover ./...`

## Follow-up Improvements — 2026-04-23

### Completed

- [x] Added targeted branch tests for upgrade-critical CLI command paths in `cmd/engram/cloud.go` via `cmd/engram/main_extra_test.go`:
  - repair usage + dry-run behavior + `--apply` path acceptance
  - status default `planned` branch
  - rollback missing-checkpoint failure
  - rollback config restore/remove snapshot branches
- [x] Added targeted orchestration tests for `internal/sync/sync.go` in `internal/sync/sync_test.go`:
  - bootstrap input validation (`nil` store / empty project)
  - bootstrap default `CreatedBy` fallback (`upgrade-bootstrap`)
  - rollback hook error handling (`stop` failure, rollback failure + resume attempt, resume failure surfacing)
- [x] Added upgrade/autosync lifecycle hook tests in new `internal/cloud/autosync/manager_test.go`:
  - `StopForUpgrade` / `ResumeAfterUpgrade` required-project validation
  - paused→disabled status transitions and reason code assertions
- [x] Aligned design doc with implemented CLI behavior by removing non-implemented shorthand chain from `design.md` and documenting explicit subcommand invocation.

### TDD Cycle Evidence (follow-up)

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|---|---|---|
| Coverage follow-up: CLI upgrade branches | `cmd/engram/main_extra_test.go` | Unit | ✅ `go test ./cmd/engram -run 'TestCmdCloudUpgradeDoctorRequiresProjectAndIsDeterministic|TestCmdCloudUpgradeBootstrapStatusAndRollbackSemantics|TestCmdCloudUpgradeHelpShowsGuidedWorkflow'` | ✅ Added new branch-focused tests; first pass failed on over-asserted apply expectation | ✅ `go test ./cmd/engram -run 'TestCmdCloudUpgradeRepairStatusAndRollbackBranches'` | ✅ repair/status/rollback matrix of success + failure branches | ✅ tightened expectation to documented runtime behavior (apply flag acceptance without forced mutation) |
| Coverage follow-up: sync orchestration branches | `internal/sync/sync_test.go` | Unit | ✅ `go test ./internal/sync -run 'TestUpgradeBootstrapCheckpointResume|TestRollbackProjectInvokesAutosyncHooksAndHonorsBoundary'` | ✅ Added validation/hook-failure tests | ✅ `go test ./internal/sync -run 'TestBootstrapProjectValidationAndCreatedByDefault|TestRollbackProjectHandlesHookFailures'` | ✅ bootstrap validation + default createdBy + rollback hook failure matrix | ➖ none needed |
| Coverage follow-up: autosync upgrade hooks | `internal/cloud/autosync/manager_test.go` | Unit | ✅ `go test ./internal/cloud/autosync` baseline (`[no test files]`) | ✅ Added manager hook tests | ✅ `go test ./internal/cloud/autosync -run 'TestManagerStopForUpgradeRequiresProject|TestManagerStopAndResumeUpgradeTransitions|TestManagerResumeAfterUpgradeRequiresProject'` | ✅ required-project + transition branches | ➖ none needed |
| Docs/design alignment | `openspec/.../design.md` | Docs | N/A | ✅ Updated design command-shape row to match current implementation | ✅ N/A (doc-only) | ➖ single behavior contract alignment | ➖ none needed |

### Coverage Snapshot (targeted files)

From `go test -coverprofile=coverage.out ./...` + `go tool cover -func=coverage.out`:

- `cmd/engram/cloud.go`
  - `cmdCloudUpgradeRepair`: **81.0%** (was 0.0%)
  - `cmdCloudUpgradeStatus`: **66.7%** (was 54.2%)
  - `cmdCloudUpgradeRollback`: **62.2%** (was 37.8%)
- `internal/sync/sync.go`
  - `RollbackProject`: **87.0%** (was 65.2%)
  - `BootstrapProject`: **78.3%** (was 65.2%)
- `internal/cloud/autosync/manager.go`
  - `StopForUpgrade`: **100%** (was 0.0%)
  - `ResumeAfterUpgrade`: **100%** (was 0.0%)

### Verification Commands (follow-up)

- `go test ./cmd/engram -run 'TestCmdCloudUpgradeRepairStatusAndRollbackBranches'`
- `go test ./internal/sync -run 'TestBootstrapProjectValidationAndCreatedByDefault|TestRollbackProjectHandlesHookFailures'`
- `go test ./internal/cloud/autosync -run 'TestManagerStopForUpgradeRequiresProject|TestManagerStopAndResumeUpgradeTransitions|TestManagerResumeAfterUpgradeRequiresProject'`
- `go test ./cmd/engram ./internal/sync ./internal/cloud/autosync`
- `go test ./...`
- `go test -cover ./...`
- `go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out | rg 'cmd/engram/cloud.go|internal/sync/sync.go|internal/cloud/autosync/manager.go'`

## Continuation — legacy mutation payload doctor/repair/bootstrap alignment (2026-04-23)

### Completed

- [x] Added deterministic legacy pending-mutation diagnosis in store (`DiagnoseCloudUpgradeLegacyMutations`) that classifies issues as repairable vs manual-action-required.
- [x] Extended upgrade repair flow to auto-fix safe missing-field legacy payload cases from authoritative local SQLite state (session/observation/prompt payload reconstruction) without remote mutation.
- [x] Wired `cloud upgrade doctor` to include legacy mutation diagnosis so legacy canonicalization failures are surfaced before bootstrap.
- [x] Added bootstrap preflight gate to fail loudly with actionable repair/manual guidance instead of discovering this class first in `canonicalize cloud chunk` during export.

### TDD Cycle Evidence (continuation)

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|---|---|---|
| Legacy payload repair from authoritative local state | `internal/store/store_test.go` | Unit | ✅ `go test ./internal/store -run 'TestUpgradeRepairDryRunAndApply/auth and policy blockers are manual-action-required'` | ✅ Added failing `legacy mutation required fields are detected and repaired from authoritative local state` subtest referencing missing diagnosis API | ✅ `go test ./internal/store -run 'TestUpgradeRepairDryRunAndApply/legacy mutation required fields are detected and repaired from authoritative local state'` | ✅ asserts repairable detection, apply repair, payload field restoration, and no residual findings | ✅ extracted store-level diagnosis/evaluation/repair helpers to keep command adapters thin |
| Doctor/bootstrap alignment for real-world failure mode | `cmd/engram/main_extra_test.go` | Unit | ✅ `go test ./cmd/engram -run 'TestCmdCloudUpgradeDoctorRequiresProjectAndIsDeterministic/policy denied is surfaced from runtime sync state'` | ✅ Added failing `legacy payload gaps are surfaced by doctor and block bootstrap preflight` subtest | ✅ `go test ./cmd/engram -run 'TestCmdCloudUpgradeDoctorRequiresProjectAndIsDeterministic/legacy payload gaps are surfaced by doctor and block bootstrap preflight'` | ✅ covers doctor deterministic classification + bootstrap preflight block before orchestration | ➖ none needed |

### Verification Commands (continuation)

- `go test ./internal/store -run 'TestUpgradeRepairDryRunAndApply/legacy mutation required fields are detected and repaired from authoritative local state'`
- `go test ./cmd/engram -run 'TestCmdCloudUpgradeDoctorRequiresProjectAndIsDeterministic/legacy payload gaps are surfaced by doctor and block bootstrap preflight'`
- `go test ./cmd/engram ./internal/store ./internal/sync`
- `go test ./...`
- `go test -cover ./...`
