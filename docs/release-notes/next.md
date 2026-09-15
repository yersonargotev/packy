# {{TAG}} — Packy v0.2

This release decouples reviewed Pack content from Packy executable releases.
Packy now consumes verified immutable Catalog Snapshots and provides explicit
catalog authoring, refresh, adoption, and withdrawal workflows.

## Changes since the previous release

- Packy now acquires the independent official Pack catalog as verified,
  immutable Catalog Snapshots. `packy init` performs the first acquisition and
  `packy catalog refresh` explicitly selects newer compatible content. Failed
  integrity, publisher, compatibility, or acquisition checks leave the
  previously selected snapshot usable.
- Active and project Pack receipts retain the exact Catalog Snapshot they were
  applied from. Catalog Refresh changes only available content; updating one
  Pack moves only that receipt to the selected snapshot, while other
  activations keep resolving their retained immutable content. Inspection and
  lifecycle operations otherwise remain offline.
- The TUI shows the selected Catalog Snapshot and offers explicit refresh
  without updating active Packs. Successful and failed refreshes use the same
  catalog behavior and diagnostics as the CLI, while existing preview,
  consent, activation, and selected-update flows remain available.
- Catalog maintainers can use `packy catalog create`, `packy catalog import`,
  and `packy catalog upstream-refresh` to prepare reviewable Pack changes from
  explicit templates and exact upstream commits. The commands preserve
  provenance and notices, validate the complete Catalog Project before an
  atomic write, and never publish directly.
- Upstream Refresh rejects unexpected edits to declared exact copies before
  applying changes. It shows old-to-new upstream differences for maintained
  adaptations and requires explicit reconciliation; unresolved adaptations or
  any acquisition, validation, or concurrent-write failure leave the entire
  request unapplied.
- Reviewed Catalog Project merges can publish complete immutable Catalog
  Snapshots independently of Packy. Pack versions remain independent, retries
  accept only byte-identical publications, and compatible content-only
  releases no longer require a Packy binary release.
- A Pack withdrawn from the selected catalog cannot be newly activated,
  installed, updated, or configured. Existing global and project receipts stay
  inspectable and removable from their retained snapshots; withdrawal never
  triggers an automatic uninstall or downgrade.
- The old release-coupled Installed Source Git checkout, repository-ancestor
  discovery, bootstrap flags, and `PACKY_SKILLS_SOURCE` environment override
  have been removed. Packaged commands resolve only the selected official
  Catalog Snapshot; tests inject local fixture sources through code.
- Existing release-coupled installations have an explicit
  [clean adoption procedure](../catalog-adoption.md). The previous Packy first
  inventories, previews, deactivates, and uninstalls its own receipts while
  their sources remain available; only then is the binary replaced and the
  current catalog initialized for explicit reinstall and reactivation. Packy
  does not convert state or delete old sources, personal data, credentials,
  Memory, or foreign host configuration.

The release artifact format is unchanged.

## Install or upgrade

Existing `v0.2.x` users must complete the
[clean catalog adoption procedure](../catalog-adoption.md) with their current
Packy binary before upgrading. Existing `v0.1.x` users must complete the
warning-first [one-time v0.2 reset](../reset-v0.2.md). Then install or upgrade
Packy and inspect the current catalog:

```sh
brew install yersonargotev/tap/packy
packy init
packy list
packy activate engram --surface codex --dry-run
packy activate engram --surface codex
```

Claude Code **2.1.203 or newer** remains the supported floor.
