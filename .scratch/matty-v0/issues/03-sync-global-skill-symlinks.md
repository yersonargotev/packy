Status: ready-for-agent

# Sync the global Matty skill bundle as symlinks

## Parent

.scratch/matty-v0/PRD.md

## What to build

Implement global skill bundle synchronization. Matty should discover the configured source skill directories, select all `engineering` and `productivity` skills plus `loop-me` and `wayfinder`, and expose them as managed symlinks under the sandboxed/global `~/.agents/skills` target.

## Acceptance criteria

- [ ] `matty install` creates symlinks for every chosen skill in the bundle.
- [ ] The bundle includes all skills from `engineering`, all skills from `productivity`, and selected in-progress skills `loop-me` and `wayfinder`.
- [ ] Existing non-Matty files or symlinks in `~/.agents/skills` are preserved.
- [ ] Re-running install/update is idempotent and does not duplicate or unnecessarily rewrite symlinks.
- [ ] `matty uninstall` removes only Matty-managed skill symlinks.
- [ ] Tests cover missing source skills, existing unmanaged paths, and dry-run behavior.

## Blocked by

- .scratch/matty-v0/issues/02-manage-matty-state-and-dry-run.md
