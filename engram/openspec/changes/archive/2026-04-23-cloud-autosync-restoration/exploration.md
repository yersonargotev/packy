# Exploration: cloud-autosync-restoration

## Problem

Integrated `feat/integrate-engram-cloud` deliberately stubbed out the autosync manager. Users work locally but their memories do NOT sync to the cloud automatically, and teammate memories do NOT flow back automatically. The user wants the original engram-cloud experience: silent background sync of local ↔ cloud.

## File-level gap

| File | Integrated (stub) | Legacy engram-cloud |
|---|---|---|
| `internal/cloud/autosync/manager.go` | 73 lines (no-op stub) | 451 lines (real: phases, backoff, lease, debounce, push+pull loop) |
| `internal/cloud/autosync/manager_test.go` | 54 lines (stop/resume only) | Full lifecycle coverage |
| `cmd/engram/main.go` | `fatal("cloud autosync is not available")` at ~line 554 | `tryStartAutosync` wiring |
| `cmd/engram/main_extra_test.go` | Asserts autosync IS REJECTED | Asserts autosync runs |
| `internal/cloud/remote/transport.go` | `PushMutations`/`PullMutations` are STUB errors | Full HTTP implementation |
| `internal/cloud/cloudserver/cloudserver.go` | **Missing `/sync/mutations/push` + `/sync/mutations/pull` routes** | Routes exist |

## Critical discovery

**The cloud server is missing mutation endpoints.** `cloudserver.go` has:
- `GET /sync/pull` (chunk manifest)
- `GET /sync/pull/{chunkID}` (chunk download)
- `POST /sync/push` (chunk upload)

But NO:
- `POST /sync/mutations/push`
- `GET /sync/mutations/pull`

Without these, the ported autosync manager cannot communicate with the cloud server end-to-end. This is the highest priority blocker.

## Store infrastructure (already complete)

The local SQLite store already has:
- `AcquireSyncLease`, `MarkSyncHealthy`, `MarkSyncFailed` — lease-based coordination
- Mutation journal with dedup/tombstones
- Project-scoped enrollment (enrolled projects only)

## Approaches

| Approach | Pros | Cons |
|---|---|---|
| **A: Direct port** | Proven code, minimal invention | Transport project-scope coupling, server endpoint gap |
| **B: Thin polling wrapper** | Lower risk | Doesn't bring teammate memories; ignores mutation journal |
| **C: Staged TDD batches** | Each batch testable; de-risks server gap first | More planning overhead |

**Recommended: C (staged)**.

## Direct port list (verbatim from `/Users/alanbuscaglia/work/engram-cloud/internal/cloud/autosync/manager.go`)

- `manager.go` logic: Manager type, Config, DefaultConfig, phase enum (`PhaseHealthy`, `PhasePushFailed`, `PhasePullFailed`, `PhaseBackoff`, `PhaseDisabled`), Run loop goroutine, NotifyDirty semantics, backoff strategy, status reporting.

## Adapt list (surgical changes)

- `internal/cloud/remote/transport.go` — implement `PushMutations(ctx, batch)` → `POST /sync/mutations/push`; `PullMutations(ctx, sinceSeq, limit)` → `GET /sync/mutations/pull?since_seq=N&limit=M`. Add `NewMutationTransport(baseURL, token)` constructor WITHOUT project requirement (autosync is global, not project-scoped).
- `internal/cloud/cloudserver/cloudserver.go` — add `/sync/mutations/push` + `/sync/mutations/pull` routes + handlers. Handlers must respect project enrollment + sync pause + admin gates.
- `cmd/engram/main.go` — delete fatal block; add `tryStartAutosync(s, cfg)` function matching legacy pattern; add `autosyncStatusAdapter` bridging `autosync.Manager.Status()` into `server.SyncStatusProvider.Status(project string)` interface.
- `cmd/engram/main_extra_test.go` — invert `TestCmdServeAutosyncLifecycleGating` (was: asserts reject; now: asserts runs when enabled).

## New code list

- `internal/cloud/autosync/manager_test.go` — full phase/backoff/lifecycle unit tests using the real manager.
- Mutation endpoint handlers (push, pull) in cloudserver with proper auth + enrollment + pause gates.
- Possibly new cloudstore queries for serving mutations (may already exist via ApplyPulledMutation path).

## Auth bridge adaptation

Legacy used JWT credential flow. Integrated uses `ENGRAM_CLOUD_TOKEN` bearer. The autosync transport must:
- Read token from env/config via `resolveCloudRuntimeConfig`
- Set `Authorization: Bearer <token>` on push/pull requests
- Handle 401/403 as `PhasePushFailed`/`PhasePullFailed`

## Runtime wiring

- Env toggle: `ENGRAM_CLOUD_AUTOSYNC=1` enables the loop (currently fatals).
- Auto-enable when `ENGRAM_CLOUD_TOKEN` + `ENGRAM_CLOUD_SERVER` both set.
- Status wiring: autosync phase → `/sync/status` endpoint → dashboard status pill (reuses existing reason_code/reason_message contract from `integrate-engram-cloud`).

## Project-scoped sync

Integrated has project enrollment. Autosync must:
- Push only mutations from enrolled projects
- Pull mutations for enrolled projects
- Ignore global mutations (or include them per legacy semantics — verify with existing sync.Manager)

## Dashboard integration

Autosync phase maps to dashboard status pill:
- `PhaseHealthy` → "Cloud Active" (already renders)
- `PhasePushFailed` / `PhasePullFailed` / `PhaseBackoff` → "Degraded" with reason

No new dashboard code needed — the existing reason_code/reason_message pipeline works.

## Tests needed

- Unit: Manager state transitions (phases, backoff, NotifyDirty, StopForUpgrade, ResumeAfterUpgrade)
- Integration: push+pull round trip via autosync with real chunk store
- CLI: autosync starts with env, stops cleanly on shutdown
- Status: `/sync/status` reflects autosync phase correctly

## Risks

1. **Cloud server mutation endpoints missing** — highest priority blocker
2. **Transport `NewRemoteTransport` project-scope coupling** — blocks autosync reuse
3. **`server.SyncStatusProvider.Status(project string)` interface mismatch** with legacy `Status()` — adapter required
4. **`StopForUpgrade`/`ResumeAfterUpgrade`** exist only in stub, not in engram-cloud manager — must ADD to ported manager (they were introduced post-stub for the upgrade-path change)
5. **Env var semantic flip** — `ENGRAM_CLOUD_AUTOSYNC=1` goes from "fatal error" to "enable" — tests must be updated
6. **Goroutine leaks** if Manager panic — add recover() wrapper
7. **Local-first invariant** — autosync must never block local writes if cloud is unreachable
8. **Pull conflicts** — dedup semantics preserved via content addressing; verify no loss on concurrent writes
9. **Deployment** — `engram.condetuti.com` needs the new mutation endpoint server before autosync clients can work

## Recommended implementation sequence (TDD RED-first)

1. **Batch 1**: cloudserver mutation endpoints + transport mutation methods + RED tests (server returns 200 on valid push, pull returns mutations since sequence)
2. **Batch 2**: Port autosync/manager.go from engram-cloud + adapt imports + add StopForUpgrade/ResumeAfterUpgrade + RED tests for phases
3. **Batch 3**: Status adapter bridging Manager.Status → server.SyncStatusProvider.Status(project) + RED tests
4. **Batch 4**: cmd/engram/main.go wiring — remove fatal, add tryStartAutosync, invert tests
5. **Batch 5**: End-to-end integration test + docs (DOCS.md autosync section, README updates)
