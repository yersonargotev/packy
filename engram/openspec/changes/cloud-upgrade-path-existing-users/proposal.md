# Proposal: Cloud upgrade path for existing users

## Intent

Existing users only have low-level steps (`cloud config`, `cloud enroll`, `sync --cloud`) and docs explicitly defer automation. That is insufficient for migration because it lacks guided preflight, first-sync bootstrap, rollback, and a single deterministic workflow for already-populated local stores.

## Scope

### In Scope
- Add a guided CLI-first upgrade flow for existing local users moving one project to Cloud.
- Define bootstrap behavior that uses local enrollment backfill as the source for first remote replication.
- Add safety/rollback status, docs, and boundary tests across store/sync/cloud/dashboard surfaces.

### Out of Scope
- Auto-running cloud onboarding from `engram setup ...` or plugin installers.
- Changing local-first defaults or making cloud mandatory.
- Org-admin policy redesign beyond what the upgrade flow must surface.

## Capabilities

### New Capabilities
- `cloud-upgrade-path`: guided migration of existing local Engram projects into opt-in cloud replication with preflight, bootstrap, and rollback semantics.

### Modified Capabilities
- None.

## Approach

Keep existing low-level commands, but add one recommended flow:
- `engram cloud upgrade --project <name> --server <url>` — preflight, persist config, verify auth/policy, enroll, snapshot rollback metadata, and stage first sync.
- `engram cloud upgrade --status --project <name>` — read-only readiness + bootstrap progress.
- `engram cloud upgrade --rollback --project <name>` — restore pre-upgrade local config/enrollment when bootstrap is incomplete.

Bootstrap SHALL use `EnrollProject` backfill to enqueue historical local mutations, then run explicit first push and verification pull/status. Local SQLite remains authoritative; remote state is derived replication.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `cmd/engram/cloud.go` | Modified | Add guided upgrade/rollback/status entrypoints. |
| `internal/store/store.go` | Modified | Persist upgrade snapshot/rollback metadata around enrollment backfill. |
| `internal/sync/sync.go` | Modified | Support first-sync bootstrap + deterministic upgrade status. |
| `internal/cloud/{remote,autosync,dashboard,cloudserver}` | Modified | Surface upgrade progress/failures consistently. |
| `README.md`, `DOCS.md`, `docs/AGENT-SETUP.md`, `docs/PLUGINS.md` | Modified | Replace “manual only” messaging with the validated upgrade contract. |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Duplicate/partial first sync | Med | Idempotent bootstrap state + push/pull regression tests. |
| Unsafe rollback after remote writes | Med | Rollback only before bootstrap completion; fail loudly afterward. |
| Docs/CLI drift | Med | Same-change docs update validated against real commands. |

## Rollback Plan

If upgrade fails before bootstrap completion, restore saved cloud config/enrollment snapshot, stop autosync, and keep local SQLite unchanged. After first successful bootstrap, rollback becomes a new explicit unenroll/disconnect action, not silent reversal.

## Dependencies

- Existing enrollment backfill behavior in `internal/store`
- Cloud sync/status surfaces already present in `internal/sync` and `internal/cloud/*`

## Success Criteria

- [ ] Existing users can migrate with one recommended CLI workflow instead of manual multi-step docs.
- [ ] First cloud bootstrap is idempotent and preserves historical local data via enrollment backfill.
- [ ] Failed or blocked upgrades expose deterministic reasons across CLI/dashboard/status.
- [ ] Docs describe the validated upgrade path and keep plugin/setup automation explicitly deferred.
