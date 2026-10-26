# Local plugin export

Export one reviewed Pack from a local retained Catalog Snapshot:

```sh
packy export example --surface agents --output ./example-plugin --json
packy export example --surface agents --skill guide --output ./guide-plugin
packy export example --surface agents --skill guide,helper \
  --snapshot <retained-snapshot-id> --output ./retained-plugin
```

Agents maps to [Agent Plugins 1.0.0](https://agent-plugins.org/specification).
This operation currently supports `agents` only; Claude plugin export is a
separate delivery. The Pack ID becomes the plugin name, and each selected
skill appears under `skills/<reviewed-name>/SKILL.md`. `plugin.json` declares
the canonical format schema, Pack name, version, and description.

Default selection includes all skills with an explicit reviewed compatibility
contract for this format and surface. `--skill` selects logical IDs within the
one named Pack. Both forms resolve effective surface definitions before adding
transitive same-Pack native `requires` dependencies and legal notices. Explicit
incompatible selections and incompatible required skills fail the entire export.
Native installation bindings and `on_demand` declarations never imply eligibility;
on-demand dependencies do not replace the native dependency graph.

The output parent must already exist. The output itself must be new or an empty
regular directory, separate from retained snapshots, Packy state, and managed
client directories. Linked destinations, links in source trees, special files,
path escapes, collisions, invalid authored references, and nonempty destinations
are rejected. Packy validates the complete staged package before one atomic
rename publishes it. Failure leaves the destination unchanged and removes staging.
If writing the final report fails after publication, the command exits nonzero;
the completed package may already exist. Treat only a complete success report
and zero exit status as successful automation evidence.

Export stays offline and never refreshes or acquires a catalog implicitly. It
does not execute content, create installation receipts, change client settings,
register agents, or install/enable the plugin. Native clients own installation
and enablement. A generated package proves file validity, not client loading,
authorization, or model-backed execution.

## Reviewed format contract

Pack schema v3 accepts `plugin_exports` only on skills and their surface variants.
The array is sorted and unique by format. Omission or an empty array declares no
eligibility; an explicitly empty variant replaces inherited eligibility.
Every object is closed and has required `format`, `name`, `prerequisites`,
`references`, and `roles` fields. Required arrays cannot be null.

```json
{
  "plugin_exports": [{
    "format": "agent-plugins",
    "name": "guide",
    "source": "skills/plugin-guide",
    "prerequisites": ["Python 3.12 or newer"],
    "references": [
      {
        "id": "helper-path",
        "file": "SKILL.md",
        "target": "skill:helper",
        "kind": "path"
      }
    ],
    "roles": []
  }]
}
```

`name` is the skill identity and directory name: lowercase kebab-case, at most
64 characters. Packy changes only the frontmatter name scalar, preserving other
reviewed metadata bytes and the complete skill tree, including optional
`agents/openai.yaml`, scripts, references, assets, and executable modes.
The entry must satisfy the Agent Skills metadata contract.

Optional `source` names an explicitly reviewed Pack-relative skill tree for the
format. Omission uses the effective native skill source. A format adaptation
of imported content requires its own explicit `origin`; normal exact-copy versus
adapted provenance and notice validation applies. Format source trees belong to
the Declared Pack Closure and immutable snapshots. The selected native
`requires` and `notices` edges still govern their closure.

References are sorted by unique `id`. In the declared skill-relative `file`,
Packy replaces only the exact authored `{{packy.export:<id>}}` marker:

- `kind: "name"` resolves a selected skill's reviewed exported name.
- `kind: "path"` resolves a relative path from that file to a selected skill
  directory or a bundled asset/notice file.
- `kind: "invocation"` requires an explicit reviewed `value` string. Agent
  Plugins defines no universal client invocation syntax; Packy never invents one.

A target must be self, a direct native dependency, or a declared notice.
References to missing files, absent markers, unavailable targets, and unresolved
Packy markers fail export. Other prose is preserved without inferred rewriting.
Append a reviewed `/SKILL.md` suffix when a path reference should name a skill's
entry rather than its directory. Authors must review literal relative references
against the final package layout; undeclared prose is never automatically adapted.

Assets and notices retain their Pack-relative paths inside the package root.
`packy-provenance.json` records the retained snapshot, Pack/version, selected
resource provenance, license/attribution, and reviewed role resolutions. It does
not include unrelated instruction content or native role definitions. Repeated
exports with identical retained inputs have identical file bytes, modes, and
`content_sha256`, independent of output directory.

## Required custom roles

Native custom-agent definitions are never copied or registered. Each direct
required `agent:<id>` edge from a skill needs one reviewed `roles` entry:

```json
{
  "resource": "agent:reviewer",
  "resolution": "external",
  "description": "The consuming client must provide the reviewed reviewer role"
}
```

`external` discloses the description in report prerequisites; Packy does not
assert it is satisfied. Alternatively, `adapted` records why an explicit
format-specific `source` safely addresses the role without native registration.
That source and the explanation must be reviewed; the engine does not infer or
perform a runtime adaptation. Missing role resolutions fail complete export.
Native dependencies and legal notices of the role remain in the selected
closure, even though the definition itself is omitted. Unsupported non-skill
operational dependencies, including instruction contributions, fail export.

## Reports and isolated acceptance

The version-1 [`plugin-export` schema](../schemas/cli/v1/plugin-export.schema.json)
is authoritative. Success reports identify the retained snapshot, Pack/version,
surface/format, selected roots, complete native resource closure, plugin name,
absolute output directory, content digest, and prerequisites. Failures report
`result: "failed"` and an actionable `error.code`/`error.message`. Prerequisites
remain declarations rather than configured, authorized, or usable evidence.

The [local candidate seam](catalog-project.md#exercise-local-candidates) supports
export with a destination outside its owned workspace:

```sh
packy catalog candidate --snapshot /absolute/assets \
  --workspace /absolute/new-workspace -- export example \
  --surface agents --output /absolute/example-plugin --json
```

Synthetic complete snapshots can prove engine behavior before the compatible
production catalog is published. Candidate labels do not establish publication
authority. Official catalog adaptation and publication follow their own delivery
and released-engine prerequisites.
