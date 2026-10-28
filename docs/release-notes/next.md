# {{TAG}} — Exact Catalog origin acquisition

## Changes since the previous release

The previous stable release is `v0.2.32`.

- Fetch the exact pinned commit for each public Catalog origin at depth one,
  without downloading unrelated branches, tags or ancestor history. This avoids
  cold Catalog validation and build timeouts when an upstream repository has
  accumulated many refs or extensive history.
- Preserve the complete requested source tree, including executable modes and
  symbolic links. TLS verification, exact-copy provenance checks, sealed-cache
  integrity, private consumer copies, cancellation and the existing acquisition
  deadline remain enforced.

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
