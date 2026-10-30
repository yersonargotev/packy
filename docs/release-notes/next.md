# {{TAG}} Direct Pack management in the TUI

## Changes since the previous release

The previous stable release is `v0.2.34`.

- Open resource selection or applicable lifecycle actions with Enter, without
  a required Pack overview step. Press `i` for full Pack details and Esc to
  return to the dashboard.
- Prioritize Pack controls and the selected Pack's description over catalog
  metadata. Keep the catalog summary compact and use `?` to inspect its
  repository, snapshot, and digest.
- Keep primary controls and catalog paging usable at the minimum terminal size
  of 48 columns and 14 rows.
- Show diagnostics and blockers when a project Pack has no applicable action,
  including blocked or drifted installations.
- Preserve preview, phase consent, application, and fresh status inspection for
  global and project operations.
- Show underlying release bundle download errors in workflow logs.
- For repository contributors, add project-scoped agent skills and bounded
  projection-batch regression verification.

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
