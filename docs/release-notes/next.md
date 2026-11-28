# {{TAG}} Multiple and private Pack catalogs

## Changes since the previous release

The previous stable release is `v0.2.37`.

- Packy reads Packs from several GitHub catalogs. `packy catalog add <owner/repo>`
  registers a catalog next to the official one and selects its first Catalog
  Snapshot. `packy catalog list` shows each catalog with its trust policy and
  selected snapshot. `packy catalog refresh [<owner/repo>]` refreshes every
  registered catalog, or one. A catalog that fails keeps its previous snapshot,
  the other catalogs refresh, and the command exits 1. `packy catalog remove
  <owner/repo>` removes a registration and keeps its retained snapshots.
- `packy catalog add` selects the official catalog first when it has no
  selected snapshot yet. Packy refuses a Catalog Snapshot that introduces a Pack
  ID that another registered catalog already provides.
- `packy catalog remove` refuses while a workstation receipt, an active personal
  project activation, or the lock of the current Git project names the catalog.
- Private GitHub repositories can be catalogs. The default `attested` trust
  policy requires a GitHub build-provenance attestation signed by the registered
  publication workflow on `refs/heads/main` on a GitHub-hosted runner,
  including attestations from GitHub's private Sigstore instance. Use
  `--workflow <path>` when that workflow is not `.github/workflows/publish.yml`. `--trust access-controlled` trusts
  GitHub access control to a private repository instead. `packy catalog list`,
  the new `CATALOG` column of `packy list`, and the TUI show it as weaker trust.
  See [private catalogs](https://github.com/yersonargotev/packy/blob/main/docs/private-catalogs.md).
- Packy reads a GitHub token only when `packy catalog add`,
  `packy catalog refresh`, or `packy catalog restore` reaches a private
  catalog. It takes the first of `GH_TOKEN`, `GITHUB_TOKEN`, and
  `gh auth token`, sends it only to `https://api.github.com`, and never writes
  it to disk. Installation, status, `packy get skill`, and `packy export` read
  retained snapshots and need no token.
- Lookups, status, installation, update, and project locks use the catalog that
  provides each Pack. Receipts in `~/.packy/packs.json` and `packy.lock.json`
  record `catalog_repository`, and plugin `packy-provenance.json` records the
  catalog repository.
- `packy catalog refresh` restores a tampered retained Catalog Snapshot.
- `packy catalog restore <owner/repo> <snapshot>` downloads one exact Catalog
  Snapshot again and restores its retained files when they changed or are
  missing, without changing the selection. A command that needs a changed older
  snapshot, through `--snapshot`, a receipt, or a project lock, names this
  command.
- `packy catalog add` errors lead with the command and the cause, and catalog
  refresh errors are shorter.
- The TUI catalog panel lists every registered catalog with its repository, trust
  policy, and selected snapshot. Each Pack row names its catalog, and Packs from
  an `access-controlled` catalog show weaker trust. `?` adds each catalog's full
  name, signing workflow, Pack count, and digest. `c` refreshes every catalog and
  marks a partial refresh.
- `packy search <query>... --surface agents|claude` finds current catalog skills
  by text in one call, across every registered catalog. A skill matches when
  every term is a case-insensitive substring of its qualified skill, native
  name, or effective description. Results are sorted, carry no score, and keep
  incompatible candidates with their reason. See
  [on-demand skill access](https://github.com/yersonargotev/packy/blob/main/docs/on-demand-skills.md#discover-current-candidates).
- Structured output adds `catalog-list`, `catalog-refresh`, and `skill-search` v1 schemas.
  `packy list --json` moves to Pack list v2 with `catalog_repository`, project
  installation and update previews move to v5, and `packy.lock.json` follows
  project lock v2. See [structured output](https://github.com/yersonargotev/packy/blob/main/docs/structured-output.md).
- Third-party catalog authors follow the
  [catalog publication guide](https://github.com/yersonargotev/packy/blob/main/docs/catalog-project.md#publish-a-third-party-catalog).
- For contributors, repository development skills are now installed from a
  separate home repository, and release cleanup removes completed local evidence.

Catalog content and Pack versions are published independently. This engine
release does not publish a Catalog Snapshot or change installed Packs.

## Adoption

This release changes the catalog store and receipt formats. Packy keeps each
catalog under `~/.local/share/packy/catalog/v2/<owner>/<repo>` and no longer
reads the earlier `catalog/selected.json` and `catalog/snapshots` layout.
Packy reads receipts, project locks, and personal project activations only at
`schema_version` 2. It rejects an earlier `packs.json` that still holds
receipts, an earlier `packy.lock.json`, or an earlier personal project
activation with an error that names the adoption guide. No converter exists.

Before you replace the binary, deactivate every global and personal project
activation and uninstall every shared project installation with the previous
Packy. Then follow the
[clean adoption procedure](https://github.com/yersonargotev/packy/blob/main/docs/catalog-adoption.md),
reinstall the Packs, and commit the new `packy.lock.json`.
Claude Code **2.1.203 or newer** remains the supported floor. Plugin generation
proves package content, not client installation, authorization, or runtime execution.

## Initialization and explicit activation

Initialize the catalog, inspect the available `packy` Pack, then preview and
activate only its `packy-cli` skill at global Agents scope:

```sh
packy init
packy list
packy show packy
packy activate packy --surface agents --resource skill:packy-cli --dry-run
packy activate packy --surface agents --resource skill:packy-cli
packy status packy --surface agents
```

The explicit selection includes the skill and its legal notice, without
project-only instructions or a concrete `--client`. Configured projections do
not prove runtime authorization or execution.

## Release verification

The immutable release workflow validates and tests the same staged archives on
Darwin and Linux for amd64 and arm64 before publication. The release includes
four archives, `SHA256SUMS`, source identity via `packy version --json`, and a
matching Homebrew formula. See the [release guidance](https://github.com/yersonargotev/packy/blob/main/docs/release.md).
