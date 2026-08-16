# {{TAG}} — Packy v0.2

This release adds a shareable Pack trust audit and makes global capability Pack
updates clean up obsolete projections recorded by the updating Pack's
installed receipt.

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
