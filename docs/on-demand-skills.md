# On-demand Skill Access

Obtain a reviewed skill without installing or activating its Pack:

```sh
packy init                          # explicit initial acquisition, requires network
packy get skill example:guide --surface codex
packy get skill example:guide --surface claude --json
```

`example:guide` is illustrative. Use `packy list` and `packy show <pack>` to
inspect your local catalog. The skill must explicitly declare reviewed on-demand
compatibility; a native binding or an installed copy is insufficient. Host
selection is mandatory. This slice supports Codex and Claude Code, using their
respective Effective Resources. OpenCode is unsupported for this operation.

Retrieval uses local retained data, works outside Git, and does not require the
skill's script runtimes. It does not refresh, download, execute scripts, install
tools, or alter project files, installed copies, receipts, activation, or permanent
instructions. It reports known prerequisites without verifying them. File
availability does not establish native registration or successful execution.

Read the returned `entry_document`, resolve supporting references from
`resource_root`, check prerequisites, and follow the skill's own procedure and
approval requirements. Resource locations contain immutable Packy-owned data;
choose the user's target project and separate writable scratch/output locations
when following the procedure. Do not run scripts with the source or resource
directory implicitly treated as the target project.

Catalog Refresh is explicit:

```sh
packy catalog refresh
packy get skill example:guide --surface codex --snapshot <returned-snapshot-id> --json
```

A new independent request follows the selected local snapshot. `--snapshot`
reuses exactly one retained identity; missing or invalid content fails without
falling back to another revision. Previously returned resource paths survive
refresh. No automatic expiration or eviction applies. Manual deletion can
invalidate retained resources. Modified or incomplete prepared trees fail on
reuse rather than being silently overwritten.

## Reviewed catalog contract

A schema v3 skill resource may declare:

```json
"on_demand": {
  "protocol": "skill-tree-v1",
  "prerequisites": ["git repository", "python3"],
  "entry_document": "ACCESS.md",
  "reference_files": ["ACCESS.md", "references/procedure.md"],
  "dependencies": [{"type": "skill", "target": "other:grilling"}]
}
```

Prerequisites are a sorted, unique, non-null array of nonempty declarations;
`[]` means no additional skill-specific prerequisites. A surface variant can replace the complete
object. Results combine these declarations with the Pack's known
`external_requirements`, sorted and deduplicated. Only selectable Packs are
available. The Catalog maintainer reviews compatibility, relative paths, procedure
and approvals, required assets, legal closure, and known requirements. Packy
never infers dependencies or rewrites arbitrary Markdown. Supporting file
resources use existing `requires` asset edges and `notices` references. Their
paths remain Pack-relative, including references outside the skill directory.
The prepared copy includes the reviewed manifest for provenance and declarations;
it is not an independently installable Pack or a new catalog.

On-demand dependencies use the optional `dependencies` array of typed
`{"type": "skill", "target": "<pack>:<skill>"}` declarations, sorted uniquely
by qualified target. Only `skill` is supported; null and unknown vocabulary are
invalid. Targets can belong to any selectable Pack in the same complete retained
Catalog Snapshot. A surface variant replaces the complete `on_demand` object,
so its dependency graph is resolved after host selection.

Same-Pack native skill dependencies continue to use direct `requires: ["skill:<logical-id>"]`
edges. Preparation includes the complete transitive skill, asset, and notice
closure across Packs after host selection. Each Pack retains its own source,
manifest provenance, version, notices, and supporting files. The complete
procedure is published atomically under preparation protocol `v2`. Every skill needs a supported host binding and
reviewed compatibility. Missing, cyclic, excluded, and incompatible dependencies
fail without a successful partial result. Repeated and diamond dependencies share
one complete prepared tree with deterministic references and ownership.

`entry_document` is optional and defaults to `SKILL.md`. A native template or
Claude composite skill must supply a distinct reviewed standalone entry document
inside its source tree, preserving its native instructions. `reference_files` is
an optional sorted, unique list of skill-relative Markdown files. Only these files
render the explicit token `{{packy.access:grilling}}` into a URL-escaped relative Markdown path to the
effective prepared entry for `skill:grilling` in the author's own Pack.
`{{packy.access:other:grilling}}` explicitly identifies another Pack's skill.
Qualified references must name self, a direct same-Pack `requires` edge, or an
explicit `on_demand.dependencies` target. Duplicate skill names across Packs
remain distinct. A cross-Pack declaration grants resource access only: it never
adds a native installation dependency or ownership of foreign files. For example, author `[Read grilling]({{packy.access:grilling}})`
and tell the consuming agent to read and follow that document, rather than assume
a native Skill tool registration. Tokens in nested reviewed documents resolve
relative to those documents, including when the target has a host variant.

Malformed or undeclared references, missing listed files, and missing entry
documents fail during complete-catalog validation, construction, and consumption, including
in unrelated Packs. Omitted optional fields keep dependency-free trees
usable. An explicit null is invalid. Other bytes and modes remain unchanged,
including scripts, host invocations, native `{{packy.skill:...}}` tokens, and
unlisted documents. Prepared reuse verifies rendered bytes as well as untouched
files; it fails on drift without overwriting modifications. Catalog Snapshot
sources and native installation/alias semantics remain unchanged. Engine release
must precede Catalog Publication using this vocabulary.

## Output and errors

Readable success lists identity, version, surface, Catalog Snapshot, entry
document, resource root, and prerequisites, with an explicit availability-only
explanation. `--json` emits one document to stdout with `schema_version: 1`,
`report: "skill-access"`, and `result: "prepared"` or `"failed"`.
See [the JSON schema](../schemas/cli/v1/skill-access.schema.json).

Success requires `qualified_skill`, `pack_version`, `surface`, `snapshot_id`,
`entry_document`, `resource_root`, `prerequisites`, and `dependencies`.
The sorted `dependencies` array lists the complete transitive prepared skill set
(excluding the root); each item carries the same seven identity/location/requirement
fields as the root. All entries retain the initiating snapshot and surface.
Dependency-free retrieval returns `[]`. Paths are absolute and
intentionally expose the usable local locations. No native-registration,
runtime-readiness, or workflow-success claim is included.

Failure exits nonzero and contains only the report envelope and
`error: {"code": "...", "message": "..."}`. The message explains the next action;
stderr may also contain a readable diagnostic. Argument errors honor `--json`.

| Code | Action |
| --- | --- |
| `invalid_arguments` | Use one qualified skill and supported flags. |
| `invalid_identity` | Use lowercase kebab-case `<pack>:<skill>`. |
| `unsupported_surface` | Select `codex` or `claude` and a skill supporting it. |
| `catalog_unavailable` | Explicitly acquire with `packy init` or `packy catalog refresh`; inspect invalid local content. |
| `snapshot_unavailable` | Restore the exact retained revision; do not substitute a newer one. |
| `unknown_skill` | Inspect `packy list` and `packy show <pack>`. |
| `incompatible_resource` | Ask the Catalog maintainer for a reviewed standalone on-demand tree. |
| `preparation_failed` | Inspect filesystem access or prepared-tree drift; preserve sources and use writable scratch/output elsewhere. |

Local authoring uses the existing candidate interface, retaining its explicit
unauthenticated provenance diagnostic:

```sh
packy catalog candidate --snapshot /absolute/assets --workspace /absolute/new-workspace -- get skill example:guide --surface codex --json
```

Candidate workspaces own their disposable project and HOME. They never select an
official catalog or weaken installation's Git requirement.
