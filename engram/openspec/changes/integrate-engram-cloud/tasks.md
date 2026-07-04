# Tasks: Integrate Engram Cloud

## Phase 1: Foundation Import & Conflict Reconciliation

- [x] 1.1 **RED** Add non-regression tests in `cmd/engram/main_test.go` + `cmd/engram/main_extra_test.go` proving unconfigured cloud keeps `serve/mcp/search/context/sync` behavior and exit codes unchanged.
- [x] 1.2 **RED** Add contract tests in `internal/sync/transport_test.go` for transport parity (file transport unchanged, remote transport must satisfy the same interface expectations).
- [x] 1.3 Import additive packages/assets: `internal/cloud/{auth,cloudstore,cloudserver,remote,autosync,dashboard}` and `internal/cloud/dashboard/*.templ`, normalizing module paths to this repo.
- [x] 1.4 Reconcile conflicting files: update `go.mod`/`go.sum`, resolve package-name collisions (`server` vs `cloudserver`), and record deferred script-touch points in `openspec/changes/integrate-engram-cloud/design.md` as explicit non-goals.
- [x] 1.5 **GREEN** Add minimal constructors/wiring stubs in imported cloud packages so tests compile without changing local runtime defaults.
- [x] 1.6 **REFACTOR** Extract shared cloud constants/types (target key, reason-code literals) into a single owned package to avoid drift across CLI/server/autosync.

## Phase 2: Cloud CLI Surface (Strict TDD)

- [x] 2.1 **RED** Add CLI tests in `cmd/engram/main_test.go` for help discovery (`engram cloud` visible), unknown subcommand failure text, and required-arg errors.
- [x] 2.2 **RED** Add isolation tests proving cloud commands do not mutate local state unless the command explicitly updates cloud config.
- [x] 2.3 **GREEN** Implement `engram cloud ...` command tree and cloud config/env parsing in `cmd/engram/main.go` with actionable stderr on invalid invocations.
- [x] 2.4 **REFACTOR** Split cloud command parsing/validation helpers from `main.go` to keep local command paths readable and unchanged.

## Phase 3: Enrollment Gate, Sync Transport, and Deterministic Failures

- [x] 3.1 **RED** Add store tests in `internal/store/store_test.go` for deterministic blocked reasons (`blocked_unenrolled`, `paused`, `auth_required`, `transport_failed`).
- [x] 3.2 **GREEN** Extend `internal/store/store.go` sync-state helpers to persist reason code/message without breaking existing local lifecycle semantics.
- [x] 3.3 **RED** Add sync tests in `internal/sync/sync_test.go` for enrolled push/pull success, unenrolled preflight rejection (no network call), and idempotent pulled mutation apply.
- [x] 3.4 **GREEN** Implement cloud transport path in `internal/sync/{sync.go,transport.go}` via `internal/cloud/remote` while preserving local transport defaults.
- [x] 3.5 **REFACTOR** Centralize preflight enrollment/auth checks so CLI-triggered sync and autosync share the exact same gate logic.

## Phase 4: Selective Runtime Wiring + Status Surface Parity

- [x] 4.1 **RED** Add startup wiring tests in `cmd/engram/main_test.go` proving autosync manager starts only for cloud-enabled long-lived processes and stays off for local-only runs.
- [x] 4.2 **GREEN** Wire cloud autosync lifecycle in `cmd/engram/main.go` using `internal/cloud/autosync` behind explicit enablement + enrollment checks.
- [x] 4.3 **RED** Add status parity tests in `internal/server/server_test.go` asserting `/sync/status` exposes the same reason code/message produced by store/autosync.
- [x] 4.4 **GREEN** Extend `internal/server/server.go` sync status payload to include cloud reason fields while preserving existing local API ownership.
- [x] 4.5 **RED/GREEN** Add dashboard tests + templates in `internal/cloud/dashboard/*` ensuring blocked/auth/network states render the same deterministic reasons shown by CLI/server.

## Phase 5: Docs Alignment and Focused Verification

- [x] 5.1 Update `README.md`, `DOCS.md`, and cloud-relevant `docs/*` pages with executable `engram cloud ...` examples, config keys, enrollment opt-in, and local-first source-of-truth statement.
- [x] 5.2 Mark plugin/script workflow validation as deferred scope in docs (`docs/AGENT-SETUP.md`, `docs/PLUGINS.md`) to reconcile command docs with current rollout boundaries.
- [x] 5.3 Run focused verification: `go test ./cmd/engram ./internal/sync ./internal/store ./internal/server ./internal/cloud/...` plus parity checks for unconfigured, unenrolled, auth-failed, and transport-failed paths.
- [x] 5.4 Run full regression and coverage gates: `go test ./...` and `go test -cover ./...`; fix remaining boundary regressions before moving to `sdd-apply` completion.

## Phase 6: Demoable Cloud Runtime (Continuation)

- [x] 6.1 **RED** Add CLI/runtime tests proving `engram cloud serve` is discoverable and starts a cloud runtime path without mutating local-first defaults.
- [x] 6.2 **GREEN** Implement `engram cloud serve` runtime wiring through `cloudserver + cloudstore + auth` with actionable startup failures.
- [x] 6.3 **RED** Add cloud server handler tests for `/health`, `/sync/push`, `/sync/pull`, `/sync/pull/{chunkID}`, auth rejection, and `/dashboard` mount.
- [x] 6.4 **GREEN** Implement practical cloudserver HTTP handlers + dashboard mount and Postgres-backed cloudstore persistence for chunk manifest/chunk reads.
- [x] 6.5 **REFACTOR** Add local Docker assets/docs (`docker-compose.cloud.yml`, `docker/cloud/Dockerfile`, README/DOCS alignment) and run targeted Docker smoke against Postgres-backed cloud runtime.
- [x] 6.6 **RED/GREEN** Fix compose host reachability by making cloud bind host configurable, wire compose-safe host config, and prove host->container cloud endpoint access via targeted Docker smoke.
