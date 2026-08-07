# {{TAG}} — Packy v0.2

This release establishes Packy as a focused installer and configurator for
Git-reviewed capability Packs on Codex, OpenCode, and Claude Code.

## Highlights

- Argote `1.0.1` projects its engineering and communication guidance as one
  cohesive instruction, so complete activation is collision-free on every
  supported surface. Addy, Engram, and Matty remain at Pack version `1.0.0`.
- Installed Pack receipts contain only current ownership, projection, and
  digest data.
- GitHub branch protection, CI, CodeQL, review, and human merge protect
  integration.
- One immutable version tag publishes Darwin and Linux archives for amd64 and
  arm64, `SHA256SUMS`, one GitHub Release, and a matching Homebrew formula.

## Install or upgrade

Existing `v0.1.x` users must complete the warning-first
[one-time v0.2 reset](../reset-v0.2.md). Then install and inspect the current
catalog:

```sh
brew install yersonargotev/tap/packy
packy init
packy pack list
packy pack activate engram --surface codex --dry-run
packy pack activate engram --surface codex
```

Claude Code **2.1.203 or newer** remains the supported floor.
