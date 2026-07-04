Status: ready-for-agent

# Inject Codex Matty prompt markers

## Parent

.scratch/matty-v0/PRD.md

## What to build

Add Codex global prompt injection for Matty's small always-on layer. Matty should manage only its own marker blocks in the Codex global instruction file, point Codex at global skills and `ask-matt`, and avoid editing Engram or Gentle AI managed content.

## Acceptance criteria

- [ ] `matty install` inserts or updates Matty-owned marker blocks in the sandboxed Codex global prompt file.
- [ ] The prompt content is small and tells Codex where global skills live, to use `ask-matt` as router, to use Engram memory, and to apply host delegation rules when available.
- [ ] Existing user content, Engram content, and `gentle-ai:*` blocks are preserved exactly outside Matty markers.
- [ ] Re-running install/update is idempotent.
- [ ] `matty uninstall` removes only Matty marker blocks.
- [ ] Tests cover insert, update, remove, existing Gentle AI markers, and dry-run.

## Blocked by

- .scratch/matty-v0/issues/02-manage-matty-state-and-dry-run.md
- .scratch/matty-v0/issues/03-sync-global-skill-symlinks.md
- .scratch/matty-v0/issues/04-install-update-and-setup-engram.md
