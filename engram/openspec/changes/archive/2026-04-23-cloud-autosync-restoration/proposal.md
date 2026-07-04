# Proposal: Restore cloud autosync (silent background push+pull)

## Intent

Integrated `feat/integrate-engram-cloud` deliberately stubbed autosync — `ENGRAM_CLOUD_AUTOSYNC=1` fatals, `PushMutations`/`PullMutations` return "not available", and `cloudserver` has no `/sync/mutations/*` routes. Users must run explicit sync commands and teammate memories never flow back automatically. Restore the 451-line legacy manager from `engram-cloud`, adapt it to token auth + project-scoped enrollment + the dashboard status contract, and ship the missing cloudserver endpoints so local ↔ cloud sync runs silently in the background.

## Scope

### In Scope
- Port `internal/cloud/autosync/manager.go` from engram-cloud (phases, lease, debounce, backoff, push+pull loop).
- Implement `PushMutations`/`PullMutations` in `internal/cloud/remote/transport.go` with token auth.
- Add `POST /sync/mutations/push` and `GET /sync/mutations/pull` to `internal/cloud/cloudserver/cloudserver.go` with enrollment + pause + admin gates.
- Replace the `fatal()` in `cmd/engram/main.go` with a `tryStartAutosync(s, cfg)` wiring serve + MCP.
- Bridge `autosync.Manager.Status()` → `server.SyncStatusProvider.Status(project)` via adapter (reuses existing `reason_code`/`reason_message`).
- Carry `StopForUpgrade`/`ResumeAfterUpgrade` forward as first-class Manager APIs (introduced by the integrated upgrade-path change).
- Invert `TestCmdServeAutosyncLifecycleGating`; add manager + transport + endpoint + status tests.
- Update `DOCS.md`, `README.md`, `docs/AGENT-SETUP.md` for the restored opt-in toggle and rollout order.

### Out of Scope
- Dashboard UX/styling changes (owned by `cloud-dashboard-visual-parity`).
- Upgrade-path semantics (already archived in `cloud-upgrade-path-existing-users`).
- Replacing bearer tokens with a new identity model.
- Cloudstore schema changes unrelated to mutation endpoints.

## Capabilities

### New Capabilities
- `cloud-autosync`: background manager that silently pushes enrolled-project mutations and pulls teammate mutations on a debounce+poll loop, with lease coordination, backoff, and status surfacing.
- `cloud-mutation-endpoints`: authenticated REST mutation push/pull routes on the cloud server, enforcing enrollment + pause + admin policy.

### Modified Capabilities
- `cloud-sync-status`: autosync phase becomes the authoritative source for lifecycle when the manager is running; `storeSyncStatusProvider` stays as fallback when disabled.

## Approach

Layered port with thin adaptations — avoid reinvention.

1. **Batch 1 (server-first, unblocks everything)**: add cloudserver mutation endpoints + transport impls with RED tests. Endpoints must exist before clients can work end-to-end.
2. **Batch 2**: port `manager.go` verbatim, update imports to integrated store interface, add `StopForUpgrade`/`ResumeAfterUpgrade`, add `recover()` wrapper on the Run goroutine.
3. **Batch 3**: status adapter bridging `Manager.Status()` to project-scoped `SyncStatusProvider.Status(project)` — autosync phase overrides, store fallback when disabled.
4. **Batch 4**: wire `tryStartAutosync` in `cmdServe` and `cmdMCP`, delete fatal, invert gating tests.
5. **Batch 5**: end-to-end round-trip test + docs alignment.

## Architectural Decisions

