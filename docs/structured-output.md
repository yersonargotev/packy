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
| `packy show PACK --json` | `schemas/cli/v6/pack-show.schema.json` |
| global Pack status | `schemas/cli/v11/pack-status.schema.json` |
| global Pack lifecycle | `schemas/cli/v11/pack-lifecycle.schema.json` |
| project Pack lifecycle | `schemas/project/v1.0.0/` |

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

Pack show v6 exposes the current Managed Pack as `catalog_identity` and includes
`resource_inventory`, the domain-owned descriptive list of every Pack resource.
Each entry includes its identity, purpose, role, direct dependencies, and
relevant notices; entries and relationships use canonical resource-identity
order. Lifecycle and status resource graphs retain their operational selection
semantics.

## Redaction

Reports never include action payload contents, credentials, authentication
material, or MCP environment values. Environment-bearing command arguments keep
the key and replace the value with `<redacted>`. Paths that would disclose a
real home or project root use their documented placeholders.
