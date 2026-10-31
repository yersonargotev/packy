# {{TAG}} Projection rollback and release maintenance

## Changes since the previous release

The previous stable release is `v0.2.36`.

- When a later projection target fails its fingerprint check or the fingerprint
  cannot be read, Packy rolls back earlier changes in the same batch. If a
  filesystem operation prevents restoration, Packy reports the rollback failure
  and full restoration is not guaranteed.
- Release preparation verifies trusted CI evidence for the exact source commit.
  The tag workflow retains the evidence and continues to test the staged archives.
- Repository contributors have a Packy verification skill and an updated
  development flow. Release corrections follow that flow and pause at a PR for
  human integration unless separately authorized.
- Repository cleanup checks evidence retention before removing temporary state.
  Release operators have a progress observer and a separate identity check before
  each protected environment approval.

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

For upgrades from the earlier incompatible surface generation, follow the
[clean adoption procedure](https://github.com/yersonargotev/packy/blob/main/docs/catalog-adoption.md).
Claude Code **2.1.203 or newer** remains the supported floor. Plugin generation
proves package content, not client installation, authorization, or runtime execution.

## Release verification

The immutable release workflow validates and tests the same staged archives on
Darwin and Linux for amd64 and arm64 before publication. The release includes
four archives, `SHA256SUMS`, source identity via `packy version --json`, and a
matching Homebrew formula. See the [release guidance](https://github.com/yersonargotev/packy/blob/main/docs/release.md).
