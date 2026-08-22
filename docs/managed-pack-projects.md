# Managed Pack Projects

A Managed Pack Project is one public repository that authors exactly one Pack.
Its root `pack.json` and the positively referenced resource roots are the full
authoring contract; unrelated repository files are outside the Pack.

## Contract

The current and only accepted project schema is
[`schemas/managed-pack/v1/pack.schema.json`](../schemas/managed-pack/v1/pack.schema.json).
Packy rejects unknown fields and any `schema_version` other than `1`. The
JSON Schema owns the closed wire shape and kind/type discriminators. The
Packy validator is the normative machine check for ordered sets, cross-field
references, filesystem types, provenance equality, and deterministic closure;
a schema-only pass is not complete project validation.

The manifest uses the same Pack resource, surface, capability, readiness, and
SemVer vocabulary as the reviewed catalog, with these Managed Pack additions:

- `origins` is a sorted catalog of immutable public External Source Project
  revisions. Each entry has a lowercase kebab-case `id`, an `owner/name`
  GitHub repository identity, a full lowercase commit object ID, and an
  optional descriptive `revision`.
- A resource with no `origin` is authored by the Managed Pack Project.
- A derived resource has one `origin` object containing the origin `id`, its
  normalized repository-relative `path`, and a whole-resource `relationship`
  of `exact-copy` or `adapted`. The origin path `.` names the External Source
  Project root. An origin path cannot select a `.git` path component because
  checkout metadata is not part of the declared commit tree; tracked files
  such as `.gitmodules` remain valid content.
- Every derived resource references at least one declared notice. A derived
  notice may link itself when that notice file carries its own terms.
  An `exact-copy` resource must have the same complete relative file set and
  bytes as its origin path. Any changed or additional byte makes the entire
  resource `adapted`.
- `exclusions` and `source_reference` are not part of this schema. The positive
  resource list defines Pack membership.

Resource sources use their final bundle-relative layout directly:

| Resource kind | Root |
| --- | --- |
| `agent` | `agents/` |
| `asset` | `assets/` |
| `command` | `commands/` |
| `instruction` | `instructions/` |
| `notice` | `notices/` |
| `skill` | `skills/` |

Typed instruction capabilities also reference `instructions/`. Resources that
do not own files, such as lifecycle and MCP server declarations, have no source
root.

## Declared Pack Closure

The Declared Pack Closure is `pack.json` plus the deterministic union of every
resource and typed-capability source root. Paths must be normalized and
repository-relative. A resource and its own typed capability may repeat the
same root, which contributes one union member. Roots owned by different
resources may not be equal, and distinct roots may not contain one another.
Every referenced path must exist and contain only directories and regular
files; absolute paths, traversal, symlinks, submodules, and special files are
rejected.

Packy sorts every closure file by slash-separated path and records its Git file
mode (`100644` or `100755`) and lowercase SHA-256. The closure digest is the
SHA-256 of these UTF-8 records in order:

```text
<path> NUL <mode> NUL <sha256> LF
```

The manifest digest is the SHA-256 of the exact root `pack.json` bytes. This
makes manifest, closure, and file-index identity deterministic without
executing project content.

## Preventive validation

Managed Pack Projects call Packy's reusable workflow before publishing an
immutable release:

```yaml
jobs:
  validate:
    uses: yersonargotev/packy/.github/workflows/managed-pack-validation.yml@main
    permissions:
      contents: read
```

For reproducible project controls, set the workflow's optional `packy_ref`
input to an immutable Packy commit. The workflow checks out public origins at
their declared commits and runs the same `internal/managedpack` validator that
Managed Pack Promotion consumes. It reads content but never runs project or
origin scripts, hooks, tests, builds, or binaries.

Packy maintainers can run the adapter directly:

```sh
go run ./internal/tools/managedpackvalidate --project /path/to/project
```

Tests and offline callers may repeat `--origin <id>=<local-root>` to supply an
already acquired exact origin tree.

## Registry and admission records

[`managed-packs/registry.json`](../managed-packs/registry.json) is Packy's
reviewed one-to-one mapping from Pack ID to public Managed Pack Project. It is
outside the end-user bundle and initially registers Addy, Argote, Engram,
Issue Delivery, Matty, Orchestrate, and pstack.

Promotion writes one Pack Admission Record to
`managed-packs/admissions/<pack-id>/<version>.json`. The v1 schema is
[`schemas/managed-pack/v1/admission-record.schema.json`](../schemas/managed-pack/v1/admission-record.schema.json).
Each append-only record pins repository and release numeric IDs, the immutable
release assertion, canonical `pack-v<version>` tag, tag object, peeled commit,
root tree, manifest and closure digests, and the complete sorted file index.
An existing record is never replaced, and a new record is linked into its final
path only after its complete contents have been written and synchronized.

The current bundled catalog remains valid while its seven Packs migrate via
higher immutable Managed Pack releases. This transition does not add a schema
selector to the catalog loader or retain a permanent multi-version loader.
