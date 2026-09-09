# {{TAG}} — Packy v0.2

This release improves global Pack inspection and update previews, and validates
the default Installed Source before using its catalog.

## Changes since the previous release

- Global status checks receipt-owned projections on Codex, OpenCode, and Claude
  Code even when the installed Pack's historical manifest is unavailable. It
  preserves the installed version and selected resources separately from the
  current catalog version, reports unavailable historical evidence explicitly,
  and inspects active receipts on retired surfaces without recommending an
  unsupported update. Read-only inspection does not advance the receipt.
- Global update previews explicitly report when the installed version's
  historical contract is unavailable, instead of implying that its contract is
  unchanged. Target resources, planned actions, and readiness remain available.
  When the installed version matches the catalog, comparisons use the receipt's
  selected resources as the baseline.
- Global status and lifecycle JSON advance to schema **v12**. Status includes
  historical-evidence availability; lifecycle contract diffs include
  `baseline_available` and, when unavailable,
  `historical_contract_unavailable`. Empty classification arrays in that case
  mean the comparison is unavailable, not that there are no changes.
- Catalog operations validate the default Installed Source at
  `~/.local/share/packy` offline before consuming it. Its Git checkout must match
  the running release, and its manifests and declared resource closures must
  match the release's Managed Pack Registry and Pack Admission Records. These
  checks do not repair or modify the checkout. Run `packy init` to align a clean
  older source; move an invalid or modified checkout aside to preserve local
  changes, then run `packy init` to create a fresh copy. Repository-ancestor
  sources and explicit `PACKY_SKILLS_SOURCE` overrides remain editable
  development sources.

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
