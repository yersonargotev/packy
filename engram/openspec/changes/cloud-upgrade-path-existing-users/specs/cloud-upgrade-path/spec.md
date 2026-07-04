# cloud-upgrade-path Specification

## Requirements

### Requirement: REQ-UPGRADE-01 — Guided Upgrade Workflow

The CLI MUST provide `doctor`, `repair`, `bootstrap`, `status`, and `rollback` under `engram cloud upgrade`. It SHALL require explicit `--project` and SHALL keep local SQLite as source of truth.

#### Scenario: Recommended upgrade path is discoverable
- GIVEN an existing local project with no cloud enrollment
- WHEN the user runs `engram cloud upgrade --help`
- THEN help output lists doctor/repair/bootstrap/status/rollback workflow
- AND guidance states cloud is opt-in replication

#### Scenario: Missing project target fails loudly
- GIVEN a user runs `engram cloud upgrade doctor` without `--project`
- WHEN argument validation runs
- THEN command exits non-zero
- AND stderr includes usage guidance

### Requirement: REQ-UPGRADE-02 — Deterministic Doctor Findings

`doctor` MUST return deterministic readiness findings with stable reason codes for auth, reachability, policy, and project validity. Findings SHALL be identical for unchanged inputs.

#### Scenario: Stable findings for unchanged environment
- GIVEN project state and credentials are unchanged
- WHEN `engram cloud upgrade doctor --project X` is executed twice
- THEN both runs return the same result and reason codes

#### Scenario: Blocked policy is explicit
- GIVEN org policy denies enrollment for the project
- WHEN `doctor` executes
- THEN result is `blocked`
- AND output includes a deterministic policy reason code/message

### Requirement: REQ-UPGRADE-03 — Safe Auto-Repair Boundaries

`repair` MUST only apply predefined safe local fixes. `repair` MUST NOT mutate remote data, MUST NOT force enrollment, and MUST report applied or skipped actions.

#### Scenario: Safe repair applies allowed local fixes
- GIVEN doctor reports a repairable local metadata issue
- WHEN `engram cloud upgrade repair --project X` runs
- THEN allowed local fix is applied and recorded in output
- AND remote state remains unchanged

#### Scenario: Non-repairable issue is not auto-mutated
- GIVEN doctor reports an auth or org-policy blocker
- WHEN `repair` runs
- THEN repair exits without bypassing the blocker
- AND output marks the finding as manual-action-required

### Requirement: REQ-UPGRADE-04 — Idempotent Bootstrap Checkpoints

`bootstrap` MUST persist checkpoints so retries are idempotent. It SHALL use enrollment backfill, run first push, and verify with pull/status without duplicate historical mutations.

#### Scenario: Retry resumes from checkpoint
- GIVEN bootstrap failed after backfill but before verification
- WHEN `engram cloud upgrade bootstrap --project X` is retried
- THEN execution resumes from persisted checkpoint
- AND completed stages are not re-enqueued

#### Scenario: Completed bootstrap is no-op on rerun
- GIVEN bootstrap for project X already completed successfully
- WHEN bootstrap runs again
- THEN command returns success with no additional historical replay
- AND status stays `ready`

### Requirement: REQ-UPGRADE-05 — Status and Rollback Semantics

`status` MUST expose stage, completion markers, and last failure reason. `rollback` MUST be allowed only before bootstrap completion and SHALL restore pre-upgrade local config/enrollment snapshot. After successful bootstrap, rollback MUST fail loudly and direct users to explicit disconnect flows.

#### Scenario: Rollback before completion restores snapshot
- GIVEN bootstrap is incomplete and snapshot metadata exists
- WHEN `engram cloud upgrade rollback --project X` runs
- THEN pre-upgrade local config/enrollment state is restored
- AND autosync is disabled

#### Scenario: Rollback after completion is blocked
- GIVEN bootstrap completed successfully for project X
- WHEN rollback is requested
- THEN command exits non-zero
- AND output says rollback is unavailable post-bootstrap

### Requirement: REQ-UPGRADE-06 — Documentation Alignment

Project docs MUST describe command forms, deterministic failure visibility, and local-first semantics. Docs SHOULD mark installer/plugin automation for this workflow as deferred.

#### Scenario: Docs match command surface
- GIVEN updated README/DOCS/agent setup/plugin docs
- WHEN command examples are compared to CLI help
- THEN documented subcommands and flags are valid and current

#### Scenario: Local-first semantics are explicit in docs
- GIVEN a reader follows upgrade documentation
- WHEN they review data ownership semantics
- THEN docs state local SQLite remains authoritative
- AND cloud is described as replication/shared access
