# {{TAG}} — Packy v0.2

This release updates the reviewed Engram Pack and makes global lifecycle results
report Pack-scoped projection ownership accurately.

## Changes since the previous release

- The reviewed Engram Pack advances from `3.1.2` to `3.3.1` through the public
  `yersonargotev/engram` Managed Pack Project release `pack-v3.3.1`. It adds a
  machine-verifiable Protocol v1 compatibility asset and updates its Codex
  memory guidance with bounded Recall, Terminal Memory preflight and checkpoint
  outcomes, mixed results, explicit continuation, and direct terminal recording
  without routine checkpoint status probes.
- Successful global lifecycle operations now report only the verified
  projections owned by the selected Pack. Unrelated active Packs no longer
  inflate the count shown by the CLI, JSON output, or TUI.

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
