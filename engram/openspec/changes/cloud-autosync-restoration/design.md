# Design: cloud-autosync-restoration

## Context

- Proposal: `openspec/changes/cloud-autosync-restoration/proposal.md` (Engram `sdd/cloud-autosync-restoration/proposal`)
- Exploration: `openspec/changes/cloud-autosync-restoration/exploration.md` (Engram `sdd/cloud-autosync-restoration/explore`)
- Spec: deferred (will land alongside tasks; not required for design)
- Copy source: `/Users/alanbuscaglia/work/engram-cloud/internal/cloud/autosync/manager.go` (451 LOC) + `internal/cloud/remote/transport.go` mutation section (lines 360-453)

## Technical Approach

Layered port. Batch 1 unblocks everything by shipping the server mutation endpoints + transport methods. Batch 2 replaces the 73-line stub manager with the legacy 451-line manager, ADAPTED (not verbatim) to carry forward `StopForUpgrade`/`ResumeAfterUpgrade` and a `recover()` wrapper. Batch 3 wires a project-scoped status adapter that composes over `storeSyncStatusProvider`. Batch 4 deletes the `fatal()` in `cmdServe`/`cmdMCP` and replaces it with `tryStartAutosync` using the existing `newCloudAutosyncManager` test seam. Batch 5 ships the round-trip integration test and docs.

No schema changes. No new store methods. The store already exposes everything needed (`AcquireSyncLease`, `ListPendingSyncMutations`, `SkipAckNonEnrolledMutations`, `ApplyPulledMutation`, `MarkSyncHealthy`, `MarkSyncFailure`).

## Architecture Decisions

### AD-1: Manager port mechanics — adapt in place

**Choice**: Copy the legacy manager struct, Config, phases, Run loop, cycle, push, pull, backoff, and state-tracking verbatim. ADAPT by (a) replacing the import path, (b) extending `Status` with `ReasonCode`/`ReasonMessage` fields already on the stub, (c) adding `StopForUpgrade(project)`/`ResumeAfterUpgrade(project)` methods that manipulate `m.status` under `m.mu`, (d) wrapping `Run` body in a `defer recover()` that sets `PhaseBackoff` + `reason_code=internal_error`. StopForUpgrade drains the Run goroutine via a dedicated `stopCh chan struct{}`; ResumeAfterUpgrade restarts a cycle via `NotifyDirty()`.

**Alternatives**: (a) byte-identical copy + separate upgrade gate type (extra indirection, breaks existing `autosyncManagerAdapter` shape); (b) rewrite from scratch (high regression risk).

**Rationale**: Keeps the proven core untouched while honoring the integrated upgrade-path contract. `StopForUpgrade` body is small (< 30 LOC) and must live INSIDE the Manager to share `m.mu`.

### AD-2: Transport HTTP contract

**Choice**: Follow legacy wire format exactly. Add `NewMutationTransport(baseURL, token)` (no project) returning a new `*MutationTransport` type sharing the same HTTP client + auth helper as `RemoteTransport`. Keep `NewRemoteTransport(baseURL, token, project)` untouched for chunk sync. Extract `setAuthorization` + `validateBaseURL` + `newHTTPStatusError` into package-level helpers both types can share.

| Call | Method | Path | Body / Query | Success | Error mapping |
|---|---|---|---|---|---|
| PushMutations | POST | `/sync/mutations/push` | `{"mutations":[{entity,entity_key,op,payload}]}` | 200 `{accepted,last_seq}` | 401→`HTTPStatusError.IsAuthFailure`, 403→`IsPolicyFailure`, 404→`server_unsupported` via ErrorCode, 409→surface verbatim, 5xx→retry (3x, 500ms/1s/2s + ±25% jitter), 429→retry after `Retry-After` |
| PullMutations | GET | `/sync/mutations/pull?since_seq=N&limit=M` | query only | 200 `{mutations:[],has_more}` | same as push |

Timeouts: 30s per request (matches existing `RemoteTransport`). No refresh-token path — ENGRAM_CLOUD_TOKEN is static.

