# Capability Packs

Capability Packs are opt-in, Git-reviewed content installed by Packy. Catalog
inspection and `--dry-run` are read-only. Mutation requires an explicit Pack,
CLI surface, and operation. Supported surfaces are Codex, OpenCode, and Claude
Code.

The selectable pack catalog currently contains `addy`, `argote`, `engram`, and `matty`.

| Pack | Version | Purpose |
| --- | --- | --- |
| `addy` | `1.0.0` | Addy agent skills. |
| `argote` | `1.0.1` | Engineering and communication guidance. |
| `engram` | `1.0.0` | Memory guidance and Engram integration. |
| `matty` | `1.0.0` | Engineering workflow skills. |

Pack versions are independent of the Packy binary version.

## Inspect and activate

```sh
packy pack list
packy pack show matty
packy pack activate matty --surface codex --dry-run
packy pack activate matty --surface codex
packy pack status matty --surface codex
```

Activation without `--resource` selects the complete Pack. Repeat `--resource`
to select explicit roots; the manifest supplies their intra-Pack dependency
closure:

```sh
packy pack activate matty --surface codex \
  --resource skill:code-review \
  --dry-run
```

External requirements such as Engram are host prerequisites, not relationships
between Packs. Multiple Packs can coexist, but each lifecycle command acts on
one Pack and installed Pack receipt.

## Installed Pack receipts

A successful activation records the current Pack version, surface, selected
resources, projected paths, and content digests. Packy uses that receipt to:

- stop before changing drifted Pack-owned paths;
- reject target collisions before mutation;
- update only to the current bundled Pack version;
- remove only unchanged paths owned by the selected receipt; and
- keep other Pack receipts independent.

Preview always runs before application. Use `--force` only after inspecting a
drift report; its authority is limited to paths in the targeted receipt.

## Authoring one Pack

Start from [the standard template](../bundle/pack-template/README.md):

1. Copy `bundle/pack-template` to `bundle/packs/<pack-id>`.
2. Add or edit reviewed content beneath the Pack directory.
3. Edit the one `pack.json` manifest.
4. Select the new Pack SemVer as the maintainer.
5. Run the focused validator:

   ```sh
   ./scripts/validate-pack-content.sh <pack-id>
   ```

The manifest declares Pack identity, version, description, selectability,
surfaces, resources, bindings, intra-Pack dependencies, external requirements,
concrete conflicts, and exclusions. The focused validator reports the Pack and
invalid field or resource directly.

No other generated catalog or version record is part of authoring. Review and
merge the manifest and content together through the normal GitHub pull-request
flow.

## Project use

Project installation writes human-authored intent to `packy.json` and generated
receipts to `packy.lock.json`. See the [project Pack lifecycle](project-pack-lifecycle.md).
