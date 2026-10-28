# {{TAG}} — Claude skill asset closure

## Changes since the previous release

The previous stable release is `v0.2.33`.

- Reject unsupported Claude skill asset layouts during Catalog qualification,
  before they can pass validation and later interrupt general or targeted
  status. External required assets need explicit reviewed composite references;
  missing references and destination collisions fail before lifecycle mutation
  at both global and project scope.
- Preserve the complete skill tree, required supporting assets, legal closure,
  modes and metadata through the existing reviewed composite-skill contract.
  The Addy Catalog correction is published independently with its own Pack
  version and immutable snapshot.
- Correct the adoption quickstart to use an available Catalog skill and the
  installed Codex controlled-check guide to distinguish the two local calls
  required for runtime evidence.

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
