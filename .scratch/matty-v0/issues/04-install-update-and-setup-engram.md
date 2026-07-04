Status: ready-for-agent

# Install, update, and configure Engram lifecycle

## Parent

.scratch/matty-v0/PRD.md

## What to build

Implement Matty's Engram lifecycle wrapper. On macOS, Matty should install or update Engram using the official Homebrew path, then delegate CLI integration to `engram setup codex` and `engram setup opencode`. The behavior must be testable with a fake command runner.

## Acceptance criteria

- [ ] `matty install` checks for `engram` and plans/runs the official install path when missing.
- [ ] `matty update` plans/runs the official update path for Engram.
- [ ] `matty install` and `matty update` run or plan `engram setup codex` and `engram setup opencode` after Engram is available.
- [ ] External command failures produce clear actionable errors.
- [ ] Dry-run reports external commands without executing them.
- [ ] Tests do not require Homebrew or Engram to be installed on the machine.

## Blocked by

- .scratch/matty-v0/issues/02-manage-matty-state-and-dry-run.md
