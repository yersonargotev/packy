# Catalog refresh

Users acquire and select the latest Catalog Snapshot of a catalog. Packy keeps each catalog below `~/.local/share/packy/catalog/v2/<owner>/<repo>`.

## Sub-features

- Refresh downloads, verifies, retains, and selects the latest official snapshot.
- A refused refresh keeps the previous selection.
- A store that has only the earlier `catalog/selected.json` layout fails reads and names `docs/catalog-adoption.md`.

## How to get to it (user POV)

Run `packy init` or `packy catalog refresh`, then `packy list --json`.

## Driving it with CLI

Preconditions: Doctor passed. Build the controlled transport binary with `go build -o "$PACKY_VERIFY_SCRATCH/packy" ./internal/cli/testdata/packytransport`. Start `go run ./internal/githubfake/cmd/githubfake -fixtures "$PACKY_VERIFY_SCRATCH/fixtures" -addr-file "$PACKY_VERIFY_SCRATCH/addr"` and wait for the address file. Export `PACKY_GITHUBFAKE_ADDR_FILE="$PACKY_VERIFY_SCRATCH/addr"`.

- Run `"$PACKY_VERIFY_SCRATCH/packy" catalog refresh` and capture the selected snapshot. Confirm `catalog/v2/yersonargotev/packy-catalog/selected.json` names it.
- Build a changed fixture with `packy catalog build --source-repository yersonargotev/packy-catalog --source-commit <commit>`. Copy its two assets into `fixtures/yersonargotev/packy-catalog/release/`, and set `tag_name` and `target_commitish` in `release.json` to the new commit. Refresh again and inspect the selection and the retained `snapshots` directories.
- Seed `catalog/selected.json` in a fresh sandbox and run `packy list --json`. Confirm exit 1, the adoption guide path, and unchanged earlier files.

## Gotchas

- The fake signs every asset it serves. Fake an attestation failure only through `attestation_workflow` in `repository.json`.
- Production `cmd/packy` reaches real GitHub. Use it only for the official catalog and record a missing network as incomplete.
