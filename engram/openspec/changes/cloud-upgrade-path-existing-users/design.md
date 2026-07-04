# Design: Cloud upgrade path for existing users

## Technical Approach

Add an additive `engram cloud upgrade` command family on top of current `cloud config`, `cloud enroll`, and `sync --cloud`. Local SQLite stays authoritative: upgrade state, rollback snapshots, repair findings, and bootstrap checkpoints are persisted in `internal/store`; `cmd/engram` stays a thin CLI adapter; `internal/sync` owns first-bootstrap orchestration; cloud packages only validate, transport, and surface status.

## Architecture Decisions

| Decision | Choice | Alternatives | Rationale |
|---|---|---|---|
| Command shape | `engram cloud upgrade {doctor|repair|bootstrap|status|rollback}` with explicit subcommand invocation (`--project` required per operation). No implicit shorthand chain is implemented. | Overload `cloud enroll` / `sync --cloud` | Keeps rollout additive, avoids hidden side effects, and preserves existing low-level commands. |
| Upgrade state model | New store-owned per-project upgrade record with stage/checkpoints/snapshot JSON | Overload `sync_state` only | `sync_state` tracks steady-state transport health, not rollback snapshots or repair history. |
| Bootstrap engine | Reuse `store.EnrollProject` backfill + `sync.Syncer` push/import/status, wrapped by resumable checkpoint stages | Separate migration-only sync path | Keeps one sync engine and inherits current idempotency (`AckSyncMutationSeqs`, `ApplyPulledChunk`, chunk dedupe). |
| Repair boundaries | Only deterministic local repairs auto-apply; auth/policy/ambiguous data stay blocked/manual | Silent auto-fix of all failures | Preserves local-first trust and keeps failures loud/testable. |

## Data Flow

```text
cloud upgrade doctor/repair/bootstrap
cmd/engram/cloud.go
  -> internal/store UpgradePlanner/Repair APIs
  -> internal/sync BootstrapProject(project, server)
       -> snapshot pre-upgrade config+enrollment
       -> EnrollProject backfill (local journal seed)
       -> remote transport push
       -> pull/status verify
  -> sync_state + upgrade_state updated
  -> CLI / dashboard / /sync/status read same reason + stage
```

State model per project:
- `planned` -> `doctor_ready|doctor_blocked`
- `repair_applied` (optional)
- `bootstrap_enrolled`
- `bootstrap_pushed`
- `bootstrap_verified`
- `rolled_back`

Checkpoint rule: each stage writes after success; retries resume from the last durable stage and skip completed work.

## File Changes

| File | Action | Description |
|---|---|---|
| `cmd/engram/cloud.go` | Modify | Add upgrade subcommand parsing/help and status/rollback output. |
| `cmd/engram/main.go` | Modify | Reuse cloud preflight/status helpers for upgrade-aware reason parity. |
| `internal/store/store.go` | Modify | Add upgrade-state persistence, rollback snapshot storage, doctor/repair queries, and schema migration. |
| `internal/sync/sync.go` | Modify | Add resumable bootstrap orchestration that reuses existing export/import/status logic. |
| `internal/cloud/remote/transport.go` | Modify | Preserve typed HTTP failures and expose bootstrap-safe error classification. |
| `internal/cloud/cloudserver/cloudserver.go` | Modify | Return machine-actionable validation/policy errors for doctor/bootstrap. |
| `internal/cloud/dashboard/dashboard.go` | Modify | Show upgrade stage + deterministic reason, not only generic degraded state. |
| `README.md`, `DOCS.md`, `docs/AGENT-SETUP.md`, `docs/PLUGINS.md` | Modify | Document validated upgrade workflow and deferred plugin automation. |

## Interfaces / Contracts

```go
type CloudUpgradeState struct {
    Project string
    Stage string // planned, doctor_ready, doctor_blocked, repair_applied, bootstrap_enrolled, bootstrap_pushed, bootstrap_verified, rolled_back
    RepairClass string // none, repairable, blocked, policy
    SnapshotJSON string // prior cloud.json presence + enrolled bool
    LastErrorCode string
    LastErrorMessage string
}
```

Repair classes:
- `repairable`: missing journal rows, soft-delete tombstones, deterministic project attribution, canonical project normalization.
- `blocked`: missing non-inferable required fields, unresolved session dependency, cross-project ambiguity.
- `policy`: `auth_required`, `policy_forbidden`, `cloud_config_error`, remote reachability/auth blockers.

No remote mutation is allowed in `doctor` or `repair`.

## Testing Strategy

| Layer | What to Test | Approach |
|---|---|---|
| Unit | CLI help/flag validation and rollback boundary | Extend `cmd/engram/*_test.go`. |
| Unit | Store doctor/repair classification + schema migration | Add focused `internal/store/store_test.go` cases. |
| Integration | Bootstrap resume/idempotency | `internal/sync/sync_test.go` covering retry after enroll/push/verify boundaries. |
| Integration | Push/pull + status parity | Cloudserver/dashboard/main tests assert same reason/stage across surfaces. |

## Migration / Rollout

Schema: add a new table (store-owned) for per-project upgrade checkpoints/snapshots; keep existing `sync_state`, `sync_mutations`, and `sync_enrolled_projects` unchanged. Rollout is additive: existing manual commands remain supported; upgrade is the recommended path for existing users only.

Rollback boundary: rollback is local-only and allowed until `bootstrap_verified`. It restores the saved cloud config/enrollment snapshot, clears upgrade checkpoints, and stops further autosync attempts. It does **not** rewrite local memory data and does **not** delete remote chunks. After `bootstrap_verified`, rollback is blocked and users must use explicit disconnect/unenroll flows.

## Open Questions

- [ ] Should `/sync/status` include a nested upgrade payload, or should dashboard/CLI read upgrade state directly from store-only wiring?
