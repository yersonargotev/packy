# {{TAG}} — Pack-local Catalog v2

This release adopts the Pack-local Catalog v2 format used by the independent
Packy catalog. Every Pack now owns its complete declared closure under one
`packs/<pack-id>/` directory.

## Changes since the previous release

The previous stable release is v0.2.23.

- Catalog Snapshot indexes and Pack manifests now use schema v2. Resource
  sources and snapshot file records are relative to the owning Pack root.
- Pack validation rejects undeclared regular files, cross-Pack physical
  sources, and paths outside the Pack root. This makes a Pack independently
  reviewable, movable, and publishable without a shared global resource tree.
- Catalog authoring stages only the target Pack, validates it together with the
  unchanged catalog, rechecks the complete catalog identity, and atomically
  exchanges that Pack directory.
- Packy no longer embeds a duplicate catalog or carries the obsolete global
  `bundle/` and skill-source implementation paths.
- The official catalog migrated all Packs together and major-versioned their
  manifests for the clean schema cut. Matty and PStack continue to own their
  distinct TDD skills within their respective Pack roots.

Catalog Snapshot v1 installations and receipts are not converted in place.
Before upgrading, use the previous Packy binary and its retained snapshot to
complete the [clean catalog adoption procedure](../catalog-adoption.md).

The release artifact format is unchanged.

## Install or upgrade

After completing the clean adoption procedure, upgrade Packy and initialize
the current Catalog v2 snapshot:

```sh
brew upgrade yersonargotev/tap/packy
packy init
packy list
packy activate engram --surface codex --dry-run
packy activate engram --surface codex
```

New installations may use `brew install yersonargotev/tap/packy` instead.
Claude Code **2.1.203 or newer** remains the supported floor.
