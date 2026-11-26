# Adopt a new catalog generation

Packy does not convert installations created by an incompatible generation.
This includes retired `codex` and OpenCode surface contracts, direct MCP/hook/command
projections, the legacy Installed Source model,
and Catalog Snapshot schema v1/v2. Adopt the current catalog with a clean,
explicit handoff. Keep the previous Packy binary and every source directory or
retained snapshot it references until the handoff is complete.

The current engine accepts only `agents` and `claude` surfaces. Retired surface
values, bindings, capabilities, project intent, receipts, and locks are rejected
rather than translated. A complete catalog containing retired declarations is
invalid; Packy does not silently load only its compatible Packs.

This procedure changes only Packy-owned installations and receipts. It does
not convert or delete source directories, personal files, credentials, Engram Memory, or
configuration owned by Codex, Claude Code, OpenCode, or another tool.

## Retired Codex and OpenCode surface installations

Before replacing a binary using `codex` or `opencode` surfaces, use it to inventory and
preview removal of its global installations, personal project activations, and
shared project installations in every affected worktree. Preserve the old
binary and its referenced content until all affected receipts are removed.
The replacement binary cannot read or remove those retired contracts.

Do not move or delete a skill or configuration directory wholesale. Earlier
OpenCode installations may share `.agents/skills` projections with Codex;
other directories can contain unmanaged user files. Remove every affected
receipt through the previous binary and let its ownership and drift rules
decide which paths can be removed. Retained snapshots and unrelated host
configuration remain untouched.

Only then upgrade and explicitly reinstall desired Packs on the remaining
supported surfaces. No retired-surface reinstall, alias, automatic state converter,
or old-root cleanup is provided.

The coordinated rollout is engine-first, then complete catalog validation and
publication against the released immutable engine revision. Never publish a
mixed-generation snapshot. Catalog Refresh is explicit and does not update
existing installations; their receipts retain their referenced snapshots. These instructions
do not assert that either release or catalog publication has already happened.
Do not perform the handoff until that compatible engine/catalog pair is
available.

## Retired Claude runtime projections

Use the same previous-binary handoff for Claude installations containing direct
MCP, hook, command, or obsolete lifecycle resources. The new contract keeps
native instructions, skills, and reviewed custom agents; it does not read or
remove retired resource receipts or convert commands into skills. Preview
removal with the binary that owns those receipts before replacing it, preserving
unrelated `.claude` configuration and the mixed user store.

## Catalog store layout

Packy keeps each registered catalog's selection and retained Catalog
Snapshots in its own directory,
`~/.local/share/packy/catalog/v2/<owner>/<repo>`. The official catalog is
`catalog/v2/yersonargotev/packy-catalog`. Packy does not read the earlier
single-catalog layout, `catalog/selected.json` and `catalog/snapshots`,
and never treats it as the official catalog. A command that reads the catalog
and finds only the earlier layout fails and names this guide; `packy init` and
`packy catalog refresh` write the current layout and succeed.

Complete steps 1 and 2 with the previous Packy, which still reads the earlier
layout. In step 3, `packy init` writes the current layout next to the earlier
one and leaves the earlier files unchanged. Packy never moves or deletes them.
Archive them under your own retention policy after no receipt of the previous
Packy references them.

## Catalog-bound receipts and locks

Every installed Pack receipt names the catalog it came from. The `pack` object
in `~/.packy/packs.json` and in each project's `packy.lock.json` records
`catalog_repository` next to `catalog_snapshot`:

```json
"pack": {
  "id": "<pack>",
  "version": "<version>",
  "catalog_repository": "yersonargotev/packy-catalog",
  "catalog_snapshot": "<commit>"
}
```

Packy reads only receipts at `schema_version` 2, project locks at
`schema_version` 2, and personal project activations at `schema_version` 2.
It rejects an earlier `packs.json` that still holds receipts, an earlier
`packy.lock.json`, or an earlier personal project activation with an error that
names this guide. An earlier `packs.json` with no receipts, which the previous
Packy leaves after the last deactivation, loads as empty, and the next change
rewrites it at `schema_version` 2. Packy leaves the rejected file
unchanged and never reads an earlier receipt as official-catalog state. No
converter exists.

To adopt catalog-bound receipts, follow steps 1 to 4 with these checks:

- In step 2, use the previous Packy to deactivate every global and personal
  project activation and to uninstall every shared project installation. The
  previous Packy removes its receipts and the project's `packy.lock.json`
  entries.
- In step 3, confirm that `packy status` and `packy status --project` exit 0
  with the current Packy before you reinstall anything.
- In step 4, reinstall the Packs. Open `~/.packy/packs.json` and
  `packy.lock.json` and confirm that each receipt names its
  `catalog_repository`.
- Commit the new `packy.lock.json` so that other workstations reinstall from
  the same catalog and snapshot.

## 1. Inventory with the previous Packy

Before replacing the binary, enter each affected Git worktree and record the
old installation state:

```sh
packy list
packy status
packy status --project
```

Keep old source directories and retained snapshots unchanged. An installed
Pack can need its referenced content to inspect, preview, deactivate, or
uninstall its receipt. There is no automatic converter, downgrade, or cleanup
command.

## 2. Preview and remove old-model installations

Still using the previous Packy binary and its referenced content, handle every
Pack and surface reported by the inventory. Preview each operation first:

```sh
# Global activation
packy deactivate <pack> --surface <surface> --dry-run
packy deactivate <pack> --surface <surface>

# Personal activation for the current project
packy deactivate <pack> --surface <surface> --project --dry-run
packy deactivate <pack> --surface <surface> --project

# Shared project installation; run only after personal deactivation
packy uninstall <pack> --surface <surface> --dry-run
packy uninstall <pack> --surface <surface>
```

Repeat `packy status` and `packy status --project`. Do not replace the binary
until the old global activation, project personal activation, and project
installation receipts are gone. If Packy reports drift or missing content,
stop and restore the referenced source or snapshot, or resolve the reported
ownership conflict; do not delete paths manually.

## 3. Replace Packy and initialize the current catalog

Only after the previous Packy reports a clean handoff may you replace or
upgrade the binary. Then initialize its verified current Catalog Snapshot:

```sh
brew upgrade yersonargotev/tap/packy
packy init
packy list
```

`packy init` acquires current catalog content. It does not import old receipts,
convert old sources, or remove personal data or foreign host configuration.

## 4. Reinstall and reactivate explicitly

Recreate only the installations you still want, using the current Packy:

```sh
# Global activation
packy activate <pack> --surface <surface> --dry-run
packy activate <pack> --surface <surface>

# Shared project installation
packy install <pack> --surface <surface> --dry-run
packy install <pack> --surface <surface>

# Separate personal activation for that project
packy activate <pack> --surface <surface> --project --dry-run
packy activate <pack> --surface <surface> --project
```

Finish with `packy status` and `packy status --project`. Once no retained
receipt references old catalog content, you may archive the source directory
or snapshot according to your own retention policy. Packy never deletes it for
you.

Catalog withdrawal prevents new installation or activation from the selected
snapshot. It does not uninstall an existing receipt. Keep the snapshot or
source referenced by an existing receipt until you have inspected and
deactivated it. Correct defective published content by publishing and selecting
a newer Pack version; withdrawal never triggers an automatic uninstall or
downgrade.
