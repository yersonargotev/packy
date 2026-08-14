# {{TAG}} — Packy v0.2

This release hardens Packy's immutable release gate so the approved
publication describes the exact candidate it ships.

## Changes since the previous release

- The Engram Pack now vendors the exact `engram-memory-cli` skill from Engram
  `v2.0.0`, preserves its MIT notice, and retires Packy's former
  `engram-memory` skill and helper without compatibility aliases.
- Release preparation now derives the default next patch from the latest
  published stable release and rejects versions already present locally,
  remotely, or in GitHub Releases.
- Release notes must cover the complete candidate delta, retain one tag
  placeholder, and keep factual version claims aligned with repository
  authorities before approval.
- The publication brief now includes release-note evidence, and its approval
  applies to protected release and Homebrew deployments only when their
  workflow run, version, and commit match exactly.

Pack lifecycle behavior and the release artifact format are unchanged.

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
