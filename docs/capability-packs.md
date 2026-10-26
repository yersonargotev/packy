# Capability Packs

Capability Packs are opt-in, Git-reviewed content installed by Packy. Catalog
inspection and `--dry-run` are read-only. Mutation requires an explicit Pack,
CLI surface, and operation. Supported surfaces are Agents and Claude.

The public [`yersonargotev/packy-catalog`](https://github.com/yersonargotev/packy-catalog)
Catalog Project is the canonical authoring source. Use `packy list` for the
Pack IDs and versions in the selected Catalog Snapshot, and `packy show <pack>`
for one Pack's purpose, supported surfaces, resources, and external
requirements. Pack versions are independent of the Packy binary version.
Once a Packy version with Catalog Snapshot support is installed, compatible
content-only publications require no Packy upgrade: `packy catalog refresh`
discovers them without changing active Packs. Update or activate one Pack
explicitly to apply newly available content.

## Inspect and activate

```sh
packy list
packy show matty
packy activate matty --surface agents --dry-run
packy activate matty --surface agents
packy status matty --surface agents
```

Activation without `--resource` selects the complete Pack. Repeat `--resource`
to select explicit roots; the manifest supplies their intra-Pack dependency
closure:

```sh
packy activate matty --surface agents \
  --resource skill:code-review \
  --dry-run
```

External requirements such as Engram are host prerequisites, not relationships
between Packs. Multiple Packs can coexist, but each lifecycle command acts on
one Pack and installed Pack receipt.

For skill variants, review the [native roots and host-discovery limits](skill-discovery.md).
Same-surface duplicate native names can block installation even when their
physical targets differ. Installations using retired contracts require the
[clean adoption procedure](catalog-adoption.md).

## Explicit custom-agent projections

Agents skills use `~/.agents/skills` globally and `.agents/skills` in a project.
Agents instruction contributions use project `AGENTS.md` only; global instruction
selection fails because there is no universal global instruction destination.
The `codex` surface value is rejected, not aliased to `agents`.

An Agents agent binding must declare its concrete client and supported scope:

```json
"client_projection": { "client": "codex", "scopes": ["global"] }
```

Required agents remain in the selected resource dependency closure. Select their
client explicitly when activating that closure:

```sh
packy activate <pack> --surface agents --client codex --resource skill:<workflow> --dry-run
packy activate <pack> --surface agents --client codex --resource skill:<workflow>
```

The current supported Codex projection is global native TOML under
`~/.codex/agents`. Project Codex roles and other client/scope combinations fail
before mutation. Preview identifies the client and scope, and the selection is
retained for update and removal. Roles participate in normal ownership,
collision, and drift protection; Packy does not configure unrelated Codex
settings, MCP servers, global instructions, or command degradations.

A client selection is valid only for a global Agents closure containing a
required native Codex role. Project selections and ordinary skill-only global
selections reject `--client` before mutation. Ordinary Agents skills do not
create `.codex` content. Complete skill trees retain scripts, executable modes, references,
legal/supporting content, and optional `agents/openai.yaml` metadata. That
metadata is part of a skill, not a custom-agent registration. A generated role
file establishes configured content only; it does not prove authorization or
successful runtime delegation.

## Controlled runtime checks

For an explicitly selected Codex role closure whose client behavior Packy cannot
observe, preview and perform the
reviewed instructions separately from activation, then explicitly record the
observed result:

```sh
packy check orchestrate --surface agents --client codex --dry-run
packy check orchestrate --surface agents --client codex --result positive
packy status orchestrate --surface agents --require usable
```

The preview identifies the exact Pack version, CLI surface, selected resource
closure, projection revision, adapter version, observable host version, and
instructions. A positive current result can satisfy strict usability; a
negative result fails it, and an identity change makes the prior result stale.
Evidence has no time-to-live while its identity is unchanged.

Controlled-check evidence lives only in `~/.packy/controlled-checks.json`. It
is personal workstation state and never enters a Pack manifest, project
manifest, project lock, or Git-managed artifact.

## Installed Pack receipts

A successful activation records the current Pack version, surface, selected
resources, projected paths, and content digests. Packy uses that receipt to:

- stop before changing drifted Pack-owned paths;
- reject target collisions before mutation;
- update only to the Pack version in the selected Catalog Snapshot;
- remove only unchanged paths owned by the selected receipt; and
- keep other Pack receipts independent.

Preview always runs before application. Use `--force` only after inspecting a
drift report; its authority is limited to paths in the targeted receipt.

## Authoring the catalog

The [Catalog Project](catalog-project.md) canonically authors every Pack at
`packs/<pack-id>/pack.json` together with its reviewed Pack-relative
resources. Each Pack keeps one `pack.json` manifest. There is no separately
maintained Pack registry and ordinary content changes do not require per-Pack
releases or promotion into Packy.

1. Change one or more Pack manifests and their reviewed resources in one
   Catalog Project branch.
2. Preserve exact upstream commits, exact-copy or adapted relationships, and
   every required notice.
3. Increase the SemVer of each changed Pack and retain every unchanged Pack's
   version.
4. Run whole-catalog validation against the target branch before review.
5. Merge the reviewed content through the Catalog Project pull-request flow.

The manifest declares Pack identity, version, description, selectability,
surfaces, resources, bindings, intra-Pack dependencies, external requirements,
origins, notices, and concrete conflicts. Validation discovers all manifests,
builds every Declared Pack Closure deterministically, verifies exact copies and
safe paths, and evaluates the complete typed runtime-fitness matrix without
executing catalog content.

Every binding declares a non-null `capabilities` array. Most bindings use an
empty array. A binding that needs reusable host-native behavior selects only a
reviewed typed capability. The `project-instruction` capability carries
typed `project_instruction` data with a stable lowercase kebab-case `id` and a
reviewed relative `source`; Agents translates it into an independently owned
marked contribution in the project's `AGENTS.md`. Unknown capability types,
missing typed data, and extension fields are rejected during admission.

The typed `external-executable-acquisition` capability explicitly enables one
supported acquisition flow for a declared external requirement on that
surface. Its reviewed data names the external tool. Engram declares the
capability on its Agents skill so Packy can offer the supported Homebrew install
when the executable is absent. External requirements remain the single source
of tool readiness: every declared name is observed generically through PATH,
and no tool receives acquisition merely because of its name. Acquisition never
authorizes a tool-owned host setup command.

Catalog Snapshot publication remains a separate mechanical continuation of a
reviewed content merge. It is not performed by local authoring validation.

## Project use

Project installation writes human-authored intent to `packy.json` and generated
receipts to `packy.lock.json`. See the [project Pack lifecycle](project-pack-lifecycle.md).

## Reviewed native skill names and references

An author can opt a skill binding into deterministic rendering:

```json
{
  "type": "skill-template",
  "skill_template": { "naming": "pack" }
}
```

The generated default name is `<pack-id>-<binding-name>`. An explicit
user alias overrides that default. Template bindings declare the canonical
invocation: `$<name>` for the reviewed Agents binding and `/<name>` for Claude.
Preview and show output follow the resolved name, including explicit aliases. Choose `"naming": "native"` for a consumer
that retains its own name while referencing renamed dependencies. Packy renders
both kinds as regular, receipt-owned trees in global and project installations.
Other Packs' installations are preserved. Remaining target or native discovery
conflicts still block.

References are authored in reviewed source as `{{packy.skill:tdd}}`, with `tdd`
being the same Pack's logical skill ID. The source keeps its host's syntax around
the token: `$` for a Codex invocation, a quoted name for a skill-tool call, or
`../<token>/SKILL.md` for a sibling link. Each other referenced skill must be a
direct `requires` dependency so custom selections retain the target. Self
references are allowed. The renderer sets the native frontmatter `name` from the
skill's effective binding and preserves all non-directive bytes elsewhere.

The capability does not infer references from arbitrary text, replace shell
variables, resolve conflicting engineering policies, or modify Catalog Snapshot
bytes. Add it only after reviewing and declaring every semantic reference that
must follow the effective name. Older engines reject this new closed capability;
release the engine before publishing an adapted Catalog Snapshot. See
[skill discovery](skill-discovery.md) and
[ADR 0043](adr/0043-render-reviewed-skill-identities-and-references.md).
