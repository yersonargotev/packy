## Exploration: cloud-upgrade-path-existing-users

### Current State
Cloud sync is already strict and deterministic, but migration UX for existing users is still manual (`cloud config` + `cloud enroll` + `sync --cloud`). The codebase already auto-repairs some legacy states at store startup (`repairEnrolledProjectSyncMutations`) and during migration backfills (`sync_mutations.project`, generated `sync_id`, legacy observation table fixes). However, cloud push validation now enforces a strong contract (required fields, session dependency integrity, canonical chunk hashing/project ownership), so historical local data/journal inconsistencies can block first cloud bootstrap.

Observed failure classes relevant to existing-user upgrades:
1. **Missing historical mutation journal for already-enrolled projects** — legacy rows exist locally but not in `sync_mutations` (auto-repaired today for enrolled projects).
2. **Soft-delete history gaps** — deleted observations/prompts exist without matching delete mutation records (partially auto-repaired today).
3. **Project attribution drift** — older rows with empty/mismatched `project` in payload/journal/session relationships can cause wrong project scoping or skipped export.
4. **Mutation payload contract violations** — legacy/hand-edited payloads missing required fields (`session_id`, `scope`, `directory`, etc.) now fail canonicalization/validation.
5. **Dependency gaps** — observations/prompts/mutations referencing unknown sessions are rejected by cloud server validation.
6. **Cross-project normalization drift** — project naming variants (`Engram` vs `engram`) can fragment enrollment/sync state and produce partial bootstrap.
7. **Preflight policy/config blockers** — auth, allowlist, server URL, unenrolled state (`blocked_unenrolled`, `auth_required`, `policy_forbidden`, `cloud_config_error`) are deterministic blockers that must stay loud.

### Affected Areas
- `cmd/engram/cloud.go` — add guided upgrade command surface (`doctor/repair/migrate/bootstrap/status/rollback`) instead of ad-hoc manual sequencing.
- `cmd/engram/main.go` (`cmdSync`, preflight/state transitions) — wire upgrade-aware bootstrap and deterministic reason propagation.
- `internal/store/store.go` — extend migration/repair APIs and persist upgrade lifecycle/rollback snapshots.
- `internal/sync/sync.go` — add bootstrap orchestration semantics (first sync idempotency, journal/snapshot hybrid behavior) while preserving existing local sync.
- `internal/cloud/cloudserver/cloudserver.go` — keep strict contract, but surface machine-actionable validation errors for repair UX.
- `internal/cloud/remote/transport.go` — propagate typed migration/bootstrap failure classes from HTTP responses.
- `internal/cloud/dashboard/dashboard.go` + `internal/server/server.go` — expose upgrade phase/reason parity beyond generic degraded state.
- `README.md`, `DOCS.md`, `docs/AGENT-SETUP.md`, `docs/PLUGINS.md` — replace manual-only cloud migration guidance with supported upgrade path.
- Tests: `cmd/engram/main_extra_test.go`, `cmd/engram/*_test.go`, `internal/store/store_test.go`, `internal/sync/sync_test.go`, `internal/cloud/cloudserver/cloudserver_test.go`.

### Approaches
1. **Strict block-first migration (no auto-repair)** — run diagnostics, block on any inconsistency, require user to repair manually.
   - Pros: Simple correctness model; low implicit mutation risk.
   - Cons: Poor UX for existing users; high support burden; violates request for systematic migration path.
   - Effort: **Medium**

2. **Auto-repair-first migration** — silently normalize/repair everything possible during first cloud bootstrap.
   - Pros: Minimal user friction; faster onboarding.
   - Cons: Risky if repairs are ambiguous (could mutate semantics unexpectedly); hard to explain/rollback.
   - Effort: **Medium/High**

3. **Guided hybrid migration (recommended)** — deterministic `doctor -> repair -> bootstrap -> verify` flow with explicit blocking for ambiguous or policy-related failures.
   - Pros: Balances safety + UX; leverages existing repair machinery; keeps failures loud and actionable.
   - Cons: More CLI/status surface area; requires upgrade-state persistence and docs/test expansion.
   - Effort: **High**

### Recommendation
Use **Approach 3 (Guided hybrid migration)**.

#### Automatic vs user-action policy
- **Auto-repair (safe/deterministic):**
  - Backfill missing journal rows for enrolled project historical data.
  - Backfill `project` attribution from payload/session when deterministic.
  - Backfill missing delete mutations for soft-deleted records.
  - Normalize project naming and enroll canonical name.
  - Rebuild first bootstrap mutation set from current local snapshot when complete fields are available.
- **Block + require user action:**
  - Missing required semantic fields that cannot be inferred safely.
  - Entity references that cannot be resolved (e.g., orphaned observation session with no recoverable source).
  - Cross-project conflicts where auto-merge would be lossy/ambiguous.
  - Auth/policy/config failures (`auth_required`, `policy_forbidden`, `cloud_config_error`).

#### UX / CLI contract
Recommended command family under `engram cloud upgrade`:
- `engram cloud upgrade doctor --project <p>`
  - Read-only diagnostics; outputs categorized findings (`repairable`, `blocked`, `policy`, `ready`) + deterministic codes.
- `engram cloud upgrade repair --project <p> [--apply|--dry-run]`
  - Applies only deterministic repair set; emits changelog and idempotent summary.
- `engram cloud upgrade bootstrap --project <p> [--resume]`
  - Performs first-cloud bootstrap with checkpointing (config/enroll snapshot, push, verify pull/status).
- `engram cloud upgrade status --project <p>`
  - Shows phase and last deterministic reason across CLI/server/dashboard.
- `engram cloud upgrade rollback --project <p>`
  - Allowed only before bootstrap completion (first successful cloud commit boundary).

`engram sync --cloud --project <p>` remains the steady-state transport command after successful bootstrap.

#### Bootstrap data model choice
Use **hybrid bootstrap**:
- Seed from **snapshot-derived mutation backfill** for historical local data (handles users with incomplete old journal history).
- Continue with **mutation journal** for incremental sync after bootstrap.
- Include dependency closure sessions to satisfy strict cloud validation.

Why not snapshot-only: loses mutation/tombstone semantics and future incremental guarantees.
Why not journal-only: fails for legacy users with incomplete or stale mutation history.

### Risks
- **Silent semantic drift risk** if repair mutates ambiguous legacy data; mitigate with strict “repairable vs blocked” classification and dry-run.
- **Duplicate/bootstrap replay risk** if checkpointing/idempotency is weak; mitigate with persisted upgrade state and resume tokens.
- **Rollback safety risk** after remote writes; enforce rollback boundary before bootstrap completion.
- **Backwards-compat regression risk** in existing `engram sync --cloud` flows; preserve current command behavior and add upgrade flow as additive.
- **Docs drift** between CLI behavior and setup/plugin guidance; update docs in same change.
- **Boundary leakage** if upgrade logic bypasses store/sync contracts; keep adapters thin and core behavior in `internal/store` + `internal/sync`.

### Ready for Proposal
**Yes** — proceed to `sdd-propose` with an explicit migration contract that defines: (1) deterministic diagnostic taxonomy, (2) safe auto-repair set, (3) block conditions requiring user action, (4) hybrid bootstrap checkpoints, (5) rollback boundary and compatibility guarantees.
