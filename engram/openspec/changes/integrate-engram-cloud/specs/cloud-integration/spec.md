# cloud-integration Specification

## Purpose

Define optional cloud capabilities that extend (not replace) local-first behavior: explicit cloud commands, cloud-backed sync paths, enrollment gates, deterministic failure visibility, and documentation accuracy.

## Requirements

### Requirement: REQ-CLOUD-01 — Local-First Default Preservation

The system MUST preserve current local behavior when cloud is not configured. Existing local commands (`serve`, `mcp`, `search`, `context`, local sync) SHALL keep equivalent outcomes, exit codes, and side effects.

#### Scenario: Unconfigured cloud keeps local command behavior
- GIVEN a project with no cloud auth or cloud endpoint configured
- WHEN the user runs existing local commands
- THEN command results match pre-cloud behavior
- AND no cloud enrollment state is required

#### Scenario: Explicit cloud command is isolated
- GIVEN a project with no cloud configuration
- WHEN the user runs a cloud-specific command
- THEN local data remains unchanged unless a cloud command explicitly mutates local config
- AND non-cloud commands remain unaffected afterward

### Requirement: REQ-CLOUD-02 — Explicit Cloud Command Surface

The CLI MUST expose `engram cloud ...` commands for authentication, connectivity, server/dashboard integration, and sync status inspection. Invalid cloud subcommands or missing required cloud arguments MUST fail with a non-zero exit code and actionable error text.

#### Scenario: Valid cloud command path is discoverable
- GIVEN the user runs CLI help
- WHEN command listings are rendered
- THEN `engram cloud` and its supported subcommands are present

#### Scenario: Invalid cloud invocation fails loudly
- GIVEN the user runs `engram cloud` with an unknown subcommand
- WHEN argument parsing completes
- THEN the command exits non-zero
- AND stderr explains the accepted cloud subcommands

### Requirement: REQ-CLOUD-03 — Opt-In Enrollment Gate

Cloud sync and autosync MUST run only for projects explicitly enrolled for cloud replication. Unenrolled projects MUST NOT push or pull cloud state.

#### Scenario: Enrolled project can use cloud sync
- GIVEN a project with valid cloud configuration and enrollment enabled
- WHEN sync is invoked against cloud transport
- THEN push and pull operations are attempted per sync policy

#### Scenario: Unenrolled project blocks cloud sync deterministically
- GIVEN a project without cloud enrollment
- WHEN cloud sync or autosync is requested
- THEN sync is rejected before network mutation
- AND status is marked blocked with a deterministic reason code/message

### Requirement: REQ-CLOUD-04 — Deterministic Failure Visibility

Blocked, paused, unauthenticated, or transport-failed cloud sync MUST be surfaced explicitly across CLI/server/dashboard status outputs. The system MUST NOT silently drop failed cloud operations.

#### Scenario: Auth failure propagates to all status surfaces
- GIVEN cloud credentials are missing or expired
- WHEN autosync cycle executes
- THEN the cycle records a failed state with reason `auth_required`
- AND CLI/server/dashboard status endpoints expose that same failure reason

#### Scenario: Network failure remains visible until recovery
- GIVEN cloud endpoint is unreachable
- WHEN push or pull is attempted
- THEN operation reports failure with retryable classification
- AND status remains failed/blocked until a successful subsequent cycle updates it

### Requirement: REQ-CLOUD-05 — Selective Runtime Wiring

Autosync and cloud background workers SHALL start only in cloud-capable long-lived processes with cloud enabled. Processes without cloud enablement MUST NOT start cloud workers.

#### Scenario: Cloud-enabled daemon starts autosync worker
- GIVEN a long-lived process with cloud enabled and enrolled project
- WHEN process startup completes
- THEN cloud autosync worker is active

#### Scenario: Local-only daemon skips autosync worker
- GIVEN the same process type with cloud disabled
- WHEN process startup completes
- THEN no cloud autosync worker is created

### Requirement: REQ-CLOUD-06 — Documentation and Command Accuracy

User-facing docs MUST describe actual cloud commands, config keys, opt-in constraints, and failure semantics. Deprecated or deferred script workflows MUST be clearly marked as out of scope for this change.

#### Scenario: Documented commands are executable as written
- GIVEN README/DOCS command examples for cloud workflows
- WHEN examples are validated against CLI help/output
- THEN command names and flags are valid and current

#### Scenario: Local-first constraints are explicitly documented
- GIVEN docs for cloud integration
- WHEN a reader follows enrollment guidance
- THEN docs clearly state local SQLite is source of truth
- AND docs explain that unenrolled projects do not sync to cloud
