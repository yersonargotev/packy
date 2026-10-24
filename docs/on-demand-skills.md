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
  "prerequisites": ["git repository", "python3"]
}
```

Prerequisites are a sorted, unique, non-null array of nonempty declarations;
`[]` means no known prerequisites. A surface variant can replace the complete
object. The Catalog maintainer reviews compatibility, relative paths, procedure
and approvals, required assets, legal closure, and known requirements. Packy
never infers dependencies or rewrites arbitrary Markdown. Supporting file
resources use existing `requires` asset edges and `notices` references. Their
paths remain Pack-relative, including references outside the skill directory.
The prepared copy includes the reviewed manifest for provenance and declarations;
it is not an independently installable Pack or a new catalog.

Skill dependencies and native rendering/composition are rejected in this first
slice. Catalog authors must supply a reviewed standalone tree rather than a
native template or a Claude composite skill. Existing installation behavior is
unchanged. Engine release must precede Catalog Publication using this field.

## Output and errors

Readable success lists identity, version, surface, Catalog Snapshot, entry
document, resource root, and prerequisites, with an explicit availability-only
explanation. `--json` emits one document to stdout with `schema_version: 1`,
`report: "skill-access"`, and `result: "prepared"` or `"failed"`.
See [the JSON schema](../schemas/cli/v1/skill-access.schema.json).

Success requires `qualified_skill`, `pack_version`, `surface`, `snapshot_id`,
`entry_document`, `resource_root`, and `prerequisites`. Paths are absolute and
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
| `skill_dependencies_unsupported` | This slice cannot prepare skills requiring other skills. |
| `preparation_failed` | Inspect filesystem access or prepared-tree drift; preserve sources and use writable scratch/output elsewhere. |

Local authoring uses the existing candidate interface, retaining its explicit
unauthenticated provenance diagnostic:

```sh
packy catalog candidate --snapshot /absolute/assets --workspace /absolute/new-workspace -- get skill example:guide --surface codex --json
```

Candidate workspaces own their disposable project and HOME. They never select an
official catalog or weaken installation's Git requirement.
