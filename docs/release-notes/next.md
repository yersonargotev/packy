# {{TAG}} — Packy v0.2

This release completes Packy's move to immutable Managed Pack Projects, updates
all seven bundled Packs, advances structured output to the current catalog and
provenance model, and fixes surface-capability resource closure.

## Changes since the previous release

- The reviewed Addy Pack advances from `1.1.0` to `2.0.3` through the public
  `yersonargotev/skills-addy` Managed Pack Project release `pack-v2.0.3`,
  expanding its reviewed skills and supporting resources. Its `spec` command
  now asks for an approved capability map before independently testable
  capabilities are specified in dependency order.
- The reviewed Argote Pack advances from `1.0.2` to `1.0.3` through the public
  `yersonargotev/argote` Managed Pack Project release `pack-v1.0.3`, preserving
  its engineering and communication guidance on Claude Code, Codex, and
  OpenCode.
- The reviewed pstack Pack advances from `1.0.0` to `1.0.1` through the public
  `yersonargotev/pstack` Managed Pack Project release `pack-v1.0.1`, preserving
  26 portable engineering skills for Claude Code, Codex, and OpenCode while
  sealing their origin and license evidence.
- The reviewed Matty Pack advances from `1.0.4` to `1.1.0` through the public
  `yersonargotev/skills-mattpocock` Managed Pack Project release
  `pack-v1.1.0`, refreshing its reviewed skill and instruction content across
  Claude Code, Codex, and OpenCode.
- The reviewed Engram Pack advances from `3.1.0` to `3.1.1` through the public
  `yersonargotev/engram` Managed Pack Project release `pack-v3.1.1`, updating
  Packy's outdated `engram-memory-cli` guidance while preserving its selective
  Codex-only runtime contract.
- The reviewed Orchestrate Pack advances from `1.0.1` to `1.0.2` through the
  public `yersonargotev/orchestrate-skill` Managed Pack Project release
  `pack-v1.0.2`, preserving the exact `$orchestrate` skill and MIT notice,
  authoring the source-less `coordinate-session` lifecycle, and keeping runtime
  usability unknown until the controlled capability check runs.
- The reviewed Issue Delivery Pack advances from `1.1.1` to `1.1.2` through
  the public `yersonargotev/issue-deliver-pack` Managed Pack Project release
  `pack-v1.1.2`, preserving its three Codex skills and Packy integration and
  dependency behavior while recording its immutable project and origin
  provenance in its Pack Admission Record.
- Managed Pack preventive validation now materializes and runtime-loads the
  exact sealed bundle before publication, checks every supported
  surface/selection row, and reports actionable content-free exact-copy drift.
- The seven current Packs now resolve exclusively through the Managed Pack
  Registry and their exact current Pack Admission Records: Addy `2.0.3`,
  Argote `1.0.3`, Engram `3.1.1`, Issue Delivery `1.1.2`, Matty `1.1.0`,
  Orchestrate `1.0.2`, and pstack `1.0.1`. Generated Pack pages link each
  immutable Managed Pack Project release and Admission Record as the bundled
  Pack's provenance authority; the retired catalog-maintenance schemas,
  loaders, configuration, classification, evidence, and workflows are gone.
- `packy show --json` advances from the checked-in `pack-show` v5 contract to
  v6, replacing `source_identity` with `catalog_identity`. Human-readable show
  output now names the current Catalog identity and its provenance authority.
- Global Pack status and lifecycle JSON advance from v10 to v11. Their resource
  graphs now reflect the selected CLI surface, and exclusions no longer expose
  the retired `source_paths` field from the old catalog-maintenance model.
- Selection, activation, status, and controlled runtime checks now retain
  resources required through surface capabilities, including project
  instructions and primary prompts, in the selected resource closure.

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
