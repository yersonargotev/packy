# Catalog registration

Users register more GitHub catalogs next to the official one, refresh them, list them, and remove them. Each registration fixes a trust policy and lives at `~/.local/share/packy/catalog/v2/<owner>/<repo>/registration.json`.

## Sub-features

- `packy catalog add <owner/repo> [--trust attested] [--workflow <path>]` downloads, validates, collision-checks, and selects the first snapshot before it writes the registration. When the official catalog has no selection, it selects the official catalog first and fails the add if it cannot. A refused add leaves no registration or snapshot directory of the new catalog. When no attestation of the new catalog verifies, the error names `--workflow`; an official catalog attestation failure, a transport error, or a Sigstore failure does not.
- `packy catalog add <owner/repo> --trust access-controlled` reads `GH_TOKEN`, then `GITHUB_TOKEN`, then `gh auth token`, and accepts only a repository that the API reports as private. A missing, rejected, or insufficient token fails with an explanation that names `docs/private-catalogs.md`. `catalog refresh` reads the token again. Packy sends the token only to `api.github.com` and never stores it.
- `packy catalog list` and `packy list` mark `access-controlled` as the weaker trust level.
- `packy catalog list [--json]` shows repository, trust policy, workflow, and selected snapshot.
- `packy catalog refresh [<owner/repo>] [--json]` refreshes every catalog or one. Each catalog refreshes on its own. The command prints every result and exits 1 when any failed. A failed catalog keeps its previous selection.
- `packy catalog remove <owner/repo>` refuses the official catalog and any catalog that an installed Pack receipt in `~/.packy/packs.json` or an active personal project activation under `~/.packy/projects` names. It does not see project locks; a project whose lock names a removed catalog fails later with an error that names `packy catalog add`. Removal keeps the retained snapshots.
- `packy list` and `packy status` name each Pack's catalog. New activations find Packs in every selected snapshot. Updates look only in the catalog that the receipt names.
- `packy status --project`, `packy install`, `packy update --project`, and `packy activate --project` fail and name a catalog that the project lock uses but this workstation has not registered.

## How to get to it (user POV)

Run `packy catalog add acme/packs`, then `packy catalog list` and `packy list`.

## Driving it with CLI

Preconditions: Doctor passed and the controlled transport from [Catalog refresh](catalog-refresh.md) runs with a new fixture directory. A new fixture directory also serves `acme/packs` with Pack `acme` and `evil/packs`, whose Pack `fixture` collides with the official catalog.

- Run `catalog refresh`, then `catalog add acme/packs`. Confirm `catalog list --json` validates against `schemas/cli/v1/catalog-list.schema.json` and shows both catalogs as `attested` with their snapshots. Confirm `list --json` shows `acme` with `catalog_repository` `acme/packs`.
- Run `catalog add evil/packs`. Confirm exit 1, a message that names Pack `fixture` and `yersonargotev/packy-catalog`, and an unchanged `find "$HOME/.local/share/packy" -type f`.
- Activate `acme` in a private PTY, then run `catalog remove acme/packs`. Confirm exit 1. Deactivate it, remove again, and confirm exit 0 and that `catalog/v2/acme/packs/snapshots` still exists.
- Set `release_status` to 500 in `fixtures/acme/packs/repository.json` and publish a newer official snapshot. Run `catalog refresh`. Confirm exit 1, a result line for each catalog, an advanced official selection, and the previous `acme/packs` selection.

## Driving private catalogs with CLI

Preconditions: the same transport, started with `-request-log "$PACKY_VERIFY_SCRATCH/requests.jsonl"` in a new fixture directory. The seeded `team/private-packs` serves Pack `team` and answers only `GH_TOKEN=packy-fixture-token`. Without the token it answers 404, and with another token it answers 401.

- Run `catalog refresh`, then `GH_TOKEN=packy-fixture-token packy catalog add team/private-packs --trust access-controlled`. Confirm exit 0, `access-controlled` in `catalog list --json`, and `team/private-packs (access-controlled)` in `packy list`.
- With `GH_TOKEN` unset and no `gh` on `PATH`, add it in a fresh sandbox. Confirm exit 1, a message that names `GH_TOKEN` and `gh auth login`, and no files below `catalog/v2/team`. Repeat with `GH_TOKEN=wrong`, and add `acme/packs --trust access-controlled` with the fixture token. Confirm exit 1 each time.
- Grep `$HOME` and `$XDG_CONFIG_HOME` for `packy-fixture-token` and confirm no match. Confirm that no line of `requests.jsonl` with host `release-assets.githubusercontent.com` lists `Authorization`.

## Gotchas

- Registration happens per workstation. A project lock that names a catalog fails `status --project` in a home that never registered it.
- Pack IDs are global. Refresh of any catalog, the official one included, refuses a snapshot that introduces a Pack ID another registered catalog provides.
