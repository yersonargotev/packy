Status: ready-for-agent

# Implement read-only doctor and conflict checks

## Parent

.scratch/matty-v0/PRD.md

## What to build

Implement `matty doctor` as a read-only health check for the full v0 setup. It should verify Matty state, global skills, Engram availability/setup, Codex prompt markers, OpenCode config, and Gentle AI conflict signals without changing files.

## Acceptance criteria

- [ ] `matty doctor` never writes files, creates symlinks, or runs setup/install/update commands.
- [ ] Doctor reports pass/warn/fail status for Matty state, skill symlinks, Engram binary, Engram setup expectations, Codex config, and OpenCode config.
- [ ] Doctor warns when `gentle-ai:*` markers or known Gentle AI OpenCode overlays are present.
- [ ] Doctor output gives actionable next steps, such as running `matty install`, `matty update`, or inspecting conflicts.
- [ ] Tests prove doctor is read-only by comparing sandbox state before and after execution.

## Blocked by

- .scratch/matty-v0/issues/03-sync-global-skill-symlinks.md
- .scratch/matty-v0/issues/04-install-update-and-setup-engram.md
- .scratch/matty-v0/issues/05-inject-codex-matty-prompts.md
- .scratch/matty-v0/issues/06-configure-opencode-matty-prompt.md
