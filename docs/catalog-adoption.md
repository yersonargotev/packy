# Adopt a new catalog generation

Packy does not convert installations created by an incompatible catalog
generation. This includes the legacy Installed Source model, Catalog
Snapshot schema v1/v2, and the move from shared Codex/OpenCode skill roots to
native OpenCode roots. Adopt the current catalog with a clean, explicit handoff.
Keep the previous Packy binary and every source directory or retained snapshot
it references until the handoff is complete.

This procedure changes only Packy-owned installations and receipts. It does
not convert or delete source directories, personal files, credentials, Engram
Memory, or configuration owned by Codex, Claude Code, OpenCode, or another
tool.

## Surface variants and the previous shared skill root

The variant-aware engine uses Pack manifest and Catalog Snapshot schema v3.
Its OpenCode skill targets are `$XDG_CONFIG_HOME/opencode/skills` globally
(defaulting to `$HOME/.config/opencode/skills`) and `.opencode/skills` in a
project. Earlier installations can still own skills under `$HOME/.agents/skills`
and project `.agents/skills`, shared with Codex. Moving the new target does not
transfer or remove those earlier receipts.

Before replacing the engine or acquiring the incompatible snapshot, use the
previous engine to deactivate old global installations, deactivate personal
project activations, and uninstall affected project installations in every
worktree. Preserve the old engine and its referenced snapshots until those
steps complete. Do not move or delete `.agents/skills` wholesale: it can contain
Codex installations and unmanaged user skills. If both surfaces own a shared
projection, remove each affected receipt through the old engine and let its
ownership rules decide whether the shared path can be removed.

Only then upgrade and explicitly reinstall each desired surface. Native
OpenCode paths do not disable its compatibility discovery of Codex/Claude
skills. Review the new [host-discovery checks](skill-discovery.md); divergent
same-name intent can remain blocked after a clean handoff. An external-skills
environment flag on one launch is not proof that all launches are isolated.

The coordinated rollout is engine-first, then complete catalog validation and
publication against the released immutable engine revision. These instructions
do not assert that either release or catalog publication has already happened.
Do not perform the handoff until that compatible engine/catalog pair is
available. No automatic state converter or old-root cleanup is provided.

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
