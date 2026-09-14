# Adopt the Catalog Snapshot model

Packy does not convert installations created by the previous Installed Source
model. Adopt the Catalog Snapshot model with a clean, explicit handoff. Keep
the previous Packy binary and every source directory it references until the
handoff is complete.

This procedure changes only Packy-owned installations and receipts. It does
not convert or delete source directories, personal files, credentials, Engram
Memory, or configuration owned by Codex, Claude Code, OpenCode, or another
tool.

## 1. Inventory with the previous Packy

Before replacing the binary, enter each affected Git worktree and record the
old installation state:

```sh
packy list
packy status
packy status --project
```

Keep the old source directories unchanged. An installed Pack can need its
referenced source to inspect, preview, deactivate, or uninstall its receipt.
There is no automatic converter, downgrade, or cleanup command.

## 2. Preview and remove old-model installations

Still using the previous Packy binary and its referenced sources, handle every
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
installation receipts are gone. If Packy reports drift or a missing source,
stop and restore the referenced source or resolve the reported ownership
conflict; do not delete paths manually.

## 3. Replace Packy and initialize the current catalog

Only after the previous Packy reports a clean handoff may you replace or
upgrade the binary. Then initialize its verified Catalog Snapshot:

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
receipt references an old source directory, you may archive it according to
your own retention policy. Packy never deletes it for you.

Catalog withdrawal prevents new installation or activation from the selected
snapshot. It does not uninstall an existing receipt. Keep the snapshot or
source referenced by an existing receipt until you have inspected and
deactivated it. Correct defective published content by publishing and selecting
a newer Pack version; withdrawal never triggers an automatic uninstall or
downgrade.
