Status: ready-for-agent

# Complete update and uninstall flows

## Parent

.scratch/matty-v0/PRD.md

## What to build

Finish `matty update` and `matty uninstall` as complete lifecycle commands. Update should refresh all Matty-managed artifacts idempotently. Uninstall should remove only Matty-managed symlinks, marker blocks, prompt entries/files, and state while leaving Engram, Gentle AI, and unmanaged user content intact.

## Acceptance criteria

- [ ] `matty update` refreshes Engram lifecycle, skill symlinks, Matty prompts/config, and state without duplicating artifacts.
- [ ] `matty uninstall` removes Matty-managed skill symlinks.
- [ ] `matty uninstall` removes Matty marker blocks and Matty OpenCode prompt/config entries only.
- [ ] `matty uninstall` leaves Engram installed and leaves Engram/Gentle AI/user content untouched.
- [ ] Re-running uninstall is safe and reports no-op where appropriate.
- [ ] Tests cover update idempotency, uninstall safety, dry-run, and partial/missing Matty state.

## Blocked by

- .scratch/matty-v0/issues/05-inject-codex-matty-prompts.md
- .scratch/matty-v0/issues/06-configure-opencode-matty-prompt.md
