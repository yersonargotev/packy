# CLI inspection

Users inspect the executable identity and the locally selected catalog without changing Pack state.

## Sub-features

- Version shows the executable identity.
- List and show inspect the selected catalog and a Pack.

## How to get to it (user POV)

Run `packy version --json`, `packy list --json`, or `packy show <pack> --json`.

## Driving it with CLI

Preconditions: Doctor passed. List and show require a selected reviewed Catalog Snapshot containing the named Pack.

- Run `"$PACKY_VERIFY_SCRATCH/packy" version --json` and capture the JSON and exit status. Compare its identity with Doctor.
- Run `"$PACKY_VERIFY_SCRATCH/packy" list --json`, then `"$PACKY_VERIFY_SCRATCH/packy" show <pack> --json`. Confirm the named Pack and expected metadata match the selected snapshot.
- Compare the sandbox home and project trees before and after. Inspection must not change Pack state.

## Gotchas

- A fresh sandbox has no selected catalog. Provision a reviewed snapshot through an authorized route before list and show; record an unavailable snapshot as incomplete.
