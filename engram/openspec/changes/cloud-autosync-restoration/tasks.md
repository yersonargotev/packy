# Tasks: Cloud Autosync Restoration

> Strict TDD active. Every RED task must be committed before its paired GREEN task.
> Batches 1–4 are sequential (each depends on the prior). Within a batch, task groups marked [parallel] may run concurrently.

---

## Batch 1 — Cloudserver mutation endpoints + Transport mutation methods

**REQs satisfied**: REQ-200, REQ-201, REQ-202, REQ-203, REQ-214

### 1.1 RED — Cloudserver mutation endpoint tests [start here]
- [x] 1.1 Create `internal/cloud/cloudserver/mutations_test.go` with failing tests:
  - `TestMutationPushEndpointAccepted` (REQ-200 happy path)
  - `TestMutationPushEndpointUnauth` (REQ-200 missing token → 401)
  - `TestMutationPushEndpointBatchTooLarge` (REQ-200 101 entries → 400)
  - `TestMutationPushEndpointEmptyBatch` (REQ-200 empty entries → 200)
  - `TestMutationPullEndpointSinceSeq` (REQ-201 since_seq=5 returns seqs 6–10)
  - `TestMutationPullEndpointHasMore` (REQ-201 150 mutations, limit=100 → has_more=true)
  - `TestMutationPullEndpointUnauth` (REQ-201 missing token → 401)
  - `TestMutationPullEndpointBeyondLatest` (REQ-201 since_seq beyond latest → empty)
  - `TestMutationPullEnrollmentFilter` (REQ-202 caller only sees enrolled projects)
  - `TestMutationPullCrossTenantLeak` (REQ-202 two callers, no cross-leak)
  - `TestMutationPullNoEnrollments` (REQ-202 no enrolled → empty 200)
  - `TestMutationPushSyncPaused409` (REQ-203 sync_enabled=false → 409)
  - `TestMutationPushNonPausedAccepted` (REQ-203 non-paused → 200)
  - `TestMutationPushPausePerProject` (REQ-203 alpha paused, beta active)
  - `TestMutationPushPauseAdminStillBlocked` (REQ-203 admin token still gets 409)

### 1.2 RED — Transport mutation method tests [parallel with 1.1]
- [x] 1.2 Add failing tests to `internal/cloud/remote/transport_test.go`:
  - `TestMutationTransportPushAccepted` (REQ-200: valid push returns accepted_seqs)
  - `TestMutationTransportPushUnauth` (REQ-200: 401 → HTTPStatusError.IsAuthFailure)
  - `TestMutationTransportPullSinceSeq` (REQ-201: pull returns mutations + has_more + latest_seq)
  - `TestMutationTransportPullUnauth` (REQ-201: 401 → error)
  - `TestMutationTransportPush404ServerUnsupported` (REQ-214: 404 → reason_code=server_unsupported)
  - `TestMutationTransportPull404ServerUnsupported` (REQ-214: 404 → reason_code=server_unsupported)
  - `TestMutationTransportPush401VsNotFound` (REQ-214: 401 → auth_required, not server_unsupported)
  - All tests use `httptest.NewServer` — no Postgres

### 1.3 GREEN — cloudstore mutation queries
- [x] 1.3 Add to `internal/cloud/cloudstore/cloudstore.go` (or new file `dashboard_queries.go`):
  - `InsertMutationBatch(ctx, batch []MutationEntry) (acceptedSeqs []int64, err error)`
  - `ListMutationsSince(ctx, sinceSeq int64, limit int, allowedProjects []string) (items []MutationEntry, hasMore bool, latestSeq int64, err error)`
  - Add corresponding tests to `internal/cloud/cloudstore/cloudstore_test.go`

### 1.4 GREEN — `internal/cloud/cloudserver/mutations.go`
- [x] 1.4 Create `internal/cloud/cloudserver/mutations.go` with:
  - `handleMutationPush(w, r)` — bearer auth, body limit 8 MiB, batch size cap 100, enrollment gate per entry, pause gate (409 on sync_enabled=false), calls `InsertMutationBatch`
  - `handleMutationPull(w, r)` — bearer auth, since_seq/limit params, server-side enrollment filter, calls `ListMutationsSince`
  - Register both routes in `cloudserver.go` `routes()` via `s.withAuth`

### 1.5 GREEN — `NewMutationTransport` + push/pull methods
- [x] 1.5 Add to `internal/cloud/remote/transport.go`:
  - `MutationTransport` struct with `baseURL`, `token`, `httpClient`
  - `NewMutationTransport(baseURL, token string) *MutationTransport`
  - `PushMutations(ctx, batch []MutationEntry) (acceptedSeqs []int64, err error)` — POST /sync/mutations/push, 30s timeout, 3x retry on 5xx/429, 404 → HTTPStatusError with reason_code=server_unsupported
  - `PullMutations(ctx, sinceSeq int64, limit int) (items []MutationEntry, hasMore bool, latestSeq int64, err error)` — GET /sync/mutations/pull?since_seq=N&limit=M, same retry policy
  - 401 returns `IsAuthFailure=true`, 404 returns `ErrorCode="server_unsupported"`

