# Catalog refresh

Users acquire and select the latest Catalog Snapshot of every registered catalog, or of one. Packy keeps each catalog below `~/.local/share/packy/catalog/v2/<owner>/<repo>`.

## Sub-features

- Refresh downloads, verifies, retains, and selects the latest snapshot of each catalog. [Catalog registration](catalog-registration.md) covers several catalogs.
- A refused refresh keeps the previous selection.
- When the selected snapshot's files change on disk, reads fail with its digest mismatch and name `packy init` or `packy catalog refresh <owner/repo>`, and `packy catalog restore <owner/repo> <commit>`. If that commit is still the latest Catalog Publication, refresh restores it from the verified release; otherwise refresh selects the newer snapshot.
- When a retained snapshot of an older commit changes on disk, reads that name it through `--snapshot`, a receipt, or a project lock fail and name `packy catalog restore <owner/repo> <commit>`. Restore downloads that exact release, verifies it, and replaces the retained files without changing the selection.
- When the Pack ID check of a refresh cannot read another catalog's selected index, the error names `packy catalog restore` for that catalog.
- A store that has only the earlier `catalog/selected.json` layout fails reads and names `docs/catalog-adoption.md`.

## How to get to it (user POV)

Run `packy init` or `packy catalog refresh`, then `packy list --json`. Run `packy catalog restore <owner/repo> <commit>` after a read names it.

## Driving it with CLI

Preconditions: Doctor passed. Build the controlled transport binary with `go build -ldflags "-X github.com/yersonargotev/packy/internal/version.SourceCommit=$(git rev-parse HEAD)" -o "$PACKY_VERIFY_SCRATCH/packy" ./internal/cli/testdata/packytransport`, because `catalog build` needs a stamped builder commit. Start `go run ./internal/githubfake/cmd/githubfake -fixtures "$PACKY_VERIFY_SCRATCH/fixtures" -addr-file "$PACKY_VERIFY_SCRATCH/addr"` and wait for the address file. Export `PACKY_GITHUBFAKE_ADDR_FILE="$PACKY_VERIFY_SCRATCH/addr"`.

- Run `"$PACKY_VERIFY_SCRATCH/packy" catalog refresh` and capture the selected snapshot. Confirm `catalog/v2/yersonargotev/packy-catalog/selected.json` names it.
- Copy `packs/` of a Catalog Project, such as a `packy-catalog` checkout, into `"$PACKY_VERIFY_SCRATCH/fixture-project"` and change one Pack version. Run `"$PACKY_VERIFY_SCRATCH/packy" catalog build --project "$PACKY_VERIFY_SCRATCH/fixture-project" --source-repository yersonargotev/packy-catalog --source-commit <commit> --out-dir "$PACKY_VERIFY_SCRATCH/dist"`. Move `fixtures/yersonargotev/packy-catalog/release/` to `fixtures/yersonargotev/packy-catalog/releases/catalog-<old commit>/` so the fake still serves the old release by its tag. Copy the two new assets into a new `release/` with a copy of `release.json`, and set its `tag_name` and `target_commitish` to the new commit. Refresh again and inspect the selection and the retained `snapshots` directories.
- Append a line to one file under the selected `snapshots/<commit>/packs/` and run `packy list`. Confirm exit 1 and the hinted command. Run that command, then confirm `packy list` succeeds, the file holds the released bytes, and `snapshots/` holds no leftover directory.
- With two retained snapshots and the newer one selected, append a line to a file under `snapshots/<older>/packs/`. Run `packy get skill <pack>:<skill> --surface agents --snapshot <older>` and `packy export <pack> --surface agents --output <dir> --snapshot <older>`. Confirm exit 1 and `packy catalog restore yersonargotev/packy-catalog <older>` in each message. Run that command and confirm `restored Catalog Snapshot <older>`. Rerun both readers, and confirm they succeed, the file holds the released bytes, `selected.json` still names the newer commit, and `snapshots/` holds only the two commits. Run restore again and confirm `is intact`.
- Seed `catalog/selected.json` in a fresh sandbox and run `packy list --json`. Confirm exit 1, the adoption guide path, and unchanged earlier files.

## Gotchas

- The fake signs every asset it serves. Fake an attestation failure only through `attestation_workflow` in `repository.json`.
- Production `cmd/packy` reaches real GitHub. Use it only for the official catalog and record a missing network as incomplete.
