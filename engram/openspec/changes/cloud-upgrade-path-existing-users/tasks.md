# Tasks: Cloud upgrade path for existing users

## Phase 1: Upgrade foundation contracts (TDD-first)

- [x] 1.1 **RED** Add `TestUpgradeStateSnapshotLifecycle` in `internal/store/store_test.go` covering snapshot save/load/clear and rollback boundary markers.
- [x] 1.2 **GREEN** Implement upgrade snapshot state persistence in `internal/store/store.go` (project-scoped, additive, no local data mutation).
- [x] 1.3 **RED** Add `TestUpgradeDeterministicReasonCodes` in `internal/sync/sync_test.go` for `repairable|blocked|policy|ready` and loud failures.
- [x] 1.4 **GREEN** Add shared upgrade reason/status contract in `internal/cloud/constants/constants.go` and wire `internal/sync/sync.go` to emit deterministic codes.
- [x] 1.5 **REFACTOR** Extract/centralize upgrade status types in `internal/sync/sync.go` (or `internal/sync/upgrade.go`) to avoid CLI/dashboard drift.

## Phase 2: Doctor and repair flow

- [x] 2.1 **RED** Add CLI tests in `cmd/engram/main_extra_test.go` for `engram cloud upgrade doctor --project <p>` categorized output and exit behavior.
- [x] 2.2 **GREEN** Implement `cloud upgrade doctor` command wiring in `cmd/engram/cloud.go` using store/sync contracts only.
- [x] 2.3 **RED** Add `TestUpgradeRepairDryRunAndApply` in `internal/store/store_test.go` for deterministic repairs vs blocked ambiguities.
- [x] 2.4 **GREEN** Implement repair planner/apply in `internal/store/store.go` (journal backfill, project normalization, delete backfill, idempotent changelog).
- [x] 2.5 **RED** Add validation classification tests in `internal/cloud/cloudserver/cloudserver_test.go` for machine-actionable migration error payloads.
- [x] 2.6 **GREEN** Propagate typed migration/repair failure classes in `internal/cloud/cloudserver/cloudserver.go` and `internal/cloud/remote/transport.go`.

## Phase 3: Bootstrap, status, rollback

- [x] 3.1 **RED** Add `TestUpgradeBootstrapCheckpointResume` in `internal/sync/sync_test.go` for idempotent first push + verification pull.
- [x] 3.2 **GREEN** Implement bootstrap orchestration in `internal/sync/sync.go`: snapshot checkpoint → enroll backfill → first push → verify pull/status.
- [x] 3.3 **RED** Add CLI tests in `cmd/engram/main_extra_test.go` for `bootstrap --resume`, `status`, and `rollback` boundary semantics.
- [x] 3.4 **GREEN** Implement `bootstrap|status|rollback` commands in `cmd/engram/cloud.go` and integrate `cmd/engram/main.go` without changing steady-state `sync --cloud` defaults.
- [x] 3.5 **RED** Add rollback safety tests in `internal/store/store_test.go` and `internal/sync/sync_test.go` (allowed pre-bootstrap-complete; blocked after remote commit).
- [x] 3.6 **GREEN** Implement rollback restore in `internal/store/store.go` and stop/resume hooks in `internal/cloud/autosync/manager.go` with loud post-boundary failures.
- [x] 3.7 **RED** Add parity tests in `internal/cloud/dashboard/dashboard_test.go` and `internal/server/server_test.go` for upgrade phase/reason exposure.
- [x] 3.8 **GREEN** Implement upgrade status surfaces in `internal/cloud/dashboard/dashboard.go` and `internal/server/server.go`.

## Phase 4: Additive rollout docs and compatibility

- [x] 4.1 **RED** Add regression tests (`cmd/engram/main_test.go`, `internal/sync/sync_test.go`) proving existing `engram sync --cloud` behavior is unchanged after upgrade additions.
- [x] 4.2 **GREEN** Finalize additive wiring/refactor to keep local-first defaults and explicit cloud opt-in behavior.
- [x] 4.3 Update `README.md`, `DOCS.md`, `docs/AGENT-SETUP.md`, and `docs/PLUGINS.md` with `doctor → repair → bootstrap → status/rollback` workflow and rollback boundary.

## Phase 5: Verification gates

- [x] 5.1 Run focused suites: `go test ./cmd/engram ./internal/store ./internal/sync ./internal/cloud/... ./internal/server`.
- [x] 5.2 Run full validation: `go test ./...`.
- [x] 5.3 Run coverage gate: `go test -cover ./...` and record gaps/follow-ups in the change notes.
