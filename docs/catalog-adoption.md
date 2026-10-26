# Adopt a new catalog generation

Packy does not convert installations created by an incompatible generation.
This includes retired `codex` and OpenCode surface contracts, the legacy Installed Source model,
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
publication against the released immutable engine revision. These instructions
do not assert that either release or catalog publication has already happened.
Do not perform the handoff until that compatible engine/catalog pair is
available.

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
