# Tasks: Cloud Autosync Restoration

Status: ALL 19 TASKS COMPLETE — 5 BATCHES, STRICT TDD

---

## Batch 1 — Cloudserver mutation endpoints + Transport mutation methods

**REQs satisfied**: REQ-200, REQ-201, REQ-202, REQ-203, REQ-214

### 1.1 RED — Cloudserver mutation endpoint tests [start here]
- [x] 1.1 Create `internal/cloud/cloudserver/mutations_test.go` with failing tests (15 tests — all pass)

### 1.2 RED — Transport mutation method tests [parallel with 1.1]
- [x] 1.2 Add failing tests to `internal/cloud/remote/transport_mutations_test.go` (7 tests — all pass)

### 1.3 GREEN — cloudstore mutation queries
- [x] 1.3 Add InsertMutationBatch + ListMutationsSince to cloudstore.go + cloud_mutations migration

### 1.4 GREEN — `internal/cloud/cloudserver/mutations.go`
- [x] 1.4 Create mutations.go with handleMutationPush + handleMutationPull + MutationStore interface; routes registered

### 1.5 GREEN — `NewMutationTransport` + push/pull methods
- [x] 1.5 Add MutationTransport to transport.go; 404→server_unsupported; Project field added to MutationEntry

---

## Batch 2 — Port Autosync Manager

**REQs satisfied**: REQ-204, REQ-205, REQ-206, REQ-207, REQ-208, REQ-212, REQ-213

### 2.1 RED — Manager phase + lifecycle tests
- [x] 2.1 Create manager_test.go with 22 tests (phases/backoff/NotifyDirty/StopForUpgrade/panic/goroutine)

### 2.2 GREEN — Replace stub
- [x] 2.2 Full 370+ LOC manager: PhaseDisabled, StopForUpgrade, ResumeAfterUpgrade, recover(), Stop()/WaitGroup

---

## Batch 3 — Status adapter

**REQs satisfied**: REQ-209, REQ-cloud-sync-status

### 3.1 RED — Status adapter tests
- [x] 3.1 Create autosync_status_test.go with 8 tests

### 3.2 GREEN — autosync_status.go
- [x] 3.2 Create autosyncStatusAdapter with phase mapping and upgrade-stage overlay

---

## Batch 4 — cmd/engram wiring + lifecycle test inversion

**REQs satisfied**: REQ-210, REQ-211, REQ-212

### 4.1 RED/4.2 GREEN (atomic)
- [x] 4.1 Invert TestCmdServeAutosyncLifecycleGating; add 6 new gating tests
- [x] 4.2 Remove fatal, add tryStartAutosync + mutationTransportAdapter, wire autosyncStatusAdapter

---

## Batch 5 — End-to-end integration test + docs

**REQs satisfied**: REQ-212 (goroutine isolation), all REQs verified end-to-end

### 5.1 RED — E2E round-trip test
- [x] 5.1 Create autosync_e2e_test.go: TestAutosyncPushPullRoundTrip + TestLocalWriteDuringTransport500 + TestGoroutineIsolationConcurrentWrites

### 5.2 GREEN — Docs updates [parallel with 5.1]
- [x] 5.2 Update DOCS.md: Cloud Autosync section (env vars, phase table, reason_code table, troubleshooting)
- [x] 5.3 Update README.md: one-line pointer to DOCS.md
- [x] 5.4 Update docs/ARCHITECTURE.md: autosync data flow diagram + mutation endpoint table
- [x] 5.5 Update docs/AGENT-SETUP.md: autosync toggle + server deploy prerequisite

### 5.3 Validation gate
- [x] 5.6 go test ./... — ALL PASS
- [x] 5.7 go test -race ./... — CLEAN
- [x] 5.8 go vet ./... — CLEAN
- [x] 5.9 gofmt -l ./... — CLEAN

---

## TDD Cycle Evidence Table

| Task | Test File | REQs Covered | RED Committed | GREEN Passing | Refactor Done |
|------|-----------|--------------|---------------|---------------|---------------|
| 1.1 | `internal/cloud/cloudserver/mutations_test.go` | REQ-200, REQ-201, REQ-202, REQ-203 | [x] | [x] | [x] |
| 1.2 | `internal/cloud/remote/transport_mutations_test.go` | REQ-200, REQ-201, REQ-214 | [x] | [x] | [x] |
| 1.3 | `internal/cloud/cloudstore/cloudstore.go` | REQ-200, REQ-201 | [x] | [x] | [x] |
| 2.1 | `internal/cloud/autosync/manager_test.go` | REQ-204–REQ-208, REQ-212, REQ-213 | [x] | [x] | [x] |
| 3.1 | `cmd/engram/autosync_status_test.go` | REQ-209, REQ-cloud-sync-status | [x] | [x] | [x] |
| 4.1 | `cmd/engram/main_extra_test.go` | REQ-210, REQ-211 | [x] | [x] | [x] |
| 5.1 | `cmd/engram/autosync_e2e_test.go` | REQ-212, all REQs E2E | [x] | [x] | [x] |

ALL TASKS COMPLETE — Status: DONE
