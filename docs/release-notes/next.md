# {{TAG}} — Packy v0.2

This release adds a shareable Pack trust audit and versioned JSON catalog
output, makes interruption and failed initialization safer, expands TUI search,
adds the pstack Pack, and updates the bundled Matty and Engram Packs.

## Changes since the previous release

- `packy audit` now combines workstation health, active global Pack health,
  and portable verification of the current project's Pack contract in one
  deterministic, redacted report. `packy audit --json` emits the checked-in
  `packy-audit` v1 contract.
- Audit preserves Packy's three-valued readiness semantics: unknown runtime
  observations remain informational, warnings exit successfully, and only
  confirmed failures return a non-zero status after emitting the full report.
- Running Audit outside a Git worktree or in a project without a Pack contract
  is informational. When a contract exists, Audit uses the same personal-state-
  free verification boundary as `packy verify`.

- `packy list --json` now emits the checked-in `pack-list` v1 contract with
  every current Pack and deterministic Pack and surface ordering. The existing
  human-readable table is unchanged.
- `Ctrl+C` now cancels pre-Apply initialization, preview, Installed Source Git
  operations, and lock waits, while cleanup failures remain visible. Once TUI
  Apply begins, exit remains deferred through repeated interrupts until Apply
  and a bounded fresh inspection finish.
- A failed `packy init` now preserves an accepted empty Installed Source with
  its identity and permissions intact. An initially absent source remains
  absent, and publication refuses to replace a concurrently created
  destination.
- TUI catalog search now matches case-insensitive literal text across Pack IDs
  and descriptions, supported surfaces, external requirements, resource IDs,
  descriptions and requirements, and surface capability types and tool names.
- The reviewed Argote Pack advances to `1.0.3` through the public
  `yersonargotev/argote` Managed Pack Project release `pack-v1.0.3`, preserving
  its engineering and communication guidance on Claude Code, Codex, and
  OpenCode.
- The reviewed pstack Pack advances to `1.0.1` through its public Managed Pack
  Project release, preserving 26 portable engineering skills for Claude Code,
  Codex, and OpenCode while sealing their origin and license evidence.
- The Matty Pack advances to `1.0.4` with the eight selected skill updates from
  upstream `v1.2.3`, while retaining its existing surface bindings and
  Packy-owned capabilities.
- The reviewed Engram Pack advances to `3.1.1` through the public
  `yersonargotev/engram` Managed Pack Project release `pack-v3.1.1`, updating
  Packy's outdated `engram-memory-cli` guidance while preserving its selective
  Codex-only runtime contract.
- The reviewed Orchestrate Pack advances to `1.0.2` through the public
  `yersonargotev/orchestrate-skill` Managed Pack Project release
  `pack-v1.0.2`, preserving the exact `$orchestrate` skill and MIT notice,
  authoring the source-less `coordinate-session` lifecycle, and keeping runtime
  usability unknown until the controlled capability check runs.
- Managed Pack preventive validation now materializes and runtime-loads the
  exact sealed bundle before publication, checks every supported
  surface/selection row, and reports actionable content-free exact-copy drift.

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
