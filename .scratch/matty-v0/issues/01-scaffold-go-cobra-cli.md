Status: ready-for-agent

# Scaffold Go+Cobra CLI with sandboxable execution

## Parent

.scratch/matty-v0/PRD.md

## What to build

Create the initial Matty Go module and Cobra command structure with `install`, `doctor`, `update`, and `uninstall` commands. Add a test harness that can run commands against a sandboxed HOME and injected command runner so future issues never need to touch the real user configuration.

## Acceptance criteria

- [ ] `go test ./...` passes from the Matty root.
- [ ] `matty --help` and each v0 subcommand help render successfully.
- [ ] Commands can resolve HOME/config paths from an injected environment in tests.
- [ ] A fake command runner can be injected for tests without executing real `brew`, `engram`, `codex`, or `opencode` commands.
- [ ] No command mutates the real HOME in tests.

## Blocked by

None - can start immediately
