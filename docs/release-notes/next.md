# {{TAG}} — Reliable Claude skill snapshot updates

## Changes since the previous release

The previous stable release is `v0.2.29`.

- Updating a global Claude Pack now moves ordinary owned skill links to the
  newly selected Catalog Snapshot even when the skill's bytes are unchanged.
  The installed link and Pack receipt stay aligned, so later deactivation can
  retire the managed skill cleanly instead of rejecting its old snapshot target
  as changed ownership.
- An already-correct link on the same snapshot remains a no-op. Links retargeted
  to a foreign path are still rejected during update and removal, including when
  the foreign tree has identical bytes. A source change after Preview invalidates
  the plan before Apply mutates the installation.
- Content-drift protection and the existing receipt format remain unchanged.
  Generated skill trees and templates retain their separate lifecycle behavior.

## Catalog adoption

Existing installations receive this correction through an explicit Pack update;
installing this engine does not update installed Packs or publish Catalog content.
The correction supports subsequent reviewed Catalog updates whose unchanged
Claude skills move between immutable snapshots.

For a new installation, acquire the official catalog and preview activation
before changing a CLI surface:

```sh
packy init
packy list
packy activate engram --surface codex --dry-run
packy activate engram --surface codex
```

Claude Code **2.1.203 or newer** remains the engine's supported floor.
