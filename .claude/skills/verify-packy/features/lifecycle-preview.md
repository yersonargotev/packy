# Lifecycle preview

Users inspect a proposed installation or activation before they consent to changes.

## Sub-features

- Preview project installation.
- Preview global activation.

## How to get to it (user POV)

Run `packy install <pack> --surface agents --dry-run` in a Git worktree or `packy activate <pack> --surface agents --dry-run`.

## Driving it with CLI

Preconditions: Doctor passed. Select a reviewed Catalog Snapshot containing the Pack in the sandbox. Use a disposable Git worktree for installation.

- In that worktree, run `"$PACKY_VERIFY_SCRATCH/packy" install <pack> --surface agents --dry-run`. Capture the preview, exit status, and planned projections. Compare the project and home trees before and after.
- Run `"$PACKY_VERIFY_SCRATCH/packy" activate <pack> --surface agents --dry-run`. Capture the preview and exit status. Compare the home and configuration trees before and after.
- Confirm neither preview creates receipts or projected resources. Report any other observed effect, including network access if the environment exposes it.

## Gotchas

- An empty sandbox lacks a selected catalog. Record this prerequisite rather than treating a rejected preview as proof of a successful one.
- The preview does not prove application or consent behavior.
