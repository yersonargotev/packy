# {{TAG}} — Packy v0.2

This release updates the reviewed Engram Pack and hardens Packy's Git repository
boundaries through patched dependencies and expanded validation coverage.

## Changes since the previous release

- The reviewed Engram Pack advances from `3.1.1` to `3.3.0` through the public
  `yersonargotev/engram` Managed Pack Project release `pack-v3.3.0`. It adds a
  machine-verifiable Protocol v1 compatibility asset and updates its Codex
  memory guidance with bounded Recall, Terminal Memory preflight and checkpoint
  outcomes, mixed results, and explicit continuation.
- Git and text-processing dependencies advance to patched releases, including
  `go-git` `5.19.2` and `x/text` `0.39.0`. Repository validation now covers
  exact installed-source and Managed Pack checkouts, rejects malformed Git
  objects and portable path collisions, and ensures credentials for one origin
  are not forwarded when a clone redirects to another origin.

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
