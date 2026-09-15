# {{TAG}} — Packy v0.2

This release makes the interactive dashboard easier to navigate and adds
searchable controls for each Pack's resource selection.

## Changes since the previous release

- Packy now acquires the independent official Pack catalog as verified,
  immutable snapshots. `packy init` performs the first acquisition and `packy
  catalog refresh` explicitly selects newer content without changing existing
  activations; inspection and lifecycle operations otherwise remain offline.
- Active and project Pack receipts retain the exact Catalog Snapshot they were
  applied from. Updating one Pack moves only that receipt to the selected
  snapshot, while other activations keep resolving their retained immutable
  content.
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
- The dashboard now stays within the terminal's visible rows. When content is
  clipped, PageUp and PageDown scroll through the bounded viewport so wrapped
  health checks cannot hide the global or current-project Pack scopes.
- System health is compact by default while keeping its overall status and
  pass, warning, and failure counts visible. Press `s` to expand or collapse the
  individual checks. Vertical mouse-wheel input scrolls the same bounded
  dashboard as a progressive enhancement; keyboard navigation remains fully
  available.
- Arrow keys and `j`/`k` continue to select Packs and now reveal the selected
  row automatically. PageUp and PageDown remain available for direct scrolling,
  and the dashboard keeps offsets valid after resizing, reloading, filtering,
  or toggling health details.
- The TUI can now activate, install, and reconfigure a Pack with an exact
  resource selection in global and project scopes. The checklist supports fuzzy
  search, pagination, select-all and clear-all actions, preserved hidden
  selections, and visible enabled and pending states. Changes remain staged
  until the user opens the existing preview, consent, apply, and verification
  flow.
- Resource dependencies follow the selected operational resources: disabling a
  required resource also proposes disabling its consumers, while supporting
  files and notices remain automatic. Clearing the global selection previews
  whole-Pack deactivation; clearing a project selection previews uninstalling
  that Pack without changing the separate personal project-activation model.

The release artifact format is unchanged.

## Install or upgrade

Existing `v0.1.x` users must complete the warning-first
[one-time v0.2 reset](../reset-v0.2.md). Then install and inspect the current
catalog:

```sh
brew install yersonargotev/tap/packy
packy init
packy list
packy activate engram --surface codex --dry-run
packy activate engram --surface codex
```

Claude Code **2.1.203 or newer** remains the supported floor.
