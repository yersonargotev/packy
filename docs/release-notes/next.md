# {{TAG}} — Packy v0.2

This release makes global capability Pack updates clean up obsolete
projections recorded by the updating Pack's installed receipt.

## Changes since the previous release

- Global capability Pack updates now retire projections owned by the updating
  Pack's installed receipt when those projections are absent from the current
  reviewed Pack. Replacing a resource therefore removes its obsolete
  projection while installing the replacement.
- Cleanup remains limited to the Pack being updated; projections owned by
  other active Packs are not retirement candidates.
- Drift protection remains in effect. A broken or otherwise drifted obsolete
  projection blocks the update unless the user explicitly supplies `--force`.

Project Pack update behavior and the release artifact format are unchanged.

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