| # | Decision | Resolution | Rationale |
|---|---|---|---|
| 1 | Mutation endpoint REST contract | `POST /sync/mutations/push` body `{target_key, entries: [{seq, project, sync_id, kind, payload, ...}]}` returns `{accepted_seqs: []}`. `GET /sync/mutations/pull?since_seq=N&limit=M` returns `{mutations: [...], has_more: bool, latest_seq: N}`. Auth: `Authorization: Bearer <token>`. No `project` query param — server filters by caller's enrolled projects. | Matches legacy engram-cloud wire format so transport port is verbatim; server-side enrollment filter is the single source of truth. |
| 2 | Transport constructor | Add `NewMutationTransport(baseURL, token)` (no project). Keep `NewRemoteTransport(baseURL, token, project)` unchanged for chunk sync. | Autosync is global (mutations carry their own project in payload); forcing empty-project through the existing constructor would dilute its contract. Separate constructor is explicit. |
| 3 | Enrollment scoping | Push: only enrolled projects (via existing `ListPendingSyncMutations` JOIN + `SkipAckNonEnrolledMutations`). Pull: server filters by caller's enrolled projects; client trusts server. | Store already enforces enrollment; keeping it there avoids duplicated logic. Server-side pull filter prevents leaking cross-tenant mutations. |
| 4 | Backoff strategy | Match legacy exactly: base 1s, max 5min, exponential × 2, ±25% jitter, ceiling 10 consecutive failures → `PhaseBackoff`. | Proven in production; diverging invites regressions. |
| 5 | `StopForUpgrade`/`ResumeAfterUpgrade` | Promote to first-class Manager methods. `StopForUpgrade` sets `PhaseDisabled` + drains Run goroutine without releasing lease. `ResumeAfterUpgrade` re-enters `PhaseIdle` and re-arms the cycle. | Integrated upgrade-path change relies on these; must survive the port or we break `cloud-upgrade-path-existing-users`. |
| 6 | Status integration | `autosyncStatusAdapter.Status(project)` returns autosync phase+reason for every project; falls back to `storeSyncStatusProvider` when manager is nil. Reuse `reason_code`/`reason_message` contract. `PhaseHealthy`→healthy, `PhaseBackoff`→degraded+`transport_failed`, `PhasePushFailed`/`PhasePullFailed`→degraded+last error, `PhaseDisabled`→upgrade_paused. | Single status pipeline into dashboard, no new UI work. |
| 7 | Auto-enable vs opt-in | Keep explicit opt-in: `ENGRAM_CLOUD_AUTOSYNC=1` required even when token+server are set. | Safer rollout; prevents surprise sync on existing setups after upgrade. Revisit for opt-out in a follow-up once stable. |
| 8 | Deployment ordering | Server endpoints MUST deploy to `engram.condetuti.com` BEFORE clients enable autosync. Document this in `DOCS.md` rollout section and add a startup log warning when `ENGRAM_CLOUD_AUTOSYNC=1` but first push returns 404 (→ `PhaseBackoff` with reason `server_unsupported`). | Client auto-disables loudly instead of failing silently, per engram-business-rules. |

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/cloud/autosync/manager.go` | Modified | Replace 73-line stub with ported 451-line manager. |
| `internal/cloud/autosync/manager_test.go` | Modified | Full phase/backoff/lifecycle/NotifyDirty/StopForUpgrade suite. |
| `internal/cloud/remote/transport.go` | Modified | Real `PushMutations`/`PullMutations`; add `NewMutationTransport`. |
| `internal/cloud/remote/transport_test.go` | Modified | Mutation request/response + auth header tests. |
| `internal/cloud/cloudserver/cloudserver.go` | Modified | Register `/sync/mutations/push` + `/sync/mutations/pull` with enrollment/pause/admin gates. |
| `internal/cloud/cloudserver/cloudserver_test.go` | Modified | Endpoint contract + policy gate tests. |
| `cmd/engram/main.go` | Modified | Delete fatal; add `tryStartAutosync` + `autosyncStatusAdapter`. |
| `cmd/engram/main_extra_test.go` | Modified | Invert `TestCmdServeAutosyncLifecycleGating`; add healthy-start + shutdown tests. |
| `DOCS.md`, `README.md`, `docs/AGENT-SETUP.md` | Modified | Restored autosync section + rollout prerequisites. |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Server endpoints not deployed before clients enable | High | Batch 1 ships server first with tests; startup logs + `PhaseBackoff` reason `server_unsupported` on 404; docs call out order. |
| Transport constructor confusion (which to use when) | Med | Separate `NewMutationTransport` + godoc on both constructors + test coverage per constructor. |
| Status adapter race against `storeSyncStatusProvider` | Med | Adapter owns composition; when manager non-nil it wins, when nil store fallback — single resolver, no double-write. |
| Goroutine panic takes down server | Low | `recover()` wrapper on Run; panic sets `PhaseBackoff` with reason `internal_error` and logs. |
| Env semantic flip (`ENGRAM_CLOUD_AUTOSYNC=1` was fatal, now opt-in) | Med | Inverted tests explicitly assert new semantics; CHANGELOG/DOCS call out the shift. |
| Pull leaks cross-tenant mutations | High | Server-side enrollment filter in pull handler; integration test asserts caller only receives their enrolled projects' mutations. |
| Lease starvation in multi-process deploys | Low | 60s lease interval matches legacy; lease-miss is logged at debug, not error. |
| Local writes blocked when cloud unreachable | High | Autosync runs in its own goroutine; never holds locks across network I/O; integration test asserts local write latency unaffected during transport 500s. |

## Rollback Plan

Revert the batch commits in reverse order: (5) docs, (4) cmdServe wiring → restore fatal, (3) status adapter, (2) manager port → restore stub, (1) server endpoints + transport. Each batch is independently revertable because earlier batches add server surface that is backwards compatible (new routes, no schema changes). If only client misbehaves, flip `ENGRAM_CLOUD_AUTOSYNC` off — server endpoints stay deployed harmlessly.

## Dependencies

- Legacy source: `/Users/alanbuscaglia/work/engram-cloud/internal/cloud/autosync/manager.go` + `transport.go`.
- Integrated store surface already done: `AcquireSyncLease`, `MarkSyncHealthy`, `MarkSyncFailed`, `ListPendingSyncMutations`, `SkipAckNonEnrolledMutations`, `ApplyPulledMutation`.
- `cloud-upgrade-path-existing-users` (archived): provides `StopForUpgrade`/`ResumeAfterUpgrade` contract the ported manager must honor.
- Server-side deploy to `engram.condetuti.com` before widespread client enablement.

## Success Criteria

- [ ] `ENGRAM_CLOUD_AUTOSYNC=1` + token + server → background loop starts in serve and MCP; no fatal.
- [ ] Local write triggers push within 500ms debounce; mutation appears on cloud within one cycle.
- [ ] Teammate mutation on cloud appears locally within 30s poll (or immediately after next dirty trigger).
- [ ] Only enrolled-project mutations are pushed; unenrolled are skip-acked (observable in store).
- [ ] Cloud server rejects pull of non-enrolled projects (integration test).
- [ ] Push/pull failures drive `PhasePushFailed`/`PhasePullFailed` with correct `reason_code`; 10 consecutive failures → `PhaseBackoff`.
- [ ] Dashboard status pill reflects autosync phase with no dashboard code changes.
- [ ] `StopForUpgrade`/`ResumeAfterUpgrade` still function as expected for the upgrade-path flow.
- [ ] Local write latency unaffected when cloud is unreachable (integration assertion).
- [ ] All new endpoints + manager transitions + transport requests have RED-first tests (strict TDD).
