# Project verification

Users check that a committed project Pack contract and its projections agree without using personal activation state.

## Sub-features

- Verify a valid contract.
- Detect a changed receipt-owned projection.

## How to get to it (user POV)

Run `packy verify` from a project Git worktree with a Pack contract.

## Driving it with CLI

Preconditions: Doctor passed. Prepare a disposable Git worktree with a reviewed installed Pack and committed `packy.json`, `packy.lock.json`, notices, and projections. The [project lifecycle guide](../../../../docs/project-pack-lifecycle.md) describes their creation.

- In the disposable project, run `"$PACKY_VERIFY_SCRATCH/packy" verify`. Capture a successful result and exit status, and compare the project and home trees before and after.
- Change one receipt-owned projection in the disposable project and repeat. Capture the failed status and drift report. Confirm the command did not repair or further change the file.

## Gotchas

- An empty project is not a valid substitute for an installed contract. The selected snapshot and an interactive consent path are prerequisites for preparing one.
