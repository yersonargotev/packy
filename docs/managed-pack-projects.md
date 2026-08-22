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
  Project root. An origin path cannot select a `.git` path component (with any
  capitalization) because checkout metadata is not part of the declared
  commit tree; tracked files such as `.gitmodules` remain valid content.
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

The end-user `bundle/` is an inert data boundary, including when admitted
resources contain `.go` or `*_test.go` files. Its nested `go.mod` prevents root
`go list`, `go test`, `go vet`, and golangci-lint `./...` patterns from
discovering or executing arbitrary admitted Go content.

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
release assertion, canonical `pack-v<version>` tag, tag ref type and SHA, the
complete ordered annotated-tag object chain, peeled commit, root tree, manifest
and closure digests, and the complete sorted file index. `tag_objects` is
ordered from the tag ref toward the commit. Each entry pins its object SHA,
immediate target SHA, and target type. A lightweight tag has `tag_ref_type` set
to `"commit"`, a `tag_ref_sha` equal to `commit`, and an empty `tag_objects`
array. An annotated tag has `tag_ref_type` set to `"tag"`, a `tag_ref_sha` equal
to the first entry's SHA, each entry targets the next tag object, and the final
entry targets `commit`. An existing record is never replaced, and a new record
is linked into its final path only after its complete contents have been written
and synchronized.

The current bundled catalog remains valid while its seven Packs migrate via
higher immutable Managed Pack releases. This transition does not add a schema
selector to the catalog loader or retain a permanent multi-version loader.

## Promotion

Packy maintainers promote exactly one immutable release from the repository
root with the repository-private adapter:

```sh
go run ./internal/tools/promotepack <pack-id>@<version>
```

The adapter is intentionally outside `cmd/packy` and Packy's release artifacts.
The parent coordinator first creates a temporary Packy snapshot whose Git remote
contains no embedded credentials. A fresh credential-free prepublication process
then fetches the registered Managed Pack Project release and exact origin commits,
uses a separate credential-free, no-network worker to validate the inert local
trees and seal the Declared Pack Closure, materializes the candidate, and runs
every repository admission gate. After that process exits, a distinct fresh
least-privilege mutation process receives only the sealed Candidate and publishes
or adopts an automation-owned ready pull request through normal fast-forward Git
operations. Every protocol file is identity- and digest-bound inside one temporary
owner-only directory, which the parent removes after the result. Project or origin
hooks, scripts, tests, builds, and binaries are never executed.

The command reports exactly one proposal, deterministic no-change, or typed
policy rejection. Promotion never replaces an admission record, never adopts a
human-edited branch or pull request, and never force-pushes automation state.
