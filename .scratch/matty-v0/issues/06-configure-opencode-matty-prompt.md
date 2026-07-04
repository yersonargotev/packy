Status: ready-for-agent

# Configure OpenCode Matty prompt layer

## Parent

.scratch/matty-v0/PRD.md

## What to build

Add OpenCode global configuration for Matty's small always-on layer. Matty should write a managed Matty prompt file where appropriate and merge a safe reference or prompt entry into the OpenCode global config without clobbering user config, Engram setup, or Gentle AI overlays.

## Acceptance criteria

- [ ] `matty install` creates/updates a small Matty prompt for OpenCode in the sandboxed OpenCode config area.
- [ ] OpenCode config is merged, not replaced.
- [ ] Existing `mcp`, provider, model, plugin, and Gentle AI agent/profile config is preserved.
- [ ] Prompt content points to global skills, `ask-matt`, Engram memory, and delegation conventions.
- [ ] Re-running install/update is idempotent.
- [ ] `matty uninstall` removes only Matty-managed OpenCode prompt/config entries.
- [ ] Tests cover missing config, existing config, existing Gentle AI overlay, and dry-run.

## Blocked by

- .scratch/matty-v0/issues/02-manage-matty-state-and-dry-run.md
- .scratch/matty-v0/issues/03-sync-global-skill-symlinks.md
- .scratch/matty-v0/issues/04-install-update-and-setup-engram.md
