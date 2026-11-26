# CLI inspection

Users inspect the executable identity, the registered catalogs, and their selected Catalog Snapshots without changing Pack state.

## Sub-features

- Version shows the executable identity.
- `catalog list` shows every registered catalog with its trust policy and selected snapshot.
- List and show inspect the Packs of every selected snapshot and a Pack. List names each Pack's catalog.

## How to get to it (user POV)

Run `packy version --json`, `packy catalog list --json`, `packy list --json`, or `packy show <pack> --json`.

## Driving it with CLI

Preconditions: Doctor passed. List and show require a selected reviewed Catalog Snapshot containing the named Pack.

- Run `"$PACKY_VERIFY_SCRATCH/packy" version --json` and capture the JSON and exit status. Compare its identity with Doctor.
- Run `"$PACKY_VERIFY_SCRATCH/packy" catalog list --json` and confirm each registered catalog and its selected snapshot.
- Run `"$PACKY_VERIFY_SCRATCH/packy" list --json`, then `"$PACKY_VERIFY_SCRATCH/packy" show <pack> --json`. Confirm the named Pack and expected metadata match the selected snapshot.
- Compare the sandbox home and project trees before and after. Inspection must not change Pack state.

## Gotchas

- A fresh sandbox has no selected catalog. Provision a reviewed snapshot through an authorized route before list and show; record an unavailable snapshot as incomplete.
