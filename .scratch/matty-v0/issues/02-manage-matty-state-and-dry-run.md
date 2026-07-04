Status: ready-for-agent

# Manage Matty state and dry-run planning

## Parent

.scratch/matty-v0/PRD.md

## What to build

Add Matty's global state model and dry-run planning behavior. Commands should be able to compute planned file writes, symlink changes, and external command invocations before applying them, and persist minimal Matty metadata in the global state file when not in dry-run mode.

## Acceptance criteria

- [ ] Matty reads/writes a small global state file under the Matty config directory in the active HOME.
- [ ] State records managed skill names, configured CLI surfaces, Matty version, and relevant paths/metadata without storing large prompt content.
- [ ] `matty install --dry-run` reports planned actions without writing files, creating symlinks, or running external install/setup commands.
- [ ] Re-running dry-run after no changes reports the same planned actions and leaves the sandbox unchanged.
- [ ] Corrupt or missing state is handled with clear errors or safe defaults.

## Blocked by

- .scratch/matty-v0/issues/01-scaffold-go-cobra-cli.md
