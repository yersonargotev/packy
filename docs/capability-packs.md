# Capability Packs

Capability Packs are opt-in, Git-reviewed content installed by Packy. Catalog
inspection and `--dry-run` are read-only. Mutation requires an explicit Pack,
CLI surface, and operation. Supported surfaces are Codex, OpenCode, and Claude
Code.

The public [`yersonargotev/packy-catalog`](https://github.com/yersonargotev/packy-catalog)
Catalog Project is the canonical authoring source. Use `packy list` for the
Pack IDs and versions embedded in the current binary, and `packy show <pack>`
for one Pack's purpose, supported surfaces, resources, and external
requirements. The generated [Pack catalog](packs/index.md) provides the same
embedded manifest-backed inventory for browsing on GitHub. Pack versions are
independent of the Packy binary version.

## Inspect and activate

```sh
packy list
packy show matty
packy activate matty --surface codex --dry-run
packy activate matty --surface codex
packy status matty --surface codex
```

Activation without `--resource` selects the complete Pack. Repeat `--resource`
to select explicit roots; the manifest supplies their intra-Pack dependency
closure:

```sh
packy activate matty --surface codex \
  --resource skill:code-review \
  --dry-run
```

External requirements such as Engram are host prerequisites, not relationships
between Packs. Multiple Packs can coexist, but each lifecycle command acts on
one Pack and installed Pack receipt.

## Controlled runtime checks

When Packy cannot observe host runtime behavior, preview and perform the
reviewed instructions separately from activation, then explicitly record the
observed result:

```sh
packy check orchestrate --surface codex --dry-run
packy check orchestrate --surface codex --result positive
packy status orchestrate --surface codex --require usable
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
- update only to the current bundled Pack version;
- remove only unchanged paths owned by the selected receipt; and
- keep other Pack receipts independent.

Preview always runs before application. Use `--force` only after inspecting a
drift report; its authority is limited to paths in the targeted receipt.

## Authoring the catalog

The [Catalog Project](catalog-project.md) canonically authors every Pack at
`bundle/packs/<pack-id>/pack.json` together with its reviewed bundle-relative
resources. There is no separately maintained Pack registry and ordinary
content changes do not require per-Pack releases or promotion into Packy.

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
reviewed relative `source`; Codex and OpenCode translate it into an independently
owned marked contribution in the project's `AGENTS.md`. The
`opencode-primary-prompt` capability carries typed `primary_prompt` data with
the same identity and source constraints; OpenCode translates it into the
global primary instruction document and its workstation configuration reference.
Project guidance remains a separate `project-instruction` contribution. Unknown capability
types, missing typed data, and extension fields are rejected during admission.

The typed `external-executable-acquisition` capability explicitly enables one
supported acquisition flow for a declared external requirement on that
surface. Its reviewed data names the external tool. Engram declares the
capability on its Codex skill so Packy can offer the supported Homebrew install
when the executable is absent. External requirements remain the single source
of tool readiness: every declared name is observed generically through PATH,
and no tool receives acquisition merely because of its name. Acquisition never
authorizes a tool-owned host setup command.

Catalog Snapshot publication remains a separate mechanical continuation of a
reviewed content merge. It is not performed by local authoring validation.

## Project use

Project installation writes human-authored intent to `packy.json` and generated
receipts to `packy.lock.json`. See the [project Pack lifecycle](project-pack-lifecycle.md).
