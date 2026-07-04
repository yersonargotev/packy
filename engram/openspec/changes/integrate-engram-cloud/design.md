# Design: Integrate Engram Cloud

## Technical Approach

Use additive integration on top of the existing local-first core. `internal/store` remains the source of truth for sessions, observations, prompts, enrollment, and sync journal state; imported `internal/cloud/*` packages provide optional auth, remote persistence, autosync orchestration, cloud server APIs, and dashboard UI. `cmd/engram/main.go` gains an explicit `engram cloud ...` tree plus gated startup hooks for cloud-capable daemons, while existing local commands keep their current defaults.

## Architecture Decisions

| Decision | Choice | Alternatives | Rationale |
|---|---|---|---|
| Source of truth | Keep SQLite + `sync_mutations` in `internal/store` authoritative | Make cloud canonical | Matches REQ-CLOUD-01 and existing local-first semantics; cloud only replicates/admin-controls. |
| Sync transport | Reuse `internal/sync.Syncer` with transport abstraction; add cloud transport in `internal/cloud/remote` | Parallel cloud-only sync engine | Preserves local chunk sync behavior and existing tests while adding remote push/pull seam. |
| Policy enforcement | Enrollment and pause/block rules enforced before network mutation; server/cloudstore owns org policy, store owns local enrollment journal | UI-only toggles or CLI-only checks | Prevents silent drops and keeps blocked behavior deterministic/testable. |
| UI placement | Put browser UI in `internal/cloud/dashboard` with cloudserver handlers; keep local JSON API in `internal/server` | Mix dashboard into `internal/server` | Respects package boundaries and project placement rules. |

## Data Flow

```text
local write
CLI/MCP/HTTP -> internal/server -> internal/store
                               -> sync_mutations + sync_state (pending)
                               -> autosync.NotifyDirty()

autosync cycle (cloud-enabled only)
autosync.Manager -> store.AcquireSyncLease/ListPendingSyncMutations
                 -> internal/cloud/remote client -> cloudserver
                 -> store.AckSyncMutationSeqs / MarkSyncHealthy
                 -> pull remote mutations -> store.ApplyPulledMutation
                 -> internal/server / dashboard / CLI status surfaces
```

Control rules:
- Unconfigured cloud: no cloud worker, no behavior change.
- Unenrolled project: reject push/pull before remote I/O; mark blocked reason in sync status.
- Paused/auth/network failures: persist degraded state in `sync_state`; expose same reason across CLI, `/sync/status`, and dashboard.

## File Changes

| File | Action | Description |
|---|---|---|
| `cmd/engram/main.go` | Modify | Add `cloud` subcommands, config/env parsing, daemon startup wiring, and cloud-aware sync/status entrypoints. |
| `internal/store/store.go` | Modify | Reuse enrollment/sync journal APIs; add any missing status reason helpers needed by autosync/cloud wiring. |
| `internal/sync/sync.go` | Modify | Keep local chunk sync; allow remote-backed export/import/status through transport-based construction. |
| `internal/sync/transport.go` | Modify | Preserve file transport and document remote transport contract used by cloud client. |
| `internal/server/server.go` | Modify | Extend `/sync/status` payload for cloud reason codes/messages without changing local handler ownership. |
| `internal/cloud/auth/*` | Add | Imported auth/session helpers for cloud login/token lifecycle. |
| `internal/cloud/cloudstore/*` | Add | Cloud persistence and org-policy access layer. |
| `internal/cloud/cloudserver/*` | Add | Cloud HTTP routes and server-enforced admin controls. |
| `internal/cloud/remote/*` | Add | `sync.Transport` implementation and client calls for push/pull/status. |
| `internal/cloud/autosync/*` | Add | Background manager, dirty notifications, retry/backoff, and status model. |
| `internal/cloud/dashboard/*.templ` | Add | Server-rendered dashboard pages/partials for cloud status/admin flows. |
| `README.md`, `DOCS.md`, `docs/*` | Modify | Document commands, env/config keys, opt-in enrollment, and deferred script validation. |

## Interfaces / Contracts

```go
// cmd -> cloud wiring
type CloudRuntime struct {
    Enabled bool
    Target  string // default: store.DefaultSyncTargetKey ("cloud")
}

// dashboard/server/cli shared status shape
type CloudSyncStatus struct {
    Phase      string
    ReasonCode string // blocked_unenrolled, auth_required, paused, transport_failed
    Message    string
}
```

`internal/cloud/remote` should implement `internal/sync.Transport`; autosync should depend on store interfaces (`ListPendingSyncMutations`, `AckSyncMutationSeqs`, `ApplyPulledMutation`, `MarkSyncFailure`, `MarkSyncHealthy`) instead of reaching into handlers.

## Testing Strategy

| Layer | What to Test | Approach |
|---|---|---|
| Unit | Cloud CLI parsing/help/errors | Extend `cmd/engram/*_test.go` with explicit `engram cloud ...` cases and unconfigured no-op regression cases. |
| Unit | Remote transport and autosync state transitions | Focused tests in `internal/cloud/remote` and `internal/cloud/autosync` for auth failure, blocked enrollment, retry/backoff. |
| Integration | Push and pull boundaries | Store + autosync + remote fake: verify enrolled projects sync, unenrolled projects fail before network, pulled mutations stay idempotent. |
| Integration | Server/dashboard status parity | `internal/server` and `internal/cloud/dashboard` tests assert same reason code/message appears on all status surfaces. |

## Migration / Rollout

Phased merge:
1. Import `internal/cloud/*` with module/dependency normalization only.
2. Add explicit `engram cloud ...` commands and help text.
3. Wire remote transport for opt-in cloud sync paths.
4. Start autosync only in cloud-enabled long-lived processes.
5. Add dashboard/admin flows and docs alignment.

Rollback boundaries match those phases: revert CLI wiring first, then autosync startup, then remote transport usage, while leaving local commands and store journal intact.

## Open Questions

- [ ] Which exact long-lived processes are cloud-capable in v1: `serve` only, or `mcp` as well when explicitly enabled?
- [ ] Should blocked/paused reasons be represented as new `sync_state.lifecycle` values or as a separate reason-code field layered on top of existing lifecycle?

## Explicit Non-Goals (Deferred Script Touch Points)

- Plugin/install script updates are deferred: `docs/AGENT-SETUP.md` and `docs/PLUGINS.md` will document current boundaries before we wire automatic cloud setup flows.
- Release/packaging scripts are deferred until CLI contract stabilizes (`engram cloud` surface + config semantics).
- External automation hooks (Claude/OpenCode/Gemini/Codex cloud bootstrap) remain out-of-scope for this apply pass to avoid mixed rollout concerns.
