# CLI inspection

Users inspect the executable identity, the registered catalogs, and their selected Catalog Snapshots without changing Pack state.

## Sub-features

- Version shows the executable identity.
- `catalog list` shows every registered catalog with its trust policy and selected snapshot.
- List and show inspect the Packs of every selected snapshot and a Pack. List names each Pack's catalog.
- Search filters the skills of every selected snapshot by text for one surface and names each candidate's catalog.

## How to get to it (user POV)

Run `packy version --json`, `packy catalog list --json`, `packy list --json`, or `packy show <pack> --json`. Run `packy search <query>... --surface <agents|claude> --json` to find skills.

## Driving it with CLI

Preconditions: Doctor passed. List and show require a selected reviewed Catalog Snapshot containing the named Pack.

- Run `"$PACKY_VERIFY_SCRATCH/packy" version --json` and capture the JSON and exit status. Compare its identity with Doctor.
- Run `"$PACKY_VERIFY_SCRATCH/packy" catalog list --json` and confirm each registered catalog and its selected snapshot.
- Run `"$PACKY_VERIFY_SCRATCH/packy" list --json`, then `"$PACKY_VERIFY_SCRATCH/packy" show <pack> --json`. Confirm the named Pack and expected metadata match the selected snapshot.
- Run `"$PACKY_VERIFY_SCRATCH/packy" search <term> <term> --surface agents --json`. Confirm every candidate matches all terms, rows sort by `qualified_skill`, and each candidate's `compatible` and `reason` equal its entry in `show <pack> --surface agents --json`. Confirm a term with no match exits 0 with `"skills": []`, and an omitted `--surface` exits 1 with an empty stdout.
- Compare the sandbox home and project trees before and after. Inspection must not change Pack state.

## Gotchas

- A fresh sandbox has no selected catalog. Provision a reviewed snapshot through an authorized route before list and show; record an unavailable snapshot as incomplete.
