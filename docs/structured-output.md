# Structured CLI output

Packy emits versioned JSON when `--json` is present. Current offline schemas
remain checked in beside the producers that use them:

These are CLI report schema versions, not Pack manifest generations. A global
lifecycle report describes one freshly computed in-memory preview; Packy
persists the resulting installed Pack receipt, not the preview.

| Command family | Schema |
| --- | --- |
| `packy audit --json` | `schemas/cli/v1/pack-audit.schema.json` |
| `packy list --json` | `schemas/cli/v1/pack-list.schema.json` |
| `packy doctor --json` | `schemas/cli/v3/doctor.schema.json` |
| `packy show PACK --json` | `schemas/cli/v7/pack-show.schema.json` |
| global Pack status | `schemas/cli/v13/pack-status.schema.json` |
| global Pack lifecycle | `schemas/cli/v13/pack-lifecycle.schema.json` |
| project installation/update preview | `schemas/project/v4/project-preview.schema.json` |
| other project Pack lifecycle | `schemas/project/v1.0.0/` |

Canonical fixtures live under `internal/cli/testdata/`. Repository tests compile
the schema selected by each document's `schema_version`, validate fixtures and
live producer examples, and reject the checked-in negative project fixtures.

## Reports

| Command | `report` |
| --- | --- |
| `packy audit --json` | `packy-audit` |
| `packy list --json` | `pack-list` |
| `packy doctor --json` | `doctor` |
| `packy show PACK --json` | `pack-show` |
| global Pack preview | `pack-lifecycle-preview` |
| successful global Pack apply | `pack-lifecycle-apply` |
| global Pack failure | `pack-lifecycle-failure` |
| `packy status --json` | `pack-status-overview` |
| targeted global status | `pack-status` |

Project lifecycle output is a newline-delimited stream because one command can
emit a preview and an apply result. Installation reports shared project state;
activation reports personal runtime state.

The Pack audit report is a deterministic, read-only composition of existing
health meanings. Its checks retain their owning severity: informational
unknown readiness does not become a warning, warnings do not fail automation,
and confirmed failures produce a complete report before the command exits
non-zero. Project verification remains portable and never includes personal
activation or controlled-check evidence. Projects outside a Git worktree and
Git projects without a Pack contract are reported as informational states.

## Current-state contract

Pack reports describe the requested Pack, surface, selected resource closure,
planned actions, blockers, installed receipt, projection health, and readiness
needed for the current operation. Project reports keep direct intent separate
from generated receipts.

Status reports expose controlled runtime evidence as `unknown`, `current`, or
`stale`. Current evidence carries only its positive or negative result,
observation time, and validity identity; raw host output, credentials, and
secret material are never recorded or emitted.

Arrays representing sets use their schema-defined deterministic order. Arrays
representing work preserve execution order.

Pack show v7 exposes `catalog_state` as `current` when describing a Pack in the
selected Catalog Snapshot, or `retained` when an installed Pack is resolved
from its retained snapshot after withdrawal from the selected catalog. Its
`catalog_identity` identifies the representative Pack metadata within an
applicable snapshot, while every `surface_contracts` entry carries the exact
Pack identity for that surface's current or retained receipt. This keeps
surface contracts truthful when retained surfaces reference different versions.
The
`resource_inventory` is the domain-owned descriptive list of every Pack
resource. Each entry includes its identity, purpose, role, direct dependencies,
and relevant notices; entries and relationships use canonical resource-identity
order. Lifecycle and status resource graphs retain their operational selection
semantics.

## Redaction

Reports never include action payload contents, credentials, authentication
material, or MCP environment values. Environment-bearing command arguments keep
the key and replace the value with `<redacted>`. Paths that would disclose a
real home or project root use their documented placeholders.

Global status v13 preserves the applied identity and selected resource identities
in `intent`, separately from catalog-current `pack_version`. When the installed
manifest is no longer available, `historical_evidence.available` is false and
its message explains the unavailable resource, dependency, and contract facts.
Projection health still compares fresh surface observations with the exact
receipt digests. Read-only inspection never advances the receipt version.
Active receipted surfaces remain inspectable even when the current catalog no
longer supports them. A newer catalog version is distinct from an applicable
update: human output, Doctor, and the TUI do not recommend an update on a
surface the current catalog has retired.

Global lifecycle v13 exposes `contract_diff.baseline_available`. When the applied
Pack version's contract cannot be reconstructed, it is false and
`unavailable_reason` is `historical_contract_unavailable`. All four classification
arrays are empty because added, changed, removed, and retained classifications
are unavailable; those empty arrays do not assert that the contract is unchanged.
Human and TUI previews explain the same limitation. The update's target resources,
planned actions, and readiness remain available independently of the comparison.
When the installed version matches the current catalog, Packy reconstructs the
baseline using the receipt's selected resources. Available comparisons set
`baseline_available` to true and omit `unavailable_reason`.

## Selected resource definitions

Show v7 and global status/lifecycle v13 expose `resource_definitions` in the
surface lifecycle contract. Project installation/update preview v4 exposes the
same domain facts at the top level. Each entry preserves the logical resource
identity and identifies its `surface`, `definition` (`common` or
`surface_variant`), effective Pack-relative `source` when applicable, imported
`origin` when present, and selected legal `notices`. CLI and TUI explanations
use those same facts. Excluded resources do not appear as selected definitions.
Preview definitions follow the selected closure; receipts continue to seal
canonical identities and actual projected bytes without persisting provenance.
Historical schema generations remain immutable documentation; producers emit
only their current generation. Project receipt and lock schemas are unchanged.