---

## Batch 2 — Port Autosync Manager

**REQs satisfied**: REQ-204, REQ-205, REQ-206, REQ-207, REQ-208, REQ-212, REQ-213

### 2.1 RED — Manager phase + lifecycle tests
- [x] 2.1 Create/replace `internal/cloud/autosync/manager_test.go` with failing tests:
  - `TestManagerPhaseTransitions` (REQ-204: idle → healthy on success)
  - `TestManagerPushFailedPhase` (REQ-204: push error → PhasePushFailed)
  - `TestManagerPullFailedPhase` (REQ-204: pull error → PhasePullFailed)
  - `TestManagerStopForUpgradeDisabled` (REQ-204/208: StopForUpgrade → PhaseDisabled)
  - `TestManagerBackoffExponentialGrowth` (REQ-205: durations ~2x each, ≤300s)
  - `TestManagerBackoffJitterBounds` (REQ-205: base 4s → actual in [3s, 5s])
  - `TestManagerBackoffCeiling` (REQ-205: 10 failures → PhaseBackoff, cycle skipped)
  - `TestManagerBackoffResetOnSuccess` (REQ-205: success resets ConsecutiveFailures=0)
  - `TestManagerNotifyDirtyOneCycle` (REQ-206: one NotifyDirty → one cycle after 500ms)
  - `TestManagerNotifyDirtyCoalesce` (REQ-206: 100 calls in 10ms → one cycle)
  - `TestManagerNotifyDirtyDuringBackoff` (REQ-206: non-blocking during Backoff)
  - `TestManagerNotifyDirtyAfterStop` (REQ-206: non-blocking after Stop)
  - `TestManagerRunContextCancel` (REQ-207: cancel → Run returns within 1s)
  - `TestManagerRunPollTicker` (REQ-207: 30s poll triggers cycle)
  - `TestManagerStopWaitsGoroutine` (REQ-207/213: Stop → no goroutines leaked via goleak)
  - `TestManagerRunPanicRecovery` (REQ-207/213: panic in cycle → PhaseBackoff+internal_error, loop continues)
  - `TestManagerStopForUpgradeHaltsCycle` (REQ-208: no cycles after StopForUpgrade)
  - `TestManagerStopForUpgradeRetainsLease` (REQ-208: lease not released until Stop)
  - `TestManagerResumeAfterUpgrade` (REQ-208: Resume → cycle runs after poll)
  - `TestManagerResumeWithoutStop` (REQ-208: Resume on Healthy is no-op)
  - `TestManagerStopBeforeRun` (REQ-213: Stop before Run returns immediately)
  - `TestManagerPanicSetsBackoff` (REQ-213: panic → PhaseBackoff+internal_error)
  - `TestManagerLoopContinuesAfterPanic` (REQ-213: loop retries after panic)
  - All tests use fake `LocalStore` + fake `CloudTransport` — no Postgres

### 2.2 GREEN — Replace `internal/cloud/autosync/manager.go` stub
- [x] 2.2 Copy `engram-cloud` manager (451 LOC) and adapt:
  - Update import paths to `github.com/Gentleman-Programming/engram/...`
  - Extend `Status` struct with `ReasonCode`, `ReasonMessage` (already present in stub, verify)
  - Add `StopForUpgrade(ctx) error` — sets PhaseDisabled, drains pending, retains lease
  - Add `ResumeAfterUpgrade(ctx) error` — sets PhaseIdle, re-arms cycle
  - Wrap `Run` body in `defer recover()` → sets PhaseBackoff + reason_code=internal_error + logs stack
  - Add `NotifyDirty()` with buffered-1 chan (non-blocking)
  - `Stop()` cancels internal ctx + WaitGroup.Wait() (no goroutine leaks)
  - Exponential backoff: base=1s, max=300s, ×2, ±25% jitter, ceiling=10 failures

---

## Batch 3 — Status adapter

**REQs satisfied**: REQ-209, REQ-cloud-sync-status

