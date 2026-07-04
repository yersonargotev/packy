# Delta for Upgrade Sync

> **Slice**: 4 (Upgrade + Sync fix)
> **Type**: Modified Capability

## MODIFIED Requirements

### Requirement: Sync Completes Across a Self-Upgrade

When "Upgrade + Sync" is invoked and the upgrade step replaces the current binary (self-upgrade), the sync step MUST still complete. If the current process cannot run sync after replacing itself, the system MUST set a `pending_sync` flag in the state file so that the newly installed binary runs sync automatically on its next launch.

The `pending_sync` flag MUST be cleared after the deferred sync completes successfully.

(Previously: "Upgrade + Sync" silently skipped the sync step after a self-upgrade, leaving the installation in a partially-updated state.)

#### Scenario: Upgrade without self-upgrade (inline sync)

- GIVEN the user invokes "Upgrade + Sync"
- AND the upgrade step does NOT replace the currently-running binary (e.g., only engram or other components are updated)
- WHEN the upgrade step completes
- THEN sync runs in the same process immediately after the upgrade
- AND no `pending_sync` flag is set

#### Scenario: Upgrade WITH self-upgrade — sync deferred

- GIVEN the user invokes "Upgrade + Sync"
- AND the upgrade step replaces the currently-running binary (self-upgrade)
- WHEN the upgrade step applies the new binary
- THEN `pending_sync = true` is written to `~/.gentle-ai/state.json` before the process exits
- AND the current process exits (or closes) after recording the flag

#### Scenario: Deferred sync runs on next launch

- GIVEN `pending_sync = true` is present in state
- WHEN the user next launches the binary (any entry point)
- THEN sync runs automatically at startup before the normal entry point proceeds
- AND after sync completes successfully `pending_sync` is cleared (set to false or removed)
- AND the user sees confirmation that sync completed

#### Scenario: Pending flag cleared after sync

- GIVEN sync was deferred and ran on the next launch
- WHEN the sync step finishes without error
- THEN `pending_sync` is removed or set to false in state
- AND subsequent launches do NOT re-run sync

#### Scenario: Deferred sync fails

- GIVEN `pending_sync = true` is present in state
- AND sync fails during the next launch
- WHEN the failure occurs
- THEN an error is surfaced to the user (not silently swallowed)
- AND `pending_sync` remains true so that the next launch retries
