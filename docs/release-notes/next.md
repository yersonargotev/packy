# {{TAG}} Skill Search and Catalog Snapshot restore

## Changes since the previous release

The previous stable release is `v0.2.38`.

- `packy search <query>... --surface agents|claude` finds current catalog skills
  by text in one call, across every registered catalog. A skill matches when
  every term is a case-insensitive substring of its qualified skill, native
  name, or effective description. Results are sorted, carry no score, and keep
  incompatible candidates with their reason. See
  [on-demand skill access](https://github.com/yersonargotev/packy/blob/main/docs/on-demand-skills.md#discover-current-candidates).
- Structured output adds the `skill-search` v1 schema. See
  [structured output](https://github.com/yersonargotev/packy/blob/main/docs/structured-output.md).
- `packy catalog restore <owner/repo> <snapshot>` downloads one exact Catalog
  Snapshot again and restores its retained files when they changed or are
  missing, without changing the selection. A command that needs a changed older
  snapshot, through `--snapshot`, a receipt, or a project lock, names this
  command.
- Packy reads a GitHub token only when `packy catalog add`,
  `packy catalog refresh`, or `packy catalog restore` reaches a private
  catalog.
- For contributors, the repository's `packy-cli` skill searches the catalog
  before falling back to `packy list` and `packy show`.

Catalog content and Pack versions are published independently. This engine
release does not publish a Catalog Snapshot or change installed Packs.

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

## Adoption

Upgrades from `v0.2.38` need no adoption steps. Upgrades from `v0.2.37` or
earlier change the catalog store and receipt formats; follow the
[clean adoption procedure](https://github.com/yersonargotev/packy/blob/main/docs/catalog-adoption.md)
before replacing the binary.
Claude Code **2.1.203 or newer** remains the supported floor. Plugin generation
proves package content, not client installation, authorization, or runtime execution.

## Release verification

The immutable release workflow validates and tests the same staged archives on
Darwin and Linux for amd64 and arm64 before publication. The release includes
four archives, `SHA256SUMS`, source identity via `packy version --json`, and a
matching Homebrew formula. See the [release guidance](https://github.com/yersonargotev/packy/blob/main/docs/release.md).