### 3.1 RED — Status adapter tests
- [x] 3.1 Create `cmd/engram/autosync_status_test.go` with failing tests:
  - `TestSyncStatusAdapterHealthy` (REQ-209: PhaseHealthy → SyncStatus.Phase="healthy")
  - `TestSyncStatusAdapterRunning` (REQ-209: PhasePushing/PhasePulling/PhaseIdle → Phase="running")
  - `TestSyncStatusAdapterBackoff` (REQ-209: PhaseBackoff → Phase="degraded", ReasonCode="transport_failed")
  - `TestSyncStatusAdapterPushFailed` (REQ-209: PhasePushFailed → Phase="degraded")
  - `TestSyncStatusAdapterPullFailed` (REQ-209: PhasePullFailed → Phase="degraded")
  - `TestSyncStatusAdapterDisabled` (REQ-209: PhaseDisabled → ReasonCode="upgrade_paused")
  - `TestSyncStatusAdapterNilFallback` (REQ-209: mgr=nil → delegates to storeSyncStatusProvider)
  - `TestSyncStatusAdapterOverlayUpgradeStage` (REQ-cloud-sync-status: upgrade-stage fields overlaid from fallback)

### 3.2 GREEN — `cmd/engram/autosync_status.go`
- [x] 3.2 Create `cmd/engram/autosync_status.go`:
  - `autosyncStatusAdapter{mgr *autosync.Manager, fallback server.SyncStatusProvider}` struct
  - Implements `server.SyncStatusProvider` interface
  - `Status(project string) server.SyncStatus`:
    - `mgr == nil` → return `fallback.Status(project)` unchanged
    - `mgr != nil` → map phase to `SyncStatus`, overlay upgrade-stage fields from `fallback.Status(project)`
    - Phase map: PhaseHealthy→healthy, PhasePushing/Pulling/Idle→running, PhasePushFailed/PullFailed/Backoff→degraded+transport_failed, PhaseDisabled→paused+reason=paused
    - 404-triggered Backoff uses reason_code=server_unsupported per REQ-214

---

## Batch 4 — cmd/engram wiring + lifecycle test inversion

**REQs satisfied**: REQ-210, REQ-211, REQ-212

### 4.1 RED — Env gating + lifecycle tests
- [x] 4.1 Update/add tests in `cmd/engram/main_extra_test.go`:
  - **INVERT** `TestCmdServeAutosyncLifecycleGating` — was: asserts fatal; now: asserts Manager starts when env set (REQ-210)
  - Add `TestAutosyncEnvAbsent` (REQ-210: no AUTOSYNC env → no Manager started)
  - Add `TestAutosyncEnvNotOne` (REQ-210: AUTOSYNC=true → no Manager, only "1" accepted)
  - Add `TestAutosyncGatingTokenMissing` (REQ-211: AUTOSYNC=1, server set, token empty → log contains ENGRAM_CLOUD_TOKEN, serve continues)
  - Add `TestAutosyncGatingServerMissing` (REQ-211: AUTOSYNC=1, token set, server empty → log contains ENGRAM_CLOUD_SERVER, serve continues)
  - Add `TestAutosyncGatingBothPresent` (REQ-211: all vars set → tryStartAutosync returns non-nil Manager)
  - Add `TestCmdServeStartsWithoutAutosync` (REQ-211: token missing → HTTP server still handles requests)

### 4.2 GREEN — `tryStartAutosync` + wiring in `cmd/engram/main.go`
- [x] 4.2 Modify `cmd/engram/main.go`:
  - Delete `fatal("cloud autosync is not available")` block (~line 554)
  - Add `tryStartAutosync(s *server.Server, cfg cloud.RuntimeConfig) (*autosync.Manager, context.CancelFunc)`:
    - Step 1: check `os.Getenv("ENGRAM_CLOUD_AUTOSYNC") == "1"` — return nil,nil if not
    - Step 2: resolve server URL — log ERROR + return nil,nil if empty
    - Step 3: resolve token — log ERROR + return nil,nil if empty
    - Step 4: `NewMutationTransport(url, token)` → `newCloudAutosyncManager(s, rt)` → `go mgr.Run(ctx)`
    - Step 5: return mgr + cancel (caller wires SIGINT → cancel before exitFunc(0))
  - Update `autosyncManagerAdapter` to bridge to new `autosync.Manager` type
  - Wire SIGINT handler: `cancel()` before `exitFunc(0)`
  - Call `tryStartAutosync` from both `cmdServe` and `cmdMCP`

---

## Batch 5 — End-to-end integration test + docs

**REQs satisfied**: REQ-212 (goroutine isolation), all REQs verified end-to-end

### 5.1 RED — E2E round-trip test
- [x] 5.1 Create `cmd/engram/autosync_e2e_test.go`:
  - `TestAutosyncPushPullRoundTrip`: spin up `httptest.Server` running `cloudserver.Handler()` + local store + Manager, write observation locally, assert it appears in cloud within 600ms (debounce 500ms + margin), assert pulled mutation appears locally within 35s (poll 30s + margin)
  - `TestLocalWriteDuringTransport500` (REQ-212: local write succeeds while cloud returns 500)
  - `TestGoroutineIsolationConcurrentWrites` (REQ-212: 1000 concurrent local writes with cloud offline — no deadlock)

