# Proposal: Integrate Engram Cloud

## Intent

Bring the proven cloud subsystem into `engram` without changing the product story: local SQLite remains primary, cloud is optional replication/shared access. This change creates the integration seam now and defers script validation until the cloud path is stable.

## Scope

### In Scope
- Import `internal/cloud/{auth,cloudstore,cloudserver,remote,autosync,dashboard}` into this repo with normalized module paths.
- Wire explicit `engram cloud ...` entrypoints and cloud config in `cmd/engram/main.go` while preserving existing local defaults.
- Enable autosync only for configured cloud-capable long-lived processes; expose loud failure states through status surfaces.
- Update docs for new commands, config, and local-first constraints in the same change.

### Out of Scope
- Rewriting launch/session/plugin scripts.
- Changing default behavior of existing local commands when cloud is not configured.
- Full UX hardening for every cloud policy edge case beyond deterministic failure surfacing.

## Capabilities

### New Capabilities
- `cloud-integration`: optional cloud auth, transport, server, dashboard, and autosync wiring that extends local-first sync without replacing it.

### Modified Capabilities
- None.

## Approach

Use selective additive integration. Import cloud packages first, then expose explicit cloud commands, then wire background autosync behind config gates. Keep `serve`, `mcp`, `search`, `context`, and local sync flows unchanged unless the user opts into cloud. Defer script validation to a follow-up change once runtime behavior is real and testable.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `cmd/engram/main.go` | Modified | Add cloud command/config wiring without changing local defaults. |
| `internal/cloud/**` | New | Add cloud auth, storage, server, remote, autosync, dashboard packages. |
| `internal/sync/{sync.go,transport.go}` | Modified | Support cloud remote path while preserving current local sync semantics. |
| `internal/store/store.go` | Modified | Keep enrollment/mutation journal as source-of-truth boundary. |
| `internal/server/server.go` | Modified | Surface explicit sync/autosync status and failures. |
| `docs/*`, `README.md`, `DOCS.md` | Modified | Align docs with actual command surface and constraints. |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Local command regression | Med | Add boundary tests in `cmd`, `sync`, `server`; preserve default code paths. |
| Cloud dependency churn | Med | Isolate module imports and review `go.mod`/`go.sum` drift tightly. |
| Silent blocked sync | High | Require explicit status/error propagation for paused/unenrolled cases. |

## Rollback Plan

Revert cloud command wiring in `cmd/engram/main.go`, disable autosync startup, and remove imported `internal/cloud/*` packages if regressions appear. Because local paths remain default and untouched by config, rollback restores current behavior cleanly.

## Dependencies

- Cloud subsystem source from `engram-cloud`
- Dependency additions required by imported cloud packages

## Success Criteria

- [ ] Existing local commands behave the same when cloud is unconfigured.
- [ ] `engram cloud ...` commands initialize and report deterministic failures.
- [ ] Push and pull regression coverage exists for cloud-enabled sync boundaries.
- [ ] Docs describe the new integration and explicitly defer script validation follow-up.
