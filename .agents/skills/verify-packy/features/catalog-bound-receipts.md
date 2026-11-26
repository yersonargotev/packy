# Catalog-bound receipts

Users install and activate Packs whose receipts name their catalog. Each receipt and each project lock entry records `catalog_repository` next to `catalog_snapshot`.

## Sub-features

- Global activation writes `~/.packy/packs.json` at `schema_version` 2 with the catalog pair.
- Project installation writes `packy.lock.json` at `schema_version` 2 with the catalog pair. The manifest `packy.json` stays at `schema_version` 1.
- An earlier `packs.json` with receipts, an earlier `packy.lock.json`, or an earlier personal project activation fails and names `docs/catalog-adoption.md`. The file stays unchanged.
- List, show, status, activation, project installation, skill access, and plugin export work from retained snapshots with the network blocked.
- `packy-provenance.json` of a plugin export and the skill-access cache path `skill-access/v3/<owner>/<repo>/<commit>` name the catalog repository.

## How to get to it (user POV)

Run `packy catalog refresh`, then `packy activate <pack> --surface agents` and, in a Git worktree, `packy install <pack> --surface agents`. Read `~/.packy/packs.json` and `packy.lock.json`.

## Driving it with CLI

Preconditions: Doctor passed and the controlled transport from [Catalog refresh](catalog-refresh.md) runs. Apply needs an interactive terminal, so give each `activate` and `install` its approval in a private PTY.

- Refresh, activate, and install. Confirm that both files name `yersonargotev/packy-catalog` and the selected commit.
- Point `PACKY_GITHUBFAKE_ADDR_FILE` at a missing file to block the network. Run `list --json`, `show <pack>`, `status`, `status --project`, `get skill <pack>:<skill> --surface agents`, and `export <pack> --surface agents --output <dir>`. Confirm exit 0 for each.
- Copy a `packs.json` that still holds receipts, or a `packy.lock.json`, written by trunk Packy into the sandbox. Run `status` or `status --project`. Confirm exit 1, the adoption guide path, and byte-identical files.

## Gotchas

- The fixture Pack from `githubfake.WriteCatalogProject` has no `on_demand` or `plugin_exports` contract. Add them to its manifest before `catalog build` to drive skill access and plugin export.