### 5.2 GREEN — Docs updates [parallel with 5.1]
- [x] 5.2 Update `DOCS.md`: add `## Cloud Autosync` section with env vars table (ENGRAM_CLOUD_AUTOSYNC, ENGRAM_CLOUD_SERVER, ENGRAM_CLOUD_TOKEN), phase table (idle/running/healthy/degraded/paused), reason_code table (transport_failed, server_unsupported, upgrade_paused, auth_required), and troubleshooting guide for server_unsupported
- [x] 5.3 Update `README.md`: add one-line pointer to DOCS.md Cloud Autosync section
- [x] 5.4 Update `docs/ARCHITECTURE.md`: add autosync box to architecture diagram, lease note
- [x] 5.5 Update `docs/AGENT-SETUP.md`: add autosync toggle instructions + server deploy prerequisite

### 5.3 Validation gate
- [x] 5.6 Run `go test ./...` — all tests pass
- [x] 5.7 Run `go test -race ./...` — no data races
- [x] 5.8 Run `go vet ./...` — no issues
- [x] 5.9 Run `gofmt -l ./...` — no unformatted files

---

## TDD Cycle Evidence Table

To be filled during `sdd-apply`. One row per test file.

| Task | Test File | REQs Covered | RED Committed | GREEN Passing | Refactor Done |
|------|-----------|--------------|---------------|---------------|---------------|
| 1.1 | `internal/cloud/cloudserver/mutations_test.go` | REQ-200, REQ-201, REQ-202, REQ-203 | [ ] | [ ] | [ ] |
| 1.2 | `internal/cloud/remote/transport_test.go` | REQ-200, REQ-201, REQ-214 | [ ] | [ ] | [ ] |
| 1.3 | `internal/cloud/cloudstore/cloudstore_test.go` | REQ-200, REQ-201 | [ ] | [ ] | [ ] |
| 2.1 | `internal/cloud/autosync/manager_test.go` | REQ-204, REQ-205, REQ-206, REQ-207, REQ-208, REQ-212, REQ-213 | [ ] | [ ] | [ ] |
| 3.1 | `cmd/engram/autosync_status_test.go` | REQ-209, REQ-cloud-sync-status | [ ] | [ ] | [ ] |
| 4.1 | `cmd/engram/main_extra_test.go` | REQ-210, REQ-211 | [ ] | [ ] | [ ] |
| 5.1 | `cmd/engram/autosync_e2e_test.go` | REQ-212, all REQs E2E | [ ] | [ ] | [ ] |

---

## Dependency Graph

```
Batch 1 (server + transport) ─────────────┐
                                           ▼
                              Batch 2 (manager port)
                                           │
                                           ▼
                              Batch 3 (status adapter)
                                           │
                                           ▼
                              Batch 4 (cmd wiring)
                                           │
                                           ▼
                              Batch 5 (e2e + docs)
```

Within Batch 1: tasks 1.1 and 1.2 (RED tests) run in parallel. Tasks 1.3, 1.4, 1.5 (GREEN) are sequential after REDs.
Within Batch 5: tasks 5.1 (e2e test) and 5.2–5.5 (docs) run in parallel.

---

## File Inventory

| File | Action | Batch |
|------|--------|-------|
| `internal/cloud/cloudserver/mutations.go` | CREATE | 1 |
| `internal/cloud/cloudserver/mutations_test.go` | CREATE | 1 |
| `internal/cloud/cloudserver/cloudserver.go` | MODIFY (register routes) | 1 |
| `internal/cloud/cloudstore/cloudstore.go` | MODIFY (InsertMutationBatch, ListMutationsSince) | 1 |
| `internal/cloud/cloudstore/cloudstore_test.go` | MODIFY | 1 |
| `internal/cloud/remote/transport.go` | MODIFY (MutationTransport + methods) | 1 |
| `internal/cloud/remote/transport_test.go` | MODIFY | 1 |
| `internal/cloud/autosync/manager.go` | REPLACE stub with 451-LOC adapted port | 2 |
| `internal/cloud/autosync/manager_test.go` | REPLACE with full suite | 2 |
| `cmd/engram/autosync_status.go` | CREATE | 3 |
| `cmd/engram/autosync_status_test.go` | CREATE | 3 |
| `cmd/engram/main.go` | MODIFY (delete fatal, add tryStartAutosync) | 4 |
| `cmd/engram/main_extra_test.go` | MODIFY (invert + add tests) | 4 |
| `cmd/engram/autosync_e2e_test.go` | CREATE | 5 |
| `DOCS.md` | MODIFY | 5 |
| `README.md` | MODIFY | 5 |
| `docs/ARCHITECTURE.md` | MODIFY | 5 |
| `docs/AGENT-SETUP.md` | MODIFY | 5 |
