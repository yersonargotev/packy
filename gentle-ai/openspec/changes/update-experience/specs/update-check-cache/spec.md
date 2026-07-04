# Update Check Cache Specification

> **Slice**: 2 (Update-check cooldown)
> **Type**: New Capability

## Purpose

The system MUST cache the result of the GitHub update check so that repeated launches within a cooldown window do not hit the GitHub API. The cache is persisted in the state file. On a failed check the cache MUST NOT be updated, so a transient rate-limit or network error does not lock out future fresh checks.

## Requirements

### Requirement: Cooldown Gate on Update Check

The system MUST skip the remote update check when the time elapsed since `last_update_check` is less than the configured cooldown TTL.

The system MUST perform the remote update check when `last_update_check` is absent (first run) or when the cooldown TTL has elapsed.

The system MUST update `last_update_check` in the state file only upon a successful check.

#### Scenario: Cache fresh — no network call

- GIVEN `last_update_check` is present in state
- AND the elapsed time since `last_update_check` is less than the cooldown TTL
- WHEN the binary launches and the update check runs
- THEN no request is made to the GitHub API
- AND the previously known update status is used

#### Scenario: Cache stale — refresh from GitHub

- GIVEN `last_update_check` is present in state
- AND the elapsed time since `last_update_check` meets or exceeds the cooldown TTL
- WHEN the binary launches and the update check runs
- THEN a request is made to the GitHub API
- AND on success `last_update_check` is updated to the current timestamp
- AND the fresh update status is used

#### Scenario: Cache missing — first run

- GIVEN `last_update_check` is absent from state (new install or cleared state)
- WHEN the binary launches and the update check runs
- THEN a request is made to the GitHub API
- AND on success `last_update_check` is set to the current timestamp

### Requirement: Rate-Limit and Failure Resilience

When the update check fails (rate-limit, network error, timeout), the system MUST NOT update `last_update_check`, MUST NOT display an error banner or crash, and MUST continue launch normally.

#### Scenario: Rate-limited response

- GIVEN the GitHub API returns a rate-limit error (HTTP 429 or equivalent)
- WHEN the binary runs the update check
- THEN `last_update_check` is NOT updated
- AND no error is shown to the user
- AND launch continues normally
- AND the next launch will retry the check (cache is not poisoned)

#### Scenario: Network error during check

- GIVEN the update check request fails due to a network error or timeout
- WHEN the binary runs the update check
- THEN `last_update_check` is NOT updated
- AND no error is shown to the user
- AND launch continues normally

### Requirement: State Persistence

`last_update_check` MUST be persisted as a field in `~/.gentle-ai/state.json`. It MUST be a timestamp (RFC 3339 or Unix epoch). Older binaries that do not know this field MUST be able to load the state file without error (additive field, backward-compatible).

#### Scenario: Older binary reads state with new field

- GIVEN the state file contains a `last_update_check` field
- WHEN an older binary (predating this change) loads the state file
- THEN the older binary ignores the unknown field
- AND it does not error or corrupt the state file