**Alternatives**: (a) monolithic `RemoteTransport` with project optional — pollutes chunk-sync invariants; (b) new transport package — over-engineered for < 100 LOC.

**Rationale**: Separate constructor makes "which transport for what" explicit. Retry policy matches legacy (engram-cloud line 340 `sleepWithJitter`); 404 → `PhaseBackoff` with `reason_code=server_unsupported` fulfills proposal AD-8 (loud auto-disable).

### AD-3: cloudserver mutation endpoints

**Choice**: Register `POST /sync/mutations/push` and `GET /sync/mutations/pull` in `cloudserver.go`'s `routes()` (next to `/sync/push`). Both go through `s.withAuth` (token gate). Admin-gate is NOT applied — mutation sync is a first-class user operation. Enrollment gate: handlers call `s.projectAuth.AuthorizeProject(mutation.Project)` per entry on push; pull reads the caller's allowed-project list from the `ProjectAuthorizer` and filters server-side. Body limit: reuse `maxPushBodyBytes = 8MiB` for push; response cap 100 mutations per pull.

Handler skeletons live in a new file `internal/cloud/cloudserver/mutations.go` (isolates the feature + new cloudstore queries). Cloudstore gains: `InsertMutationBatch(ctx, project, entries)` (append to existing mutation journal table) and `ListMutationsSince(ctx, allowedProjects, sinceSeq, limit)` (paginated, filtered by the caller's enrolled projects). Response when project not enrolled on push: HTTP 403 + `{"error_code":"blocked_unenrolled"}`. On pull without any enrolled projects: HTTP 200 + empty mutations, `has_more:false`.

**Alternatives**: (a) in-line in cloudserver.go — bloats the file (already 900+ LOC when dashboard expanded); (b) gate admin-only — breaks non-admin users; (c) pass project in query — duplicates payload data.

**Rationale**: New file keeps the route registration diff small and matches the legacy layout (engram-cloud has a separate `cloudserver/mutations.go`). Enrollment check is single-source-of-truth via the existing `ProjectAuthorizer`.

### AD-4: Status adapter

**Choice**: New file `cmd/engram/autosync_status.go` defining:

```go
type autosyncStatusAdapter struct {
    mgr      cloudAutosyncManager // nil when autosync disabled
    fallback server.SyncStatusProvider
}
func (a autosyncStatusAdapter) Status(project string) server.SyncStatus { ... }
```

When `mgr != nil`: returns `mapPhaseToServerStatus(mgr.Status(), project)` and OVERLAYS the upgrade-stage fields by calling `a.fallback.Status(project)` (pulls `UpgradeStage`/`UpgradeReasonCode`/`UpgradeReasonMessage` from the store). When `mgr == nil`: delegates entirely to fallback (preserves current integrated behavior). Phase mapping: `PhaseHealthy`→`healthy`, `PhasePushing`/`PhasePulling`→`running`, `PhasePushFailed`/`PhasePullFailed`/`PhaseBackoff`→`degraded` + `reason_code=transport_failed`, `PhaseDisabled` (from StopForUpgrade)→`paused` + `reason_code=paused`, `PhaseIdle`→`idle`.

**Alternatives**: (a) shove into existing `storeSyncStatusProvider` — tight coupling; (b) bolt onto `autosyncManagerAdapter` — adapter would need to carry store + cfg, violates SRP.

**Rationale**: Adapter composes — no double-writes, upgrade stage always visible, autosync phase overrides store lifecycle when live. New file is < 100 LOC and unit-testable with fakes.

### AD-5: Manager construction & lifecycle

**Choice**: Add `tryStartAutosync(s *store.Store, cfg store.Config) (cloudAutosyncManager, context.CancelFunc)` in `cmd/engram/main.go` right above `cmdServe`. Body:

1. If `!envBool("ENGRAM_CLOUD_AUTOSYNC")` → return nil, nil.
2. `cc, err := resolveCloudRuntimeConfig(cfg)`. If cc.ServerURL or cc.Token empty → log warning, return nil, nil.
3. `rt, err := remote.NewMutationTransport(cc.ServerURL, cc.Token)`. On error → log, return nil, nil.
4. `mgr := newCloudAutosyncManager(s, rt)` (existing seam, drops the `nil` arg; adapter is updated to pass rt through).
5. `ctx, cancel := context.WithCancel(context.Background())`; `go mgr.Run(ctx)`; return mgr, cancel.

`cmdServe` and `cmdMCP` both call `tryStartAutosync`. Lifecycle ownership: caller holds `cancel`; SIGINT/SIGTERM handler calls `cancel()` BEFORE `exitFunc(0)` so the Run goroutine releases the lease (legacy pattern in manager.go line 177 `defer m.releaseLease()`). `s.Close()` runs after cancel via the existing `defer`.

**Alternatives**: (a) manager self-manages context — breaks test seams; (b) wire cancel into a broader shutdown registry — out of scope.

**Rationale**: Smallest diff to existing `newCloudAutosyncManager` seam; cleanly ordered shutdown; testable via the same env toggle.

### AD-6: Env configuration precedence

**Choice**: Precedence (checked in `tryStartAutosync` in order, fail-fast with log at each gate):

1. `ENGRAM_CLOUD_AUTOSYNC` — must be truthy (`1|true|yes|on`), else return nil silently.
2. `ENGRAM_CLOUD_SERVER` — from env OR `cloud.json` (via `resolveCloudRuntimeConfig`). Empty → log `autosync: cloud server not configured`, return nil.
3. `ENGRAM_CLOUD_TOKEN` — env only (the config ignores persisted tokens per existing `resolveCloudRuntimeConfig` at line 370). Empty → log `autosync: cloud token missing`, return nil.
4. `validateCloudServerURL(cc.ServerURL)` — invalid URL → log, return nil.

Never fatal — autosync is opt-in, startup must never block local serve.

**Alternatives**: (a) auto-enable when token+server set (rejected in proposal AD-7); (b) single AUTOSYNC env var carries URL+token — breaks current config surface.

**Rationale**: Explicit opt-in + ordered fail-fast gives predictable boot behavior. Matches proposal AD-7.

### AD-7: Goroutine safety

**Choice**: Keep legacy `sync.RWMutex` on `Manager`. All `Status`/`setPhase`/`recordFailure`/`recordSuccess` already mutex-guarded. `NotifyDirty` uses buffered-1 channel (non-blocking). `StopForUpgrade`/`ResumeAfterUpgrade` take `m.mu.Lock()` to mutate `status` fields and signal a separate `stopCh`. Run goroutine wraps body in `defer func() { if r := recover(); r != nil { m.setReason("internal_error", fmt.Sprint(r)); m.setPhase(PhaseBackoff); log.Printf(...) } }()`. Context cancellation returns cleanly via the existing `case <-ctx.Done()`.

**Alternatives**: atomic.Value for status (micro-opt, adds cast boilerplate).

**Rationale**: Proven pattern in legacy; recover() addresses proposal risk #6.

### AD-8: Tests layout

| Package | Test file | Needs Postgres? | Approach |
|---|---|---|---|
| `internal/cloud/autosync` | `manager_test.go` (rewrite) | No | Fake `LocalStore` + fake `CloudTransport`; cover phase transitions, backoff math, NotifyDirty coalescing, lease miss, StopForUpgrade/ResumeAfterUpgrade, recover on panic |
| `internal/cloud/remote` | `transport_test.go` (extend) | No | `httptest.Server`; assert request shape (method, path, body, Authorization), retry on 5xx/429, error mapping for 401/403/404 |
| `internal/cloud/cloudserver` | `cloudserver_test.go` + new `mutations_test.go` | Yes (existing pattern uses real Postgres via `cloudstoretest`) | Push accepts enrolled project, rejects unenrolled with 403, pull returns only caller's enrolled projects |
| `internal/cloud/cloudstore` | `cloudstore_test.go` (extend) | Yes | `InsertMutationBatch` + `ListMutationsSince` happy path, pagination, project filter |
| `cmd/engram` | `main_extra_test.go` (invert) | No | `TestCmdServeAutosyncLifecycleGating`: asserts autosync STARTS when env set; add healthy-start + graceful-shutdown subtests |
| `cmd/engram` | `autosync_status_test.go` (new) | No | Adapter mapping with fake manager + fake fallback |

Integration round-trip (Batch 5): new `cmd/engram/autosync_e2e_test.go` spins up `httptest.Server` with real `cloudserver.Handler()`, writes locally, asserts mutation arrives on server within one cycle.

**Rationale**: Reuses existing test infrastructure; postgres only where it already is. Unit layer covers 80% of logic without DB.

### AD-9: Docs alignment

| File | Section | Change |
|---|---|---|
| `DOCS.md` | new `## Cloud Autosync` | How to enable (env vars), what it does, `reason_code` table, troubleshooting (`server_unsupported`) |
| `README.md` | Cloud section | One-line: "Set `ENGRAM_CLOUD_AUTOSYNC=1` to enable background sync" |
| `docs/AGENT-SETUP.md` | Cloud sync subsection | Add autosync toggle; note server must deploy endpoints first |
| `docs/ARCHITECTURE.md` | Cloud subsystem | Add autosync box to the diagram; callout about lease coordination |

### AD-10: Batch ordering (confirmation)

Dependencies (→ means "must land before"):

```
Batch 1 (server endpoints + transport) ──→ Batch 2 (manager port) ──→ Batch 3 (status adapter) ──→ Batch 4 (cmd wiring)
                                                                                                        │
                                                                                                        ▼
                                                                                                  Batch 5 (e2e + docs)
```

Batch 1 first because without server endpoints + transport methods there is nothing to test Batch 2 against. Batch 3 depends on the real Manager type from Batch 2 (need real phase constants). Batch 4 depends on status adapter to wire `SetSyncStatus`. Batch 5 is the only batch that exercises all four prior batches end-to-end.

Every batch independently revertable per proposal rollback plan.

## Data Flow

```
  Local write (server.go handler)
        │
        │ s.notifyWrite()
        ▼
  Manager.NotifyDirty() ── buffered chan (1) ──┐
                                               │
                                   debounce(500ms)
                                               │
                                               ▼
                                    Manager.cycle(ctx)
                                               │
        ┌──────────────────────────────────────┼────────────────────────────────────────┐
        ▼                                      ▼                                        ▼
  AcquireSyncLease                     push: ListPending ──→ MutationTransport.Push ──→ cloudserver POST /sync/mutations/push
  (skip if taken)                                                                         │
                                                                                          ▼
                                                                                   cloudstore.InsertMutationBatch (enrollment-gated)
                                                                                          │
  (other client running                                                                   │
   the same Manager)                                                                      │
        │                                                                                 │
        │ poll tick (30s) OR NotifyDirty                                                  │
        ▼                                                                                 │
  Manager.cycle → pull: GetSyncState ──→ MutationTransport.Pull ──→ GET /sync/mutations/pull ◀┘
                                                                    │
                                                                    ▼
                                                                  cloudstore.ListMutationsSince (filtered by caller's enrolled projects)
                                                                    │
                                                                    ▼
                                                            store.ApplyPulledMutation (local dedup via seq)
                                                                    │
                                                                    ▼
                                                            MarkSyncHealthy → storeSyncStatusProvider
                                                                    │
                                                                    ▼
                                              autosyncStatusAdapter.Status(project) → /sync/status → dashboard pill
```

## File Changes

| File | Action | Description |
|---|---|---|
| `internal/cloud/autosync/manager.go` | COPIED_VERBATIM + ADAPTED | Replace 73-line stub with ported 451-line manager; adapt imports; add `StopForUpgrade`/`ResumeAfterUpgrade`; add `recover()` on Run |
| `internal/cloud/autosync/manager_test.go` | MODIFIED | Full phase/backoff/lifecycle/NotifyDirty/StopForUpgrade suite |
| `internal/cloud/remote/transport.go` | MODIFIED | Implement `PushMutations`/`PullMutations`; extract shared helpers; add `MutationTransport` + `NewMutationTransport` |
| `internal/cloud/remote/transport_test.go` | MODIFIED | Mutation request/response + auth + retry + error mapping tests |
| `internal/cloud/cloudserver/mutations.go` | NEW | `handleMutationPush`, `handleMutationPull` + route registration helper |
| `internal/cloud/cloudserver/cloudserver.go` | MODIFIED | Register new routes in `routes()` alongside `/sync/push` |
| `internal/cloud/cloudserver/cloudserver_test.go` | MODIFIED | Add mutation endpoint coverage + policy gate tests |
| `internal/cloud/cloudserver/mutations_test.go` | NEW | Isolated handler tests with fake store + auth |
| `internal/cloud/cloudstore/cloudstore.go` | MODIFIED | `InsertMutationBatch` + `ListMutationsSince` (journal queries) |
| `internal/cloud/cloudstore/cloudstore_test.go` | MODIFIED | New query coverage |
| `cmd/engram/main.go` | MODIFIED | Delete fatal; add `tryStartAutosync`; update `newCloudAutosyncManager` seam signature to accept transport |
| `cmd/engram/autosync_status.go` | NEW | `autosyncStatusAdapter` + phase→server.SyncStatus mapping |
| `cmd/engram/autosync_status_test.go` | NEW | Adapter unit tests |
| `cmd/engram/main_extra_test.go` | MODIFIED | Invert `TestCmdServeAutosyncLifecycleGating`; add healthy-start + shutdown subtests |
| `cmd/engram/autosync_e2e_test.go` | NEW | End-to-end round-trip via `httptest.Server` (Batch 5) |
| `DOCS.md` | MODIFIED | `## Cloud Autosync` section |
| `README.md` | MODIFIED | Cloud one-liner |
| `docs/AGENT-SETUP.md` | MODIFIED | Autosync toggle |
| `docs/ARCHITECTURE.md` | MODIFIED | Autosync box + lease note |

## Interfaces / Contracts

```go
// transport.go — new constructor
func NewMutationTransport(baseURL, token string) (*MutationTransport, error)

type MutationTransport struct { /* same shape as RemoteTransport sans project */ }
func (mt *MutationTransport) PushMutations(mutations []MutationEntry) (*PushMutationsResult, error)
func (mt *MutationTransport) PullMutations(sinceSeq int64, limit int) (*PullMutationsResponse, error)

// autosync/manager.go — ported + extended
func (m *Manager) StopForUpgrade(project string) error
func (m *Manager) ResumeAfterUpgrade(project string) error

// cmd/engram/autosync_status.go
type autosyncStatusAdapter struct {
    mgr      cloudAutosyncManager
    fallback server.SyncStatusProvider
}
func (a autosyncStatusAdapter) Status(project string) server.SyncStatus

// cmd/engram/main.go
func tryStartAutosync(s *store.Store, cfg store.Config) (cloudAutosyncManager, context.CancelFunc)
```

HTTP wire format:
```json
// POST /sync/mutations/push
{"mutations":[{"entity":"observation","entity_key":"...","op":"upsert","payload":{...},"project":"engram"}]}
// → 200 {"accepted":3,"last_seq":42}

// GET /sync/mutations/pull?since_seq=41&limit=100
// → 200 {"mutations":[{"seq":42,"entity":"observation","entity_key":"...","op":"upsert","payload":{...},"occurred_at":"..."}],"has_more":false}
```

## Testing Strategy

| REQ | Test file | Layer |
|---|---|---|
| REQ autosync runs when env set | `cmd/engram/main_extra_test.go` | CLI |
| REQ graceful shutdown | `cmd/engram/main_extra_test.go` | CLI |
| REQ push round-trip | `cmd/engram/autosync_e2e_test.go` + `cloudserver/mutations_test.go` | Integration |
| REQ pull round-trip | `cmd/engram/autosync_e2e_test.go` | Integration |
| REQ enrollment gate (push 403) | `cloudserver/mutations_test.go` | Handler |
| REQ enrollment gate (pull filter) | `cloudserver/mutations_test.go` | Handler |
| REQ phase transitions | `autosync/manager_test.go` | Unit |
| REQ backoff math | `autosync/manager_test.go` | Unit |
| REQ NotifyDirty debounce | `autosync/manager_test.go` | Unit |
| REQ StopForUpgrade/Resume | `autosync/manager_test.go` | Unit |
| REQ transport retry (5xx) | `remote/transport_test.go` | Unit |
| REQ transport error mapping | `remote/transport_test.go` | Unit |
| REQ status adapter mapping | `cmd/engram/autosync_status_test.go` | Unit |
| REQ local writes unblocked on cloud-down | `cmd/engram/autosync_e2e_test.go` | Integration |

Strict TDD: every GREEN step requires a preceding RED in the same batch. No implementation without a failing test.

## Rollout Plan

Batch 1 — server + transport (RED-first per AD-10):
- RED: push/pull handler tests + transport tests
- GREEN: `cloudstore.InsertMutationBatch`, `ListMutationsSince`, `handleMutationPush`, `handleMutationPull`, `MutationTransport`
- REFACTOR: extract shared transport helpers

Batch 2 — manager port:
- RED: phase transitions, backoff, NotifyDirty, StopForUpgrade tests against the (still-stub) manager
- GREEN: replace stub with ported 451-line manager + adaptations
- REFACTOR: verify `autosyncManagerAdapter` still compiles

Batch 3 — status adapter:
- RED: `autosync_status_test.go` with fake manager + fake fallback
- GREEN: implement adapter
- REFACTOR: ensure `storeSyncStatusProvider` untouched

Batch 4 — cmd wiring:
- RED: invert `TestCmdServeAutosyncLifecycleGating` to expect success; add new assertions
- GREEN: delete fatal, add `tryStartAutosync`, wire `SetOnWrite` + `SetSyncStatus`
- REFACTOR: align MCP path

Batch 5 — e2e + docs:
- RED: `autosync_e2e_test.go` round-trip
- GREEN: any missing wiring (likely none)
- REFACTOR + docs update

Deployment note: server endpoints MUST be deployed to `engram.condetuti.com` BEFORE any client sets `ENGRAM_CLOUD_AUTOSYNC=1`. If not, transport returns 404 → manager enters `PhaseBackoff` with `reason_code=server_unsupported` per AD-8 of the proposal.

## Open Questions

None — all 10 implementation decisions resolved.

## References

- Proposal: Engram `sdd/cloud-autosync-restoration/proposal` + `openspec/changes/cloud-autosync-restoration/proposal.md`
- Exploration: Engram `sdd/cloud-autosync-restoration/explore` + `openspec/changes/cloud-autosync-restoration/exploration.md`
- Legacy manager: `/Users/alanbuscaglia/work/engram-cloud/internal/cloud/autosync/manager.go`
- Legacy transport mutation section: `/Users/alanbuscaglia/work/engram-cloud/internal/cloud/remote/transport.go:360-453`
- Legacy mutation routes: `/Users/alanbuscaglia/work/engram-cloud/internal/cloud/cloudserver/cloudserver.go:119-122`
- Integrated stub manager: `/Users/alanbuscaglia/work/engram/internal/cloud/autosync/manager.go`
- Integrated stub transport: `/Users/alanbuscaglia/work/engram/internal/cloud/remote/transport.go:252-280`
- Integrated fatal: `/Users/alanbuscaglia/work/engram/cmd/engram/main.go:553-556`
- Integrated cloudserver routes: `/Users/alanbuscaglia/work/engram/internal/cloud/cloudserver/cloudserver.go:197-199`
- Server SyncStatusProvider: `/Users/alanbuscaglia/work/engram/internal/server/server.go:26-45`
- Related archived change: `cloud-upgrade-path-existing-users` (source of StopForUpgrade/ResumeAfterUpgrade contract)
